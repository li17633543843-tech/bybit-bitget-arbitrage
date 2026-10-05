package paper

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/market"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/orderbook"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/statistics"
)

type Direction string

const (
	LongBybitShortBitget Direction = "LONG_BYBIT_SHORT_BITGET"
	LongBitgetShortBybit Direction = "LONG_BITGET_SHORT_BYBIT"
)

type CloseReason string

const (
	CloseConvergence CloseReason = "CONVERGENCE"
	CloseStop        CloseReason = "Z_SCORE_STOP"
	CloseTimeout     CloseReason = "MAX_HOLD_TIME"
)

type Config struct {
	Window             int
	Warmup             int
	EntryZ             float64
	ExitZ              float64
	StopZ              float64
	MaximumHold        time.Duration
	TargetNotional     float64
	BybitFeeBps        float64
	BitgetFeeBps       float64
	SafetyBpsPerLeg    float64
	MaximumPositions   int
	MaximumGrossUSDT   float64
	SampleInterval     time.Duration
	MinimumHold        time.Duration
	EntryConfirm       time.Duration
	MaximumEntryZ      float64
	MaximumBookSkew    time.Duration
}

type Tick struct {
	Now              time.Time
	BybitBook        orderbook.Book
	BitgetBook       orderbook.Book
	BybitInstrument  market.Instrument
	BitgetInstrument market.Instrument
	BybitTicker      market.Ticker
	BitgetTicker     market.Ticker
}

type Position struct {
	ID                 string        `json:"id"`
	Symbol             string        `json:"symbol"`
	Direction          Direction     `json:"direction"`
	Quantity           float64       `json:"quantity"`
	OpenedAt           time.Time     `json:"opened_at"`
	EntryZ             float64       `json:"entry_z"`
	EntrySpreadBps     float64       `json:"entry_spread_bps"`
	BybitEntryPrice    float64       `json:"bybit_entry_price"`
	BitgetEntryPrice   float64       `json:"bitget_entry_price"`
	BybitEntryNotional float64       `json:"bybit_entry_notional"`
	BitgetEntryNotional float64      `json:"bitget_entry_notional"`
	EntryFees          float64       `json:"entry_fees"`
	EntrySafetyCost    float64       `json:"entry_safety_cost"`
	FundingPnL         float64       `json:"funding_pnl"`
	NextBybitFunding   time.Time     `json:"next_bybit_funding"`
	NextBitgetFunding  time.Time     `json:"next_bitget_funding"`
}

type Event struct {
	Type          string       `json:"type"`
	Time          time.Time    `json:"time"`
	Position      Position     `json:"position"`
	CloseReason   CloseReason  `json:"close_reason,omitempty"`
	CurrentZ      float64      `json:"current_z"`
	SpreadBps     float64      `json:"spread_bps"`
	GrossPnL      float64      `json:"gross_pnl,omitempty"`
	Fees          float64      `json:"fees,omitempty"`
	SafetyCost    float64      `json:"safety_cost,omitempty"`
	FundingPnL    float64      `json:"funding_pnl,omitempty"`
	NetPnL        float64      `json:"net_pnl,omitempty"`
	Equity        float64      `json:"equity"`
}

type Snapshot struct {
	Equity       float64    `json:"equity"`
	RealizedPnL  float64    `json:"realized_pnl"`
	Trades       int        `json:"trades"`
	Wins         int        `json:"wins"`
	Losses       int        `json:"losses"`
	PeakEquity   float64    `json:"peak_equity"`
	MaxDrawdown  float64    `json:"max_drawdown"`
	TotalHoldSeconds float64 `json:"total_hold_seconds"`
	AverageHoldSeconds float64 `json:"average_hold_seconds"`
	Positions    []Position `json:"positions"`
}

type Engine struct {
	mu          sync.RWMutex
	config      Config
	statistics map[string]*statistics.Rolling
	positions  map[string]Position
	realized   float64
	trades     int
	wins       int
	losses     int
	peakEquity float64
	maxDrawdown float64
	totalHold time.Duration
	nextID     uint64
	lastSample map[string]time.Time
	entrySignals map[string]entrySignal
}

type entrySignal struct {
	direction int
	since     time.Time
}

