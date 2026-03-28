package technical

import (
	"sync"
	"time"
)

type SignalType string

const (
	SignalFirstTouch    SignalType = "first_touch"    // Threshold crossed for first time
	SignalMomentumShift SignalType = "momentum_shift" // Sharp acceleration detected
	SignalPeakDetected  SignalType = "peak_detected"  // Price hit extreme and started reversing
	SignalReversal      SignalType = "reversal"       // Direction changed with conviction
	SignalDeepening     SignalType = "deepening"      // Movement continuing same direction
	SignalExhaustion    SignalType = "exhaustion"     // Movement slowing, volume declining
	SignalContinuation  SignalType = "continuation"   // Movement resumed after brief pause
	SignalConsensus     SignalType = "consensus"      // All exchanges agree
	SignalDivergence    SignalType = "divergence"     // Exchange deviation detected
)

// SignalConfidence levels
type SignalConfidence string

const (
	ConfidenceHigh   SignalConfidence = "high"   // All exchanges agree
	ConfidenceMedium SignalConfidence = "medium" // 2/3 exchanges agree
	ConfidenceLow    SignalConfidence = "low"    // Single exchange or high divergence
)

// MarketCondition describes overall market state
type MarketCondition string

const (
	MarketSmooth MarketCondition = "smooth" // Clean directional move
	MarketChoppy MarketCondition = "choppy" // Oscillating, >3 direction changes
	MarketFlash  MarketCondition = "flash"  // Flash crash/pump (<2s duration)
)

// ============================================================
// Movement Tracking (Per-Symbol State Machine)
// ============================================================

// MovementState tracks a symbol's price action in real-time
type MovementState struct {
	Mu sync.RWMutex

	// Identity
	Symbol   string
	Exchange string

	// Price tracking
	FirstPrice     float64   // Price when movement started
	PeakPrice      float64   // Highest price this candle
	ValleyPrice    float64   // Lowest price this candle
	CurrentPrice   float64   // Latest price
	LastAlertPrice float64   // Last price we sent alert for
	LastAlertTime  time.Time // When we sent last alert

	// Movement metadata
	Direction         string    // "up" or "down"
	MovementStartTime time.Time // When threshold first crossed
	DirectionChanges  int       // Count of reversals (choppiness)
	LastDirectionTime time.Time // When direction last changed
	PeakTime          time.Time // When peak was hit
	ValleyTime        time.Time // When valley was hit

	// Velocity tracking
	PriceHistory    []PricePoint // Last N price points for velocity calculation
	CurrentVelocity float64      // %/second

	// State flags
	IsActive        bool      // Movement in progress
	LastUpdateTime  time.Time // Last data received
	MarketCondition MarketCondition
	AlertsSent      int    // Count of alerts for this movement
	MovementID      string // Unique ID for this movement
}

// GetMovementDuration returns how long movement has been active
func (state *MovementState) GetMovementDuration() time.Duration {
	state.Mu.RLock()
	defer state.Mu.RUnlock()

	if !state.IsActive {
		return 0
	}

	return time.Since(state.MovementStartTime)
}

// GetPriceRange returns price range for this movement
func (state *MovementState) GetPriceRange() PriceRange {
	state.Mu.RLock()
	defer state.Mu.RUnlock()

	min := state.ValleyPrice
	max := state.PeakPrice

	if min == 0 {
		return PriceRange{}
	}

	span := (max - min) / min

	return PriceRange{
		Min:     min,
		Max:     max,
		SpanPct: span * 100,
	}
}

// IncrementAlertsSent increments the alerts sent counter
func (state *MovementState) IncrementAlertsSent() {
	state.Mu.Lock()
	defer state.Mu.Unlock()
	state.AlertsSent++
}

// PricePoint for velocity calculation
type PricePoint struct {
	Price     float64
	Timestamp time.Time
}

// ============================================================
// Cross-Exchange Analysis
// ============================================================

// CrossExchangeMetrics holds analysis across all exchanges
type CrossExchangeMetrics struct {
	// Basic stats
	AvgPrice       float64 // Average price across exchanges
	AvgChange      float64 // Average price change across exchanges
	StdDeviation   float64 // Price spread between exchanges
	BestEntryPrice float64 // Lowest for drops, highest for pumps

	// Exchange data
	ExchangePrices  map[string]float64 // exchange -> price
	ExchangeChanges map[string]float64 // exchange -> change %
	LeadingExchange string             // Which exchange moved first/most
	ExchangesAgree  int                // Count of exchanges in agreement (2 or 3)

	// Signal classification
	Confidence     SignalConfidence
	IsDivergence   bool    // True if exchanges disagree significantly
	DivergenceSize float64 // Max deviation from average (%)

	// Opportunity flags
	ArbitrageOpportunity bool    // Price spread > threshold
	ArbitrageSpread      float64 // Size of arbitrage opportunity
}

// ============================================================
// Signal Generation
// ============================================================

// IndicatorSignal represents buy/sell/neutral signals from indicators
type IndicatorSignal int

const (
	SignalStrongSell IndicatorSignal = iota - 2
	SignalSell
	SignalNeutral
	SignalBuy
	SignalStrongBuy
)

// String converts IndicatorSignal to string for JSON
func (s IndicatorSignal) String() string {
	switch s {
	case SignalStrongBuy:
		return "strong_buy"
	case SignalBuy:
		return "buy"
	case SignalNeutral:
		return "neutral"
	case SignalSell:
		return "sell"
	case SignalStrongSell:
		return "strong_sell"
	default:
		return "neutral"
	}
}

