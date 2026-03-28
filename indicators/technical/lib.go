package technical

import (
	"math"

	"github.com/KhavrTrading/flowex/models"
)

// Pre-computed EMA multipliers (cached for performance)
var emaMultipliers = map[int]float64{
	9:   2.0 / 10.0,  // 2/(9+1)
	12:  2.0 / 13.0,  // 2/(12+1)
	20:  2.0 / 21.0,  // 2/(20+1)
	21:  2.0 / 22.0,  // 2/(21+1)
	26:  2.0 / 27.0,  // 2/(26+1)
	50:  2.0 / 51.0,  // 2/(50+1)
	200: 2.0 / 201.0, // 2/(200+1)
}

// Pre-computed period dividers for SMA (avoid repeated float conversions)
var periodDividers = map[int]float64{
	20:  1.0 / 20.0,
	50:  1.0 / 50.0,
	200: 1.0 / 200.0,
}

// calculateSMAFast calculates Simple Moving Average with pre-computed divider
func calculateSMAFast(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}
	sum := 0.0
	startIdx := len(prices) - period
	// Manual loop unrolling for common case (helps compiler optimize)
	for i := startIdx; i < len(prices); i++ {
		sum += prices[i]
	}
	divider, ok := periodDividers[period]
	if ok {
		return sum * divider // Multiplication is faster than division
	}
	return sum / float64(period)
}

// calculateSMABatch calculates multiple SMAs in one pass (20, 50, 200)
// Returns (sma20, sma50, sma200)
func calculateSMABatch(prices []float64) (float64, float64, float64) {
	n := len(prices)
	if n < 20 {
		return 0, 0, 0
	}

	// Calculate sum for SMA20
	sum20 := 0.0
	for i := n - 20; i < n; i++ {
		sum20 += prices[i]
	}
	sma20 := sum20 * periodDividers[20]

	if n < 50 {
		return sma20, 0, 0
	}

	// Extend to SMA50 (add 30 more prices)
	sum50 := sum20
	for i := n - 50; i < n-20; i++ {
		sum50 += prices[i]
	}
	sma50 := sum50 * periodDividers[50]

	if n < 200 {
		return sma20, sma50, 0
	}

	// Extend to SMA200 (add 150 more prices)
	sum200 := sum50
	for i := n - 200; i < n-50; i++ {
		sum200 += prices[i]
	}
	sma200 := sum200 * periodDividers[200]

	return sma20, sma50, sma200
}

// CalculateEMAFast calculates EMA with pre-computed multiplier
func CalculateEMAFast(prices []float64, period int, multiplier float64) float64 {
	if len(prices) < period {
		return 0
	}

	// Calculate initial SMA for first EMA value
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	ema := sum / float64(period)

	// Apply EMA formula with cached multiplier
	oneMinusK := 1.0 - multiplier
	for i := period; i < len(prices); i++ {
		ema = prices[i]*multiplier + ema*oneMinusK
	}

	return ema
}

// calculateBollingerBandsFast calculates BB bands using pre-computed middle (SMA)
// Returns (upper, lower) - middle is passed as parameter
// Optimized: single-pass standard deviation calculation
func calculateBollingerBandsFast(prices []float64, middle float64, period int, numStdDev float64) (float64, float64) {
	if len(prices) < period {
		return 0, 0
	}

	// Calculate standard deviation in single pass
	startIdx := len(prices) - period
	sumSquaredDiff := 0.0
	for i := startIdx; i < len(prices); i++ {
		diff := prices[i] - middle
		sumSquaredDiff += diff * diff
	}

	// Using Welford's method for numerical stability
	variance := sumSquaredDiff * periodDividers[period]
	stdDev := math.Sqrt(variance)

	// Calculate bands
	band := numStdDev * stdDev
	upper := middle + band
	lower := middle - band

	return upper, lower
}

