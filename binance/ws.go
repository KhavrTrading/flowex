package binance

import (
	"fmt"
	"strings"
	"sync"

	"github.com/KhavrTrading/flowex/models"
	"github.com/KhavrTrading/flowex/ws"
)

// DepthLevel controls the number of order book levels in depth snapshots.
type DepthLevel int

const (
	Depth5  DepthLevel = 5
	Depth10 DepthLevel = 10
	Depth20 DepthLevel = 20 // default
)

// DepthSpeed controls the update frequency for depth streams.
type DepthSpeed string

const (
	Speed100ms  DepthSpeed = "100ms"
	Speed250ms  DepthSpeed = "250ms" // Binance default for some streams
	Speed500ms  DepthSpeed = "500ms"
	SpeedDefault DepthSpeed = "" // use exchange default
)

// TradeMode controls which trade stream to subscribe to.
type TradeMode string

const (
	TradeAggregated TradeMode = "aggTrade" // default — aggregated trades
	TradeIndividual TradeMode = "trade"    // individual trades (higher volume)
)

// ManagerConfig holds exchange-specific configuration.
type ManagerConfig struct {
	WorkerConfig ws.WorkerConfig
	DepthLevel   DepthLevel // default: Depth20
	DepthSpeed   DepthSpeed // default: SpeedDefault
	TradeMode    TradeMode  // default: TradeAggregated
	Interval     string     // candle interval, default: "1m"
}

// DefaultManagerConfig returns production defaults.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		WorkerConfig: ws.DefaultWorkerConfig(),
		DepthLevel:   Depth20,
		DepthSpeed:   SpeedDefault,
		TradeMode:    TradeAggregated,
		Interval:     "1m",
	}
}

// Manager is a Binance-specific WebSocket manager.
type Manager struct {
	*ws.BaseManager
	cfg ManagerConfig

	// Per-symbol stream config (for reconnect)
	streamCfgMu sync.RWMutex
	streamCfg   map[string]*symbolStreamCfg
}

type symbolStreamCfg struct {
	depthLevel DepthLevel
	depthSpeed DepthSpeed
	tradeMode  TradeMode
	interval   string
}

// NewManager creates a new Binance WebSocket manager with default config.
func NewManager() *Manager {
	return NewManagerWithConfig(DefaultManagerConfig())
}

// NewManagerWithConfig creates a manager with custom config.
func NewManagerWithConfig(cfg ManagerConfig) *Manager {
	m := &Manager{
		cfg:       cfg,
		streamCfg: make(map[string]*symbolStreamCfg),
	}
	m.BaseManager = ws.NewBaseManager("Binance", cfg.WorkerConfig, func(symbol, clientKey string) (*ws.BaseClient, error) {
		parts := strings.Split(clientKey, "|")
		urlType := "market"
		if len(parts) > 1 {
			urlType = parts[1]
		}
		url := fmt.Sprintf("wss://fstream.binance.com/%s/ws", urlType)

		client, err := NewClient(symbol, url)
		if err != nil {
			return nil, err
		}
		client.SetResubscribe(func(c *ws.BaseClient) error {
			return m.resubscribeAll(symbol, clientKey, c)
		})
		return client, nil
	})
	return m
}

func (m *Manager) getStreamCfg(symbol string) *symbolStreamCfg {
	m.streamCfgMu.RLock()
	sc := m.streamCfg[symbol]
	m.streamCfgMu.RUnlock()
	if sc != nil {
		return sc
	}

	m.streamCfgMu.Lock()
	defer m.streamCfgMu.Unlock()
	if sc = m.streamCfg[symbol]; sc != nil {
		return sc
	}
	sc = &symbolStreamCfg{
		depthLevel: m.cfg.DepthLevel,
		depthSpeed: m.cfg.DepthSpeed,
		tradeMode:  m.cfg.TradeMode,
		interval:   m.cfg.Interval,
	}
	m.streamCfg[symbol] = sc
	return sc
}

// SubscribeCandle subscribes to candle data using the configured interval.
func (m *Manager) SubscribeCandle(symbol string, handler ws.CandleHandler) error {
	return m.SubscribeCandleWithInterval(symbol, m.cfg.Interval, handler)
}

// SubscribeCandleWithInterval subscribes to candle data with a specific interval.
// Intervals: "1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "8h", "12h", "1d", "1w".
func (m *Manager) SubscribeCandleWithInterval(symbol, interval string, handler ws.CandleHandler) error {
	worker := m.GetOrCreateWorker(symbol)
	clientKey := symbol + "|market"
	client, err := m.GetOrCreateClientByKey(symbol, clientKey)
	if err != nil {
		return fmt.Errorf("binance candle %s: %w", symbol, err)
	}

	sc := m.getStreamCfg(symbol)
	sc.interval = interval

	SetCandleCallback(symbol, func(msg ws.CandleMsg) {
		worker.EnqueueCandle(msg)
	})

	m.ActivateStreamByKey(symbol, clientKey, ws.StreamCandle)
	return SubscribeStream(client, CandleStreamName(symbol, interval), 1)
}

