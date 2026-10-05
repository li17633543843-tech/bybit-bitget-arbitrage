package market

import (
	"context"
	"math"
	"time"
)

type Ticker struct {
	Exchange      string
	Symbol        string
	Bid           float64
	BidSize       float64
	Ask           float64
	AskSize       float64
	Turnover24h   float64
	FundingRate   float64
	NextFundingAt time.Time
	ObservedAt    time.Time
}

func (t Ticker) Valid() bool {
	return t.Symbol != "" && t.Bid > 0 && t.Ask > 0 && t.Ask >= t.Bid
}

type Source interface {
	Name() string
	Tickers(ctx context.Context) (map[string]Ticker, error)
}

type Instrument struct {
	Exchange       string
	Symbol         string
	BaseCoin       string
	QuoteCoin      string
	SettleCoin     string
	Status         string
	ContractType   string
	TickSize       float64
	QuantityStep   float64
	MinimumQty     float64
	MinimumNotional float64
	MaximumMarketQty float64
	FundingInterval time.Duration
}

func (i Instrument) TradableUSDTPerpetual() bool {
	return i.Symbol != "" && i.QuoteCoin == "USDT" && i.SettleCoin == "USDT" &&
		i.Status == "Trading" && i.ContractType == "Perpetual" && i.QuantityStep > 0
}

func CommonQuantity(targetNotional, referencePrice float64, instruments ...Instrument) (float64, bool) {
	if targetNotional <= 0 || referencePrice <= 0 || len(instruments) == 0 {
		return 0, false
	}
	quantity := targetNotional / referencePrice
	step := 0.0
	minimum := 0.0
	maximum := math.Inf(1)
	for _, instrument := range instruments {
		if instrument.QuantityStep <= 0 {
			return 0, false
		}
		if instrument.QuantityStep > step {
			step = instrument.QuantityStep
		}
		if instrument.MinimumQty > minimum {
			minimum = instrument.MinimumQty
		}
		if instrument.MinimumNotional > 0 {
			minimumByNotional := instrument.MinimumNotional / referencePrice
			if minimumByNotional > minimum {
				minimum = minimumByNotional
			}
		}
		if instrument.MaximumMarketQty > 0 && instrument.MaximumMarketQty < maximum {
			maximum = instrument.MaximumMarketQty
		}
	}
	if quantity < minimum {
		quantity = minimum
	}
	quantity = math.Floor((quantity+step*1e-9)/step) * step
	if quantity < minimum {
		quantity = math.Ceil((minimum-step*1e-9)/step) * step
	}
	if quantity <= 0 || quantity > maximum {
		return 0, false
	}
	return quantity, true
}
