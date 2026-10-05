package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/exchange/bitget"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/exchange/bybit"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/ledger"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/market"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/orderbook"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/paper"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/strategy"
)

type candidate struct {
	symbol string
	score  float64
}

func main() {
	candidateLimit := flag.Int("candidates", 50, "number of common symbols to validate over WebSocket")
	targetNotional := flag.Float64("notional", 100, "target USDT notional per leg")
	minimumTurnover := flag.Float64("min-turnover", 1_000_000, "minimum 24h quote turnover on each exchange")
	minimumNetBps := flag.Float64("min-edge-bps", 5, "minimum depth-adjusted net edge")
	bybitFee := flag.Float64("bybit-fee-bps", 5.5, "actual Bybit taker fee")
	bitgetFee := flag.Float64("bitget-fee-bps", 6, "actual Bitget taker fee")
	safetyBps := flag.Float64("buffer-bps", 3, "latency and slippage allowance")
	maxAge := flag.Duration("max-book-age", 2*time.Second, "maximum accepted age of either book")
	paperEnabled := flag.Bool("paper", false, "enable stateful four-leg paper trading")
	window := flag.Int("window", 300, "rolling spread window")
	warmup := flag.Int("warmup", 300, "samples required before paper entries")
	entryZ := flag.Float64("entry-z", 2.5, "absolute z-score required to open")
	exitZ := flag.Float64("exit-z", .5, "absolute z-score used to close on convergence")
	stopZ := flag.Float64("stop-z", 4, "absolute z-score stop")
	maxHold := flag.Duration("max-hold", 2*time.Hour, "maximum paper position holding time")
	maxPositions := flag.Int("max-positions", 5, "maximum concurrent paper positions")
	maxGross := flag.Float64("max-gross-usdt", 2_000, "maximum total two-leg paper exposure")
	sampleInterval := flag.Duration("sample-interval", time.Second, "minimum interval between spread samples per symbol")
	paperLedgerPath := flag.String("paper-ledger", "var/paper-events.jsonl", "paper event ledger")
	paperSnapshotPath := flag.String("paper-snapshot", "var/paper-snapshot.json", "latest paper portfolio snapshot")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpClient := &http.Client{Timeout: 8 * time.Second}
	bybitREST := bybit.New(httpClient)
	bitgetREST := bitget.New(httpClient)
	byTickers, bgTickers, byInstruments, bgInstruments, err := loadMarket(ctx, bybitREST, bitgetREST)
	if err != nil {
		log.Fatal(err)
	}
	symbols := chooseCandidates(byTickers, bgTickers, byInstruments, bgInstruments, *minimumTurnover, *candidateLimit)
	if len(symbols) == 0 {
		log.Fatal("no common liquid symbols found")
	}
	log.Printf("subscribing to %d symbols: %v", len(symbols), symbols)
	byTickerStore := market.NewTickerStore(byTickers)
	bgTickerStore := market.NewTickerStore(bgTickers)
	go refreshTickers(ctx, bybitREST, bitgetREST, byTickerStore, bgTickerStore)

	var paperEngine *paper.Engine
	var paperLedger *ledger.JSONL
	if *paperEnabled {
		paperEngine = paper.New(paper.Config{
			Window: *window, Warmup: *warmup, EntryZ: *entryZ, ExitZ: *exitZ, StopZ: *stopZ,
			MaximumHold: *maxHold, TargetNotional: *targetNotional,
			BybitFeeBps: *bybitFee, BitgetFeeBps: *bitgetFee, SafetyBpsPerLeg: *safetyBps,
			MaximumPositions: *maxPositions, MaximumGrossUSDT: *maxGross, SampleInterval: *sampleInterval,
		})
		paperLedger = ledger.NewJSONL(*paperLedgerPath)
		var restored paper.Snapshot
		if err := ledger.ReadSnapshot(*paperSnapshotPath, &restored); err == nil {
			if err := paperEngine.Restore(restored); err != nil {
				log.Fatalf("restore paper snapshot: %v", err)
			}
			log.Printf("restored paper portfolio: positions=%d trades=%d equity=%.4f", len(restored.Positions), restored.Trades, restored.Equity)
		} else if !os.IsNotExist(err) {
			log.Fatalf("read paper snapshot: %v", err)
		}
		log.Printf("paper trading enabled; no private API or real orders are used")
	}

	updates := make(chan orderbook.Book, 1024)
	go reconnect(ctx, "bybit", func() error { return bybit.NewBookFeed().Run(ctx, symbols, updates) })
	go reconnect(ctx, "bitget", func() error { return bitget.NewBookFeed().Run(ctx, symbols, updates) })

	store := orderbook.NewStore()
	lastPrinted := make(map[string]time.Time)
	for {
		select {
		case <-ctx.Done():
			return
		case book := <-updates:
			store.Put(book)
			other := "bybit"
			if book.Exchange == "bybit" {
				other = "bitget"
			}
			otherBook, ok := store.Get(other, book.Symbol)
			if !ok {
				continue
			}
			now := time.Now()
			if now.Sub(book.UpdatedAt) > *maxAge || now.Sub(otherBook.UpdatedAt) > *maxAge {
				continue
			}
			evaluateBothDirections(now, store, book.Symbol, byInstruments[book.Symbol], bgInstruments[book.Symbol], *targetNotional, *minimumNetBps, *bybitFee, *bitgetFee, *safetyBps, lastPrinted)
			if paperEngine != nil {
				byBook, byOK := store.Get("bybit", book.Symbol)
				bgBook, bgOK := store.Get("bitget", book.Symbol)
				byTicker, byTickerOK := byTickerStore.Get(book.Symbol)
				bgTicker, bgTickerOK := bgTickerStore.Get(book.Symbol)
				if byOK && bgOK && byTickerOK && bgTickerOK {
					events := paperEngine.OnTick(paper.Tick{Now: now, BybitBook: byBook, BitgetBook: bgBook, BybitInstrument: byInstruments[book.Symbol], BitgetInstrument: bgInstruments[book.Symbol], BybitTicker: byTicker, BitgetTicker: bgTicker})
					for _, event := range events {
						log.Printf("PAPER %-7s %-14s direction=%s z=%.2f net=%.4f equity=%.4f reason=%s", event.Type, event.Position.Symbol, event.Position.Direction, event.CurrentZ, event.NetPnL, event.Equity, event.CloseReason)
						if err := paperLedger.Append(event); err != nil {
							log.Printf("paper ledger write failed: %v", err)
						}
					}
					if len(events) > 0 {
						if err := ledger.WriteSnapshot(*paperSnapshotPath, paperEngine.Snapshot()); err != nil {
							log.Printf("paper snapshot write failed: %v", err)
						}
					}
				}
			}
		}
	}
}

