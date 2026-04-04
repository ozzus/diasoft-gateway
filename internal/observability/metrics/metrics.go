package metrics

import (
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Service struct {
	registry          *prometheus.Registry
	cacheLookups      *prometheus.CounterVec
	rateLimitRequests *prometheus.CounterVec
	kafkaEvents       *prometheus.CounterVec
	kafkaEventAge     *prometheus.HistogramVec
}

func New() *Service {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
	)

	service := &Service{
		registry: registry,
		cacheLookups: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_verification_cache_lookups_total",
				Help: "Total cache lookups for public diploma verification.",
			},
			[]string{"lookup_type", "result"},
		),
		rateLimitRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_rate_limit_requests_total",
				Help: "Total public requests processed by the Redis rate limiter.",
			},
			[]string{"path", "decision"},
		),
		kafkaEvents: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_kafka_events_total",
				Help: "Total Kafka events handled by the gateway consumer.",
			},
			[]string{"topic", "event_type", "status"},
		),
		kafkaEventAge: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gateway_kafka_event_age_seconds",
				Help:    "Age of Kafka events at the moment they are successfully projected.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"topic", "event_type"},
		),
	}

	registry.MustRegister(
		service.cacheLookups,
		service.rateLimitRequests,
		service.kafkaEvents,
		service.kafkaEventAge,
	)

	return service
}

func (s *Service) Handler() http.Handler {
	if s == nil || s.registry == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{
		Registry:          s.registry,
		EnableOpenMetrics: true,
	})
}

func (s *Service) ObserveVerificationCache(key, result string) {
	if s == nil {
		return
	}
	s.cacheLookups.WithLabelValues(cacheLookupType(key), strings.TrimSpace(result)).Inc()
}

func (s *Service) ObserveRateLimit(path, decision string) {
	if s == nil {
		return
	}
	s.rateLimitRequests.WithLabelValues(strings.TrimSpace(path), strings.TrimSpace(decision)).Inc()
}

func (s *Service) ObserveKafkaEvent(topic, eventType, status string) {
	if s == nil {
		return
	}
	s.kafkaEvents.WithLabelValues(strings.TrimSpace(topic), normalizeEventType(eventType), strings.TrimSpace(status)).Inc()
}

func (s *Service) ObserveKafkaEventAge(topic, eventType string, age time.Duration) {
	if s == nil {
		return
	}
	if age < 0 {
		age = 0
	}
	s.kafkaEventAge.WithLabelValues(strings.TrimSpace(topic), normalizeEventType(eventType)).Observe(age.Seconds())
}

func cacheLookupType(key string) string {
	switch {
	case strings.HasPrefix(key, "verify:number:"):
		return "lookup"
	case strings.HasPrefix(key, "verify:token:"):
		return "token"
	default:
		return "unknown"
	}
}

func normalizeEventType(eventType string) string {
	if strings.TrimSpace(eventType) == "" {
		return "unknown"
	}
	return strings.TrimSpace(eventType)
}