func New(config Config) *Engine {
	if config.Window < 2 {
		config.Window = 300
	}
	if config.Warmup < 2 || config.Warmup > config.Window {
		config.Warmup = config.Window
	}
	if config.MaximumPositions < 1 {
		config.MaximumPositions = 1
	}
	return &Engine{config: config, statistics: make(map[string]*statistics.Rolling), positions: make(map[string]Position), lastSample: make(map[string]time.Time), entrySignals: make(map[string]entrySignal)}
}

func (e *Engine) OnTick(tick Tick) []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !validTick(tick) {
		return nil
	}
	if e.config.MaximumBookSkew > 0 && absDuration(tick.BybitBook.UpdatedAt.Sub(tick.BitgetBook.UpdatedAt)) > e.config.MaximumBookSkew {
		delete(e.entrySignals, tick.BybitBook.Symbol)
		return nil
	}
	symbol := tick.BybitBook.Symbol
	if previous := e.lastSample[symbol]; e.config.SampleInterval > 0 && tick.Now.Sub(previous) < e.config.SampleInterval {
		return nil
	}
	e.lastSample[symbol] = tick.Now
	spreadBps := canonicalSpreadBps(tick.BybitBook, tick.BitgetBook)
	rolling := e.statistics[symbol]
	if rolling == nil {
		rolling = statistics.NewRolling(e.config.Window)
		e.statistics[symbol] = rolling
	}
	stats := rolling.Add(spreadBps)
	events := make([]Event, 0, 3)

	if position, exists := e.positions[symbol]; exists {
		position, fundingEvents := e.applyFunding(position, tick, stats.ZScore, spreadBps)
		e.positions[symbol] = position
		events = append(events, fundingEvents...)
		if !rolling.Ready(e.config.Warmup) {
			return events
		}
		reason, closeNow := e.closeDecision(position, stats.ZScore, tick.Now)
		if closeNow {
			if event, ok := e.closePosition(position, tick, stats.ZScore, spreadBps, reason); ok {
				delete(e.positions, symbol)
				events = append(events, event)
			}
		}
		return events
	}

	if !rolling.Ready(e.config.Warmup) || math.Abs(stats.ZScore) < e.config.EntryZ || (e.config.MaximumEntryZ > 0 && math.Abs(stats.ZScore) > e.config.MaximumEntryZ) || len(e.positions) >= e.config.MaximumPositions {
		delete(e.entrySignals, symbol)
		return events
	}
	if math.Abs(spreadBps-stats.Mean) < e.roundTripCostBps() {
		delete(e.entrySignals, symbol)
		return events
	}
	direction := 1
	if stats.ZScore < 0 {
		direction = -1
	}
	signal, exists := e.entrySignals[symbol]
	if !exists || signal.direction != direction {
		e.entrySignals[symbol] = entrySignal{direction: direction, since: tick.Now}
		if e.config.EntryConfirm > 0 {
			return events
		}
	} else if tick.Now.Sub(signal.since) < e.config.EntryConfirm {
		return events
	}
	if position, ok := e.openPosition(tick, stats.ZScore, spreadBps); ok {
		e.positions[symbol] = position
		delete(e.entrySignals, symbol)
		events = append(events, Event{Type: "OPEN", Time: tick.Now, Position: position, CurrentZ: stats.ZScore, SpreadBps: spreadBps, Fees: position.EntryFees, SafetyCost: position.EntrySafetyCost, Equity: e.realized})
	}
	return events
}

