package activation_test

import (
	"context"
	"testing"

	"github.com/adortb/adortb-data-marketplace/internal/activation"
	"github.com/redis/go-redis/v9"
)

func TestAttributionService_CheckTargeting_EmptySegments(t *testing.T) {
	// 不需要真实 Redis，空 segment_ids 直接返回
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	svc := activation.NewAttributionService(rdb)

	req := activation.TargetingCheckRequest{
		UserIDHash: "abc123",
		SegmentIDs: []int64{},
	}

	result, err := svc.CheckTargeting(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.MatchedSegmentIDs) != 0 {
		t.Errorf("expected empty matched segments, got %v", result.MatchedSegmentIDs)
	}
}

func TestAttributionService_CheckTargeting_EmptyUserHash(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	svc := activation.NewAttributionService(rdb)

	req := activation.TargetingCheckRequest{
		UserIDHash: "",
		SegmentIDs: []int64{1, 2},
	}

	_, err := svc.CheckTargeting(context.Background(), req)
	if err == nil {
		t.Error("expected error for empty user_id_hash")
	}
}
