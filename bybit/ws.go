package bybit

import (
	"fmt"
	"sync"

	"github.com/KhavrTrading/flowex/ws"
)

// DepthLevel controls the number of order book levels.
type DepthLevel int

const (
	Depth1   DepthLevel = 1   // Top of book only
	Depth50  DepthLevel = 50  // default
	Depth200 DepthLevel = 200
	Depth500 DepthLevel = 500 // Full book
)

// ManagerConfig holds Bybit-specific configuration.
type ManagerConfig struct {
	WorkerConfig ws.WorkerConfig
	DepthLevel   DepthLevel // default: Depth50
	Interval     string     // candle interval, default: "1" (1 minute)
}

// DefaultManagerConfig returns production defaults.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		WorkerConfig: ws.DefaultWorkerConfig(),
		DepthLevel:   Depth50,
		Interval:     "1",
	}
}

// Manager is a Bybit-specific WebSocket manager.
type Manager struct {
	*ws.BaseManager
	cfg ManagerConfig

	streamCfgMu sync.RWMutex
	streamCfg   map[string]*symbolStreamCfg
}

type symbolStreamCfg struct {
	depthLevel DepthLevel
	interval   string
}

// NewManager creates a new Bybit WebSocket manager with default config.
func NewManager() *Manager {
	return NewManagerWithConfig(DefaultManagerConfig())
}

// NewManagerWithConfig creates a manager with custom config.
func NewManagerWithConfig(cfg ManagerConfig) *Manager {
	m := &Manager{
		cfg:       cfg,
		streamCfg: make(map[string]*symbolStreamCfg),
	}
	m.BaseManager = ws.NewBaseManager("Bybit", cfg.WorkerConfig, func(symbol string) (*ws.BaseClient, error) {
		client, err := NewClient(symbol)
		if err != nil {
			return nil, err
		}
		client.SetResubscribe(func(c *ws.BaseClient) error {
			return m.resubscribeAll(symbol, c)
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
// Bybit intervals: "1" (1m), "3", "5", "15", "30", "60", "120", "240", "360", "720", "D", "W", "M".
func (m *Manager) SubscribeCandleWithInterval(symbol, interval string, handler ws.CandleHandler) error {
	worker := m.GetOrCreateWorker(symbol)
	client, err := m.GetOrCreateClient(symbol)
	if err != nil {
		return fmt.Errorf("bybit candle %s: %w", symbol, err)
	}

	sc := m.getStreamCfg(symbol)
	sc.interval = interval

	simple := ToSimpleSymbol(symbol)
	SetCandleCallback(symbol, func(msg ws.CandleMsg) {
		worker.EnqueueCandle(msg)
	})

	m.ActivateStream(symbol, ws.StreamCandle)
	return SubscribeStream(client, fmt.Sprintf("kline.%s.%s", interval, simple))
}

// SubscribeDepth subscribes to depth data using the configured level.
func (m *Manager) SubscribeDepth(symbol string, handler ws.DepthHandler) error {
	return m.SubscribeDepthWithLevel(symbol, m.cfg.DepthLevel, handler)
}

// SubscribeDepthWithLevel subscribes to depth data with a specific number of levels.
// levels: Depth1, Depth50, Depth200, Depth500.
func (m *Manager) SubscribeDepthWithLevel(symbol string, level DepthLevel, handler ws.DepthHandler) error {
	worker := m.GetOrCreateWorker(symbol)
	client, err := m.GetOrCreateClient(symbol)
	if err != nil {
		return fmt.Errorf("bybit depth %s: %w", symbol, err)
	}

	sc := m.getStreamCfg(symbol)
	sc.depthLevel = level

	simple := ToSimpleSymbol(symbol)
	SetDepthCallback(symbol, func(msg ws.DepthMsg) {
		worker.EnqueueDepth(msg)
	})

	m.ActivateStream(symbol, ws.StreamDepth)
	return SubscribeStream(client, fmt.Sprintf("orderbook.%d.%s", int(level), simple))
}

// SubscribeTrade subscribes to public trade data.
func (m *Manager) SubscribeTrade(symbol string, handler ws.TradeHandler) error {
	worker := m.GetOrCreateWorker(symbol)
	client, err := m.GetOrCreateClient(symbol)
	if err != nil {
		return fmt.Errorf("bybit trade %s: %w", symbol, err)
	}

	simple := ToSimpleSymbol(symbol)
	SetTradeCallback(symbol, func(msg ws.TradeMsg) {
		worker.EnqueueTrade(msg)
	})

	m.ActivateStream(symbol, ws.StreamTrade)
	return SubscribeStream(client, fmt.Sprintf("publicTrade.%s", simple))
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

// Unsubscribe removes a specific stream.
func (m *Manager) Unsubscribe(symbol string, st ws.StreamType) error {
	m.DeactivateStream(symbol, st)
	return nil
}

// UnsubscribeAll removes all streams for a symbol.
func (m *Manager) UnsubscribeAll(symbol string) error {
	m.DeactivateStream(symbol, ws.StreamCandle)
	m.DeactivateStream(symbol, ws.StreamDepth)
	m.DeactivateStream(symbol, ws.StreamTrade)
	return nil
}

func (m *Manager) resubscribeAll(symbol string, client *ws.BaseClient) error {
	streams := m.GetActiveStreams(symbol)
	sc := m.getStreamCfg(symbol)
	simple := ToSimpleSymbol(symbol)

	for st := range streams {
		switch st {
		case ws.StreamCandle:
			SubscribeStream(client, fmt.Sprintf("kline.%s.%s", sc.interval, simple))
		case ws.StreamDepth:
			SubscribeStream(client, fmt.Sprintf("orderbook.%d.%s", int(sc.depthLevel), simple))
		case ws.StreamTrade:
			SubscribeStream(client, fmt.Sprintf("publicTrade.%s", simple))
		}
	}
	return nil
}
