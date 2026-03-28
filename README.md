# flowex

[![Go Reference](https://pkg.go.dev/badge/github.com/KhavrTrading/flowex.svg)](https://pkg.go.dev/github.com/KhavrTrading/flowex)
[![Go Report Card](https://goreportcard.com/badge/github.com/KhavrTrading/flowex)](https://goreportcard.com/report/github.com/KhavrTrading/flowex)
[![Go](https://github.com/KhavrTrading/flowex/actions/workflows/go.yml/badge.svg)](https://github.com/KhavrTrading/flowex/actions/workflows/go.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Production-tested Go library for real-time cryptocurrency market data via WebSocket. Supports **Binance Futures**, **Bybit Linear**, and **Bitget Futures** with a unified interface.

## Install

```bash
go get github.com/KhavrTrading/flowex
```

Requires **Go 1.22+**

## Package Map

```
flowex/
  binance/     — Binance Futures WebSocket manager
  bybit/       — Bybit Linear WebSocket manager
  bitget/      — Bitget Futures/Spot WebSocket manager
  ws/          — Core engine: client, worker, manager, snapshots
  models/      — Candle, Trade, Ticker types
  depth/       — Order book metrics (75 fields) + time-bucketed store
  candles/     — REST fetchers + timeframe aggregation
  indicators/  — EMA, RSI, MACD, ATR, Bollinger, StochRSI, S/R
  examples/    — Working examples
```

## Quick Start

```go
package main

import (
    "fmt"
    "os"
    "os/signal"
    "time"

    "github.com/KhavrTrading/flowex/binance"
)

func main() {
    mgr := binance.NewManager()

    // Subscribe to candles + depth + trades for BTCUSDT
    mgr.SubscribeAll("BTCUSDT", nil, nil, nil)

    // Poll snapshots every 5 seconds
    go func() {
        for range time.Tick(5 * time.Second) {
            snap := mgr.GetSnapshot("BTCUSDT")
            if snap == nil {
                continue
            }
            fmt.Printf("Candles: %d | Trades: %d | Depth points: %d\n",
                len(snap.Candles), len(snap.Trades), snap.DepthStore.Size())

            if len(snap.Candles) > 0 {
                c := snap.Candles[len(snap.Candles)-1]
                fmt.Printf("  Last: O=%.2f H=%.2f L=%.2f C=%.2f V=%.4f\n",
                    c.Open, c.High, c.Low, c.Close, c.Volume)
            }
        }
    }()

    // Wait for Ctrl+C
    ch := make(chan os.Signal, 1)
    signal.Notify(ch, os.Interrupt)
    <-ch

    mgr.Shutdown()
}
```

See [examples/basic/main.go](examples/basic/main.go) for a complete working example with worker hooks, snapshot polling, and metrics monitoring.

---

## Connecting to Exchanges

Each exchange has its own manager. Create one, subscribe to symbols, read snapshots.

### Binance Futures

```go
import "github.com/KhavrTrading/flowex/binance"

mgr := binance.NewManager()                        // default config
mgr.SubscribeAll("BTCUSDT", nil, nil, nil)          // candles + depth + trades
mgr.SubscribeAll("ETHUSDT", nil, nil, nil)          // subscribe to multiple symbols
```

### Bybit V5 Linear

```go
import "github.com/KhavrTrading/flowex/bybit"

mgr := bybit.NewManager()
mgr.SubscribeAll("BTCUSDT", nil, nil, nil)
```

### Bitget USDT-Futures

```go
import "github.com/KhavrTrading/flowex/bitget"

mgr := bitget.NewManager()                          // defaults to USDT-FUTURES
mgr.SubscribeAll("BTCUSDT", nil, nil, nil)

// Or for spot:
cfg := bitget.DefaultManagerConfig()
cfg.InstType = bitget.InstSpot
spotMgr := bitget.NewManagerWithConfig(cfg)
```

### Multi-Exchange (same symbol, all exchanges)

```go
binanceMgr := binance.NewManager()
bybitMgr   := bybit.NewManager()
bitgetMgr  := bitget.NewManager()

for _, symbol := range []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"} {
    binanceMgr.SubscribeAll(symbol, nil, nil, nil)
    bybitMgr.SubscribeAll(symbol, nil, nil, nil)
    bitgetMgr.SubscribeAll(symbol, nil, nil, nil)
}
```

---

## Subscribe Selectively

You don't have to subscribe to everything. Pick what you need:

```go
mgr := binance.NewManager()

// Only candles
mgr.SubscribeCandle("BTCUSDT", nil)

// Only depth
mgr.SubscribeDepth("ETHUSDT", nil)

// Only trades
mgr.SubscribeTrade("SOLUSDT", nil)

// Unsubscribe one stream
mgr.Unsubscribe("BTCUSDT", ws.StreamCandle)

// Unsubscribe everything for a symbol
mgr.UnsubscribeAll("ETHUSDT")
```

---

## Depth Streams: Levels & Speed

Each exchange offers different order book depth options.

### Binance Depth Options

```go
import "github.com/KhavrTrading/flowex/binance"

mgr := binance.NewManager()

// Default: 20 levels, exchange default speed
mgr.SubscribeDepth("BTCUSDT", nil)

// 5 levels, 100ms updates (fastest)
mgr.SubscribeDepthWithConfig("BTCUSDT", binance.Depth5, binance.Speed100ms, nil)

// 10 levels, 500ms updates (lowest bandwidth)
mgr.SubscribeDepthWithConfig("ETHUSDT", binance.Depth10, binance.Speed500ms, nil)

// 20 levels, 100ms updates
mgr.SubscribeDepthWithConfig("SOLUSDT", binance.Depth20, binance.Speed100ms, nil)
```

Available options:
| Levels | Constants |
|--------|-----------|
| 5 | `binance.Depth5` |
| 10 | `binance.Depth10` |
| 20 | `binance.Depth20` (default) |

| Speed | Constants | Notes |
|-------|-----------|-------|
| 100ms | `binance.Speed100ms` | Fastest, highest bandwidth |
| 250ms | `binance.Speed250ms` | |
| 500ms | `binance.Speed500ms` | Lowest bandwidth |
| default | `binance.SpeedDefault` | Exchange decides |

### Bybit Depth Options

```go
import "github.com/KhavrTrading/flowex/bybit"

mgr := bybit.NewManager()

// Default: 50 levels
mgr.SubscribeDepth("BTCUSDT", nil)

// Top-of-book only (1 level) - minimal bandwidth
mgr.SubscribeDepthWithLevel("BTCUSDT", bybit.Depth1, nil)

// 200 levels
mgr.SubscribeDepthWithLevel("BTCUSDT", bybit.Depth200, nil)

// 500 levels - full book
mgr.SubscribeDepthWithLevel("BTCUSDT", bybit.Depth500, nil)
```

Available: `bybit.Depth1`, `bybit.Depth50` (default), `bybit.Depth200`, `bybit.Depth500`

### Bitget Depth Options

```go
import "github.com/KhavrTrading/flowex/bitget"

mgr := bitget.NewManager()

// Default: full book ("books")
mgr.SubscribeDepth("BTCUSDT", nil)

// Top 5 levels only
mgr.SubscribeDepthWithChannel("BTCUSDT", bitget.DepthBooks5, nil)

// Top 15 levels
mgr.SubscribeDepthWithChannel("BTCUSDT", bitget.DepthBooks15, nil)
```

Available: `bitget.DepthFull` (default), `bitget.DepthBooks5`, `bitget.DepthBooks15`

---

## Trade Streams

### Binance: Aggregate vs Individual Trades

Binance offers two trade stream types:

```go
mgr := binance.NewManager()

// Default: aggregate trades (recommended - lower bandwidth)
mgr.SubscribeTrade("BTCUSDT", nil)

// Individual trades (every single fill, higher volume)
mgr.SubscribeTradeWithMode("BTCUSDT", binance.TradeIndividual, nil)

// Or set it globally in config:
cfg := binance.DefaultManagerConfig()
cfg.TradeMode = binance.TradeIndividual
mgr = binance.NewManagerWithConfig(cfg)
```

| Mode | Stream | Notes |
|------|--------|-------|
| `binance.TradeAggregated` | `@aggTrade` | Trades at same price/time combined (default, lower bandwidth) |
| `binance.TradeIndividual` | `@trade` | Every individual fill (higher volume, more granular) |

### Bybit & Bitget

These exchanges have a single public trade stream each. No mode selection needed:

```go
bybitMgr.SubscribeTrade("BTCUSDT", nil)   // publicTrade stream
bitgetMgr.SubscribeTrade("BTCUSDT", nil)  // trade stream
```

---

## Candle Intervals

All exchanges default to 1-minute candles. You can change the interval:

### Binance

```go
mgr := binance.NewManager()

mgr.SubscribeCandle("BTCUSDT", nil)                             // default 1m
mgr.SubscribeCandleWithInterval("BTCUSDT", "5m", nil)           // 5-minute
mgr.SubscribeCandleWithInterval("ETHUSDT", "1h", nil)           // 1-hour
mgr.SubscribeCandleWithInterval("SOLUSDT", "4h", nil)           // 4-hour
```

Binance intervals: `"1m"`, `"3m"`, `"5m"`, `"15m"`, `"30m"`, `"1h"`, `"2h"`, `"4h"`, `"6h"`, `"8h"`, `"12h"`, `"1d"`, `"1w"`

### Bybit

```go
mgr := bybit.NewManager()

mgr.SubscribeCandleWithInterval("BTCUSDT", "5", nil)    // 5-minute
mgr.SubscribeCandleWithInterval("BTCUSDT", "60", nil)   // 1-hour
mgr.SubscribeCandleWithInterval("BTCUSDT", "D", nil)    // daily
```

Bybit intervals: `"1"`, `"3"`, `"5"`, `"15"`, `"30"`, `"60"`, `"120"`, `"240"`, `"360"`, `"720"`, `"D"`, `"W"`, `"M"`

### Bitget

```go
mgr := bitget.NewManager()

mgr.SubscribeCandleWithInterval("BTCUSDT", "5m", nil)   // 5-minute
mgr.SubscribeCandleWithInterval("BTCUSDT", "1H", nil)   // 1-hour
mgr.SubscribeCandleWithInterval("BTCUSDT", "1D", nil)   // daily
```

Bitget intervals: `"1m"`, `"5m"`, `"15m"`, `"30m"`, `"1H"`, `"4H"`, `"6H"`, `"12H"`, `"1D"`, `"1W"`

---

## Setting Defaults via Config

Instead of passing options on every call, set them once in the manager config:

```go
// Binance: 5-level depth at 100ms, individual trades, 5m candles
cfg := binance.DefaultManagerConfig()
cfg.DepthLevel = binance.Depth5
cfg.DepthSpeed = binance.Speed100ms
cfg.TradeMode  = binance.TradeIndividual
cfg.Interval   = "5m"

mgr := binance.NewManagerWithConfig(cfg)
mgr.SubscribeAll("BTCUSDT", nil, nil, nil) // uses all the config above
```

```go
// Bybit: 200-level depth, 15m candles
cfg := bybit.DefaultManagerConfig()
cfg.DepthLevel = bybit.Depth200
cfg.Interval   = "15"

mgr := bybit.NewManagerWithConfig(cfg)
```

```go
// Bitget: spot market, books5 depth
cfg := bitget.DefaultManagerConfig()
cfg.InstType     = bitget.InstSpot
cfg.DepthChannel = bitget.DepthBooks5
cfg.Interval     = "5m"

mgr := bitget.NewManagerWithConfig(cfg)
```

---

## Reading Data: Snapshots

Every symbol produces immutable snapshots every second (configurable). Read them lock-free from any goroutine.

```go
// Snapshot is an immutable, point-in-time view of a symbol's state.
type Snapshot struct {
    Timestamp  time.Time               // when the snapshot was taken
    Candles    []models.CandleHLCV     // historical + live candle bars
    DepthStore *depth.Store            // order book metrics with time-bucketed storage
    Trades     []models.NormalizedTrade // recent trades, normalized across exchanges
}
```

```go
snap := mgr.GetSnapshot("BTCUSDT")
if snap == nil {
    // No data yet
    return
}

// Candles (OHLCV)
for _, c := range snap.Candles {
    fmt.Printf("ts=%d O=%.2f H=%.2f L=%.2f C=%.2f V=%.4f\n",
        c.Ts, c.Open, c.High, c.Low, c.Close, c.Volume)
}

// Trades (normalized across exchanges)
for _, t := range snap.Trades {
    fmt.Printf("[%s] %s %.4f @ %.2f\n", t.Exchange, t.Side, t.SizeUSD, t.Price)
}

// Depth metrics
latest := snap.DepthStore.GetLatest()
if latest != nil {
    fmt.Printf("Spread: %.2f bps | Imbalance: %.3f | Mid: %.2f\n",
        latest.SpreadBps, latest.ImbalanceRatio10, latest.MidPrice)
}

// Historical depth (last 30 seconds)
recent := snap.DepthStore.GetLastNSeconds(30)
```

---

## Data Models

### CandleHLCV

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

Helper methods: `GetTimestamp()`, `HL2()`, `HLC3()`.

### CandleHLC

Lighter candle without Open/Volume — used by ATR, Bollinger, and Support/Resistance indicators.

```go
type CandleHLC struct {
    High  float64
    Low   float64
    Close float64
}
```

### NormalizedTrade

Unified trade format across all exchanges.

```go
type NormalizedTrade struct {
    Timestamp int64   // Unix milliseconds
    Price     float64
    Size      float64 // base currency
    SizeUSD   float64
    Side      string  // "buy" or "sell"
    TradeID   string
    Symbol    string  // e.g. "BTCUSDT"
    Exchange  string  // "binance", "bybit", "bitget"
}
```

---

## Custom Processing Hooks

Workers fire callbacks on every state change. Plug your own logic:

```go
worker := mgr.GetOrCreateWorker("BTCUSDT")

// Called after every candle update (same-minute update or new bar)
worker.SetOnCandleUpdate(func(candles []models.CandleHLCV) {
    if len(candles) >= 14 {
        closes := make([]float64, len(candles))
        for i, c := range candles {
            closes[i] = c.Close
        }
        rsi := indicators.CalculateRSI(closes, 14)
        fmt.Printf("RSI(14): %.2f\n", rsi)
    }
})

// Called after every depth update with the computed metrics
worker.SetOnDepthUpdate(func(m depth.DepthMetrics) {
    fmt.Printf("Bid liq: $%.0f | Ask liq: $%.0f | Spread: %.2f bps\n",
        m.BidLiquidity10, m.AskLiquidity10, m.SpreadBps)
})

// Called after every trade
worker.SetOnTradeUpdate(func(t models.NormalizedTrade) {
    if t.SizeUSD > 50000 {
        fmt.Printf("LARGE %s: $%.0f @ %.2f\n", t.Side, t.SizeUSD, t.Price)
    }
})
```

---

## Worker Tuning

```go
cfg := ws.DefaultWorkerConfig()

cfg.CandleChSize = 128           // Candle channel buffer (default 64)
cfg.DepthChSize  = 4096          // Depth channel buffer (default 2048)
cfg.TradeChSize  = 4096          // Trade channel buffer (default 2048)

cfg.MaxCandles         = 1500    // Candle history length (default 750)
cfg.MaxNormTrades      = 5000    // Normalized trade buffer (default 2000)
cfg.MaxDepthMetrics    = 20000   // Depth metric storage (default 10000)
cfg.MaxDepthSeconds    = 1800    // Keep 30 min of depth data (default 1000s)
cfg.RecentMetricsSize  = 200     // Fast-access depth buffer (default 100)

cfg.SnapshotInterval = 500 * time.Millisecond  // Snapshot frequency (default 1s)

// Pass to any exchange manager
mgr := binance.NewManagerWithConfig(binance.ManagerConfig{WorkerConfig: cfg})
```

---

## Historical Data (REST)

Fetch candles from exchange REST APIs:

```go
import "github.com/KhavrTrading/flowex/candles"

// Binance: up to 1500 per request
data, err := candles.FetchBinanceCandles("BTCUSDT", "1m", 750)
data, err := candles.FetchBinanceCandles("ETHUSDT", "5m", 500)

// Bybit: up to 200 per request
data, err := candles.FetchBybitCandles("BTCUSDT", "1", 200)

// Bitget: up to 200 per request
data, err := candles.FetchBitgetCandles("BTCUSDT", "1m", 200)

// Also available as CandleHLC (without open/volume):
hlc, err := candles.FetchBinanceCandleHLC("BTCUSDT", "1m", 750)
```

### Candle Aggregation

```go
import "github.com/KhavrTrading/flowex/candles"

oneMin, _ := candles.FetchBinanceCandles("BTCUSDT", "1m", 750)

fiveMin   := candles.Aggregate1mTo5m(oneMin)   // 1m -> 5m
fifteenMin := candles.Aggregate1mTo15m(oneMin)  // 1m -> 15m

// Custom duration (e.g., 3 minutes)
threeMin := candles.Aggregate(oneMin, 3*60*1000)
```

---

## Technical Indicators

Built-in standard indicators that work on `[]float64` or `[]models.CandleHLC`:

```go
import "github.com/KhavrTrading/flowex/indicators"

closes := []float64{100, 101, 99, 102, 103, ...}

// EMA
ema20 := indicators.CalculateEMA(closes, 20)
emaSeries := indicators.CalculateEMAList(closes, 20)  // full series

// RSI
rsi := indicators.CalculateRSI(closes, 14)

// MACD (12/26/9)
macd, signal, histogram := indicators.CalculateMACD(closes)

// Stochastic RSI
stochRSI := indicators.CalculateStochRSI(closes, 14, 14)

// ATR (needs CandleHLC with High/Low/Close)
atr := indicators.CalculateATR(hlcCandles, 14)
atr, threshold, rising, err := indicators.EvaluateATR(hlcCandles, 14, 0.02)

// Bollinger Mean Deviation
score, oscSD := indicators.BMD(hlcCandles, "1m")
score, oscSD = indicators.BollingerMeanDeviation(hlcCandles, 20, 25)

// Support/Resistance (pivot-based)
supportPct, resistancePct, srScore := indicators.SupportResistance(hlcCandles, 5, 20)
```

---

## Architecture

```
                    +-----------+
  WebSocket  -----> |  Client   |  (per-symbol connection, auto-reconnect, heartbeat)
                    +-----+-----+
                          |
                     callbacks (non-blocking)
                          |
                    +-----v-----+
                    |  Worker   |  (per-symbol actor goroutine, owns ALL state)
                    |           |
                    |  candles  |  <- channel (buf 64)
                    |  depth    |  <- channel (buf 2048)
                    |  trades   |  <- channel (buf 2048)
                    |           |
                    |  hooks    |  -> user callbacks (OnCandle, OnDepth, OnTrade)
                    |           |
                    +-----+-----+
                          |
                    atomic.Store (every 1s)
                          |
                    +-----v-----+
                    | Snapshot  |  (immutable, lock-free reads from any goroutine)
                    +-----------+
```

- **One goroutine per symbol** -- no locks needed for state mutation
- **Non-blocking enqueue** -- if channel is full, oldest message is dropped (never blocks WS read)
- **Atomic snapshots** -- readers never contend with the writer
- **Auto-reconnect** -- connection drops trigger reconnect + resubscribe to all active streams

---

## Packages

| Package | Description |
|---------|-------------|
| `ws/` | Core: BaseClient, SymbolWorker (actor), BaseManager (pool), PubSub[T], interfaces |
| `binance/` | Binance Futures adapter (depth5/10/20, aggTrade/trade, all candle intervals) |
| `bybit/` | Bybit V5 Linear adapter (depth 1/50/200/500, all candle intervals) |
| `bitget/` | Bitget V2 adapter (books/books5/books15, spot/futures, all candle intervals) |
| `models/` | CandleHLC, CandleHLCV, NormalizedTrade, TickerData |
| `depth/` | Order book metrics (75 fields) + time-bucketed store with enrichment |
| `indicators/` | EMA, RSI, ATR, MACD, StochRSI, Bollinger, Support/Resistance |
| `indicators/technical/` | Batch-optimized calculator, ADX, MMI, signal types, movement tracking |
| `candles/` | REST fetchers (Binance/Bybit/Bitget) + timeframe aggregator |

See [DOCUMENTATION.md](DOCUMENTATION.md) for the full API reference — all 75 depth metric fields, store query methods, worker monitoring, historical data seeding, and more.

## Dependencies

Only two:
- `github.com/gorilla/websocket`
- `github.com/sirupsen/logrus`

## License

MIT
