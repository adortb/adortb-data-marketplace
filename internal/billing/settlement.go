package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/adortb/adortb-data-marketplace/internal/provider"
)

// SettlementReport 月度对账报告
type SettlementReport struct {
	ProviderID      int64     `json:"provider_id"`
	ProviderName    string    `json:"provider_name"`
	Period          string    `json:"period"` // "2025-01"
	TotalFees       float64   `json:"total_fees"`       // 总应收
	ProviderEarning float64   `json:"provider_earning"` // 数据商收入
	PlatformRevenue float64   `json:"platform_revenue"` // 平台收入
	RevshareRate    float64   `json:"revshare_rate"`
	GeneratedAt     time.Time `json:"generated_at"`
}

// SegmentSettlement 受众包级别结算
type SegmentSettlement struct {
	SegmentID   int64   `json:"segment_id"`
	Impressions int64   `json:"impressions"`
	TotalFees   float64 `json:"total_fees"`
}

// ProviderEarningsQuery 收益查询
type ProviderEarningsQuery struct {
	ProviderID int64
	From       time.Time
	To         time.Time
}

// ProviderEarningsResult 收益结果
type ProviderEarningsResult struct {
	ProviderID      int64                `json:"provider_id"`
	TotalImpressions int64              `json:"total_impressions"`
	TotalFees       float64             `json:"total_fees"`
	ProviderEarning float64             `json:"provider_earning"`
	BySegment       []*SegmentSettlement `json:"by_segment"`
}

// SegmentLister 查询 provider 旗下所有 segment ID 的接口
type SegmentLister interface {
	ListSegmentIDsByProvider(ctx context.Context, providerID int64) ([]int64, error)
}

// SettlementService 月度对账结算服务
type SettlementService struct {
	usageRepo    UsageRepository
	providerRepo provider.Repository
	segLister    SegmentLister
}

// NewSettlementService 创建结算服务
func NewSettlementService(usageRepo UsageRepository, providerRepo provider.Repository, segLister SegmentLister) *SettlementService {
	return &SettlementService{
		usageRepo:    usageRepo,
		providerRepo: providerRepo,
		segLister:    segLister,
	}
}

// RunMonthlySettlement 运行月度结算（按数据提供方）
func (s *SettlementService) RunMonthlySettlement(ctx context.Context, providerID int64, year int, month time.Month) (*SettlementReport, error) {
	p, err := s.providerRepo.GetByID(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	from := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0).Add(-time.Second) // 月末

	segmentIDs, err := s.segLister.ListSegmentIDsByProvider(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("list segment ids: %w", err)
	}

	records, err := s.usageRepo.ListBySegmentIDs(ctx, segmentIDs, from, to)
	if err != nil {
		return nil, fmt.Errorf("list usage: %w", err)
	}

	totalFees := 0.0
	for _, r := range records {
		totalFees += r.Fees
	}

	providerEarning := totalFees * p.RevshareRate
	platformRevenue := totalFees - providerEarning

	return &SettlementReport{
		ProviderID:      providerID,
		ProviderName:    p.Name,
		Period:          fmt.Sprintf("%d-%02d", year, month),
		TotalFees:       totalFees,
		ProviderEarning: providerEarning,
		PlatformRevenue: platformRevenue,
		RevshareRate:    p.RevshareRate,
		GeneratedAt:     time.Now(),
	}, nil
}

// GetProviderEarnings 获取数据商收益（供 API 使用）
func (s *SettlementService) GetProviderEarnings(ctx context.Context, q ProviderEarningsQuery) (*ProviderEarningsResult, error) {
	p, err := s.providerRepo.GetByID(ctx, q.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	segmentIDs, err := s.segLister.ListSegmentIDsByProvider(ctx, q.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("list segment ids: %w", err)
	}

	records, err := s.usageRepo.ListBySegmentIDs(ctx, segmentIDs, q.From, q.To)
	if err != nil {
		return nil, fmt.Errorf("list usage by provider: %w", err)
	}

	// 按 segment 聚合
	segMap := map[int64]*SegmentSettlement{}
	totalImpressions := int64(0)
	totalFees := 0.0

	for _, r := range records {
		if _, ok := segMap[r.SegmentID]; !ok {
			segMap[r.SegmentID] = &SegmentSettlement{SegmentID: r.SegmentID}
		}
		segMap[r.SegmentID].Impressions += r.Impressions
		segMap[r.SegmentID].TotalFees += r.Fees
		totalImpressions += r.Impressions
		totalFees += r.Fees
	}

	bySegment := make([]*SegmentSettlement, 0, len(segMap))
	for _, ss := range segMap {
		bySegment = append(bySegment, ss)
	}

	return &ProviderEarningsResult{
		ProviderID:       q.ProviderID,
		TotalImpressions: totalImpressions,
		TotalFees:        totalFees,
		ProviderEarning:  totalFees * p.RevshareRate,
		BySegment:        bySegment,
	}, nil
}
