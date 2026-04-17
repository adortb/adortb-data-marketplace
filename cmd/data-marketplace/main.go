package main

import (
	"database/sql"
	"log/slog"
	"os"

	"github.com/adortb/adortb-data-marketplace/internal/activation"
	"github.com/adortb/adortb-data-marketplace/internal/api"
	"github.com/adortb/adortb-data-marketplace/internal/billing"
	"github.com/adortb/adortb-data-marketplace/internal/marketplace"
	"github.com/adortb/adortb-data-marketplace/internal/provider"
	"github.com/adortb/adortb-data-marketplace/internal/segment"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	dsn := getEnv("DATABASE_URL", "postgres://localhost/adortb_data_marketplace?sslmode=disable")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		logger.Error("failed to open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		logger.Error("failed to ping database", "err", err)
		os.Exit(1)
	}

	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})

	// Provider 层
	providerRepo := provider.NewPGRepository(db)
	registry := provider.NewRegistry(providerRepo)
	onboarding := provider.NewOnboardingService(registry, providerRepo)

	// Segment 层
	segmentRepo := segment.NewPGRepository(db)
	catalog := segment.NewCatalog(segmentRepo)
	uploader := segment.NewUploader(db, rdb, catalog)
	pricer := segment.NewPricer()

	// Marketplace 层
	listing := marketplace.NewListing(catalog)
	search := marketplace.NewSearchService(catalog)

	// Activation 层
	activationRepo := activation.NewPGActivationRepository(db)
	activator := activation.NewActivator(activationRepo, catalog)
	attribution := activation.NewAttributionService(rdb)

	// Billing 层
	usageRepo := billing.NewPGUsageRepository(db)
	usageTracker := billing.NewUsageTracker(usageRepo)
	settlement := billing.NewSettlementService(usageRepo, providerRepo, catalog)

	// API 服务
	srv := api.New(api.Config{
		Registry:     registry,
		Onboarding:   onboarding,
		Catalog:      catalog,
		Uploader:     uploader,
		Pricer:       pricer,
		Listing:      listing,
		Search:       search,
		Activator:    activator,
		Attribution:  attribution,
		UsageTracker: usageTracker,
		Settlement:   settlement,
		Logger:       logger,
	})

	addr := getEnv("LISTEN_ADDR", ":8098")
	if err := srv.ListenAndServe(addr); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
