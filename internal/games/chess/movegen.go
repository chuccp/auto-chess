package chess

import (
	"context"
	"sort"
	"time"
)

var knightOffsets = [][2]int{
	{1, 2}, {2, 1}, {2, -1}, {1, -2},
	{-1, -2}, {-2, -1}, {-2, 1}, {-1, 2},
}

var kingOffsets = [][2]int{
	{0, 1}, {1, 1}, {1, 0}, {1, -1},
	{0, -1}, {-1, -1}, {-1, 0}, {-1, 1},
}

var bishopDirs = [][2]int{{1, 1}, {1, -1}, {-1, -1}, {-1, 1}}
var rookDirs = [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}

func fileOf(sq int) int { return sq & 7 }
func rankOf(sq int) int { return sq >> 3 }
func onBoard(file, rank int) bool {
	return file >= 0 && file < 8 && rank >= 0 && rank < 8
}
func squareAt(file, rank int) int { return rank*8 + file }

func pseudoMoves(st *State) []move {
	var moves []move
	for from := 0; from < 64; from++ {
		p := st.Board[from]
		if p == 0 || colorOf(p) != st.Turn {
			continue
		}
		file, rank := fileOf(from), rankOf(from)
		switch absPiece(p) {
		case Pawn:
			dir := 1
			startRank, promoRank := 1, 7
			if st.Turn == Black {
				dir = -1
				startRank, promoRank = 6, 0
			}
			one := squareAt(file, rank+dir)
			if onBoard(file, rank+dir) && st.Board[one] == 0 {
				moves = appendPawn(moves, from, one, promoRank)
				twoRank := rank + 2*dir
				if rank == startRank && onBoard(file, twoRank) {
					two := squareAt(file, twoRank)
					if st.Board[two] == 0 {
						moves = append(moves, move{From: from, To: two})
					}
				}
			}
			for _, df := range []int{-1, 1} {
				tf, tr := file+df, rank+dir
				if !onBoard(tf, tr) {
					continue
				}
				to := squareAt(tf, tr)
				target := st.Board[to]
				if target != 0 && colorOf(target) != st.Turn {
					moves = appendPawn(moves, from, to, promoRank)
				} else if target == 0 && to == st.EP {
					moves = append(moves, move{From: from, To: to})
				}
			}
		case Knight:
			for _, d := range knightOffsets {
				tf, tr := file+d[0], rank+d[1]
				if !onBoard(tf, tr) {
					continue
				}
				to := squareAt(tf, tr)
				if st.Board[to] == 0 || colorOf(st.Board[to]) != st.Turn {
					moves = append(moves, move{From: from, To: to})
				}
			}
		case Bishop:
			moves = appendSlides(moves, st, from, bishopDirs)
		case Rook:
			moves = appendSlides(moves, st, from, rookDirs)
		case Queen:
			moves = appendSlides(moves, st, from, bishopDirs)
			moves = appendSlides(moves, st, from, rookDirs)
		case King:
			for _, d := range kingOffsets {
				tf, tr := file+d[0], rank+d[1]
				if !onBoard(tf, tr) {
					continue
				}
				to := squareAt(tf, tr)
				if st.Board[to] == 0 || colorOf(st.Board[to]) != st.Turn {
					moves = append(moves, move{From: from, To: to})
				}
			}
			moves = append(moves, castlingMoves(st, from)...)
		}
	}
	return moves
}

func appendPawn(moves []move, from, to, promoRank int) []move {
	if rankOf(to) == promoRank {
		for _, p := range []int8{Queen, Rook, Bishop, Knight} {
			moves = append(moves, move{From: from, To: to, Promotion: p})
		}
		return moves
	}
	return append(moves, move{From: from, To: to})
}

func appendSlides(moves []move, st *State, from int, dirs [][2]int) []move {
	file, rank := fileOf(from), rankOf(from)
	for _, d := range dirs {
		tf, tr := file+d[0], rank+d[1]
		for onBoard(tf, tr) {
			to := squareAt(tf, tr)
			target := st.Board[to]
			if target == 0 {
				moves = append(moves, move{From: from, To: to})
			} else {
				if colorOf(target) != st.Turn {
					moves = append(moves, move{From: from, To: to})
				}
				break
			}
			tf += d[0]
			tr += d[1]
		}
	}
	return moves
}

