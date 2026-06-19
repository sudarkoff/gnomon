package gnomon

import (
	"context"
	"testing"
	"time"
)

type fakeData struct {
	rows map[string][]Row
	err  map[string]error
}

func (f fakeData) Query(_ context.Context, sql string) ([]Row, error) {
	if e := f.err[sql]; e != nil {
		return nil, e
	}
	return f.rows[sql], nil
}

type capturedCall struct {
	metric  string
	samples []Sample
}

type fakeStore struct {
	calls  []capturedCall
	series []Point
}

func (f *fakeStore) UpsertSnapshots(_ context.Context, _ time.Time, metric string, s []Sample) error {
	f.calls = append(f.calls, capturedCall{metric, s})
	return nil
}
func (f *fakeStore) ReadSeries(_ context.Context, _ string, _, _ time.Time) ([]Point, error) {
	return f.series, nil
}

func TestCaptureScalarAndGrouped(t *testing.T) {
	data := fakeData{rows: map[string][]Row{
		"SCALAR":  {{"value": int64(42)}},
		"GROUPED": {{"dimension": "month", "value": float64(3)}, {"dimension": "year", "value": float64(1)}},
	}}
	store := &fakeStore{}
	g := New(data, store)
	if err := g.Register(
		Metric{Name: "s", SQL: "SCALAR", Kind: Snapshot},
		Metric{Name: "g", SQL: "GROUPED", Kind: Snapshot},
		Metric{Name: "rt", SQL: "IGNORED", Kind: ReadThrough},
	); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := g.Capture(context.Background(), time.Now()); err != nil {
		t.Fatalf("capture: %v", err)
	}
	if len(store.calls) != 2 {
		t.Fatalf("expected 2 upserts (snapshot metrics only), got %d", len(store.calls))
	}
	// scalar -> one sample, empty dimension
	if store.calls[0].metric != "s" || len(store.calls[0].samples) != 1 ||
		store.calls[0].samples[0].Dimension != "" || store.calls[0].samples[0].Value != 42 {
		t.Fatalf("bad scalar capture: %+v", store.calls[0])
	}
	// grouped -> two samples with dimensions
	if store.calls[1].metric != "g" || len(store.calls[1].samples) != 2 ||
		store.calls[1].samples[0].Dimension != "month" {
		t.Fatalf("bad grouped capture: %+v", store.calls[1])
	}
}

func TestCaptureJoinsErrors(t *testing.T) {
	data := fakeData{
		rows: map[string][]Row{"OK": {{"value": int64(1)}}},
		err:  map[string]error{"BAD": context.DeadlineExceeded},
	}
	store := &fakeStore{}
	g := New(data, store)
	_ = g.Register(
		Metric{Name: "bad", SQL: "BAD", Kind: Snapshot},
		Metric{Name: "ok", SQL: "OK", Kind: Snapshot},
	)
	err := g.Capture(context.Background(), time.Now())
	if err == nil {
		t.Fatal("expected error from failing metric")
	}
	// the good metric still got captured
	if len(store.calls) != 1 || store.calls[0].metric != "ok" {
		t.Fatalf("good metric should still capture: %+v", store.calls)
	}
}
