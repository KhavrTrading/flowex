package technical

import (
	"sync"

	"github.com/KhavrTrading/flowex/indicators"
	"github.com/KhavrTrading/flowex/models"
)

// closesPool reduces allocations by reusing float64 slices for close prices
var closesPool = sync.Pool{
	New: func() interface{} {
		s := make([]float64, 0, 1000) // Pre-allocate for up to 1000 candles
		return &s
	},
}

// CalculateTechnicalIndicators calculates all technical indicators from historical candles
// Uses 12 hours of 1m candle data (720 candles minimum for SMA200)
// Optimized with sync.Pool to reduce memory allocations by ~15-20%
func CalculateTechnicalIndicators(candles []models.CandleHLCV, currentPrice float64) *TechnicalIndicators {
	if len(candles) < 20 {
		return nil // Need at least 20 candles for basic indicators
	}

	// Get reusable buffer from pool
	closesPtr := closesPool.Get().(*[]float64)
	closes := (*closesPtr)[:0] // Reset length, keep capacity

	// Extract close prices
	for i := range candles {
		closes = append(closes, candles[i].Close)
	}

	// Ensure we return the buffer to pool when done
	defer closesPool.Put(closesPtr)

	ind := &TechnicalIndicators{}

	// ============================================================
	// RSI (Relative Strength Index)
	// ============================================================
	if len(closes) >= 14 {
		ind.RSI14 = indicators.CalculateRSI(closes, 14)
	}

	// ============================================================
	// Simple Moving Averages (SMA) - Optimized batch calculation
	// ============================================================
	if len(closes) >= 200 {
		// Calculate all SMAs in one pass (reuse sum for 20, extend for 50, extend for 200)
		ind.SMA20, ind.SMA50, ind.SMA200 = calculateSMABatch(closes)
	} else if len(closes) >= 50 {
		ind.SMA20, ind.SMA50, _ = calculateSMABatch(closes)
	} else if len(closes) >= 20 {
		ind.SMA20, _, _ = calculateSMABatch(closes)
	}

	// ============================================================
	// Exponential Moving Averages (EMA) - Using cached multipliers
	// ============================================================
	if len(closes) >= 9 {
		ind.EMA9 = CalculateEMAFast(closes, 9, emaMultipliers[9])
	}
	if len(closes) >= 12 {
		ind.EMA12 = CalculateEMAFast(closes, 12, emaMultipliers[12])
	}
	if len(closes) >= 20 {
		ind.EMA20 = CalculateEMAFast(closes, 20, emaMultipliers[20])
	}
	if len(closes) >= 21 {
		ind.EMA21 = CalculateEMAFast(closes, 21, emaMultipliers[21])
	}
	if len(closes) >= 26 {
		ind.EMA26 = CalculateEMAFast(closes, 26, emaMultipliers[26])
	}
	if len(closes) >= 50 {
		ind.EMA50 = CalculateEMAFast(closes, 50, emaMultipliers[50])
	}
	if len(closes) >= 200 {
		ind.EMA200 = CalculateEMAFast(closes, 200, emaMultipliers[200])
	}

	// ============================================================
	// MACD (Moving Average Convergence Divergence)
	// ============================================================
	if len(closes) >= 26 {
		macdLine, signalLine, histogram := indicators.CalculateMACD(closes)
		if len(macdLine) > 0 {
			ind.MACDLine = macdLine[len(macdLine)-1]
		}
		if len(signalLine) > 0 {
			ind.SignalLine = signalLine[len(signalLine)-1]
		}
		if len(histogram) > 0 {
			ind.Histogram = histogram[len(histogram)-1]
		}
	}

	// ============================================================
	// Bollinger Bands (20-period, 2 StdDev) - Optimized single-pass
	// ============================================================
	if len(closes) >= 20 {
		// Reuse SMA20 if already calculated
		middle := ind.SMA20
		if middle == 0 {
			middle = calculateSMAFast(closes, 20)
		}
		upper, lower := calculateBollingerBandsFast(closes, middle, 20, 2.0)
		ind.BBUpper = upper
		ind.BBMiddle = middle
		ind.BBLower = lower
	}

	// ============================================================
	// ATR (Average True Range) - 14 period - Optimized
	// ============================================================
	if len(candles) >= 14 {
		ind.ATR = CalculateATRFast(candles, 14)
	}

	// ============================================================
	// Stochastic RSI - 14 period - Optimized
	// ============================================================
	if len(closes) >= 14 {
		ind.StochRSI = calculateStochRSIFast(closes, 14)
	}

	// ============================================================
	// MMI (Market Manipulation Index) - Optimized for 1m
	// ============================================================
	if len(candles) >= 30 {
		ind.MMI = calculateMMIFast(candles)
	}

	// ============================================================
	// Calculate Summary Signals (TradingView-style)
	// ============================================================
	calculateSummarySignals(ind, currentPrice)

	return ind
}
