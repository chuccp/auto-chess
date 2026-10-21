// Package gomoku implements free-style five-in-a-row on a 15x15 board with a
// tactical alpha-beta AI.
package gomoku

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

const (
	Rows = 15
	Cols = 15
	Size = Rows * Cols

	Empty int8 = 0
	Black int8 = 1
	White int8 = 2

	winScore = 10_000_000
)

// State is a gomoku position.
type State struct {
	Board  [Size]int8
	Turn   int8
	Moves  int
	Last   int
	Winner int8
	Draw   bool
}

// Game implements game.Game.
type Game struct{}

// New returns a gomoku engine.
func New() *Game { return &Game{} }

// Meta implements game.Game.
func (g *Game) Meta() game.Meta {
	return game.Meta{
		ID:          "gomoku",
		Aliases:     []string{"five-in-a-row", "gobang"},
		Name:        "五子棋",
		EnglishName: "Gomoku",
		Tagline:     "15×15 自由规则，横竖斜先连成五子者胜。",
		Category:    "连珠类",
		Players:     2,
		Board: game.BoardMeta{
			Rows: Rows, Cols: Cols, Cells: Size, Type: "grid",
			Origin: "row 0 为最上行，index = row*15 + col",
			Legend: map[string]string{"": "空位", "b": "黑方", "w": "白方"},
		},
		Colors:         []game.ColorMeta{{ID: "b", Name: "黑方"}, {ID: "w", Name: "白方"}},
		PositionFormat: `{"board":["","", ...225格], "turn":"b"}`,
		MoveFormat:     `{"index": 落子格子下标（0-224）}`,
		Difficulties:   game.DifficultyOrder,
		Features:       []string{"自由五子规则（长连也算胜）", "攻防威胁检测", "候选点裁剪", "迭代加深 Alpha-Beta"},
		Rules: []string{
			"黑方先行，双方轮流在空交叉点落子。",
			"横、竖、两条斜线任意方向先形成连续五子或更多的一方获胜。",
			"棋盘填满仍未分胜负则为和棋。",
		},
	}
}

// Initial implements game.Game.
func (g *Game) Initial() game.Position {
	return &State{Turn: Black, Last: -1}
}

