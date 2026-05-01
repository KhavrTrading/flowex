//! Safe, typed Rust bindings to the flowex shared library.
//!
//! ```no_run
//! use flowex::Flowex;
//!
//! let fx = unsafe { Flowex::load("flowex.dll") }.unwrap();
//! let rsi = fx.rsi(&[100.0, 101.0, 99.0, 102.0, 103.0], 14);
//! println!("rsi = {rsi}");
//! ```

use std::ffi::{CStr, CString};
use std::os::raw::c_char;
use std::sync::Arc;

use flowex_sys::FlowexLib;

pub mod types;
pub use types::{
    BollingerScore, Candle, DepthMetrics, HlcCandle, IndicatorSignal, MacdSeries, Snapshot,
    SupportResistance, TechnicalIndicators, Trade,
};

/// Errors returned by safe flowex calls.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("failed to load flowex library: {0}")]
    Load(#[from] libloading::Error),
    #[error("input contained interior NUL byte: {0}")]
    Nul(#[from] std::ffi::NulError),
    #[error("input could not be serialized: {0}")]
    InputSerialize(serde_json::Error),
    #[error("flowex returned invalid utf-8: {0}")]
    Utf8(#[from] std::str::Utf8Error),
    #[error("flowex returned invalid json: {0}")]
    ResultJson(serde_json::Error),
    #[error("flowex returned an error: {0}")]
    Remote(String),
    #[error("flowex returned null where a value was expected")]
    UnexpectedNull,
    #[error("invalid store handle")]
    InvalidHandle,
}

pub type Result<T> = std::result::Result<T, Error>;

/// Loaded flowex shared library. Cloneable (`Arc` inside), cheap to pass around.
#[derive(Clone)]
pub struct Flowex {
    inner: Arc<FlowexLib>,
}

impl Flowex {
    /// Load the flowex shared library from a path.
    ///
    /// # Safety
    /// The library at `path` must be a genuine flowex build with the
    /// expected symbols and signatures.
    pub unsafe fn load<P: AsRef<std::ffi::OsStr>>(path: P) -> Result<Self> {
        let lib = FlowexLib::load(path)?;
        Ok(Self { inner: Arc::new(lib) })
    }

    /// RSI over a slice of close prices.
    pub fn rsi(&self, closes: &[f64], period: i32) -> f64 {
        unsafe { (self.inner.rsi)(closes.as_ptr(), closes.len(), period) }
    }

    /// EMA over a slice of prices.
    pub fn ema(&self, prices: &[f64], period: i32) -> f64 {
        unsafe { (self.inner.ema)(prices.as_ptr(), prices.len(), period) }
    }

    /// Compute depth metrics from a raw L2 snapshot.
    ///
    /// `bids` and `asks` are [price, quantity] string pairs, ordered best-first.
    pub fn compute_depth_metrics(
        &self,
        symbol: &str,
        timestamp_ms: i64,
        bids: &[[&str; 2]],
        asks: &[[&str; 2]],
    ) -> Result<DepthMetrics> {
        let symbol = CString::new(symbol)?;
        let bids_json = serde_json::to_string(bids).map_err(Error::InputSerialize)?;
        let asks_json = serde_json::to_string(asks).map_err(Error::InputSerialize)?;
        let bids_c = CString::new(bids_json)?;
        let asks_c = CString::new(asks_json)?;

        let ptr = unsafe {
            (self.inner.compute_depth_metrics)(
                symbol.as_ptr(),
                timestamp_ms,
                bids_c.as_ptr(),
                asks_c.as_ptr(),
            )
        };
        self.take_json(ptr)
    }

    // ---------------------------------------------------------------------
    // Indicators
    // ---------------------------------------------------------------------

    /// MACD line, signal line, and histogram series.
    pub fn macd(&self, prices: &[f64]) -> Result<MacdSeries> {
        let ptr = unsafe { (self.inner.macd)(prices.as_ptr(), prices.len()) };
        self.take_json(ptr)
    }

    /// Stochastic RSI series over close prices.
    pub fn stoch_rsi(&self, closes: &[f64], rsi_period: i32, stoch_period: i32) -> Result<Vec<f64>> {
        let ptr = unsafe {
            (self.inner.stoch_rsi)(closes.as_ptr(), closes.len(), rsi_period, stoch_period)
        };
        self.take_json(ptr)
    }

    /// ATR(period) from a slice of HLC candles.
    pub fn atr(&self, candles: &[HlcCandle], period: i32) -> Result<f64> {
        let json = serde_json::to_string(candles).map_err(Error::InputSerialize)?;
        let c = CString::new(json)?;
        Ok(unsafe { (self.inner.atr)(c.as_ptr(), period) })
    }

    /// Bollinger mean-deviation score.
    pub fn bollinger(
        &self,
        candles: &[HlcCandle],
        base_len: i32,
        dev_len: i32,
    ) -> Result<BollingerScore> {
        let json = serde_json::to_string(candles).map_err(Error::InputSerialize)?;
        let c = CString::new(json)?;
        let ptr = unsafe { (self.inner.bollinger)(c.as_ptr(), base_len, dev_len) };
        self.take_json(ptr)
    }

    /// Pivot-based support / resistance distances.
    pub fn support_resistance(
        &self,
        candles: &[HlcCandle],
        lookback: i32,
        ret_window: i32,
    ) -> Result<SupportResistance> {
        let json = serde_json::to_string(candles).map_err(Error::InputSerialize)?;
        let c = CString::new(json)?;
        let ptr = unsafe { (self.inner.support_resistance)(c.as_ptr(), lookback, ret_window) };
        self.take_json(ptr)
    }

    /// Full bundled technical-indicator computation over OHLCV candles.
    pub fn technical_indicators(
        &self,
        candles: &[Candle],
        current_price: f64,
    ) -> Result<TechnicalIndicators> {
        let json = serde_json::to_string(candles).map_err(Error::InputSerialize)?;
        let c = CString::new(json)?;
        let ptr = unsafe { (self.inner.technical_indicators)(c.as_ptr(), current_price) };
        self.take_json(ptr)
    }

    // ---------------------------------------------------------------------
    // Candles
    // ---------------------------------------------------------------------

    /// Fetch historical OHLCV candles from an exchange REST endpoint.
    /// `exchange` is `"binance"`, `"bybit"`, or `"bitget"`.
    pub fn fetch_candles(
        &self,
        exchange: &str,
        symbol: &str,
        interval: &str,
        limit: i32,
    ) -> Result<Vec<Candle>> {
        let ex = CString::new(exchange)?;
        let sym = CString::new(symbol)?;
        let iv = CString::new(interval)?;
        let ptr = unsafe {
            (self.inner.fetch_candles)(ex.as_ptr(), sym.as_ptr(), iv.as_ptr(), limit)
        };
        self.take_json(ptr)
    }

    /// Aggregate OHLCV candles into bars of `duration_ms`.
    pub fn aggregate_candles(&self, candles: &[Candle], duration_ms: i64) -> Result<Vec<Candle>> {
        let json = serde_json::to_string(candles).map_err(Error::InputSerialize)?;
        let c = CString::new(json)?;
        let ptr = unsafe { (self.inner.aggregate_candles)(c.as_ptr(), duration_ms) };
        self.take_json(ptr)
    }

    // ---------------------------------------------------------------------
    // Depth store + exchange manager (handle-based)
    // ---------------------------------------------------------------------

    /// Create a new depth store. `recent_cap <= 0` uses the library default.
    pub fn new_store(&self, recent_cap: i32) -> Store {
        let handle = unsafe { (self.inner.store_new)(recent_cap) };
        Store { lib: self.clone(), handle }
    }

    /// Create a new exchange manager. `exchange` is one of `"binance"`,
    /// `"bybit"`, `"bitget"`. Returns `None` for an unknown exchange.
    pub fn new_manager(&self, exchange: &str) -> Result<Manager> {
        let ex = CString::new(exchange)?;
        let handle = unsafe { (self.inner.manager_new)(ex.as_ptr()) };
        if handle == 0 {
            return Err(Error::Remote(format!("unknown_exchange: {exchange}")));
        }
        Ok(Manager { lib: self.clone(), handle })
    }

    /// Consume a `*mut c_char` returned by flowex: copy to a Rust string,
    /// free through the DLL's allocator, then parse as JSON.
    ///
    /// If the returned payload is `{"error":"..."}`, surfaces as `Error::Remote`.
    fn take_json<T: serde::de::DeserializeOwned>(&self, ptr: *mut c_char) -> Result<T> {
        if ptr.is_null() {
            return Err(Error::UnexpectedNull);
        }
        let s = unsafe { CStr::from_ptr(ptr) }.to_str()?.to_owned();
        unsafe { (self.inner.free_string)(ptr) };

        if let Ok(v) = serde_json::from_str::<serde_json::Value>(&s) {
            if let Some(err) = v.get("error").and_then(|v| v.as_str()) {
                return Err(Error::Remote(err.to_owned()));
            }
        }
        serde_json::from_str::<T>(&s).map_err(Error::ResultJson)
    }
}

/// RAII handle to a flowex depth store.
///
/// The underlying store is released from the Go-side registry when this drops.
pub struct Store {
    lib: Flowex,
    handle: u64,
}

impl Store {
    /// Insert a depth-metrics snapshot, with cleanup bounds.
    pub fn add(&self, metrics: &DepthMetrics, max_metrics: i32, max_seconds: i32) -> Result<()> {
        let json = serde_json::to_string(metrics).map_err(Error::InputSerialize)?;
        let c = CString::new(json)?;
        let rc = unsafe {
            (self.lib.inner.store_add)(self.handle, c.as_ptr(), max_metrics, max_seconds)
        };
        match rc {
            0 => Ok(()),
            1 => Err(Error::InvalidHandle),
            _ => Err(Error::Remote(format!("store_add rc={rc}"))),
        }
    }

    /// Latest snapshot, if any.
    pub fn latest(&self) -> Result<Option<DepthMetrics>> {
        let ptr = unsafe { (self.lib.inner.store_get_latest)(self.handle) };
        self.lib.take_json::<Option<DepthMetrics>>(ptr)
    }

    /// The N most recent snapshots, oldest first.
    pub fn recent(&self, n: i32) -> Result<Vec<DepthMetrics>> {
        let ptr = unsafe { (self.lib.inner.store_get_recent_n)(self.handle, n) };
        self.lib.take_json(ptr)
    }

    /// Current size of the store.
    pub fn size(&self) -> i32 {
        unsafe { (self.lib.inner.store_size)(self.handle) }
    }
}

impl Drop for Store {
    fn drop(&mut self) {
        unsafe { (self.lib.inner.store_free)(self.handle) };
    }
}

/// Which stream to toggle in `Manager::unsubscribe`.
#[derive(Debug, Clone, Copy)]
pub enum StreamType {
    Candle,
    Depth,
    Trade,
}

impl StreamType {
    fn as_str(self) -> &'static str {
        match self {
            StreamType::Candle => "candle",
            StreamType::Depth => "depth",
            StreamType::Trade => "trade",
        }
    }
}

/// RAII handle to an exchange WebSocket manager.
///
/// The standard workflow:
/// 1. `new_manager("binance")` → `Manager`
/// 2. optionally `seed_candles(symbol, history)` for warm indicator context
/// 3. `subscribe_all(symbol)` (or the per-stream variants)
/// 4. poll `snapshot(symbol)` whenever you want current state — **lock-free**
/// 5. drop the `Manager` to tear down all goroutines
pub struct Manager {
    lib: Flowex,
    handle: u64,
}

impl Manager {
    /// Subscribe to candle (kline) updates for a symbol.
    pub fn subscribe_candle(&self, symbol: &str) -> Result<()> {
        let s = CString::new(symbol)?;
        check_rc(unsafe { (self.lib.inner.manager_subscribe_candle)(self.handle, s.as_ptr()) })
    }

    /// Subscribe to order-book depth updates for a symbol.
    pub fn subscribe_depth(&self, symbol: &str) -> Result<()> {
        let s = CString::new(symbol)?;
        check_rc(unsafe { (self.lib.inner.manager_subscribe_depth)(self.handle, s.as_ptr()) })
    }

    /// Subscribe to trade-tape updates for a symbol.
    pub fn subscribe_trade(&self, symbol: &str) -> Result<()> {
        let s = CString::new(symbol)?;
        check_rc(unsafe { (self.lib.inner.manager_subscribe_trade)(self.handle, s.as_ptr()) })
    }

    /// Subscribe to candle + depth + trade at once.
    pub fn subscribe_all(&self, symbol: &str) -> Result<()> {
        let s = CString::new(symbol)?;
        check_rc(unsafe { (self.lib.inner.manager_subscribe_all)(self.handle, s.as_ptr()) })
    }

    /// Deactivate one stream for a symbol.
    pub fn unsubscribe(&self, symbol: &str, stream: StreamType) -> Result<()> {
        let s = CString::new(symbol)?;
        let k = CString::new(stream.as_str())?;
        check_rc(unsafe {
            (self.lib.inner.manager_unsubscribe)(self.handle, s.as_ptr(), k.as_ptr())
        })
    }

    /// Deactivate every stream for a symbol.
    pub fn unsubscribe_all(&self, symbol: &str) -> Result<()> {
        let s = CString::new(symbol)?;
        check_rc(unsafe {
            (self.lib.inner.manager_unsubscribe_all)(self.handle, s.as_ptr())
        })
    }

    /// Bulk-load historical candles so indicator computation has context.
    pub fn seed_candles(&self, symbol: &str, candles: &[Candle]) -> Result<()> {
        let s = CString::new(symbol)?;
        let json = serde_json::to_string(candles).map_err(Error::InputSerialize)?;
        let c = CString::new(json)?;
        check_rc(unsafe {
            (self.lib.inner.manager_seed_candles)(self.handle, s.as_ptr(), c.as_ptr())
        })
    }

    /// The killer feature: a lock-free snapshot of candles, latest depth
    /// metrics, recent trades, and computed indicators — all in one read.
    pub fn snapshot(&self, symbol: &str) -> Result<Option<Snapshot>> {
        let s = CString::new(symbol)?;
        let ptr = unsafe { (self.lib.inner.manager_get_snapshot)(self.handle, s.as_ptr()) };
        self.lib.take_json::<Option<Snapshot>>(ptr)
    }

    /// Manager status as a generic JSON value (active streams, workers, etc.).
    pub fn status(&self) -> Result<serde_json::Value> {
        let ptr = unsafe { (self.lib.inner.manager_get_status)(self.handle) };
        self.lib.take_json(ptr)
    }
}

impl Drop for Manager {
    fn drop(&mut self) {
        unsafe { (self.lib.inner.manager_shutdown)(self.handle) };
    }
}

fn check_rc(rc: std::os::raw::c_int) -> Result<()> {
    match rc {
        0 => Ok(()),
        1 => Err(Error::InvalidHandle),
        2 => Err(Error::Remote("subscribe/seed failed".into())),
        other => Err(Error::Remote(format!("rc={other}"))),
    }
}
