package activation

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/adortb/adortb-data-marketplace/internal/segment"
)

// Operator 操作类型
type Operator string

const (
	OperatorInclude Operator = "include"
	OperatorExclude Operator = "exclude"
)

// Activation 活动受众包激活记录
type Activation struct {
	ID          int64     `json:"id"`
	CampaignID  int64     `json:"campaign_id"`
	SegmentID   int64     `json:"segment_id"`
	Operator    Operator  `json:"operator"`
	ActivatedAt time.Time `json:"activated_at"`
}

// ActivationRepository 激活记录存储接口
type ActivationRepository interface {
	Create(ctx context.Context, a *Activation) (*Activation, error)
	ListByCampaign(ctx context.Context, campaignID int64) ([]*Activation, error)
}

// Activator 活动激活管理
type Activator struct {
	repo    ActivationRepository
	catalog *segment.Catalog
}

// NewActivator 创建激活管理
func NewActivator(repo ActivationRepository, catalog *segment.Catalog) *Activator {
	return &Activator{repo: repo, catalog: catalog}
}

// ActivateRequest 激活请求
type ActivateRequest struct {
	SegmentID int64    `json:"segment_id"`
	Operator  Operator `json:"operator"`
}

// Activate 激活受众包到活动
func (a *Activator) Activate(ctx context.Context, campaignID int64, req ActivateRequest) (*Activation, error) {
	if req.Operator == "" {
		req.Operator = OperatorInclude
	}
	if req.Operator != OperatorInclude && req.Operator != OperatorExclude {
		return nil, fmt.Errorf("invalid operator: %s", req.Operator)
	}

	// 验证受众包存在且已审批
	seg, err := a.catalog.GetByID(ctx, req.SegmentID)
	if err != nil {
		return nil, fmt.Errorf("segment not found: %w", err)
	}
	if seg.Status != segment.StatusApproved {
		return nil, fmt.Errorf("segment %d is not approved", req.SegmentID)
	}

	activation := &Activation{
		CampaignID: campaignID,
		SegmentID:  req.SegmentID,
		Operator:   req.Operator,
	}

	created, err := a.repo.Create(ctx, activation)
	if err != nil {
		return nil, fmt.Errorf("create activation: %w", err)
	}
	return created, nil
}

// ListByCampaign 列出活动的受众包激活
func (a *Activator) ListByCampaign(ctx context.Context, campaignID int64) ([]*Activation, error) {
	return a.repo.ListByCampaign(ctx, campaignID)
}

// PGActivationRepository PostgreSQL 实现
type PGActivationRepository struct {
	db *sql.DB
}

// NewPGActivationRepository 创建PG存储
func NewPGActivationRepository(db *sql.DB) *PGActivationRepository {
	return &PGActivationRepository{db: db}
}

func (r *PGActivationRepository) Create(ctx context.Context, a *Activation) (*Activation, error) {
	const q = `
		INSERT INTO segment_activations (campaign_id, segment_id, operator)
		VALUES ($1, $2, $3)
		RETURNING id, campaign_id, segment_id, operator, activated_at`

	created := &Activation{}
	err := r.db.QueryRowContext(ctx, q, a.CampaignID, a.SegmentID, a.Operator).Scan(
		&created.ID, &created.CampaignID, &created.SegmentID, &created.Operator, &created.ActivatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert segment_activations: %w", err)
	}
	return created, nil
}

func (r *PGActivationRepository) ListByCampaign(ctx context.Context, campaignID int64) ([]*Activation, error) {
	const q = `
		SELECT id, campaign_id, segment_id, operator, activated_at
		FROM segment_activations WHERE campaign_id = $1
		ORDER BY activated_at DESC`

	rows, err := r.db.QueryContext(ctx, q, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list segment_activations: %w", err)
	}
	defer rows.Close()

	var activations []*Activation
	for rows.Next() {
		a := &Activation{}
		if err := rows.Scan(&a.ID, &a.CampaignID, &a.SegmentID, &a.Operator, &a.ActivatedAt); err != nil {
			return nil, fmt.Errorf("scan activation row: %w", err)
		}
		activations = append(activations, a)
	}
	return activations, rows.Err()
}
