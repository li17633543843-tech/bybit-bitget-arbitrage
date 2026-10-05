package strategy

import (
	"fmt"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/orderbook"
)

type DepthConfig struct {
	BuyFeeBps      float64
	SellFeeBps     float64
	SafetyBps      float64
	MinimumNetBps  float64
}

type DepthOpportunity struct {
	Symbol          string
	BuyExchange     string
	SellExchange    string
	Quantity        float64
	BuyAverage      float64
	BuyWorst        float64
	SellAverage     float64
	SellWorst       float64
	GrossEdgeBps    float64
	NetEdgeBps      float64
	ExpectedPnL     float64
	BuyLevelsUsed   int
	SellLevelsUsed  int
}

func EvaluateDepth(buyBook, sellBook orderbook.Book, quantity float64, config DepthConfig) (DepthOpportunity, error) {
	if buyBook.Symbol == "" || buyBook.Symbol != sellBook.Symbol {
		return DepthOpportunity{}, fmt.Errorf("order books must have the same non-empty symbol")
	}
	buy, err := buyBook.Buy(quantity)
	if err != nil {
		return DepthOpportunity{}, fmt.Errorf("buy book: %w", err)
	}
	sell, err := sellBook.Sell(quantity)
	if err != nil {
		return DepthOpportunity{}, fmt.Errorf("sell book: %w", err)
	}
	grossBps := (sell.AveragePrice/buy.AveragePrice - 1) * 10_000
	netBps := grossBps - config.BuyFeeBps - config.SellFeeBps - config.SafetyBps
	if netBps < config.MinimumNetBps {
		return DepthOpportunity{}, fmt.Errorf("net edge %.4f bps is below minimum %.4f bps", netBps, config.MinimumNetBps)
	}
	buyFee := buy.QuoteNotional * config.BuyFeeBps / 10_000
	sellFee := sell.QuoteNotional * config.SellFeeBps / 10_000
	safetyCost := buy.QuoteNotional * config.SafetyBps / 10_000
	return DepthOpportunity{
		Symbol:         buyBook.Symbol,
		BuyExchange:    buyBook.Exchange,
		SellExchange:   sellBook.Exchange,
		Quantity:       quantity,
		BuyAverage:     buy.AveragePrice,
		BuyWorst:       buy.WorstPrice,
		SellAverage:    sell.AveragePrice,
		SellWorst:      sell.WorstPrice,
		GrossEdgeBps:   grossBps,
		NetEdgeBps:     netBps,
		ExpectedPnL:    sell.QuoteNotional - buy.QuoteNotional - buyFee - sellFee - safetyCost,
		BuyLevelsUsed:  buy.LevelsUsed,
		SellLevelsUsed: sell.LevelsUsed,
	}, nil
}
