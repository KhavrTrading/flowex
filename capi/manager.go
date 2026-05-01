package main

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/KhavrTrading/flowex/binance"
	"github.com/KhavrTrading/flowex/bitget"
	"github.com/KhavrTrading/flowex/bybit"
	"github.com/KhavrTrading/flowex/depth"
	"github.com/KhavrTrading/flowex/indicators/technical"
	"github.com/KhavrTrading/flowex/models"
	"github.com/KhavrTrading/flowex/ws"
)

// managerIface is the method set every exchange manager shares through its
// embedded *ws.BaseManager. Each concrete type (binance.Manager, bybit.Manager,
// bitget.Manager) satisfies this automatically.
type managerIface interface {
	SubscribeCandle(symbol string, handler ws.CandleHandler) error
	SubscribeDepth(symbol string, handler ws.DepthHandler) error
	SubscribeTrade(symbol string, handler ws.TradeHandler) error
	SubscribeAll(symbol string, ch ws.CandleHandler, dh ws.DepthHandler, th ws.TradeHandler) error
	Unsubscribe(symbol string, st ws.StreamType) error
	UnsubscribeAll(symbol string) error
	GetSnapshot(symbol string) *ws.Snapshot
	GetStatus() map[string]any
	Shutdown()
	GetOrCreateWorker(symbol string) *ws.SymbolWorker
}

var (
	mgrMu   sync.RWMutex
	mgrMap  = map[uint64]managerIface{}
	mgrNext uint64
)

func putMgr(m managerIface) uint64 {
	h := atomic.AddUint64(&mgrNext, 1)
	mgrMu.Lock()
	mgrMap[h] = m
	mgrMu.Unlock()
	return h
}

func getMgr(h uint64) managerIface {
	mgrMu.RLock()
	m := mgrMap[h]
	mgrMu.RUnlock()
	return m
}

// Handlers are no-ops because the internal SymbolWorker already aggregates
// candles/depth/trades into the lock-free snapshot; Rust callers poll the
// snapshot instead of receiving per-event callbacks across the FFI boundary.
func noopCandle(models.CandleHLCV)                {}
func noopDepth(_, _ [][]string, _ int64)          {}
func noopTrade(models.NormalizedTrade)            {}

// -----------------------------------------------------------------------------
// Manager lifecycle
// -----------------------------------------------------------------------------

// flowex_manager_new creates a new exchange manager. `exchange` is one of
// "binance", "bybit", "bitget". Returns an opaque handle, or 0 on error.
//
//export flowex_manager_new
func flowex_manager_new(exchange *C.char) C.uint64_t {
	var m managerIface
	switch C.GoString(exchange) {
	case "binance":
		m = binance.NewManager()
	case "bybit":
		m = bybit.NewManager()
	case "bitget":
		m = bitget.NewManager()
	default:
		return 0
	}
	return C.uint64_t(putMgr(m))
}

// flowex_manager_shutdown stops all goroutines and releases the manager.
//
//export flowex_manager_shutdown
func flowex_manager_shutdown(h C.uint64_t) {
	mgrMu.Lock()
	m := mgrMap[uint64(h)]
	delete(mgrMap, uint64(h))
	mgrMu.Unlock()
	if m != nil {
		m.Shutdown()
	}
}

// -----------------------------------------------------------------------------
// Subscriptions (no-op handlers; state lives in the aggregated worker snapshot)
// -----------------------------------------------------------------------------

// flowex_manager_subscribe_candle activates the candle stream for a symbol.
// Returns 0 on success, non-zero on error.
//
//export flowex_manager_subscribe_candle
func flowex_manager_subscribe_candle(h C.uint64_t, symbol *C.char) C.int {
	m := getMgr(uint64(h))
	if m == nil {
		return 1
	}
	if err := m.SubscribeCandle(C.GoString(symbol), noopCandle); err != nil {
		return 2
	}
	return 0
}

//export flowex_manager_subscribe_depth
func flowex_manager_subscribe_depth(h C.uint64_t, symbol *C.char) C.int {
	m := getMgr(uint64(h))
	if m == nil {
		return 1
	}
	if err := m.SubscribeDepth(C.GoString(symbol), noopDepth); err != nil {
		return 2
	}
	return 0
}

