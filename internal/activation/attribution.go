package activation

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const redisSegmentKeyPrefix = "segment:users:"

// TargetingCheckRequest DSP 查询请求
type TargetingCheckRequest struct {
	UserIDHash string  `json:"user_id_hash"`
	SegmentIDs []int64 `json:"segment_ids"`
}

// TargetingCheckResult 查询结果
type TargetingCheckResult struct {
	MatchedSegmentIDs []int64 `json:"matched_segment_ids"`
}

// AttributionService 受众包命中归因服务（供 DSP 查询）
type AttributionService struct {
	redis *redis.Client
}

// NewAttributionService 创建归因服务
func NewAttributionService(rdb *redis.Client) *AttributionService {
	return &AttributionService{redis: rdb}
}

// CheckTargeting 检查用户命中哪些受众包（高频，Redis O(1) SET 查询）
func (s *AttributionService) CheckTargeting(ctx context.Context, req TargetingCheckRequest) (*TargetingCheckResult, error) {
	if req.UserIDHash == "" {
		return nil, fmt.Errorf("user_id_hash is required")
	}
	if len(req.SegmentIDs) == 0 {
		return &TargetingCheckResult{MatchedSegmentIDs: []int64{}}, nil
	}

	// 使用 Redis pipeline 并行查询多个 segment
	pipe := s.redis.Pipeline()
	cmds := make([]*redis.BoolCmd, len(req.SegmentIDs))
	for i, segID := range req.SegmentIDs {
		key := fmt.Sprintf("%s%d", redisSegmentKeyPrefix, segID)
		cmds[i] = pipe.SIsMember(ctx, key, req.UserIDHash)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("redis pipeline sismember: %w", err)
	}

	matched := make([]int64, 0, len(req.SegmentIDs))
	for i, cmd := range cmds {
		if cmd.Val() {
			matched = append(matched, req.SegmentIDs[i])
		}
	}

	return &TargetingCheckResult{MatchedSegmentIDs: matched}, nil
}
