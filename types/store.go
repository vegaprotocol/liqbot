package types

type Store struct {
	market  cache[MarketData]
	balance cache[Balance]
}

func NewStore() *Store {
	return &Store{
		market:  newCache[MarketData](),
		balance: newCache[Balance](),
	}
}

func (s *Store) Balance() Balance {
	return s.balance.get()
}

func (s *Store) Market() MarketData {
	return s.market.get()
}

func (s *Store) OpenVolume() int64 {
	return s.market.get().openVolume
}

func (s *Store) BalanceSet(sets ...func(*Balance)) {
	s.balance.set(sets...)
}

func (s *Store) MarketSet(sets ...func(*MarketData)) {
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

func newCache[T any]() (c cache[T]) {
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

	return
}
