package paper

import (
	"testing"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/market"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/orderbook"
)

func TestEngineOpensAndClosesConvergedSpread(t *testing.T) {
	engine := New(Config{Window: 3, Warmup: 3, EntryZ: 1, ExitZ: .8, StopZ: 5, TargetNotional: 100, MaximumPositions: 2, MaximumGrossUSDT: 1_000})
	start := time.Unix(10_000, 0)
	engine.OnTick(testTick(start, 100, 100))
	engine.OnTick(testTick(start.Add(time.Second), 100, 100))
	openEvents := engine.OnTick(testTick(start.Add(2*time.Second), 102, 100))
	if len(openEvents) != 1 || openEvents[0].Type != "OPEN" {
		t.Fatalf("expected open event, got %+v", openEvents)
	}
	closeEvents := engine.OnTick(testTick(start.Add(3*time.Second), 100, 100))
	if len(closeEvents) != 1 || closeEvents[0].Type != "CLOSE" {
		t.Fatalf("expected close event, got %+v", closeEvents)
	}
	if closeEvents[0].NetPnL <= 0 {
		t.Fatalf("expected profitable convergence, got %+v", closeEvents[0])
	}
	if snapshot := engine.Snapshot(); snapshot.Trades != 1 || snapshot.Wins != 1 || len(snapshot.Positions) != 0 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestEngineRequiresRoundTripCostsToBeCovered(t *testing.T) {
	engine := New(Config{Window: 3, Warmup: 3, EntryZ: 1, ExitZ: .5, StopZ: 5, TargetNotional: 100, BybitFeeBps: 10, BitgetFeeBps: 10, SafetyBpsPerLeg: 5, MaximumPositions: 1})
	start := time.Unix(10_000, 0)
	engine.OnTick(testTick(start, 100, 100))
	engine.OnTick(testTick(start.Add(time.Second), 100, 100))
	events := engine.OnTick(testTick(start.Add(2*time.Second), 100.3, 100))
	if len(events) != 0 {
		t.Fatalf("spread that cannot cover round trip costs must be rejected: %+v", events)
	}
}

func TestFundingPaysShortAndChargesLong(t *testing.T) {
	engine := New(Config{Window: 3, Warmup: 3})
	settlement := time.Unix(20_000, 0)
	position := Position{
		ID: "paper-1", Symbol: "ALTUSDT", Direction: LongBybitShortBitget, Quantity: 1,
		NextBybitFunding: settlement, NextBitgetFunding: settlement,
	}
	tick := testTick(settlement, 100, 100)
	tick.BybitTicker.FundingRate = .001
	tick.BitgetTicker.FundingRate = .002
	tick.BybitInstrument.FundingInterval = 8 * time.Hour
	tick.BitgetInstrument.FundingInterval = 8 * time.Hour

	updated, events := engine.applyFunding(position, tick, 0, 0)
	if len(events) != 2 {
		t.Fatalf("expected two funding events, got %+v", events)
	}
	// Long Bybit pays 0.1; short Bitget receives 0.2.
	if updated.FundingPnL < .099 || updated.FundingPnL > .101 {
		t.Fatalf("unexpected net funding: %v", updated.FundingPnL)
	}
}

func TestRestorePreservesOpenPositionsAndPerformance(t *testing.T) {
	engine := New(Config{MaximumPositions: 2})
	snapshot := Snapshot{RealizedPnL: 12.5, Trades: 3, Wins: 2, Losses: 1, PeakEquity: 15, MaxDrawdown: 2.5, TotalHoldSeconds: 90, Positions: []Position{{ID: "paper-1", Symbol: "ALTUSDT", Quantity: 1}}}
	if err := engine.Restore(snapshot); err != nil {
		t.Fatal(err)
	}
	restored := engine.Snapshot()
	if restored.RealizedPnL != 12.5 || restored.Trades != 3 || len(restored.Positions) != 1 || restored.TotalHoldSeconds != 90 {
		t.Fatalf("unexpected restored snapshot: %+v", restored)
	}
}

func testTick(now time.Time, bybitMid, bitgetMid float64) Tick {
	quantityStep := .001
	return Tick{
		Now: now,
		BybitBook: orderbook.Book{Exchange: "bybit", Symbol: "ALTUSDT", Bids: []orderbook.Level{{Price: bybitMid - .01, Size: 100}}, Asks: []orderbook.Level{{Price: bybitMid + .01, Size: 100}}},
		BitgetBook: orderbook.Book{Exchange: "bitget", Symbol: "ALTUSDT", Bids: []orderbook.Level{{Price: bitgetMid - .01, Size: 100}}, Asks: []orderbook.Level{{Price: bitgetMid + .01, Size: 100}}},
		BybitInstrument: market.Instrument{QuantityStep: quantityStep, MinimumQty: quantityStep, MaximumMarketQty: 1_000},
		BitgetInstrument: market.Instrument{QuantityStep: quantityStep, MinimumQty: quantityStep, MaximumMarketQty: 1_000},
	}
}
