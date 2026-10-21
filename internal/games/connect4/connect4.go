// Package connect4 implements a 7x6 gravity placement game with a negamax AI.
package connect4

import (
	"context"
	"fmt"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

const (
	Rows = 6
	Cols = 7
	Size = Rows * Cols

	Empty  int8 = 0
	Red    int8 = 1
	Yellow int8 = 2

	winScore = 1_000_000
)

// State is a complete connect-four position.
type State struct {
	Board   [Size]int8
	Turn    int8
	Moves   int
	Last    int
	Winner  int8
	IsDraw  bool
	winLine [4]int
}

// Game implements game.Game.
type Game struct{}

// New returns a connect-four engine.
func New() *Game { return &Game{} }

// Meta implements game.Game.
func (g *Game) Meta() game.Meta {
	return game.Meta{
		ID:          "connect4",
		Aliases:     []string{"connect-four", "four-in-a-row"},
		Name:        "四子棋",
		EnglishName: "Connect Four",
		Tagline:     "重力落子，横竖斜先连成四子者胜。",
		Category:    "连珠类",
		Players:     2,
		Board: game.BoardMeta{
			Rows: Rows, Cols: Cols, Cells: Size, Type: "grid",
			Origin: "row 0 为最上行，index = row*7 + col",
			Legend: map[string]string{"": "空位", "r": "红方", "y": "黄方"},
		},
		Colors:         []game.ColorMeta{{ID: "r", Name: "红方"}, {ID: "y", Name: "黄方"}},
		PositionFormat: `{"board":["r","","", ...42格], "turn":"r"}`,
		MoveFormat:     `{"index": 落点格子下标（0-41）}；落点必须是该列最下方的空位`,
		Difficulties:   game.DifficultyOrder,
		Features:       []string{"完整重力规则", "连四判定", "Alpha-Beta 搜索", "中心优先启发"},
		Rules: []string{
			"红方先行，双方轮流选择一列落子，棋子落到该列最下方的空位。",
			"任意方向（横、竖、两条斜线）先连成四子的一方获胜。",
			"棋盘填满且无人连成四子则为和棋。",
		},
	}
}

// Initial implements game.Game.
func (g *Game) Initial() game.Position {
	return &State{Turn: Red, Last: -1}
}

// Decode implements game.Game.
func (g *Game) Decode(in game.PositionInput) (game.Position, error) {
	st := &State{Turn: Red, Last: -1}
	if len(in.Board) != Size {
		return nil, game.NewError(game.CodeInvalidPosition, "connect4 棋盘必须包含 %d 个格子，实际 %d 个", Size, len(in.Board))
	}
	for i, cell := range in.Board {
		switch cell {
		case "", "0", ".", "-":
			st.Board[i] = Empty
		case "r", "R", "red":
			st.Board[i] = Red
			st.Moves++
		case "y", "Y", "yellow":
			st.Board[i] = Yellow
			st.Moves++
		default:
			return nil, game.NewError(game.CodeInvalidPosition, "第 %d 格的值 %q 无效，只支持 r / y / 空", i, cell)
		}
	}
	// Validate gravity: no piece may float above an empty cell.
	for col := 0; col < Cols; col++ {
		seenEmpty := false
		for row := Rows - 1; row >= 0; row-- {
			if st.Board[row*Cols+col] == Empty {
				seenEmpty = true
			} else if seenEmpty {
				return nil, game.NewError(game.CodeInvalidPosition, "第 %d 列存在悬空棋子", col)
			}
		}
	}
	switch in.Turn {
	case "", "r", "R", "red", "w":
		st.Turn = Red
	case "y", "Y", "yellow", "b":
		st.Turn = Yellow
	default:
		return nil, game.NewError(game.CodeInvalidPosition, "turn 必须是 r 或 y，实际 %q", in.Turn)
	}
	red, yellow := count(st.Board, Red), count(st.Board, Yellow)
	if red != yellow && red != yellow+1 {
		return nil, game.NewError(game.CodeInvalidPosition, "红黄棋子数量不平衡（红 %d / 黄 %d）", red, yellow)
	}
	expected := Red
	if red > yellow {
		expected = Yellow
	}
	if in.Turn != "" && st.Turn != expected {
		return nil, game.NewError(game.CodeInvalidPosition, "turn=%s 与棋子数量不一致，当前应为 %s", turnString(st.Turn), turnString(expected))
	}
	st.Turn = expected
	st.recompute()
	return st, nil
}

func count(b [Size]int8, c int8) int {
	n := 0
	for _, v := range b {
		if v == c {
			n++
		}
	}
	return n
}

// recompute derives winner and draw state from a board.
func (s *State) recompute() {
	s.Winner = 0
	s.IsDraw = false
	s.Last = -1
	for i, v := range s.Board {
		if v != Empty {
			if line := winningLine(s.Board, i, v); line != nil {
				s.Winner = v
				copy(s.winLine[:], line)
				break
			}
		}
	}
	if s.Winner == 0 && s.Moves == Size {
		s.IsDraw = true
	}
}

// Encode implements game.Game.
func (g *Game) Encode(pos game.Position, includeLegalMoves bool) game.PositionView {
	st := pos.(*State)
	board := make([]string, Size)
	for i, v := range st.Board {
		switch v {
		case Red:
			board[i] = "r"
		case Yellow:
			board[i] = "y"
		default:
			board[i] = ""
		}
	}
	status := game.Status{State: "playing"}
	switch {
	case st.Winner == Red:
		status = game.Status{State: "win", Winner: "r", Reason: "four"}
	case st.Winner == Yellow:
		status = game.Status{State: "win", Winner: "y", Reason: "four"}
	case st.IsDraw:
		status = game.Status{State: "draw", Reason: "board-full"}
	}
	view := game.PositionView{
		Board:      board,
		Turn:       turnString(st.Turn),
		MoveNumber: st.Moves + 1,
		Status:     status,
	}
	if st.Last >= 0 {
		m := coordMove(st.Last)
		view.LastMove = &m
	}
	if includeLegalMoves {
		view.LegalMoves = g.LegalMoves(st)
		view.LegalMoveCount = len(view.LegalMoves)
	}
	return view
}

// LegalMoves implements game.Game.
func (g *Game) LegalMoves(pos game.Position) []game.Move {
	st := pos.(*State)
	if st.Winner != 0 || st.IsDraw {
		return nil
	}
	moves := make([]game.Move, 0, Cols)
	for col := 0; col < Cols; col++ {
		if idx, ok := landing(st.Board, col); ok {
			moves = append(moves, coordMove(idx))
		}
	}
	return moves
}

// Apply implements game.Game.
func (g *Game) Apply(pos game.Position, in game.MoveInput) (game.Position, game.Move, error) {
	st := pos.(*State)
	if st.Winner != 0 || st.IsDraw {
		return nil, game.Move{}, game.NewError(game.CodeGameOver, "对局已经结束")
	}
	if in.Index == nil {
		return nil, game.Move{}, game.NewError(game.CodeInvalidMove, "connect4 需要使用 index 指定落点")
	}
	idx := *in.Index
	if idx < 0 || idx >= Size {
		return nil, game.Move{}, game.NewError(game.CodeInvalidMove, "落点 %d 超出棋盘", idx)
	}
	col := idx % Cols
	want, ok := landing(st.Board, col)
	if !ok {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "第 %d 列已满", col)
	}
	if want != idx {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "第 %d 列的落点应为 %d，而不是 %d", col, want, idx)
	}
	next := *st
	next.Board[idx] = st.Turn
	next.Last = idx
	next.Moves++
	if line := winningLine(next.Board, idx, st.Turn); line != nil {
		next.Winner = st.Turn
		copy(next.winLine[:], line)
	} else if next.Moves == Size {
		next.IsDraw = true
	}
	next.Turn = other(st.Turn)
	m := coordMove(idx)
	m.Notation = notation(idx)
	return &next, m, nil
}

