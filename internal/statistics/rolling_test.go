package statistics

import (
	"math"
	"testing"
)

func TestRollingWindowEvictsOldValues(t *testing.T) {
	rolling := NewRolling(3)
	rolling.Add(1)
	rolling.Add(2)
	rolling.Add(3)
	snapshot := rolling.Add(6)
	if snapshot.Count != 3 || math.Abs(snapshot.Mean-11.0/3.0) > 1e-9 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}
