package tracker

import (
	"testing"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/scanner"
)

func TestTrackerRequiresSamplesAndDuration(t *testing.T) {
	start := time.Unix(1_000, 0)
	tracker := New(Config{MinSamples: 3, MinDuration: 2 * time.Second, ExpireAfter: 10 * time.Second})
	opportunity := scanner.Opportunity{Symbol: "ALTUSDT", BuyExchange: "bybit", SellExchange: "bitget"}

	if got := tracker.Update(start, []scanner.Opportunity{opportunity}); len(got) != 0 {
		t.Fatalf("first sample must not confirm")
	}
	if got := tracker.Update(start.Add(time.Second), []scanner.Opportunity{opportunity}); len(got) != 0 {
		t.Fatalf("second sample must not confirm")
	}
	got := tracker.Update(start.Add(2*time.Second), []scanner.Opportunity{opportunity})
	if len(got) != 1 || !got[0].Confirmed {
		t.Fatalf("third sample after duration must confirm: %+v", got)
	}
}

func TestTrackerRestartsExpiredSignal(t *testing.T) {
	start := time.Unix(1_000, 0)
	tracker := New(Config{MinSamples: 2, ExpireAfter: 5 * time.Second})
	opportunity := scanner.Opportunity{Symbol: "ALTUSDT", BuyExchange: "bybit", SellExchange: "bitget"}

	tracker.Update(start, []scanner.Opportunity{opportunity})
	got := tracker.Update(start.Add(6*time.Second), []scanner.Opportunity{opportunity})
	if len(got) != 0 {
		t.Fatalf("expired signal must restart confirmation")
	}
}
