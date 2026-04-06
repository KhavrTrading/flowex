package depth

import "sync"

// Store holds time-bucketed depth metrics for fast pattern detection.
// Thread-safe for concurrent reads and writes.
type Store struct {
	mu sync.RWMutex

	// Time-bucketed storage: timestamp_in_seconds -> metrics in that second.
	// At 100ms updates, each bucket has ~10 metrics per second.
	BySecond map[int64][]DepthMetrics

	// Recent metrics for ultra-fast access (last N metrics).
	Recent []DepthMetrics

	// Tracking for cleanup
	OldestSecond int64
	NewestSecond int64
	TotalMetrics int

	recentCap int // max items in Recent buffer
}

// NewStore creates a new time-bucketed depth metrics store.
func NewStore() *Store {
	return &Store{
		BySecond:  make(map[int64][]DepthMetrics),
		Recent:    make([]DepthMetrics, 0, 100),
		recentCap: 100,
	}
}

// NewStoreWithCap creates a store with a custom recent-buffer capacity.
func NewStoreWithCap(recentCap int) *Store {
	return &Store{
		BySecond:  make(map[int64][]DepthMetrics),
		Recent:    make([]DepthMetrics, 0, recentCap),
		recentCap: recentCap,
	}
}

// Add inserts a new depth metric, maintaining time-bucket and recent buffer limits.
func (s *Store) Add(m DepthMetrics, maxMetrics, maxSeconds int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	second := m.Timestamp / 1000
	s.BySecond[second] = append(s.BySecond[second], m)

	s.Recent = append(s.Recent, m)
	if len(s.Recent) > s.recentCap {
		s.Recent = s.Recent[1:]
	}

	if s.OldestSecond == 0 || second < s.OldestSecond {
		s.OldestSecond = second
	}
	if second > s.NewestSecond {
		s.NewestSecond = second
	}
	s.TotalMetrics++

	s.cleanupLocked(maxMetrics, maxSeconds)
}

// AddAndEnrich inserts a metric and computes velocity/momentum/z-score enrichments.
func (s *Store) AddAndEnrich(m DepthMetrics, maxMetrics, maxSeconds, momentumLookback, zscoreLookback int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Velocity from previous metric
	if len(s.Recent) > 0 {
		prev := &s.Recent[len(s.Recent)-1]
		computeVelocity(prev, &m)
	}

	// Momentum
	if len(s.Recent) >= momentumLookback {
		computeMomentum(s.Recent, &m, momentumLookback)
	}

	// Z-scores
	if len(s.Recent) >= zscoreLookback {
		computeZScores(s.Recent, &m, zscoreLookback)
	}

	second := m.Timestamp / 1000
	s.BySecond[second] = append(s.BySecond[second], m)

	s.Recent = append(s.Recent, m)
	if len(s.Recent) > s.recentCap {
		s.Recent = s.Recent[1:]
	}

	if s.OldestSecond == 0 || second < s.OldestSecond {
		s.OldestSecond = second
	}
	if second > s.NewestSecond {
		s.NewestSecond = second
	}
	s.TotalMetrics++

	s.cleanupLocked(maxMetrics, maxSeconds)
}

func (s *Store) cleanupLocked(maxMetrics, maxSeconds int) {
	if s.NewestSecond-s.OldestSecond > int64(maxSeconds) {
		cutoff := s.NewestSecond - int64(maxSeconds)
		for sec := s.OldestSecond; sec < cutoff; sec++ {
			if metrics, ok := s.BySecond[sec]; ok {
				s.TotalMetrics -= len(metrics)
				delete(s.BySecond, sec)
			}
		}
		s.OldestSecond = cutoff
	}

	for s.TotalMetrics > maxMetrics && s.OldestSecond < s.NewestSecond {
		if metrics, ok := s.BySecond[s.OldestSecond]; ok {
			s.TotalMetrics -= len(metrics)
			delete(s.BySecond, s.OldestSecond)
		}
		s.OldestSecond++
	}
}

// GetLatest returns the most recent depth metric, or nil.
func (s *Store) GetLatest() *DepthMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.Recent) == 0 {
		return nil
	}
	m := s.Recent[len(s.Recent)-1]
	return &m
}

// GetRecent returns a copy of the recent metrics buffer.
func (s *Store) GetRecent() []DepthMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DepthMetrics, len(s.Recent))
	copy(out, s.Recent)
	return out
}

// GetLastNSeconds returns all metrics from the last N seconds.
func (s *Store) GetLastNSeconds(seconds int) []DepthMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.NewestSecond == 0 {
		return nil
	}
	cutoff := s.NewestSecond - int64(seconds)
	result := make([]DepthMetrics, 0, seconds*10)
	for sec := cutoff; sec <= s.NewestSecond; sec++ {
		if metrics, ok := s.BySecond[sec]; ok {
			result = append(result, metrics...)
		}
	}
	return result
}

// GetByTimeRange returns metrics within [startMs, endMs] inclusive.
func (s *Store) GetByTimeRange(startMs, endMs int64) []DepthMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.NewestSecond == 0 || startMs > endMs {
		return nil
	}
	startSec := startMs / 1000
	endSec := endMs / 1000
	if startSec < s.OldestSecond {
		startSec = s.OldestSecond
	}
	if endSec > s.NewestSecond {
		endSec = s.NewestSecond
	}
	if startSec > endSec {
		return nil
	}
	est := (endSec - startSec + 1) * 10
	result := make([]DepthMetrics, 0, est)
	for sec := startSec; sec <= endSec; sec++ {
		for _, m := range s.BySecond[sec] {
			if m.Timestamp >= startMs && m.Timestamp <= endMs {
				result = append(result, m)
			}
		}
	}
	return result
}

