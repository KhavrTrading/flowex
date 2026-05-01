//! End-to-end smoke test for the flowex Rust bindings.
//!
//! Prereqs:
//!     pwsh capi/build.ps1         # builds flowex.dll at the repo root
//!     cargo run --manifest-path rust/Cargo.toml --example smoke
//!
//! Env overrides:
//!     FLOWEX_DLL   — path to the shared library (default: `../flowex.dll`)
//!     FLOWEX_LIVE  — set to `1` to exercise the live Binance WS path

use std::time::Duration;

use flowex::Flowex;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let path = std::env::var("FLOWEX_DLL").unwrap_or_else(|_| default_path().to_owned());
    println!("loading {path}");
    let fx = unsafe { Flowex::load(&path) }?;

    // --- Pure-function indicators -----------------------------------------
    let closes: Vec<f64> = (0..80).map(|i| 100.0 + (i as f64).sin() * 5.0).collect();
    println!("rsi(14)      = {:.4}", fx.rsi(&closes, 14));
    println!("ema(20)      = {:.4}", fx.ema(&closes, 20));

    let macd = fx.macd(&closes)?;
    println!("macd last    = {:.4}", macd.macd.last().copied().unwrap_or(0.0));

    let stoch = fx.stoch_rsi(&closes, 14, 14)?;
    println!("stoch_rsi n  = {}", stoch.len());

    // --- Depth snapshot computation ---------------------------------------
    let bids = [["60000.0", "1.0"], ["59995.0", "2.0"], ["59990.0", "3.0"]];
    let asks = [["60005.0", "1.0"], ["60010.0", "2.0"], ["60015.0", "3.0"]];
    let dm = fx.compute_depth_metrics("BTCUSDT", 1_700_000_000_000, &bids, &asks)?;
    println!("depth mid    = {}  spread_bps = {:.4}", dm.mid_price, dm.spread_bps);

    // --- REST candle fetch + bundled technical indicators -----------------
    match fx.fetch_candles("binance", "BTCUSDT", "1m", 100) {
        Ok(candles) if !candles.is_empty() => {
            println!("fetched {} candles, last close = {}",
                candles.len(), candles.last().unwrap().close);
            let last_price = candles.last().unwrap().close;
            let ind = fx.technical_indicators(&candles, last_price)?;
            println!("technical rsi_14 = {:.2}  macd_line = {:.4}  bb_upper = {:.2}",
                ind.rsi_14, ind.macd_line, ind.bb_upper);
        }
        Ok(_) => println!("fetch_candles returned 0 rows"),
        Err(e) => println!("fetch_candles skipped: {e}"),
    }

    // --- Live manager + lock-free snapshot --------------------------------
    if std::env::var("FLOWEX_LIVE").as_deref() == Ok("1") {
        println!("\n--- live binance BTCUSDT ---");
        let mgr = fx.new_manager("binance")?;

        // Seed history so indicators populate on the first snapshot.
        let history = fx.fetch_candles("binance", "BTCUSDT", "1m", 200)?;
        mgr.seed_candles("BTCUSDT", &history)?;
        println!("seeded {} historical candles", history.len());

        mgr.subscribe_all("BTCUSDT")?;

        for i in 0..4 {
            std::thread::sleep(Duration::from_secs(2));
            match mgr.snapshot("BTCUSDT")? {
                Some(s) => println!(
                    "t+{}s  candles={}  trades={}  depth_mid={:?}  rsi_14={:?}",
                    (i + 1) * 2,
                    s.candles.len(),
                    s.trades.len(),
                    s.depth_latest.as_ref().map(|d| d.mid_price),
                    s.indicators.as_ref().map(|i| i.rsi_14),
                ),
                None => println!("t+{}s  no snapshot yet", (i + 1) * 2),
            }
        }
    } else {
        println!("\n(set FLOWEX_LIVE=1 to exercise the live WS path)");
    }

    Ok(())
}

#[cfg(target_os = "windows")]
fn default_path() -> &'static str { "../flowex.dll" }
#[cfg(target_os = "linux")]
fn default_path() -> &'static str { "../libflowex.so" }
#[cfg(target_os = "macos")]
fn default_path() -> &'static str { "../libflowex.dylib" }
