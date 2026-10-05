package execution

import (
	"errors"
	"testing"
	"time"
)

func TestHappyPath(t *testing.T) {
	now := time.Unix(1_000, 0)
	tx, err := NewTransaction("arb-1", "ALTUSDT", 10, now)
	if err != nil {
		t.Fatal(err)
	}
	path := []State{StateValidated, StateFirstSubmitting, StateFirstFilled, StateHedging, StateHedged, StateReconciling, StateCompleted}
	for _, next := range path {
		if err := tx.Transition(next, now.Add(time.Second)); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
}

func TestCannotCompleteWithoutReconciliation(t *testing.T) {
	tx, _ := NewTransaction("arb-1", "ALTUSDT", 10, time.Now())
	if err := tx.Transition(StateCompleted, time.Now()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}

func TestExposureTracksUnhedgedQuantity(t *testing.T) {
	tx := Transaction{FirstFilled: 8, HedgeFilled: 5}
	if tx.Exposure() != 3 {
		t.Fatalf("expected exposure 3, got %v", tx.Exposure())
	}
}
