package game

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRemainingBudget(t *testing.T) {
	if got := RemainingBudget(6*time.Second, time.Second); got != 5*time.Second {
		t.Fatalf("remaining = %v, want 5s", got)
	}
	if got := RemainingBudget(300*time.Millisecond, 2*time.Second); got != 200*time.Millisecond {
		t.Fatalf("over-budget fallback = %v, want the 200ms floor", got)
	}
}

func TestMoveInputAcceptsIndexesAndCoordObjects(t *testing.T) {
	cases := []struct {
		raw  string
		from int
		to   int
		idx  int
	}{
		{`{"from":12,"to":20}`, 12, 20, -1},
		{`{"index":100}`, -1, -1, 100},
		{`{"from":{"index":12,"row":1,"col":4},"to":{"index":20,"row":2,"col":4}}`, 12, 20, -1},
		{`{"index":{"index":100,"row":12,"col":4}}`, -1, -1, 100},
	}
	for _, c := range cases {
		var in MoveInput
		if err := json.Unmarshal([]byte(c.raw), &in); err != nil {
			t.Fatalf("%s: %v", c.raw, err)
		}
		if c.from >= 0 && (in.From == nil || *in.From != c.from) {
			t.Fatalf("%s: from = %v, want %d", c.raw, in.From, c.from)
		}
		if c.to >= 0 && (in.To == nil || *in.To != c.to) {
			t.Fatalf("%s: to = %v, want %d", c.raw, in.To, c.to)
		}
		if c.idx >= 0 && (in.Index == nil || *in.Index != c.idx) {
			t.Fatalf("%s: index = %v, want %d", c.raw, in.Index, c.idx)
		}
	}
	var bad MoveInput
	if err := json.Unmarshal([]byte(`{"from":"e2"}`), &bad); err == nil {
		t.Fatal("string square should be rejected")
	}
}
