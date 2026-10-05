package strategy

import (
	"testing"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/orderbook"
)

func TestEvaluateDepthUsesVWAPAndCosts(t *testing.T) {
	buy := orderbook.Book{
		Exchange: "bybit",
		Symbol:   "ALTUSDT",
		Asks:     []orderbook.Level{{Price: 100, Size: 1}, {Price: 101, Size: 1}},
	}
	sell := orderbook.Book{
		Exchange: "bitget",
		Symbol:   "ALTUSDT",
		Bids:     []orderbook.Level{{Price: 103, Size: 1}, {Price: 102, Size: 1}},
	}

	opportunity, err := EvaluateDepth(buy, sell, 2, DepthConfig{BuyFeeBps: 5, SellFeeBps: 5, SafetyBps: 2, MinimumNetBps: 1})
	if err != nil {
		t.Fatal(err)
	}
	if opportunity.BuyAverage != 100.5 || opportunity.SellAverage != 102.5 {
		t.Fatalf("unexpected VWAP: %+v", opportunity)
	}
	if opportunity.ExpectedPnL <= 0 || opportunity.BuyLevelsUsed != 2 || opportunity.SellLevelsUsed != 2 {
		t.Fatalf("unexpected opportunity: %+v", opportunity)
	}
}

func TestEvaluateDepthRejectsTopLevelMirage(t *testing.T) {
	buy := orderbook.Book{Exchange: "bybit", Symbol: "ALTUSDT", Asks: []orderbook.Level{{Price: 100, Size: .01}, {Price: 110, Size: 10}}}
	sell := orderbook.Book{Exchange: "bitget", Symbol: "ALTUSDT", Bids: []orderbook.Level{{Price: 105, Size: .01}, {Price: 99, Size: 10}}}

	if _, err := EvaluateDepth(buy, sell, 1, DepthConfig{MinimumNetBps: 1}); err == nil {
		t.Fatal("expected apparent top-level spread to disappear at executable depth")
	}
}
