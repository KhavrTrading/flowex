package models

// NormalizedTrade represents a trade from any exchange in a unified format.
type NormalizedTrade struct {
	Timestamp int64   `json:"timestamp"` // Unix timestamp in milliseconds
	Price     float64 `json:"price"`
	Size      float64 `json:"size"`     // Base-currency size
	SizeUSD   float64 `json:"size_usd"` // Notional in USD
	Side      string  `json:"side"`     // "buy" or "sell"
	TradeID   string  `json:"trade_id"`
	Symbol    string  `json:"symbol"`
	Exchange  string  `json:"exchange"` // "binance" | "bybit" | "bitget"
}
