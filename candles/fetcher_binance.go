package candles

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/KhavrTrading/flowex/models"
	"github.com/valyala/fastjson"
)

// FetchBinanceCandles fetches historical klines from Binance Futures REST API.
// interval: "1m", "5m", "15m", "1h", etc.
// limit: max 1500 per request.
func FetchBinanceCandles(symbol, interval string, limit int) ([]models.CandleHLCV, error) {
	url := fmt.Sprintf(
		"https://fapi.binance.com/fapi/v1/klines?symbol=%s&interval=%s&limit=%d",
		symbol, interval, limit,
	)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("binance klines request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("binance klines status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("binance klines read: %w", err)
	}

	v, err := fastjson.ParseBytes(body)
	if err != nil {
		return nil, fmt.Errorf("binance klines parse: %w", err)
	}

	raw := v.GetArray()
	candles := make([]models.CandleHLCV, 0, len(raw))
	for _, row := range raw {
		items := row.GetArray()
		if len(items) < 6 {
			continue
		}
		// Binance: [timestamp(number), open(string), high, low, close, volume, ...]
		ts := items[0].GetInt64()
		c, err := models.NewCandleHLCVFromStrings(ts,
			string(items[1].GetStringBytes()),
			string(items[2].GetStringBytes()),
			string(items[3].GetStringBytes()),
			string(items[4].GetStringBytes()),
			string(items[5].GetStringBytes()),
		)
		if err != nil {
			continue
		}
		candles = append(candles, c)
	}

	return candles, nil
}

// FetchBinanceCandleHLC fetches historical klines as CandleHLC.
func FetchBinanceCandleHLC(symbol, interval string, limit int) ([]models.CandleHLC, error) {
	url := fmt.Sprintf(
		"https://fapi.binance.com/fapi/v1/klines?symbol=%s&interval=%s&limit=%d",
		symbol, interval, limit,
	)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("binance klines request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("binance klines status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("binance klines read: %w", err)
	}

	v, err := fastjson.ParseBytes(body)
	if err != nil {
		return nil, fmt.Errorf("binance klines parse: %w", err)
	}

	raw := v.GetArray()
	candles := make([]models.CandleHLC, 0, len(raw))
	for _, row := range raw {
		items := row.GetArray()
		if len(items) < 6 {
			continue
		}
		slice := []string{
			strconv.FormatInt(items[0].GetInt64(), 10),
			string(items[1].GetStringBytes()), // open (unused but keeps index)
			string(items[2].GetStringBytes()), // high
			string(items[3].GetStringBytes()), // low
			string(items[4].GetStringBytes()), // close
			string(items[5].GetStringBytes()), // volume (unused)
		}
		c, err := models.NewCandleHLCFromSlice(slice)
		if err != nil {
			continue
		}
		candles = append(candles, c)
	}

	return candles, nil
}
