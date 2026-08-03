// Package metrics exposes Prometheus instrumentation for the HTTP server and
// the database pool.
//
// # Why not promauto and the default registry
//
// promauto registers into a package-level global and panics on a duplicate
// registration. That makes two things impossible: constructing the metrics
// twice in one process (which every table-driven test does), and asserting on
// collected values without the previous test's data still in them. Everything
// here hangs off an explicit *Metrics, built by the composition root and
// passed where it is needed, for the same reason the loggers are
// (docs/architectures/06-quality/observability.md, Problem 1).
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// namespace prefixes every metric, so a dashboard can select this service's
// series without matching on a label.
const namespace = "kopiochi"

// Metrics owns a private registry and the collectors registered into it.
type Metrics struct {
	registry *prometheus.Registry

	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

// New builds the metric set and registers it, along with the standard Go
// runtime and process collectors.
func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),

		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total HTTP requests, by method, route pattern and status.",
		}, []string{"method", "route", "status"}),

		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency, by method and route pattern.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "route"}),

		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "http_requests_in_flight",
			Help:      "HTTP requests currently being served.",
		}),
	}

	m.registry.MustRegister(
		m.requests, m.duration, m.inFlight,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return m
}

// Registry exposes the underlying registry, for registering collectors built
// elsewhere (the pool collector) and for tests that want to gather.
func (m *Metrics) Registry() *prometheus.Registry { return m.registry }

// Handler serves the exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		// A failing collector must not take the scrape endpoint down with it;
		// Prometheus reports the error and keeps the rest of the series.
		ErrorHandling: promhttp.ContinueOnError,
	})
}

// The observability document also proposes auth_failures_total{reason} and
// rate_limit_rejections_total. Neither is here.
//
// Rate-limit rejections are already http_requests_total{status="429"}: the
// middleware is registered ahead of the limiter, so it observes the 429 the
// limiter writes. A second counter for the same event would be a metric that
// can disagree with itself.
//
// Authentication failures broken down by reason (bad_password, expired_token,
// wrong_class, locked) genuinely are not derivable — status="401" lumps them
// together and hides the one that matters, wrong_class. Getting them means
// instrumenting the identity module's login path and auth middleware, which is
// a change to that module rather than to this package. It is carried forward
// rather than stubbed: a collector nothing increments reports zero, which
// reads as "no attacks" rather than "not measured".

// Middleware records a counter, a latency histogram and an in-flight gauge for
// every request.
//
// Requests are labelled with the chi *route pattern* (/api/v1/users/{id}),
// never the raw path. A raw path makes one time series per distinct URL, so a
// crawler hitting /api/v1/users/1 … /api/v1/users/9999999 creates millions of
// series and takes the Prometheus server down — an availability problem caused
// by the monitoring, from unauthenticated traffic.
//
// It must be registered after chi's routing context exists, which in practice
// means anywhere in NewRouter's stack: chi installs the RouteContext before
// any middleware runs, and fills in the pattern by the time the handler
// returns.
func (m *Metrics) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m.inFlight.Inc()
			defer m.inFlight.Dec()

			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			route := routePattern(r)
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			m.requests.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
			m.duration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		})
	}
}

// unmatchedRoute is the label used when no route matched — a 404. Bucketing
// them under one value is the whole point: unmatched paths are exactly the
// unbounded set, and scanning traffic consists of nothing else.
const unmatchedRoute = "unmatched"

func routePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return unmatchedRoute
	}
	if pattern := rctx.RoutePattern(); pattern != "" {
		return pattern
	}
	return unmatchedRoute
}