// Search implements game.Game.
func (g *Game) Search(ctx context.Context, pos game.Position, opts game.SearchOptions) (game.SearchResult, error) {
	st := pos.(*State)
	start := time.Now()
	result := game.SearchResult{Game: "connect4", Difficulty: game.NormalizeDifficulty(opts.Difficulty)}
	if st.Winner != 0 || st.IsDraw {
		result.GameOver = true
		result.Status = g.Encode(st, false).Status
		return result, nil
	}
	depth := game.DepthFor(opts.Difficulty, opts.MaxDepth, 12)
	limit := game.TimeFor(opts.Difficulty, opts.TimeLimit)
	deadline := start.Add(limit)
	nodes := int64(0)
	timedOut := false
	var best int = -1
	var bestScore int
	var reached int
	bestLine := []int{}

	legal := legalIndexes(st)
	rootMoves := make([]int, 0, len(legal))
	for _, idx := range legal {
		if !opts.Forbids(coordMove(idx)) {
			rootMoves = append(rootMoves, idx)
		}
	}
	if len(legal) > 0 && len(rootMoves) == 0 {
		return game.SearchResult{}, game.NewError(game.CodeBadRequest, "forbiddenMoves 覆盖了全部合法着法，没有可走的棋")
	}
	orderMoves(st, rootMoves)
	for d := 1; d <= depth; d++ {
		if time.Now().After(deadline) || ctx.Err() != nil {
			timedOut = true
			break
		}
		alpha, beta := -winScore*2, winScore*2
		var localBest int = -1
		var localScore int
		complete := true
		for _, idx := range rootMoves {
			child := drop(st, idx)
			score := -negamax(child, d-1, -beta, -alpha, 1, &nodes, deadline, ctx, &timedOut)
			if timedOut {
				complete = false
				break
			}
			if localBest == -1 || score > localScore {
				localScore = score
				localBest = idx
			}
			if score > alpha {
				alpha = score
			}
		}
		if localBest < 0 || !complete {
			break
		}
		best = localBest
		bestScore = localScore
		bestLine = []int{localBest}
		reached = d
		bestMove := coordMove(best)
		opts.Report(game.SearchProgress{
			Depth: d, Score: bestScore, Nodes: nodes, TimeMs: time.Since(start).Milliseconds(),
			Best: &bestMove, PV: []game.Move{bestMove},
		})
		// Keep the previous best first at the next iteration.
		rootMoves = moveToFront(rootMoves, best)
		if abs(bestScore) >= winScore-100 {
			break
		}
		if time.Now().After(deadline) || timedOut {
			break
		}
	}
	if best < 0 {
		// The only way this happens is an immediate timeout; fall back to the
		// first playable move so the API always returns something playable.
		if len(rootMoves) == 0 {
			result.GameOver = true
			result.Status = g.Encode(st, false).Status
			return result, nil
		}
		best = rootMoves[0]
		bestScore = 0
	}
	next, move, _ := g.Apply(st, game.MoveInput{Index: &best})
	nv := g.Encode(next, false)
	result.Move = &move
	result.Status = nv.Status
	result.GameOver = nv.Status.Finished()
	result.Position = &nv
	result.Evaluation = game.Evaluation{Score: bestScore, Unit: "centipawn", Perspective: turnString(st.Turn)}
	if abs(bestScore) >= winScore-100 {
		result.Evaluation.MateIn = (winScore - abs(bestScore) + 1) / 2
		if bestScore < 0 {
			result.Evaluation.MateIn = -result.Evaluation.MateIn
		}
	}
	result.Search = game.SearchInfo{Depth: reached, Nodes: nodes, TimeMs: time.Since(start).Milliseconds(), TimedOut: timedOut}
	if len(bestLine) > 0 {
		result.Search.PV = []game.Move{coordMove(bestLine[0])}
	}
	return result, nil
}

