package ws

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/KhavrTrading/flowex/depth"
	"github.com/KhavrTrading/flowex/indicators/technical"
	"github.com/KhavrTrading/flowex/models"

	log "github.com/sirupsen/logrus"
)

// WorkerConfig holds tunable parameters for a SymbolWorker.
type WorkerConfig struct {
	CandleChSize int // Candle channel buffer (default 256)
	DepthChSize  int // Depth channel buffer (default 2048)
	TradeChSize  int // Trade channel buffer (default 2048)

	MaxCandles         int // Max candles to keep (default 750)
	MaxTradesPerSymbol int // Max raw trades (default 1000)
	MaxNormTrades      int // Max normalized trades (default 2000)
	MaxDepthMetrics    int // Max depth metrics in store (default 10000)
	MaxDepthSeconds    int // Max seconds of depth data (default 1000)
	RecentMetricsSize  int // Recent depth buffer size (default 100)

	SnapshotInterval time.Duration // How often to update snapshot (default 1s)
}

// DefaultWorkerConfig returns production-tested defaults.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		CandleChSize:       256,
		DepthChSize:        2048,
		TradeChSize:        2048,
		MaxCandles:         750,
		MaxTradesPerSymbol: 1000,
		MaxNormTrades:      2000,
		MaxDepthMetrics:    10000,
		MaxDepthSeconds:    1000,
		RecentMetricsSize:  100,
		SnapshotInterval:   1 * time.Second,
	}
}

// CandleMsg is a raw candle update passed to the worker.
type CandleMsg struct {
	Timestamp int64
	Open      string
	High      string
	Low       string
	Close     string
	Volume    string
}

// DepthMsg is a raw depth update passed to the worker.
type DepthMsg struct {
	Bids      [][]string
	Asks      [][]string
	Timestamp int64
}

// TradeMsg is a raw trade update passed to the worker.
type TradeMsg struct {
	TradeID      string
	Price        string
	Quantity     string
	Side         string // "buy" or "sell"
	Timestamp    int64
	IsBuyerMaker bool // alternative to Side: if true, taker sold
}

// SymbolWorker is a per-symbol actor that owns all state and processes updates
// sequentially in a single goroutine. No locks needed for state access.
//
// Architecture: WS callbacks → enqueue to channel → worker loop → update state → atomic snapshot
type SymbolWorker struct {
	symbol string
	config WorkerConfig
	ctx    context.Context
	cancel context.CancelFunc

	// Incoming update channels (non-blocking enqueue)
	candleCh chan CandleMsg
	depthCh  chan DepthMsg
	tradeCh  chan TradeMsg

	// State (owned exclusively by the worker goroutine)
	candles    []models.CandleHLCV
	depthStore *depth.Store
	normTrades []models.NormalizedTrade
	indicators *technical.TechnicalIndicators

	// Atomic snapshot for lock-free reads
	snap atomic.Value // *Snapshot

	// User-provided hooks
	onCandleUpdate func([]models.CandleHLCV)
	onDepthUpdate  func(depth.DepthMetrics)
	onTradeUpdate  func(models.NormalizedTrade)

	// Errors (small lock for status reporting)
	errorsMu     sync.RWMutex
	recentErrors []string

	// Metrics
	candleDropped atomic.Int64
	depthDropped  atomic.Int64
	tradeDropped  atomic.Int64
	processed     atomic.Int64
}

// NewSymbolWorker creates and starts a worker for the given symbol.
func NewSymbolWorker(symbol string, cfg WorkerConfig) *SymbolWorker {
	ctx, cancel := context.WithCancel(context.Background())
	w := &SymbolWorker{
		symbol:       symbol,
		config:       cfg,
		ctx:          ctx,
		cancel:       cancel,
		candleCh:     make(chan CandleMsg, cfg.CandleChSize),
		depthCh:      make(chan DepthMsg, cfg.DepthChSize),
		tradeCh:      make(chan TradeMsg, cfg.TradeChSize),
		candles:      make([]models.CandleHLCV, 0, cfg.MaxCandles),
		depthStore:   depth.NewStoreWithCap(cfg.RecentMetricsSize),
		normTrades:   make([]models.NormalizedTrade, 0, cfg.MaxNormTrades),
		recentErrors: make([]string, 0, 10),
	}
	go w.loop()
	log.Infof("[%s] Worker started", symbol)
	return w
}

