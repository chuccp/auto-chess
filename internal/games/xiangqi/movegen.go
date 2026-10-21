package xiangqi

var orthogonal = [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}
var diagonal = [][2]int{{1, 1}, {1, -1}, {-1, -1}, {-1, 1}}
var knightMoves = [][2]int{
	{1, 2}, {2, 1}, {2, -1}, {1, -2},
	{-1, -2}, {-2, -1}, {-2, 1}, {-1, 2},
}

func inBoard(file, rank int) bool {
	return file >= 0 && file < Files && rank >= 0 && rank < Ranks
}

func inPalace(file, rank int, color int8) bool {
	if file < 3 || file > 5 {
		return false
	}
	if color == Red {
		return rank >= 0 && rank <= 2
	}
	return rank >= 7 && rank <= 9
}

func onOwnSide(rank int, color int8) bool {
	if color == Red {
		return rank <= 4
	}
	return rank >= 5
}

func pseudoMoves(st *State) []move {
	var moves []move
	for from, p := range st.Board {
		if p == 0 || colorOf(p) != st.Turn {
			continue
		}
		file, rank := from%Files, from/Files
		switch absPiece(p) {
		case King:
			for _, d := range orthogonal {
				f, r := file+d[0], rank+d[1]
				if !inBoard(f, r) || !inPalace(f, r, st.Turn) {
					continue
				}
				to := r*Files + f
				if st.Board[to] == 0 || colorOf(st.Board[to]) != st.Turn {
					moves = append(moves, move{from, to})
				}
			}
			// Flying general: the king may capture the opposing king along an
			// open file (this also makes exposing the file illegal).
			for _, dir := range []int{-1, 1} {
				for r := rank + dir; r >= 0 && r < Ranks; r += dir {
					q := st.Board[r*Files+file]
					if q == 0 {
						continue
					}
					if q == -King*st.Turn {
						moves = append(moves, move{from, r*Files + file})
					}
					break
				}
			}
		case Advisor:
			for _, d := range diagonal {
				f, r := file+d[0], rank+d[1]
				if !inBoard(f, r) || !inPalace(f, r, st.Turn) {
					continue
				}
				to := r*Files + f
				if st.Board[to] == 0 || colorOf(st.Board[to]) != st.Turn {
					moves = append(moves, move{from, to})
				}
			}
		case Bishop:
			for _, d := range diagonal {
				f, r := file+d[0], rank+d[1]
				toFile, toRank := file+2*d[0], rank+2*d[1]
				if !inBoard(toFile, toRank) || !onOwnSide(toRank, st.Turn) {
					continue
				}
				eye := r*Files + f
				if st.Board[eye] != 0 {
					continue
				}
				to := toRank*Files + toFile
				if st.Board[to] == 0 || colorOf(st.Board[to]) != st.Turn {
					moves = append(moves, move{from, to})
				}
			}
		case Knight:
			for _, d := range knightMoves {
				f, r := file+d[0], rank+d[1]
				if !inBoard(f, r) {
					continue
				}
				var legFile, legRank int
				if abs(d[0]) == 2 {
					legFile, legRank = file+sign(d[0]), rank
				} else {
					legFile, legRank = file, rank+sign(d[1])
				}
				if st.Board[legRank*Files+legFile] != 0 {
					continue
				}
				to := r*Files + f
				if st.Board[to] == 0 || colorOf(st.Board[to]) != st.Turn {
					moves = append(moves, move{from, to})
				}
			}
		case Rook:
			for _, d := range orthogonal {
				f, r := file+d[0], rank+d[1]
				for inBoard(f, r) {
					to := r*Files + f
					target := st.Board[to]
					if target == 0 {
						moves = append(moves, move{from, to})
					} else {
						if colorOf(target) != st.Turn {
							moves = append(moves, move{from, to})
						}
						break
					}
					f += d[0]
					r += d[1]
				}
			}
		case Cannon:
			for _, d := range orthogonal {
				f, r := file+d[0], rank+d[1]
				screen := false
				for inBoard(f, r) {
					to := r*Files + f
					target := st.Board[to]
					if !screen {
						if target == 0 {
							moves = append(moves, move{from, to})
						} else {
							screen = true
						}
					} else if target != 0 {
						if colorOf(target) != st.Turn {
							moves = append(moves, move{from, to})
						}
						break
					}
					f += d[0]
					r += d[1]
				}
			}
		case Pawn:
			dir := 1
			if st.Turn == Black {
				dir = -1
			}
			// Forward.
			if r := rank + dir; inBoard(file, r) {
				to := r*Files + file
				if st.Board[to] == 0 || colorOf(st.Board[to]) != st.Turn {
					moves = append(moves, move{from, to})
				}
			}
			// Sideways after crossing the river.
			crossed := (st.Turn == Red && rank >= 5) || (st.Turn == Black && rank <= 4)
			if crossed {
				for _, df := range []int{-1, 1} {
					f := file + df
					if !inBoard(f, rank) {
						continue
					}
					to := rank*Files + f
					if st.Board[to] == 0 || colorOf(st.Board[to]) != st.Turn {
						moves = append(moves, move{from, to})
					}
				}
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

func legalCaptures(st *State) []move {
	pseudo := pseudoMoves(st)
	out := make([]move, 0, len(pseudo)/2)
	for _, m := range pseudo {
		if st.Board[m.To] == 0 {
			continue
		}
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
	captured := next.Board[m.To]
	if next.hash == 0 {
		next.hash = calcHash(st.Board, st.Turn)
	}
	next.hash ^= zobristPieces[int(p)+8][m.From] ^ zobristPieces[int(p)+8][m.To]
	if captured != 0 {
		next.hash ^= zobristPieces[int(captured)+8][m.To]
	}
	next.hash ^= zobristTurn
	next.Board[m.From] = 0
	next.Board[m.To] = p
	if captured != 0 || absPiece(p) == Pawn {
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

func makeNull(st *State) *State {
	next := *st
	if next.hash == 0 {
		next.hash = calcHash(st.Board, st.Turn)
	}
	next.hash ^= zobristTurn
	next.Turn = -st.Turn
	next.Halfmove++
	return &next
}

func hasNonPawnMaterial(st *State, side int8) bool {
	for _, p := range st.Board {
		if p == 0 || colorOf(p) != side {
			continue
		}
		switch absPiece(p) {
		case Rook, Cannon, Knight:
			return true
		}
	}
	return false
}

func inCheck(st *State, side int8) bool {
	king := kingSquare(st.Board, side)
	if king < 0 {
		return true
	}
	return isAttacked(st.Board, king, side)
}

func kingSquare(board [Size]int8, side int8) int {
	for i, p := range board {
		if p == King*side {
			return i
		}
	}
	return -1
}

// isAttacked reports whether sq is attacked by the opposing side (by).
func isAttacked(board [Size]int8, sq int, by int8) bool {
	file, rank := sq%Files, sq/Files
	enemy := -by

	// Rook and flying general.
	for _, d := range orthogonal {
		f, r := file+d[0], rank+d[1]
		for inBoard(f, r) {
			p := board[r*Files+f]
			if p != 0 {
				ap := absPiece(p)
				if colorOf(p) == enemy {
					if ap == Rook {
						return true
					}
					if ap == King && d[0] == 0 {
						return true
					}
				}
				break
			}
			f += d[0]
			r += d[1]
		}
	}
	// Cannons need exactly one screen.
	for _, d := range orthogonal {
		f, r := file+d[0], rank+d[1]
		screens := 0
		for inBoard(f, r) {
			p := board[r*Files+f]
			if p != 0 {
				screens++
				if screens == 2 {
					if colorOf(p) == enemy && absPiece(p) == Cannon {
						return true
					}
					break
				}
			}
			f += d[0]
			r += d[1]
		}
	}
	// Knight attacks (check the attacking knight's leg).
	for _, d := range knightMoves {
		f, r := file+d[0], rank+d[1]
		if !inBoard(f, r) {
			continue
		}
		p := board[r*Files+f]
		if p != enemy*Knight {
			continue
		}
		var legFile, legRank int
		if abs(d[0]) == 2 {
			legFile, legRank = file+sign(d[0]), rank+d[1]
		} else {
			legFile, legRank = file+d[0], rank+sign(d[1])
		}
		if board[legRank*Files+legFile] == 0 {
			return true
		}
	}
	// Pawns.
	if by == Black {
		// Red pawns attack upward (+rank) and sideways after crossing.
		if rank-1 >= 0 && board[(rank-1)*Files+file] == Pawn {
			return true
		}
		if rank >= 5 {
			for _, df := range []int{-1, 1} {
				if f := file + df; inBoard(f, rank) && board[rank*Files+f] == Pawn {
					return true
				}
			}
		}
	} else {
		// Black pawns attack downward (-rank) and sideways after crossing.
		if rank+1 < Ranks && board[(rank+1)*Files+file] == -Pawn {
			return true
		}
		if rank <= 4 {
			for _, df := range []int{-1, 1} {
				if f := file + df; inBoard(f, rank) && board[rank*Files+f] == -Pawn {
					return true
				}
			}
		}
	}
	// Adjacent king.
	for _, d := range orthogonal {
		f, r := file+d[0], rank+d[1]
		if inBoard(f, r) && board[r*Files+f] == enemy*King {
			return true
		}
	}
	return false
}

func orderMoves(st *State, moves []move, history *[2][Size][Size]int32, killer0, killer1 move) {
	orderMovesBoard(st.Board, st.Turn, moves, history, killer0, killer1)
}

func orderMovesBoard(board [Size]int8, turn int8, moves []move, history *[2][Size][Size]int32, killer0, killer1 move) {
	for i := 1; i < len(moves); i++ {
		m := moves[i]
		score := moveScoreBoard(board, turn, m, history, killer0, killer1)
		j := i - 1
		for ; j >= 0 && moveScoreBoard(board, turn, moves[j], history, killer0, killer1) < score; j-- {
			moves[j+1] = moves[j]
		}
		moves[j+1] = m
	}
}

func moveScore(st *State, m move, history *[2][Size][Size]int32, killer0, killer1 move) int {
	return moveScoreBoard(st.Board, st.Turn, m, history, killer0, killer1)
}

func moveScoreBoard(board [Size]int8, turn int8, m move, history *[2][Size][Size]int32, killer0, killer1 move) int {
	score := 0
	if target := board[m.To]; target != 0 {
		score += 1000 + pieceValue[absPiece(target)]*10 - pieceValue[absPiece(board[m.From])]
	} else {
		if m == killer0 {
			score += 800
		} else if m == killer1 {
			score += 700
		}
		if history != nil {
			side := 0
			if turn == Black {
				side = 1
			}
			score += int(history[side][m.From][m.To])
		}
	}
	// Prefer central and forward moves slightly.
	file := m.To % Files
	score += 4 - abs(file-4)
	return score
}

func moveToFront(moves []move, m move) []move {
	out := make([]move, 0, len(moves))
	out = append(out, m)
	for _, v := range moves {
		if v != m {
			out = append(out, v)
		}
	}
	return out
}

func sign(v int) int {
	if v < 0 {
		return -1
	}
	return 1
}
