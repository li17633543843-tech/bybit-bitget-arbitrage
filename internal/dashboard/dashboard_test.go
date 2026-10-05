package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/paper"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/strategy"
)

func TestStatusHandlerReturnsCurrentState(t *testing.T) {
	state := New()
	state.RecordOpportunity(time.Now(), strategy.DepthOpportunity{Symbol: "ALTUSDT", BuyExchange: "bybit", SellExchange: "bitget", NetEdgeBps: 12.5})
	state.RecordPaper(paper.Snapshot{RealizedPnL: 1.25, Trades: 2}, []paper.Event{{Type: "CLOSE", Time: time.Now(), Position: paper.Position{Symbol: "ALTUSDT"}}})

	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	response := httptest.NewRecorder()
	state.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	var payload Payload
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Snapshot.Trades != 2 || len(payload.Opportunities) != 1 || len(payload.RecentEvents) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestDashboardPageIsServed(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	New().Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.Len() == 0 {
		t.Fatalf("dashboard page was not served: status=%d bytes=%d", response.Code, response.Body.Len())
	}
}
