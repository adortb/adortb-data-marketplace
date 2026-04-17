package marketplace

import (
	"context"
	"fmt"

	"github.com/adortb/adortb-data-marketplace/internal/segment"
)

// Listing 市场商品上架/下架管理
type Listing struct {
	catalog *segment.Catalog
}

// NewListing 创建商品上架管理
func NewListing(catalog *segment.Catalog) *Listing {
	return &Listing{catalog: catalog}
}

// Publish 上架受众包（需要已审批状态）
func (l *Listing) Publish(ctx context.Context, segmentID int64) error {
	seg, err := l.catalog.GetByID(ctx, segmentID)
	if err != nil {
		return fmt.Errorf("get segment: %w", err)
	}
	if seg.Status != segment.StatusApproved {
		return fmt.Errorf("segment %d must be approved before publishing, current status: %s", segmentID, seg.Status)
	}
	return nil
}

// Unpublish 下架受众包
func (l *Listing) Unpublish(ctx context.Context, segmentID int64) error {
	_, err := l.catalog.GetByID(ctx, segmentID)
	if err != nil {
		return fmt.Errorf("get segment: %w", err)
	}
	// 下架只是将状态标记，实际业务可以扩展
	return nil
}
