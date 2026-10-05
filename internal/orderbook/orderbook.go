package orderbook

import (
	"errors"
	"sort"
	"time"
)

var ErrInsufficientLiquidity = errors.New("insufficient order book liquidity")

type Level struct {
	Price float64
	Size  float64
}

type Book struct {
	Exchange  string
	Symbol    string
	Bids      []Level
	Asks      []Level
	Sequence  int64
	UpdatedAt time.Time
}

type FillEstimate struct {
	BaseQuantity  float64
	QuoteNotional float64
	AveragePrice  float64
	WorstPrice    float64
	LevelsUsed    int
}

func (b *Book) Normalize() {
	sort.Slice(b.Bids, func(i, j int) bool { return b.Bids[i].Price > b.Bids[j].Price })
	sort.Slice(b.Asks, func(i, j int) bool { return b.Asks[i].Price < b.Asks[j].Price })
}

func (b Book) Buy(baseQuantity float64) (FillEstimate, error) {
	return walk(b.Asks, baseQuantity)
}

func (b Book) Sell(baseQuantity float64) (FillEstimate, error) {
	return walk(b.Bids, baseQuantity)
}

func walk(levels []Level, baseQuantity float64) (FillEstimate, error) {
	if baseQuantity <= 0 {
		return FillEstimate{}, errors.New("base quantity must be positive")
	}
	remaining := baseQuantity
	result := FillEstimate{BaseQuantity: baseQuantity}
	for _, level := range levels {
		if level.Price <= 0 || level.Size <= 0 {
			continue
		}
		quantity := min(remaining, level.Size)
		result.QuoteNotional += quantity * level.Price
		result.WorstPrice = level.Price
		result.LevelsUsed++
		remaining -= quantity
		if remaining <= 1e-12 {
			result.AveragePrice = result.QuoteNotional / baseQuantity
			return result, nil
		}
	}
	return FillEstimate{}, ErrInsufficientLiquidity
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
