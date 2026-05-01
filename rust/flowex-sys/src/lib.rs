//! Raw FFI bindings to the flowex C-ABI shared library.
//!
//! Load the library at runtime via [`FlowexLib::load`], then call the
//! exported functions. All heap-allocated C strings returned by flowex must
//! be released via [`FlowexLib::free_string`] — the DLL and the caller may
//! link different C runtimes, so freeing through any other allocator is UB.
//!
//! Higher-level safe wrappers live in the sibling `flowex` crate; most users
//! should depend on that instead of reaching into this sys crate directly.

use std::ffi::OsStr;
use std::os::raw::{c_char, c_double, c_int, c_longlong};

use libloading::{Library, Symbol};
use std::sync::OnceLock;

pub type FnFreeString = unsafe extern "C" fn(*mut c_char);

pub type FnRsi = unsafe extern "C" fn(*const c_double, usize, c_int) -> c_double;
pub type FnEma = unsafe extern "C" fn(*const c_double, usize, c_int) -> c_double;
pub type FnMacd = unsafe extern "C" fn(*const c_double, usize) -> *mut c_char;
pub type FnStochRsi = unsafe extern "C" fn(*const c_double, usize, c_int, c_int) -> *mut c_char;
pub type FnAtr = unsafe extern "C" fn(*const c_char, c_int) -> c_double;
pub type FnBollinger = unsafe extern "C" fn(*const c_char, c_int, c_int) -> *mut c_char;
pub type FnSupportResistance = unsafe extern "C" fn(*const c_char, c_int, c_int) -> *mut c_char;
pub type FnTechnicalIndicators =
    unsafe extern "C" fn(*const c_char, c_double) -> *mut c_char;

pub type FnComputeDepthMetrics =
    unsafe extern "C" fn(*const c_char, c_longlong, *const c_char, *const c_char) -> *mut c_char;
pub type FnStoreNew = unsafe extern "C" fn(c_int) -> u64;
pub type FnStoreFree = unsafe extern "C" fn(u64);
pub type FnStoreAdd = unsafe extern "C" fn(u64, *const c_char, c_int, c_int) -> c_int;
pub type FnStoreGetLatest = unsafe extern "C" fn(u64) -> *mut c_char;
pub type FnStoreGetRecentN = unsafe extern "C" fn(u64, c_int) -> *mut c_char;
pub type FnStoreSize = unsafe extern "C" fn(u64) -> c_int;

pub type FnFetchCandles =
    unsafe extern "C" fn(*const c_char, *const c_char, *const c_char, c_int) -> *mut c_char;
pub type FnAggregateCandles = unsafe extern "C" fn(*const c_char, c_longlong) -> *mut c_char;

pub type FnManagerNew = unsafe extern "C" fn(*const c_char) -> u64;
pub type FnManagerShutdown = unsafe extern "C" fn(u64);
pub type FnManagerSubscribeSymbol = unsafe extern "C" fn(u64, *const c_char) -> c_int;
pub type FnManagerUnsubscribe =
    unsafe extern "C" fn(u64, *const c_char, *const c_char) -> c_int;
pub type FnManagerSeedCandles =
    unsafe extern "C" fn(u64, *const c_char, *const c_char) -> c_int;
pub type FnManagerGetSnapshot = unsafe extern "C" fn(u64, *const c_char) -> *mut c_char;
pub type FnManagerGetStatus = unsafe extern "C" fn(u64) -> *mut c_char;

/// Resolved handle to a loaded flowex shared library.
///
/// Holds all exported function pointers. The underlying [`Library`] is
/// intentionally leaked (stored in a process-wide `OnceLock`) because the
/// Go runtime embedded in the DLL does not survive `FreeLibrary` — unloading
/// it triggers a crash on process exit. For a trading library this is a
/// non-issue; you only want one flowex instance per process anyway.
pub struct FlowexLib {
    pub free_string: FnFreeString,

    pub rsi: FnRsi,
    pub ema: FnEma,
    pub macd: FnMacd,
    pub stoch_rsi: FnStochRsi,
    pub atr: FnAtr,
    pub bollinger: FnBollinger,
    pub support_resistance: FnSupportResistance,
    pub technical_indicators: FnTechnicalIndicators,

