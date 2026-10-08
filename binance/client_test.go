package binance

import (
	"testing"

	"github.com/KhavrTrading/flowex/ws"
)

// aggTrade payloads carry the aggregate id in "a" (no "t"); plain trade payloads carry "t".
// Reading "t" for both made every aggTrade id "0", so downstream could never dedupe trades.
func TestDispatcher_TradeIDPerEventType(t *testing.T) {
	const sym = "TESTUSDT"
	var got []ws.TradeMsg
	SetTradeCallback(sym, func(m ws.TradeMsg) { got = append(got, m) })
	defer delete(tradeCallbacks, sym)

	dispatch := makeDispatcher(nil, sym)
	dispatch([]byte(`{"e":"aggTrade","E":1700000000100,"s":"TESTUSDT","a":5933014,"p":"0.087","q":"100","f":100,"l":105,"T":1700000000090,"m":true}`))
	dispatch([]byte(`{"e":"trade","E":1700000000200,"T":1700000000190,"s":"TESTUSDT","t":4242,"p":"0.088","q":"5","X":"MARKET","m":false}`))

	if len(got) != 2 {
		t.Fatalf("got %d trades, want 2", len(got))
	}
	if got[0].TradeID != "5933014" {
		t.Errorf("aggTrade TradeID = %q, want %q (field \"a\")", got[0].TradeID, "5933014")
	}
	if got[1].TradeID != "4242" {
		t.Errorf("trade TradeID = %q, want %q (field \"t\")", got[1].TradeID, "4242")
	}
	if got[0].Timestamp != 1700000000090 || got[0].Price != "0.087" || !got[0].IsBuyerMaker {
		t.Errorf("aggTrade = %+v, want T/p/m carried through", got[0])
	}
}
