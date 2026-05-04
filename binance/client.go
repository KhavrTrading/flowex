package binance

import (
	"fmt"
	"strings"
	"time"

	"github.com/KhavrTrading/flowex/ws"
	"github.com/valyala/fastjson"

	log "github.com/sirupsen/logrus"
)

// NewClient creates a Binance futures WebSocket client for one symbol.
func NewClient(symbol string) (*ws.BaseClient, error) {
	cfg := ws.DefaultClientConfig("Binance", "wss://fstream.binance.com/market/ws")
	// Binance doesn't need application-level ping (server pings at protocol level)

	client := ws.NewBaseClient(symbol, cfg)
	client.SetDispatch(makeDispatcher(client, symbol))

	if err := client.Connect(); err != nil {
		return nil, err
	}
	return client, nil
}

// SubscribeStream subscribes to a named Binance stream (e.g., "btcusdt@kline_1m").
func SubscribeStream(client *ws.BaseClient, streamName string, id int) error {
	payload := fmt.Sprintf(`{"method":"SUBSCRIBE","params":["%s"],"id":%d}`, streamName, id)
	return client.WriteMessage([]byte(payload))
}

// CandleStreamName returns the Binance stream name for candles.
// interval: "1m", "3m", "5m", "15m", "30m", "1h", "4h", "1d", etc.
func CandleStreamName(symbol, interval string) string {
	return fmt.Sprintf("%s@kline_%s", strings.ToLower(symbol), interval)
}

// DepthStreamName returns the Binance stream name for partial depth snapshots.
// levels: 5, 10, or 20. speed: "100ms", "250ms", or "500ms" (empty = default).
func DepthStreamName(symbol string, levels int, speed string) string {
	lower := strings.ToLower(symbol)
	if speed != "" {
		return fmt.Sprintf("%s@depth%d@%s", lower, levels, speed)
	}
	return fmt.Sprintf("%s@depth%d", lower, levels)
}

// DiffDepthStreamName returns the Binance stream name for incremental depth updates.
// speed: "100ms", "250ms", "500ms" (empty = default 250ms).
func DiffDepthStreamName(symbol, speed string) string {
	lower := strings.ToLower(symbol)
	if speed != "" {
		return fmt.Sprintf("%s@depth@%s", lower, speed)
	}
	return fmt.Sprintf("%s@depth", lower)
}

// AggTradeStreamName returns the stream name for aggregate trades (default).
func AggTradeStreamName(symbol string) string {
	return fmt.Sprintf("%s@aggTrade", strings.ToLower(symbol))
}

// TradeStreamName returns the stream name for individual trades.
func TradeStreamName(symbol string) string {
	return fmt.Sprintf("%s@trade", strings.ToLower(symbol))
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

		eventType := string(v.GetStringBytes("e"))

		// Detect depth snapshots (no "e" field but has "bids"/"asks")
		if eventType == "" {
			if v.Exists("bids") && v.Exists("asks") {
				eventType = "depthSnapshot"
			}
		}

		switch eventType {
		case "kline":
			if cb := candleCallbacks[symbol]; cb != nil {
				k := v.Get("k")
				if k == nil {
					return
				}
				cb(ws.CandleMsg{
					Timestamp: k.GetInt64("t"),
					Open:      string(k.GetStringBytes("o")),
					High:      string(k.GetStringBytes("h")),
					Low:       string(k.GetStringBytes("l")),
					Close:     string(k.GetStringBytes("c")),
					Volume:    string(k.GetStringBytes("v")),
				})
			}
		case "depthUpdate", "depthSnapshot":
			if cb := depthCallbacks[symbol]; cb != nil {
				bids := parseStringPairs(v.GetArray("b"))
				if bids == nil {
					bids = parseStringPairs(v.GetArray("bids"))
				}
				asks := parseStringPairs(v.GetArray("a"))
				if asks == nil {
					asks = parseStringPairs(v.GetArray("asks"))
				}
				eventTime := v.GetInt64("E")
				if eventTime == 0 {
					eventTime = time.Now().UnixMilli()
				}
				cb(ws.DepthMsg{
					Bids:      bids,
					Asks:      asks,
					Timestamp: eventTime,
				})
			}
		case "aggTrade", "trade":
			if cb := tradeCallbacks[symbol]; cb != nil {
				cb(ws.TradeMsg{
					TradeID:      fmt.Sprintf("%d", v.GetInt64("t")),
					Price:        string(v.GetStringBytes("p")),
					Quantity:     string(v.GetStringBytes("q")),
					IsBuyerMaker: v.GetBool("m"),
					Timestamp:    v.GetInt64("T"),
				})
			}
		default:
			if eventType != "" {
				log.Debugf("[Binance:%s] unknown event: %s", symbol, eventType)
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