// Stop gracefully shuts down the worker.
func (w *SymbolWorker) Stop() {
	w.cancel()
	log.Infof("[%s] Worker stopped", w.symbol)
}

// Symbol returns the symbol this worker manages.
func (w *SymbolWorker) Symbol() string { return w.symbol }

// SetOnCandleUpdate sets a hook called after candle state changes.
func (w *SymbolWorker) SetOnCandleUpdate(fn func([]models.CandleHLCV)) {
	w.onCandleUpdate = fn
}

// SetOnDepthUpdate sets a hook called after a new depth metric is computed.
func (w *SymbolWorker) SetOnDepthUpdate(fn func(depth.DepthMetrics)) {
	w.onDepthUpdate = fn
}

// SetOnTradeUpdate sets a hook called after a new trade is normalized.
func (w *SymbolWorker) SetOnTradeUpdate(fn func(models.NormalizedTrade)) {
	w.onTradeUpdate = fn
}

// ===================== MAIN EVENT LOOP =====================

func (w *SymbolWorker) loop() {
	ticker := time.NewTicker(w.config.SnapshotInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case msg := <-w.candleCh:
			w.applyCandle(msg)
			w.processed.Add(1)
		case msg := <-w.depthCh:
			w.applyDepth(msg)
			w.processed.Add(1)
		case msg := <-w.tradeCh:
			w.applyTrade(msg)
			w.processed.Add(1)
		case <-ticker.C:
			w.updateSnapshot()
		}
	}
}

// ===================== STATE MUTATIONS =====================

func (w *SymbolWorker) applyCandle(msg CandleMsg) {
	c, err := models.NewCandleHLCVFromStrings(msg.Timestamp, msg.Open, msg.High, msg.Low, msg.Close, msg.Volume)
	if err != nil {
		w.addError(fmt.Sprintf("parse candle: %v", err))
		return
	}

	if len(w.candles) == 0 {
		w.candles = append(w.candles, c)
	} else {
		last := len(w.candles) - 1
		lastTs := w.candles[last].GetTimestamp()
		switch {
		case c.GetTimestamp() == lastTs:
			// Same minute — update in place
			if c.High > w.candles[last].High {
				w.candles[last].High = c.High
			}
			if c.Low < w.candles[last].Low {
				w.candles[last].Low = c.Low
			}
			w.candles[last].Close = c.Close
			w.candles[last].Volume = c.Volume
		case c.GetTimestamp() > lastTs:
			// New minute — append and trim
			if len(w.candles) >= w.config.MaxCandles {
				w.candles = w.candles[1:]
			}
			w.candles = append(w.candles, c)
		default:
			// c.GetTimestamp() < lastTs — out-of-order, drop and record
			w.candleDropped.Add(1)
			w.addError(fmt.Sprintf("out-of-order candle: incoming=%d last=%d", c.GetTimestamp(), lastTs))
			return
		}
	}

	// Recalculate indicators (need at least 14 candles for RSI)
	if len(w.candles) >= 14 {
		w.indicators = technical.CalculateTechnicalIndicators(w.candles, w.candles[len(w.candles)-1].Close)
	}

	if w.onCandleUpdate != nil {
		w.onCandleUpdate(w.candles)
	}
}

func (w *SymbolWorker) applyDepth(msg DepthMsg) {
	m := depth.ComputeDepthMetrics(w.symbol, msg.Timestamp, msg.Bids, msg.Asks)
	w.depthStore.AddAndEnrich(m, w.config.MaxDepthMetrics, w.config.MaxDepthSeconds, 10, w.config.RecentMetricsSize)

	if w.onDepthUpdate != nil {
		w.onDepthUpdate(m)
	}
}

