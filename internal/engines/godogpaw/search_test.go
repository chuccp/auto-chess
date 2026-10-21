package godogpaw

import (
	"sync"
	"testing"
)

// TestConcurrentSearches guards the per-position search state: before it was
// introduced, PV tables, the transposition table and the stop flag were package
// globals and searches had to be serialized.
func TestConcurrentSearches(t *testing.T) {
	fens := []string{
		"rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
		"r1bakab1r/9/1cn3nc1/p1p1p1p1p/9/9/P1P1P1P1P/1C2C1N2/9/RNBAKAB1R w - - 4 3",
		"1rbakab1r/9/2n3nc1/p3p1p1p/1c7/2p3P2/P1P1P3P/1CN1C1N2/9/1RBAKABR1 w - - 0 7",
	}
	moves := make([]MoveNG, len(fens))
	var wg sync.WaitGroup
	for i, fen := range fens {
		wg.Add(1)
		go func(i int, fen string) {
			defer wg.Done()
			pos := new(PositionNG)
			pos.Set(fen)
			moves[i] = pos.SearchWithProgress(SearchLimits{Depth: 4}, func(SearchProgress) {})
		}(i, fen)
	}
	wg.Wait()
	for i, m := range moves {
		if m == MOVE_NONE {
			t.Fatalf("position %d: no move", i)
		}
		pos := new(PositionNG)
		pos.Set(fens[i])
		if !pos.Legal(m) {
			t.Fatalf("position %d: illegal move %s", i, Move2Str(m))
		}
	}
}
