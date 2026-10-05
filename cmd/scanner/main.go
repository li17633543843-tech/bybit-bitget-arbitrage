package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/exchange/bitget"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/exchange/bybit"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/ledger"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/market"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/scanner"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/tracker"
)

func main() {
	interval := flag.Duration("interval", 3*time.Second, "scan interval")
	minEdge := flag.Float64("min-edge-bps", 5, "minimum net edge after fees and safety buffer")
	minTurnover := flag.Float64("min-turnover", 1_000_000, "minimum 24h quote turnover on each exchange")
	minTopNotional := flag.Float64("min-top-notional", 100, "minimum executable USDT at best prices")
	bybitFee := flag.Float64("bybit-fee-bps", 5.5, "Bybit taker fee in bps; set to your actual tier")
	bitgetFee := flag.Float64("bitget-fee-bps", 6, "Bitget taker fee in bps; set to your actual tier")
	buffer := flag.Float64("buffer-bps", 3, "latency/slippage safety buffer")
	limit := flag.Int("limit", 20, "maximum opportunities printed per scan")
	minSamples := flag.Int("min-samples", 3, "consecutive profitable observations required")
	minDuration := flag.Duration("min-duration", 5*time.Second, "minimum time an opportunity must persist")
	ledgerPath := flag.String("ledger", "var/opportunities.jsonl", "confirmed opportunity ledger path")
	flag.Parse()

	httpClient := &http.Client{Timeout: 5 * time.Second}
	by := bybit.New(httpClient)
	bg := bitget.New(httpClient)
	cfg := scanner.Config{
		BybitTakerFeeBps:  *bybitFee,
		BitgetTakerFeeBps: *bitgetFee,
		SafetyBufferBps:   *buffer,
		MinNetEdgeBps:     *minEdge,
		MinTurnover24h:    *minTurnover,
		MinTopNotional:    *minTopNotional,
	}
	opportunityTracker := tracker.New(tracker.Config{
		MinSamples:  *minSamples,
		MinDuration: *minDuration,
		ExpireAfter: 3 * *interval,
	})
	opportunityLedger := ledger.NewJSONL(*ledgerPath)

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		type bybitResult struct {
			tickers map[string]market.Ticker
			err     error
		}
		type bitgetResult struct {
			tickers map[string]market.Ticker
			err     error
		}
		byCh := make(chan bybitResult, 1)
		bgCh := make(chan bitgetResult, 1)
		go func() {
			tickers, err := by.Tickers(ctx)
			byCh <- bybitResult{tickers: tickers, err: err}
		}()
		go func() {
			tickers, err := bg.Tickers(ctx)
			bgCh <- bitgetResult{tickers: tickers, err: err}
		}()
		byResult := <-byCh
		bgResult := <-bgCh
		cancel()
		if byResult.err != nil || bgResult.err != nil {
			log.Printf("scan failed: bybit=%v bitget=%v", byResult.err, bgResult.err)
		} else {
			byTickers := byResult.tickers
			bgTickers := bgResult.tickers
			opportunities := scanner.Find(byTickers, bgTickers, cfg)
			observations := opportunityTracker.Update(time.Now(), opportunities)
			if len(observations) > *limit {
				observations = observations[:*limit]
			}
			fmt.Printf("\n%s common=%d raw=%d confirmed=%d\n", time.Now().Format(time.RFC3339), commonCount(byTickers, bgTickers), len(opportunities), len(observations))
			fmt.Printf("%-14s %-16s %-12s %-12s %10s %10s %12s %10s %8s\n", "SYMBOL", "DIRECTION", "BUY", "SELL", "GROSS", "NET", "TOP USDT", "FUND Δ", "AGE")
			for _, observation := range observations {
				op := observation.Opportunity
				fmt.Printf("%-14s %-16s %-12.8g %-12.8g %9.2fbp %9.2fbp %12.2f %9.2fbp %8s\n",
					op.Symbol, op.BuyExchange+">"+op.SellExchange, op.BuyPrice, op.SellPrice,
					op.GrossEdgeBps, op.NetEdgeBps, op.TopNotional, op.FundingDeltaBps,
					observation.LastSeen.Sub(observation.FirstSeen).Round(time.Second))
				if err := opportunityLedger.Append(observation); err != nil {
					log.Printf("ledger write failed: %v", err)
				}
			}
		}
		time.Sleep(*interval)
	}
}

func commonCount[A any](left, right map[string]A) int {
	n := 0
	for symbol := range left {
		if _, ok := right[symbol]; ok {
			n++
		}
	}
	return n
}