// MarshalJSON implements json.Marshaler for IndicatorSignal
func (s IndicatorSignal) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// TechnicalIndicators holds calculated indicators from historical candle data
type TechnicalIndicators struct {
	// RSI (Relative Strength Index) - 14 period
	RSI14 float64 `json:"rsi_14"`

	// Moving Averages
	SMA20  float64 `json:"sma_20"`  // Simple Moving Average 20
	SMA50  float64 `json:"sma_50"`  // Simple Moving Average 50
	SMA200 float64 `json:"sma_200"` // Simple Moving Average 200
	EMA9   float64 `json:"ema_9"`   // Exponential Moving Average 9
	EMA12  float64 `json:"ema_12"`  // Exponential Moving Average 12
	EMA20  float64 `json:"ema_20"`  // Exponential Moving Average 20
	EMA21  float64 `json:"ema_21"`  // Exponential Moving Average 21
	EMA26  float64 `json:"ema_26"`  // Exponential Moving Average 26
	EMA50  float64 `json:"ema_50"`  // Exponential Moving Average 50
	EMA200 float64 `json:"ema_200"` // Exponential Moving Average 200

	// MACD
	MACDLine   float64 `json:"macd_line"`
	SignalLine float64 `json:"signal_line"`
	Histogram  float64 `json:"histogram"`

	// Bollinger Bands (20-period, 2 StdDev)
	BBUpper  float64 `json:"bb_upper"`
	BBMiddle float64 `json:"bb_middle"`
	BBLower  float64 `json:"bb_lower"`

	// Volatility & Advanced Indicators
	ATR      float64 `json:"atr"`       // Average True Range (14-period)
	StochRSI float64 `json:"stoch_rsi"` // Stochastic RSI
	MMI      float64 `json:"mmi"`       // Market Manipulation Index (0-100)

	// Summary Signals (using typed constants)
	MASummary     IndicatorSignal `json:"ma_summary"`
	OscillatorSum IndicatorSignal `json:"oscillator_sum"`
	OverallSum    IndicatorSignal `json:"overall_sum"`

	// Counts for TradingView-style summary
	MABuy       int `json:"ma_buy"`
	MASell      int `json:"ma_sell"`
	MANeutral   int `json:"ma_neutral"`
	OscillBuy   int `json:"oscill_buy"`
	OscillSell  int `json:"oscill_sell"`
	OscillNeutr int `json:"oscill_neutral"`
}

// SignalCrossExchangeData holds cross-exchange metrics for signals
type SignalCrossExchangeData struct {
	Binance         float64 `json:"binance"`
	Bitget          float64 `json:"bitget"`
	Bybit           float64 `json:"bybit"`
	Avg             float64 `json:"avg"`
	StdDev          float64 `json:"std_dev"`
	BestPrice       float64 `json:"best_price"`
	LeadingExchange string  `json:"leading_exchange"`
	ArbitrageSpread float64 `json:"arbitrage_spread,omitempty"`
}

// ============================================================
// Signal Batching
// ============================================================

// SignalBatch represents a group of prioritized signals for one movement
type SignalBatch struct {
	MovementID    string
	Symbol        string
	Signals       []TradingSignal
	BatchTime     time.Time
	MovementStart time.Time
	MovementEnd   time.Time
}

// PriceRange describes the price movement range
type PriceRange struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	SpanPct float64 `json:"span_pct"`
}

// TradingSignal is the final enriched signal sent to strategies
type TradingSignal struct {
	// Basic info
	Type        SignalType `json:"type"`
	Exchange    string     `json:"exchange"`
	Symbol      string     `json:"symbol"`
	Timeframe   string     `json:"timeframe"`
	PriceChange float64    `json:"price_change"`
	Open        float64    `json:"open"`
	Close       float64    `json:"close"`
	Timestamp   string     `json:"timestamp"`

	// Movement context
	MovementID       string     `json:"movement_id"`
	SignalRank       int        `json:"signal_rank"` // 1=best, 2=average, 3=initial
	PriceRange       PriceRange `json:"price_range"`
	PeakPrice        float64    `json:"peak_price"`        // Highest price in movement
	PeakTime         time.Time  `json:"peak_time"`         // When peak occurred
	ValleyPrice      float64    `json:"valley_price"`      // Lowest price in movement
	ValleyTime       time.Time  `json:"valley_time"`       // When valley occurred
	TimeInMotion     float64    `json:"time_in_motion"`    // seconds
	Velocity         float64    `json:"movement_velocity"` // %/second
	DirectionChanges int        `json:"direction_changes"`

	// Cross-exchange data
	Confidence     SignalConfidence         `json:"confidence"`
	ExchangesAgree int                      `json:"exchanges_agreeing"`
	CrossExchange  *SignalCrossExchangeData `json:"cross_exchange,omitempty"`

	// Market context
	MarketCondition MarketCondition `json:"market_condition,omitempty"`
	IsCounterTrend  bool            `json:"counter_trend,omitempty"`

	// Technical indicators (calculated from 12h historical data)
	Indicators *TechnicalIndicators `json:"indicators,omitempty"`

	// Expiry
	ValidUntil time.Time `json:"valid_until"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
}