// Decode implements game.Game.
func (g *Game) Decode(in game.PositionInput) (game.Position, error) {
	if len(in.Board) != Size {
		return nil, game.NewError(game.CodeInvalidPosition, "gomoku 棋盘必须包含 %d 个格子，实际 %d 个", Size, len(in.Board))
	}
	st := &State{Turn: Black, Last: -1}
	for i, cell := range in.Board {
		switch cell {
		case "", "0", ".", "-":
		case "b", "B", "black":
			st.Board[i] = Black
			st.Moves++
		case "w", "W", "white":
			st.Board[i] = White
			st.Moves++
		default:
			return nil, game.NewError(game.CodeInvalidPosition, "第 %d 格的值 %q 无效，只支持 b / w / 空", i, cell)
		}
	}
	b, w := count(st.Board, Black), count(st.Board, White)
	if b != w && b != w+1 {
		return nil, game.NewError(game.CodeInvalidPosition, "黑白棋子数量不平衡（黑 %d / 白 %d）", b, w)
	}
	switch in.Turn {
	case "", "b", "B", "black":
		st.Turn = Black
	case "w", "W", "white":
		st.Turn = White
	default:
		return nil, game.NewError(game.CodeInvalidPosition, "turn 必须是 b 或 w，实际 %q", in.Turn)
	}
	// Black moves first, so the piece counts determine whose turn it is. Reject
	// a turn that would let one side move twice in a row.
	expected := Black
	if b > w {
		expected = White
	}
	if in.Turn != "" && st.Turn != expected {
		return nil, game.NewError(game.CodeInvalidPosition,
			"turn=%s 与棋子数量不一致，当前应为 %s", turnString(st.Turn), turnString(expected))
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

func (s *State) recompute() {
	s.Winner = 0
	s.Draw = false
	for i, v := range s.Board {
		if v != Empty && wouldWin(s.Board, i, v) {
			s.Winner = v
			break
		}
	}
	if s.Winner == 0 && s.Moves == Size {
		s.Draw = true
	}
}

// Encode implements game.Game.
func (g *Game) Encode(pos game.Position, includeLegalMoves bool) game.PositionView {
	st := pos.(*State)
	board := make([]string, Size)
	for i, v := range st.Board {
		switch v {
		case Black:
			board[i] = "b"
		case White:
			board[i] = "w"
		default:
			board[i] = ""
		}
	}
	status := game.Status{State: "playing"}
	switch {
	case st.Winner == Black:
		status = game.Status{State: "win", Winner: "b", Reason: "five"}
	case st.Winner == White:
		status = game.Status{State: "win", Winner: "w", Reason: "five"}
	case st.Draw:
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
		m.Notation = notation(st.Last)
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
	if st.Winner != 0 || st.Draw {
		return nil
	}
	moves := make([]game.Move, 0, 64)
	for i, v := range st.Board {
		if v == Empty {
			moves = append(moves, coordMove(i))
		}
	}
	return moves
}

// Apply implements game.Game.
func (g *Game) Apply(pos game.Position, in game.MoveInput) (game.Position, game.Move, error) {
	st := pos.(*State)
	if st.Winner != 0 || st.Draw {
		return nil, game.Move{}, game.NewError(game.CodeGameOver, "对局已经结束")
	}
	if in.Index == nil {
		return nil, game.Move{}, game.NewError(game.CodeInvalidMove, "gomoku 需要使用 index 指定落子位置")
	}
	idx := *in.Index
	if idx < 0 || idx >= Size {
		return nil, game.Move{}, game.NewError(game.CodeInvalidMove, "落子位置 %d 超出棋盘", idx)
	}
	if st.Board[idx] != Empty {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "位置 %s 已有棋子", notation(idx))
	}
	next := *st
	next.Board[idx] = st.Turn
	next.Last = idx
	next.Moves++
	if wouldWin(next.Board, idx, st.Turn) {
		next.Winner = st.Turn
	} else if next.Moves == Size {
		next.Draw = true
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
	result := game.SearchResult{Game: "gomoku", Difficulty: game.NormalizeDifficulty(opts.Difficulty)}
	if st.Winner != 0 || st.Draw {
		result.GameOver = true
		result.Status = g.Encode(st, false).Status
		return result, nil
	}
	depth := game.DepthFor(opts.Difficulty, opts.MaxDepth, 8)
	if opts.MaxDepth == 0 {
		switch game.NormalizeDifficulty(opts.Difficulty) {
		case "easy":
			depth = 2
		case "normal":
			depth = 4
		case "hard":
			depth = 6
		case "master":
			depth = 8
		}
	}
	limit := game.TimeFor(opts.Difficulty, opts.TimeLimit)
	deadline := start.Add(limit)
	nodes := int64(0)
	timedOut := false
	rng := rand.New(rand.NewSource(opts.Seed))

	// Caller-banned points must never be played; everything else stays legal.
	banned := func(idx int) bool { return opts.Forbids(coordMove(idx)) }
	keep := func(list []int) []int {
		out := make([]int, 0, len(list))
		for _, idx := range list {
			if !banned(idx) {
				out = append(out, idx)
			}
		}
		return out
	}

	// Opening: play the center point (unless the caller forbids it).
	if st.Moves == 0 {
		idx := (Rows/2)*Cols + Cols/2
		if !banned(idx) {
			next, move, _ := g.Apply(st, game.MoveInput{Index: &idx})
			nv := g.Encode(next, false)
			result.Move = &move
			result.Position = &nv
			result.Status = nv.Status
			result.Evaluation = game.Evaluation{Score: 0, Unit: "centipawn", Perspective: turnString(st.Turn)}
			result.Search = game.SearchInfo{Depth: 1, TimeMs: time.Since(start).Milliseconds(), PV: []game.Move{move}}
			opts.Report(game.SearchProgress{Depth: 1, Score: 0, TimeMs: time.Since(start).Milliseconds(), Best: &move, PV: []game.Move{move}})
			return result, nil
		}
	}

	// Immediate win.
	if wins := keep(winningMoves(st)); len(wins) > 0 {
		idx := wins[rng.Intn(len(wins))]
		next, move, _ := g.Apply(st, game.MoveInput{Index: &idx})
		nv := g.Encode(next, false)
		result.Move = &move
		result.Position = &nv
		result.Status = nv.Status
		result.GameOver = nv.Status.Finished()
		result.Evaluation = game.Evaluation{Score: winScore, Unit: "centipawn", Perspective: turnString(st.Turn), MateIn: 1}
		result.Search = game.SearchInfo{Depth: 1, TimeMs: time.Since(start).Milliseconds(), PV: []game.Move{move}}
		opts.Report(game.SearchProgress{Depth: 1, Score: winScore, TimeMs: time.Since(start).Milliseconds(), Best: &move, PV: []game.Move{move}})
		return result, nil
	}

	candidates := keep(candidates(st, depth, 18))
	if blocks := keep(winningMovesFor(st, other(st.Turn))); len(blocks) > 0 {
		// The opponent threatens an immediate five. Only blocking moves can
		// survive, so do not let the candidate cap discard them.
		candidates = blocks
	}
	if len(candidates) == 0 {
		// The ban list covered every heuristic candidate; every remaining
		// legal point is still playable.
		for idx, v := range st.Board {
			if v == Empty && !banned(idx) {
				candidates = append(candidates, idx)
			}
		}
	}
	if len(candidates) == 0 {
		return game.SearchResult{}, game.NewError(game.CodeBadRequest, "forbiddenMoves 覆盖了全部合法着法，没有可走的棋")
	}
	best := candidates[0]
	bestScore := 0
	reached := 0
	for d := 1; d <= depth; d++ {
		if time.Now().After(deadline) || ctx.Err() != nil {
			timedOut = true
			break
		}
		alpha, beta := -winScore*2, winScore*2
		localBest := -1
		localScore := 0
		complete := true
		for _, idx := range candidates {
			child := place(st, idx)
			score := -negamax(child, d-1, -beta, -alpha, 1, &nodes, deadline, ctx, &timedOut)
			if timedOut {
				complete = false
				break
			}
			if localBest < 0 || score > localScore {
				localBest = idx
				localScore = score
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
		reached = d
		bestMove := coordMove(best)
		opts.Report(game.SearchProgress{
			Depth: d, Score: bestScore, Nodes: nodes, TimeMs: time.Since(start).Milliseconds(),
			Best: &bestMove, PV: []game.Move{bestMove},
		})
		candidates = moveToFront(candidates, best)
		if bestScore >= winScore-1000 {
			break
		}
	}
	next, move, _ := g.Apply(st, game.MoveInput{Index: &best})
	nv := g.Encode(next, false)
	result.Move = &move
	result.Status = nv.Status
	result.GameOver = nv.Status.Finished()
	result.Position = &nv
	result.Evaluation = game.Evaluation{Score: bestScore, Unit: "centipawn", Perspective: turnString(st.Turn)}
	if abs(bestScore) >= winScore-1000 {
		result.Evaluation.MateIn = (winScore - abs(bestScore) + 2) / 2
		if bestScore < 0 {
			result.Evaluation.MateIn = -result.Evaluation.MateIn
		}
	}
	result.Search = game.SearchInfo{Depth: reached, Nodes: nodes, TimeMs: time.Since(start).Milliseconds(), TimedOut: timedOut, PV: []game.Move{move}}
	return result, nil
}

func negamax(st *State, depth, alpha, beta, ply int, nodes *int64, deadline time.Time, ctx context.Context, timedOut *bool) int {
	*nodes++
	if *nodes&511 == 0 && (time.Now().After(deadline) || ctx.Err() != nil) {
		*timedOut = true
		return 0
	}
	if st.Winner != 0 {
		return -winScore + ply
	}
	if st.Draw {
		return 0
	}
	if depth <= 0 {
		return evaluate(st, st.Turn)
	}
	// Win now.
	if wins := winningMoves(st); len(wins) > 0 {
		return winScore - ply
	}
	// The opponent threatens to win: forced blocks only.
	if blocks := winningMovesFor(st, other(st.Turn)); len(blocks) > 0 {
		if len(blocks) > 2 {
			return -winScore + ply + 1
		}
		best := -winScore * 2
		for _, idx := range blocks {
			child := place(st, idx)
			score := -negamax(child, depth-1, -beta, -alpha, ply+1, nodes, deadline, ctx, timedOut)
			if score > best {
				best = score
			}
			if score > alpha {
				alpha = score
			}
			if alpha >= beta || *timedOut {
				break
			}
		}
		return best
	}
	moves := candidates(st, depth, 10)
	best := -winScore * 2
	for _, idx := range moves {
		child := place(st, idx)
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

func place(st *State, idx int) *State {
	next := *st
	next.Board[idx] = st.Turn
	next.Last = idx
	next.Moves++
	if wouldWin(next.Board, idx, st.Turn) {
		next.Winner = st.Turn
	} else if next.Moves == Size {
		next.Draw = true
	}
	next.Turn = other(st.Turn)
	return &next
}

func winningMoves(st *State) []int {
	return winningMovesFor(st, st.Turn)
}

func winningMovesFor(st *State, color int8) []int {
	var out []int
	for i, v := range st.Board {
		if v != Empty {
			continue
		}
		// Cheap neighborhood filter.
		if !hasNeighbor(st.Board, i, 2) {
			continue
		}
		st.Board[i] = color
		win := wouldWin(st.Board, i, color)
		st.Board[i] = Empty
		if win {
			out = append(out, i)
			if len(out) >= 8 {
				break
			}
		}
	}
	return out
}

func candidates(st *State, depth, max int) []int {
	type scored struct {
		idx   int
		score int
	}
	var list []scored
	for i, v := range st.Board {
		if v != Empty || !hasNeighbor(st.Board, i, 2) {
			continue
		}
		list = append(list, scored{idx: i, score: moveScore(st.Board, i, st.Turn)})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].score > list[j].score })
	if len(list) == 0 {
		// Empty board fallback.
		return []int{(Rows/2)*Cols + Cols/2}
	}
	if len(list) > max {
		list = list[:max]
	}
	out := make([]int, len(list))
	for i, s := range list {
		out[i] = s.idx
	}
	return out
}

func hasNeighbor(b [Size]int8, idx, dist int) bool {
	row, col := idx/Cols, idx%Cols
	for dr := -dist; dr <= dist; dr++ {
		for dc := -dist; dc <= dist; dc++ {
			if dr == 0 && dc == 0 {
				continue
			}
			r, c := row+dr, col+dc
			if r >= 0 && r < Rows && c >= 0 && c < Cols && b[r*Cols+c] != Empty {
				return true
			}
		}
	}
	return false
}

func moveScore(b [Size]int8, idx int, turn int8) int {
	row, col := idx/Cols, idx%Cols
	center := 14 - abs(row-7) - abs(col-7)
	score := center
	score += linePotential(b, idx, turn) * 2
	score += linePotential(b, idx, other(turn))
	return score
}

// linePotential scores how valuable placing color at idx would be, using the
// longest contiguous run plus open ends in each direction.
func linePotential(b [Size]int8, idx int, color int8) int {
	total := 0
	row, col := idx/Cols, idx%Cols
	for _, d := range [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}} {
		count := 1
		open := 0
		for _, sign := range []int{-1, 1} {
			r, c := row+d[0]*sign, col+d[1]*sign
			for r >= 0 && r < Rows && c >= 0 && c < Cols && b[r*Cols+c] == color {
				count++
				r += d[0] * sign
				c += d[1] * sign
			}
			if r >= 0 && r < Rows && c >= 0 && c < Cols && b[r*Cols+c] == Empty {
				open++
			}
		}
		total += runScore(count, open)
	}
	return total
}

func runScore(count, open int) int {
	switch {
	case count >= 5:
		return winScore
	case count == 4:
		if open == 2 {
			return 1_000_000
		}
		if open == 1 {
			return 100_000
		}
	case count == 3:
		if open == 2 {
			return 20_000
		}
		if open == 1 {
			return 2_000
		}
	case count == 2:
		if open == 2 {
			return 500
		}
		if open == 1 {
			return 100
		}
	case count == 1:
		if open == 2 {
			return 20
		}
		return 5
	}
	return 0
}

func evaluate(st *State, side int8) int {
	black, white := 0, 0
	for i, v := range st.Board {
		if v == Empty {
			continue
		}
		score := cellScore(st.Board, i, v)
		if v == Black {
			black += score
		} else {
			white += score
		}
	}
	diff := black - white
	if side == White {
		return -diff
	}
	return diff
}

func cellScore(b [Size]int8, idx int, color int8) int {
	row, col := idx/Cols, idx%Cols
	total := 0
	for _, d := range [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}} {
		// Count each run once, from its start.
		pr, pc := row-d[0], col-d[1]
		if pr >= 0 && pr < Rows && pc >= 0 && pc < Cols && b[pr*Cols+pc] == color {
			continue
		}
		count := 0
		r, c := row, col
		for r >= 0 && r < Rows && c >= 0 && c < Cols && b[r*Cols+c] == color {
			count++
			r += d[0]
			c += d[1]
		}
		open := 0
		if pr >= 0 && pr < Rows && pc >= 0 && pc < Cols && b[pr*Cols+pc] == Empty {
			open++
		}
		if r >= 0 && r < Rows && c >= 0 && c < Cols && b[r*Cols+c] == Empty {
			open++
		}
		total += runScore(count, open)
	}
	return total
}

func wouldWin(b [Size]int8, idx int, color int8) bool {
	row, col := idx/Cols, idx%Cols
	for _, d := range [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}} {
		count := 1
		for _, sign := range []int{-1, 1} {
			r, c := row+d[0]*sign, col+d[1]*sign
			for r >= 0 && r < Rows && c >= 0 && c < Cols && b[r*Cols+c] == color {
				count++
				r += d[0] * sign
				c += d[1] * sign
			}
		}
		if count >= 5 {
			return true
		}
	}
	return false
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
	return game.Move{Index: &game.Coord{Index: idxCopy, Row: idx / Cols, Col: idx % Cols}}
}

func notation(idx int) string {
	return fmt.Sprintf("%c%d", 'A'+idx%Cols, idx/Cols+1)
}

func turnString(t int8) string {
	if t == White {
		return "w"
	}
	return "b"
}

func other(t int8) int8 {
	if t == Black {
		return White
	}
	return Black
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