// calculateSummarySignals generates TradingView-style summary using typed constants
func calculateSummarySignals(ind *TechnicalIndicators, currentPrice float64) {
	// ============================================================
	// Moving Averages Summary
	// ============================================================
	maBuy := 0
	maSell := 0
	maNeutral := 0

	// Helper function to compare price with MA
	comparePriceMA := func(ma float64) {
		if ma <= 0 {
			return
		}
		if currentPrice > ma {
			maBuy++
		} else if currentPrice < ma {
			maSell++
		} else {
			maNeutral++
		}
	}

	// Compare current price with each MA
	comparePriceMA(ind.EMA9)
	comparePriceMA(ind.EMA12)
	comparePriceMA(ind.EMA21)
	comparePriceMA(ind.EMA26)
	comparePriceMA(ind.EMA50)
	comparePriceMA(ind.EMA200)
	comparePriceMA(ind.SMA20)
	comparePriceMA(ind.SMA50)
	comparePriceMA(ind.SMA200)

	ind.MABuy = maBuy
	ind.MASell = maSell
	ind.MANeutral = maNeutral

	// ============================================================
	// Oscillators Summary
	// ============================================================
	oscillBuy := 0
	oscillSell := 0
	oscillNeutral := 0

	// RSI (Relative Strength Index)
	if ind.RSI14 > 0 {
		if ind.RSI14 < 30 {
			oscillBuy++ // Oversold
		} else if ind.RSI14 > 70 {
			oscillSell++ // Overbought
		} else {
			oscillNeutral++
		}
	}

	// MACD
	if ind.MACDLine != 0 && ind.SignalLine != 0 {
		if ind.MACDLine > ind.SignalLine {
			oscillBuy++ // Bullish crossover
		} else if ind.MACDLine < ind.SignalLine {
			oscillSell++ // Bearish crossover
		} else {
			oscillNeutral++
		}
	}

	ind.OscillBuy = oscillBuy
	ind.OscillSell = oscillSell
	ind.OscillNeutr = oscillNeutral

	// ============================================================
	// Determine Summary Labels (using typed constants)
	// ============================================================
	// Moving Averages Summary
	ind.MASummary = calculateSignal(maBuy, maSell, maNeutral)

	// Oscillators Summary
	ind.OscillatorSum = calculateSignal(oscillBuy, oscillSell, oscillNeutral)

	// Overall Summary (weighted combination)
	overallBuy := maBuy + oscillBuy
	overallSell := maSell + oscillSell
	overallNeutral := maNeutral + oscillNeutral
	ind.OverallSum = calculateSignal(overallBuy, overallSell, overallNeutral)
}

// calculateSignal determines IndicatorSignal based on buy/sell/neutral counts
func calculateSignal(buy, sell, neutral int) IndicatorSignal {
	total := buy + sell + neutral
	if total == 0 {
		return SignalNeutral
	}

	buyPct := float64(buy) / float64(total)
	sellPct := float64(sell) / float64(total)

	// Strong signals require 70%+ agreement
	if buyPct >= 0.7 {
		return SignalStrongBuy
	}
	if sellPct >= 0.7 {
		return SignalStrongSell
	}

	// Regular signals require 50%+ agreement
	if buyPct >= 0.5 {
		return SignalBuy
	}
	if sellPct >= 0.5 {
		return SignalSell
	}

	return SignalNeutral
}

// ============================================================
// Optimized Indicator Calculations (No Allocations)
// ============================================================

