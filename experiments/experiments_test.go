package experiments_test

import (
	"math"
	"testing"

	"github.com/sudarkoff/gnomon/experiments"
)

const epsilon = 1e-6

func TestAnalyzeKnownCase(t *testing.T) {
	control := experiments.Arm{Name: "control", N: 1000, Conversions: 200}
	treatment := experiments.Arm{Name: "treatment", N: 1000, Conversions: 240}

	results := experiments.Analyze(control, []experiments.Arm{treatment}, 0.95)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]

	if math.Abs(r.ControlRate-0.20) > 0.001 {
		t.Errorf("ControlRate: want ≈0.20, got %f", r.ControlRate)
	}
	if math.Abs(r.Rate-0.24) > 0.001 {
		t.Errorf("Rate: want ≈0.24, got %f", r.Rate)
	}
	if math.Abs(r.AbsLift-0.04) > 0.001 {
		t.Errorf("AbsLift: want ≈0.04, got %f", r.AbsLift)
	}
	if math.Abs(r.RelLift-0.20) > 0.001 {
		t.Errorf("RelLift: want ≈0.20, got %f", r.RelLift)
	}
	if math.Abs(r.Z-2.16) > 0.02 {
		t.Errorf("Z: want ≈2.16 (±0.02), got %f", r.Z)
	}
	if math.Abs(r.PValue-0.031) > 0.003 {
		t.Errorf("PValue: want ≈0.031 (±0.003), got %f", r.PValue)
	}
	if !r.Significant {
		t.Errorf("Significant: want true at conf=0.95, got false (PValue=%f)", r.PValue)
	}
}

func TestInverseNormalCDF(t *testing.T) {
	// We test via the CI bounds which implicitly call inverseNormalCDF.
	// For direct testing we do a round-trip: at 95% conf the zcrit should be
	// ~1.95996. We verify that the CI is about ±zcrit*SE wide.
	control := experiments.Arm{Name: "control", N: 1000, Conversions: 500}
	treatment := experiments.Arm{Name: "treatment", N: 1000, Conversions: 500}
	results := experiments.Analyze(control, []experiments.Arm{treatment}, 0.95)
	r := results[0]

	// SE = sqrt(0.5*0.5/1000 + 0.5*0.5/1000) = sqrt(0.0005) ≈ 0.022360679...
	se := math.Sqrt(0.5*0.5/1000 + 0.5*0.5/1000)
	// zcrit for 0.975 ≈ 1.95996
	expectedHalfWidth := 1.95996 * se
	actualHalfWidth := (r.CIHigh - r.CILow) / 2

	if math.Abs(actualHalfWidth-expectedHalfWidth) > 0.001 {
		t.Errorf("CI half-width: want ≈%f (zcrit≈1.95996), got %f", expectedHalfWidth, actualHalfWidth)
	}

	// Also verify zcrit for 0.95 arm: use conf=0.90 so zcrit=inverseNormalCDF(0.95)≈1.6449
	results90 := experiments.Analyze(control, []experiments.Arm{treatment}, 0.90)
	r90 := results90[0]
	expectedHalfWidth90 := 1.6449 * se
	actualHalfWidth90 := (r90.CIHigh - r90.CILow) / 2
	if math.Abs(actualHalfWidth90-expectedHalfWidth90) > 0.001 {
		t.Errorf("CI half-width (conf=0.90): want ≈%f (zcrit≈1.6449), got %f", expectedHalfWidth90, actualHalfWidth90)
	}
}

func TestZeroN(t *testing.T) {
	control := experiments.Arm{Name: "control", N: 0, Conversions: 0}
	treatment := experiments.Arm{Name: "treatment", N: 100, Conversions: 10}

	results := experiments.Analyze(control, []experiments.Arm{treatment}, 0.95)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]

	if r.Significant {
		t.Error("Significant should be false when N=0")
	}
	assertFinite(t, "ZeroN", r)
}

func TestZeroConversions(t *testing.T) {
	control := experiments.Arm{Name: "control", N: 1000, Conversions: 0}
	treatment := experiments.Arm{Name: "treatment", N: 1000, Conversions: 0}

	results := experiments.Analyze(control, []experiments.Arm{treatment}, 0.95)
	r := results[0]

	if r.Significant {
		t.Error("Significant should be false when both conversions=0")
	}
	assertFinite(t, "ZeroConversions", r)
}

func TestNegativeLift(t *testing.T) {
	control := experiments.Arm{Name: "control", N: 1000, Conversions: 300}
	treatment := experiments.Arm{Name: "treatment", N: 1000, Conversions: 200}

	results := experiments.Analyze(control, []experiments.Arm{treatment}, 0.95)
	r := results[0]

	if r.AbsLift >= 0 {
		t.Errorf("AbsLift should be negative, got %f", r.AbsLift)
	}
	if r.RelLift >= 0 {
		t.Errorf("RelLift should be negative, got %f", r.RelLift)
	}
	assertFinite(t, "NegativeLift", r)
}

func TestAllFieldsFinite(t *testing.T) {
	control := experiments.Arm{Name: "control", N: 1000, Conversions: 0}
	arms := []experiments.Arm{
		{Name: "zero_control_nonzero_treatment", N: 1000, Conversions: 100},
		{Name: "n_zero", N: 0, Conversions: 0},
		{Name: "all_converted_treatment", N: 1000, Conversions: 1000},
	}

	results := experiments.Analyze(control, arms, 0.95)
	for _, r := range results {
		assertFinite(t, r.Name, r)
	}

	// Also test control with N=0
	control0 := experiments.Arm{Name: "control", N: 0, Conversions: 0}
	arms0 := []experiments.Arm{
		{Name: "treatment_with_zero_control", N: 500, Conversions: 50},
	}
	results0 := experiments.Analyze(control0, arms0, 0.95)
	for _, r := range results0 {
		assertFinite(t, r.Name+"(zero control)", r)
	}
}

func TestAnalyzeMultipleTreatments(t *testing.T) {
	control := experiments.Arm{Name: "control", N: 1000, Conversions: 200}
	t1 := experiments.Arm{Name: "t1", N: 1000, Conversions: 220}
	t2 := experiments.Arm{Name: "t2", N: 1000, Conversions: 180}

	results := experiments.Analyze(control, []experiments.Arm{t1, t2}, 0.95)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "t1" {
		t.Errorf("results[0].Name: want t1, got %s", results[0].Name)
	}
	if results[1].Name != "t2" {
		t.Errorf("results[1].Name: want t2, got %s", results[1].Name)
	}
}

// assertFinite checks that every numeric field in a Result is finite (not NaN or Inf).
func assertFinite(t *testing.T, label string, r experiments.Result) {
	t.Helper()
	fields := map[string]float64{
		"Rate":        r.Rate,
		"ControlRate": r.ControlRate,
		"AbsLift":     r.AbsLift,
		"RelLift":     r.RelLift,
		"SE":          r.SE,
		"Z":           r.Z,
		"PValue":      r.PValue,
		"CILow":       r.CILow,
		"CIHigh":      r.CIHigh,
	}
	for name, v := range fields {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("[%s] field %s is not finite: %v", label, name, v)
		}
	}
}

var _ = epsilon // suppress unused warning
