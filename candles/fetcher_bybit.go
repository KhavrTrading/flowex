package candles

import (
	"fmt"
	"io"
	"net/http"

	"github.com/KhavrTrading/flowex/models"
	"github.com/valyala/fastjson"
)

// FetchBybitCandles fetches historical klines from Bybit V5 REST API.
// interval: "1" (1m), "5" (5m), "15", "60", "240", "D", "W".
// limit: max 200 per request.
func FetchBybitCandles(symbol, interval string, limit int) ([]models.CandleHLCV, error) {
	url := fmt.Sprintf(
		"https://api.bybit.com/v5/market/kline?category=linear&symbol=%s&interval=%s&limit=%d",
		symbol, interval, limit,
	)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("bybit klines request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bybit klines status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bybit klines read: %w", err)
	}

	v, err := fastjson.ParseBytes(body)
	if err != nil {
		return nil, fmt.Errorf("bybit klines parse: %w", err)
	}

	if v.GetInt("retCode") != 0 {
		return nil, fmt.Errorf("bybit API error: %s", string(v.GetStringBytes("retMsg")))
	}

	raw := v.Get("result").GetArray("list")

	// Bybit returns newest first — reverse for chronological order
	candles := make([]models.CandleHLCV, 0, len(raw))
	for i := len(raw) - 1; i >= 0; i-- {
		items := raw[i].GetArray()
		if len(items) < 6 {
			continue
		}
		// Bybit format: [timestamp, open, high, low, close, volume, turnover] — all strings
		slice := make([]string, 6)
		for j := 0; j < 6; j++ {
			slice[j] = string(items[j].GetStringBytes())
		}
		c, err := models.NewCandleHLCVFromSlice(slice)
		if err != nil {
			continue
		}
		candles = append(candles, c)
	}

	return candles, nil
}
