package gnomon

import (
	"context"
	"testing"
	"time"
)

func TestQueryReadThrough(t *testing.T) {
	data := fakeData{rows: map[string][]Row{"RT": {{"a": int64(1)}}}}
	g := New(data, &fakeStore{})
	_ = g.Register(Metric{Name: "rt", SQL: "RT", Kind: ReadThrough})

	rows, err := g.Query(context.Background(), "rt")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if _, err := g.Query(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for unknown metric")
	}
	_ = g.Register(Metric{Name: "snap", SQL: "X", Kind: Snapshot})
	if _, err := g.Query(context.Background(), "snap"); err == nil {
		t.Fatal("expected error querying a Snapshot metric via Query")
	}
}

func TestSeries(t *testing.T) {
	store := &fakeStore{series: []Point{{Dimension: "", Value: 5}}}
	g := New(fakeData{}, store)
	_ = g.Register(Metric{Name: "snap", SQL: "X", Kind: Snapshot})

	pts, err := g.Series(context.Background(), "snap", time.Now().AddDate(0, 0, -7), time.Now())
	if err != nil {
		t.Fatalf("series: %v", err)
	}
	if len(pts) != 1 || pts[0].Value != 5 {
		t.Fatalf("bad series: %+v", pts)
	}
	if _, err := g.Series(context.Background(), "missing", time.Time{}, time.Time{}); err == nil {
		t.Fatal("expected error for unknown metric")
	}
}
