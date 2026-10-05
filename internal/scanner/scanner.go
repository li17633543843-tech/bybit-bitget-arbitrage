package scanner

import (
	"sort"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/market"
)

type Config struct {
	BybitTakerFeeBps  float64
	BitgetTakerFeeBps float64
	SafetyBufferBps   float64
	MinNetEdgeBps     float64
	MinTurnover24h    float64
	MinTopNotional    float64
}

type Opportunity struct {
	Symbol            string
	BuyExchange       string
	SellExchange      string
	BuyPrice          float64
	SellPrice         float64
	GrossEdgeBps      float64
	NetEdgeBps        float64
	TopNotional       float64
	MinimumTurnover24 float64
	FundingDeltaBps   float64
}

func Find(bybitTickers, bitgetTickers map[string]market.Ticker, cfg Config) []Opportunity {
	result := make([]Opportunity, 0)
	for symbol, by := range bybitTickers {
		bg, ok := bitgetTickers[symbol]
		if !ok || !by.Valid() || !bg.Valid() {
			continue
		}
		turnover := min(by.Turnover24h, bg.Turnover24h)
		if turnover < cfg.MinTurnover24h {
			continue
		}

		result = appendIfProfitable(result, symbol, by, bg, cfg, turnover)
		result = appendIfProfitable(result, symbol, bg, by, cfg, turnover)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].NetEdgeBps > result[j].NetEdgeBps })
	return result
}

func appendIfProfitable(dst []Opportunity, symbol string, buy, sell market.Ticker, cfg Config, turnover float64) []Opportunity {
	gross := (sell.Bid/buy.Ask - 1) * 10_000
	fees := cfg.BybitTakerFeeBps + cfg.BitgetTakerFeeBps
	net := gross - fees - cfg.SafetyBufferBps
	topNotional := min(buy.Ask*buy.AskSize, sell.Bid*sell.BidSize)
	if net < cfg.MinNetEdgeBps || topNotional < cfg.MinTopNotional {
		return dst
	}
	return append(dst, Opportunity{
		Symbol:            symbol,
		BuyExchange:       buy.Exchange,
		SellExchange:      sell.Exchange,
		BuyPrice:          buy.Ask,
		SellPrice:         sell.Bid,
		GrossEdgeBps:      gross,
		NetEdgeBps:        net,
		TopNotional:       topNotional,
		MinimumTurnover24: turnover,
		FundingDeltaBps:   (sell.FundingRate - buy.FundingRate) * 10_000,
	})
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
