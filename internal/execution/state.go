package execution

import (
	"errors"
	"fmt"
	"time"
)

type State string

const (
	StateDetected         State = "DETECTED"
	StateValidated        State = "VALIDATED"
	StateFirstSubmitting  State = "FIRST_LEG_SUBMITTING"
	StateFirstPartial     State = "FIRST_LEG_PARTIAL"
	StateFirstFilled      State = "FIRST_LEG_FILLED"
	StateHedging          State = "HEDGING"
	StateHedged           State = "HEDGED"
	StateReconciling      State = "RECONCILING"
	StateCompleted        State = "COMPLETED"
	StateUnknownOrder     State = "UNKNOWN_ORDER"
	StateUnhedged         State = "UNHEDGED"
	StateRecoveryRequired State = "RECOVERY_REQUIRED"
	StateHalted           State = "HALTED"
)

var ErrInvalidTransition = errors.New("invalid execution state transition")

type Transaction struct {
	ID             string    `json:"id"`
	Symbol         string    `json:"symbol"`
	State          State     `json:"state"`
	TargetQuantity float64   `json:"target_quantity"`
	FirstFilled    float64   `json:"first_filled"`
	HedgeFilled    float64   `json:"hedge_filled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func NewTransaction(id, symbol string, quantity float64, now time.Time) (Transaction, error) {
	if id == "" || symbol == "" || quantity <= 0 {
		return Transaction{}, errors.New("id, symbol and positive quantity are required")
	}
	return Transaction{ID: id, Symbol: symbol, State: StateDetected, TargetQuantity: quantity, CreatedAt: now, UpdatedAt: now}, nil
}

func (t *Transaction) Transition(next State, now time.Time) error {
	if !allowed(t.State, next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, t.State, next)
	}
	t.State = next
	t.UpdatedAt = now
	return nil
}

func (t Transaction) Exposure() float64 { return t.FirstFilled - t.HedgeFilled }

func allowed(current, next State) bool {
	allowedTransitions := map[State]map[State]bool{
		StateDetected:        {StateValidated: true, StateHalted: true},
		StateValidated:       {StateFirstSubmitting: true, StateHalted: true},
		StateFirstSubmitting: {StateFirstPartial: true, StateFirstFilled: true, StateUnknownOrder: true, StateHalted: true},
		StateFirstPartial:    {StateHedging: true, StateUnknownOrder: true, StateUnhedged: true},
		StateFirstFilled:     {StateHedging: true, StateUnknownOrder: true, StateUnhedged: true},
		StateHedging:         {StateHedged: true, StateUnhedged: true, StateUnknownOrder: true},
		StateHedged:          {StateReconciling: true, StateRecoveryRequired: true},
		StateReconciling:     {StateCompleted: true, StateRecoveryRequired: true},
		StateUnknownOrder:    {StateRecoveryRequired: true, StateHedging: true, StateHalted: true},
		StateUnhedged:        {StateHedging: true, StateRecoveryRequired: true, StateHalted: true},
		StateRecoveryRequired: {StateHedging: true, StateReconciling: true, StateHalted: true},
	}
	return allowedTransitions[current][next]
}
