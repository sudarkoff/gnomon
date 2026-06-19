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