func (w *SymbolWorker) applyTrade(msg TradeMsg) {
	side := msg.Side
	if side == "" {
		if msg.IsBuyerMaker {
			side = "sell"
		} else {
			side = "buy"
		}
	}

	price := parseFloatSafe(msg.Price)
	qty := parseFloatSafe(msg.Quantity)

	nt := models.NormalizedTrade{
		Symbol:    w.symbol,
		TradeID:   msg.TradeID,
		Price:     price,
		SizeUSD:   qty,
		Side:      side,
		Timestamp: msg.Timestamp,
	}
	w.normTrades = append(w.normTrades, nt)

	if len(w.normTrades) > w.config.MaxNormTrades {
		w.normTrades = w.normTrades[len(w.normTrades)-w.config.MaxNormTrades:]
	}

	if w.onTradeUpdate != nil {
		w.onTradeUpdate(nt)
	}
}

// ===================== CHANNEL ENQUEUE (non-blocking) =====================

// EnqueueCandle sends a candle update to the worker. Non-blocking; drops oldest if full.
func (w *SymbolWorker) EnqueueCandle(msg CandleMsg) {
	select {
	case w.candleCh <- msg:
	default:
		select {
		case <-w.candleCh:
			w.candleDropped.Add(1)
		default:
		}
		select {
		case w.candleCh <- msg:
		default:
			w.candleDropped.Add(1)
		}
	}
}

// EnqueueDepth sends a depth update to the worker. Non-blocking; drops oldest if full.
func (w *SymbolWorker) EnqueueDepth(msg DepthMsg) {
	select {
	case w.depthCh <- msg:
	default:
		select {
		case <-w.depthCh:
			w.depthDropped.Add(1)
		default:
		}
		select {
		case w.depthCh <- msg:
		default:
			w.depthDropped.Add(1)
		}
	}
}

// EnqueueTrade sends a trade update to the worker. Non-blocking; drops oldest if full.
func (w *SymbolWorker) EnqueueTrade(msg TradeMsg) {
	select {
	case w.tradeCh <- msg:
	default:
		select {
		case <-w.tradeCh:
			w.tradeDropped.Add(1)
		default:
		}
		select {
		case w.tradeCh <- msg:
		default:
			w.tradeDropped.Add(1)
		}
	}
}

// SeedCandles bulk-loads historical candles into the worker via the candle channel.
// Goroutine-safe but lossy under load: enqueue uses a non-blocking send that drops on
// a full channel. Use SeedCandlesDirect (or Manager.SubscribeAllWithSeed) for startup
// seeding; reserve SeedCandles for post-subscribe gap-fill (e.g., reconnect catch-up).
func (w *SymbolWorker) SeedCandles(candles []models.CandleHLCV) {
	for _, c := range candles {
		w.EnqueueCandle(CandleMsg{
			Timestamp: c.Ts,
			Open:      fmt.Sprintf("%g", c.Open),
			High:      fmt.Sprintf("%g", c.High),
			Low:       fmt.Sprintf("%g", c.Low),
			Close:     fmt.Sprintf("%g", c.Close),
			Volume:    fmt.Sprintf("%g", c.Volume),
		})
	}
}

// SeedCandlesDirect synchronously replaces the worker's candle history and
// recomputes indicators once. Candles must be sorted ascending by timestamp;
// the slice is trimmed to MaxCandles, keeping the most recent.
//
// CONTRACT: Caller MUST invoke this BEFORE any live data reaches the worker
// (i.e., before the first SubscribeCandle / SubscribeAll). The worker's
// loop() goroutine is already running at construction, so this method is NOT
// safe to call concurrently with EnqueueCandle. Use Manager.SubscribeAllWithSeed
// to enforce this ordering.
//
// Note: the periodic snapshot tick (SnapshotInterval, default 1s) reads
// w.candles from loop(); under `go test -race` you may see a benign race
// report if the seed lands during a tick. In production this is invisible
// because seed runs once at startup well before any consumer reads the snapshot.
func (w *SymbolWorker) SeedCandlesDirect(candles []models.CandleHLCV) {
	if len(candles) == 0 {
		return
	}
	if len(candles) > w.config.MaxCandles {
		candles = candles[len(candles)-w.config.MaxCandles:]
	}
	w.candles = append(w.candles[:0], candles...)
	if len(w.candles) >= 14 {
		w.indicators = technical.CalculateTechnicalIndicators(
			w.candles, w.candles[len(w.candles)-1].Close,
		)
	}
	w.updateSnapshot()
}