    pub compute_depth_metrics: FnComputeDepthMetrics,
    pub store_new: FnStoreNew,
    pub store_free: FnStoreFree,
    pub store_add: FnStoreAdd,
    pub store_get_latest: FnStoreGetLatest,
    pub store_get_recent_n: FnStoreGetRecentN,
    pub store_size: FnStoreSize,

    pub fetch_candles: FnFetchCandles,
    pub aggregate_candles: FnAggregateCandles,

    pub manager_new: FnManagerNew,
    pub manager_shutdown: FnManagerShutdown,
    pub manager_subscribe_candle: FnManagerSubscribeSymbol,
    pub manager_subscribe_depth: FnManagerSubscribeSymbol,
    pub manager_subscribe_trade: FnManagerSubscribeSymbol,
    pub manager_subscribe_all: FnManagerSubscribeSymbol,
    pub manager_unsubscribe: FnManagerUnsubscribe,
    pub manager_unsubscribe_all: FnManagerSubscribeSymbol,
    pub manager_seed_candles: FnManagerSeedCandles,
    pub manager_get_snapshot: FnManagerGetSnapshot,
    pub manager_get_status: FnManagerGetStatus,
}

impl FlowexLib {
    /// Load a flowex shared library from `path`.
    ///
    /// `path` should be the platform-specific filename — `flowex.dll` on
    /// Windows, `libflowex.so` on Linux, `libflowex.dylib` on macOS.
    ///
    /// # Safety
    /// The library at `path` must be a genuine flowex C-ABI build with the
    /// expected symbols and signatures. Loading an unrelated DLL is UB.
    pub unsafe fn load<P: AsRef<OsStr>>(path: P) -> Result<Self, libloading::Error> {
        static LIB: OnceLock<Library> = OnceLock::new();
        let lib_ref: &'static Library = match LIB.get() {
            Some(l) => l,
            None => {
                let loaded = Library::new(path)?;
                // If another thread won the race, discard ours (still leaked).
                let _ = LIB.set(loaded);
                LIB.get().expect("library was just set")
            }
        };
        macro_rules! sym {
            ($name:literal) => {{
                let s: Symbol<_> = lib_ref.get($name)?;
                *s
            }};
        }
        Ok(Self {
            free_string: sym!(b"flowex_free_string"),

            rsi: sym!(b"flowex_rsi"),
            ema: sym!(b"flowex_ema"),
            macd: sym!(b"flowex_macd"),
            stoch_rsi: sym!(b"flowex_stoch_rsi"),
            atr: sym!(b"flowex_atr"),
            bollinger: sym!(b"flowex_bollinger"),
            support_resistance: sym!(b"flowex_support_resistance"),
            technical_indicators: sym!(b"flowex_technical_indicators"),

            compute_depth_metrics: sym!(b"flowex_compute_depth_metrics"),
            store_new: sym!(b"flowex_store_new"),
            store_free: sym!(b"flowex_store_free"),
            store_add: sym!(b"flowex_store_add"),
            store_get_latest: sym!(b"flowex_store_get_latest"),
            store_get_recent_n: sym!(b"flowex_store_get_recent_n"),
            store_size: sym!(b"flowex_store_size"),

            fetch_candles: sym!(b"flowex_fetch_candles"),
            aggregate_candles: sym!(b"flowex_aggregate_candles"),

            manager_new: sym!(b"flowex_manager_new"),
            manager_shutdown: sym!(b"flowex_manager_shutdown"),
            manager_subscribe_candle: sym!(b"flowex_manager_subscribe_candle"),
            manager_subscribe_depth: sym!(b"flowex_manager_subscribe_depth"),
            manager_subscribe_trade: sym!(b"flowex_manager_subscribe_trade"),
            manager_subscribe_all: sym!(b"flowex_manager_subscribe_all"),
            manager_unsubscribe: sym!(b"flowex_manager_unsubscribe"),
            manager_unsubscribe_all: sym!(b"flowex_manager_unsubscribe_all"),
            manager_seed_candles: sym!(b"flowex_manager_seed_candles"),
            manager_get_snapshot: sym!(b"flowex_manager_get_snapshot"),
            manager_get_status: sym!(b"flowex_manager_get_status"),
        })
    }
}
