package activation_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/adortb/adortb-data-marketplace/internal/activation"
	"github.com/adortb/adortb-data-marketplace/internal/segment"
)

// mockActivationRepo 内存模拟
type mockActivationRepo struct {
	records []*activation.Activation
	nextID  int64
}

func newMockActivationRepo() *mockActivationRepo {
	return &mockActivationRepo{nextID: 1}
}

func (r *mockActivationRepo) Create(_ context.Context, a *activation.Activation) (*activation.Activation, error) {
	created := *a
	created.ID = r.nextID
	created.ActivatedAt = time.Now()
	r.nextID++
	r.records = append(r.records, &created)
	return &created, nil
}

func (r *mockActivationRepo) ListByCampaign(_ context.Context, campaignID int64) ([]*activation.Activation, error) {
	var result []*activation.Activation
	for _, a := range r.records {
		if a.CampaignID == campaignID {
			result = append(result, a)
		}
	}
	return result, nil
}

// mockSegmentCatalog 内存模拟
type mockSegmentRepo struct {
	segments map[int64]*segment.Segment
}

func newMockSegmentCatalogRepo() *mockSegmentRepo {
	return &mockSegmentRepo{
		segments: map[int64]*segment.Segment{
			1: {ID: 1, Name: "高消费女性", Status: segment.StatusApproved, CPMFee: 2.0},
			2: {ID: 2, Name: "汽车买家", Status: segment.StatusDraft, CPMFee: 3.0},
		},
	}
}

func (r *mockSegmentRepo) Create(_ context.Context, s *segment.Segment) (*segment.Segment, error) {
	return s, nil
}
func (r *mockSegmentRepo) GetByID(_ context.Context, id int64) (*segment.Segment, error) {
	s, ok := r.segments[id]
	if !ok {
		return nil, fmt.Errorf("segment %d not found", id)
	}
	return s, nil
}
func (r *mockSegmentRepo) UpdateStatus(_ context.Context, _ int64, _ segment.Status, _ *time.Time) error {
	return nil
}
func (r *mockSegmentRepo) UpdateUserCount(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (r *mockSegmentRepo) List(_ context.Context, _ segment.ListFilter) ([]*segment.Segment, error) {
	return nil, nil
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

func TestActivator_Activate(t *testing.T) {
	actRepo := newMockActivationRepo()
	segRepo := newMockSegmentCatalogRepo()
	catalog := segment.NewCatalog(segRepo)
	activator := activation.NewActivator(actRepo, catalog)
	ctx := context.Background()

	t.Run("activate approved segment", func(t *testing.T) {
		req := activation.ActivateRequest{
			SegmentID: 1, // StatusApproved
			Operator:  activation.OperatorInclude,
		}
		act, err := activator.Activate(ctx, 100, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if act.CampaignID != 100 {
			t.Errorf("expected campaign_id 100, got %d", act.CampaignID)
		}
		if act.Operator != activation.OperatorInclude {
			t.Errorf("expected include operator, got %s", act.Operator)
		}
	})

	t.Run("cannot activate unapproved segment", func(t *testing.T) {
		req := activation.ActivateRequest{
			SegmentID: 2, // StatusDraft
		}
		_, err := activator.Activate(ctx, 100, req)
		if err == nil {
			t.Error("expected error for unapproved segment")
		}
	})

	t.Run("invalid operator fails", func(t *testing.T) {
		req := activation.ActivateRequest{
			SegmentID: 1,
			Operator:  "invalid",
		}
		_, err := activator.Activate(ctx, 100, req)
		if err == nil {
			t.Error("expected error for invalid operator")
		}
	})

	t.Run("default operator is include", func(t *testing.T) {
		req := activation.ActivateRequest{SegmentID: 1}
		act, err := activator.Activate(ctx, 200, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if act.Operator != activation.OperatorInclude {
			t.Errorf("expected default include, got %s", act.Operator)
		}
	})
}

func TestActivator_ListByCampaign(t *testing.T) {
	actRepo := newMockActivationRepo()
	segRepo := newMockSegmentCatalogRepo()
	catalog := segment.NewCatalog(segRepo)
	activator := activation.NewActivator(actRepo, catalog)
	ctx := context.Background()

	// 激活两个受众包到同一活动
	activator.Activate(ctx, 300, activation.ActivateRequest{SegmentID: 1}) //nolint:errcheck
	activator.Activate(ctx, 300, activation.ActivateRequest{SegmentID: 1}) //nolint:errcheck
	activator.Activate(ctx, 400, activation.ActivateRequest{SegmentID: 1}) //nolint:errcheck

	activations, err := activator.ListByCampaign(ctx, 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(activations) != 2 {
		t.Errorf("expected 2 activations for campaign 300, got %d", len(activations))
	}
}