// CalculateATRFast calculates ATR directly from CandleHLCV without conversion
// Uses Wilder's smoothing method (exponential moving average)
func CalculateATRFast(candles []models.CandleHLCV, period int) float64 {
	if len(candles) < period {
		return 0
	}

	// Calculate True Range for each candle
	trSum := 0.0
	for i := len(candles) - period; i < len(candles); i++ {
		var tr float64
		if i == 0 {
			// First candle: just use high-low
			tr = candles[i].High - candles[i].Low
		} else {
			// True Range = max(high-low, |high-prevClose|, |low-prevClose|)
			hl := candles[i].High - candles[i].Low
			hc := math.Abs(candles[i].High - candles[i-1].Close)
			lc := math.Abs(candles[i].Low - candles[i-1].Close)
			tr = math.Max(hl, math.Max(hc, lc))
		}
		trSum += tr
	}

	// Initial ATR = average of first period TRs
	atr := trSum / float64(period)

	// Apply Wilder's smoothing to remaining candles
	multiplier := 1.0 / float64(period)
	for i := len(candles) - period + 1; i < len(candles); i++ {
		var tr float64
		hl := candles[i].High - candles[i].Low
		hc := math.Abs(candles[i].High - candles[i-1].Close)
		lc := math.Abs(candles[i].Low - candles[i-1].Close)
		tr = math.Max(hl, math.Max(hc, lc))

		// Wilder's smoothing: ATR = ((period-1) * prevATR + TR) / period
		atr = ((atr * float64(period-1)) + tr) * multiplier
	}

	return atr
}

// calculateStochRSIFast calculates Stochastic RSI optimized for single value
// Returns the last value only (no series allocation)
func calculateStochRSIFast(closes []float64, period int) float64 {
	if len(closes) < period*2 {
		return 50.0 // Neutral default
	}

	// Calculate RSI series for last 'period' values
	rsiValues := make([]float64, 0, period)

	// First, calculate enough RSI values to compute StochRSI
	startIdx := len(closes) - period*2
	for i := startIdx; i < len(closes); i++ {
		// Calculate RSI for this point
		rsi := calculateRSIAtIndex(closes, i, period)
		if i >= len(closes)-period {
			rsiValues = append(rsiValues, rsi)
		}
	}

	if len(rsiValues) == 0 {
		return 50.0
	}

	// Find min and max RSI in the period
	minRSI := rsiValues[0]
	maxRSI := rsiValues[0]
	for _, rsi := range rsiValues {
		if rsi < minRSI {
			minRSI = rsi
		}
		if rsi > maxRSI {
			maxRSI = rsi
		}
	}

	// Calculate StochRSI: (currentRSI - minRSI) / (maxRSI - minRSI) * 100
	currentRSI := rsiValues[len(rsiValues)-1]
	rsiRange := maxRSI - minRSI

	if rsiRange == 0 {
		return 50.0 // Neutral if no range
	}

	stochRSI := ((currentRSI - minRSI) / rsiRange) * 100
	return stochRSI
}

