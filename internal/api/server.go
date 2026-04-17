package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/adortb/adortb-data-marketplace/internal/activation"
	"github.com/adortb/adortb-data-marketplace/internal/billing"
	"github.com/adortb/adortb-data-marketplace/internal/marketplace"
	"github.com/adortb/adortb-data-marketplace/internal/provider"
	"github.com/adortb/adortb-data-marketplace/internal/segment"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server HTTP API 服务
type Server struct {
	mux           *http.ServeMux
	registry      *provider.Registry
	onboarding    *provider.OnboardingService
	catalog       *segment.Catalog
	uploader      *segment.Uploader
	pricer        *segment.Pricer
	listing       *marketplace.Listing
	search        *marketplace.SearchService
	activator     *activation.Activator
	attribution   *activation.AttributionService
	usageTracker  *billing.UsageTracker
	settlement    *billing.SettlementService
	logger        *slog.Logger
}

// Config 服务配置
type Config struct {
	Registry    *provider.Registry
	Onboarding  *provider.OnboardingService
	Catalog     *segment.Catalog
	Uploader    *segment.Uploader
	Pricer      *segment.Pricer
	Listing     *marketplace.Listing
	Search      *marketplace.SearchService
	Activator   *activation.Activator
	Attribution *activation.AttributionService
	UsageTracker *billing.UsageTracker
	Settlement  *billing.SettlementService
	Logger      *slog.Logger
}

// New 创建 HTTP 服务
func New(cfg Config) *Server {
	s := &Server{
		mux:          http.NewServeMux(),
		registry:     cfg.Registry,
		onboarding:   cfg.Onboarding,
		catalog:      cfg.Catalog,
		uploader:     cfg.Uploader,
		pricer:       cfg.Pricer,
		listing:      cfg.Listing,
		search:       cfg.Search,
		activator:    cfg.Activator,
		attribution:  cfg.Attribution,
		usageTracker: cfg.UsageTracker,
		settlement:   cfg.Settlement,
		logger:       cfg.Logger,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// 系统接口
	s.mux.Handle("GET /metrics", promhttp.Handler())
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// 数据提供方侧
	s.mux.HandleFunc("POST /v1/providers/apply", s.handleProviderApply)
	s.mux.HandleFunc("POST /v1/providers/{id}/approve", s.handleProviderApprove)
	s.mux.HandleFunc("POST /v1/providers/{id}/segments", s.handleSegmentCreate)
	s.mux.HandleFunc("POST /v1/segments/{id}/users/upload", s.handleUsersUpload)
	s.mux.HandleFunc("GET /v1/providers/{id}/earnings", s.handleProviderEarnings)

	// 广告主侧
	s.mux.HandleFunc("GET /v1/marketplace/segments", s.handleMarketplaceSearch)
	s.mux.HandleFunc("GET /v1/segments/{id}", s.handleSegmentDetail)
	s.mux.HandleFunc("POST /v1/campaigns/{campaign_id}/segments", s.handleCampaignActivate)
	s.mux.HandleFunc("GET /v1/campaigns/{campaign_id}/segments/usage", s.handleCampaignUsage)

	// 平台管理
	s.mux.HandleFunc("POST /v1/segments/{id}/approve", s.handleSegmentApprove)

	// DSP 运行时查询
	s.mux.HandleFunc("POST /v1/targeting/check", s.handleTargetingCheck)
}

// ServeHTTP 实现 http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe 启动服务
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	s.logger.Info("data marketplace server starting", "addr", addr)
	return srv.ListenAndServe()
}

// --- 工具方法 ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pathID(r *http.Request, key string) (int64, error) {
	v := r.PathValue(key)
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %s", key, v)
	}
	return id, nil
}

// --- 处理器 ---

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleProviderApply(w http.ResponseWriter, r *http.Request) {
	var req provider.ApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	p, err := s.registry.Apply(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleProviderApprove(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	apiKey, err := s.onboarding.ApproveAndIssueKey(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"api_key": apiKey})
}

func (s *Server) handleSegmentCreate(w http.ResponseWriter, r *http.Request) {
	providerID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req segment.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	seg, err := s.catalog.CreateSegment(r.Context(), providerID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, seg)
}

func (s *Server) handleUsersUpload(w http.ResponseWriter, r *http.Request) {
	segmentID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.uploader.UploadStream(r.Context(), segmentID, r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleProviderEarnings(w http.ResponseWriter, r *http.Request) {
	providerID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	from := time.Now().AddDate(0, -1, 0)
	to := time.Now()

	result, err := s.settlement.GetProviderEarnings(r.Context(), billing.ProviderEarningsQuery{
		ProviderID: providerID,
		From:       from,
		To:         to,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMarketplaceSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	minSize, _ := strconv.ParseInt(q.Get("min_size"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	req := marketplace.SearchRequest{
		Category: segment.Category(q.Get("category")),
		MinSize:  minSize,
		Limit:    limit,
		Offset:   offset,
	}

	segments, err := s.search.Search(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"segments": segments, "count": len(segments)})
}

func (s *Server) handleSegmentDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	seg, err := s.catalog.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, seg)
}

func (s *Server) handleSegmentApprove(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.catalog.Approve(r.Context(), id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (s *Server) handleCampaignActivate(w http.ResponseWriter, r *http.Request) {
	campaignID, err := pathID(r, "campaign_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req activation.ActivateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	act, err := s.activator.Activate(r.Context(), campaignID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, act)
}

func (s *Server) handleCampaignUsage(w http.ResponseWriter, r *http.Request) {
	campaignID, err := pathID(r, "campaign_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	from := time.Now().AddDate(0, -1, 0)
	to := time.Now()

	records, err := s.usageTracker.GetCampaignUsage(r.Context(), campaignID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": records})
}

func (s *Server) handleTargetingCheck(w http.ResponseWriter, r *http.Request) {
	var req activation.TargetingCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := s.attribution.CheckTargeting(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// Shutdown 优雅关闭（预留接口）
func (s *Server) Shutdown(_ context.Context) error {
	return nil
}