func refreshTickers(ctx context.Context, by *bybit.Client, bg *bitget.Client, byStore, bgStore *market.TickerStore) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			byTickers, bgTickers, err := loadTickersOnly(refreshCtx, by, bg)
			cancel()
			if err != nil {
				log.Printf("ticker refresh failed: %v", err)
				continue
			}
			byStore.Replace(byTickers)
			bgStore.Replace(bgTickers)
		}
	}
}

func loadTickersOnly(ctx context.Context, by *bybit.Client, bg *bitget.Client) (map[string]market.Ticker, map[string]market.Ticker, error) {
	type result struct {
		name string
		value map[string]market.Ticker
		err error
	}
	results := make(chan result, 2)
	go func() { value, err := by.Tickers(ctx); results <- result{name: "bybit", value: value, err: err} }()
	go func() { value, err := bg.Tickers(ctx); results <- result{name: "bitget", value: value, err: err} }()
	var byTickers, bgTickers map[string]market.Ticker
	for range 2 {
		item := <-results
		if item.err != nil {
			return nil, nil, fmt.Errorf("%s tickers: %w", item.name, item.err)
		}
		if item.name == "bybit" {
			byTickers = item.value
		} else {
			bgTickers = item.value
		}
	}
	return byTickers, bgTickers, nil
}

func loadMarket(ctx context.Context, by *bybit.Client, bg *bitget.Client) (map[string]market.Ticker, map[string]market.Ticker, map[string]market.Instrument, map[string]market.Instrument, error) {
	type result struct {
		name    string
		tickers map[string]market.Ticker
		instruments map[string]market.Instrument
		err     error
	}
	results := make(chan result, 4)
	go func() { tickers, err := by.Tickers(ctx); results <- result{name: "bybit", tickers: tickers, err: err} }()
	go func() { tickers, err := bg.Tickers(ctx); results <- result{name: "bitget", tickers: tickers, err: err} }()
	go func() { instruments, err := by.Instruments(ctx); results <- result{name: "bybit-instruments", instruments: instruments, err: err} }()
	go func() { instruments, err := bg.Instruments(ctx); results <- result{name: "bitget-instruments", instruments: instruments, err: err} }()
	var byTickers, bgTickers map[string]market.Ticker
	var byInstruments, bgInstruments map[string]market.Instrument
	for range 4 {
		value := <-results
		if value.err != nil {
			return nil, nil, nil, nil, fmt.Errorf("load %s: %w", value.name, value.err)
		}
		switch value.name {
		case "bybit":
			byTickers = value.tickers
		case "bitget":
			bgTickers = value.tickers
		case "bybit-instruments":
			byInstruments = value.instruments
		case "bitget-instruments":
			bgInstruments = value.instruments
		}
	}
	return byTickers, bgTickers, byInstruments, bgInstruments, nil
}

