package segment

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	redisSegmentKeyPrefix = "segment:users:"
	uploadBatchSize       = 500
)

// UserRecord 用户记录
type UserRecord struct {
	UserIDHash string `json:"user_id_hash"`
}

// UploadResult 上传结果
type UploadResult struct {
	Total   int64 `json:"total"`
	Succeed int64 `json:"succeed"`
	Failed  int64 `json:"failed"`
}

// Uploader 受众包用户列表上传器
type Uploader struct {
	db      *sql.DB
	redis   *redis.Client
	catalog *Catalog
}

// NewUploader 创建上传器
func NewUploader(db *sql.DB, rdb *redis.Client, catalog *Catalog) *Uploader {
	return &Uploader{db: db, redis: rdb, catalog: catalog}
}

// UploadStream 流式上传用户列表（支持大文件）
// 格式：每行一个 JSON {"user_id_hash":"xxx"}
func (u *Uploader) UploadStream(ctx context.Context, segmentID int64, r io.Reader) (*UploadResult, error) {
	seg, err := u.catalog.GetByID(ctx, segmentID)
	if err != nil {
		return nil, fmt.Errorf("segment not found: %w", err)
	}
	if seg.Status != StatusApproved {
		return nil, fmt.Errorf("segment %d is not approved", segmentID)
	}

	result := &UploadResult{}
	batch := make([]string, 0, uploadBatchSize)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1MB 缓冲

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec UserRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			atomic.AddInt64(&result.Failed, 1)
			continue
		}
		if rec.UserIDHash == "" {
			atomic.AddInt64(&result.Failed, 1)
			continue
		}

		batch = append(batch, rec.UserIDHash)
		atomic.AddInt64(&result.Total, 1)

		if len(batch) >= uploadBatchSize {
			succeed, err := u.flushBatch(ctx, segmentID, batch)
			atomic.AddInt64(&result.Succeed, int64(succeed))
			if err != nil {
				atomic.AddInt64(&result.Failed, int64(len(batch)-succeed))
			}
			batch = batch[:0]
		}
	}

	// 处理最后一批
	if len(batch) > 0 {
		succeed, err := u.flushBatch(ctx, segmentID, batch)
		atomic.AddInt64(&result.Succeed, int64(succeed))
		if err != nil {
			atomic.AddInt64(&result.Failed, int64(len(batch)-succeed))
		}
	}

	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("scan upload stream: %w", err)
	}

	// 更新受众包用户数量
	if err := u.catalog.repo.UpdateUserCount(ctx, segmentID, result.Succeed); err != nil {
		return result, fmt.Errorf("update user count: %w", err)
	}

	return result, nil
}

// flushBatch 批量写入 Redis 和 PG
func (u *Uploader) flushBatch(ctx context.Context, segmentID int64, hashes []string) (int, error) {
	redisKey := fmt.Sprintf("%s%d", redisSegmentKeyPrefix, segmentID)

	// Redis pipeline 批量写入
	pipe := u.redis.Pipeline()
	for _, h := range hashes {
		pipe.SAdd(ctx, redisKey, h)
	}
	pipe.Expire(ctx, redisKey, 30*24*time.Hour) // 30天过期
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("redis pipeline sadd: %w", err)
	}

	// PG fallback 批量 upsert
	succeed, err := u.batchUpsertPG(ctx, segmentID, hashes)
	return succeed, err
}

// batchUpsertPG 批量 upsert 到 PG（作为 fallback 持久化）
func (u *Uploader) batchUpsertPG(ctx context.Context, segmentID int64, hashes []string) (int, error) {
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO segment_users (segment_id, user_id_hash)
		VALUES ($1, $2)
		ON CONFLICT (segment_id, user_id_hash) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("prepare segment_users insert: %w", err)
	}
	defer stmt.Close()

	succeed := 0
	for _, h := range hashes {
		if _, err := stmt.ExecContext(ctx, segmentID, h); err != nil {
			continue
		}
		succeed++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit segment_users: %w", err)
	}
	return succeed, nil
}