func castlingMoves(st *State, from int) []move {
	var moves []move
	if st.Turn == White && from == 4 {
		if st.Castling&castleWK != 0 && st.Board[5] == 0 && st.Board[6] == 0 && st.Board[7] == Rook {
			if !isAttacked(st.Board, 4, Black) && !isAttacked(st.Board, 5, Black) && !isAttacked(st.Board, 6, Black) {
				moves = append(moves, move{From: 4, To: 6})
			}
		}
		if st.Castling&castleWQ != 0 && st.Board[3] == 0 && st.Board[2] == 0 && st.Board[1] == 0 && st.Board[0] == Rook {
			if !isAttacked(st.Board, 4, Black) && !isAttacked(st.Board, 3, Black) && !isAttacked(st.Board, 2, Black) {
				moves = append(moves, move{From: 4, To: 2})
			}
		}
	}
	if st.Turn == Black && from == 60 {
		if st.Castling&castleBK != 0 && st.Board[61] == 0 && st.Board[62] == 0 && st.Board[63] == -Rook {
			if !isAttacked(st.Board, 60, White) && !isAttacked(st.Board, 61, White) && !isAttacked(st.Board, 62, White) {
				moves = append(moves, move{From: 60, To: 62})
			}
		}
		if st.Castling&castleBQ != 0 && st.Board[59] == 0 && st.Board[58] == 0 && st.Board[57] == 0 && st.Board[56] == -Rook {
			if !isAttacked(st.Board, 60, White) && !isAttacked(st.Board, 59, White) && !isAttacked(st.Board, 58, White) {
				moves = append(moves, move{From: 60, To: 58})
			}
		}
	}
	return moves
}

func legalMoves(st *State) []move {
	pseudo := pseudoMoves(st)
	out := make([]move, 0, len(pseudo))
	for _, m := range pseudo {
		next := makeMove(st, m)
		if !inCheck(next, st.Turn) {
			out = append(out, m)
		}
	}
	return out
}

func makeMove(st *State, m move) *State {
	next := *st
	p := next.Board[m.From]
	capturedSq := m.To
	captured := next.Board[m.To]
	if absPiece(p) == Pawn && m.To == st.EP && fileOf(m.From) != fileOf(m.To) {
		capturedSq = m.To - 8*int(st.Turn)
		captured = next.Board[capturedSq]
	}
	next.Board[capturedSq] = 0
	next.Board[m.From] = 0
	if m.Promotion != 0 {
		next.Board[m.To] = m.Promotion * st.Turn
	} else {
		next.Board[m.To] = p
	}
	// Castling rook movement.
	if absPiece(p) == King && abs(fileOf(m.To)-fileOf(m.From)) == 2 {
		switch m.To {
		case 6:
			next.Board[5], next.Board[7] = Rook, 0
		case 2:
			next.Board[3], next.Board[0] = Rook, 0
		case 62:
			next.Board[61], next.Board[63] = -Rook, 0
		case 58:
			next.Board[59], next.Board[56] = -Rook, 0
		}
	}
	// Castling rights.
	switch m.From {
	case 4:
		next.Castling &^= castleWK | castleWQ
	case 7:
		next.Castling &^= castleWK
	case 0:
		next.Castling &^= castleWQ
	case 60:
		next.Castling &^= castleBK | castleBQ
	case 63:
		next.Castling &^= castleBK
	case 56:
		next.Castling &^= castleBQ
	}
	switch capturedSq {
	case 7:
		next.Castling &^= castleWK
	case 0:
		next.Castling &^= castleWQ
	case 63:
		next.Castling &^= castleBK
	case 56:
		next.Castling &^= castleBQ
	}
	// En passant target.
	next.EP = -1
	if absPiece(p) == Pawn && abs(m.To-m.From) == 16 {
		next.EP = (m.From + m.To) / 2
	}
	// Clocks.
	if absPiece(p) == Pawn || captured != 0 {
		next.Halfmove = 0
	} else {
		next.Halfmove++
	}
	if st.Turn == Black {
		next.Fullmove++
	}
	next.Turn = -st.Turn
	return &next
}

func inCheck(st *State, side int8) bool {
	king := kingSquare(st.Board, side)
	if king < 0 {
		return true
	}
	return isAttacked(st.Board, king, -side)
}

func kingSquare(board [64]int8, side int8) int {
	for i, p := range board {
		if p == King*side {
			return i
		}
	}
	return -1
}

func isAttacked(board [64]int8, sq int, by int8) bool {
	file, rank := fileOf(sq), rankOf(sq)
	// Pawns.
	if by == White {
		for _, df := range []int{-1, 1} {
			r, f := rank-1, file+df
			if onBoard(f, r) && board[squareAt(f, r)] == Pawn {
				return true
			}
		}
	} else {
		for _, df := range []int{-1, 1} {
			r, f := rank+1, file+df
			if onBoard(f, r) && board[squareAt(f, r)] == -Pawn {
				return true
			}
		}
	}
	// Knights.
	for _, d := range knightOffsets {
		f, r := file+d[0], rank+d[1]
		if onBoard(f, r) && board[squareAt(f, r)] == Knight*by {
			return true
		}
	}
	// King.
	for _, d := range kingOffsets {
		f, r := file+d[0], rank+d[1]
		if onBoard(f, r) && board[squareAt(f, r)] == King*by {
			return true
		}
	}
	// Rook / queen rays.
	for _, d := range rookDirs {
		f, r := file+d[0], rank+d[1]
		for onBoard(f, r) {
			p := board[squareAt(f, r)]
			if p != 0 {
				if colorOf(p) == by && (absPiece(p) == Rook || absPiece(p) == Queen) {
					return true
				}
				break
			}
			f += d[0]
			r += d[1]
		}
	}
	// Bishop / queen rays.
	for _, d := range bishopDirs {
		f, r := file+d[0], rank+d[1]
		for onBoard(f, r) {
			p := board[squareAt(f, r)]
			if p != 0 {
				if colorOf(p) == by && (absPiece(p) == Bishop || absPiece(p) == Queen) {
					return true
				}
				break
			}
			f += d[0]
			r += d[1]
		}
	}
	return false
}

