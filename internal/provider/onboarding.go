package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// OnboardingService 数据提供方接入审核流程
type OnboardingService struct {
	registry *Registry
	repo     Repository
}

// NewOnboardingService 创建接入服务
func NewOnboardingService(registry *Registry, repo Repository) *OnboardingService {
	return &OnboardingService{registry: registry, repo: repo}
}

// ApproveAndIssueKey 审批通过并颁发 API Key
func (s *OnboardingService) ApproveAndIssueKey(ctx context.Context, providerID int64) (string, error) {
	if err := s.registry.Approve(ctx, providerID); err != nil {
		return "", fmt.Errorf("approve provider: %w", err)
	}

	apiKey, err := generateAPIKey()
	if err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}

	if err := s.updateAPIKey(ctx, providerID, apiKey); err != nil {
		return "", fmt.Errorf("update api key: %w", err)
	}

	return apiKey, nil
}

// updateAPIKey 更新数据提供方的 API Key
func (s *OnboardingService) updateAPIKey(ctx context.Context, providerID int64, apiKey string) error {
	pgRepo, ok := s.repo.(*PGRepository)
	if !ok {
		return fmt.Errorf("repository does not support api key update")
	}

	const q = `UPDATE data_providers SET api_key = $1 WHERE id = $2`
	_, err := pgRepo.db.ExecContext(ctx, q, apiKey, providerID)
	if err != nil {
		return fmt.Errorf("update api_key: %w", err)
	}
	return nil
}

// generateAPIKey 生成随机 API Key
func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return "dm_" + hex.EncodeToString(b), nil
}
