package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// scrape returns the exposition text from the metrics handler.
func scrape(t *testing.T, m *Metrics) string {
	t.Helper()

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("scrape returned %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// TestNew_CanBeCalledTwice is the reason this package avoids promauto and the
// default registry: those panic on a duplicate registration, which makes a
// second instance in one process impossible — and every table-driven test
// wants one.
func TestNew_CanBeCalledTwice(t *testing.T) {
	t.Parallel()

	_ = New()
	_ = New()
}

func TestMiddleware_CountsRequests(t *testing.T) {
	t.Parallel()

	m := New()
	r := chi.NewRouter()
	r.Use(m.Middleware())
	r.Get("/api/v1/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 3; i++ {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/users/7", nil))
	}

	got := testutil.ToFloat64(m.requests.WithLabelValues(http.MethodGet, "/api/v1/users/{id}", "200"))
	if got != 3 {
		t.Errorf("http_requests_total = %v, want 3", got)
	}
}

// TestMiddleware_LabelsWithTheRoutePatternNotThePath is the cardinality guard,
// and the most consequential assertion in this file.
//
// Labelling by raw path makes one time series per distinct URL. An
// unauthenticated crawler walking /api/v1/users/1 … /api/v1/users/9999999
// would then create millions of series and take the Prometheus server down —
// an availability incident caused by the monitoring, triggered by traffic
// nobody had to authenticate to send.
func TestMiddleware_LabelsWithTheRoutePatternNotThePath(t *testing.T) {
	t.Parallel()

	m := New()
	r := chi.NewRouter()
	r.Use(m.Middleware())
	r.Get("/api/v1/users/{id}", func(w http.ResponseWriter, r *http.Request) {})

	for _, id := range []string{"1", "2", "3", "999999"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/users/"+id, nil))
	}

	body := scrape(t, m)
	if !strings.Contains(body, `route="/api/v1/users/{id}"`) {
		t.Errorf("no series labelled with the route pattern:\n%s", body)
	}
	for _, id := range []string{"1", "2", "3", "999999"} {
		if strings.Contains(body, `route="/api/v1/users/`+id+`"`) {
			t.Fatalf("a raw path appears as a label value (id=%s); this is unbounded cardinality", id)
		}
	}

	// All four requests must land on the one series, not four.
	if got := testutil.ToFloat64(m.requests.WithLabelValues(http.MethodGet, "/api/v1/users/{id}", "200")); got != 4 {
		t.Errorf("counter = %v, want all 4 requests on one series", got)
	}
}

// TestMiddleware_UnmatchedPathsShareOneLabel: 404s are precisely the unbounded
// set — scanning traffic consists of nothing else — so they must not each get
// their own series either.
func TestMiddleware_UnmatchedPathsShareOneLabel(t *testing.T) {
	t.Parallel()

	m := New()
	r := chi.NewRouter()
	r.Use(m.Middleware())
	r.Get("/known", func(w http.ResponseWriter, r *http.Request) {})

	for _, p := range []string{"/wp-admin", "/.env", "/phpmyadmin", "/etc/passwd"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
	}

	got := testutil.ToFloat64(m.requests.WithLabelValues(http.MethodGet, unmatchedRoute, "404"))
	if got != 4 {
		t.Errorf("unmatched counter = %v, want 4 (all scanning traffic on one series)", got)
	}

	body := scrape(t, m)
	if strings.Contains(body, `route="/.env"`) {
		t.Errorf("an unmatched raw path became a label value:\n%s", body)
	}
}

func TestMiddleware_RecordsTheStatus(t *testing.T) {
	t.Parallel()

	m := New()
	r := chi.NewRouter()
	r.Use(m.Middleware())
	r.Get("/boom", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	if got := testutil.ToFloat64(m.requests.WithLabelValues(http.MethodGet, "/boom", "500")); got != 1 {
		t.Errorf("500 counter = %v, want 1", got)
	}
}

// TestMiddleware_ImplicitOKIsRecordedAs200: a handler that writes nothing
// sends 200, and recording chi's raw 0 would put every such request into a
// status="0" series that means nothing.
func TestMiddleware_ImplicitOKIsRecordedAs200(t *testing.T) {
	t.Parallel()

	m := New()
	r := chi.NewRouter()
	r.Use(m.Middleware())
	r.Get("/quiet", func(w http.ResponseWriter, r *http.Request) {})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/quiet", nil))

	if got := testutil.ToFloat64(m.requests.WithLabelValues(http.MethodGet, "/quiet", "200")); got != 1 {
		t.Errorf("200 counter = %v, want 1", got)
	}
}

func TestMiddleware_ObservesLatency(t *testing.T) {
	t.Parallel()

	m := New()
	r := chi.NewRouter()
	r.Use(m.Middleware())
	r.Get("/timed", func(w http.ResponseWriter, r *http.Request) {})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/timed", nil))

	if got := testutil.CollectAndCount(m.duration); got == 0 {
		t.Error("no latency series was recorded")
	}
}

// TestMiddleware_InFlightReturnsToZero: the gauge is incremented and
// decremented around each request, so a leak here would show as permanently
// rising load on an idle service.
func TestMiddleware_InFlightReturnsToZero(t *testing.T) {
	t.Parallel()

	m := New()
	r := chi.NewRouter()
	r.Use(m.Middleware())
	r.Get("/x", func(w http.ResponseWriter, r *http.Request) {})

	for i := 0; i < 10; i++ {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	}

	if got := testutil.ToFloat64(m.inFlight); got != 0 {
		t.Errorf("in_flight = %v after all requests completed, want 0", got)
	}
}

// TestMiddleware_InFlightIsDecrementedOnPanic: the decrement is deferred, so a
// panicking handler must not leave the gauge stuck above zero. Without the
// defer, one panic permanently inflates the load reading.
func TestMiddleware_InFlightIsDecrementedOnPanic(t *testing.T) {
	t.Parallel()

	m := New()
	h := m.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	func() {
		defer func() { _ = recover() }()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}()

	if got := testutil.ToFloat64(m.inFlight); got != 0 {
		t.Errorf("in_flight = %v after a panicking handler, want 0", got)
	}
}

func TestHandler_ExposesTheNamespacedMetrics(t *testing.T) {
	t.Parallel()

	m := New()

	// A labelled vector exposes no series until some label combination has
	// been used, so a scrape of a freshly built registry legitimately contains
	// neither the counter nor the histogram. One request is needed before
	// there is anything to assert on.
	r := chi.NewRouter()
	r.Use(m.Middleware())
	r.Get("/seed", func(w http.ResponseWriter, r *http.Request) {})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/seed", nil))

	body := scrape(t, m)

	for _, name := range []string{
		"kopiochi_http_requests_total",
		"kopiochi_http_request_duration_seconds",
		"kopiochi_http_requests_in_flight",
		"go_goroutines", // the runtime collector is registered too
	} {
		if !strings.Contains(body, name) {
			t.Errorf("scrape does not expose %q", name)
		}
	}
}
