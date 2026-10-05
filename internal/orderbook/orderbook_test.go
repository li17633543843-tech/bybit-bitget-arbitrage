package orderbook

import (
	"errors"
	"testing"
)

func TestBuyWalksMultipleAskLevels(t *testing.T) {
	book := Book{Asks: []Level{{Price: 100, Size: 1}, {Price: 102, Size: 2}}}
	fill, err := book.Buy(2)
	if err != nil {
		t.Fatal(err)
	}
	if fill.QuoteNotional != 202 || fill.AveragePrice != 101 || fill.WorstPrice != 102 || fill.LevelsUsed != 2 {
		t.Fatalf("unexpected fill: %+v", fill)
	}
}

func TestSellUsesHighestBidsFirst(t *testing.T) {
	book := Book{Bids: []Level{{Price: 99, Size: 2}, {Price: 100, Size: 1}}}
	book.Normalize()
	fill, err := book.Sell(2)
	if err != nil {
		t.Fatal(err)
	}
	if fill.QuoteNotional != 199 || fill.AveragePrice != 99.5 {
		t.Fatalf("unexpected fill: %+v", fill)
	}
}

func TestWalkRejectsInsufficientLiquidity(t *testing.T) {
	book := Book{Asks: []Level{{Price: 100, Size: 0.5}}}
	_, err := book.Buy(1)
	if !errors.Is(err, ErrInsufficientLiquidity) {
		t.Fatalf("expected insufficient liquidity, got %v", err)
	}
}
