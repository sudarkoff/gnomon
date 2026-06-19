package gnomon

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestToFloat(t *testing.T) {
	num := pgtype.Numeric{}
	if err := num.Scan("19.50"); err != nil {
		t.Fatalf("scan numeric: %v", err)
	}
	cases := []struct {
		in   any
		want float64
	}{
		{int64(5), 5},
		{int32(7), 7},
		{float64(3.5), 3.5},
		{num, 19.5},
	}
	for _, c := range cases {
		got, err := toFloat(c.in)
		if err != nil {
			t.Fatalf("toFloat(%v): %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("toFloat(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := toFloat("nope"); err == nil {
		t.Fatal("expected error for string")
	}
	if _, err := toFloat(nil); err == nil {
		t.Fatal("expected error for nil")
	}
}
