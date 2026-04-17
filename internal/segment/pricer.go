package segment

import "fmt"

// PricingType 定价类型
type PricingType string

const (
	PricingCPM      PricingType = "cpm"  // 按千次收费
	PricingFlatFee  PricingType = "flat" // 固定月费
)

// PricingStrategy 定价策略接口
type PricingStrategy interface {
	// CalculateFee 计算费用
	CalculateFee(impressions int64) float64
	Type() PricingType
}

// CPMStrategy CPM 定价策略
type CPMStrategy struct {
	cpmFee float64 // 每千次费用
}

// NewCPMStrategy 创建 CPM 定价
func NewCPMStrategy(cpmFee float64) *CPMStrategy {
	return &CPMStrategy{cpmFee: cpmFee}
}

func (s *CPMStrategy) CalculateFee(impressions int64) float64 {
	return float64(impressions) / 1000.0 * s.cpmFee
}

func (s *CPMStrategy) Type() PricingType {
	return PricingCPM
}

// FlatFeeStrategy 固定费用策略（月费）
type FlatFeeStrategy struct {
	flatFee float64
}

// NewFlatFeeStrategy 创建固定费用策略
func NewFlatFeeStrategy(flatFee float64) *FlatFeeStrategy {
	return &FlatFeeStrategy{flatFee: flatFee}
}

func (s *FlatFeeStrategy) CalculateFee(_ int64) float64 {
	return s.flatFee
}

func (s *FlatFeeStrategy) Type() PricingType {
	return PricingFlatFee
}

// Pricer 受众包定价器
type Pricer struct{}

// NewPricer 创建定价器
func NewPricer() *Pricer {
	return &Pricer{}
}

// GetStrategy 根据受众包获取定价策略
func (p *Pricer) GetStrategy(seg *Segment) (PricingStrategy, error) {
	if seg.FlatFee != nil && *seg.FlatFee > 0 {
		return NewFlatFeeStrategy(*seg.FlatFee), nil
	}
	if seg.CPMFee > 0 {
		return NewCPMStrategy(seg.CPMFee), nil
	}
	return nil, fmt.Errorf("segment %d has no valid pricing", seg.ID)
}

// CalculateFee 计算受众包使用费用
func (p *Pricer) CalculateFee(seg *Segment, impressions int64) (float64, error) {
	strategy, err := p.GetStrategy(seg)
	if err != nil {
		return 0, err
	}
	return strategy.CalculateFee(impressions), nil
}
