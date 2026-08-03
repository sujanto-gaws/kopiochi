package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeStats satisfies PoolStats with fixed numbers. It exists to prove the
// interface is genuinely narrow: if it ever required importing pgxpool, this
// struct would not compile, and internal/metrics would have picked up a driver
// dependency.
type fakeStats struct {
	acquired, idle, total, max     int32
	newConns, acquires             int64
	emptyAcquires, canceledAcquire int64
}

func (f fakeStats) AcquiredConns() int32        { return f.acquired }
func (f fakeStats) IdleConns() int32            { return f.idle }
func (f fakeStats) TotalConns() int32           { return f.total }
func (f fakeStats) MaxConns() int32             { return f.max }
func (f fakeStats) NewConnsCount() int64        { return f.newConns }
func (f fakeStats) AcquireCount() int64         { return f.acquires }
func (f fakeStats) EmptyAcquireCount() int64    { return f.emptyAcquires }
func (f fakeStats) CanceledAcquireCount() int64 { return f.canceledAcquire }

func scrapeBody(t *testing.T, m *Metrics) string {
	t.Helper()

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("scrape returned %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestRegisterPool_ExposesTheStats(t *testing.T) {
	t.Parallel()

	m := New()
	stats := fakeStats{acquired: 3, idle: 7, total: 10, max: 20, newConns: 12, acquires: 400, emptyAcquires: 5}
	if err := m.RegisterPool(func() PoolStats { return stats }); err != nil {
		t.Fatalf("RegisterPool() error = %v", err)
	}

	body := scrapeBody(t, m)

	want := map[string]string{
		"kopiochi_db_pool_acquired_connections":    "3",
		"kopiochi_db_pool_idle_connections":        "7",
		"kopiochi_db_pool_total_connections":       "10",
		"kopiochi_db_pool_max_connections":         "20",
		"kopiochi_db_pool_new_connections_total":   "12",
		"kopiochi_db_pool_acquires_total":          "400",
		"kopiochi_db_pool_empty_acquires_total":    "5",
		"kopiochi_db_pool_canceled_acquires_total": "0",
	}
	for name, value := range want {
		if !strings.Contains(body, name+" "+value) {
			t.Errorf("expected %q with value %s in:\n%s", name, value, body)
		}
	}
}

// TestRegisterPool_ReadsAtScrapeTime is why this is a Collector rather than
// gauges updated on a ticker: the second scrape must show the new numbers with
// no tick in between. For a saturation signal, one tick of staleness is
// exactly the interval that matters.
func TestRegisterPool_ReadsAtScrapeTime(t *testing.T) {
	t.Parallel()

	m := New()
	current := fakeStats{acquired: 1, total: 1, max: 10}
	if err := m.RegisterPool(func() PoolStats { return current }); err != nil {
		t.Fatalf("RegisterPool() error = %v", err)
	}

	if body := scrapeBody(t, m); !strings.Contains(body, "kopiochi_db_pool_acquired_connections 1") {
		t.Fatalf("first scrape did not report 1:\n%s", body)
	}

	current = fakeStats{acquired: 9, total: 9, max: 10}

	if body := scrapeBody(t, m); !strings.Contains(body, "kopiochi_db_pool_acquired_connections 9") {
		t.Errorf("second scrape did not pick up the new value:\n%s", body)
	}
}

// TestRegisterPool_NilStatsDoesNotBreakTheScrape: the pool may not exist yet,
// or may be gone during shutdown. Reporting nothing is correct; failing the
// scrape would take every other metric down with it.
func TestRegisterPool_NilStatsDoesNotBreakTheScrape(t *testing.T) {
	t.Parallel()

	m := New()
	if err := m.RegisterPool(func() PoolStats { return nil }); err != nil {
		t.Fatalf("RegisterPool() error = %v", err)
	}

	body := scrapeBody(t, m)

	if strings.Contains(body, "kopiochi_db_pool_acquired_connections") {
		t.Error("pool series were reported despite nil stats")
	}
	// Everything else must still be there.
	if !strings.Contains(body, "kopiochi_http_requests_in_flight") {
		t.Errorf("a nil pool took the rest of the scrape down with it:\n%s", body)
	}
}

func TestRegisterPool_TwiceIsAnError(t *testing.T) {
	t.Parallel()

	m := New()
	if err := m.RegisterPool(func() PoolStats { return fakeStats{} }); err != nil {
		t.Fatalf("first RegisterPool() error = %v", err)
	}
	if err := m.RegisterPool(func() PoolStats { return fakeStats{} }); err == nil {
		t.Error("RegisterPool() twice returned no error; duplicate collectors would double-report")
	}
}
