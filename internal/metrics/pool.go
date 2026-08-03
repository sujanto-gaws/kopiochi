package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// PoolStats is the subset of *pgxpool.Stat this package reads.
//
// Declared here rather than importing pgxpool so internal/metrics does not
// depend on the driver: *pgxpool.Stat satisfies it structurally, and a test
// can supply a plain struct. Pool saturation is usually the first visible
// symptom of the connection-churn problem in
// docs/architectures/05-data/persistence-and-pooling.md, so these are worth
// having before an incident rather than after.
type PoolStats interface {
	AcquiredConns() int32
	IdleConns() int32
	TotalConns() int32
	MaxConns() int32
	NewConnsCount() int64
	AcquireCount() int64
	EmptyAcquireCount() int64
	CanceledAcquireCount() int64
}

// poolCollector reports pool statistics.
//
// It is a Collector rather than a gauge updated on a ticker, so the values are
// read at scrape time. A ticker would add a goroutine to own and shut down,
// and would report numbers up to one tick stale — which for a saturation
// signal is exactly the interval you care about.
type poolCollector struct {
	stats func() PoolStats

	acquired         *prometheus.Desc
	idle             *prometheus.Desc
	total            *prometheus.Desc
	max              *prometheus.Desc
	newConns         *prometheus.Desc
	acquireCount     *prometheus.Desc
	emptyAcquire     *prometheus.Desc
	canceledAcquires *prometheus.Desc
}

// RegisterPool adds pool statistics to the registry. stats is called on every
// scrape and may return nil, which reports nothing rather than failing the
// scrape — the pool can legitimately not exist yet.
func (m *Metrics) RegisterPool(stats func() PoolStats) error {
	const sub = "db_pool"

	desc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}

	return m.registry.Register(&poolCollector{
		stats:            stats,
		acquired:         desc("acquired_connections", "Connections currently checked out."),
		idle:             desc("idle_connections", "Connections currently idle in the pool."),
		total:            desc("total_connections", "Connections currently in the pool, idle plus acquired."),
		max:              desc("max_connections", "Configured maximum pool size."),
		newConns:         desc("new_connections_total", "Connections established since start."),
		acquireCount:     desc("acquires_total", "Successful acquires since start."),
		emptyAcquire:     desc("empty_acquires_total", "Acquires that had to wait for a free connection."),
		canceledAcquires: desc("canceled_acquires_total", "Acquires cancelled by their context before succeeding."),
	})
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquired
	ch <- c.idle
	ch <- c.total
	ch <- c.max
	ch <- c.newConns
	ch <- c.acquireCount
	ch <- c.emptyAcquire
	ch <- c.canceledAcquires
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	if c.stats == nil {
		return
	}
	s := c.stats()
	if s == nil {
		return
	}

	gauge := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v)
	}
	counter := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v)
	}

	gauge(c.acquired, float64(s.AcquiredConns()))
	gauge(c.idle, float64(s.IdleConns()))
	gauge(c.total, float64(s.TotalConns()))
	gauge(c.max, float64(s.MaxConns()))

	// Monotonic since process start, so counters — a dashboard rate()s these.
	counter(c.newConns, float64(s.NewConnsCount()))
	counter(c.acquireCount, float64(s.AcquireCount()))
	// The one to alert on: a nonzero rate means requests are queueing for a
	// connection, which is saturation showing up before latency does.
	counter(c.emptyAcquire, float64(s.EmptyAcquireCount()))
	counter(c.canceledAcquires, float64(s.CanceledAcquireCount()))
}
