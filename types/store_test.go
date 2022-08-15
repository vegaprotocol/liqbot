package types

import (
	"testing"

	vt "code.vegaprotocol.io/vega/types"

	"code.vegaprotocol.io/liqbot/types/num"
)

func Test_newBalanceCache(t *testing.T) {
	c := newCache[Balance]()

	general := num.NewUint(12)
	margin := num.NewUint(44)
	bond := num.NewUint(66)

	c.set(
		SetGeneral(general),
		SetMargin(margin),
		SetBond(bond),
	)

	if !c.get().General().EQ(general) {
		t.Errorf("expected %v, got %v", general, c.get().General())
	}
	if !c.get().Margin().EQ(margin) {
		t.Errorf("expected %v, got %v", margin, c.get().Margin())
	}
	if !c.get().Bond().EQ(bond) {
		t.Errorf("expected %v, got %v", bond, c.get().Bond())
	}
	if !c.get().Total().EQ(num.Sum(general, margin, bond)) {
		t.Errorf("expected %v, got %v", num.Sum(general, margin, bond), c.get().Total())
	}
}

func Test_newMarketCache(t *testing.T) {
	m := newCache[MarketData]()

	m.set(
		SetStaticMidPrice(num.NewUint(12)),
		SetMarkPrice(num.NewUint(44)),
		SetTargetStake(num.NewUint(66)),
		SetSuppliedStake(num.NewUint(88)),
		SetTradingMode(vt.MarketTradingModeContinuous),
		SetOpenVolume(12),
	)

	if !m.get().StaticMidPrice().EQ(num.NewUint(12)) {
		t.Errorf("expected %v, got %v", num.NewUint(12), m.get().StaticMidPrice())
	}

	if !m.get().MarkPrice().EQ(num.NewUint(44)) {
		t.Errorf("expected %v, got %v", num.NewUint(44), m.get().MarkPrice())
	}

	if !m.get().TargetStake().EQ(num.NewUint(66)) {
		t.Errorf("expected %v, got %v", num.NewUint(66), m.get().TargetStake())
	}

	if !m.get().SuppliedStake().EQ(num.NewUint(88)) {
		t.Errorf("expected %v, got %v", num.NewUint(88), m.get().SuppliedStake())
	}

	if m.get().TradingMode() != vt.MarketTradingModeContinuous {
		t.Errorf("expected %v, got %v", vt.MarketTradingModeContinuous, m.get().TradingMode())
	}

	if m.get().OpenVolume() != 12 {
		t.Errorf("expected %v, got %v", 12, m.get().OpenVolume())
	}
}