//export flowex_manager_subscribe_trade
func flowex_manager_subscribe_trade(h C.uint64_t, symbol *C.char) C.int {
	m := getMgr(uint64(h))
	if m == nil {
		return 1
	}
	if err := m.SubscribeTrade(C.GoString(symbol), noopTrade); err != nil {
		return 2
	}
	return 0
}

// flowex_manager_subscribe_all activates candle + depth + trade streams at once.
//
//export flowex_manager_subscribe_all
func flowex_manager_subscribe_all(h C.uint64_t, symbol *C.char) C.int {
	m := getMgr(uint64(h))
	if m == nil {
		return 1
	}
	if err := m.SubscribeAll(C.GoString(symbol), noopCandle, noopDepth, noopTrade); err != nil {
		return 2
	}
	return 0
}

// flowex_manager_unsubscribe deactivates a single stream.
// `streamType` is "candle" | "depth" | "trade".
//
//export flowex_manager_unsubscribe
func flowex_manager_unsubscribe(h C.uint64_t, symbol, streamType *C.char) C.int {
	m := getMgr(uint64(h))
	if m == nil {
		return 1
	}
	if err := m.Unsubscribe(C.GoString(symbol), ws.StreamType(C.GoString(streamType))); err != nil {
		return 2
	}
	return 0
}

//export flowex_manager_unsubscribe_all
func flowex_manager_unsubscribe_all(h C.uint64_t, symbol *C.char) C.int {
	m := getMgr(uint64(h))
	if m == nil {
		return 1
	}
	if err := m.UnsubscribeAll(C.GoString(symbol)); err != nil {
		return 2
	}
	return 0
}

// flowex_manager_seed_candles bulk-loads historical candles into a symbol's
// worker so indicator computation has a warm context.
//
//export flowex_manager_seed_candles
func flowex_manager_seed_candles(h C.uint64_t, symbol, candlesJSON *C.char) C.int {
	m := getMgr(uint64(h))
	if m == nil {
		return 1
	}
	var in []models.CandleHLCV
	if err := json.Unmarshal([]byte(C.GoString(candlesJSON)), &in); err != nil {
		return 2
	}
	worker := m.GetOrCreateWorker(C.GoString(symbol))
	worker.SeedCandles(in)
	return 0
}

// -----------------------------------------------------------------------------
// Snapshot — the lock-free "give me everything now" endpoint
// -----------------------------------------------------------------------------

// snapshotDTO is the wire shape of a full symbol snapshot. Mirrored 1:1 on
// the Rust side as `Snapshot`. Explicit DTO because ws.Snapshot has no
// json tags and embeds live pointers we don't want to marshal.
type snapshotDTO struct {
	TimestampMs  int64                            `json:"timestamp_ms"`
	Candles      []models.CandleHLCV              `json:"candles"`
	DepthLatest  *depth.DepthMetrics              `json:"depth_latest"`
	Trades       []models.NormalizedTrade         `json:"trades"`
	Indicators   *technical.TechnicalIndicators   `json:"indicators"`
}

// flowex_manager_get_snapshot returns the current aggregated state for a
// subscribed symbol as JSON. The underlying read is lock-free — suitable to
// poll at any cadence. Returns "null" if the symbol has no worker yet.
// Caller must free.
//
//export flowex_manager_get_snapshot
func flowex_manager_get_snapshot(h C.uint64_t, symbol *C.char) *C.char {
	m := getMgr(uint64(h))
	if m == nil {
		return C.CString("null")
	}
	snap := m.GetSnapshot(C.GoString(symbol))
	if snap == nil {
		return C.CString("null")
	}
	dto := snapshotDTO{
		TimestampMs: snap.Timestamp.UnixMilli(),
		Candles:     snap.Candles,
		Trades:      snap.Trades,
		Indicators:  snap.Indicators,
	}
	if snap.DepthStore != nil {
		dto.DepthLatest = snap.DepthStore.GetLatest()
	}
	return marshalC(dto)
}

// flowex_manager_get_status returns the manager's status map as JSON.
// Caller must free.
//
//export flowex_manager_get_status
func flowex_manager_get_status(h C.uint64_t) *C.char {
	m := getMgr(uint64(h))
	if m == nil {
		return C.CString("null")
	}
	return marshalC(m.GetStatus())
}
