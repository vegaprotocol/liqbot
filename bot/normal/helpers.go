package normal

import (
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"github.com/pkg/errors"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/optimize"
	"gonum.org/v1/gonum/stat/distuv"
)

func hexToRaw(hexBytes []byte) ([]byte, error) {
	raw := make([]byte, hex.DecodedLen(len(hexBytes)))
	n, err := hex.Decode(raw, hexBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode hex")
	}
	if n != len(raw) {
		return nil, fmt.Errorf("failed to decode hex: decoded %d bytes, expected to decode %d bytes", n, len(raw))
	}
	return raw, nil
}

func max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func abs(a int64) int64 {
	if a < 0 {
		return -a
	}
	return a
}

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

func DiscreteThreeLevelProbabilities(V []float64, muHat float64, sigmaHat float64) ([]float64, error) {
	Vsq := make([]float64, len(V))
	A := mat.NewDense(3, len(V), nil)
	for i := 0; i < len(V); i++ {
		Vsq[i] = V[i] * V[i]
		A.Set(0, i, V[i])
		A.Set(1, i, Vsq[i])
		A.Set(2, i, 1.0)
	}
	//fa := mat.Formatted(A, mat.Prefix("    "), mat.Squeeze())
	//fmt.Printf("A = %v\n\n", fa)

	b := mat.NewVecDense(3, nil)
	b.SetVec(0, muHat)
	b.SetVec(1, sigmaHat*sigmaHat+muHat*muHat)
	b.SetVec(2, 1.0)

	//p := mat.NewVecDense(3, nil)

	fnToMinimize := func(x []float64) float64 {
		xVec := mat.NewVecDense(3, x)
		lhsVec := mat.NewDense(3, 1, nil)
		lhsVec.Mul(A, xVec)
		rhsVec := mat.NewVecDense(3, nil)
		rhsVec.SubVec(lhsVec.ColView(0), b)
		norm := floats.Norm(rhsVec.RawVector().Data, 2)
		penalty := 0.0
		for i := 0; i < len(x); i++ {
			penaltyTerm := math.Min(0.0, x[i])
			penalty += 1e5 * penaltyTerm * penaltyTerm
		}
		//fmt.Printf("norm = %v, penalty = %v\n", norm, penalty)
		return norm + penalty
	}

	optProblem := optimize.Problem{
		Func: fnToMinimize,
		Grad: nil,
	}
	p0 := []float64{1 / 3, 1 / 3, 1 / 3}
	optResult, err := optimize.Minimize(optProblem, p0, nil, nil)
	if err != nil {
		return p0, err
	}
	if err = optResult.Status.Err(); err != nil {
		return p0, err
	}
	p := optResult.X
	//fmt.Printf("optResult.Status: %v\n", optResult.Status)
	//fmt.Printf("optResult.p: %0.4g\n", p)
	//fmt.Printf("result.F: %0.4g\n", optResult.F)
	//fmt.Printf("result.Stats.FuncEvaluations: %d\n", optResult.Stats.FuncEvaluations)

	//err := p.SolveVec(A, b)
	//fa = mat.Formatted(p, mat.Prefix("    "), mat.Squeeze())
	//fmt.Printf("p = %v\n\n", fa)
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

// input is a float price (so divide uint64 price  by 10^{num of decimals})
// it returns a float price which you want to multiply by 10^{num of decimals} and then round
func GeneratePriceUsingDiscreteThreeLevel(M0, delta, sigma, tgtTimeHorizonYrFrac, N float64) (price float64, err error) {
	err = nil
	muHat := -0.5 * sigma * sigma * tgtTimeHorizonYrFrac
	sigmaHat := math.Sqrt(N*tgtTimeHorizonYrFrac) * sigma
	V := make([]float64, 3)
	V[0] = N * math.Log((M0-delta)/M0)
	V[1] = 0.0
	V[2] = N * math.Log((M0+delta)/M0)
	probabilities, err := DiscreteThreeLevelProbabilities(V, muHat, sigmaHat)
	if err != nil {
		return 0, err
	}
	// now we have the probabilities - we just need to generate a random sample
	shockX := V[RandomChoice(probabilities)]
	Y := math.Exp(shockX / float64(N))
	price = M0 * Y
	return price, err

}

func RandomChoice(probabilities []float64) uint64 {
	x := distuv.UnitUniform.Rand()
	for i := uint64(0); i < uint64(len(probabilities)); i++ {
		x -= probabilities[i]
		if x <= 0 {
			return i
		}

	}
	return 0
}