// SubscribeDepth subscribes to depth data using the configured level and speed.
func (m *Manager) SubscribeDepth(symbol string, handler ws.DepthHandler) error {
	return m.SubscribeDepthWithConfig(symbol, m.cfg.DepthLevel, m.cfg.DepthSpeed, handler)
}

// SubscribeDepthWithConfig subscribes to depth data with specific level and speed.
// levels: Depth5, Depth10, Depth20. speed: Speed100ms, Speed250ms, Speed500ms.
func (m *Manager) SubscribeDepthWithConfig(symbol string, level DepthLevel, speed DepthSpeed, handler ws.DepthHandler) error {
	worker := m.GetOrCreateWorker(symbol)
	clientKey := symbol + "|public"
	client, err := m.GetOrCreateClientByKey(symbol, clientKey)
	if err != nil {
		return fmt.Errorf("binance depth %s: %w", symbol, err)
	}

	sc := m.getStreamCfg(symbol)
	sc.depthLevel = level
	sc.depthSpeed = speed

	SetDepthCallback(symbol, func(msg ws.DepthMsg) {
		worker.EnqueueDepth(msg)
	})

	m.ActivateStreamByKey(symbol, clientKey, ws.StreamDepth)
	return SubscribeStream(client, DepthStreamName(symbol, int(level), string(speed)), 2)
}

// SubscribeTrade subscribes to trade data using the configured mode.
func (m *Manager) SubscribeTrade(symbol string, handler ws.TradeHandler) error {
	return m.SubscribeTradeWithMode(symbol, m.cfg.TradeMode, handler)
}

// SubscribeTradeWithMode subscribes to trades with a specific mode.
// mode: TradeAggregated (aggTrade) or TradeIndividual (trade).
func (m *Manager) SubscribeTradeWithMode(symbol string, mode TradeMode, handler ws.TradeHandler) error {
	worker := m.GetOrCreateWorker(symbol)
	clientKey := symbol + "|market"
	client, err := m.GetOrCreateClientByKey(symbol, clientKey)
	if err != nil {
		return fmt.Errorf("binance trade %s: %w", symbol, err)
	}

	sc := m.getStreamCfg(symbol)
	sc.tradeMode = mode

	SetTradeCallback(symbol, func(msg ws.TradeMsg) {
		worker.EnqueueTrade(msg)
	})

	m.ActivateStreamByKey(symbol, clientKey, ws.StreamTrade)

	var stream string
	if mode == TradeIndividual {
		stream = TradeStreamName(symbol)
	} else {
		stream = AggTradeStreamName(symbol)
	}
	return SubscribeStream(client, stream, 3)
}

// SubscribeAll subscribes to candles, depth, and trades using default config.
func (m *Manager) SubscribeAll(symbol string, ch ws.CandleHandler, dh ws.DepthHandler, th ws.TradeHandler) error {
	if err := m.SubscribeCandle(symbol, ch); err != nil {
		return err
	}
	if err := m.SubscribeDepth(symbol, dh); err != nil {
		return err
	}
	return m.SubscribeTrade(symbol, th)
}

// SubscribeAllWithSeed seeds historical candles into the worker BEFORE
// subscribing to live streams, guaranteeing no race between seed and live
// data. seed must be sorted ascending by timestamp.
func (m *Manager) SubscribeAllWithSeed(symbol string, seed []models.CandleHLCV) error {
	w := m.GetOrCreateWorker(symbol)
	w.SeedCandlesDirect(seed)
	return m.SubscribeAll(symbol, nil, nil, nil)
}

// Unsubscribe removes a specific stream for a symbol.
func (m *Manager) Unsubscribe(symbol string, st ws.StreamType) error {
	clientKey := symbol + "|market"
	if st == ws.StreamDepth {
		clientKey = symbol + "|public"
	}
	m.DeactivateStreamByKey(symbol, clientKey, st)
	return nil
}

// UnsubscribeAll removes all streams for a symbol.
func (m *Manager) UnsubscribeAll(symbol string) error {
	m.DeactivateStreamByKey(symbol, symbol+"|market", ws.StreamCandle)
	m.DeactivateStreamByKey(symbol, symbol+"|public", ws.StreamDepth)
	m.DeactivateStreamByKey(symbol, symbol+"|market", ws.StreamTrade)
	return nil
}

func (m *Manager) resubscribeAll(symbol, clientKey string, client *ws.BaseClient) error {
	streams := m.GetActiveStreamsByKey(clientKey)
	sc := m.getStreamCfg(symbol)

	for st := range streams {
		switch st {
		case ws.StreamCandle:
			SubscribeStream(client, CandleStreamName(symbol, sc.interval), 1)
		case ws.StreamDepth:
			SubscribeStream(client, DepthStreamName(symbol, int(sc.depthLevel), string(sc.depthSpeed)), 2)
		case ws.StreamTrade:
			if sc.tradeMode == TradeIndividual {
				SubscribeStream(client, TradeStreamName(symbol), 3)
			} else {
				SubscribeStream(client, AggTradeStreamName(symbol), 3)
			}
		}
	}
	return nil
}
