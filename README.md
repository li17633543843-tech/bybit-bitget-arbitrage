# Bybit–Bitget USDT perpetual spread scanner

This is the read-only first stage of a cross-exchange arbitrage system. It discovers common USDT perpetual symbols and ranks both trading directions by fee-adjusted top-of-book spread.

It does **not** use API keys and does **not** place orders.

## Run

Requires Go 1.23 or newer:

```bash
go test ./...
go run ./cmd/scanner \
  -interval 3s \
  -min-edge-bps 5 \
  -min-turnover 1000000 \
  -min-top-notional 100 \
  -bybit-fee-bps 5.5 \
  -bitget-fee-bps 6 \
  -buffer-bps 3 \
  -min-samples 3 \
  -min-duration 5s \
  -ledger var/opportunities.jsonl
```

### WebSocket depth scanner

The depth scanner performs one REST universe discovery, subscribes the strongest common candidates to Bybit 50-level and Bitget 15-level public order books, and continuously evaluates both directions using the requested USDT notional:

```bash
go run ./cmd/depthscan \
  -candidates 50 \
  -notional 100 \
  -min-turnover 1000000 \
  -min-edge-bps 5 \
  -bybit-fee-bps 5.5 \
  -bitget-fee-bps 6 \
  -buffer-bps 3 \
  -max-book-age 2s
```

Both feeds send heartbeats, reconnect after a disconnect, reject stale cross-exchange book pairs, and never authenticate or place orders.

Before subscribing, the scanner loads both exchanges' live contract specifications and keeps only matching, actively traded USDT perpetuals. The simulated quantity is adjusted to the strictest minimum quantity, quantity step, minimum notional, and maximum market-order quantity across both venues.

### Stateful paper trading

Enable the complete open-hold-close simulation explicitly:

```bash
go run ./cmd/depthscan \
  -paper \
  -candidates 50 \
  -notional 100 \
  -window 300 \
  -warmup 300 \
  -sample-interval 1s \
  -entry-z 2.5 \
  -max-entry-z 8 \
  -entry-confirm 3s \
  -exit-z 0.5 \
  -stop-z 4 \
  -min-hold 5s \
  -max-hold 2h \
  -max-book-skew 500ms \
  -max-positions 5 \
  -max-gross-usdt 2000 \
  -bybit-fee-bps 5.5 \
  -bitget-fee-bps 6 \
  -buffer-bps 3 \
  -paper-ledger var/paper-events.jsonl \
  -paper-snapshot var/paper-snapshot.json
```

Paper mode maintains an independent rolling log-price spread per symbol. It opens the cheap venue long and the expensive venue short after warmup and an entry Z-score, then closes both legs on convergence, the Z-score stop, or maximum holding time. It charges all four taker fees, multi-level VWAP, a per-leg safety cost, and funding settlements. Only one paper position per symbol is allowed, with global position-count and gross-exposure limits.

Entries must remain beyond the threshold in the same direction for `-entry-confirm`, are rejected when their absolute Z-score exceeds `-max-entry-z`, and require the two exchange books to be within `-max-book-skew`. Convergence exits respect `-min-hold`; stop-loss and maximum-hold exits remain immediate. Bitget funding rate and next settlement time are loaded from its current funding-rate endpoint rather than inferred from ticker data.

Every open, funding, and close event is appended to JSONL. An atomic portfolio snapshot stores open positions, realized equity, wins/losses, peak equity, maximum drawdown, and average holding time. The snapshot is restored automatically after restart; restored positions must warm their rolling window again before a statistical exit is allowed.

### Read-only monitoring dashboard

`depthscan` serves a dashboard and JSON status API on `127.0.0.1:8080` by default:

```bash
curl http://127.0.0.1:8080/healthz
```

The page shows recent executable opportunities, open paper positions, realized performance, drawdown, and recent paper events. Keep it bound to localhost and expose it through an authenticated HTTPS reverse proxy. For temporary direct testing only, use `-http-addr :8080` and restrict port 8080 in the cloud firewall to your own IP. Set `-http-addr ''` to disable it.

Fee flags must be changed to the actual fee tier of each account. The funding delta is displayed for context but is not subtracted from the immediate entry spread because its effect depends on holding time and the next settlement times.

An opportunity is printed only after it has met both the consecutive sample and persistence-time requirements. Confirmed observations are appended to a JSONL ledger for later replay and parameter analysis.

The project also contains a multi-level order-book walker for estimating VWAP and a guarded execution transaction state machine. These are isolated from live exchange credentials until paper execution and recovery tests are complete.

Depth evaluation calculates both legs at the requested base quantity, rejects insufficient liquidity, deducts fees and a safety allowance, and reports expected quote-currency PnL. This prevents a tiny first-level quote from making an illiquid altcoin look profitable.

## Current limitations

- Candidate discovery is a startup REST snapshot; newly emerging symbols require a restart until dynamic subscription refresh is added.
- WebSocket books use `float64` for read-only analytics; live order quantities and prices must use exchange precision metadata and fixed-point values.
- Read-only quantity normalization currently uses floating-point arithmetic; order submission must convert exchange decimal strings to fixed-point integers before live trading.
- Paper fills include depth VWAP and configurable safety cost but not a delayed-order queue; private account streams and live execution are intentionally absent.

## Safe development sequence

1. Collect and persist scanner observations.
2. Validate the included WebSocket books against live exchange traffic and add dynamic subscription refresh.
3. Wire the included multi-level executable-price calculation into live books.
4. Wire the included execution state machine into a paper broker.
5. Add private order/position streams and startup reconciliation.
6. Only then add explicitly enabled live orders with a kill switch.
