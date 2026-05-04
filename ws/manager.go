package ws

import (
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

// ClientFactory creates a new BaseClient for a symbol and connects it.
type ClientFactory func(symbol, clientKey string) (*BaseClient, error)

// BaseManager manages per-symbol workers and WebSocket clients.
// It provides the subscribe/unsubscribe API and handles worker/client lifecycle.
type BaseManager struct {
	mu            sync.RWMutex
	clients       map[string]*BaseClient          // key: clientKey
	workers       map[string]*SymbolWorker        // key: symbol
	activeStreams map[string]map[StreamType]bool  // key: clientKey
	symbolClients map[string]map[string]bool      // symbol -> set of clientKeys
	workerConfig  WorkerConfig
	clientFactory ClientFactory
	label         string
}

// NewBaseManager creates a new manager with the given config.
func NewBaseManager(label string, wcfg WorkerConfig, factory ClientFactory) *BaseManager {
	return &BaseManager{
		clients:       make(map[string]*BaseClient),
		workers:       make(map[string]*SymbolWorker),
		activeStreams: make(map[string]map[StreamType]bool),
		symbolClients: make(map[string]map[string]bool),
		workerConfig:  wcfg,
		clientFactory: factory,
		label:         label,
	}
}

// GetOrCreateWorker returns an existing worker or creates a new one.
func (m *BaseManager) GetOrCreateWorker(symbol string) *SymbolWorker {
	m.mu.RLock()
	w, ok := m.workers[symbol]
	m.mu.RUnlock()
	if ok {
		return w
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.workers[symbol]; ok {
		return w
	}

	w = NewSymbolWorker(symbol, m.workerConfig)
	m.workers[symbol] = w
	return w
}

// GetOrCreateClient is a helper that uses symbol as the clientKey.
func (m *BaseManager) GetOrCreateClient(symbol string) (*BaseClient, error) {
	return m.GetOrCreateClientByKey(symbol, symbol)
}

// GetOrCreateClientByKey returns an existing client or creates, connects, and starts one.
func (m *BaseManager) GetOrCreateClientByKey(symbol, clientKey string) (*BaseClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.clients[clientKey]; ok {
		return c, nil
	}

	c, err := m.clientFactory(symbol, clientKey)
	if err != nil {
		return nil, fmt.Errorf("create client %s (%s): %w", symbol, clientKey, err)
	}

	m.clients[clientKey] = c
	if m.activeStreams[clientKey] == nil {
		m.activeStreams[clientKey] = make(map[StreamType]bool)
	}
	if m.symbolClients[symbol] == nil {
		m.symbolClients[symbol] = make(map[string]bool)
	}
	m.symbolClients[symbol][clientKey] = true

	go c.ListenLoop()
	return c, nil
}

// ActivateStream is a helper that uses symbol as the clientKey.
func (m *BaseManager) ActivateStream(symbol string, st StreamType) {
	m.ActivateStreamByKey(symbol, symbol, st)
}

// ActivateStreamByKey marks a stream as active (idempotent) for a specific clientKey.
func (m *BaseManager) ActivateStreamByKey(symbol, clientKey string, st StreamType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeStreams[clientKey] == nil {
		m.activeStreams[clientKey] = make(map[StreamType]bool)
	}
	m.activeStreams[clientKey][st] = true
}

// DeactivateStream is a helper that uses symbol as the clientKey.
func (m *BaseManager) DeactivateStream(symbol string, st StreamType) {
	m.DeactivateStreamByKey(symbol, symbol, st)
}

// DeactivateStreamByKey removes a stream. If no streams remain for the clientKey, the client is stopped.
// If no clients remain for the symbol, the worker is stopped.
func (m *BaseManager) DeactivateStreamByKey(symbol, clientKey string, st StreamType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeStreams[clientKey] == nil {
		return
	}
	delete(m.activeStreams[clientKey], st)

	if len(m.activeStreams[clientKey]) == 0 {
		if c, ok := m.clients[clientKey]; ok {
			c.Stop()
			c.Close()
			delete(m.clients, clientKey)
		}
		delete(m.activeStreams, clientKey)
		
		if sc := m.symbolClients[symbol]; sc != nil {
			delete(sc, clientKey)
			if len(sc) == 0 {
				delete(m.symbolClients, symbol)
				if w, ok := m.workers[symbol]; ok {
					w.Stop()
					delete(m.workers, symbol)
				}
			}
		}
	}
}

// GetActiveStreams is a helper that uses symbol as the clientKey.
func (m *BaseManager) GetActiveStreams(symbol string) map[StreamType]bool {
	return m.GetActiveStreamsByKey(symbol)
}

// GetActiveStreamsByKey returns a copy of active streams for a specific clientKey.
func (m *BaseManager) GetActiveStreamsByKey(clientKey string) map[StreamType]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[StreamType]bool)
	for k, v := range m.activeStreams[clientKey] {
		out[k] = v
	}
	return out
}

// GetSnapshot returns the snapshot for a symbol, or nil.
func (m *BaseManager) GetSnapshot(symbol string) *Snapshot {
	m.mu.RLock()
	w, ok := m.workers[symbol]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return w.GetSnapshot()
}

// GetStatus returns a summary of active symbols, streams, and metrics.
func (m *BaseManager) GetStatus() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	symbols := make([]string, 0, len(m.workers))
	for s := range m.workers {
		symbols = append(symbols, s)
	}

	metrics := make(map[string]map[string]int64)
	for s, w := range m.workers {
		metrics[s] = w.GetMetrics()
	}

	return map[string]any{
		"label":   m.label,
		"symbols": symbols,
		"metrics": metrics,
	}
}

// Shutdown stops all clients and workers.
func (m *BaseManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, c := range m.clients {
		c.Stop()
		c.Close()
	}
	for _, w := range m.workers {
		w.Stop()
	}
	m.clients = make(map[string]*BaseClient)
	m.workers = make(map[string]*SymbolWorker)
	m.activeStreams = make(map[string]map[StreamType]bool)
	m.symbolClients = make(map[string]map[string]bool)

	log.Infof("[%s] Manager shut down", m.label)
}

