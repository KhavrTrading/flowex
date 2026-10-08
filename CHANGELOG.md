# Changelog

## Unreleased (v0.4.2)

### Fixes
- **Binance**: aggTrade `TradeID` now reads the aggregate id `a` (was `t`, which aggTrade lacks, so every id was `"0"`); plain `trade` events still read `t`.

## v0.1.0 (2026-03-26)

Initial public release.

### Features
- **Binance Futures** WebSocket adapter (depth 5/10/20, aggTrade/trade, all candle intervals)
- **Bybit V5 Linear** WebSocket adapter (depth 1/50/200/500, all candle intervals)
- **Bitget V2** WebSocket adapter (books/books5/books15, spot/futures, all candle intervals)
- Per-symbol actor workers with atomic snapshots
- Order book depth metrics (100+ fields) with time-bucketed store
- Normalized trade model across exchanges
- Technical indicators: EMA, RSI, ATR, MACD, StochRSI, Bollinger, Support/Resistance
- REST candle fetchers for Binance, Bybit, Bitget with timeframe aggregation
- Auto-reconnect with resubscribe on connection drops
- Non-blocking channel enqueue (drop oldest on overflow)