func (e *Engine) openPosition(tick Tick, zScore, spreadBps float64) (Position, bool) {
	referencePrice := bestMid(tick.BybitBook)
	quantity, ok := market.CommonQuantity(e.config.TargetNotional, referencePrice, tick.BybitInstrument, tick.BitgetInstrument)
	if !ok {
		return Position{}, false
	}
	position := Position{Symbol: tick.BybitBook.Symbol, Quantity: quantity, OpenedAt: tick.Now, EntryZ: zScore, EntrySpreadBps: spreadBps, NextBybitFunding: tick.BybitTicker.NextFundingAt, NextBitgetFunding: tick.BitgetTicker.NextFundingAt}
	e.nextID++
	position.ID = fmt.Sprintf("paper-%d-%d", tick.Now.UnixMilli(), e.nextID)

	var bybitFill, bitgetFill orderbook.FillEstimate
	var err error
	if zScore > 0 {
		position.Direction = LongBitgetShortBybit
		bybitFill, err = tick.BybitBook.Sell(quantity)
		if err == nil {
			bitgetFill, err = tick.BitgetBook.Buy(quantity)
		}
	} else {
		position.Direction = LongBybitShortBitget
		bybitFill, err = tick.BybitBook.Buy(quantity)
		if err == nil {
			bitgetFill, err = tick.BitgetBook.Sell(quantity)
		}
	}
	if err != nil {
		return Position{}, false
	}
	position.BybitEntryPrice = bybitFill.AveragePrice
	position.BitgetEntryPrice = bitgetFill.AveragePrice
	position.BybitEntryNotional = bybitFill.QuoteNotional
	position.BitgetEntryNotional = bitgetFill.QuoteNotional
	position.EntryFees = bybitFill.QuoteNotional*e.config.BybitFeeBps/10_000 + bitgetFill.QuoteNotional*e.config.BitgetFeeBps/10_000
	position.EntrySafetyCost = (bybitFill.QuoteNotional + bitgetFill.QuoteNotional) * e.config.SafetyBpsPerLeg / 10_000
	if e.grossExposure()+position.BybitEntryNotional+position.BitgetEntryNotional > e.config.MaximumGrossUSDT && e.config.MaximumGrossUSDT > 0 {
		return Position{}, false
	}
	return position, true
}

func (e *Engine) closePosition(position Position, tick Tick, zScore, spreadBps float64, reason CloseReason) (Event, bool) {
	var bybitExit, bitgetExit orderbook.FillEstimate
	var err error
	if position.Direction == LongBybitShortBitget {
		bybitExit, err = tick.BybitBook.Sell(position.Quantity)
		if err == nil {
			bitgetExit, err = tick.BitgetBook.Buy(position.Quantity)
		}
	} else {
		bybitExit, err = tick.BybitBook.Buy(position.Quantity)
		if err == nil {
			bitgetExit, err = tick.BitgetBook.Sell(position.Quantity)
		}
	}
	if err != nil {
		return Event{}, false
	}

	gross := 0.0
	if position.Direction == LongBybitShortBitget {
		gross = (bybitExit.QuoteNotional - position.BybitEntryNotional) + (position.BitgetEntryNotional - bitgetExit.QuoteNotional)
	} else {
		gross = (position.BybitEntryNotional - bybitExit.QuoteNotional) + (bitgetExit.QuoteNotional - position.BitgetEntryNotional)
	}
	exitFees := bybitExit.QuoteNotional*e.config.BybitFeeBps/10_000 + bitgetExit.QuoteNotional*e.config.BitgetFeeBps/10_000
	exitSafety := (bybitExit.QuoteNotional + bitgetExit.QuoteNotional) * e.config.SafetyBpsPerLeg / 10_000
	totalFees := position.EntryFees + exitFees
	totalSafety := position.EntrySafetyCost + exitSafety
	net := gross - totalFees - totalSafety + position.FundingPnL
	e.realized += net
	e.trades++
	e.totalHold += tick.Now.Sub(position.OpenedAt)
	if net >= 0 {
		e.wins++
	} else {
		e.losses++
	}
	if e.realized > e.peakEquity {
		e.peakEquity = e.realized
	}
	drawdown := e.peakEquity - e.realized
	if drawdown > e.maxDrawdown {
		e.maxDrawdown = drawdown
	}
	return Event{Type: "CLOSE", Time: tick.Now, Position: position, CloseReason: reason, CurrentZ: zScore, SpreadBps: spreadBps, GrossPnL: gross, Fees: totalFees, SafetyCost: totalSafety, FundingPnL: position.FundingPnL, NetPnL: net, Equity: e.realized}, true
}

