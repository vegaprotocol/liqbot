package normal

import (
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"github.com/pkg/errors"
	"gonum.org/v1/gonum/mat"
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
	b := mat.NewVecDense(3, nil)
	b.SetVec(0, muHat)
	b.SetVec(1, sigmaHat*sigmaHat+muHat*muHat)
	b.SetVec(2, 1.0)
	p := mat.NewVecDense(3, nil)

	err := p.SolveVec(A, b)
	return p.RawVector().Data, err
}

func GeneratePriceUsingDiscreteThreeLevel(M0, delta, sigma, tgtTimeHorizonYrFrac, N float64) (price uint64, err error) {
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
	price = uint64(math.Round(M0 * Y))
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
