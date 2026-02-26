package metrics

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/linnemanlabs/go-core/version"
)

type ServerMetrics struct {
	reg             *prometheus.Registry
	handler         http.Handler
	inflight        prometheus.Gauge
	reqTotal        *prometheus.CounterVec
	reqDur          *prometheus.HistogramVec
	respBytes       *prometheus.HistogramVec
	httpPanicTotal  prometheus.Counter
	buildInfo       *prometheus.GaugeVec
	errorsTotal     *prometheus.CounterVec
	profilingActive prometheus.Gauge
}

// New returns a fresh registry + standard collectors + HTTP metrics
// safe labels only (method, route, code) to avoid path/cardinality explosions
func New() *ServerMetrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &ServerMetrics{
		inflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "http_inflight_requests",
			Help: "Current number of in-flight HTTP requests",
		}),
		reqTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by method, route, and status",
		}, []string{"method", "route", "status"}),
		reqDur: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Request latency by method and route",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"method", "route"}),
		respBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_response_size_bytes",
			Help:    "Response size by method and route",
			Buckets: []float64{256, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 52428800},
		}, []string{"method", "route"}),
		httpPanicTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "http_panic_total",
			Help: "Total number of recovered httpserver panics",
		}),
		buildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "build_info",
			Help: "Build metadata (value is always 1)",
		}, []string{"app", "component", "version", "commit", "commit_date", "build_id", "build_date", "vcs_dirty", "go_version"}),
		errorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_errors_total",
			Help: "Total 5xx HTTP server errors by method and route (SLI)",
		}, []string{"method", "route"}),
		profilingActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "profiling_active",
			Help: "Whether continuous profiling is active (1) or disabled/failed (0)",
		}),
	}
	reg.MustRegister(
		m.inflight,
		m.reqTotal,
		m.reqDur,
		m.respBytes,
		m.httpPanicTotal,
		m.buildInfo,
		m.errorsTotal,
		m.profilingActive,
	)

	m.handler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
	m.reg = reg
	return m
}

// Registry returns the underlying Prometheus registry, which can be used to register additional custom metrics as needed.
func (m *ServerMetrics) Registry() *prometheus.Registry {
	return m.reg
}

func (m *ServerMetrics) IncHttpPanic() {
	m.httpPanicTotal.Inc()
}

func (m *ServerMetrics) Handler() http.Handler {
	return m.handler
}

// set once at startup.
func (m *ServerMetrics) SetBuildInfoFromVersion(app, component string, vi *version.Info) {
	dirty := "unknown"
	if vi.VCSDirty != nil {
		dirty = strconv.FormatBool(*vi.VCSDirty)
	}
	m.buildInfo.With(prometheus.Labels{
		"app":         app,
		"component":   component,
		"version":     vi.Version,
		"commit":      vi.Commit,
		"commit_date": vi.CommitDate,
		"build_id":    vi.BuildId,
		"build_date":  vi.BuildDate,
		"go_version":  vi.GoVersion,
		"vcs_dirty":   dirty,
	}).Set(1)
}

func (m *ServerMetrics) SetProfilingActive(active bool) {
	if active {
		m.profilingActive.Set(1)
	} else {
		m.profilingActive.Set(0)
	}
}
