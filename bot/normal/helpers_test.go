package normal

import (
	"fmt"
	"math"
	"testing"

	"gonum.org/v1/gonum/floats"
)

func TestDiscreteThreeLevel(t *testing.T) {
	pythonCalculatedP := []float64{6.33754599e-04, 9.98732510e-01, 6.33735671e-04}
	V := []float64{-18.04515056, 0.0, 17.95514944}
	muHat := -5.703855806525211e-05
	sigmaHat := 0.6408413361120014

	p, err := DiscreteThreeLevelProbabilities(V, muHat, sigmaHat)
	if err != nil {
		t.Error("Failed")
	} else {
		diff := make([]float64, len(p))
		floats.SubTo(diff, p, pythonCalculatedP)
		normOfDiff := floats.Norm(diff, 2)
		if normOfDiff > 1e-6 {
			errStr := fmt.Sprintf("Norm of diff is %v", normOfDiff)
			t.Error(errStr)
		}
	}

	// okay copy and paste for loop maybe in a test it's acceptable?
	pythonCalculatedP = []float64{0.50003918, 0.0, 0.49996082}
	V = []float64{-0.46011682, 0.0, 0.46005802}
	muHat = -0.000057
	sigmaHat = 0.641

	p, err = DiscreteThreeLevelProbabilities(V, muHat, sigmaHat)
	if err != nil {
		t.Error(err)
	} else {
		diff := make([]float64, len(p))
		floats.SubTo(diff, p, pythonCalculatedP)
		normOfDiff := floats.Norm(diff, 2)
		if normOfDiff > 1e-1 {
			errStr := fmt.Sprintf("Norm of diff is %v", normOfDiff)
			t.Error(errStr)
		}
	}
}

func TestRandomChoice(t *testing.T) {
	probs := []float64{0.1, 0.7, 0.2}
	numSamples := 10000000
	buckets := make([]int64, 3)
	for i := 0; i < len(buckets); i++ {
		buckets[i] = 0
	}

	for sampleIdx := 0; sampleIdx < numSamples; sampleIdx++ {
		i := randomChoice(probs)
		buckets[i]++
	}

	sampleProbs := make([]float64, 3)
	for i := 0; i < len(sampleProbs); i++ {
		sampleProbs[i] = float64(buckets[i]) / float64(numSamples)
	}

	diff := make([]float64, len(sampleProbs))
	floats.SubTo(diff, sampleProbs, probs)
	normOfDiff := floats.Norm(diff, 2)
	if normOfDiff > 1e-3 {
		t.Error("Failed")
	}
}

func TestThreeLevelPriceGen(t *testing.T) {
	// GeneratePriceUsingDiscreteThreeLevel(M0, delta, sigma, tgtTimeHorizonYrFrac, N float64)
	M0 := 39123.0                               // external price
	delta := float64(10)                        // i.e. we want jumps of this size
	sigma := 1.0                                // e.g. 5.0 = 500%
	tgtTimeHorizonYrFrac := 1.0 / 24.0 / 365.25 // i.e. one hour
	N := 3600                                   // 1/N times per hour

	muHat := -0.5 * sigma * sigma * tgtTimeHorizonYrFrac
	sigmaHat := math.Sqrt(float64(N)*tgtTimeHorizonYrFrac) * sigma
	V := make([]float64, 3)
	V[0] = float64(N) * math.Log((M0-delta)/M0)
	V[1] = 0.0
	V[2] = float64(N) * math.Log((M0+delta)/M0)
	probabilities, err := DiscreteThreeLevelProbabilities(V, muHat, sigmaHat)
	fmt.Printf("p: %0.4g\n", probabilities)

	if err != nil {
		t.Error(err)
	}

	priceLevels := make([]float64, N)
	priceLevels[0] = M0

	for i := 0; i < len(priceLevels)-1; i++ {
		price, err := GeneratePriceUsingDiscreteThreeLevel(priceLevels[i], delta, sigma, tgtTimeHorizonYrFrac, float64(N))
		if err != nil {
			t.Error(err)
		}
		priceLevels[i+1] = price
		fmt.Printf("%v, %f\n", i, price)
	}
}
