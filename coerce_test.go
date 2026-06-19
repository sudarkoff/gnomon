package gnomon

import (
	"math"
	"testing"
	"time"
)

// coerce converts a raw database value to a float64 for use as a Sample.Value.
// It handles the numeric types that a pgx scan into any produces.

func TestCoerceInt64(t *testing.T) {
	got, err := coerce(int64(42))
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Fatalf("want 42, got %v", got)
	}
}

func TestCoerceFloat64(t *testing.T) {
	got, err := coerce(float64(3.14))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-3.14) > 1e-9 {
		t.Fatalf("want 3.14, got %v", got)
	}
}

func TestCoerceFloat32(t *testing.T) {
	got, err := coerce(float32(1.5))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-1.5) > 1e-6 {
		t.Fatalf("want 1.5, got %v", got)
	}
}

func TestCoerceInt32(t *testing.T) {
	got, err := coerce(int32(7))
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Fatalf("want 7, got %v", got)
	}
}

func TestCoerceInt(t *testing.T) {
	got, err := coerce(int(99))
	if err != nil {
		t.Fatal(err)
	}
	if got != 99 {
		t.Fatalf("want 99, got %v", got)
	}
}

func TestCoerceStringNumeric(t *testing.T) {
	got, err := coerce("123.45")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-123.45) > 1e-9 {
		t.Fatalf("want 123.45, got %v", got)
	}
}

func TestCoerceStringInvalid(t *testing.T) {
	_, err := coerce("not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric string")
	}
}

func TestCoerceNil(t *testing.T) {
	got, err := coerce(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("want 0 for nil, got %v", got)
	}
}

func TestCoerceUnsupportedType(t *testing.T) {
	_, err := coerce(time.Now())
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}
