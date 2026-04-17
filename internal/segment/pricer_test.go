package segment_test

import (
	"testing"

	"github.com/adortb/adortb-data-marketplace/internal/segment"
)

func TestCPMStrategy(t *testing.T) {
	tests := []struct {
		name        string
		cpmFee      float64
		impressions int64
		want        float64
	}{
		{"zero impressions", 1.0, 0, 0},
		{"1000 impressions at $1 CPM", 1.0, 1000, 1.0},
		{"500 impressions at $2 CPM", 2.0, 500, 1.0},
		{"10000 impressions at $0.50 CPM", 0.50, 10000, 5.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := segment.NewCPMStrategy(tt.cpmFee)
			got := s.CalculateFee(tt.impressions)
			if got != tt.want {
				t.Errorf("CalculateFee(%d) = %f, want %f", tt.impressions, got, tt.want)
			}
			if s.Type() != segment.PricingCPM {
				t.Errorf("expected type CPM, got %s", s.Type())
			}
		})
	}
}

func TestFlatFeeStrategy(t *testing.T) {
	s := segment.NewFlatFeeStrategy(100.0)

	// 固定费用不随 impressions 变化
	if got := s.CalculateFee(0); got != 100.0 {
		t.Errorf("expected 100.0, got %f", got)
	}
	if got := s.CalculateFee(999999); got != 100.0 {
		t.Errorf("expected 100.0, got %f", got)
	}
	if s.Type() != segment.PricingFlatFee {
		t.Errorf("expected type FlatFee, got %s", s.Type())
	}
}

func TestPricerGetStrategy(t *testing.T) {
	pricer := segment.NewPricer()

	t.Run("CPM strategy when no flat fee", func(t *testing.T) {
		seg := &segment.Segment{CPMFee: 2.0}
		strategy, err := pricer.GetStrategy(seg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strategy.Type() != segment.PricingCPM {
			t.Errorf("expected CPM strategy")
		}
	})

	t.Run("FlatFee strategy takes precedence", func(t *testing.T) {
		flatFee := 50.0
		seg := &segment.Segment{CPMFee: 1.0, FlatFee: &flatFee}
		strategy, err := pricer.GetStrategy(seg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strategy.Type() != segment.PricingFlatFee {
			t.Errorf("expected FlatFee strategy")
		}
	})

	t.Run("error when no pricing", func(t *testing.T) {
		seg := &segment.Segment{CPMFee: 0}
		_, err := pricer.GetStrategy(seg)
		if err == nil {
			t.Error("expected error for segment with no pricing")
		}
	})
}

func TestPricerCalculateFee(t *testing.T) {
	pricer := segment.NewPricer()
	seg := &segment.Segment{CPMFee: 2.0}

	fee, err := pricer.CalculateFee(seg, 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fee != 10.0 {
		t.Errorf("expected 10.0, got %f", fee)
	}
}
