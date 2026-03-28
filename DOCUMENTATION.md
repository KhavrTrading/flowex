# Flowex API Reference

Full API reference for the flowex library. For installation, quick start, and usage examples, see [README.md](README.md).

---

## Table of Contents

- [Snapshot & Manager Interface](#snapshot--manager-interface)
- [Data Models](#data-models)
- [Depth Metrics Reference](#depth-metrics-reference)
- [Depth Store Query Methods](#depth-store-query-methods)
- [Historical Data Seeding](#historical-data-seeding)
- [Candle Deduplication](#candle-deduplication)
- [Worker Monitoring](#worker-monitoring)
- [Worker Error Tracking](#worker-error-tracking)
- [Handler Callbacks](#handler-callbacks)
- [Convenience Worker Accessors](#convenience-worker-accessors)
- [Auto-Reconnect Behavior](#auto-reconnect-behavior)
- [Handler Types Reference](#handler-types-reference)
- [Technical Indicators (Optimized)](#technical-indicators-optimized)
- [Signal & Movement Types](#signal--movement-types)

---

## Snapshot & Manager Interface

Every exchange manager implements the `ws.Manager` interface:

```go
type Manager interface {
    SubscribeCandle(symbol string, handler CandleHandler) error
    SubscribeDepth(symbol string, handler DepthHandler) error
    SubscribeTrade(symbol string, handler TradeHandler) error
    SubscribeAll(symbol string, ch CandleHandler, dh DepthHandler, th TradeHandler) error
    Unsubscribe(symbol string, streamType StreamType) error
    UnsubscribeAll(symbol string) error
    GetSnapshot(symbol string) *Snapshot
    GetStatus() map[string]interface{}
    Shutdown()
}
```

`GetSnapshot` returns an immutable, point-in-time view:

```go
type Snapshot struct {
    Timestamp  time.Time               // when the snapshot was taken
    Candles    []models.CandleHLCV     // historical + live candle bars
    DepthStore *depth.Store            // order book metrics with time-bucketed storage
    Trades     []models.NormalizedTrade // recent trades, normalized across exchanges
}
```

Snapshots are updated atomically at a configurable interval (default 1s). Readers never contend with the writer — safe to call from any goroutine.

---

## Data Models

### CandleHLCV

OHLCV bar from any exchange. Used in snapshots and by indicators that need volume.

```go
type CandleHLCV struct {
    Ts     int64   // Unix millisecond timestamp
    Open   float64
    High   float64
    Low    float64
    Close  float64
    Volume float64
}
```

Helper methods: `GetTimestamp()`, `HL2()` (High+Low)/2, `HLC3()` (High+Low+Close)/3.

### CandleHLC

Lighter candle without Open/Volume. Used by ATR, Bollinger, and Support/Resistance indicators.

```go
type CandleHLC struct {
    // ts is unexported — access via GetTimestamp()
    High  float64
    Low   float64
    Close float64
}
```

Methods: `GetTimestamp()`, `GetHigh()`, `GetLow()`, `GetClose()`.

### NormalizedTrade

Unified trade format across all exchanges.

```go
type NormalizedTrade struct {
    Timestamp int64   // Unix milliseconds
    Price     float64
    Size      float64 // base currency amount
    SizeUSD   float64
    Side      string  // "buy" or "sell"
    TradeID   string
    Symbol    string  // e.g. "BTCUSDT"
    Exchange  string  // "binance", "bybit", "bitget"
}
```

### TickerData

Defined in `models/ticker.go`. Currently reserved for future ticker stream support — not actively used by any manager.

```go
type TickerData struct {
    Symbol   string
    LastPr   float64
    Bid      float64
    Ask      float64
    BidStr   string
    AskStr   string
    Price    float64
    PriceStr string
}
```

---

## Depth Metrics Reference

`depth.DepthMetrics` contains 75 computed fields from raw order book data. Fields are grouped by category.

### Spread (7 fields)

| Field | Type | Description |
|-------|------|-------------|
| `Timestamp` | int64 | Unix milliseconds |
| `Symbol` | string | Trading pair |
| `BestBid` | float64 | Best bid price |
| `BestAsk` | float64 | Best ask price |
| `Spread` | float64 | ask - bid |
| `SpreadBps` | float64 | spread / mid * 10000 (basis points) |
| `MidPrice` | float64 | (bid + ask) / 2 |

### Liquidity — USD Value (8 fields)

Dollar value of resting orders at each depth level.

| Field | Type | Description |
|-------|------|-------------|
| `BidLiquidity5` | float64 | Bid-side liquidity, top 5 levels |
| `AskLiquidity5` | float64 | Ask-side liquidity, top 5 levels |
| `BidLiquidity10` | float64 | Top 10 levels |
| `AskLiquidity10` | float64 | Top 10 levels |
| `BidLiquidity20` | float64 | Top 20 levels |
| `AskLiquidity20` | float64 | Top 20 levels |
| `BidLiquidity50` | float64 | Top 50 levels |
| `AskLiquidity50` | float64 | Top 50 levels |

### Volume — Coin Size (8 fields)

Raw coin volume at each depth level (not USD-denominated).

| Field | Type | Description |
|-------|------|-------------|
| `BidVolume5` | float64 | Bid volume, top 5 levels |
| `AskVolume5` | float64 | Ask volume, top 5 levels |
| `BidVolume10` | float64 | Top 10 levels |
| `AskVolume10` | float64 | Top 10 levels |
| `BidVolume20` | float64 | Top 20 levels |
| `AskVolume20` | float64 | Top 20 levels |
| `BidVolume50` | float64 | Top 50 levels |
| `AskVolume50` | float64 | Top 50 levels |

### Imbalance (6 fields)

Measures bid/ask asymmetry. Ratio > 1 = bid-heavy (bullish signal). Delta ranges -100 to +100.

| Field | Type | Description |
|-------|------|-------------|
| `ImbalanceRatio5` | float64 | bid_liq / ask_liq at 5 levels |
| `ImbalanceRatio10` | float64 | At 10 levels |
| `ImbalanceRatio20` | float64 | At 20 levels |
| `ImbalanceRatio50` | float64 | At 50 levels |
| `ImbalanceDelta10` | float64 | (bid-ask)/(bid+ask)*100 at 10 levels |
| `ImbalanceDelta20` | float64 | (bid-ask)/(bid+ask)*100 at 20 levels |

### Walls — Largest Single Orders (6 fields)

Detects large resting orders that may act as support/resistance.

| Field | Type | Description |
|-------|------|-------------|
| `LargestBidSize` | float64 | Biggest single bid order (coin size) |
| `LargestBidPrice` | float64 | Price level of that bid |
| `LargestBidValue` | float64 | USD value of that bid |
| `LargestAskSize` | float64 | Biggest single ask order (coin size) |
| `LargestAskPrice` | float64 | Price level of that ask |
| `LargestAskValue` | float64 | USD value of that ask |

### Slippage Estimation (16 fields)

Estimated price impact (%) for a market order of a given USD size.

| Field | Type | Description |
|-------|------|-------------|
| `SlippageBuy100` | float64 | Slippage to buy $100 |
| `SlippageSell100` | float64 | Slippage to sell $100 |
| `SlippageBuy1K` | float64 | $1,000 |
| `SlippageSell1K` | float64 | $1,000 |
| `SlippageBuy5K` | float64 | $5,000 |
| `SlippageSell5K` | float64 | $5,000 |
| `SlippageBuy10K` | float64 | $10,000 |
| `SlippageSell10K` | float64 | $10,000 |
| `SlippageBuy50K` | float64 | $50,000 |
| `SlippageSell50K` | float64 | $50,000 |
| `SlippageBuy100K` | float64 | $100,000 |
| `SlippageSell100K` | float64 | $100,000 |
| `SlippageBuy500K` | float64 | $500,000 |
| `SlippageSell500K` | float64 | $500,000 |
| `SlippageBuy1M` | float64 | $1,000,000 |
| `SlippageSell1M` | float64 | $1,000,000 |

### Velocity — Rate of Change (5 fields)

How fast metrics are changing. Computed from historical store data.

| Field | Type | Description |
|-------|------|-------------|
| `LiquidityVelocity10` | float64 | Rate of change of liquidity at 10 levels |
| `LiquidityVelocity50` | float64 | At 50 levels |
| `ImbalanceVelocity` | float64 | Rate of imbalance shift |
| `SpreadVelocity` | float64 | Rate of spread change |
| `WallVelocity` | float64 | Rate of wall size change |

### Momentum (4 fields)

Trend direction indicators derived from order flow.

| Field | Type | Description |
|-------|------|-------------|
| `BuyPressureMomentum` | float64 | Buy-side pressure trend |
| `SellPressureMomentum` | float64 | Sell-side pressure trend |
| `WallBuildingBid` | bool | True if bid wall is growing over time |
| `WallBuildingAsk` | bool | True if ask wall is growing over time |

### Statistical Z-Scores (3 fields)

How unusual the current value is compared to recent history. High absolute z-score = unusual.

| Field | Type | Description |
|-------|------|-------------|
| `LiquidityZScore10` | float64 | How unusual current liquidity is |
| `ImbalanceZScore` | float64 | How unusual imbalance is |
| `SpreadZScore` | float64 | How unusual spread is |

### Depth Quality & Microstructure (12 fields)

| Field | Type | Description |
|-------|------|-------------|
| `BidLevelsCount` | int | Number of bid price levels in the book |
| `AskLevelsCount` | int | Number of ask price levels |
| `AvgBidSize10` | float64 | Average bid size in top 10 levels |
| `AvgAskSize10` | float64 | Average ask size in top 10 levels |
| `TopBidConcentration5` | float64 | How concentrated top 5 bids are |
| `TopAskConcentration5` | float64 | How concentrated top 5 asks are |
| `SpreadNormImbalanceDelta10` | float64 | Spread-normalized imbalance at 10 levels |
| `SpreadNormImbalanceDelta20` | float64 | Spread-normalized imbalance at 20 levels |
| `SlippageGradientBuy` | float64 | How slippage scales with order size (buy) |
| `SlippageGradientSell` | float64 | How slippage scales with order size (sell) |
| `SlippageSkew1K` | float64 | Buy vs sell slippage asymmetry at $1K |
| `SlippageSkew10K` | float64 | Buy vs sell slippage asymmetry at $10K |

All fields have JSON tags (e.g., `json:"spread_bps"`). The full struct is defined in `depth/metrics.go`.

---

## Depth Store Query Methods

`depth.Store` provides time-bucketed storage with several query methods:

```go
store := snap.DepthStore

// Most recent metric, or nil if no data yet
latest := store.GetLatest()

// Copy of the recent metrics buffer (default last 100 entries)
recent := store.GetRecent()

// All metrics from the last N seconds
last30s := store.GetLastNSeconds(30)

// Metrics within a specific time window (Unix milliseconds, inclusive)
ranged := store.GetByTimeRange(startMs, endMs)

// Total number of stored metrics
count := store.Size()
```

All methods are thread-safe (read-locked). `GetLatest()` and `GetRecent()` return copies — safe to hold across calls.

---

## Historical Data Seeding

Most strategies need candle history on startup. Fetch via REST, then feed into the worker:

```go
mgr := binance.NewManager()
mgr.SubscribeAll("BTCUSDT", nil, nil, nil)

// Access the worker
worker := mgr.GetOrCreateWorker("BTCUSDT")

// Fetch historical candles via REST
hist, err := candles.FetchBinanceCandles("BTCUSDT", "1m", 500)
if err != nil {
    log.Fatal(err)
}

// Seed them into the worker
for _, c := range hist {
    worker.EnqueueCandle(ws.CandleMsg{
        Timestamp: c.Ts,
        Open:      fmt.Sprintf("%f", c.Open),
        High:      fmt.Sprintf("%f", c.High),
        Low:       fmt.Sprintf("%f", c.Low),
        Close:     fmt.Sprintf("%f", c.Close),
        Volume:    fmt.Sprintf("%f", c.Volume),
    })
}
// Snapshot now has 500 candles immediately — no waiting for live bars
```

The `CandleMsg` struct expects string values (matching the raw WebSocket format):

```go
type CandleMsg struct {
    Timestamp          int64
    Open, High, Low, Close, Volume string
}
```

Similarly available: `EnqueueDepth(DepthMsg)` and `EnqueueTrade(TradeMsg)`.

All enqueue methods are non-blocking. If the channel is full, the oldest message is dropped and the drop counter increments.

---

## Candle Deduplication

The worker automatically deduplicates candles using timestamp logic:

| Incoming candle timestamp | Behavior |
|--------------------------|----------|
| **Same** as last candle | Updates in place: High (if higher), Low (if lower), Close, Volume |
| **Newer** than last candle | Appends as new bar, trims oldest if over `MaxCandles` |
| **Older** than last candle | Silently ignored |

This means you can safely overlap historical and live data — for example, fetch 500 historical candles then subscribe to live, even if some timestamps overlap. The worker handles dedup automatically. No risk of duplicate bars.

---

## Worker Monitoring

Track worker health via `GetMetrics()`:

```go
worker := mgr.GetOrCreateWorker("BTCUSDT")
metrics := worker.GetMetrics()

metrics["processed"]      // total messages processed across all channels
metrics["candle_dropped"] // candle messages dropped (channel was full)
metrics["depth_dropped"]  // depth messages dropped
metrics["trade_dropped"]  // trade messages dropped
metrics["candle_queue"]   // current candle channel fill level
metrics["depth_queue"]    // current depth channel fill level
metrics["trade_queue"]    // current trade channel fill level
```

**When to act:**
- If `*_dropped` counts are climbing, the worker can't keep up. Increase the corresponding `*ChSize` in `WorkerConfig`.
- If `*_queue` values are consistently near capacity, consider reducing subscription load or increasing buffer sizes.
- `processed` count growing steadily = healthy worker.

---

## Worker Error Tracking

Workers track the last 10 parse/processing errors:

```go
worker := mgr.GetOrCreateWorker("BTCUSDT")
errors := worker.GetRecentErrors()
// Returns []string, e.g.:
// ["[15:04:05] parse candle: strconv.ParseFloat: ..."]
```

Useful for detecting malformed exchange data or API format changes. Errors are timestamped with `[HH:MM:SS]` prefix.

---

## Handler Callbacks

### Subscribe-time handlers

The subscribe methods accept handler functions that fire on every raw message from the WebSocket:

```go
// Called for every candle update from the exchange
mgr.SubscribeCandle("BTCUSDT", func(candle models.CandleHLCV) {
    fmt.Printf("candle: O=%.2f C=%.2f V=%.4f\n", candle.Open, candle.Close, candle.Volume)
})

// Called for every depth snapshot
mgr.SubscribeDepth("BTCUSDT", func(bids, asks [][]string, ts int64) {
    fmt.Printf("depth: %d bids, %d asks\n", len(bids), len(asks))
})

// Called for every trade
mgr.SubscribeTrade("BTCUSDT", func(trade models.NormalizedTrade) {
    fmt.Printf("trade: %s $%.0f @ %.2f\n", trade.Side, trade.SizeUSD, trade.Price)
})

// Pass nil for any handler you don't need
mgr.SubscribeAll("BTCUSDT", nil, nil, nil)
```

### Worker hooks (SetOn*Update)

Worker hooks fire inside the worker goroutine **after** state has been mutated:

```go
worker := mgr.GetOrCreateWorker("BTCUSDT")

// Called after candle state is updated — receives the full candle slice
worker.SetOnCandleUpdate(func(candles []models.CandleHLCV) {
    // candles includes all history, not just the latest
})

// Called after depth metrics are computed
worker.SetOnDepthUpdate(func(m depth.DepthMetrics) {
    // m is the freshly computed metric
})

// Called after a trade is normalized and stored
worker.SetOnTradeUpdate(func(t models.NormalizedTrade) {
    // t is the single new trade
})
```

**Key difference:**
- **Subscribe handlers** fire on the dispatch path (raw WebSocket messages, before processing)
- **Worker hooks** fire inside the worker loop (after state mutation, with access to full state)

Use subscribe handlers for logging/forwarding raw data. Use worker hooks for strategy logic that depends on accumulated state.

---

## Convenience Worker Accessors

Shortcuts that read from the snapshot internally:

```go
worker := mgr.GetOrCreateWorker("BTCUSDT")

candles := worker.GetCandles()          // []models.CandleHLCV (or nil)
store   := worker.GetDepthStore()       // *depth.Store (or nil)
trades  := worker.GetNormalizedTrades() // []models.NormalizedTrade (or nil)
```

These are equivalent to `worker.GetSnapshot().Candles`, etc., with nil-safety built in.

---

## Auto-Reconnect Behavior

The WebSocket client automatically handles connection drops:

1. On read error: waits `ReconnectDelay` (default **2 seconds**), then reconnects
2. After reconnect: calls `ResubscribeFunc` to restore all active stream subscriptions
3. No manual intervention needed — the manager handles the full lifecycle

**Connection defaults:**

```go
ClientConfig{
    ReadBufferSize:  16 * 1024,       // 16 KB
    WriteBufferSize: 16 * 1024,       // 16 KB
    ReconnectDelay:  2 * time.Second,
}
```

**Additional details:**
- WebSocket compression is enabled by default (`dialer.EnableCompression = true`)
- Heartbeat pings are exchange-specific and handled automatically by each adapter
- Binance: no application-level ping (uses WebSocket protocol pings)
- Bybit/Bitget: application-level pings configured internally by their `NewClient()` functions

---

## Handler Types Reference

Defined in `ws/interfaces.go`:

```go
// CandleHandler is called when a new candle update arrives.
type CandleHandler func(candle models.CandleHLCV)

// DepthHandler is called when a new order book snapshot arrives.
type DepthHandler func(bids, asks [][]string, timestamp int64)

// TradeHandler is called when a new trade arrives.
type TradeHandler func(trade models.NormalizedTrade)
```

Stream type constants for unsubscribe:

```go
const (
    StreamCandle StreamType = "candle"
    StreamDepth  StreamType = "depth"
    StreamTrade  StreamType = "trade"
)
```

---

## Technical Indicators (Optimized)

The `indicators/technical` package provides batch-optimized indicator calculations with pre-computed multipliers, single-pass algorithms, and pooled memory. These complement the standard `indicators/` package.

### CalculateTechnicalIndicators

Computes all indicators in one call. Returns a `TechnicalIndicators` struct with RSI, SMA, EMA, MACD, Bollinger Bands, ATR, StochRSI, MMI, and TradingView-style summary signals.

```go
import "github.com/KhavrTrading/flowex/indicators/technical"

// Needs at least 20 candles, 200+ for full SMA200/EMA200
result := technical.CalculateTechnicalIndicators(candles, currentPrice)
if result == nil {
    return // not enough data
}

// Individual indicators
result.RSI14       // RSI (14-period)
result.EMA9        // EMA 9
result.SMA200      // SMA 200
result.MACDLine    // MACD line
result.SignalLine  // MACD signal
result.Histogram   // MACD histogram
result.BBUpper     // Bollinger upper band
result.BBMiddle    // Bollinger middle
result.BBLower     // Bollinger lower band
result.ATR         // Average True Range (14)
result.StochRSI    // Stochastic RSI
result.MMI         // Market Manipulation Index (0-100: 0-30=clean, 30-70=normal, 70-100=manipulated)

// TradingView-style summary signals
result.MASummary     // technical.SignalStrongBuy / SignalBuy / SignalNeutral / SignalSell / SignalStrongSell
result.OscillatorSum // same scale
result.OverallSum    // combined weighted signal

// Signal counts
result.MABuy, result.MASell, result.MANeutral       // how many MAs agree
result.OscillBuy, result.OscillSell, result.OscillNeutr // how many oscillators agree
```

### Standalone optimized functions

```go
// EMA with pre-computed multiplier (faster than indicators.CalculateEMA)
ema := technical.CalculateEMAFast(prices, 20, 2.0/21.0)

// ATR directly from CandleHLCV (no conversion to CandleHLC needed)
atr := technical.CalculateATRFast(candles, 14)

// ADX — trend strength (0-100: <20=weak, 20-40=strong, >40=very strong)
adx := technical.CalculateADXFast(candles, 14)
```

### TechnicalIndicators struct

```go
type TechnicalIndicators struct {
    RSI14      float64         // RSI (14-period)
    SMA20      float64         // Simple Moving Averages
    SMA50      float64
    SMA200     float64
    EMA9       float64         // Exponential Moving Averages
    EMA12      float64
    EMA20      float64
    EMA21      float64
    EMA26      float64
    EMA50      float64
    EMA200     float64
    MACDLine   float64         // MACD
    SignalLine float64
    Histogram  float64
    BBUpper    float64         // Bollinger Bands
    BBMiddle   float64
    BBLower    float64
    ATR        float64         // Average True Range
    StochRSI   float64         // Stochastic RSI
    MMI        float64         // Market Manipulation Index (0-100)

    // TradingView-style summaries
    MASummary     IndicatorSignal // StrongBuy(-2) to StrongSell(2)
    OscillatorSum IndicatorSignal
    OverallSum    IndicatorSignal

    // Signal counts
    MABuy, MASell, MANeutral          int
    OscillBuy, OscillSell, OscillNeutr int
}
```

---

## Signal & Movement Types

The `indicators/technical` package also defines types for building real-time signal pipelines and cross-exchange analysis.

### Signal Classification

```go
// What kind of signal was generated
type SignalType string

const (
    SignalFirstTouch    SignalType = "first_touch"    // Threshold crossed for first time
    SignalMomentumShift SignalType = "momentum_shift" // Sharp acceleration detected
    SignalPeakDetected  SignalType = "peak_detected"  // Price hit extreme, started reversing
    SignalReversal      SignalType = "reversal"        // Direction changed with conviction
    SignalDeepening     SignalType = "deepening"       // Movement continuing same direction
    SignalExhaustion    SignalType = "exhaustion"      // Movement slowing, volume declining
    SignalContinuation  SignalType = "continuation"    // Movement resumed after brief pause
    SignalConsensus     SignalType = "consensus"       // All exchanges agree
    SignalDivergence    SignalType = "divergence"      // Exchange deviation detected
)

// How confident is the signal
type SignalConfidence string

const (
    ConfidenceHigh   SignalConfidence = "high"   // All exchanges agree
    ConfidenceMedium SignalConfidence = "medium" // 2/3 exchanges agree
    ConfidenceLow    SignalConfidence = "low"    // Single exchange or high divergence
)

// Overall market state
type MarketCondition string

const (
    MarketSmooth MarketCondition = "smooth" // Clean directional move
    MarketChoppy MarketCondition = "choppy" // Oscillating, >3 direction changes
    MarketFlash  MarketCondition = "flash"  // Flash crash/pump (<2s duration)
)
```

### MovementState

Tracks a symbol's price action as a real-time state machine. Thread-safe via embedded `sync.RWMutex`.

```go
state := &technical.MovementState{
    Symbol:   "BTCUSDT",
    Exchange: "binance",
}

// Query movement
duration := state.GetMovementDuration() // how long active
priceRange := state.GetPriceRange()     // PriceRange{Min, Max, SpanPct}
state.IncrementAlertsSent()             // track alerts

// Key fields
state.CurrentPrice    // latest price
state.PeakPrice       // highest this movement
state.ValleyPrice     // lowest this movement
state.Direction       // "up" or "down"
state.CurrentVelocity // %/second
state.MarketCondition // smooth/choppy/flash
state.DirectionChanges // reversal count (choppiness)
state.IsActive        // movement in progress
```

### CrossExchangeMetrics

Holds analysis across multiple exchanges for the same symbol.

```go
type CrossExchangeMetrics struct {
    AvgPrice       float64            // average price across exchanges
    AvgChange      float64            // average price change
    StdDeviation   float64            // price spread between exchanges
    BestEntryPrice float64            // best price for entry

    ExchangePrices  map[string]float64 // exchange -> price
    ExchangeChanges map[string]float64 // exchange -> change %
    LeadingExchange string             // which exchange moved first/most
    ExchangesAgree  int                // count in agreement (2 or 3)

    Confidence           SignalConfidence
    IsDivergence         bool    // exchanges disagree significantly
    DivergenceSize       float64 // max deviation from average (%)
    ArbitrageOpportunity bool    // price spread > threshold
    ArbitrageSpread      float64 // size of opportunity
}
```

### TradingSignal

The final enriched signal with full context — price action, cross-exchange data, technical indicators, and movement metadata.

```go
type TradingSignal struct {
    // Identity
    Type      SignalType // first_touch, reversal, exhaustion, etc.
    Exchange  string
    Symbol    string
    Timeframe string

    // Price data
    PriceChange float64
    Open        float64
    Close       float64
    PeakPrice   float64
    ValleyPrice float64

    // Movement context
    MovementID       string
    SignalRank       int            // 1=best, 2=average, 3=initial
    PriceRange       PriceRange     // {Min, Max, SpanPct}
    TimeInMotion     float64        // seconds
    Velocity         float64        // %/second
    DirectionChanges int

    // Cross-exchange
    Confidence     SignalConfidence
    ExchangesAgree int
    CrossExchange  *SignalCrossExchangeData

    // Market context
    MarketCondition MarketCondition
    IsCounterTrend  bool

    // Technical indicators
    Indicators *TechnicalIndicators

    // Lifecycle
    ValidUntil time.Time
    CreatedAt  time.Time
}
```

### SignalBatch

Groups prioritized signals for one movement.

```go
type SignalBatch struct {
    MovementID    string
    Symbol        string
    Signals       []TradingSignal
    BatchTime     time.Time
    MovementStart time.Time
    MovementEnd   time.Time
}
```
