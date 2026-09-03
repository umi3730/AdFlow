package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	eventdomain "github.com/zhanghaiyang/adflow/internal/event/domain"
)

type Metrics struct {
	registry              *prometheus.Registry
	httpRequests          *prometheus.CounterVec
	httpDuration          *prometheus.HistogramVec
	httpInFlight          prometheus.Gauge
	decisionResults       *prometheus.CounterVec
	decisionDuration      prometheus.Histogram
	decisionAdmission     *prometheus.CounterVec
	decisionQueueDuration prometheus.Histogram
	decisionInFlight      prometheus.Gauge
	decisionTimeouts      prometheus.Counter
	events                *prometheus.CounterVec
	outboxDepth           *prometheus.GaugeVec
	outboxResults         *prometheus.CounterVec
	kafkaConsumerLag      *prometheus.GaugeVec
}

func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "adflow", Subsystem: "http", Name: "requests_total", Help: "Total HTTP requests by route and status.",
		}, []string{"method", "route", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "adflow", Subsystem: "http", Name: "request_duration_seconds", Help: "HTTP request duration by route.",
			Buckets: []float64{0.001, 0.003, 0.005, 0.01, 0.02, 0.05, 0.08, 0.15, 0.3, 0.75, 1.5},
		}, []string{"method", "route"}),
		httpInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "adflow", Subsystem: "http", Name: "in_flight_requests", Help: "Current in-flight HTTP requests.",
		}),
		decisionResults: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "adflow", Subsystem: "decision", Name: "results_total", Help: "Decision outcomes by match state and reason.",
		}, []string{"matched", "reason"}),
		decisionDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "adflow", Subsystem: "decision", Name: "duration_seconds", Help: "End-to-end decision duration.",
			Buckets: []float64{0.0005, 0.001, 0.003, 0.005, 0.01, 0.02, 0.05, 0.08, 0.15},
		}),
		decisionAdmission: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "adflow", Subsystem: "decision", Name: "admission_total", Help: "Decision admission outcomes before business processing.",
		}, []string{"result"}),
		decisionQueueDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "adflow", Subsystem: "decision", Name: "queue_duration_seconds", Help: "Time spent waiting for a decision concurrency slot.",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.003, 0.005, 0.01, 0.02, 0.05, 0.1},
		}),
		decisionInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "adflow", Subsystem: "decision", Name: "in_flight", Help: "Decision requests currently executing after admission.",
		}),
		decisionTimeouts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "adflow", Subsystem: "decision", Name: "execution_timeouts_total", Help: "Decision executions canceled by the configured processing deadline.",
		}),
		events: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "adflow", Subsystem: "event", Name: "records_total", Help: "Ad events by type and idempotency result.",
		}, []string{"type", "recorded"}),
		outboxDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "adflow", Subsystem: "outbox", Name: "rows", Help: "Current outbox rows by status.",
		}, []string{"status"}),
		outboxResults: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "adflow", Subsystem: "outbox", Name: "relay_results_total", Help: "Outbox relay outcomes.",
		}, []string{"result"}),
		kafkaConsumerLag: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "adflow", Subsystem: "kafka", Name: "consumer_lag", Help: "Observed Kafka consumer lag by topic and partition.",
		}, []string{"topic", "partition"}),
	}
	m.registry.MustRegister(
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.httpRequests, m.httpDuration, m.httpInFlight, m.decisionResults, m.decisionDuration,
		m.decisionAdmission, m.decisionQueueDuration, m.decisionInFlight, m.decisionTimeouts, m.events,
		m.outboxDepth, m.outboxResults, m.kafkaConsumerLag,
	)
	return m
}

func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		m.httpInFlight.Inc()
		defer m.httpInFlight.Dec()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		m.httpRequests.WithLabelValues(c.Request.Method, route, strconv.Itoa(c.Writer.Status())).Inc()
		m.httpDuration.WithLabelValues(c.Request.Method, route).Observe(time.Since(started).Seconds())
	}
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ObserveDecision(matched bool, reason string, duration time.Duration) {
	m.decisionResults.WithLabelValues(strconv.FormatBool(matched), reason).Inc()
	m.decisionDuration.Observe(duration.Seconds())
}

func (m *Metrics) ObserveDecisionAdmission(result string, queueDuration time.Duration) {
	m.decisionAdmission.WithLabelValues(result).Inc()
	if result != "rate_limited" {
		m.decisionQueueDuration.Observe(queueDuration.Seconds())
	}
}

func (m *Metrics) AddDecisionInFlight(delta float64) {
	m.decisionInFlight.Add(delta)
}

func (m *Metrics) ObserveDecisionTimeout() {
	m.decisionTimeouts.Inc()
}

func (m *Metrics) ObserveEvent(eventType string, recorded bool) {
	m.events.WithLabelValues(eventType, strconv.FormatBool(recorded)).Inc()
}

func (m *Metrics) SetOutboxDepth(stats eventdomain.OutboxStats) {
	m.outboxDepth.WithLabelValues("pending").Set(float64(stats.Pending))
	m.outboxDepth.WithLabelValues("processing").Set(float64(stats.Processing))
	m.outboxDepth.WithLabelValues("published").Set(float64(stats.Published))
	m.outboxDepth.WithLabelValues("dead_lettered").Set(float64(stats.DeadLettered))
}

func (m *Metrics) ObserveOutboxResult(result string) {
	m.outboxResults.WithLabelValues(result).Inc()
}

func (m *Metrics) SetKafkaConsumerLag(topic string, partition int32, lag int64) {
	m.kafkaConsumerLag.WithLabelValues(topic, strconv.FormatInt(int64(partition), 10)).Set(float64(lag))
}