func chooseCandidates(bybitTickers, bitgetTickers map[string]market.Ticker, bybitInstruments, bitgetInstruments map[string]market.Instrument, minTurnover float64, limit int) []string {
	candidates := make([]candidate, 0)
	for symbol, by := range bybitTickers {
		bg, ok := bitgetTickers[symbol]
		if !ok || by.Turnover24h < minTurnover || bg.Turnover24h < minTurnover {
			continue
		}
		byInstrument, byOK := bybitInstruments[symbol]
		bgInstrument, bgOK := bitgetInstruments[symbol]
		if !byOK || !bgOK || byInstrument.BaseCoin != bgInstrument.BaseCoin || !byInstrument.TradableUSDTPerpetual() || !bgInstrument.TradableUSDTPerpetual() {
			continue
		}
		byToBG := (bg.Bid/by.Ask - 1) * 10_000
		bgToBY := (by.Bid/bg.Ask - 1) * 10_000
		score := byToBG
		if bgToBY > score {
			score = bgToBY
		}
		candidates = append(candidates, candidate{symbol: symbol, score: score})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	symbols := make([]string, 0, len(candidates))
	for _, item := range candidates {
		symbols = append(symbols, item.symbol)
	}
	return symbols
}

func evaluateBothDirections(now time.Time, store *orderbook.Store, symbol string, byInstrument, bgInstrument market.Instrument, notional, minNet, byFee, bgFee, safety float64, lastPrinted map[string]time.Time) {
	byBook, byOK := store.Get("bybit", symbol)
	bgBook, bgOK := store.Get("bitget", symbol)
	if !byOK || !bgOK || len(byBook.Asks) == 0 || len(bgBook.Asks) == 0 {
		return
	}
	byQuantity, byValid := market.CommonQuantity(notional, byBook.Asks[0].Price, byInstrument, bgInstrument)
	if byValid {
		evaluateDirection(now, byBook, bgBook, byQuantity, strategy.DepthConfig{BuyFeeBps: byFee, SellFeeBps: bgFee, SafetyBps: safety, MinimumNetBps: minNet}, lastPrinted)
	}
	bgQuantity, bgValid := market.CommonQuantity(notional, bgBook.Asks[0].Price, byInstrument, bgInstrument)
	if bgValid {
		evaluateDirection(now, bgBook, byBook, bgQuantity, strategy.DepthConfig{BuyFeeBps: bgFee, SellFeeBps: byFee, SafetyBps: safety, MinimumNetBps: minNet}, lastPrinted)
	}
}

func evaluateDirection(now time.Time, buyBook, sellBook orderbook.Book, quantity float64, config strategy.DepthConfig, lastPrinted map[string]time.Time) {
	opportunity, err := strategy.EvaluateDepth(buyBook, sellBook, quantity, config)
	if err != nil {
		return
	}
	key := opportunity.Symbol + ":" + opportunity.BuyExchange + ":" + opportunity.SellExchange
	if now.Sub(lastPrinted[key]) < time.Second {
		return
	}
	lastPrinted[key] = now
	fmt.Printf("%s %-14s %-16s qty=%-12.8g buy=%-12.8g sell=%-12.8g net=%7.2fbp pnl=%9.4f USDT levels=%d/%d\n",
		now.Format(time.RFC3339Nano), opportunity.Symbol, opportunity.BuyExchange+">"+opportunity.SellExchange,
		opportunity.Quantity, opportunity.BuyAverage, opportunity.SellAverage, opportunity.NetEdgeBps,
		opportunity.ExpectedPnL, opportunity.BuyLevelsUsed, opportunity.SellLevelsUsed)
}

func reconnect(ctx context.Context, name string, run func() error) {
	for ctx.Err() == nil {
		if err := run(); err != nil && ctx.Err() == nil {
			log.Printf("%s websocket disconnected: %v", name, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}
