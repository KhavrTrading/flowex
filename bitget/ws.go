package bitget

import (
	"fmt"
	"sync"

	"github.com/KhavrTrading/flowex/models"
	"github.com/KhavrTrading/flowex/ws"
)

// DepthChannel controls the order book depth channel.
type DepthChannel string

const (
	DepthFull    DepthChannel = "books"   // Full depth (default)
	DepthBooks5  DepthChannel = "books5"  // Top 5 levels
	DepthBooks15 DepthChannel = "books15" // Top 15 levels
)

// InstType controls the instrument type.
type InstType string

const (
	InstUSDTFutures InstType = "USDT-FUTURES" // default
	InstSpot        InstType = "SPOT"
	InstCoinFutures InstType = "COIN-FUTURES"
)

// ManagerConfig holds Bitget-specific configuration.
type ManagerConfig struct {
	WorkerConfig ws.WorkerConfig
	InstType     InstType     // default: InstUSDTFutures
	DepthChannel DepthChannel // default: DepthFull
	Interval     string       // candle channel suffix, default: "1m"
}

// DefaultManagerConfig returns production defaults.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		WorkerConfig: ws.DefaultWorkerConfig(),
		InstType:     InstUSDTFutures,
		DepthChannel: DepthFull,
		Interval:     "1m",
	}
}

// Manager is a Bitget-specific WebSocket manager.
type Manager struct {
	*ws.BaseManager
	cfg ManagerConfig

	streamCfgMu sync.RWMutex
	streamCfg   map[string]*symbolStreamCfg
}

type symbolStreamCfg struct {
	depthChannel DepthChannel
	interval     string
}

// NewManager creates a new Bitget WebSocket manager for USDT futures.
func NewManager() *Manager {
	return NewManagerWithConfig(DefaultManagerConfig())
}

// NewManagerWithConfig creates a manager with custom config.
func NewManagerWithConfig(cfg ManagerConfig) *Manager {
	m := &Manager{
		cfg:       cfg,
		streamCfg: make(map[string]*symbolStreamCfg),
	}
	m.BaseManager = ws.NewBaseManager("Bitget", cfg.WorkerConfig, func(symbol string) (*ws.BaseClient, error) {
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
		depthChannel: m.cfg.DepthChannel,
		interval:     m.cfg.Interval,
	}
	m.streamCfg[symbol] = sc
	return sc
}

// SubscribeCandle subscribes to candle data using the configured interval.
func (m *Manager) SubscribeCandle(symbol string, handler ws.CandleHandler) error {
	return m.SubscribeCandleWithInterval(symbol, m.cfg.Interval, handler)
}

// SubscribeCandleWithInterval subscribes to candle data with a specific interval.
// Bitget intervals: "1m", "5m", "15m", "30m", "1H", "4H", "6H", "12H", "1D", "1W".
func (m *Manager) SubscribeCandleWithInterval(symbol, interval string, handler ws.CandleHandler) error {
	worker := m.GetOrCreateWorker(symbol)
	client, err := m.GetOrCreateClient(symbol)
	if err != nil {
		return fmt.Errorf("bitget candle %s: %w", symbol, err)
	}

	sc := m.getStreamCfg(symbol)
	sc.interval = interval

	simple := ToSimpleSymbol(symbol)
	SetCandleCallback(symbol, func(msg ws.CandleMsg) {
		worker.EnqueueCandle(msg)
	})

	channel := fmt.Sprintf("candle%s", interval)
	m.ActivateStream(symbol, ws.StreamCandle)
	return SubscribeStream(client, string(m.cfg.InstType), channel, simple)
}

// SubscribeDepth subscribes to depth data using the configured channel.
func (m *Manager) SubscribeDepth(symbol string, handler ws.DepthHandler) error {
	return m.SubscribeDepthWithChannel(symbol, m.cfg.DepthChannel, handler)
}

// SubscribeDepthWithChannel subscribes to depth data with a specific channel.
// channel: DepthFull ("books"), DepthBooks5 ("books5"), DepthBooks15 ("books15").
func (m *Manager) SubscribeDepthWithChannel(symbol string, channel DepthChannel, handler ws.DepthHandler) error {
	worker := m.GetOrCreateWorker(symbol)
	client, err := m.GetOrCreateClient(symbol)
	if err != nil {
		return fmt.Errorf("bitget depth %s: %w", symbol, err)
	}

	sc := m.getStreamCfg(symbol)
	sc.depthChannel = channel

	simple := ToSimpleSymbol(symbol)
	SetDepthCallback(symbol, func(msg ws.DepthMsg) {
		worker.EnqueueDepth(msg)
	})

	m.ActivateStream(symbol, ws.StreamDepth)
	return SubscribeStream(client, string(m.cfg.InstType), string(channel), simple)
}

// SubscribeTrade subscribes to trade data.
func (m *Manager) SubscribeTrade(symbol string, handler ws.TradeHandler) error {
	worker := m.GetOrCreateWorker(symbol)
	client, err := m.GetOrCreateClient(symbol)
	if err != nil {
		return fmt.Errorf("bitget trade %s: %w", symbol, err)
	}

	simple := ToSimpleSymbol(symbol)
	SetTradeCallback(symbol, func(msg ws.TradeMsg) {
		worker.EnqueueTrade(msg)
	})

	m.ActivateStream(symbol, ws.StreamTrade)
	return SubscribeStream(client, string(m.cfg.InstType), "trade", simple)
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
	it := string(m.cfg.InstType)

	for st := range streams {
		switch st {
		case ws.StreamCandle:
			SubscribeStream(client, it, fmt.Sprintf("candle%s", sc.interval), simple)
		case ws.StreamDepth:
			SubscribeStream(client, it, string(sc.depthChannel), simple)
		case ws.StreamTrade:
			SubscribeStream(client, it, "trade", simple)
		}
	}
	return nil
}
