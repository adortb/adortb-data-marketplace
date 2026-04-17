package segment

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Status 受众包状态
type Status string

const (
	StatusDraft    Status = "draft"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

// Category 受众包分类
type Category string

const (
	CategoryDemographic Category = "demographic"
	CategoryBehavioral  Category = "behavioral"
	CategoryIntent      Category = "intent"
	CategoryGeo         Category = "geo"
)

// Segment 受众包实体
type Segment struct {
	ID          int64      `json:"id"`
	ProviderID  int64      `json:"provider_id"`
	SegmentID   string     `json:"segment_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	IABTaxonomy string     `json:"iab_taxonomy"`
	Category    Category   `json:"category"`
	UserCount   int64      `json:"user_count"`
	RecencyDays int        `json:"recency_days"`
	CPMFee      float64    `json:"cpm_fee"`
	FlatFee     *float64   `json:"flat_fee,omitempty"`
	Status      Status     `json:"status"`
	ApprovedAt  *time.Time `json:"approved_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Repository 受众包存储接口
type Repository interface {
	Create(ctx context.Context, s *Segment) (*Segment, error)
	GetByID(ctx context.Context, id int64) (*Segment, error)
	UpdateStatus(ctx context.Context, id int64, status Status, approvedAt *time.Time) error
	UpdateUserCount(ctx context.Context, id int64, count int64) error
	List(ctx context.Context, filter ListFilter) ([]*Segment, error)
	ListIDsByProvider(ctx context.Context, providerID int64) ([]int64, error)
}

// ListFilter 查询过滤条件
type ListFilter struct {
	Category Category
	MinSize  int64
	Status   Status
	Limit    int
	Offset   int
}

// Catalog 受众包目录管理
type Catalog struct {
	repo Repository
}

// NewCatalog 创建目录管理
func NewCatalog(repo Repository) *Catalog {
	return &Catalog{repo: repo}
}

// CreateSegment 创建受众包
func (c *Catalog) CreateSegment(ctx context.Context, providerID int64, req CreateRequest) (*Segment, error) {
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("invalid create request: %w", err)
	}

	s := &Segment{
		ProviderID:  providerID,
		SegmentID:   req.SegmentID,
		Name:        req.Name,
		Description: req.Description,
		IABTaxonomy: req.IABTaxonomy,
		Category:    req.Category,
		RecencyDays: req.RecencyDays,
		CPMFee:      req.CPMFee,
		FlatFee:     req.FlatFee,
		Status:      StatusDraft,
	}

	created, err := c.repo.Create(ctx, s)
	if err != nil {
		return nil, fmt.Errorf("failed to create segment: %w", err)
	}
	return created, nil
}

// Approve 审批受众包
func (c *Catalog) Approve(ctx context.Context, id int64) error {
	s, err := c.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("segment not found: %w", err)
	}
	if s.Status != StatusDraft {
		return fmt.Errorf("segment status is %s, can only approve draft segments", s.Status)
	}

	now := time.Now()
	if err := c.repo.UpdateStatus(ctx, id, StatusApproved, &now); err != nil {
		return fmt.Errorf("failed to approve segment: %w", err)
	}
	return nil
}

// GetByID 获取受众包
func (c *Catalog) GetByID(ctx context.Context, id int64) (*Segment, error) {
	return c.repo.GetByID(ctx, id)
}

// List 列出受众包
func (c *Catalog) List(ctx context.Context, filter ListFilter) ([]*Segment, error) {
	return c.repo.List(ctx, filter)
}

// ListSegmentIDsByProvider 列出 provider 旗下所有 segment ID（供结算使用）
func (c *Catalog) ListSegmentIDsByProvider(ctx context.Context, providerID int64) ([]int64, error) {
	return c.repo.ListIDsByProvider(ctx, providerID)
}

