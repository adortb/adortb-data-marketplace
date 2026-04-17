package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics Prometheus 指标集
type Metrics struct {
	SegmentUploadTotal    prometheus.Counter
	SegmentUploadFailed   prometheus.Counter
	TargetingChecksTotal  prometheus.Counter
	TargetingHitsTotal    prometheus.Counter
	ImpressionTracked     prometheus.Counter
	SettlementRuns        prometheus.Counter
	HTTPRequestDuration   *prometheus.HistogramVec
}

// New 初始化指标
func New() *Metrics {
	return &Metrics{
		SegmentUploadTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "dm_segment_upload_total",
			Help: "Total number of users uploaded to segments",
		}),
		SegmentUploadFailed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "dm_segment_upload_failed_total",
			Help: "Total number of failed user uploads",
		}),
		TargetingChecksTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "dm_targeting_checks_total",
			Help: "Total targeting check requests",
		}),
		TargetingHitsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "dm_targeting_hits_total",
			Help: "Total targeting check hits",
		}),
		ImpressionTracked: promauto.NewCounter(prometheus.CounterOpts{
			Name: "dm_impressions_tracked_total",
			Help: "Total impressions tracked",
		}),
		SettlementRuns: promauto.NewCounter(prometheus.CounterOpts{
			Name: "dm_settlement_runs_total",
			Help: "Total settlement runs",
		}),
		HTTPRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "dm_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path", "status"}),
	}
}
