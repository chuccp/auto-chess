package chess

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chuccp/auto-chess/internal/game"
)

func parseFEN(fen string) (*State, error) {
	fields := strings.Fields(strings.TrimSpace(fen))
	if len(fields) < 4 {
		return nil, game.NewError(game.CodeInvalidPosition, "FEN 至少需要 4 个字段：%q", fen)
	}
	st := &State{EP: -1, Fullmove: 1, LastFrom: -1, LastTo: -1}
	rows := strings.Split(fields[0], "/")
	if len(rows) != 8 {
		return nil, game.NewError(game.CodeInvalidPosition, "FEN 棋盘必须有 8 行，实际 %d 行", len(rows))
	}
	for i, row := range rows {
		rank := 7 - i
		file := 0
		for j := 0; j < len(row); j++ {
			c := row[j]
			if c >= '1' && c <= '8' {
				file += int(c - '0')
				continue
			}
			p, ok := pieceFromLetter(c)
			if !ok {
				return nil, game.NewError(game.CodeInvalidPosition, "FEN 中出现无效棋子字符 %q", string(c))
			}
			if file >= 8 {
				return nil, game.NewError(game.CodeInvalidPosition, "FEN 第 %d 行超过 8 列", i+1)
			}
			if c >= 'A' && c <= 'Z' {
				st.Board[squareAt(file, rank)] = p
			} else {
				st.Board[squareAt(file, rank)] = -p
			}
			file++
		}
		if file != 8 {
			return nil, game.NewError(game.CodeInvalidPosition, "FEN 第 %d 行有 %d 列，应为 8 列", i+1, file)
		}
	}
	switch fields[1] {
	case "w":
		st.Turn = White
	case "b":
		st.Turn = Black
	default:
		return nil, game.NewError(game.CodeInvalidPosition, "FEN 行棋方必须是 w 或 b，实际 %q", fields[1])
	}
	if fields[2] != "-" {
		for _, c := range fields[2] {
			switch c {
			case 'K':
				st.Castling |= castleWK
			case 'Q':
				st.Castling |= castleWQ
			case 'k':
				st.Castling |= castleBK
			case 'q':
				st.Castling |= castleBQ
			default:
				return nil, game.NewError(game.CodeInvalidPosition, "FEN 王车易位字段无效：%q", fields[2])
			}
		}
	}
	if fields[3] != "-" {
		sq, ok := parseSquare(fields[3])
		if !ok {
			return nil, game.NewError(game.CodeInvalidPosition, "FEN 吃过路兵格无效：%q", fields[3])
		}
		st.EP = sq
	}
	if len(fields) > 4 {
		if n, err := strconv.Atoi(fields[4]); err == nil && n >= 0 {
			st.Halfmove = n
		}
	}
	if len(fields) > 5 {
		if n, err := strconv.Atoi(fields[5]); err == nil && n >= 1 {
			st.Fullmove = n
		}
	}
	// Exactly one king per side.
	if countPiece(st.Board, King) != 1 || countPiece(st.Board, -King) != 1 {
		return nil, game.NewError(game.CodeInvalidPosition, "FEN 必须包含双方各一个王")
	}
	// The side that just moved may not be in check; such a position is illegal.
	if inCheck(st, -st.Turn) {
		return nil, game.NewError(game.CodeInvalidPosition, "FEN 违规：非行棋方处于被将军状态")
	}
	return st, nil
}

func countPiece(board [64]int8, p int8) int {
	n := 0
	for _, v := range board {
		if v == p {
			n++
		}
	}
	return n
}

func (st *State) fen() string {
	var rows []string
	for rank := 7; rank >= 0; rank-- {
		var b strings.Builder
		empty := 0
		for file := 0; file < 8; file++ {
			p := st.Board[squareAt(file, rank)]
			if p == 0 {
				empty++
				continue
			}
			if empty > 0 {
				b.WriteString(strconv.Itoa(empty))
				empty = 0
			}
			letter := pieceLetter(absPiece(p))
			if p > 0 {
				letter &^= 0x20
			}
			b.WriteByte(letter)
		}
		if empty > 0 {
			b.WriteString(strconv.Itoa(empty))
		}
		rows = append(rows, b.String())
	}
	var castling strings.Builder
	if st.Castling&castleWK != 0 {
		castling.WriteByte('K')
	}
	if st.Castling&castleWQ != 0 {
		castling.WriteByte('Q')
	}
	if st.Castling&castleBK != 0 {
		castling.WriteByte('k')
	}
	if st.Castling&castleBQ != 0 {
		castling.WriteByte('q')
	}
	castle := castling.String()
	if castle == "" {
		castle = "-"
	}
	ep := "-"
	if st.EP >= 0 && st.EP < 64 {
		ep = squareName(st.EP)
	}
	return fmt.Sprintf("%s %s %s %s %d %d", strings.Join(rows, "/"), colorString(st.Turn), castle, ep, st.Halfmove, st.Fullmove)
}

func (g *Game) status(st *State) game.Status {
	legal := legalMoves(st)
	check := inCheck(st, st.Turn)
	if len(legal) == 0 {
		if check {
			return game.Status{State: "checkmate", Winner: colorString(-st.Turn), Reason: "checkmate", InCheck: true}
		}
		return game.Status{State: "stalemate", Reason: "stalemate"}
	}
	if st.Halfmove >= 100 {
		return game.Status{State: "draw", Reason: "fifty-move"}
	}
	if insufficientMaterial(st.Board) {
		return game.Status{State: "draw", Reason: "insufficient-material"}
	}
	if check {
		return game.Status{State: "check", InCheck: true}
	}
	return game.Status{State: "playing"}
}

func insufficientMaterial(board [64]int8) bool {
	var bishopsWhite, bishopsBlack, knightsWhite, knightsBlack int
	var bishopSquaresWhite, bishopSquaresBlack []int
	for sq, p := range board {
		switch absPiece(p) {
		case Pawn, Rook, Queen:
			return false
		case Bishop:
			if p > 0 {
				bishopsWhite++
				bishopSquaresWhite = append(bishopSquaresWhite, sq)
			} else {
				bishopsBlack++
				bishopSquaresBlack = append(bishopSquaresBlack, sq)
			}
		case Knight:
			if p > 0 {
				knightsWhite++
			} else {
				knightsBlack++
			}
		}
	}
	minorTotal := bishopsWhite + bishopsBlack + knightsWhite + knightsBlack
	if minorTotal <= 1 {
		return true
	}
	// K+B vs K+B with bishops on the same colour is a dead draw.
	if minorTotal == 2 && bishopsWhite == 1 && bishopsBlack == 1 {
		cw := (fileOf(bishopSquaresWhite[0]) + rankOf(bishopSquaresWhite[0])) % 2
		cb := (fileOf(bishopSquaresBlack[0]) + rankOf(bishopSquaresBlack[0])) % 2
		return cw == cb
	}
	return false
}