// CreateRequest 创建请求
type CreateRequest struct {
	SegmentID   string   `json:"segment_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IABTaxonomy string   `json:"iab_taxonomy"`
	Category    Category `json:"category"`
	RecencyDays int      `json:"recency_days"`
	CPMFee      float64  `json:"cpm_fee"`
	FlatFee     *float64 `json:"flat_fee,omitempty"`
}

func (r CreateRequest) validate() error {
	if r.SegmentID == "" {
		return fmt.Errorf("segment_id is required")
	}
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	if r.CPMFee <= 0 {
		return fmt.Errorf("cpm_fee must be positive")
	}
	return nil
}

// PGRepository PostgreSQL 实现
type PGRepository struct {
	db *sql.DB
}

// NewPGRepository 创建PG存储
func NewPGRepository(db *sql.DB) *PGRepository {
	return &PGRepository{db: db}
}

func (r *PGRepository) Create(ctx context.Context, s *Segment) (*Segment, error) {
	const q = `
		INSERT INTO audience_segments
			(provider_id, segment_id, name, description, iab_taxonomy, category, recency_days, cpm_fee, flat_fee, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, provider_id, segment_id, name, description, iab_taxonomy, category,
		          user_count, recency_days, cpm_fee, flat_fee, status, approved_at, created_at`

	created := &Segment{}
	err := r.db.QueryRowContext(ctx, q,
		s.ProviderID, s.SegmentID, s.Name, s.Description, s.IABTaxonomy,
		s.Category, s.RecencyDays, s.CPMFee, s.FlatFee, s.Status,
	).Scan(
		&created.ID, &created.ProviderID, &created.SegmentID, &created.Name,
		&created.Description, &created.IABTaxonomy, &created.Category,
		&created.UserCount, &created.RecencyDays, &created.CPMFee, &created.FlatFee,
		&created.Status, &created.ApprovedAt, &created.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert audience_segments: %w", err)
	}
	return created, nil
}

func (r *PGRepository) GetByID(ctx context.Context, id int64) (*Segment, error) {
	const q = `
		SELECT id, provider_id, segment_id, name, description, iab_taxonomy, category,
		       user_count, recency_days, cpm_fee, flat_fee, status, approved_at, created_at
		FROM audience_segments WHERE id = $1`

	s := &Segment{}
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&s.ID, &s.ProviderID, &s.SegmentID, &s.Name, &s.Description,
		&s.IABTaxonomy, &s.Category, &s.UserCount, &s.RecencyDays,
		&s.CPMFee, &s.FlatFee, &s.Status, &s.ApprovedAt, &s.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("segment %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("query audience_segments: %w", err)
	}
	return s, nil
}

func (r *PGRepository) UpdateStatus(ctx context.Context, id int64, status Status, approvedAt *time.Time) error {
	const q = `UPDATE audience_segments SET status = $1, approved_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, q, status, approvedAt, id)
	if err != nil {
		return fmt.Errorf("update audience_segments status: %w", err)
	}
	return nil
}

func (r *PGRepository) UpdateUserCount(ctx context.Context, id int64, count int64) error {
	const q = `UPDATE audience_segments SET user_count = $1 WHERE id = $2`
	_, err := r.db.ExecContext(ctx, q, count, id)
	if err != nil {
		return fmt.Errorf("update audience_segments user_count: %w", err)
	}
	return nil
}

func (r *PGRepository) ListIDsByProvider(ctx context.Context, providerID int64) ([]int64, error) {
	const q = `SELECT id FROM audience_segments WHERE provider_id = $1`
	rows, err := r.db.QueryContext(ctx, q, providerID)
	if err != nil {
		return nil, fmt.Errorf("list segment ids by provider: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan segment id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *PGRepository) List(ctx context.Context, filter ListFilter) ([]*Segment, error) {
	q := `
		SELECT id, provider_id, segment_id, name, description, iab_taxonomy, category,
		       user_count, recency_days, cpm_fee, flat_fee, status, approved_at, created_at
		FROM audience_segments WHERE 1=1`
	args := []any{}

	if filter.Status != "" {
		args = append(args, filter.Status)
		q += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if filter.Category != "" {
		args = append(args, filter.Category)
		q += fmt.Sprintf(" AND category = $%d", len(args))
	}
	if filter.MinSize > 0 {
		args = append(args, filter.MinSize)
		q += fmt.Sprintf(" AND user_count >= $%d", len(args))
	}

	args = append(args, filter.Limit, filter.Offset)
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list audience_segments: %w", err)
	}
	defer rows.Close()

	var segments []*Segment
	for rows.Next() {
		s := &Segment{}
		if err := rows.Scan(
			&s.ID, &s.ProviderID, &s.SegmentID, &s.Name, &s.Description,
			&s.IABTaxonomy, &s.Category, &s.UserCount, &s.RecencyDays,
			&s.CPMFee, &s.FlatFee, &s.Status, &s.ApprovedAt, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan audience_segment row: %w", err)
		}
		segments = append(segments, s)
	}
	return segments, rows.Err()
}
