package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"

	"github.com/KhavrTrading/flowex/candles"
	"github.com/KhavrTrading/flowex/models"
)

// flowex_fetch_candles pulls historical OHLCV candles from an exchange REST
// endpoint. Supported exchanges: "binance", "bybit", "bitget".
//
// Returns a JSON array of {ts, open, high, low, close, volume} on success, or
// {"error":"..."} on failure. Caller must free.
//
//export flowex_fetch_candles
func flowex_fetch_candles(exchange, symbol, interval *C.char, limit C.int) *C.char {
	ex := C.GoString(exchange)
	sym := C.GoString(symbol)
	iv := C.GoString(interval)
	lim := int(limit)

	var (
		out []models.CandleHLCV
		err error
	)
	switch ex {
	case "binance":
		out, err = candles.FetchBinanceCandles(sym, iv, lim)
	case "bybit":
		out, err = candles.FetchBybitCandles(sym, iv, lim)
	case "bitget":
		out, err = candles.FetchBitgetCandles(sym, iv, lim)
	default:
		return C.CString(`{"error":"unknown_exchange"}`)
	}
	if err != nil {
		return C.CString(`{"error":"fetch_failed"}`)
	}
	return marshalC(out)
}

// flowex_aggregate_candles aggregates 1m (or finer) OHLCV candles into bars of
// `durationMs`. Accepts a JSON array matching CandleHLCV; returns the same
// shape. Caller must free.
//
//export flowex_aggregate_candles
func flowex_aggregate_candles(candlesJSON *C.char, durationMs C.longlong) *C.char {
	var in []models.CandleHLCV
	if err := json.Unmarshal([]byte(C.GoString(candlesJSON)), &in); err != nil {
		return C.CString(`{"error":"candles_json_invalid"}`)
	}
	return marshalC(candles.Aggregate(in, int64(durationMs)))
}
