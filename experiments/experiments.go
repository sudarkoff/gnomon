// Package experiments provides a tiny frequentist A/B analysis core: compare
// treatment arms to a control with a two-proportion z-test, reporting lift,
// significance, and a confidence interval. Domain-agnostic — it operates on
// raw (exposed, converted) counts and has no database or gnomon-engine deps.
package experiments

import "math"

// Arm is one variation's observed counts.
type Arm struct {
	Name        string
	N           int // exposed units
	Conversions int // units that converted
}

// Result is one treatment arm compared to the control arm.
type Result struct {
	Name        string  `json:"name"`
	Rate        float64 `json:"rate"`         // treatment conversion rate
	ControlRate float64 `json:"control_rate"`
	AbsLift     float64 `json:"abs_lift"`     // rate - control_rate
	RelLift     float64 `json:"rel_lift"`     // (rate - control_rate) / control_rate; 0 when control_rate==0
	SE          float64 `json:"se"`           // std error of the difference (unpooled, used for the CI)
	Z           float64 `json:"z"`
	PValue      float64 `json:"p_value"`      // two-sided
	CILow       float64 `json:"ci_low"`       // CI bounds on the ABSOLUTE difference
	CIHigh      float64 `json:"ci_high"`
	Significant bool    `json:"significant"`  // PValue < (1 - conf)
}

// Analyze compares each treatment arm to control via a two-proportion z-test.
// conf is the confidence level, e.g. 0.95.
func Analyze(control Arm, treatments []Arm, conf float64) []Result {
	results := make([]Result, 0, len(treatments))

	nC := control.N
	cC := control.Conversions
	var pC float64
	if nC > 0 {
		pC = float64(cC) / float64(nC)
	}

	for _, t := range treatments {
		nT := t.N
		cT := t.Conversions
		var pT float64
		if nT > 0 {
			pT = float64(cT) / float64(nT)
		}

		absLift := pT - pC
		var relLift float64
		if pC != 0 {
			relLift = absLift / pC
		}

		// Edge case: either arm has N==0 — return a degenerate result.
		if nT == 0 || nC == 0 {
			r := Result{
				Name:        t.Name,
				Rate:        pT,
				ControlRate: pC,
				AbsLift:     absLift,
				RelLift:     0,
				SE:          0,
				Z:           0,
				PValue:      1,
				CILow:       absLift,
				CIHigh:      absLift,
				Significant: false,
			}
			results = append(results, sanitize(r))
			continue
		}

		// Pooled z-test.
		phat := float64(cT+cC) / float64(nT+nC)
		sePool := math.Sqrt(phat * (1 - phat) * (1/float64(nT) + 1/float64(nC)))

		var z float64
		var pval float64 = 1
		if sePool > 0 {
			z = (pT - pC) / sePool
			pval = math.Erfc(math.Abs(z) / math.Sqrt2)
		}

		// Wald (unpooled) SE for the CI.
		seWald := math.Sqrt(pT*(1-pT)/float64(nT) + pC*(1-pC)/float64(nC))
		zcrit := inverseNormalCDF((1 + conf) / 2)
		ciLow := absLift - zcrit*seWald
		ciHigh := absLift + zcrit*seWald

		r := Result{
			Name:        t.Name,
			Rate:        pT,
			ControlRate: pC,
			AbsLift:     absLift,
			RelLift:     relLift,
			SE:          seWald,
			Z:           z,
			PValue:      pval,
			CILow:       ciLow,
			CIHigh:      ciHigh,
			Significant: pval < (1 - conf),
		}
		results = append(results, sanitize(r))
	}
	return results
}

// sanitize replaces any NaN or Inf fields with 0 to guarantee JSON-safety.
func sanitize(r Result) Result {
	r.Rate = finiteOr(r.Rate, 0)
	r.ControlRate = finiteOr(r.ControlRate, 0)
	r.AbsLift = finiteOr(r.AbsLift, 0)
	r.RelLift = finiteOr(r.RelLift, 0)
	r.SE = finiteOr(r.SE, 0)
	r.Z = finiteOr(r.Z, 0)
	r.PValue = finiteOr(r.PValue, 1)
	r.CILow = finiteOr(r.CILow, 0)
	r.CIHigh = finiteOr(r.CIHigh, 0)
	return r
}

func finiteOr(v, fallback float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fallback
	}
	return v
}

// inverseNormalCDF approximates the inverse of the standard normal CDF using
// the Acklam rational approximation. Accurate to ~1e-9 for p in (0, 1).
// Returns approximately 1.959964 for p=0.975.
func inverseNormalCDF(p float64) float64 {
	// Coefficients for the rational approximation.
	const (
		a1 = -3.969683028665376e+01
		a2 =  2.209460984245205e+02
		a3 = -2.759285104469687e+02
		a4 =  1.383577518672690e+02
		a5 = -3.066479806614716e+01
		a6 =  2.506628277459239e+00

		b1 = -5.447609879822406e+01
		b2 =  1.615858368580409e+02
		b3 = -1.556989798598866e+02
		b4 =  6.680131188771972e+01
		b5 = -1.328068155288572e+01

		c1 = -7.784894002430293e-03
		c2 = -3.223964580411365e-01
		c3 = -2.400758277161838e+00
		c4 = -2.549732539343734e+00
		c5 =  4.374664141464968e+00
		c6 =  2.938163982698783e+00

		d1 = 7.784695709041462e-03
		d2 = 3.224671290700398e-01
		d3 = 2.445134137142996e+00
		d4 = 3.754408661907416e+00

		pLow  = 0.02425
		pHigh = 1 - pLow
	)

	switch {
	case p <= 0:
		return math.Inf(-1)
	case p >= 1:
		return math.Inf(1)
	case p < pLow:
		// Lower tail.
		q := math.Sqrt(-2 * math.Log(p))
		return (((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	case p <= pHigh:
		// Central region.
		q := p - 0.5
		r := q * q
		return (((((a1*r+a2)*r+a3)*r+a4)*r+a5)*r+a6)*q /
			(((((b1*r+b2)*r+b3)*r+b4)*r+b5)*r+1)
	default:
		// Upper tail — use symmetry.
		q := math.Sqrt(-2 * math.Log(1-p))
		return -(((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	}
}
