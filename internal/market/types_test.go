package market

import "testing"

func TestCommonQuantityRespectsBothVenues(t *testing.T) {
	bybit := Instrument{QuantityStep: .1, MinimumQty: .2, MaximumMarketQty: 100}
	bitget := Instrument{QuantityStep: .01, MinimumQty: .05, MinimumNotional: 25, MaximumMarketQty: 50}
	quantity, ok := CommonQuantity(20, 100, bybit, bitget)
	if !ok || quantity != .3 {
		t.Fatalf("expected quantity 0.3, got %v ok=%v", quantity, ok)
	}
}

func TestCommonQuantityRejectsMaximum(t *testing.T) {
	instrument := Instrument{QuantityStep: 1, MinimumQty: 1, MaximumMarketQty: 2}
	if _, ok := CommonQuantity(1_000, 100, instrument); ok {
		t.Fatal("expected quantity above maximum to be rejected")
	}
}
