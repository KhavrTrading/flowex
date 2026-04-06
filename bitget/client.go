package bitget

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/KhavrTrading/flowex/ws"
	"github.com/valyala/fastjson"

	log "github.com/sirupsen/logrus"
)

// NewClient creates a Bitget WebSocket client for one symbol.
func NewClient(symbol string) (*ws.BaseClient, error) {
	cfg := ws.DefaultClientConfig("Bitget", "wss://ws.bitget.com/v2/ws/public")

	// Bitget needs a 20-second ping (plain "ping" string, not JSON)
	cfg.PingInterval = 20 * time.Second
	cfg.PingMessage = func() ([]byte, error) {
		return []byte("ping"), nil
	}

	client := ws.NewBaseClient(symbol, cfg)
	client.SetDispatch(makeDispatcher(client, symbol))

	if err := client.Connect(); err != nil {
		return nil, err
	}
	return client, nil
}

// SubscribeStream subscribes to a Bitget stream.
func SubscribeStream(client *ws.BaseClient, instType, channel, instId string) error {
	payload := fmt.Sprintf(`{"op":"subscribe","args":[{"instType":"%s","channel":"%s","instId":"%s"}]}`,
		instType, channel, instId)
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
		// Bitget sends "pong" as plain text
		if len(msg) == 4 && string(msg) == "pong" {
			return
		}

		v, err := p.ParseBytes(msg)
		if err != nil {
			return
		}

		arg := v.Get("arg")
		if arg == nil {
			return
		}
		channel := string(arg.GetStringBytes("channel"))

		switch {
		case strings.HasPrefix(channel, "candle"):
			if cb := candleCallbacks[symbol]; cb != nil {
				for _, item := range v.GetArray("data") {
					row := item.GetArray()
					if len(row) < 6 {
						continue
					}
					ts, _ := strconv.ParseInt(string(row[0].GetStringBytes()), 10, 64)
					cb(ws.CandleMsg{
						Timestamp: ts,
						Open:      string(row[1].GetStringBytes()),
						High:      string(row[2].GetStringBytes()),
						Low:       string(row[3].GetStringBytes()),
						Close:     string(row[4].GetStringBytes()),
						Volume:    string(row[5].GetStringBytes()),
					})
				}
			}
		case strings.HasPrefix(channel, "books"):
			if cb := depthCallbacks[symbol]; cb != nil {
				for _, item := range v.GetArray("data") {
					ts, _ := strconv.ParseInt(string(item.GetStringBytes("ts")), 10, 64)
					cb(ws.DepthMsg{
						Bids:      parseStringPairs(item.GetArray("bids")),
						Asks:      parseStringPairs(item.GetArray("asks")),
						Timestamp: ts,
					})
				}
			}
		case channel == "trade":
			if cb := tradeCallbacks[symbol]; cb != nil {
				for _, item := range v.GetArray("data") {
					ts, _ := strconv.ParseInt(string(item.GetStringBytes("ts")), 10, 64)
					cb(ws.TradeMsg{
						TradeID:   string(item.GetStringBytes("tradeId")),
						Price:     string(item.GetStringBytes("price")),
						Quantity:  string(item.GetStringBytes("size")),
						Side:      string(item.GetStringBytes("side")),
						Timestamp: ts,
					})
				}
			}
		default:
			log.Debugf("[Bitget:%s] unknown channel: %s", symbol, channel)
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

// ToSimpleSymbol converts "BTCUSDT:USDT" to "BTCUSDT".
func ToSimpleSymbol(symbol string) string {
	if idx := strings.Index(symbol, ":"); idx > 0 {
		return symbol[:idx]
	}
	return symbol
}

// ParseTimestampMs parses a string timestamp to int64 milliseconds.
func ParseTimestampMs(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}
