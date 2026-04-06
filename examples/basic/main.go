package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KhavrTrading/flowex/binance"

	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetLevel(log.InfoLevel)

	mgr := binance.NewManager()

	symbols := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "XRPUSDT", "DOGEUSDT"}

	for _, sym := range symbols {
		if err := mgr.SubscribeAll(sym, nil, nil, nil); err != nil {
			log.Fatalf("Subscribe %s failed: %v", sym, err)
		}
		fmt.Printf("Subscribed to %s\n", sym)
	}

	start := time.Now()

	// Print stats every 5 seconds
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			elapsed := time.Since(start).Seconds()
			fmt.Printf("\n=== %.0fs elapsed ===\n", elapsed)
			var totalProcessed, totalDropped int64
			for _, sym := range symbols {
				worker := mgr.GetOrCreateWorker(sym)
				snap := worker.GetSnapshot()
				if snap == nil {
					continue
				}
				m := worker.GetMetrics()
				proc := m["processed"]
				drops := m["candle_dropped"] + m["depth_dropped"] + m["trade_dropped"]
				totalProcessed += proc
				totalDropped += drops
				fmt.Printf("  %-10s candles=%d depth=%d trades=%d | processed=%d dropped=%d\n",
					sym,
					len(snap.Candles),
					snap.DepthStore.Size(),
					len(snap.Trades),
					proc, drops,
				)
			}
			fmt.Printf("  TOTAL: %d msgs (%.0f/s) | dropped: %d\n",
				totalProcessed,
				float64(totalProcessed)/elapsed,
				totalDropped,
			)
		}
	}()

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
	mgr.Shutdown()
}
