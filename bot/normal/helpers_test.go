package normal

import (
	"fmt"
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
			t.Error("Failed")
		}
	}
	fmt.Printf("Bye!")
}

func TestRandomChoice(t *testing.T) {
	probs := []float64{0.1, 0.7, 0.2}
	numSamples := 10000000
	buckets := make([]int64, 3)
	for i := 0; i < len(buckets); i++ {
		buckets[i] = 0
	}

	for sampleIdx := 0; sampleIdx < numSamples; sampleIdx++ {
		i := RandomChoice(probs)
		buckets[i] += 1
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
	//GeneratePriceUsingDiscreteThreeLevel(M0, delta, sigma, tgtTimeHorizonYrFrac, N float64)
	M0 := uint64(10000)                         // external price
	delta := 2.0                                // i.e. we want jumps of size 1.0
	sigma := 0.5                                // i.e. 50%
	tgtTimeHorizonYrFrac := 1.0 / 24.0 / 365.25 // i.e. one hour
	N := 1800                                   // we post twice a second

	priceLevels := make([]uint64, N)
	priceLevels[0] = M0

	for i := 0; i < len(priceLevels)-1; i++ {
		price, err := GeneratePriceUsingDiscreteThreeLevel(float64(priceLevels[i]), delta, sigma, tgtTimeHorizonYrFrac, float64(N))
		if err != nil {
			t.Error(err)
		}
		priceLevels[i+1] = price
		fmt.Printf("%v, %v\n", i, price)
	}
}
