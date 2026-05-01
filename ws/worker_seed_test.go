package ws

import (
	"strings"
	"testing"
	"time"

	"github.com/KhavrTrading/flowex/models"
)

func makeSyntheticCandles(n int, startTs int64, basePrice float64) []models.CandleHLCV {
	out := make([]models.CandleHLCV, n)
	for i := range n {
		p := basePrice + float64(i)
		out[i] = models.CandleHLCV{
			Ts:     startTs + int64(i)*60_000,
			Open:   p,
			High:   p + 1,
			Low:    p - 1,
			Close:  p,
			Volume: 100,
		}
	}
	return out
}

func TestSeedCandlesDirect_PopulatesSnapshot(t *testing.T) {
	cfg := DefaultWorkerConfig()
	cfg.SnapshotInterval = 25 * time.Millisecond
	w := NewSymbolWorker("TESTUSDT", cfg)
	defer w.Stop()

	seed := makeSyntheticCandles(50, 1_700_000_000_000, 100.0)
	w.SeedCandlesDirect(seed)

	got := w.GetCandles()
	if len(got) != 50 {
		t.Fatalf("expected 50 candles in snapshot, got %d", len(got))
	}
	if w.GetIndicators() == nil {
		t.Fatal("expected indicators to be computed for 50-candle seed (>= 14)")
	}
}

func TestSeedCandlesDirect_TrimsToMaxCandles(t *testing.T) {
	cfg := DefaultWorkerConfig()
	cfg.MaxCandles = 100
	cfg.SnapshotInterval = 25 * time.Millisecond
	w := NewSymbolWorker("TESTUSDT", cfg)
	defer w.Stop()

	seed := makeSyntheticCandles(250, 1_700_000_000_000, 100.0)
	w.SeedCandlesDirect(seed)

	got := w.GetCandles()
	if len(got) != 100 {
		t.Fatalf("expected trim to 100 candles, got %d", len(got))
	}
	// Must keep the most recent — first kept candle is index 150 of seed.
	if got[0].Ts != seed[150].Ts {
		t.Fatalf("expected first kept Ts=%d (seed[150]), got %d", seed[150].Ts, got[0].Ts)
	}
	if got[len(got)-1].Ts != seed[len(seed)-1].Ts {
		t.Fatalf("expected last Ts=%d, got %d", seed[len(seed)-1].Ts, got[len(got)-1].Ts)
	}
}

func TestApplyCandle_OutOfOrderRecorded(t *testing.T) {
	cfg := DefaultWorkerConfig()
	cfg.SnapshotInterval = 25 * time.Millisecond
	w := NewSymbolWorker("TESTUSDT", cfg)
	defer w.Stop()

	baseTs := int64(1_700_000_000_000)
	enqueue := func(ts int64) {
		w.EnqueueCandle(CandleMsg{
			Timestamp: ts,
			Open:      "100", High: "101", Low: "99", Close: "100", Volume: "1",
		})
	}
	enqueue(baseTs)
	enqueue(baseTs + 60_000)
	enqueue(baseTs - 60_000) // out-of-order — must be recorded as a drop

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if w.GetMetrics()["candle_dropped"] >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if got := w.GetMetrics()["candle_dropped"]; got < 1 {
		t.Fatalf("expected candle_dropped >= 1, got %d", got)
	}
	errs := w.GetRecentErrors()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "out-of-order") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an 'out-of-order' entry in recent errors, got %v", errs)
	}
}

// Sanity: ensure SeedCandlesDirect on empty input is a no-op.
func TestSeedCandlesDirect_EmptyNoop(t *testing.T) {
	cfg := DefaultWorkerConfig()
	cfg.SnapshotInterval = 25 * time.Millisecond
	w := NewSymbolWorker("TESTUSDT", cfg)
	defer w.Stop()

	w.SeedCandlesDirect(nil)
	w.SeedCandlesDirect([]models.CandleHLCV{})

	if got := w.GetCandles(); len(got) != 0 {
		t.Fatalf("expected 0 candles, got %d (%v)", len(got), got)
	}
	if w.GetIndicators() != nil {
		t.Fatal("expected indicators to be nil after empty seed")
	}
}

