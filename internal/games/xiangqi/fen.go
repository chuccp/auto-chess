package xiangqi

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chuccp/auto-chess/internal/game"
)

func parseFEN(fen string) (*State, error) {
	fields := strings.Fields(strings.TrimSpace(fen))
	if len(fields) < 4 {
		return nil, game.NewError(game.CodeInvalidPosition, "象棋 FEN 至少需要 4 个字段：%q", fen)
	}
	st := &State{Fullmove: 1, LastFrom: -1, LastTo: -1}
	rows := strings.Split(fields[0], "/")
	if len(rows) != Ranks {
		return nil, game.NewError(game.CodeInvalidPosition, "象棋 FEN 棋盘必须有 %d 行，实际 %d 行", Ranks, len(rows))
	}
	for i, row := range rows {
		rank := Ranks - 1 - i
		file := 0
		for j := 0; j < len(row); j++ {
			c := row[j]
			if c >= '1' && c <= '9' {
				file += int(c - '0')
				continue
			}
			p, ok := letterPiece[c|0x20]
			if !ok {
				return nil, game.NewError(game.CodeInvalidPosition, "象棋 FEN 中出现无效棋子字符 %q", string(c))
			}
			if file >= Files {
				return nil, game.NewError(game.CodeInvalidPosition, "象棋 FEN 第 %d 行超过 %d 列", i+1, Files)
			}
			if c >= 'A' && c <= 'Z' {
				st.Board[rank*Files+file] = p
			} else {
				st.Board[rank*Files+file] = -p
			}
			file++
		}
		if file != Files {
			return nil, game.NewError(game.CodeInvalidPosition, "象棋 FEN 第 %d 行有 %d 列，应为 %d 列", i+1, file, Files)
		}
	}
	switch fields[1] {
	case "w", "r":
		st.Turn = Red
	case "b":
		st.Turn = Black
	default:
		return nil, game.NewError(game.CodeInvalidPosition, "象棋 FEN 行棋方必须是 w（红）或 b（黑），实际 %q", fields[1])
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
	if countKing(st.Board, Red) != 1 || countKing(st.Board, Black) != 1 {
		return nil, game.NewError(game.CodeInvalidPosition, "象棋 FEN 必须包含红黑各一个将帅")
	}
	if inCheck(st, -st.Turn) {
		return nil, game.NewError(game.CodeInvalidPosition, "象棋 FEN 违规：非行棋方处于被将军状态")
	}
	return st, nil
}

func countKing(board [Size]int8, side int8) int {
	n := 0
	for _, p := range board {
		if p == King*side {
			n++
		}
	}
	return n
}

func (st *State) fen() string {
	var rows []string
	for rank := Ranks - 1; rank >= 0; rank-- {
		var b strings.Builder
		empty := 0
		for file := 0; file < Files; file++ {
			p := st.Board[rank*Files+file]
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
				letter = strings.ToUpper(letter)
			}
			b.WriteString(letter)
		}
		if empty > 0 {
			b.WriteString(strconv.Itoa(empty))
		}
		rows = append(rows, b.String())
	}
	turn := "b"
	if st.Turn == Red {
		turn = "w"
	}
	return fmt.Sprintf("%s %s - - %d %d", strings.Join(rows, "/"), turn, st.Halfmove, st.Fullmove)
}
