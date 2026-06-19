// Package gnomon is a tiny, standardized BI layer: register business metrics
// (live ReadThrough or daily Snapshot), then capture and serve them. It owns no
// scheduling and no auth — the host wires Capture into its own job runner and
// wraps adminhttp with its own auth middleware.
package gnomon

import (
	"context"
	"fmt"
	"time"
)

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
)

// Metric is a single registered metric. For Snapshot kind, SQL must return a
// `value` column and may return a `dimension` column. For ReadThrough kind, SQL
// may return arbitrary columns.
type Metric struct {
	Name  string
	Title string
	Kind  Kind
	SQL   string
	Unit  Unit
	Chart Chart
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

// Register validates and adds metrics. Names must be unique and non-empty; SQL
// must be non-empty.
func (g *Gnomon) Register(ms ...Metric) error {
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
