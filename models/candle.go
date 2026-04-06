package models

import "strconv"

// CandleHLC represents a candle with High, Low, Close prices (no volume).
type CandleHLC struct {
	ts   int64 // internal unix-ms timestamp of this bar
	High float64
	Low  float64
	Close float64
}

// NewCandleHLCFromSlice parses a CandleHLC from a string slice.
// Expected format: [ts(ms), _, high, low, close, ...].
func NewCandleHLCFromSlice(slice []string) (CandleHLC, error) {
	ts, err := strconv.ParseInt(slice[0], 10, 64)
	if err != nil {
		return CandleHLC{}, err
	}
	high, err := strconv.ParseFloat(slice[2], 64)
	if err != nil {
		return CandleHLC{}, err
	}
	low, err := strconv.ParseFloat(slice[3], 64)
	if err != nil {
		return CandleHLC{}, err
	}
	closeV, err := strconv.ParseFloat(slice[4], 64)
	if err != nil {
		return CandleHLC{}, err
	}
	return CandleHLC{ts: ts, High: high, Low: low, Close: closeV}, nil
}

func (c CandleHLC) GetTimestamp() int64 { return c.ts }
func (c CandleHLC) GetHigh() float64    { return c.High }
func (c CandleHLC) GetLow() float64     { return c.Low }
func (c CandleHLC) GetClose() float64   { return c.Close }

// CandleHLCV represents a candle with Open, High, Low, Close prices and Volume.
type CandleHLCV struct {
	Ts     int64   // Unix-ms timestamp of this bar
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// NewCandleHLCVFromSlice parses a CandleHLCV from a string slice.
// Expected format: [ts(ms), open, high, low, close, volume].
func NewCandleHLCVFromSlice(slice []string) (CandleHLCV, error) {
	ts, err := strconv.ParseInt(slice[0], 10, 64)
	if err != nil {
		return CandleHLCV{}, err
	}
	return NewCandleHLCVFromStrings(ts, slice[1], slice[2], slice[3], slice[4], slice[5])
}

// NewCandleHLCVFromStrings parses a CandleHLCV from individual string fields.
func NewCandleHLCVFromStrings(ts int64, open, high, low, close, volume string) (CandleHLCV, error) {
	openF, err := strconv.ParseFloat(open, 64)
	if err != nil {
		return CandleHLCV{}, err
	}
	highF, err := strconv.ParseFloat(high, 64)
	if err != nil {
		return CandleHLCV{}, err
	}
	lowF, err := strconv.ParseFloat(low, 64)
	if err != nil {
		return CandleHLCV{}, err
	}
	closeF, err := strconv.ParseFloat(close, 64)
	if err != nil {
		return CandleHLCV{}, err
	}
	volumeF, err := strconv.ParseFloat(volume, 64)
	if err != nil {
		return CandleHLCV{}, err
	}
	return CandleHLCV{Ts: ts, Open: openF, High: highF, Low: lowF, Close: closeF, Volume: volumeF}, nil
}

func (c CandleHLCV) GetTimestamp() int64 { return c.Ts }

// HL2 returns (High + Low) / 2.
func (c CandleHLCV) HL2() float64 { return (c.High + c.Low) / 2.0 }

// HLC3 returns (High + Low + Close) / 3.
func (c CandleHLCV) HLC3() float64 { return (c.High + c.Low + c.Close) / 3.0 }