// ===================== SNAPSHOT =====================

func (w *SymbolWorker) updateSnapshot() {
	snap := &Snapshot{
		Timestamp:  time.Now(),
		Candles:    make([]models.CandleHLCV, len(w.candles)),
		DepthStore: w.depthStore,
		Trades:     make([]models.NormalizedTrade, len(w.normTrades)),
		Indicators: w.indicators,
	}
	copy(snap.Candles, w.candles)
	copy(snap.Trades, w.normTrades)
	w.snap.Store(snap)
}

// GetSnapshot returns the latest immutable snapshot (lock-free read).
func (w *SymbolWorker) GetSnapshot() *Snapshot {
	v := w.snap.Load()
	if v == nil {
		return nil
	}
	return v.(*Snapshot)
}

// GetCandles returns the latest candle data from the snapshot.
func (w *SymbolWorker) GetCandles() []models.CandleHLCV {
	s := w.GetSnapshot()
	if s == nil {
		return nil
	}
	return s.Candles
}

// GetDepthStore returns the depth metrics store.
func (w *SymbolWorker) GetDepthStore() *depth.Store {
	s := w.GetSnapshot()
	if s == nil {
		return nil
	}
	return s.DepthStore
}

// GetNormalizedTrades returns the latest normalized trades from the snapshot.
func (w *SymbolWorker) GetNormalizedTrades() []models.NormalizedTrade {
	s := w.GetSnapshot()
	if s == nil {
		return nil
	}
	return s.Trades
}

// GetIndicators returns the cached technical indicators from the snapshot.
func (w *SymbolWorker) GetIndicators() *technical.TechnicalIndicators {
	s := w.GetSnapshot()
	if s == nil {
		return nil
	}
	return s.Indicators
}

// ===================== METRICS =====================

// GetMetrics returns current worker metrics.
func (w *SymbolWorker) GetMetrics() map[string]int64 {
	return map[string]int64{
		"processed":      w.processed.Load(),
		"candle_dropped": w.candleDropped.Load(),
		"depth_dropped":  w.depthDropped.Load(),
		"trade_dropped":  w.tradeDropped.Load(),
		"candle_queue":   int64(len(w.candleCh)),
		"depth_queue":    int64(len(w.depthCh)),
		"trade_queue":    int64(len(w.tradeCh)),
	}
}

func (w *SymbolWorker) addError(errMsg string) {
	w.errorsMu.Lock()
	defer w.errorsMu.Unlock()
	ts := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), errMsg)
	w.recentErrors = append(w.recentErrors, ts)
	if len(w.recentErrors) > 10 {
		w.recentErrors = w.recentErrors[len(w.recentErrors)-10:]
	}
}

// GetRecentErrors returns a copy of recent errors.
func (w *SymbolWorker) GetRecentErrors() []string {
	w.errorsMu.RLock()
	defer w.errorsMu.RUnlock()
	if len(w.recentErrors) == 0 {
		return nil
	}
	out := make([]string, len(w.recentErrors))
	copy(out, w.recentErrors)
	return out
}

// ===================== HELPERS =====================

func parseFloatSafe(s string) float64 {
	v := 0.0
	neg := false
	i := 0
	if len(s) == 0 {
		return 0
	}
	if s[0] == '-' {
		neg = true
		i = 1
	}
	for ; i < len(s) && s[i] != '.'; i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0
		}
		v = v*10 + float64(s[i]-'0')
	}
	if i < len(s) && s[i] == '.' {
		i++
		frac := 0.1
		for ; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				break
			}
			v += float64(s[i]-'0') * frac
			frac *= 0.1
		}
	}
	if neg {
		return -v
	}
	return v
}
