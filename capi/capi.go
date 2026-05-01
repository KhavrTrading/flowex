// Package main is the cgo entry point that exposes flowex as a C-ABI shared
// library. Build with:
//
//	go build -buildmode=c-shared -o flowex.dll ./capi
//
// Rules of the boundary:
//   - Every function returning a *C.char returns a heap-allocated C string that
//     the caller must free via flowex_free_string. Never free it with any other
//     allocator — the Go DLL and the caller may link different C runtimes.
//   - Structured results cross as JSON strings. The Rust side deserializes with
//     serde into typed structs that mirror the json tags on the Go models.
//   - Stateful objects (e.g. depth.Store) are kept in a Go-side registry and
//     addressed by an opaque uint64 handle. The caller owns the handle
//     lifetime and must release it via the matching *_free function.
package main

/*
#include <stdlib.h>
#include <stdint.h>
#include <string.h>

typedef uint64_t flowex_uint64_t;
*/
import "C"

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/KhavrTrading/flowex/depth"
	"github.com/KhavrTrading/flowex/indicators"
)

// main is required for -buildmode=c-shared but is never invoked.
func main() {}

// -----------------------------------------------------------------------------
// String/memory helpers
// -----------------------------------------------------------------------------

// flowex_free_string releases a C string previously returned by any flowex
// function. Calling it on nil is a no-op.
//
//export flowex_free_string
func flowex_free_string(s *C.char) {
	if s != nil {
		C.free(unsafe.Pointer(s))
	}
}

// marshalC marshals v to JSON and returns a freshly allocated C string.
// On marshal error it returns a JSON error object so the Rust side can parse
// a uniform shape.
func marshalC(v any) *C.char {
	buf, err := json.Marshal(v)
	if err != nil {
		return C.CString(`{"error":"marshal_failed"}`)
	}
	return C.CString(string(buf))
}

// -----------------------------------------------------------------------------
// Stateless indicators (scalar return shape)
// -----------------------------------------------------------------------------

// flowex_rsi computes RSI over the given close prices. Pass a pointer to the
// first element of a contiguous float64 array plus its length.
//
//export flowex_rsi
func flowex_rsi(closes *C.double, n C.size_t, period C.int) C.double {
	if n == 0 || closes == nil {
		return 0
	}
	slice := unsafe.Slice((*float64)(unsafe.Pointer(closes)), int(n))
	return C.double(indicators.CalculateRSI(slice, int(period)))
}

// flowex_ema computes a single EMA value over the given close prices.
//
//export flowex_ema
func flowex_ema(prices *C.double, n C.size_t, period C.int) C.double {
	if n == 0 || prices == nil {
		return 0
	}
	slice := unsafe.Slice((*float64)(unsafe.Pointer(prices)), int(n))
	return C.double(indicators.CalculateEMA(slice, int(period)))
}

// -----------------------------------------------------------------------------
// Depth metrics (JSON return shape)
// -----------------------------------------------------------------------------

// flowex_compute_depth_metrics computes DepthMetrics from a raw snapshot.
//
// bidsJSON and asksJSON are expected to be JSON arrays of [price, qty] string
// pairs, matching the exchange wire format used throughout flowex.
//
// Returns a JSON string describing DepthMetrics. Caller must free.
//
//export flowex_compute_depth_metrics
func flowex_compute_depth_metrics(symbol *C.char, timestampMs C.longlong, bidsJSON, asksJSON *C.char) *C.char {
	sym := C.GoString(symbol)

	var bids, asks [][]string
	if err := json.Unmarshal([]byte(C.GoString(bidsJSON)), &bids); err != nil {
		return C.CString(`{"error":"bids_json_invalid"}`)
	}
	if err := json.Unmarshal([]byte(C.GoString(asksJSON)), &asks); err != nil {
		return C.CString(`{"error":"asks_json_invalid"}`)
	}

	m := depth.ComputeDepthMetrics(sym, int64(timestampMs), bids, asks)
	return marshalC(m)
}

// -----------------------------------------------------------------------------
// Depth store (opaque handle shape)
// -----------------------------------------------------------------------------

var (
	storeMu   sync.RWMutex
	storeMap  = map[uint64]*depth.Store{}
	storeNext uint64
)

func putStore(s *depth.Store) uint64 {
	h := atomic.AddUint64(&storeNext, 1)
	storeMu.Lock()
	storeMap[h] = s
	storeMu.Unlock()
	return h
}

func getStore(h uint64) *depth.Store {
	storeMu.RLock()
	s := storeMap[h]
	storeMu.RUnlock()
	return s
}

// flowex_store_new creates a new depth.Store. recentCap <= 0 uses the default.
// Returns an opaque handle; 0 means error.
//
//export flowex_store_new
func flowex_store_new(recentCap C.int) C.uint64_t {
	var s *depth.Store
	if recentCap > 0 {
		s = depth.NewStoreWithCap(int(recentCap))
	} else {
		s = depth.NewStore()
	}
	return C.uint64_t(putStore(s))
}

// flowex_store_free releases a store handle. No-op if the handle is unknown.
//
//export flowex_store_free
func flowex_store_free(h C.uint64_t) {
	storeMu.Lock()
	delete(storeMap, uint64(h))
	storeMu.Unlock()
}

// flowex_store_add inserts a DepthMetrics JSON document into the store.
// Returns 0 on success, non-zero on error.
//
//export flowex_store_add
func flowex_store_add(h C.uint64_t, metricsJSON *C.char, maxMetrics, maxSeconds C.int) C.int {
	s := getStore(uint64(h))
	if s == nil {
		return 1
	}
	var m depth.DepthMetrics
	if err := json.Unmarshal([]byte(C.GoString(metricsJSON)), &m); err != nil {
		return 2
	}
	s.Add(m, int(maxMetrics), int(maxSeconds))
	return 0
}

// flowex_store_get_latest returns the most recent DepthMetrics as JSON, or
// "null" if the store is empty / handle unknown. Caller must free.
//
//export flowex_store_get_latest
func flowex_store_get_latest(h C.uint64_t) *C.char {
	s := getStore(uint64(h))
	if s == nil {
		return C.CString("null")
	}
	m := s.GetLatest()
	if m == nil {
		return C.CString("null")
	}
	return marshalC(m)
}

// flowex_store_get_recent_n returns the N most recent DepthMetrics as a JSON
// array (oldest first). Caller must free.
//
//export flowex_store_get_recent_n
func flowex_store_get_recent_n(h C.uint64_t, n C.int) *C.char {
	s := getStore(uint64(h))
	if s == nil {
		return C.CString("[]")
	}
	return marshalC(s.GetRecentN(int(n)))
}

// flowex_store_size returns the number of metrics currently in the store.
// Returns -1 for an unknown handle.
//
//export flowex_store_size
func flowex_store_size(h C.uint64_t) C.int {
	s := getStore(uint64(h))
	if s == nil {
		return -1
	}
	return C.int(s.Size())
}
