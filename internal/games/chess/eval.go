package chess

// evaluate returns a centipawn score from the perspective of the side to move.
func evaluate(st *State) int {
	var white, black int
	var whitePawns, blackPawns [8]int
	var whiteBishops, blackBishops int
	var nonPawn int

	for _, p := range st.Board {
		if p == 0 {
			continue
		}
		piece := absPiece(p)
		if piece != Pawn && piece != King {
			nonPawn += pieceValue[piece]
		}
	}
	endgame := nonPawn < 1500

	for sq, p := range st.Board {
		if p == 0 {
			continue
		}
		piece := absPiece(p)
		value := pieceValue[piece]
		idx := sq
		if p < 0 {
			idx = sq ^ 56
		}
		pos := tableFor(piece, endgame)[idx]
		if p > 0 {
			white += value + pos
			switch piece {
			case Pawn:
				whitePawns[fileOf(sq)]++
			case Bishop:
				whiteBishops++
			}
		} else {
			black += value + pos
			switch piece {
			case Pawn:
				blackPawns[fileOf(sq)]++
			case Bishop:
				blackBishops++
			}
		}
	}
	if whiteBishops >= 2 {
		white += 30
	}
	if blackBishops >= 2 {
		black += 30
	}

	// Pawn structure.
	for file := 0; file < 8; file++ {
		if whitePawns[file] > 1 {
			white -= 12 * (whitePawns[file] - 1)
		}
		if blackPawns[file] > 1 {
			black -= 12 * (blackPawns[file] - 1)
		}
		if whitePawns[file] > 0 && !hasPawn(whitePawns, file-1) && !hasPawn(whitePawns, file+1) {
			white -= 15 * whitePawns[file]
		}
		if blackPawns[file] > 0 && !hasPawn(blackPawns, file-1) && !hasPawn(blackPawns, file+1) {
			black -= 15 * blackPawns[file]
		}
	}
	// Passed pawns.
	for sq, p := range st.Board {
		if absPiece(p) != Pawn {
			continue
		}
		file, rank := fileOf(sq), rankOf(sq)
		if p > 0 {
			if !enemyPawnAhead(st.Board, file, rank, Black) {
				white += passedBonus[rank]
			}
		} else {
			if !enemyPawnAhead(st.Board, file, rank, White) {
				black += passedBonus[7-rank]
			}
		}
	}
	// Rooks on open / semi-open files.
	for sq, p := range st.Board {
		if absPiece(p) != Rook {
			continue
		}
		file := fileOf(sq)
		ownPawns, enemyPawns := whitePawns, blackPawns
		if p < 0 {
			ownPawns, enemyPawns = blackPawns, whitePawns
		}
		if ownPawns[file] == 0 {
			if enemyPawns[file] == 0 {
				if p > 0 {
					white += 15
				} else {
					black += 15
				}
			} else {
				if p > 0 {
					white += 8
				} else {
					black += 8
				}
			}
		}
	}

	score := white - black
	if st.Turn == Black {
		return -score
	}
	return score
}

func tableFor(piece int8, endgame bool) *[64]int {
	switch piece {
	case Pawn:
		return &pawnTable
	case Knight:
		return &knightTable
	case Bishop:
		return &bishopTable
	case Rook:
		return &rookTable
	case Queen:
		return &queenTable
	case King:
		if endgame {
			return &kingEndTable
		}
		return &kingMidTable
	}
	return &pawnTable
}

func hasPawn(pawns [8]int, file int) bool {
	return file >= 0 && file < 8 && pawns[file] > 0
}

func enemyPawnAhead(board [64]int8, file, rank int, enemy int8) bool {
	for df := -1; df <= 1; df++ {
		f := file + df
		if f < 0 || f > 7 {
			continue
		}
		if enemy == Black {
			for r := rank + 1; r < 8; r++ {
				if board[squareAt(f, r)] == -Pawn {
					return true
				}
			}
		} else {
			for r := rank - 1; r >= 0; r-- {
				if board[squareAt(f, r)] == Pawn {
					return true
				}
			}
		}
	}
	return false
}

var pieceValue = [7]int{0, 100, 320, 330, 500, 900, 0}

var passedBonus = [8]int{0, 5, 10, 20, 35, 60, 100, 150}

var pawnTable = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	5, 10, 10, -20, -20, 10, 10, 5,
	5, -5, -10, 0, 0, -10, -5, 5,
	0, 0, 0, 20, 20, 0, 0, 0,
	5, 5, 10, 25, 25, 10, 5, 5,
	10, 10, 20, 30, 30, 20, 10, 10,
	50, 50, 50, 50, 50, 50, 50, 50,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var knightTable = [64]int{
	-50, -40, -30, -30, -30, -30, -40, -50,
	-40, -20, 0, 0, 0, 0, -20, -40,
	-30, 0, 10, 15, 15, 10, 0, -30,
	-30, 5, 15, 20, 20, 15, 5, -30,
	-30, 0, 15, 20, 20, 15, 0, -30,
	-30, 5, 10, 15, 15, 10, 5, -30,
	-40, -20, 0, 5, 5, 0, -20, -40,
	-50, -40, -30, -30, -30, -30, -40, -50,
}

var bishopTable = [64]int{
	-20, -10, -10, -10, -10, -10, -10, -20,
	-10, 0, 0, 0, 0, 0, 0, -10,
	-10, 0, 5, 10, 10, 5, 0, -10,
	-10, 5, 5, 10, 10, 5, 5, -10,
	-10, 0, 10, 10, 10, 10, 0, -10,
	-10, 10, 10, 10, 10, 10, 10, -10,
	-10, 5, 0, 0, 0, 0, 5, -10,
	-20, -10, -10, -10, -10, -10, -10, -20,
}

var rookTable = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	5, 10, 10, 10, 10, 10, 10, 5,
	-5, 0, 0, 0, 0, 0, 0, -5,
	-5, 0, 0, 0, 0, 0, 0, -5,
	-5, 0, 0, 0, 0, 0, 0, -5,
	-5, 0, 0, 0, 0, 0, 0, -5,
	-5, 0, 0, 0, 0, 0, 0, -5,
	0, 0, 0, 5, 5, 0, 0, 0,
}

var queenTable = [64]int{
	-20, -10, -10, -5, -5, -10, -10, -20,
	-10, 0, 0, 0, 0, 0, 0, -10,
	-10, 0, 5, 5, 5, 5, 0, -10,
	-5, 0, 5, 5, 5, 5, 0, -5,
	0, 0, 5, 5, 5, 5, 0, -5,
	-10, 5, 5, 5, 5, 5, 0, -10,
	-10, 0, 5, 0, 0, 0, 0, -10,
	-20, -10, -10, -5, -5, -10, -10, -20,
}

var kingMidTable = [64]int{
	20, 30, 10, 0, 0, 10, 30, 20,
	20, 20, 0, 0, 0, 0, 20, 20,
	-10, -20, -20, -20, -20, -20, -20, -10,
	-20, -30, -30, -40, -40, -30, -30, -20,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
}

var kingEndTable = [64]int{
	-50, -30, -30, -30, -30, -30, -30, -50,
	-30, -30, 0, 0, 0, 0, -30, -30,
	-30, -10, 20, 30, 30, 20, -10, -30,
	-30, -10, 30, 40, 40, 30, -10, -30,
	-30, -10, 30, 40, 40, 30, -10, -30,
	-30, -10, 20, 30, 30, 20, -10, -30,
	-30, -20, -10, 0, 0, -10, -20, -30,
	-50, -40, -30, -20, -20, -30, -40, -50,
}
