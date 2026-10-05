package bitget

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/exchange"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/market"
)

const tickersURL = "https://api.bitget.com/api/v2/mix/market/tickers?productType=USDT-FUTURES"
const currentFundingURL = "https://api.bitget.com/api/v2/mix/market/current-fund-rate?productType=USDT-FUTURES"

type Client struct{ HTTP exchange.HTTPClient }

func New(httpClient *http.Client) *Client {
	return &Client{HTTP: exchange.HTTPClient{Client: httpClient}}
}

func (c *Client) Name() string { return "bitget" }

type tickerResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		Symbol      string `json:"symbol"`
		Bid         string `json:"bidPr"`
		BidSize     string `json:"bidSz"`
		Ask         string `json:"askPr"`
		AskSize     string `json:"askSz"`
		Turnover24h string `json:"quoteVolume"`
		FundingRate string `json:"fundingRate"`
		NextFundingTime string `json:"nextFundingTime"`
	} `json:"data"`
}

type currentFundingResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		Symbol              string `json:"symbol"`
		FundingRate         string `json:"fundingRate"`
		FundingRateInterval string `json:"fundingRateInterval"`
		NextUpdate          string `json:"nextUpdate"`
	} `json:"data"`
}

func (c *Client) Tickers(ctx context.Context) (map[string]market.Ticker, error) {
	var response tickerResponse
	if err := c.HTTP.GetJSON(ctx, tickersURL, &response); err != nil {
		return nil, err
	}
	if response.Code != "00000" {
		return nil, fmt.Errorf("bitget: code=%s message=%s", response.Code, response.Msg)
	}
	var funding currentFundingResponse
	if err := c.HTTP.GetJSON(ctx, currentFundingURL, &funding); err != nil {
		return nil, fmt.Errorf("bitget current funding: %w", err)
	}
	if funding.Code != "00000" {
		return nil, fmt.Errorf("bitget current funding: code=%s message=%s", funding.Code, funding.Msg)
	}

	now := time.Now()
	result := make(map[string]market.Ticker, len(response.Data))
	for _, item := range response.Data {
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
	for _, item := range funding.Data {
		ticker, ok := result[item.Symbol]
		if !ok {
			continue
		}
		ticker.FundingRate = number(item.FundingRate)
		ticker.NextFundingAt = millisecondsTime(item.NextUpdate)
		result[item.Symbol] = ticker
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
