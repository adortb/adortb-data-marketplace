package billing

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// UsageRecord 使用记录
type UsageRecord struct {
	ID         int64     `json:"id"`
	SegmentID  int64     `json:"segment_id"`
	CampaignID *int64    `json:"campaign_id,omitempty"`
	Date       time.Time `json:"date"`
	Impressions int64    `json:"impressions"`
	Fees       float64   `json:"fees"`
}

// UsageRepository 使用记录存储接口
type UsageRepository interface {
	Upsert(ctx context.Context, r *UsageRecord) error
	ListBySegment(ctx context.Context, segmentID int64, from, to time.Time) ([]*UsageRecord, error)
	ListByCampaign(ctx context.Context, campaignID int64, from, to time.Time) ([]*UsageRecord, error)
	// ListBySegmentIDs 按多个受众包 ID 查询（用于 provider 级别聚合）
	ListBySegmentIDs(ctx context.Context, segmentIDs []int64, from, to time.Time) ([]*UsageRecord, error)
}

// UsageTracker 使用次数记录（高并发，按天聚合）
type UsageTracker struct {
	repo UsageRepository
}

// NewUsageTracker 创建使用记录追踪器
func NewUsageTracker(repo UsageRepository) *UsageTracker {
	return &UsageTracker{repo: repo}
}

// TrackImpression 记录一次 impression 使用
func (t *UsageTracker) TrackImpression(ctx context.Context, segmentID int64, campaignID *int64, fee float64) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	record := &UsageRecord{
		SegmentID:   segmentID,
		CampaignID:  campaignID,
		Date:        today,
		Impressions: 1,
		Fees:        fee,
	}
	if err := t.repo.Upsert(ctx, record); err != nil {
		return fmt.Errorf("track impression: %w", err)
	}
	return nil
}

// TrackBatch 批量记录 impression（高性能批量接口）
func (t *UsageTracker) TrackBatch(ctx context.Context, segmentID int64, campaignID *int64, impressions int64, totalFee float64) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	record := &UsageRecord{
		SegmentID:   segmentID,
		CampaignID:  campaignID,
		Date:        today,
		Impressions: impressions,
		Fees:        totalFee,
	}
	if err := t.repo.Upsert(ctx, record); err != nil {
		return fmt.Errorf("track batch: %w", err)
	}
	return nil
}

// GetCampaignUsage 获取活动使用情况
func (t *UsageTracker) GetCampaignUsage(ctx context.Context, campaignID int64, from, to time.Time) ([]*UsageRecord, error) {
	return t.repo.ListByCampaign(ctx, campaignID, from, to)
}

// PGUsageRepository PostgreSQL 实现
type PGUsageRepository struct {
	db *sql.DB
}

// NewPGUsageRepository 创建PG存储
func NewPGUsageRepository(db *sql.DB) *PGUsageRepository {
	return &PGUsageRepository{db: db}
}

// Upsert 按天聚合 upsert（原子性 ON CONFLICT 累加）
func (r *PGUsageRepository) Upsert(ctx context.Context, record *UsageRecord) error {
	var campaignID interface{}
	if record.CampaignID != nil {
		campaignID = *record.CampaignID
	}

	const q = `
		INSERT INTO segment_usage (segment_id, campaign_id, date, impressions, fees)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (segment_id, campaign_id, date)
		DO UPDATE SET
			impressions = segment_usage.impressions + EXCLUDED.impressions,
			fees        = segment_usage.fees + EXCLUDED.fees`

	_, err := r.db.ExecContext(ctx, q,
		record.SegmentID, campaignID, record.Date.Format("2006-01-02"),
		record.Impressions, record.Fees,
	)
	if err != nil {
		return fmt.Errorf("upsert segment_usage: %w", err)
	}
	return nil
}

func (r *PGUsageRepository) ListBySegment(ctx context.Context, segmentID int64, from, to time.Time) ([]*UsageRecord, error) {
	const q = `
		SELECT id, segment_id, campaign_id, date, impressions, fees
		FROM segment_usage
		WHERE segment_id = $1 AND date >= $2 AND date <= $3
		ORDER BY date`

	return r.queryUsage(ctx, q, segmentID, from.Format("2006-01-02"), to.Format("2006-01-02"))
}

func (r *PGUsageRepository) ListByCampaign(ctx context.Context, campaignID int64, from, to time.Time) ([]*UsageRecord, error) {
	const q = `
		SELECT id, segment_id, campaign_id, date, impressions, fees
		FROM segment_usage
		WHERE campaign_id = $1 AND date >= $2 AND date <= $3
		ORDER BY date`

	return r.queryUsage(ctx, q, campaignID, from.Format("2006-01-02"), to.Format("2006-01-02"))
}

func (r *PGUsageRepository) ListBySegmentIDs(ctx context.Context, segmentIDs []int64, from, to time.Time) ([]*UsageRecord, error) {
	if len(segmentIDs) == 0 {
		return nil, nil
	}
	// 构建 IN 子句
	placeholders := ""
	args := []any{from.Format("2006-01-02"), to.Format("2006-01-02")}
	for i, id := range segmentIDs {
		if i > 0 {
			placeholders += ","
		}
		args = append(args, id)
		placeholders += fmt.Sprintf("$%d", len(args))
	}
	q := fmt.Sprintf(`
		SELECT id, segment_id, campaign_id, date, impressions, fees
		FROM segment_usage
		WHERE date >= $1 AND date <= $2 AND segment_id IN (%s)
		ORDER BY date`, placeholders)
	return r.queryUsage(ctx, q, args...)
}

func (r *PGUsageRepository) queryUsage(ctx context.Context, q string, args ...any) ([]*UsageRecord, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query segment_usage: %w", err)
	}
	defer rows.Close()

	var records []*UsageRecord
	for rows.Next() {
		rec := &UsageRecord{}
		var campaignID sql.NullInt64
		var date string
		if err := rows.Scan(&rec.ID, &rec.SegmentID, &campaignID, &date, &rec.Impressions, &rec.Fees); err != nil {
			return nil, fmt.Errorf("scan usage row: %w", err)
		}
		if campaignID.Valid {
			rec.CampaignID = &campaignID.Int64
		}
		rec.Date, _ = time.Parse("2006-01-02", date)
		records = append(records, rec)
	}
	return records, rows.Err()
}
