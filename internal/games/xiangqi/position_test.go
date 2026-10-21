package xiangqi

import "testing"

func perftPosition(p *Position, depth int) int {
	if depth == 0 {
		return 1
	}
	moves := p.legalMoves()
	if depth == 1 {
		return len(moves)
	}
	total := 0
	for _, m := range moves {
		u := p.doMove(m)
		total += perftPosition(p, depth-1)
		p.undo(u)
	}
	return total
}

func TestPositionPerft(t *testing.T) {
	st, _ := parseFEN("rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1")
	p := newPosition(st)
	for d := 1; d <= 3; d++ {
		if got, want := perftPosition(p, d), perft(st, d); got != want {
			t.Fatalf("position perft(%d) = %d, want %d", d, got, want)
		}
	}
}
