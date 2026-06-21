// Package gnomon is a tiny, standardized BI layer: register business metrics
// (live ReadThrough or daily Snapshot), then capture and serve them. It owns no
// scheduling and no auth -- the host wires Capture into its own job runner and
// wraps adminhttp with its own auth middleware.
package gnomon

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrUnknownMetric is returned by Query and Series when the requested metric
// name has not been registered.
var ErrUnknownMetric = errors.New("gnomon: unknown metric")

// ErrWrongKind is returned by Query when called on a Snapshot metric, or by
// Series when called on a ReadThrough metric.
var ErrWrongKind = errors.New("gnomon: wrong metric kind")

type Kind int

const (
	// ReadThrough metrics are queried live on request; nothing is stored.
	ReadThrough Kind = iota
	// Snapshot metrics are captured once per day into the snapshot store for trends.
	Snapshot
)

type Unit int

const (
	Count Unit = iota
	Percent
	Cents
)

type Chart int

const (
	Stat Chart = iota
	Line
	Bar
	Funnel
)

// Measure is one plotted/displayed series within a metric: which row column to
// read, an optional display label, and the unit to format it with.
type Measure struct {
	Name  string
	Label string
	Unit  Unit
}

// Metric is a single registered metric. For Snapshot kind, SQL must return a
// value column and may return a dimension column. For ReadThrough kind, SQL
// may return arbitrary columns; Dimension names the x-axis column and Measures
// names the columns to plot/show (in order).
type Metric struct {
	Name      string
	Title     string
	Kind      Kind
	SQL       string
	Unit      Unit
	Chart     Chart
	Dimension string
	Measures  []Measure
}

// Row is one result row from a ReadThrough query (column name -> value).
type Row map[string]any

// Sample is one captured snapshot value for a metric on a given day.
type Sample struct {
	Dimension string
	Value     float64
}

// Point is one stored snapshot data point read back from the store.
type Point struct {
	CapturedOn time.Time `json:"captured_on"`
	Dimension  string    `json:"dimension"`
	Value      float64   `json:"value"`
}

// DataSource runs metric SQL against the host's data database.
type DataSource interface {
	Query(ctx context.Context, sql string) ([]Row, error)
}

// Store persists and reads snapshot history.
type Store interface {
	UpsertSnapshots(ctx context.Context, on time.Time, metric string, samples []Sample) error
	ReadSeries(ctx context.Context, metric string, from, to time.Time) ([]Point, error)
}

// Gnomon is the engine: a metric registry over a DataSource + Store.
type Gnomon struct {
	data   DataSource
	store  Store
	byName map[string]Metric
	order  []string
}

func New(data DataSource, store Store) *Gnomon {
	return &Gnomon{data: data, store: store, byName: map[string]Metric{}}
}

// Register validates and adds metrics in a single atomic operation. Names must
// be unique (against existing registrations AND within the batch itself) and
// non-empty; SQL must be non-empty. If any metric in the batch fails validation,
// no metrics from the batch are registered.
func (g *Gnomon) Register(ms ...Metric) error {
	// First pass: validate ALL metrics before mutating anything.
	seen := make(map[string]struct{}, len(ms))
	for _, m := range ms {
		if m.Name == "" {
			return fmt.Errorf("gnomon: metric name must not be empty")
		}
		if m.SQL == "" {
			return fmt.Errorf("gnomon: metric %q has empty SQL", m.Name)
		}
		if _, dup := g.byName[m.Name]; dup {
			return fmt.Errorf("gnomon: duplicate metric name %q", m.Name)
		}
		if _, dup := seen[m.Name]; dup {
			return fmt.Errorf("gnomon: duplicate metric name %q in batch", m.Name)
		}
		seen[m.Name] = struct{}{}
	}
	// Second pass: commit only after all validations pass.
	for _, m := range ms {
		g.byName[m.Name] = m
		g.order = append(g.order, m.Name)
	}
	return nil
}

// Metrics returns registered metrics in registration order.
func (g *Gnomon) Metrics() []Metric {
	out := make([]Metric, 0, len(g.order))
	for _, name := range g.order {
		out = append(out, g.byName[name])
	}
	return out
}

// Capture runs every registered Snapshot metric's SQL, parses the rows into
// samples, and upserts them stamped with on. ReadThrough metrics are skipped.
// One metric's failure does not abort the rest; all errors are joined.
//
// A snapshot metric is all-or-nothing per capture: if any row fails to parse,
// that metric's entire sample set for the day is skipped and the error is
// joined (other metrics still capture).
func (g *Gnomon) Capture(ctx context.Context, on time.Time) error {
	var errs []error
	for _, name := range g.order {
		m := g.byName[name]
		if m.Kind != Snapshot {
			continue
		}
		rows, err := g.data.Query(ctx, m.SQL)
		if err != nil {
			errs = append(errs, fmt.Errorf("gnomon: query metric %q: %w", name, err))
			continue
		}
		samples, err := rowsToSamples(rows)
		if err != nil {
			errs = append(errs, fmt.Errorf("gnomon: parse metric %q: %w", name, err))
			continue
		}
		if err := g.store.UpsertSnapshots(ctx, on, name, samples); err != nil {
			errs = append(errs, fmt.Errorf("gnomon: store metric %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// Query runs a ReadThrough metric and returns its rows. It errors if the metric
// is unknown or is not a ReadThrough metric.
func (g *Gnomon) Query(ctx context.Context, name string) ([]Row, error) {
	m, ok := g.byName[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownMetric, name)
	}
	if m.Kind != ReadThrough {
		return nil, fmt.Errorf("%w: %q", ErrWrongKind, name)
	}
	return g.data.Query(ctx, m.SQL)
}

// Series reads stored snapshot history for a Snapshot metric in [from, to].
func (g *Gnomon) Series(ctx context.Context, name string, from, to time.Time) ([]Point, error) {
	m, ok := g.byName[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownMetric, name)
	}
	if m.Kind != Snapshot {
		return nil, fmt.Errorf("%w: %q", ErrWrongKind, name)
	}
	return g.store.ReadSeries(ctx, name, from, to)
}

func rowsToSamples(rows []Row) ([]Sample, error) {
	out := make([]Sample, 0, len(rows))
	for _, r := range rows {
		raw, ok := r["value"]
		if !ok {
			return nil, fmt.Errorf("gnomon: snapshot row missing 'value' column")
		}
		v, err := toFloat(raw)
		if err != nil {
			return nil, err
		}
		dim := ""
		if d, ok := r["dimension"]; ok && d != nil {
			dim = fmt.Sprintf("%v", d)
		}
		out = append(out, Sample{Dimension: dim, Value: v})
	}
	return out, nil
}
