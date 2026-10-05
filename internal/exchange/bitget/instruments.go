package bitget

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/market"
)

const instrumentsURL = "https://api.bitget.com/api/v2/mix/market/contracts?productType=USDT-FUTURES"

type instrumentResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		Symbol           string `json:"symbol"`
		BaseCoin         string `json:"baseCoin"`
		QuoteCoin        string `json:"quoteCoin"`
		SymbolType       string `json:"symbolType"`
		Status           string `json:"symbolStatus"`
		MinimumQty       string `json:"minTradeNum"`
		QuantityStep     string `json:"sizeMultiplier"`
		MinimumNotional  string `json:"minTradeUSDT"`
		MaximumMarketQty string `json:"maxMarketOrderQty"`
		PricePlaces      string `json:"pricePlace"`
		PriceEndStep     string `json:"priceEndStep"`
		FundingInterval  string `json:"fundInterval"`
	} `json:"data"`
}

func (c *Client) Instruments(ctx context.Context) (map[string]market.Instrument, error) {
	var response instrumentResponse
	if err := c.HTTP.GetJSON(ctx, instrumentsURL, &response); err != nil {
		return nil, err
	}
	if response.Code != "00000" {
		return nil, fmt.Errorf("bitget instruments: code=%s message=%s", response.Code, response.Msg)
	}
	result := make(map[string]market.Instrument, len(response.Data))
	for _, item := range response.Data {
		status := ""
		if item.Status == "normal" {
			status = "Trading"
		}
		contractType := ""
		if item.SymbolType == "perpetual" {
			contractType = "Perpetual"
		}
		fundingHours, _ := strconv.Atoi(item.FundingInterval)
		instrument := market.Instrument{
			Exchange:         c.Name(),
			Symbol:           item.Symbol,
			BaseCoin:         item.BaseCoin,
			QuoteCoin:        item.QuoteCoin,
			SettleCoin:       "USDT",
			Status:           status,
			ContractType:     contractType,
			TickSize:         bitgetTickSize(item.PricePlaces, item.PriceEndStep),
			QuantityStep:     number(item.QuantityStep),
			MinimumQty:       number(item.MinimumQty),
			MinimumNotional:  number(item.MinimumNotional),
			MaximumMarketQty: number(item.MaximumMarketQty),
			FundingInterval:  time.Duration(fundingHours) * time.Hour,
		}
		if instrument.TradableUSDTPerpetual() {
			result[instrument.Symbol] = instrument
		}
	}
	return result, nil
}

func bitgetTickSize(pricePlaces, priceEndStep string) float64 {
	places, err := strconv.Atoi(pricePlaces)
	if err != nil || places < 0 {
		return 0
	}
	step := number(priceEndStep)
	for range places {
		step /= 10
	}
	return step
}
