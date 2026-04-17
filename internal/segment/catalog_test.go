package segment_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/adortb/adortb-data-marketplace/internal/segment"
)

// mockSegmentRepo 内存模拟存储
type mockSegmentRepo struct {
	segments map[int64]*segment.Segment
	nextID   int64
}

func newMockSegmentRepo() *mockSegmentRepo {
	return &mockSegmentRepo{
		segments: make(map[int64]*segment.Segment),
		nextID:   1,
	}
}

func (r *mockSegmentRepo) Create(_ context.Context, s *segment.Segment) (*segment.Segment, error) {
	created := *s
	created.ID = r.nextID
	created.CreatedAt = time.Now()
	r.nextID++
	r.segments[created.ID] = &created
	return &created, nil
}

func (r *mockSegmentRepo) GetByID(_ context.Context, id int64) (*segment.Segment, error) {
	s, ok := r.segments[id]
	if !ok {
		return nil, fmt.Errorf("segment %d not found", id)
	}
	return s, nil
}

func (r *mockSegmentRepo) UpdateStatus(_ context.Context, id int64, status segment.Status, approvedAt *time.Time) error {
	s, ok := r.segments[id]
	if !ok {
		return errors.New("not found")
	}
	s.Status = status
	s.ApprovedAt = approvedAt
	return nil
}

func (r *mockSegmentRepo) UpdateUserCount(_ context.Context, id int64, count int64) error {
	s, ok := r.segments[id]
	if !ok {
		return errors.New("not found")
	}
	s.UserCount = count
	return nil
}

func (r *mockSegmentRepo) ListIDsByProvider(_ context.Context, providerID int64) ([]int64, error) {
	var ids []int64
	for _, s := range r.segments {
		if s.ProviderID == providerID {
			ids = append(ids, s.ID)
		}
	}
	return ids, nil
}

func (r *mockSegmentRepo) List(_ context.Context, filter segment.ListFilter) ([]*segment.Segment, error) {
	var result []*segment.Segment
	for _, s := range r.segments {
		if filter.Status != "" && s.Status != filter.Status {
			continue
		}
		if filter.Category != "" && s.Category != filter.Category {
			continue
		}
		if filter.MinSize > 0 && s.UserCount < filter.MinSize {
			continue
		}
		result = append(result, s)
	}
	return result, nil
}

func TestCatalog_CreateSegment(t *testing.T) {
	repo := newMockSegmentRepo()
	catalog := segment.NewCatalog(repo)
	ctx := context.Background()

	t.Run("valid segment creation", func(t *testing.T) {
		req := segment.CreateRequest{
			SegmentID:   "seg-001",
			Name:        "高消费女性",
			Description: "25-45岁高消费能力女性用户",
			Category:    segment.CategoryDemographic,
			CPMFee:      2.50,
			RecencyDays: 30,
		}

		seg, err := catalog.CreateSegment(ctx, 1, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seg.ID == 0 {
			t.Error("expected non-zero ID")
		}
		if seg.Status != segment.StatusDraft {
			t.Errorf("expected draft status, got %s", seg.Status)
		}
		if seg.Name != req.Name {
			t.Errorf("expected name %s, got %s", req.Name, seg.Name)
		}
	})

	t.Run("missing segment_id fails", func(t *testing.T) {
		req := segment.CreateRequest{Name: "test", CPMFee: 1.0}
		_, err := catalog.CreateSegment(ctx, 1, req)
		if err == nil {
			t.Error("expected error for missing segment_id")
		}
	})

	t.Run("missing name fails", func(t *testing.T) {
		req := segment.CreateRequest{SegmentID: "seg-002", CPMFee: 1.0}
		_, err := catalog.CreateSegment(ctx, 1, req)
		if err == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("zero CPM fee fails", func(t *testing.T) {
		req := segment.CreateRequest{SegmentID: "seg-003", Name: "test", CPMFee: 0}
		_, err := catalog.CreateSegment(ctx, 1, req)
		if err == nil {
			t.Error("expected error for zero CPM fee")
		}
	})
}

func TestCatalog_Approve(t *testing.T) {
	repo := newMockSegmentRepo()
	catalog := segment.NewCatalog(repo)
	ctx := context.Background()

	// 先创建一个受众包
	req := segment.CreateRequest{
		SegmentID: "seg-approve-01",
		Name:      "汽车潜在购买者",
		CPMFee:    3.0,
	}
	created, _ := catalog.CreateSegment(ctx, 1, req)

	t.Run("approve draft segment", func(t *testing.T) {
		if err := catalog.Approve(ctx, created.ID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		seg, _ := catalog.GetByID(ctx, created.ID)
		if seg.Status != segment.StatusApproved {
			t.Errorf("expected approved, got %s", seg.Status)
		}
		if seg.ApprovedAt == nil {
			t.Error("expected approved_at to be set")
		}
	})

	t.Run("cannot approve non-draft segment", func(t *testing.T) {
		// 已审批的不能再次审批
		err := catalog.Approve(ctx, created.ID)
		if err == nil {
			t.Error("expected error when approving non-draft segment")
		}
	})

	t.Run("approve non-existent segment fails", func(t *testing.T) {
		err := catalog.Approve(ctx, 99999)
		if err == nil {
			t.Error("expected error for non-existent segment")
		}
	})
}

func TestCatalog_List(t *testing.T) {
	repo := newMockSegmentRepo()
	catalog := segment.NewCatalog(repo)
	ctx := context.Background()

	// 创建并审批多个受众包
	segs := []segment.CreateRequest{
		{SegmentID: "s1", Name: "S1", Category: segment.CategoryDemographic, CPMFee: 1.0},
		{SegmentID: "s2", Name: "S2", Category: segment.CategoryBehavioral, CPMFee: 2.0},
		{SegmentID: "s3", Name: "S3", Category: segment.CategoryDemographic, CPMFee: 1.5},
	}

	for _, req := range segs {
		created, _ := catalog.CreateSegment(ctx, 1, req)
		catalog.Approve(ctx, created.ID) //nolint:errcheck
	}

	t.Run("filter by category", func(t *testing.T) {
		filter := segment.ListFilter{
			Category: segment.CategoryDemographic,
			Status:   segment.StatusApproved,
			Limit:    10,
		}
		result, err := catalog.List(ctx, filter)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("expected 2 demographic segments, got %d", len(result))
		}
	})
}