func negamax(st *State, depth, alpha, beta, ply int, nodes *int64, deadline time.Time, ctx context.Context, timedOut *bool) int {
	*nodes++
	if *nodes&1023 == 0 && (time.Now().After(deadline) || ctx.Err() != nil) {
		*timedOut = true
		return 0
	}
	if st.Winner != 0 {
		return -winScore + ply
	}
	if st.IsDraw {
		return 0
	}
	if depth <= 0 {
		return evaluate(st)
	}
	moves := legalIndexes(st)
	orderMoves(st, moves)
	best := -winScore * 2
	for _, idx := range moves {
		child := drop(st, idx)
		score := -negamax(child, depth-1, -beta, -alpha, ply+1, nodes, deadline, ctx, timedOut)
		if score > best {
			best = score
		}
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			break
		}
		if *timedOut {
			break
		}
	}
	return best
}

func evaluate(st *State) int {
	score := 0
	// Every 4-cell window contributes pattern pressure.
	for _, w := range windows {
		r, y := 0, 0
		for _, idx := range w {
			switch st.Board[idx] {
			case Red:
				r++
			case Yellow:
				y++
			}
		}
		if r > 0 && y > 0 {
			continue
		}
		switch {
		case r == 3:
			score += 60
		case r == 2:
			score += 12
		case r == 1:
			score += 2
		case y == 3:
			score -= 60
		case y == 2:
			score -= 12
		case y == 1:
			score -= 2
		}
	}
	// Center control.
	for row := 0; row < Rows; row++ {
		v := st.Board[row*Cols+3]
		if v == Red {
			score += 6
		} else if v == Yellow {
			score -= 6
		}
	}
	if st.Turn == Yellow {
		return -score
	}
	return score
}