// Size returns the total number of stored metrics.
func (s *Store) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalMetrics
}

// GetRecentN returns the last n metrics from the recent buffer.
// If n >= buffer length, returns all metrics.
func (s *Store) GetRecentN(n int) []DepthMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n >= len(s.Recent) {
		out := make([]DepthMetrics, len(s.Recent))
		copy(out, s.Recent)
		return out
	}
	out := make([]DepthMetrics, n)
	copy(out, s.Recent[len(s.Recent)-n:])
	return out
}

// GetMetricsByTimeRangeClamped returns metrics within [startMs, endMs], clamped to available data.
// Alias for GetByTimeRange which already clamps to store bounds.
func (s *Store) GetMetricsByTimeRangeClamped(startMs, endMs int64) []DepthMetrics {
	return s.GetByTimeRange(startMs, endMs)
}

// --- enrichment helpers ---

func fastSqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func computeVelocity(prev, cur *DepthMetrics) {
	dt := float64(cur.Timestamp-prev.Timestamp) / 1000.0
	if dt <= 0 {
		return
	}
	prevLiq10 := prev.BidLiquidity10 + prev.AskLiquidity10
	if prevLiq10 > 0 {
		curLiq10 := cur.BidLiquidity10 + cur.AskLiquidity10
		cur.LiquidityVelocity10 = ((curLiq10 - prevLiq10) / prevLiq10) * 100 / dt
	}
	prevLiq50 := prev.BidLiquidity50 + prev.AskLiquidity50
	if prevLiq50 > 0 {
		curLiq50 := cur.BidLiquidity50 + cur.AskLiquidity50
		cur.LiquidityVelocity50 = ((curLiq50 - prevLiq50) / prevLiq50) * 100 / dt
	}
	cur.ImbalanceVelocity = (cur.ImbalanceRatio10 - prev.ImbalanceRatio10) / dt
	if prev.Spread > 0 {
		cur.SpreadVelocity = ((cur.Spread - prev.Spread) / prev.Spread) * 100 / dt
	}
	maxWall := 0.0
	if d := cur.LargestBidValue - prev.LargestBidValue; d > maxWall {
		maxWall = d
	}
	if d := cur.LargestAskValue - prev.LargestAskValue; d > maxWall {
		maxWall = d
	}
	cur.WallVelocity = maxWall / dt
}

func computeMomentum(recent []DepthMetrics, cur *DepthMetrics, lookback int) {
	start := len(recent) - lookback
	if start < 1 {
		start = 1
	}
	bidInc, askInc := 0, 0
	for i := start; i < len(recent); i++ {
		if recent[i].BidLiquidity10 > recent[i-1].BidLiquidity10 {
			bidInc++
		}
		if recent[i].AskLiquidity10 > recent[i-1].AskLiquidity10 {
			askInc++
		}
	}
	n := len(recent) - start
	if n > 0 {
		cur.BuyPressureMomentum = float64(bidInc) / float64(n) * 100
		cur.SellPressureMomentum = float64(askInc) / float64(n) * 100
	}

	// Wall building: 3 consecutive increases
	if len(recent) >= 3 {
		bidWall, askWall := true, true
		for i := len(recent) - 2; i >= len(recent)-3 && i >= 1; i-- {
			if recent[i].LargestBidValue <= recent[i-1].LargestBidValue {
				bidWall = false
			}
			if recent[i].LargestAskValue <= recent[i-1].LargestAskValue {
				askWall = false
			}
		}
		cur.WallBuildingBid = bidWall
		cur.WallBuildingAsk = askWall
	}
}

func computeZScores(recent []DepthMetrics, cur *DepthMetrics, lookback int) {
	start := len(recent) - lookback
	if start < 0 {
		start = 0
	}
	n := float64(len(recent) - start)
	var sumLiq, sumImb, sumSpread float64
	for i := start; i < len(recent); i++ {
		sumLiq += recent[i].BidLiquidity10 + recent[i].AskLiquidity10
		sumImb += recent[i].ImbalanceRatio10
		sumSpread += recent[i].Spread
	}
	meanLiq := sumLiq / n
	meanImb := sumImb / n
	meanSpread := sumSpread / n

	var sqLiq, sqImb, sqSpread float64
	for i := start; i < len(recent); i++ {
		liq := recent[i].BidLiquidity10 + recent[i].AskLiquidity10
		sqLiq += (liq - meanLiq) * (liq - meanLiq)
		sqImb += (recent[i].ImbalanceRatio10 - meanImb) * (recent[i].ImbalanceRatio10 - meanImb)
		sqSpread += (recent[i].Spread - meanSpread) * (recent[i].Spread - meanSpread)
	}

	stdLiq := fastSqrt(sqLiq / n)
	stdImb := fastSqrt(sqImb / n)
	stdSpread := fastSqrt(sqSpread / n)

	curLiq := cur.BidLiquidity10 + cur.AskLiquidity10
	if stdLiq > 0 {
		cur.LiquidityZScore10 = (curLiq - meanLiq) / stdLiq
	}
	if stdImb > 0 {
		cur.ImbalanceZScore = (cur.ImbalanceRatio10 - meanImb) / stdImb
	}
	if stdSpread > 0 {
		cur.SpreadZScore = (cur.Spread - meanSpread) / stdSpread
	}
}
