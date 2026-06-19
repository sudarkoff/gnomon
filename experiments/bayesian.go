package experiments

import (
	"math"
	"math/rand"
	"sort"
)

// BayesResult is one treatment arm's Bayesian comparison to control, using
// Beta(1,1) (uniform) posteriors and Monte Carlo simulation.
type BayesResult struct {
	Name            string  `json:"name"`
	Rate            float64 `json:"rate"`             // treatment posterior-mean rate
	ControlRate     float64 `json:"control_rate"`
	ProbBeatControl float64 `json:"prob_beat_control"` // P(rate_t > rate_c)
	ExpectedLoss    float64 `json:"expected_loss"`     // E[max(0, rate_c - rate_t)] — risk of shipping the treatment
	LiftCILow       float64 `json:"lift_ci_low"`       // credible interval on absolute lift (rate_t - rate_c)
	LiftCIHigh      float64 `json:"lift_ci_high"`
}

const bayesianSamples = 100_000

// AnalyzeBayesian compares each treatment arm to control via Monte Carlo over
// Beta posteriors. conf is the credible level for the lift interval, e.g. 0.95.
// Deterministic: uses a fixed internal seed, so identical inputs yield identical
// results.
func AnalyzeBayesian(control Arm, treatments []Arm, conf float64) []BayesResult {
	// Local RNG with fixed seed — never touches global rand.
	rng := rand.New(rand.NewSource(1))

	// Beta posterior parameters for control: Beta(1 + conversions, 1 + (N - conversions))
	aC := float64(1 + control.Conversions)
	bC := float64(1 + (control.N - control.Conversions))

	// Control posterior mean.
	controlRate := aC / (aC + bC)

	// Draw control samples once; reuse across all treatments for paired comparison.
	controlDraws := make([]float64, bayesianSamples)
	for i := range controlDraws {
		controlDraws[i] = betaSample(rng, aC, bC)
	}

	results := make([]BayesResult, 0, len(treatments))
	for _, t := range treatments {
		aT := float64(1 + t.Conversions)
		bT := float64(1 + (t.N - t.Conversions))

		treatmentRate := aT / (aT + bT)

		// Draw treatment samples and compute per-draw statistics.
		var probBeat, lossSum float64
		lifts := make([]float64, bayesianSamples)

		for i := 0; i < bayesianSamples; i++ {
			tDraw := betaSample(rng, aT, bT)
			cDraw := controlDraws[i]

			lift := tDraw - cDraw
			lifts[i] = lift

			if tDraw > cDraw {
				probBeat++
			}
			if cDraw > tDraw {
				lossSum += cDraw - tDraw
			}
		}

		probBeat /= float64(bayesianSamples)
		expectedLoss := lossSum / float64(bayesianSamples)

		// Empirical credible interval on absolute lift.
		sort.Float64s(lifts)
		lo := (1 - conf) / 2
		hi := (1 + conf) / 2
		liftCILow := quantile(lifts, lo)
		liftCIHigh := quantile(lifts, hi)

		r := BayesResult{
			Name:            t.Name,
			Rate:            treatmentRate,
			ControlRate:     controlRate,
			ProbBeatControl: probBeat,
			ExpectedLoss:    expectedLoss,
			LiftCILow:       liftCILow,
			LiftCIHigh:      liftCIHigh,
		}
		results = append(results, sanitizeBayes(r))
	}
	return results
}

// betaSample draws one sample from Beta(a, b) using the relation
// Beta(a,b) = Ga/(Ga+Gb) where Ga ~ Gamma(a,1), Gb ~ Gamma(b,1).
func betaSample(rng *rand.Rand, a, b float64) float64 {
	ga := gammaSample(rng, a)
	gb := gammaSample(rng, b)
	sum := ga + gb
	if sum == 0 {
		// Degenerate: return 0.5 as a safe fallback.
		return 0.5
	}
	return ga / sum
}

// gammaSample draws one sample from Gamma(shape, 1) using the Marsaglia–Tsang
// method for shape >= 1. For shape < 1, uses the boost: Gamma(a) = Gamma(a+1)*U^(1/a).
func gammaSample(rng *rand.Rand, shape float64) float64 {
	if shape < 1 {
		// Boost: Gamma(shape) = Gamma(shape+1) * U^(1/shape)
		u := rng.Float64()
		if u == 0 {
			u = math.SmallestNonzeroFloat64
		}
		return gammaSample(rng, shape+1) * math.Pow(u, 1/shape)
	}

	// Marsaglia–Tsang (2000) for shape >= 1.
	d := shape - 1.0/3.0
	c := 1.0 / math.Sqrt(9*d)
	for {
		x := rng.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v // v^3
		u := rng.Float64()
		// Squeeze acceptance test (fast path).
		xSq := x * x
		if u < 1-0.0331*(xSq*xSq) {
			return d * v
		}
		// Log acceptance test (slow path).
		if math.Log(u) < 0.5*xSq+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}

// quantile returns the p-th quantile (0 ≤ p ≤ 1) of a sorted slice.
func quantile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[n-1]
	}
	// Linear interpolation.
	pos := p * float64(n-1)
	lo := int(pos)
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// sanitizeBayes replaces any NaN or Inf fields with safe defaults.
func sanitizeBayes(r BayesResult) BayesResult {
	r.Rate = finiteOr(r.Rate, 0)
	r.ControlRate = finiteOr(r.ControlRate, 0)
	r.ProbBeatControl = finiteOr(r.ProbBeatControl, 0)
	r.ExpectedLoss = finiteOr(r.ExpectedLoss, 0)
	r.LiftCILow = finiteOr(r.LiftCILow, 0)
	r.LiftCIHigh = finiteOr(r.LiftCIHigh, 0)
	return r
}