var windows = buildWindows()

func buildWindows() [][4]int {
	var out [][4]int
	for row := 0; row < Rows; row++ {
		for col := 0; col < Cols; col++ {
			for _, d := range [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}} {
				r2, c2 := row+3*d[0], col+3*d[1]
				if r2 < 0 || r2 >= Rows || c2 < 0 || c2 >= Cols {
					continue
				}
				out = append(out, [4]int{
					row*Cols + col,
					(row+d[0])*Cols + col + d[1],
					(row+2*d[0])*Cols + col + 2*d[1],
					(row+3*d[0])*Cols + col + 3*d[1],
				})
			}
		}
	}
	return out
}

func winningLine(b [Size]int8, idx int, color int8) []int {
	row, col := idx/Cols, idx%Cols
	for _, d := range [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}} {
		line := []int{idx}
		for _, sign := range []int{-1, 1} {
			r, c := row+d[0]*sign, col+d[1]*sign
			for r >= 0 && r < Rows && c >= 0 && c < Cols && b[r*Cols+c] == color {
				if sign < 0 {
					line = append([]int{r*Cols + c}, line...)
				} else {
					line = append(line, r*Cols+c)
				}
				r += d[0] * sign
				c += d[1] * sign
			}
		}
		if len(line) >= 4 {
			return line[:4]
		}
	}
	return nil
}

func legalIndexes(st *State) []int {
	out := make([]int, 0, Cols)
	for col := 0; col < Cols; col++ {
		if idx, ok := landing(st.Board, col); ok {
			out = append(out, idx)
		}
	}
	return out
}

func landing(b [Size]int8, col int) (int, bool) {
	for row := Rows - 1; row >= 0; row-- {
		idx := row*Cols + col
		if b[idx] == Empty {
			return idx, true
		}
	}
	return 0, false
}

func drop(st *State, idx int) *State {
	next := *st
	next.Board[idx] = st.Turn
	next.Last = idx
	next.Moves++
	if line := winningLine(next.Board, idx, st.Turn); line != nil {
		next.Winner = st.Turn
		copy(next.winLine[:], line)
	} else if next.Moves == Size {
		next.IsDraw = true
	}
	next.Turn = other(st.Turn)
	return &next
}

func orderMoves(st *State, moves []int) {
	// Center columns first, then moves that complete or block a line.
	score := func(idx int) int {
		col := idx % Cols
		v := 10 - abs(col-3)*2
		if winningLine(st.Board, idx, st.Turn) != nil {
			v += 1000
		}
		if winningLine(st.Board, idx, other(st.Turn)) != nil {
			v += 500
		}
		return v
	}
	for i := 1; i < len(moves); i++ {
		for j := i; j > 0 && score(moves[j]) > score(moves[j-1]); j-- {
			moves[j], moves[j-1] = moves[j-1], moves[j]
		}
	}
}

func moveToFront(moves []int, m int) []int {
	out := make([]int, 0, len(moves))
	out = append(out, m)
	for _, v := range moves {
		if v != m {
			out = append(out, v)
		}
	}
	return out
}

func coordMove(idx int) game.Move {
	idxCopy := idx
	return game.Move{
		Index: &game.Coord{Index: idxCopy, Row: idx / Cols, Col: idx % Cols},
	}
}

func notation(idx int) string {
	return fmt.Sprintf("%c%d", 'A'+idx%Cols, Rows-idx/Cols)
}

func turnString(t int8) string {
	if t == Yellow {
		return "y"
	}
	return "r"
}

func other(t int8) int8 {
	if t == Red {
		return Yellow
	}
	return Red
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
