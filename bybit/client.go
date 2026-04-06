package bybit

import (
	"fmt"
	"strings"
	"time"

	"github.com/KhavrTrading/flowex/ws"
	"github.com/valyala/fastjson"

	log "github.com/sirupsen/logrus"
)

// NewClient creates a Bybit V5 linear WebSocket client for one symbol.
func NewClient(symbol string) (*ws.BaseClient, error) {
	cfg := ws.DefaultClientConfig("Bybit", "wss://stream.bybit.com/v5/public/linear")

	// Bybit needs a 15-second ping
	cfg.PingInterval = 15 * time.Second
	cfg.PingMessage = func() ([]byte, error) {
		return []byte(`{"op":"ping"}`), nil
	}

	client := ws.NewBaseClient(symbol, cfg)
	client.SetDispatch(makeDispatcher(client, symbol))

	if err := client.Connect(); err != nil {
		return nil, err
	}
	return client, nil
}

// SubscribeStream subscribes to a named Bybit stream (e.g., "kline.1.BTCUSDT").
func SubscribeStream(client *ws.BaseClient, streamName string) error {
	payload := fmt.Sprintf(`{"op":"subscribe","args":["%s"]}`, streamName)
	return client.WriteMessage([]byte(payload))
}

// Callbacks set by the manager — typed as ws.*Msg for zero-copy dispatch.
var (
	candleCallbacks = make(map[string]func(ws.CandleMsg))
	depthCallbacks  = make(map[string]func(ws.DepthMsg))
	tradeCallbacks  = make(map[string]func(ws.TradeMsg))
)

// SetCandleCallback registers a candle callback for a symbol.
func SetCandleCallback(symbol string, cb func(ws.CandleMsg)) {
	candleCallbacks[symbol] = cb
}

// SetDepthCallback registers a depth callback for a symbol.
func SetDepthCallback(symbol string, cb func(ws.DepthMsg)) {
	depthCallbacks[symbol] = cb
}

// SetTradeCallback registers a trade callback for a symbol.
func SetTradeCallback(symbol string, cb func(ws.TradeMsg)) {
	tradeCallbacks[symbol] = cb
}

func makeDispatcher(client *ws.BaseClient, symbol string) ws.DispatchFunc {
	var p fastjson.Parser
	return func(msg []byte) {
		v, err := p.ParseBytes(msg)
		if err != nil {
			return
		}

		// Pong response
		if string(v.GetStringBytes("op")) == "pong" {
			return
		}

		topic := string(v.GetStringBytes("topic"))

		switch {
		case strings.HasPrefix(topic, "kline."):
			if cb := candleCallbacks[symbol]; cb != nil {
				for _, item := range v.GetArray("data") {
					cb(ws.CandleMsg{
						Timestamp: item.GetInt64("start"),
						Open:      string(item.GetStringBytes("open")),
						High:      string(item.GetStringBytes("high")),
						Low:       string(item.GetStringBytes("low")),
						Close:     string(item.GetStringBytes("close")),
						Volume:    string(item.GetStringBytes("volume")),
					})
				}
			}
		case strings.HasPrefix(topic, "orderbook."):
			if cb := depthCallbacks[symbol]; cb != nil {
				d := v.Get("data")
				if d == nil {
					return
				}
				cb(ws.DepthMsg{
					Bids:      parseStringPairs(d.GetArray("b")),
					Asks:      parseStringPairs(d.GetArray("a")),
					Timestamp: v.GetInt64("ts"),
				})
			}
		case strings.HasPrefix(topic, "publicTrade."):
			if cb := tradeCallbacks[symbol]; cb != nil {
				for _, item := range v.GetArray("data") {
					cb(ws.TradeMsg{
						TradeID:   string(item.GetStringBytes("i")),
						Price:     string(item.GetStringBytes("p")),
						Quantity:  string(item.GetStringBytes("v")),
						Side:      strings.ToLower(string(item.GetStringBytes("S"))),
						Timestamp: item.GetInt64("T"),
					})
				}
			}
		default:
			if topic != "" {
				log.Debugf("[Bybit:%s] unknown topic: %s", symbol, topic)
			}
		}
	}
}

// parseStringPairs converts a fastjson array of [price, qty] pairs to [][]string.
func parseStringPairs(arr []*fastjson.Value) [][]string {
	if len(arr) == 0 {
		return nil
	}
	result := make([][]string, 0, len(arr))
	for _, item := range arr {
		pair := item.GetArray()
		if len(pair) < 2 {
			continue
		}
		result = append(result, []string{
			string(pair[0].GetStringBytes()),
			string(pair[1].GetStringBytes()),
		})
	}
	return result
}

// ToSimpleSymbol converts "BTC/USDT:USDT" to "BTCUSDT".
func ToSimpleSymbol(symbol string) string {
	s := strings.Replace(symbol, "/", "", 1)
	if idx := strings.Index(s, ":"); idx > 0 {
		s = s[:idx]
	}
	return s
}
