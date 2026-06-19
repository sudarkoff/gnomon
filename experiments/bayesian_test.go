package experiments_test

import (
	"math"
	"testing"

	"github.com/sudarkoff/gnomon/experiments"
)

// TestBayesianKnownCase: control 20% vs treatment 24% — treatment clearly better.
func TestBayesianKnownCase(t *testing.T) {
	control := experiments.Arm{Name: "control", N: 1000, Conversions: 200}
	treatment := experiments.Arm{Name: "treatment", N: 1000, Conversions: 240}

	results := experiments.AnalyzeBayesian(control, []experiments.Arm{treatment}, 0.95)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]

	if r.ProbBeatControl < 0.95 || r.ProbBeatControl > 1.0 {
		t.Errorf("ProbBeatControl: want [0.95, 1.0], got %f", r.ProbBeatControl)
	}
	if r.ExpectedLoss < 0 {
		t.Errorf("ExpectedLoss must be >= 0, got %f", r.ExpectedLoss)
	}
	if r.ExpectedLoss >= 0.01 {
		t.Errorf("ExpectedLoss should be small for clearly better treatment, got %f", r.ExpectedLoss)
	}
	if !(r.LiftCILow < 0.04 && r.LiftCIHigh > 0.04) {
		t.Errorf("95%% CI should bracket 0.04: LiftCILow=%f, LiftCIHigh=%f", r.LiftCILow, r.LiftCIHigh)
	}
	if math.Abs(r.Rate-0.24) > 0.01 {
		t.Errorf("Rate: want ≈0.24, got %f", r.Rate)
	}
	if math.Abs(r.ControlRate-0.20) > 0.01 {
		t.Errorf("ControlRate: want ≈0.20, got %f", r.ControlRate)
	}
}

// TestBayesianReproducible: same inputs must yield bit-identical results (fixed seed).
func TestBayesianReproducible(t *testing.T) {
	control := experiments.Arm{Name: "control", N: 1000, Conversions: 200}
	treatment := experiments.Arm{Name: "treatment", N: 1000, Conversions: 240}

	r1 := experiments.AnalyzeBayesian(control, []experiments.Arm{treatment}, 0.95)
	r2 := experiments.AnalyzeBayesian(control, []experiments.Arm{treatment}, 0.95)

	if len(r1) != len(r2) {
		t.Fatalf("length mismatch: %d vs %d", len(r1), len(r2))
	}
	for i := range r1 {
		if r1[i] != r2[i] {
			t.Errorf("results[%d] differ: %+v vs %+v", i, r1[i], r2[i])
		}
	}
}

// TestBayesianEqualArms: same counts → ProbBeatControl ≈ 0.5.
func TestBayesianEqualArms(t *testing.T) {
	control := experiments.Arm{Name: "control", N: 1000, Conversions: 200}
	treatment := experiments.Arm{Name: "treatment", N: 1000, Conversions: 200}

	results := experiments.AnalyzeBayesian(control, []experiments.Arm{treatment}, 0.95)
	r := results[0]

	if math.Abs(r.ProbBeatControl-0.5) > 0.05 {
		t.Errorf("ProbBeatControl for equal arms: want ≈0.5 (±0.05), got %f", r.ProbBeatControl)
	}
}

// TestBayesianWorseTreatment: treatment clearly worse.
func TestBayesianWorseTreatment(t *testing.T) {
	control := experiments.Arm{Name: "control", N: 1000, Conversions: 240}
	treatment := experiments.Arm{Name: "treatment", N: 1000, Conversions: 200}

	results := experiments.AnalyzeBayesian(control, []experiments.Arm{treatment}, 0.95)
	r := results[0]

	if r.ProbBeatControl >= 0.5 {
		t.Errorf("ProbBeatControl for worse treatment: want < 0.5, got %f", r.ProbBeatControl)
	}
	// Expected loss should be meaningful (treatment lags control by ~4pp)
	if r.ExpectedLoss < 0.005 {
		t.Errorf("ExpectedLoss for worse treatment should be > 0.005, got %f", r.ExpectedLoss)
	}
	// CI high should be near or below 0 for clearly worse treatment
	if r.LiftCIHigh > 0.01 {
		t.Errorf("LiftCIHigh for clearly worse treatment expected near/below 0, got %f", r.LiftCIHigh)
	}
}

// TestBayesianAllFieldsFinite: no NaN/Inf even for degenerate inputs.
func TestBayesianAllFieldsFinite(t *testing.T) {
	arms := []struct {
		control   experiments.Arm
		treatment experiments.Arm
		label     string
	}{
		{
			experiments.Arm{Name: "control", N: 1000, Conversions: 200},
			experiments.Arm{Name: "treatment", N: 1000, Conversions: 240},
			"normal",
		},
		{
			experiments.Arm{Name: "control", N: 0, Conversions: 0},
			experiments.Arm{Name: "treatment", N: 1000, Conversions: 200},
			"zero_control_N",
		},
		{
			experiments.Arm{Name: "control", N: 1000, Conversions: 1000},
			experiments.Arm{Name: "treatment", N: 1000, Conversions: 1000},
			"all_converted",
		},
		{
			experiments.Arm{Name: "control", N: 1000, Conversions: 0},
			experiments.Arm{Name: "treatment", N: 1000, Conversions: 0},
			"zero_converted",
		},
	}

	for _, tc := range arms {
		results := experiments.AnalyzeBayesian(tc.control, []experiments.Arm{tc.treatment}, 0.95)
		r := results[0]
		assertBayesFinite(t, tc.label, r)
	}
}

// TestBayesianMultipleTreatments: 2 treatments → 2 results in order.
func TestBayesianMultipleTreatments(t *testing.T) {
	control := experiments.Arm{Name: "control", N: 1000, Conversions: 200}
	t1 := experiments.Arm{Name: "t1", N: 1000, Conversions: 240}
	t2 := experiments.Arm{Name: "t2", N: 1000, Conversions: 160}

	results := experiments.AnalyzeBayesian(control, []experiments.Arm{t1, t2}, 0.95)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "t1" {
		t.Errorf("results[0].Name: want t1, got %s", results[0].Name)
	}
	if results[1].Name != "t2" {
		t.Errorf("results[1].Name: want t2, got %s", results[1].Name)
	}
	// t1 is better, t2 is worse
	if results[0].ProbBeatControl < 0.95 {
		t.Errorf("t1 ProbBeatControl should be high, got %f", results[0].ProbBeatControl)
	}
	if results[1].ProbBeatControl > 0.05 {
		t.Errorf("t2 ProbBeatControl should be low, got %f", results[1].ProbBeatControl)
	}
}

func assertBayesFinite(t *testing.T, label string, r experiments.BayesResult) {
	t.Helper()
	fields := map[string]float64{
		"Rate":            r.Rate,
		"ControlRate":     r.ControlRate,
		"ProbBeatControl": r.ProbBeatControl,
		"ExpectedLoss":    r.ExpectedLoss,
		"LiftCILow":       r.LiftCILow,
		"LiftCIHigh":      r.LiftCIHigh,
	}
	for name, v := range fields {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("[%s] field %s is not finite: %v", label, name, v)
		}
	}
}
