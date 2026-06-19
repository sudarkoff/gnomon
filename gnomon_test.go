package gnomon

import "testing"

func TestRegisterRejectsEmptyName(t *testing.T) {
	g := New(nil, nil)
	if err := g.Register(Metric{Name: "", SQL: "SELECT 1"}); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestRegisterRejectsEmptySQL(t *testing.T) {
	g := New(nil, nil)
	if err := g.Register(Metric{Name: "x", SQL: ""}); err == nil {
		t.Fatal("expected error for empty SQL")
	}
}

func TestRegisterRejectsDuplicate(t *testing.T) {
	g := New(nil, nil)
	if err := g.Register(Metric{Name: "x", SQL: "SELECT 1"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := g.Register(Metric{Name: "x", SQL: "SELECT 1"}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestMetricsPreservesOrder(t *testing.T) {
	g := New(nil, nil)
	if err := g.Register(
		Metric{Name: "b", SQL: "SELECT 1"},
		Metric{Name: "a", SQL: "SELECT 1"},
	); err != nil {
		t.Fatalf("register: %v", err)
	}
	got := g.Metrics()
	if len(got) != 2 || got[0].Name != "b" || got[1].Name != "a" {
		t.Fatalf("order not preserved: %+v", got)
	}
}

// TestRegisterBatchAtomic verifies that a batch with an intra-batch duplicate
// returns an error AND leaves Metrics() empty -- no partial registration.
func TestRegisterBatchAtomic(t *testing.T) {
	g := New(nil, nil)
	err := g.Register(
		Metric{Name: "a", SQL: "SELECT 1"},
		Metric{Name: "a", SQL: "SELECT 2"},
	)
	if err == nil {
		t.Fatal("expected error for intra-batch duplicate")
	}
	if got := g.Metrics(); len(got) != 0 {
		t.Fatalf("expected no metrics registered after failed batch, got %d: %+v", len(got), got)
	}
}
