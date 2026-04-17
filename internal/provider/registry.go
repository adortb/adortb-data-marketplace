package provider

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Status 数据提供方状态
type Status string

const (
	StatusPending   Status = "pending"
	StatusApproved  Status = "approved"
	StatusSuspended Status = "suspended"
)

// Provider 数据提供方实体
type Provider struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Company      string     `json:"company"`
	ContactEmail string     `json:"contact_email"`
	Description  string     `json:"description"`
	Website      string     `json:"website"`
	Status       Status     `json:"status"`
	RevshareRate float64    `json:"revshare_rate"`
	APIKey       string     `json:"api_key,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
}

// Repository 数据提供方存储接口
type Repository interface {
	Create(ctx context.Context, p *Provider) (*Provider, error)
	GetByID(ctx context.Context, id int64) (*Provider, error)
	GetByAPIKey(ctx context.Context, apiKey string) (*Provider, error)
	UpdateStatus(ctx context.Context, id int64, status Status, approvedAt *time.Time) error
	List(ctx context.Context, status Status, limit, offset int) ([]*Provider, error)
}

// Registry 数据提供方注册管理
type Registry struct {
	repo Repository
}

// NewRegistry 创建注册管理器
func NewRegistry(repo Repository) *Registry {
	return &Registry{repo: repo}
}

// Apply 申请成为数据提供方
func (r *Registry) Apply(ctx context.Context, req ApplyRequest) (*Provider, error) {
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("invalid apply request: %w", err)
	}

	p := &Provider{
		Name:         req.Name,
		Company:      req.Company,
		ContactEmail: req.ContactEmail,
		Description:  req.Description,
		Website:      req.Website,
		Status:       StatusPending,
		RevshareRate: 0.70, // 默认分成 70%
	}

	created, err := r.repo.Create(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}
	return created, nil
}

// Approve 审批数据提供方
func (r *Registry) Approve(ctx context.Context, id int64) error {
	p, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("provider not found: %w", err)
	}
	if p.Status != StatusPending {
		return fmt.Errorf("provider status is %s, can only approve pending providers", p.Status)
	}

	now := time.Now()
	if err := r.repo.UpdateStatus(ctx, id, StatusApproved, &now); err != nil {
		return fmt.Errorf("failed to approve provider: %w", err)
	}
	return nil
}

// Suspend 暂停数据提供方
func (r *Registry) Suspend(ctx context.Context, id int64) error {
	if _, err := r.repo.GetByID(ctx, id); err != nil {
		return fmt.Errorf("provider not found: %w", err)
	}
	if err := r.repo.UpdateStatus(ctx, id, StatusSuspended, nil); err != nil {
		return fmt.Errorf("failed to suspend provider: %w", err)
	}
	return nil
}

// GetByID 获取数据提供方
func (r *Registry) GetByID(ctx context.Context, id int64) (*Provider, error) {
	p, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	return p, nil
}

// ApplyRequest 申请请求
type ApplyRequest struct {
	Name         string `json:"name"`
	Company      string `json:"company"`
	ContactEmail string `json:"contact_email"`
	Description  string `json:"description"`
	Website      string `json:"website"`
}

func (r ApplyRequest) validate() error {
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	if r.ContactEmail == "" {
		return fmt.Errorf("contact_email is required")
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

func (r *PGRepository) Create(ctx context.Context, p *Provider) (*Provider, error) {
	const q = `
		INSERT INTO data_providers (name, company, contact_email, description, website, status, revshare_rate)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, name, company, contact_email, description, website, status, revshare_rate, api_key, created_at, approved_at`

	created := &Provider{}
	var apiKey sql.NullString
	err := r.db.QueryRowContext(ctx, q,
		p.Name, p.Company, p.ContactEmail, p.Description, p.Website, p.Status, p.RevshareRate,
	).Scan(
		&created.ID, &created.Name, &created.Company, &created.ContactEmail,
		&created.Description, &created.Website, &created.Status, &created.RevshareRate,
		&apiKey, &created.CreatedAt, &created.ApprovedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert data_providers: %w", err)
	}
	if apiKey.Valid {
		created.APIKey = apiKey.String
	}
	return created, nil
}

func (r *PGRepository) GetByID(ctx context.Context, id int64) (*Provider, error) {
	const q = `
		SELECT id, name, company, contact_email, description, website, status, revshare_rate, api_key, created_at, approved_at
		FROM data_providers WHERE id = $1`

	p := &Provider{}
	var apiKey sql.NullString
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&p.ID, &p.Name, &p.Company, &p.ContactEmail, &p.Description, &p.Website,
		&p.Status, &p.RevshareRate, &apiKey, &p.CreatedAt, &p.ApprovedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("provider %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("query data_providers: %w", err)
	}
	if apiKey.Valid {
		p.APIKey = apiKey.String
	}
	return p, nil
}

func (r *PGRepository) GetByAPIKey(ctx context.Context, apiKey string) (*Provider, error) {
	const q = `
		SELECT id, name, company, contact_email, description, website, status, revshare_rate, api_key, created_at, approved_at
		FROM data_providers WHERE api_key = $1`

	p := &Provider{}
	var ak sql.NullString
	err := r.db.QueryRowContext(ctx, q, apiKey).Scan(
		&p.ID, &p.Name, &p.Company, &p.ContactEmail, &p.Description, &p.Website,
		&p.Status, &p.RevshareRate, &ak, &p.CreatedAt, &p.ApprovedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("provider with api_key not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query data_providers by api_key: %w", err)
	}
	if ak.Valid {
		p.APIKey = ak.String
	}
	return p, nil
}

func (r *PGRepository) UpdateStatus(ctx context.Context, id int64, status Status, approvedAt *time.Time) error {
	const q = `UPDATE data_providers SET status = $1, approved_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, q, status, approvedAt, id)
	if err != nil {
		return fmt.Errorf("update data_providers status: %w", err)
	}
	return nil
}

func (r *PGRepository) List(ctx context.Context, status Status, limit, offset int) ([]*Provider, error) {
	q := `
		SELECT id, name, company, contact_email, description, website, status, revshare_rate, api_key, created_at, approved_at
		FROM data_providers`
	args := []any{}
	if status != "" {
		q += " WHERE status = $1"
		args = append(args, status)
		q += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	} else {
		q += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	}
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list data_providers: %w", err)
	}
	defer rows.Close()

	var providers []*Provider
	for rows.Next() {
		p := &Provider{}
		var apiKey sql.NullString
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Company, &p.ContactEmail, &p.Description, &p.Website,
			&p.Status, &p.RevshareRate, &apiKey, &p.CreatedAt, &p.ApprovedAt,
		); err != nil {
			return nil, fmt.Errorf("scan data_provider row: %w", err)
		}
		if apiKey.Valid {
			p.APIKey = apiKey.String
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}
