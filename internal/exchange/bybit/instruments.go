package bybit

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/market"
)

const instrumentsURL = "https://api.bybit.com/v5/market/instruments-info"

type instrumentResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		NextCursor string `json:"nextPageCursor"`
		List       []struct {
			Symbol       string `json:"symbol"`
			ContractType string `json:"contractType"`
			Status       string `json:"status"`
			BaseCoin     string `json:"baseCoin"`
			QuoteCoin    string `json:"quoteCoin"`
			SettleCoin   string `json:"settleCoin"`
			FundingInterval int `json:"fundingInterval"`
			PriceFilter struct {
				TickSize string `json:"tickSize"`
			} `json:"priceFilter"`
			LotSizeFilter struct {
				MinimumQty       string `json:"minOrderQty"`
				QuantityStep     string `json:"qtyStep"`
				MinimumNotional  string `json:"minNotionalValue"`
				MaximumMarketQty string `json:"maxMarketOrderQty"`
			} `json:"lotSizeFilter"`
		} `json:"list"`
	} `json:"result"`
}

func (c *Client) Instruments(ctx context.Context) (map[string]market.Instrument, error) {
	result := make(map[string]market.Instrument)
	cursor := ""
	for {
		endpoint := instrumentsURL + "?category=linear&status=Trading&limit=1000"
		if cursor != "" {
			endpoint += "&cursor=" + url.QueryEscape(cursor)
		}
		var response instrumentResponse
		if err := c.HTTP.GetJSON(ctx, endpoint, &response); err != nil {
			return nil, err
		}
		if response.RetCode != 0 {
			return nil, fmt.Errorf("bybit instruments: code=%d message=%s", response.RetCode, response.RetMsg)
		}
		for _, item := range response.Result.List {
			contractType := ""
			if item.ContractType == "LinearPerpetual" {
				contractType = "Perpetual"
			}
			instrument := market.Instrument{
				Exchange:         c.Name(),
				Symbol:           item.Symbol,
				BaseCoin:         item.BaseCoin,
				QuoteCoin:        item.QuoteCoin,
				SettleCoin:       item.SettleCoin,
				Status:           item.Status,
				ContractType:     contractType,
				TickSize:         number(item.PriceFilter.TickSize),
				QuantityStep:     number(item.LotSizeFilter.QuantityStep),
				MinimumQty:       number(item.LotSizeFilter.MinimumQty),
				MinimumNotional:  number(item.LotSizeFilter.MinimumNotional),
				MaximumMarketQty: number(item.LotSizeFilter.MaximumMarketQty),
				FundingInterval:  time.Duration(item.FundingInterval) * time.Minute,
			}
			if instrument.TradableUSDTPerpetual() {
				result[instrument.Symbol] = instrument
			}
		}
		if response.Result.NextCursor == "" || response.Result.NextCursor == cursor {
			break
		}
		cursor = response.Result.NextCursor
	}
	return result, nil
}
