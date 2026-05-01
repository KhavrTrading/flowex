package main

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"encoding/json"
	"unsafe"

	"github.com/KhavrTrading/flowex/indicators"
	"github.com/KhavrTrading/flowex/indicators/technical"
	"github.com/KhavrTrading/flowex/models"
)

// -----------------------------------------------------------------------------
// Simple indicators (scalar / small-struct returns)
// -----------------------------------------------------------------------------

// flowex_macd computes MACD on a close-price series. Returns a JSON object
// {"macd":[...], "signal":[...], "histogram":[...]}. Caller must free.
//
//export flowex_macd
func flowex_macd(prices *C.double, n C.size_t) *C.char {
	if n == 0 || prices == nil {
		return C.CString(`{"macd":[],"signal":[],"histogram":[]}`)
	}
	slice := unsafe.Slice((*float64)(unsafe.Pointer(prices)), int(n))
	macdLine, signalLine, hist := indicators.CalculateMACD(slice)
	return marshalC(map[string]any{
		"macd":      macdLine,
		"signal":    signalLine,
		"histogram": hist,
	})
}

// flowex_stoch_rsi computes Stochastic RSI series. Returns a JSON array of
// floats. Caller must free.
//
//export flowex_stoch_rsi
func flowex_stoch_rsi(closes *C.double, n C.size_t, rsiPeriod, stochPeriod C.int) *C.char {
	if n == 0 || closes == nil {
		return C.CString(`[]`)
	}
	slice := unsafe.Slice((*float64)(unsafe.Pointer(closes)), int(n))
	return marshalC(indicators.CalculateStochRSI(slice, int(rsiPeriod), int(stochPeriod)))
}

// -----------------------------------------------------------------------------
// HLC-based indicators (ATR, Bollinger, support/resistance)
// -----------------------------------------------------------------------------

// hlcInput is the wire shape for functions that operate on HLC candles.
type hlcInput struct {
	Ts    int64   `json:"ts"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
	Close float64 `json:"close"`
}

func parseHLCArray(jsonStr string) ([]models.CandleHLC, error) {
	var in []hlcInput
	if err := json.Unmarshal([]byte(jsonStr), &in); err != nil {
		return nil, err
	}
	out := make([]models.CandleHLC, len(in))
	for i, c := range in {
		out[i] = models.NewCandleHLC(c.Ts, c.High, c.Low, c.Close)
	}
	return out, nil
}

// flowex_atr computes ATR(period) from a JSON array of HLC candles:
//
//	[{"ts":..,"high":..,"low":..,"close":..}, ...]
//
//export flowex_atr
func flowex_atr(candlesJSON *C.char, period C.int) C.double {
	candles, err := parseHLCArray(C.GoString(candlesJSON))
	if err != nil {
		return 0
	}
	return C.double(indicators.CalculateATR(candles, int(period)))
}

// flowex_bollinger returns Bollinger mean-deviation score + oscillator SD
// as JSON {"score":..,"osc_sd":..}.
//
//export flowex_bollinger
func flowex_bollinger(candlesJSON *C.char, baseLen, devLen C.int) *C.char {
	candles, err := parseHLCArray(C.GoString(candlesJSON))
	if err != nil {
		return C.CString(`{"error":"candles_json_invalid"}`)
	}
	score, oscSD := indicators.BollingerMeanDeviation(candles, int(baseLen), int(devLen))
	return marshalC(map[string]float64{"score": score, "osc_sd": oscSD})
}

// flowex_support_resistance returns {support_pct, resistance_pct, score}.
//
//export flowex_support_resistance
func flowex_support_resistance(candlesJSON *C.char, lookback, retWindow C.int) *C.char {
	candles, err := parseHLCArray(C.GoString(candlesJSON))
	if err != nil {
		return C.CString(`{"error":"candles_json_invalid"}`)
	}
	sup, res, score := indicators.SupportResistance(candles, int(lookback), int(retWindow))
	return marshalC(map[string]float64{
		"support_pct":    sup,
		"resistance_pct": res,
		"score":          score,
	})
}

// -----------------------------------------------------------------------------
// Full technical-indicator bundle (the big one)
// -----------------------------------------------------------------------------

// flowex_technical_indicators runs the full bundled indicator computation over
// a JSON array of OHLCV candles. Returns technical.TechnicalIndicators as JSON.
//
//export flowex_technical_indicators
func flowex_technical_indicators(candlesJSON *C.char, currentPrice C.double) *C.char {
	var candles []models.CandleHLCV
	if err := json.Unmarshal([]byte(C.GoString(candlesJSON)), &candles); err != nil {
		return C.CString(`{"error":"candles_json_invalid"}`)
	}
	ind := technical.CalculateTechnicalIndicators(candles, float64(currentPrice))
	if ind == nil {
		return C.CString(`null`)
	}
	return marshalC(ind)
}
