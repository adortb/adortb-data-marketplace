package marketplace

import (
	"context"
	"fmt"

	"github.com/adortb/adortb-data-marketplace/internal/segment"
)

// SearchRequest 搜索请求
type SearchRequest struct {
	Category segment.Category `json:"category"`
	MinSize  int64            `json:"min_size"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
}

// SearchService 广告主查找受众包
type SearchService struct {
	catalog *segment.Catalog
}

// NewSearchService 创建搜索服务
func NewSearchService(catalog *segment.Catalog) *SearchService {
	return &SearchService{catalog: catalog}
}

// Search 搜索受众包（只返回已审批的）
func (s *SearchService) Search(ctx context.Context, req SearchRequest) ([]*segment.Segment, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Limit > 100 {
		req.Limit = 100
	}

	filter := segment.ListFilter{
		Category: req.Category,
		MinSize:  req.MinSize,
		Status:   segment.StatusApproved,
		Limit:    req.Limit,
		Offset:   req.Offset,
	}

	segments, err := s.catalog.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("search segments: %w", err)
	}
	return segments, nil
}