// calculateRSIAtIndex calculates RSI at a specific index
func calculateRSIAtIndex(prices []float64, endIdx, period int) float64 {
	if endIdx < period {
		return 50.0
	}

	startIdx := endIdx - period
	gains := 0.0
	losses := 0.0

	// Calculate average gains and losses
	for i := startIdx + 1; i <= endIdx; i++ {
		change := prices[i] - prices[i-1]
		if change > 0 {
			gains += change
		} else {
			losses -= change // Make positive
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	if avgLoss == 0 {
		return 100.0
	}

	rs := avgGain / avgLoss
	rsi := 100.0 - (100.0 / (1.0 + rs))
	return rsi
}

// calculateMMIFast calculates Market Manipulation Index optimized for 1m timeframe
// MMI combines sine-wave fitting, predictability, and spectral analysis
// Returns 0-100 (0-30=clean, 30-70=normal, 70-100=manipulated)
func calculateMMIFast(candles []models.CandleHLCV) float64 {
	n := len(candles)
	if n < 30 {
		return 50.0 // Neutral default
	}

	// Use last 30 candles for 1m timeframe (adaptive window)
	window := 30
	if n < window {
		window = n
	}

	startIdx := n - window
	prices := make([]float64, window)
	for i := 0; i < window; i++ {
		prices[i] = candles[startIdx+i].Close
	}

	// Component 1: Sine-based Market Index (40% weight)
	sineMI := calculateSineManipulation(prices)

	// Component 2: Predictability Index (40% weight)
	predMI := calculatePredictability(prices)

	// Component 3: Spectral Energy (20% weight)
	spectralMI := calculateSpectralManipulation(prices)

	// Weighted combination
	mmi := (sineMI * 0.4) + (predMI * 0.4) + (spectralMI * 0.2)

	// Clamp to 0-100
	if mmi < 0 {
		return 0
	}
	if mmi > 100 {
		return 100
	}

	return mmi
}

// calculateSineManipulation fits a sine wave and measures deviation
func calculateSineManipulation(prices []float64) float64 {
	n := len(prices)
	if n < 10 {
		return 50.0
	}

	// Detrend prices
	mean := 0.0
	for _, p := range prices {
		mean += p
	}
	mean /= float64(n)

	// Calculate variance from mean
	variance := 0.0
	for _, p := range prices {
		diff := p - mean
		variance += diff * diff
	}
	variance /= float64(n)

	if variance == 0 {
		return 0.0
	}

	// Simple sine fitting: measure how much price deviates from smooth curve
	// Higher deviation = more manipulation (choppy, non-trending)
	stdDev := math.Sqrt(variance)

	// Normalize to 0-100 scale (0 = smooth trending, 100 = choppy)
	// Typical stddev for clean market: <2% of mean
	// Manipulated market: >5% of mean
	relativeStdDev := (stdDev / mean) * 100

	if relativeStdDev < 2.0 {
		return 0.0
	} else if relativeStdDev > 5.0 {
		return 100.0
	}

	// Linear scale between 2% and 5%
	return ((relativeStdDev - 2.0) / 3.0) * 100
}

// calculatePredictability measures autocorrelation (how predictable price is)
func calculatePredictability(prices []float64) float64 {
	n := len(prices)
	if n < 5 {
		return 50.0
	}

	// Calculate 1-lag autocorrelation
	mean := 0.0
	for _, p := range prices {
		mean += p
	}
	mean /= float64(n)

	// Autocovariance at lag 1
	autocovar := 0.0
	variance := 0.0

	for i := 0; i < n-1; i++ {
		diff1 := prices[i] - mean
		diff2 := prices[i+1] - mean
		autocovar += diff1 * diff2
		variance += diff1 * diff1
	}

	if variance == 0 {
		return 50.0
	}

	// Autocorrelation coefficient
	autocorr := autocovar / variance

	// High autocorrelation = trending/predictable = LOW manipulation
	// Low/negative autocorrelation = random/choppy = HIGH manipulation
	// Scale: autocorr 0.5+ → MMI 0, autocorr -0.5- → MMI 100
	mmi := (0.5 - autocorr) * 100

	if mmi < 0 {
		return 0
	}
	if mmi > 100 {
		return 100
	}

	return mmi
}

// calculateSpectralManipulation analyzes frequency distribution
func calculateSpectralManipulation(prices []float64) float64 {
	n := len(prices)
	if n < 10 {
		return 50.0
	}

	// Simplified spectral analysis: measure high-frequency energy
	// Count direction changes (zigzags) = high-frequency noise
	changes := 0
	for i := 2; i < n; i++ {
		// Direction change if: (p[i] > p[i-1] AND p[i-1] < p[i-2]) OR vice versa
		if (prices[i] > prices[i-1] && prices[i-1] < prices[i-2]) ||
			(prices[i] < prices[i-1] && prices[i-1] > prices[i-2]) {
			changes++
		}
	}

	// More zigzags = more manipulation
	// Typical: 30% direction changes = normal
	// >60% = highly manipulated
	changeRate := float64(changes) / float64(n-2) * 100

	if changeRate < 30.0 {
		return 0.0
	} else if changeRate > 60.0 {
		return 100.0
	}

	return ((changeRate - 30.0) / 30.0) * 100
}

// calculateADXFast calculates Average Directional Index (ADX) from candles
// ADX measures trend strength (0-100): <20=weak/no trend, 20-40=strong trend, >40=very strong trend
// Returns the last ADX value only (optimized for single value calculation)
func CalculateADXFast(candles []models.CandleHLCV, period int) float64 {
	n := len(candles)
	lookbackTotal := (2 * period) - 1

	if n <= lookbackTotal {
		return 0.0 // Not enough data
	}

	const epsilon = 1e-10 // For zero checks
	periodF := float64(period)
	periodInv := 1.0 / periodF // Pre-compute division

	// Inline helper: calculate True Range
	calcTR := func(high, low, prevClose float64) float64 {
		tr := high - low
		hc := math.Abs(high - prevClose)
		lc := math.Abs(low - prevClose)
		return math.Max(tr, math.Max(hc, lc))
	}

	// Initialize with first candle
	today := 0
	prevHigh := candles[today].High
	prevLow := candles[today].Low
	prevClose := candles[today].Close
	prevMinusDM := 0.0
	prevPlusDM := 0.0
	prevTR := 0.0

	// Phase 1: Accumulate initial DM and TR (first 'period' bars)
	for i := 1; i < period; i++ {
		high := candles[i].High
		low := candles[i].Low

		diffP := high - prevHigh
		diffM := prevLow - low

		// Directional Movement
		if diffM > 0 && diffP < diffM {
			prevMinusDM += diffM
		} else if diffP > 0 && diffP > diffM {
			prevPlusDM += diffP
		}

		prevTR += calcTR(high, low, prevClose)
		prevHigh = high
		prevLow = low
		prevClose = candles[i].Close
	}

	// Phase 2: Calculate smoothed DI and accumulate DX (next 'period' bars)
	sumDX := 0.0
	endPhase2 := period * 2
	for i := period; i < endPhase2 && i < n; i++ {
		high := candles[i].High
		low := candles[i].Low

		diffP := high - prevHigh
		diffM := prevLow - low

		// Smooth DM (Wilder's smoothing)
		prevMinusDM -= prevMinusDM * periodInv
		prevPlusDM -= prevPlusDM * periodInv

		if diffM > 0 && diffP < diffM {
			prevMinusDM += diffM
		} else if diffP > 0 && diffP > diffM {
			prevPlusDM += diffP
		}

		// Smooth TR
		tr := calcTR(high, low, prevClose)
		prevTR = prevTR - (prevTR * periodInv) + tr

		// Calculate DX
		if prevTR > epsilon {
			minusDI := prevMinusDM / prevTR
			plusDI := prevPlusDM / prevTR
			sumDI := minusDI + plusDI

			if sumDI > epsilon {
				dx := math.Abs(minusDI-plusDI) / sumDI
				sumDX += dx
			}
		}

		prevHigh = high
		prevLow = low
		prevClose = candles[i].Close
	}

	// Initial ADX = average of DX values
	prevADX := (sumDX * periodInv) * 100.0

	// Phase 3: Smooth ADX to the end (remaining bars)
	for i := endPhase2; i < n; i++ {
		high := candles[i].High
		low := candles[i].Low

		diffP := high - prevHigh
		diffM := prevLow - low

		prevMinusDM -= prevMinusDM * periodInv
		prevPlusDM -= prevPlusDM * periodInv

		if diffM > 0 && diffP < diffM {
			prevMinusDM += diffM
		} else if diffP > 0 && diffP > diffM {
			prevPlusDM += diffP
		}

		tr := calcTR(high, low, prevClose)
		prevTR = prevTR - (prevTR * periodInv) + tr

		if prevTR > epsilon {
			minusDI := prevMinusDM / prevTR
			plusDI := prevPlusDM / prevTR
			sumDI := minusDI + plusDI

			if sumDI > epsilon {
				dx := (math.Abs(minusDI-plusDI) / sumDI) * 100.0
				// Smooth ADX: ADX = ((period-1) * prevADX + currentDX) / period
				prevADX = ((prevADX * (periodF - 1.0)) + dx) * periodInv
			}
		}

		prevHigh = high
		prevLow = low
		prevClose = candles[i].Close
	}

	return prevADX
}
