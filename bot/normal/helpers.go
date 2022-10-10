package normal

import (
	"errors"
	"math"
	"time"

	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/optimize"
	"gonum.org/v1/gonum/stat/distuv"
)

func doze(d time.Duration, stop chan bool) error {
	interval := 100 * time.Millisecond

	for d > interval {
		select {
		case <-stop:
			return errors.New("doze interrupted")

		default:
			time.Sleep(interval)
			d -= interval
		}
	}

	time.Sleep(d)

	return nil
}

// discreteThreeLevelProbabilities is a method for calculating price levels.
func discreteThreeLevelProbabilities(V []float64, muHat float64, sigmaHat float64) ([]float64, error) {
	vsq := make([]float64, len(V))
	a := mat.NewDense(3, len(V), nil)

	for i := 0; i < len(V); i++ {
		vsq[i] = V[i] * V[i]
		a.Set(0, i, V[i])
		a.Set(1, i, vsq[i])
		a.Set(2, i, 1.0)
	}

	b := mat.NewVecDense(3, nil)
	b.SetVec(0, muHat)
	b.SetVec(1, sigmaHat*sigmaHat+muHat*muHat)
	b.SetVec(2, 1.0)

	fnToMinimize := func(x []float64) float64 {
		xVec := mat.NewVecDense(3, x)
		lhsVec := mat.NewDense(3, 1, nil)
		lhsVec.Mul(a, xVec)
		rhsVec := mat.NewVecDense(3, nil)
		rhsVec.SubVec(lhsVec.ColView(0), b)
		norm := floats.Norm(rhsVec.RawVector().Data, 2)
		penalty := 0.0

		for i := 0; i < len(x); i++ {
			penaltyTerm := math.Min(0.0, x[i])
			penalty += 1e5 * penaltyTerm * penaltyTerm
		}

		return norm + penalty
	}

	optProblem := optimize.Problem{
		Func: fnToMinimize,
		Grad: nil,
	}

	p0 := []float64{1.0 / 3.0, 1.0 / 3.0, 1.0 / 3.0}

	optResult, err := optimize.Minimize(optProblem, p0, nil, nil)
	if err != nil {
		return p0, err
	}

	if err = optResult.Status.Err(); err != nil {
		return p0, err
	}

	p := optResult.X

	correction := 0.0

	for i := 0; i < len(p); i++ {
		if p[i] < 0 {
			correction -= p[i]
			p[i] = 0.0
		}
	}

	p[2] = p[2] + correction

	return p, err
}

// generatePriceUsingDiscreteThreeLevel is a method for calculating price levels
// input is a float price (so divide uint64 price  by 10^{num of decimals})
// it returns a float price which you want to multiply by 10^{num of decimals} and then round.
func generatePriceUsingDiscreteThreeLevel(m0, delta, sigma, tgtTimeHorizonYrFrac, n float64) (float64, error) {
	muHat := -0.5 * sigma * sigma * tgtTimeHorizonYrFrac
	sigmaHat := math.Sqrt(n*tgtTimeHorizonYrFrac) * sigma
	v := make([]float64, 3)

	v[0] = n * math.Log((m0-delta)/m0)
	v[1] = 0.0
	v[2] = n * math.Log((m0+delta)/m0)

	probabilities, err := discreteThreeLevelProbabilities(v, muHat, sigmaHat)
	if err != nil {
		return 0, err
	}

	// now we have the probabilities - we just need to generate a random sample
	shockX := v[randomChoice(probabilities)]
	y := math.Exp(shockX / n)

	price := m0 * y

	return price, nil
}

func randomChoice(probabilities []float64) uint64 {
	x := distuv.UnitUniform.Rand()

	for i := uint64(0); i < uint64(len(probabilities)); i++ {
		x -= probabilities[i]
		if x <= 0 {
			return i
		}
	}

	return 0
}