func (e *Engine) applyFunding(position Position, tick Tick, zScore, spreadBps float64) (Position, []Event) {
	events := make([]Event, 0, 2)
	bybitMid := bestMid(tick.BybitBook)
	bitgetMid := bestMid(tick.BitgetBook)
	for !position.NextBybitFunding.IsZero() && !tick.Now.Before(position.NextBybitFunding) {
		payment := position.Quantity * bybitMid * tick.BybitTicker.FundingRate
		if position.Direction == LongBybitShortBitget {
			payment = -payment
		}
		position.FundingPnL += payment
		events = append(events, Event{Type: "FUNDING", Time: position.NextBybitFunding, Position: position, CurrentZ: zScore, SpreadBps: spreadBps, FundingPnL: payment, Equity: e.realized + position.FundingPnL})
		position.NextBybitFunding = position.NextBybitFunding.Add(tick.BybitInstrument.FundingInterval)
		if tick.BybitInstrument.FundingInterval <= 0 {
			position.NextBybitFunding = time.Time{}
		}
	}
	for !position.NextBitgetFunding.IsZero() && !tick.Now.Before(position.NextBitgetFunding) {
		payment := position.Quantity * bitgetMid * tick.BitgetTicker.FundingRate
		if position.Direction == LongBitgetShortBybit {
			payment = -payment
		}
		position.FundingPnL += payment
		events = append(events, Event{Type: "FUNDING", Time: position.NextBitgetFunding, Position: position, CurrentZ: zScore, SpreadBps: spreadBps, FundingPnL: payment, Equity: e.realized + position.FundingPnL})
		position.NextBitgetFunding = position.NextBitgetFunding.Add(tick.BitgetInstrument.FundingInterval)
		if tick.BitgetInstrument.FundingInterval <= 0 {
			position.NextBitgetFunding = time.Time{}
		}
	}
	return position, events
}

func (e *Engine) closeDecision(position Position, zScore float64, now time.Time) (CloseReason, bool) {
	if math.Abs(zScore) >= e.config.StopZ && e.config.StopZ > 0 {
		return CloseStop, true
	}
	if math.Abs(zScore) <= e.config.ExitZ && now.Sub(position.OpenedAt) >= e.config.MinimumHold {
		return CloseConvergence, true
	}
	if e.config.MaximumHold > 0 && now.Sub(position.OpenedAt) >= e.config.MaximumHold {
		return CloseTimeout, true
	}
	return "", false
}

func (e *Engine) Snapshot() Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	positions := make([]Position, 0, len(e.positions))
	for _, position := range e.positions {
		positions = append(positions, position)
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i].Symbol < positions[j].Symbol })
	averageHold := 0.0
	if e.trades > 0 {
		averageHold = e.totalHold.Seconds() / float64(e.trades)
	}
	return Snapshot{Equity: e.realized, RealizedPnL: e.realized, Trades: e.trades, Wins: e.wins, Losses: e.losses, PeakEquity: e.peakEquity, MaxDrawdown: e.maxDrawdown, TotalHoldSeconds: e.totalHold.Seconds(), AverageHoldSeconds: averageHold, Positions: positions}
}

func (e *Engine) Restore(snapshot Snapshot) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	positions := make(map[string]Position, len(snapshot.Positions))
	for _, position := range snapshot.Positions {
		if position.Symbol == "" || position.Quantity <= 0 || position.ID == "" {
			return fmt.Errorf("invalid restored position: %+v", position)
		}
		if _, exists := positions[position.Symbol]; exists {
			return fmt.Errorf("duplicate restored position for %s", position.Symbol)
		}
		positions[position.Symbol] = position
	}
	e.positions = positions
	e.realized = snapshot.RealizedPnL
	e.trades = snapshot.Trades
	e.wins = snapshot.Wins
	e.losses = snapshot.Losses
	e.peakEquity = snapshot.PeakEquity
	e.maxDrawdown = snapshot.MaxDrawdown
	e.totalHold = time.Duration(snapshot.TotalHoldSeconds * float64(time.Second))
	return nil
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func (e *Engine) grossExposure() float64 {
	total := 0.0
	for _, position := range e.positions {
		total += position.BybitEntryNotional + position.BitgetEntryNotional
	}
	return total
}

func (e *Engine) roundTripCostBps() float64 {
	return 2*(e.config.BybitFeeBps+e.config.BitgetFeeBps) + 4*e.config.SafetyBpsPerLeg
}

func validTick(tick Tick) bool {
	return tick.Now.IsZero() == false && tick.BybitBook.Symbol != "" && tick.BybitBook.Symbol == tick.BitgetBook.Symbol &&
		len(tick.BybitBook.Bids) > 0 && len(tick.BybitBook.Asks) > 0 && len(tick.BitgetBook.Bids) > 0 && len(tick.BitgetBook.Asks) > 0
}

func canonicalSpreadBps(bybitBook, bitgetBook orderbook.Book) float64 {
	return math.Log(bestMid(bybitBook)/bestMid(bitgetBook)) * 10_000
}

func bestMid(book orderbook.Book) float64 {
	return (book.Bids[0].Price + book.Asks[0].Price) / 2
}
