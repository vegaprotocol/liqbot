package types

type BalanceStore struct {
	balance cache[Balance]
}

func NewBalanceStore() *BalanceStore {
	return &BalanceStore{
		balance: newCache[Balance](),
	}
}

func (s *BalanceStore) Balance() Balance {
	return s.balance.get()
}

func (s *BalanceStore) BalanceSet(sets ...func(*Balance)) {
	s.balance.set(sets...)
}

type MarketStore struct {
	market cache[MarketData]
}

func NewMarketStore() *MarketStore {
	return &MarketStore{
		market: newCache[MarketData](),
	}
}

func (s *MarketStore) Market() MarketData {
	return s.market.get()
}

func (s *MarketStore) OpenVolume() int64 {
	return s.market.get().openVolume
}

func (s *MarketStore) MarketSet(sets ...func(*MarketData)) {
	s.market.set(sets...)
}

type cache[T any] struct {
	getCh chan chan T
	setCh chan []func(*T)
}

func (c cache[T]) get() T {
	r := make(chan T)
	c.getCh <- r
	return <-r
}

func (c cache[T]) set(f ...func(*T)) {
	c.setCh <- f
}

func newCache[T any]() cache[T] {
	var c cache[T]

	c.getCh = make(chan chan T)
	c.setCh = make(chan []func(*T))

	go func() {
		var d T
		for {
			select {
			case g := <-c.getCh:
				g <- d
			case s := <-c.setCh:
				for _, fn := range s {
					fn(&d)
				}
			}
		}
	}()

	return c
}