type searcher struct {
	ctx      context.Context
	deadline time.Time
	nodes    int64
	timedOut bool
	killers  [64][2]move
	history  [64][64]int
	pv       [64][]move
}

func (s *searcher) checkTime() bool {
	if s.timedOut {
		return true
	}
	if s.nodes&1023 == 0 {
		select {
		case <-s.ctx.Done():
			s.timedOut = true
			return true
		default:
		}
		if time.Now().After(s.deadline) {
			s.timedOut = true
			return true
		}
	}
	return false
}

func (s *searcher) negamax(st *State, depth, alpha, beta, ply int) int {
	s.nodes++
	if s.checkTime() {
		return 0
	}
	if st.Halfmove >= 100 {
		return 0
	}
	moves := legalMoves(st)
	if len(moves) == 0 {
		if inCheck(st, st.Turn) {
			return -mateScore + ply
		}
		return 0
	}
	if depth <= 0 {
		return s.quiesce(st, alpha, beta, ply)
	}
	s.pv[ply] = s.pv[ply][:0]
	s.order(st, moves, ply)
	best := -mateScore * 2
	for i, m := range moves {
		child := makeMove(st, m)
		score := -s.negamax(child, depth-1, -beta, -alpha, ply+1)
		if score > best {
			best = score
			line := make([]move, 0, 1+len(s.pv[ply+1]))
			line = append(line, m)
			line = append(line, s.pv[ply+1]...)
			s.pv[ply] = line
		}
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			if st.Board[m.To] == 0 && m.Promotion == 0 {
				if !sameMove(s.killers[ply][0], m) {
					s.killers[ply][1] = s.killers[ply][0]
					s.killers[ply][0] = m
				}
				s.history[m.From][m.To] += depth * depth
			}
			break
		}
		if s.timedOut {
			break
		}
		_ = i
	}
	return best
}

func (s *searcher) quiesce(st *State, alpha, beta, ply int) int {
	s.nodes++
	if s.checkTime() {
		return 0
	}
	if st.Halfmove >= 100 {
		return 0
	}
	var moves []move
	if inCheck(st, st.Turn) {
		moves = legalMoves(st)
		if len(moves) == 0 {
			return -mateScore + ply
		}
	} else {
		stand := evaluate(st)
		if stand >= beta {
			return beta
		}
		if stand > alpha {
			alpha = stand
		}
		for _, m := range legalMoves(st) {
			if st.Board[m.To] != 0 || m.Promotion != 0 || (absPiece(st.Board[m.From]) == Pawn && m.To == st.EP) {
				moves = append(moves, m)
			}
		}
	}
	s.order(st, moves, ply)
	best := -mateScore * 2
	if !inCheck(st, st.Turn) {
		best = alpha
	}
	for _, m := range moves {
		child := makeMove(st, m)
		score := -s.quiesce(child, -beta, -alpha, ply+1)
		if score > best {
			best = score
		}
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			break
		}
		if s.timedOut {
			break
		}
	}
	return best
}

func (s *searcher) order(st *State, moves []move, ply int) {
	sort.SliceStable(moves, func(i, j int) bool {
		return s.moveScore(st, moves[i], ply) > s.moveScore(st, moves[j], ply)
	})
}

func (s *searcher) moveScore(st *State, m move, ply int) int {
	score := 0
	if m.Promotion != 0 {
		score += 8000
	}
	if captured := st.Board[m.To]; captured != 0 {
		score += 10000 + int(absPiece(captured))*10 - int(absPiece(st.Board[m.From]))
	} else if absPiece(st.Board[m.From]) == Pawn && m.To == st.EP {
		score += 10000 + int(Pawn)*10
	}
	if sameMove(s.killers[ply][0], m) {
		score += 7000
	} else if sameMove(s.killers[ply][1], m) {
		score += 6000
	}
	score += s.history[m.From][m.To]
	return score
}

func sameMove(a, b move) bool {
	return a.From == b.From && a.To == b.To && a.Promotion == b.Promotion
}
