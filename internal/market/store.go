package market

import "sync"

type TickerStore struct {
	mu      sync.RWMutex
	tickers map[string]Ticker
}

func NewTickerStore(initial map[string]Ticker) *TickerStore {
	store := &TickerStore{}
	store.Replace(initial)
	return store
}

func (s *TickerStore) Replace(tickers map[string]Ticker) {
	copyOfTickers := make(map[string]Ticker, len(tickers))
	for symbol, ticker := range tickers {
		copyOfTickers[symbol] = ticker
	}
	s.mu.Lock()
	s.tickers = copyOfTickers
	s.mu.Unlock()
}

func (s *TickerStore) Get(symbol string) (Ticker, bool) {
	s.mu.RLock()
	ticker, ok := s.tickers[symbol]
	s.mu.RUnlock()
	return ticker, ok
}
