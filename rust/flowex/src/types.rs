//! Typed data models. Field names mirror the Go json tags exactly.
//!
//! Kept in sync with:
//!   - `depth/metrics.go`
//!   - `models/candle.go`
//!   - `models/trade.go`
//!   - `indicators/technical/models.go`
//!   - `capi/manager.go` (snapshotDTO)
//!
//! If Go fields get added or renamed, update this file to match.

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Candle {
    pub ts: i64,
    pub open: f64,
    pub high: f64,
    pub low: f64,
    pub close: f64,
    pub volume: f64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Trade {
    pub timestamp: i64,
    pub price: f64,
    pub size: f64,
    pub size_usd: f64,
    pub side: String,
    pub trade_id: String,
    pub symbol: String,
    pub exchange: String,
}

/// Mirrors `IndicatorSignal` — Go marshals this as a lowercase string.
#[derive(Debug, Clone, Copy, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum IndicatorSignal {
    StrongSell,
    Sell,
    #[default]
    Neutral,
    Buy,
    StrongBuy,
}

/// Matches `indicators/technical/models.go::TechnicalIndicators`.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct TechnicalIndicators {
    pub rsi_14: f64,

    pub sma_20: f64,
    pub sma_50: f64,
    pub sma_200: f64,
    pub ema_9: f64,
    pub ema_12: f64,
    pub ema_20: f64,
    pub ema_21: f64,
    pub ema_26: f64,
    pub ema_50: f64,
    pub ema_200: f64,

    pub macd_line: f64,
    pub signal_line: f64,
    pub histogram: f64,

    pub bb_upper: f64,
    pub bb_middle: f64,
    pub bb_lower: f64,

    pub atr: f64,
    pub stoch_rsi: f64,
    pub mmi: f64,

    pub ma_summary: IndicatorSignal,
    pub oscillator_sum: IndicatorSignal,
    pub overall_sum: IndicatorSignal,

    pub ma_buy: i32,
    pub ma_sell: i32,
    pub ma_neutral: i32,
    pub oscill_buy: i32,
    pub oscill_sell: i32,
    pub oscill_neutral: i32,
}

/// Bundled lock-free snapshot returned by `Manager::snapshot`.
/// Mirrors `capi/manager.go::snapshotDTO`.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Snapshot {
    pub timestamp_ms: i64,
    pub candles: Vec<Candle>,
    pub depth_latest: Option<DepthMetrics>,
    pub trades: Vec<Trade>,
    pub indicators: Option<TechnicalIndicators>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct MacdSeries {
    pub macd: Vec<f64>,
    pub signal: Vec<f64>,
    pub histogram: Vec<f64>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct BollingerScore {
    pub score: f64,
    pub osc_sd: f64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct SupportResistance {
    pub support_pct: f64,
    pub resistance_pct: f64,
    pub score: f64,
}

/// Input row for HLC-based indicators (ATR, Bollinger, support/resistance).
#[derive(Debug, Clone, Copy, Serialize)]
pub struct HlcCandle {
    pub ts: i64,
    pub high: f64,
    pub low: f64,
    pub close: f64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct DepthMetrics {
    pub timestamp: i64,
    pub symbol: String,

    pub best_bid: f64,
    pub best_ask: f64,
    pub spread: f64,
    pub spread_bps: f64,
    pub mid_price: f64,

    pub bid_liquidity_5: f64,
    pub ask_liquidity_5: f64,
    pub bid_liquidity_10: f64,
    pub ask_liquidity_10: f64,
    pub bid_liquidity_20: f64,
    pub ask_liquidity_20: f64,
    pub bid_liquidity_50: f64,
    pub ask_liquidity_50: f64,

    pub bid_volume_5: f64,
    pub ask_volume_5: f64,
    pub bid_volume_10: f64,
    pub ask_volume_10: f64,
    pub bid_volume_20: f64,
    pub ask_volume_20: f64,
    pub bid_volume_50: f64,
    pub ask_volume_50: f64,

    pub imbalance_ratio_5: f64,
    pub imbalance_ratio_10: f64,
    pub imbalance_ratio_20: f64,
    pub imbalance_ratio_50: f64,

    pub imbalance_delta_10: f64,
    pub imbalance_delta_20: f64,

    pub largest_bid_size: f64,
    pub largest_bid_price: f64,
    pub largest_bid_value: f64,
    pub largest_ask_size: f64,
    pub largest_ask_price: f64,
    pub largest_ask_value: f64,

    pub slippage_buy_100: f64,
    pub slippage_sell_100: f64,
    pub slippage_buy_1k: f64,
    pub slippage_sell_1k: f64,
    pub slippage_buy_5k: f64,
    pub slippage_sell_5k: f64,
    pub slippage_buy_10k: f64,
    pub slippage_sell_10k: f64,
    pub slippage_buy_50k: f64,
    pub slippage_sell_50k: f64,
    pub slippage_buy_100k: f64,
    pub slippage_sell_100k: f64,
    pub slippage_buy_500k: f64,
    pub slippage_sell_500k: f64,
    pub slippage_buy_1m: f64,
    pub slippage_sell_1m: f64,

    pub liquidity_velocity_10: f64,
    pub liquidity_velocity_50: f64,
    pub imbalance_velocity: f64,
    pub spread_velocity: f64,
    pub wall_velocity: f64,

    pub buy_pressure_momentum: f64,
    pub sell_pressure_momentum: f64,
    pub wall_building_bid: bool,
    pub wall_building_ask: bool,

    pub liquidity_zscore_10: f64,
    pub imbalance_zscore: f64,
    pub spread_zscore: f64,

    pub bid_levels_count: i32,
    pub ask_levels_count: i32,
    pub avg_bid_size_10: f64,
    pub avg_ask_size_10: f64,
    pub top_bid_concentration_5: f64,
    pub top_ask_concentration_5: f64,
    pub spread_norm_imbalance_delta_10: f64,
    pub spread_norm_imbalance_delta_20: f64,
    pub slippage_gradient_buy: f64,
    pub slippage_gradient_sell: f64,
    pub slippage_skew_1k: f64,
    pub slippage_skew_10k: f64,
}
