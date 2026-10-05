package bybit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/exchange"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/market"
)

const tickersURL = "https://api.bybit.com/v5/market/tickers?category=linear"

type Client struct{ HTTP exchange.HTTPClient }

func New(httpClient *http.Client) *Client {
	return &Client{HTTP: exchange.HTTPClient{Client: httpClient}}
}

func (c *Client) Name() string { return "bybit" }

type tickerResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		List []struct {
			Symbol      string `json:"symbol"`
			Bid         string `json:"bid1Price"`
			BidSize     string `json:"bid1Size"`
			Ask         string `json:"ask1Price"`
			AskSize     string `json:"ask1Size"`
			Turnover24h string `json:"turnover24h"`
			FundingRate string `json:"fundingRate"`
			NextFundingTime string `json:"nextFundingTime"`
		} `json:"list"`
	} `json:"result"`
}

func (c *Client) Tickers(ctx context.Context) (map[string]market.Ticker, error) {
	var response tickerResponse
	if err := c.HTTP.GetJSON(ctx, tickersURL, &response); err != nil {
		return nil, err
	}
	if response.RetCode != 0 {
		return nil, fmt.Errorf("bybit: code=%d message=%s", response.RetCode, response.RetMsg)
	}

	now := time.Now()
	result := make(map[string]market.Ticker, len(response.Result.List))
	for _, item := range response.Result.List {
		t := market.Ticker{
			Exchange:    c.Name(),
			Symbol:      item.Symbol,
			Bid:         number(item.Bid),
			BidSize:     number(item.BidSize),
			Ask:         number(item.Ask),
			AskSize:     number(item.AskSize),
			Turnover24h: number(item.Turnover24h),
			FundingRate: number(item.FundingRate),
			NextFundingAt: millisecondsTime(item.NextFundingTime),
			ObservedAt:  now,
		}
		if t.Valid() {
			result[t.Symbol] = t
		}
	}
	return result, nil
}

func number(raw string) float64 {
	v, _ := strconv.ParseFloat(raw, 64)
	return v
}

func millisecondsTime(raw string) time.Time {
	milliseconds, _ := strconv.ParseInt(raw, 10, 64)
	if milliseconds <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(milliseconds)
}
