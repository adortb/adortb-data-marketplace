package billing_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/adortb/adortb-data-marketplace/internal/billing"
	"github.com/adortb/adortb-data-marketplace/internal/provider"
)

// mockUsageRepository 内存模拟
type mockUsageRepository struct {
	records []*billing.UsageRecord
	nextID  int64
}

func newMockUsageRepository() *mockUsageRepository {
	return &mockUsageRepository{nextID: 1}
}

func (r *mockUsageRepository) Upsert(_ context.Context, rec *billing.UsageRecord) error {
	// 按 segment_id + campaign_id + date 查找并累加
	key := fmt.Sprintf("%d_%v_%s", rec.SegmentID, rec.CampaignID, rec.Date.Format("2006-01-02"))
	for _, existing := range r.records {
		eKey := fmt.Sprintf("%d_%v_%s", existing.SegmentID, existing.CampaignID, existing.Date.Format("2006-01-02"))
		if key == eKey {
			existing.Impressions += rec.Impressions
			existing.Fees += rec.Fees
			return nil
		}
	}
	created := *rec
	created.ID = r.nextID
	r.nextID++
	r.records = append(r.records, &created)
	return nil
}

func (r *mockUsageRepository) ListBySegment(_ context.Context, segmentID int64, from, to time.Time) ([]*billing.UsageRecord, error) {
	var result []*billing.UsageRecord
	for _, rec := range r.records {
		if rec.SegmentID == segmentID && !rec.Date.Before(from) && !rec.Date.After(to) {
			result = append(result, rec)
		}
	}
	return result, nil
}

func (r *mockUsageRepository) ListByCampaign(_ context.Context, campaignID int64, from, to time.Time) ([]*billing.UsageRecord, error) {
	var result []*billing.UsageRecord
	for _, rec := range r.records {
		if rec.CampaignID != nil && *rec.CampaignID == campaignID && !rec.Date.Before(from) && !rec.Date.After(to) {
			result = append(result, rec)
		}
	}
	return result, nil
}

func (r *mockUsageRepository) ListBySegmentIDs(_ context.Context, segmentIDs []int64, from, to time.Time) ([]*billing.UsageRecord, error) {
	idSet := make(map[int64]struct{}, len(segmentIDs))
	for _, id := range segmentIDs {
		idSet[id] = struct{}{}
	}
	var result []*billing.UsageRecord
	for _, rec := range r.records {
		if _, ok := idSet[rec.SegmentID]; ok && !rec.Date.Before(from) && !rec.Date.After(to) {
			result = append(result, rec)
		}
	}
	return result, nil
}

// mockProviderRepository 内存模拟
type mockProviderRepository struct {
	providers map[int64]*provider.Provider
}

func newMockProviderRepository() *mockProviderRepository {
	return &mockProviderRepository{
		providers: map[int64]*provider.Provider{
			1: {
				ID:           1,
				Name:         "Experian",
				RevshareRate: 0.70,
				Status:       provider.StatusApproved,
			},
		},
	}
}

func (r *mockProviderRepository) Create(_ context.Context, p *provider.Provider) (*provider.Provider, error) {
	return p, nil
}

func (r *mockProviderRepository) GetByID(_ context.Context, id int64) (*provider.Provider, error) {
	p, ok := r.providers[id]
	if !ok {
		return nil, fmt.Errorf("provider %d not found", id)
	}
	return p, nil
}

func (r *mockProviderRepository) GetByAPIKey(_ context.Context, _ string) (*provider.Provider, error) {
	return nil, fmt.Errorf("not found")
}

func (r *mockProviderRepository) UpdateStatus(_ context.Context, id int64, status provider.Status, approvedAt *time.Time) error {
	p, ok := r.providers[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	p.Status = status
	p.ApprovedAt = approvedAt
	return nil
}

func (r *mockProviderRepository) List(_ context.Context, _ provider.Status, _, _ int) ([]*provider.Provider, error) {
	var result []*provider.Provider
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result, nil
}

func TestUsageTracker_TrackBatch(t *testing.T) {
	repo := newMockUsageRepository()
	tracker := billing.NewUsageTracker(repo)
	ctx := context.Background()

	campaignID := int64(100)

	t.Run("track impressions accumulate", func(t *testing.T) {
		if err := tracker.TrackBatch(ctx, 1, &campaignID, 1000, 2.0); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := tracker.TrackBatch(ctx, 1, &campaignID, 500, 1.0); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		from := time.Now().AddDate(0, -1, 0)
		to := time.Now().AddDate(0, 1, 0)
		records, err := tracker.GetCampaignUsage(ctx, campaignID, from, to)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		totalImpressions := int64(0)
		totalFees := 0.0
		for _, r := range records {
			totalImpressions += r.Impressions
			totalFees += r.Fees
		}

		if totalImpressions != 1500 {
			t.Errorf("expected 1500 impressions, got %d", totalImpressions)
		}
		if totalFees != 3.0 {
			t.Errorf("expected $3.0 fees, got %f", totalFees)
		}
	})
}

// mockSegmentLister 模拟 provider -> segment ID 列表
type mockSegmentLister struct {
	// providerID -> segmentIDs
	data map[int64][]int64
}

func (m *mockSegmentLister) ListSegmentIDsByProvider(_ context.Context, providerID int64) ([]int64, error) {
	return m.data[providerID], nil
}

func TestSettlementService_RunMonthlySettlement(t *testing.T) {
	usageRepo := newMockUsageRepository()
	providerRepo := newMockProviderRepository()
	segLister := &mockSegmentLister{
		data: map[int64][]int64{
			1: {10, 11}, // provider 1 有两个 segment
		},
	}
	settlement := billing.NewSettlementService(usageRepo, providerRepo, segLister)
	ctx := context.Background()

	// 插入使用记录（segmentID=10 属于 provider 1）
	today := time.Now().UTC().Truncate(24 * time.Hour)
	usageRepo.Upsert(ctx, &billing.UsageRecord{ //nolint:errcheck
		SegmentID:   10,
		Date:        today,
		Impressions: 10000,
		Fees:        20.0,
	})

	t.Run("settlement calculates revshare correctly", func(t *testing.T) {
		report, err := settlement.RunMonthlySettlement(ctx, 1, today.Year(), today.Month())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if report.RevshareRate != 0.70 {
			t.Errorf("expected revshare 0.70, got %f", report.RevshareRate)
		}

		// 数据商获得 70%（浮点近似比较，误差 <0.0001）
		if diff := report.ProviderEarning - 14.0; diff < -0.0001 || diff > 0.0001 {
			t.Errorf("expected provider earning ~14.0, got %f", report.ProviderEarning)
		}

		// 平台获得 30%（= totalFees - providerEarning，避免浮点精度问题）
		if diff := report.PlatformRevenue - 6.0; diff < -0.0001 || diff > 0.0001 {
			t.Errorf("expected platform revenue ~6.0, got %f", report.PlatformRevenue)
		}

		if diff := report.TotalFees - 20.0; diff < -0.0001 || diff > 0.0001 {
			t.Errorf("expected total fees 20.0, got %f", report.TotalFees)
		}
	})

	t.Run("settlement for non-existent provider fails", func(t *testing.T) {
		_, err := settlement.RunMonthlySettlement(ctx, 99999, today.Year(), today.Month())
		if err == nil {
			t.Error("expected error for non-existent provider")
		}
	})
}
