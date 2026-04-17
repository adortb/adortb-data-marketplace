package provider_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/adortb/adortb-data-marketplace/internal/provider"
)

// mockProviderRepo 内存模拟存储
type mockProviderRepo struct {
	providers map[int64]*provider.Provider
	apiKeys   map[string]*provider.Provider
	nextID    int64
}

func newMockProviderRepo() *mockProviderRepo {
	return &mockProviderRepo{
		providers: make(map[int64]*provider.Provider),
		apiKeys:   make(map[string]*provider.Provider),
		nextID:    1,
	}
}

func (r *mockProviderRepo) Create(_ context.Context, p *provider.Provider) (*provider.Provider, error) {
	created := *p
	created.ID = r.nextID
	created.CreatedAt = time.Now()
	r.nextID++
	r.providers[created.ID] = &created
	return &created, nil
}

func (r *mockProviderRepo) GetByID(_ context.Context, id int64) (*provider.Provider, error) {
	p, ok := r.providers[id]
	if !ok {
		return nil, fmt.Errorf("provider %d not found", id)
	}
	return p, nil
}

func (r *mockProviderRepo) GetByAPIKey(_ context.Context, apiKey string) (*provider.Provider, error) {
	p, ok := r.apiKeys[apiKey]
	if !ok {
		return nil, fmt.Errorf("provider not found")
	}
	return p, nil
}

func (r *mockProviderRepo) UpdateStatus(_ context.Context, id int64, status provider.Status, approvedAt *time.Time) error {
	p, ok := r.providers[id]
	if !ok {
		return fmt.Errorf("provider %d not found", id)
	}
	p.Status = status
	p.ApprovedAt = approvedAt
	return nil
}

func (r *mockProviderRepo) List(_ context.Context, _ provider.Status, _, _ int) ([]*provider.Provider, error) {
	var result []*provider.Provider
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result, nil
}

func TestRegistry_Apply(t *testing.T) {
	repo := newMockProviderRepo()
	registry := provider.NewRegistry(repo)
	ctx := context.Background()

	t.Run("valid application", func(t *testing.T) {
		req := provider.ApplyRequest{
			Name:         "Experian",
			Company:      "Experian PLC",
			ContactEmail: "api@experian.com",
			Description:  "Global data analytics company",
			Website:      "https://experian.com",
		}

		p, err := registry.Apply(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ID == 0 {
			t.Error("expected non-zero ID")
		}
		if p.Status != provider.StatusPending {
			t.Errorf("expected pending status, got %s", p.Status)
		}
		if p.RevshareRate != 0.70 {
			t.Errorf("expected revshare 0.70, got %f", p.RevshareRate)
		}
	})

	t.Run("missing name fails", func(t *testing.T) {
		req := provider.ApplyRequest{ContactEmail: "test@test.com"}
		_, err := registry.Apply(ctx, req)
		if err == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("missing email fails", func(t *testing.T) {
		req := provider.ApplyRequest{Name: "Test Provider"}
		_, err := registry.Apply(ctx, req)
		if err == nil {
			t.Error("expected error for missing email")
		}
	})
}

func TestRegistry_Approve(t *testing.T) {
	repo := newMockProviderRepo()
	registry := provider.NewRegistry(repo)
	ctx := context.Background()

	req := provider.ApplyRequest{
		Name:         "Acxiom",
		ContactEmail: "api@acxiom.com",
	}
	p, _ := registry.Apply(ctx, req)

	t.Run("approve pending provider", func(t *testing.T) {
		if err := registry.Approve(ctx, p.ID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated, _ := registry.GetByID(ctx, p.ID)
		if updated.Status != provider.StatusApproved {
			t.Errorf("expected approved, got %s", updated.Status)
		}
		if updated.ApprovedAt == nil {
			t.Error("expected approved_at to be set")
		}
	})

	t.Run("cannot approve non-pending", func(t *testing.T) {
		err := registry.Approve(ctx, p.ID)
		if err == nil {
			t.Error("expected error when approving non-pending provider")
		}
	})

	t.Run("approve non-existent fails", func(t *testing.T) {
		err := registry.Approve(ctx, 99999)
		if err == nil {
			t.Error("expected error for non-existent provider")
		}
	})
}

func TestRegistry_Suspend(t *testing.T) {
	repo := newMockProviderRepo()
	registry := provider.NewRegistry(repo)
	ctx := context.Background()

	req := provider.ApplyRequest{Name: "Test", ContactEmail: "t@t.com"}
	p, _ := registry.Apply(ctx, req)
	registry.Approve(ctx, p.ID) //nolint:errcheck

	t.Run("suspend approved provider", func(t *testing.T) {
		if err := registry.Suspend(ctx, p.ID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated, _ := registry.GetByID(ctx, p.ID)
		if updated.Status != provider.StatusSuspended {
			t.Errorf("expected suspended, got %s", updated.Status)
		}
	})

	t.Run("suspend non-existent fails", func(t *testing.T) {
		err := registry.Suspend(ctx, 99999)
		if err == nil {
			t.Error("expected error for non-existent provider")
		}
	})
}
