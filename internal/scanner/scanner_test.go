package scanner

import (
	"testing"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/market"
)

func TestFindRanksNetExecutableOpportunities(t *testing.T) {
	bybit := map[string]market.Ticker{
		"ALTUSDT": {Exchange: "bybit", Symbol: "ALTUSDT", Bid: 99, BidSize: 20, Ask: 100, AskSize: 10, Turnover24h: 2_000_000},
	}
	bitget := map[string]market.Ticker{
		"ALTUSDT": {Exchange: "bitget", Symbol: "ALTUSDT", Bid: 101, BidSize: 5, Ask: 102, AskSize: 20, Turnover24h: 3_000_000},
	}
	cfg := Config{BybitTakerFeeBps: 5.5, BitgetTakerFeeBps: 6, SafetyBufferBps: 2, MinNetEdgeBps: 20, MinTopNotional: 100}

	got := Find(bybit, bitget, cfg)
	if len(got) != 1 {
		t.Fatalf("expected one opportunity, got %d", len(got))
	}
	if got[0].BuyExchange != "bybit" || got[0].SellExchange != "bitget" {
		t.Fatalf("unexpected direction: %+v", got[0])
	}
	if got[0].TopNotional != 505 {
		t.Fatalf("expected top notional 505, got %.2f", got[0].TopNotional)
	}
}

func TestFindRejectsIlliquidTopOfBook(t *testing.T) {
	bybit := map[string]market.Ticker{"ALTUSDT": {Exchange: "bybit", Symbol: "ALTUSDT", Bid: 99, BidSize: 1, Ask: 100, AskSize: .1, Turnover24h: 2_000_000}}
	bitget := map[string]market.Ticker{"ALTUSDT": {Exchange: "bitget", Symbol: "ALTUSDT", Bid: 110, BidSize: .1, Ask: 111, AskSize: 1, Turnover24h: 2_000_000}}

	got := Find(bybit, bitget, Config{MinNetEdgeBps: 1, MinTopNotional: 100})
	if len(got) != 0 {
		t.Fatalf("expected no opportunity, got %d", len(got))
	}
}
