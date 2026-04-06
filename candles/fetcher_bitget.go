package candles

import (
	"fmt"
	"io"
	"net/http"

	"github.com/KhavrTrading/flowex/models"
	"github.com/valyala/fastjson"
)

// FetchBitgetCandles fetches historical klines from Bitget V2 REST API.
// granularity: "1m", "5m", "15m", "1H", "4H", "1D", etc.
// limit: max 200 per request.
func FetchBitgetCandles(symbol, granularity string, limit int) ([]models.CandleHLCV, error) {
	url := fmt.Sprintf(
		"https://api.bitget.com/api/v2/mix/market/candles?productType=USDT-FUTURES&symbol=%s&granularity=%s&limit=%d",
		symbol, granularity, limit,
	)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("bitget klines request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitget klines status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bitget klines read: %w", err)
	}

	v, err := fastjson.ParseBytes(body)
	if err != nil {
		return nil, fmt.Errorf("bitget klines parse: %w", err)
	}

	if string(v.GetStringBytes("code")) != "00000" {
		return nil, fmt.Errorf("bitget API error: %s", string(v.GetStringBytes("msg")))
	}

	raw := v.GetArray("data")

	// Bitget returns newest first — reverse for chronological order
	candles := make([]models.CandleHLCV, 0, len(raw))
	for i := len(raw) - 1; i >= 0; i-- {
		items := raw[i].GetArray()
		if len(items) < 6 {
			continue
		}
		// Bitget format: [timestamp, open, high, low, close, volume, ...] — all strings
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
