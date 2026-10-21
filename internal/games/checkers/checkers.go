// Package checkers implements Chinese Checkers on the 121-hole star board used
// by chess-train, with a distance-heuristic search AI.
package checkers

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

const (
	Rows  = 17
	Holes = 121

	Red   int8 = 1
	Black int8 = 2

	mateScore = 100_000
)

var rowCounts = [Rows]int{1, 2, 3, 4, 13, 12, 11, 10, 9, 10, 11, 12, 13, 4, 3, 2, 1}

var (
	holeRow   [Holes]int
	holeX2    [Holes]int
	holeByXY  = map[[2]int]int{}
	neighbors [Holes][]int
	homeRed   map[int]bool
	homeBlack map[int]bool
)

func init() {
	for row := 0; row < Rows; row++ {
		count := rowCounts[row]
		for i := 0; i < count; i++ {
			x2 := i*2 - (count - 1)
			id := 0
			for r := 0; r < row; r++ {
				id += rowCounts[r]
			}
			id += i
			holeRow[id] = row
			holeX2[id] = x2
			holeByXY[[2]int{row, x2}] = id
		}
	}
	dirs := [][2]int{{0, 2}, {0, -2}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	for id := 0; id < Holes; id++ {
		for _, d := range dirs {
			if n, ok := holeByXY[[2]int{holeRow[id] + d[0], holeX2[id] + d[1]}]; ok {
				neighbors[id] = append(neighbors[id], n)
			}
		}
	}
	homeBlack = map[int]bool{}
	homeRed = map[int]bool{}
	for id := 0; id < Holes; id++ {
		if holeRow[id] <= 3 {
			homeBlack[id] = true
		}
		if holeRow[id] >= 13 {
			homeRed[id] = true
		}
	}
}

// State is a Chinese-checkers position.
type State struct {
	Board     [Holes]int8
	Turn      int8
	LastFrom  int
	LastTo    int
	Winner    int8
	WinReason string
	Moves     int
}

type rawMove struct {
	from, to int
	jump     bool
}

// Game implements game.Game.
type Game struct{}

// New returns a Chinese-checkers engine.
func New() *Game { return &Game{} }

// Meta implements game.Game.
func (g *Game) Meta() game.Meta {
	return game.Meta{
		ID:          "checkers",
		Aliases:     []string{"chinese-checkers", "tiaoqi"},
		Name:        "中国跳棋",
		EnglishName: "Chinese Checkers",
		Tagline:     "121 孔星形棋盘，十子归营，可连跳。",
		Category:    "跳棋类",
		Players:     2,
		Board: game.BoardMeta{
			Rows: Rows, Cols: 13, Cells: Holes, Type: "grid",
			Origin: "17 行，每行孔数 1,2,3,4,13,12,11,10,9,10,11,12,13,4,3,2,1；index 为孔位编号，row 从上到下，col 为 x2（可为负）",
			Legend: map[string]string{"": "空孔", "r": "红方", "b": "黑方"},
		},
		Colors:         []game.ColorMeta{{ID: "r", Name: "红方"}, {ID: "b", Name: "黑方"}},
		PositionFormat: `{"board":["r","","", ...121孔], "turn":"r"}`,
		MoveFormat:     `{"from": 起点孔位, "to": 终点孔位}；一步可走一格，或连续跳跃`,
		Difficulties:   game.DifficultyOrder,
		Features:       []string{"121 孔星形棋盘", "单步与连跳", "十子归营判胜", "最短路径评估", "束搜索"},
		Rules: []string{
			"红方先行，双方各 10 枚棋子，初始占据对顶的两个角。",
			"每回合可将一枚棋子走到相邻空孔，或连续跳过相邻棋子落到空孔。",
			"跳跃可以跨越任意颜色的棋子，被跨越的棋子不动，也不被吃掉。",
			"先把自己的 10 枚棋子全部走进对面角的阵营者获胜。",
			"一方无任何合法走法时判负。",
		},
	}
}

// Initial implements game.Game.
func (g *Game) Initial() game.Position {
	st := &State{Turn: Red, LastFrom: -1, LastTo: -1}
	for id := 0; id < Holes; id++ {
		if homeRed[id] {
			st.Board[id] = Red
		} else if homeBlack[id] {
			st.Board[id] = Black
		}
	}
	return st
}

// Decode implements game.Game.
func (g *Game) Decode(in game.PositionInput) (game.Position, error) {
	if len(in.Board) != Holes {
		return nil, game.NewError(game.CodeInvalidPosition, "checkers 棋盘必须包含 %d 个孔位，实际 %d 个", Holes, len(in.Board))
	}
	st := &State{Turn: Red, LastFrom: -1, LastTo: -1}
	for i, cell := range in.Board {
		switch cell {
		case "", "0", ".", "-":
		case "r", "R", "red":
			st.Board[i] = Red
		case "b", "B", "black":
			st.Board[i] = Black
		default:
			return nil, game.NewError(game.CodeInvalidPosition, "第 %d 孔的值 %q 无效，只支持 r / b / 空", i, cell)
		}
	}
	switch in.Turn {
	case "", "r", "R", "red":
		st.Turn = Red
	case "b", "B", "black":
		st.Turn = Black
	default:
		return nil, game.NewError(game.CodeInvalidPosition, "turn 必须是 r 或 b，实际 %q", in.Turn)
	}
	st.recomputeWinner()
	return st, nil
}

func (s *State) recomputeWinner() {
	s.Winner = 0
	s.WinReason = ""
	for _, color := range []int8{Red, Black} {
		if allHome(s.Board, color) {
			s.Winner = color
			s.WinReason = "all-home"
			return
		}
	}
	if len(genMoves(s)) == 0 {
		s.Winner = other(s.Turn)
		s.WinReason = "blocked"
	}
}

// Encode implements game.Game.
func (g *Game) Encode(pos game.Position, includeLegalMoves bool) game.PositionView {
	st := pos.(*State)
	board := make([]string, Holes)
	for i, p := range st.Board {
		switch p {
		case Red:
			board[i] = "r"
		case Black:
			board[i] = "b"
		default:
			board[i] = ""
		}
	}
	status := game.Status{State: "playing"}
	if st.Winner != 0 {
		status = game.Status{State: "win", Winner: colorString(st.Winner), Reason: st.WinReason}
	}
	view := game.PositionView{
		Board:      board,
		Turn:       colorString(st.Turn),
		MoveNumber: st.Moves + 1,
		Status:     status,
	}
	if st.LastFrom >= 0 {
		m := moveView(st.LastFrom, st.LastTo, isAdjacent(st.LastFrom, st.LastTo))
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
	if st.Winner != 0 {
		return nil
	}
	raw := genMoves(st)
	out := make([]game.Move, 0, len(raw))
	for _, m := range raw {
		out = append(out, moveView(m.from, m.to, m.jump))
	}
	return out
}

// Apply implements game.Game.
func (g *Game) Apply(pos game.Position, in game.MoveInput) (game.Position, game.Move, error) {
	st := pos.(*State)
	if st.Winner != 0 {
		return nil, game.Move{}, game.NewError(game.CodeGameOver, "对局已经结束")
	}
	if in.From == nil || in.To == nil {
		return nil, game.Move{}, game.NewError(game.CodeInvalidMove, "checkers 需要使用 from 与 to 指定起点和终点")
	}
	from, to := *in.From, *in.To
	if from < 0 || from >= Holes || to < 0 || to >= Holes {
		return nil, game.Move{}, game.NewError(game.CodeInvalidMove, "孔位下标超出范围")
	}
	if st.Board[from] != st.Turn {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "起点没有轮到行动的一方棋子")
	}
	if st.Board[to] != 0 {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "终点孔位已有棋子")
	}
	var legal *rawMove
	for _, m := range genMoves(st) {
		if m.from == from && m.to == to {
			c := m
			legal = &c
			break
		}
	}
	if legal == nil {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "该走法不合法")
	}
	next := *st
	next.Board[from] = 0
	next.Board[to] = st.Turn
	next.LastFrom, next.LastTo = from, to
	next.Moves++
	next.Turn = other(st.Turn)
	if allHome(next.Board, st.Turn) {
		next.Winner = st.Turn
		next.WinReason = "all-home"
	} else if len(genMoves(&next)) == 0 {
		next.Winner = st.Turn
		next.WinReason = "blocked"
	}
	m := moveView(from, to, legal.jump)
	return &next, m, nil
}

// Search implements game.Game.
func (g *Game) Search(ctx context.Context, pos game.Position, opts game.SearchOptions) (game.SearchResult, error) {
	st := pos.(*State)
	start := time.Now()
	result := game.SearchResult{Game: "checkers", Difficulty: game.NormalizeDifficulty(opts.Difficulty)}
	if st.Winner != 0 {
		result.GameOver = true
		result.Status = g.Encode(st, false).Status
		return result, nil
	}
	depth := 2
	switch game.NormalizeDifficulty(opts.Difficulty) {
	case "easy":
		depth = 1
	case "normal":
		depth = 2
	case "hard":
		depth = 3
	case "master":
		depth = 4
	}
	if opts.MaxDepth > 0 {
		depth = game.DepthFor(opts.Difficulty, opts.MaxDepth, 5)
	}
	limit := game.TimeFor(opts.Difficulty, opts.TimeLimit)
	deadline := start.Add(limit)
	nodes := int64(0)
	timedOut := false

	moves := genMoves(st)
	if len(moves) == 0 {
		result.GameOver = true
		result.Status = g.Encode(st, false).Status
		return result, nil
	}
	allowed := make([]rawMove, 0, len(moves))
	for _, m := range moves {
		if !opts.Forbids(moveView(m.from, m.to, m.jump)) {
			allowed = append(allowed, m)
		}
	}
	moves = allowed
	if len(moves) == 0 {
		return game.SearchResult{}, game.NewError(game.CodeBadRequest, "forbiddenMoves 覆盖了全部合法着法，没有可走的棋")
	}
	orderMoves(st, moves, depth)
	rootBeam := moves
	if len(rootBeam) > 24 {
		rootBeam = rootBeam[:24]
	}
	best := rootBeam[0]
	bestScore := 0
	reached := 0
	for d := 1; d <= depth; d++ {
		if time.Now().After(deadline) || ctx.Err() != nil {
			timedOut = true
			break
		}
		alpha, beta := -mateScore*2, mateScore*2
		localBest := -1
		localScore := 0
		complete := true
		for _, m := range rootBeam {
			child := applyRaw(st, m)
			var score int
			if child.Winner == st.Turn {
				score = mateScore - 1
			} else {
				score = -negamax(child, d-1, -beta, -alpha, 1, &nodes, deadline, ctx, &timedOut)
			}
			if timedOut {
				complete = false
				break
			}
			if localBest < 0 || score > localScore {
				localBest = m.from*Holes + m.to
				localScore = score
				best = m
			}
			if score > alpha {
				alpha = score
			}
		}
		if localBest < 0 || !complete {
			break
		}
		bestScore = localScore
		reached = d
		bestMove := moveView(best.from, best.to, best.jump)
		opts.Report(game.SearchProgress{
			Depth: d, Score: bestScore, Nodes: nodes, TimeMs: time.Since(start).Milliseconds(),
			Best: &bestMove, PV: []game.Move{bestMove},
		})
	}
	next, move, _ := g.Apply(st, game.MoveInput{From: &best.from, To: &best.to})
	nv := g.Encode(next, false)
	result.Move = &move
	result.Status = nv.Status
	result.GameOver = nv.Status.Finished()
	result.Position = &nv
	result.Evaluation = game.Evaluation{Score: bestScore, Unit: "centipawn", Perspective: colorString(st.Turn)}
	if bestScore >= mateScore-1000 {
		result.Evaluation.MateIn = 1
	}
	result.Search = game.SearchInfo{Depth: reached, Nodes: nodes, TimeMs: time.Since(start).Milliseconds(), TimedOut: timedOut, PV: []game.Move{move}}
	return result, nil
}

func negamax(st *State, depth, alpha, beta, ply int, nodes *int64, deadline time.Time, ctx context.Context, timedOut *bool) int {
	*nodes++
	if *nodes&255 == 0 && (time.Now().After(deadline) || ctx.Err() != nil) {
		*timedOut = true
		return 0
	}
	if st.Winner != 0 {
		return -mateScore + ply
	}
	if depth <= 0 {
		return evaluate(st)
	}
	moves := genMoves(st)
	if len(moves) == 0 {
		return -mateScore + ply
	}
	orderMoves(st, moves, depth)
	if len(moves) > 12 {
		moves = moves[:12]
	}
	best := -mateScore * 2
	for _, m := range moves {
		child := applyRaw(st, m)
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

func applyRaw(st *State, m rawMove) *State {
	next := *st
	next.Board[m.from] = 0
	next.Board[m.to] = st.Turn
	next.LastFrom, next.LastTo = m.from, m.to
	next.Moves++
	next.Turn = other(st.Turn)
	if allHome(next.Board, st.Turn) {
		next.Winner = st.Turn
		next.WinReason = "all-home"
	} else if len(genMoves(&next)) == 0 {
		next.Winner = st.Turn
		next.WinReason = "blocked"
	}
	return &next
}

func evaluate(st *State) int {
	score := 0
	for id, p := range st.Board {
		if p == 0 {
			continue
		}
		dist := distRed[id]
		goal := homeBlack[id]
		if p == Black {
			dist = distBlack[id]
			goal = homeRed[id]
		}
		v := -dist*12 + 200
		if goal {
			v += 120
		}
		v += (8 - abs(holeX2[id])) / 2
		if p == Red {
			score += v
		} else {
			score -= v
		}
	}
	if st.Turn == Black {
		return -score
	}
	return score
}

func orderMoves(st *State, moves []rawMove, depth int) {
	score := func(m rawMove) int {
		s := 0
		fromDist, toDist := distanceToGoal(st.Turn, m.from), distanceToGoal(st.Turn, m.to)
		s += (fromDist - toDist) * 30
		if m.jump {
			s += 15
		}
		if isGoal(st.Turn, m.to) {
			s += 200
		}
		if isGoal(st.Turn, m.from) && !isGoal(st.Turn, m.to) {
			s -= 180
		}
		return s
	}
	sort.SliceStable(moves, func(i, j int) bool { return score(moves[i]) > score(moves[j]) })
}

func distanceToGoal(color int8, id int) int {
	if color == Red {
		return distRed[id]
	}
	return distBlack[id]
}

func isGoal(color int8, id int) bool {
	if color == Red {
		return homeBlack[id]
	}
	return homeRed[id]
}

func genMoves(st *State) []rawMove {
	var moves []rawMove
	seen := map[int]bool{}
	add := func(from, to int, jump bool) {
		key := from*Holes + to
		if seen[key] {
			return
		}
		seen[key] = true
		moves = append(moves, rawMove{from: from, to: to, jump: jump})
	}
	for from, p := range st.Board {
		if p != st.Turn {
			continue
		}
		for _, n := range neighbors[from] {
			if st.Board[n] == 0 {
				add(from, n, false)
			}
		}
		// Jump chains.
		visited := map[int]bool{from: true}
		queue := []int{from}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			row, x2 := holeRow[cur], holeX2[cur]
			for _, d := range [][2]int{{0, 2}, {0, -2}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
				mid, okMid := holeByXY[[2]int{row + d[0], x2 + d[1]}]
				land, okLand := holeByXY[[2]int{row + 2*d[0], x2 + 2*d[1]}]
				if !okMid || !okLand {
					continue
				}
				if st.Board[mid] == 0 || st.Board[land] != 0 || visited[land] {
					continue
				}
				visited[land] = true
				add(from, land, true)
				queue = append(queue, land)
			}
		}
	}
	return moves
}

func allHome(board [Holes]int8, color int8) bool {
	goal := homeBlack
	if color == Black {
		goal = homeRed
	}
	count := 0
	for id, p := range board {
		if p == color {
			count++
			if !goal[id] {
				return false
			}
		}
	}
	return count == len(goal)
}

func isAdjacent(a, b int) bool {
	for _, n := range neighbors[a] {
		if n == b {
			return true
		}
	}
	return false
}

func moveView(from, to int, jump bool) game.Move {
	f, t := from, to
	return game.Move{
		From:     &game.Coord{Index: f, Row: holeRow[f], Col: holeX2[f]},
		To:       &game.Coord{Index: t, Row: holeRow[t], Col: holeX2[t]},
		Jump:     jump,
		Notation: fmt.Sprintf("%d/%d→%d/%d", holeRow[f]+1, holeX2[f], holeRow[t]+1, holeX2[t]),
	}
}

func colorString(c int8) string {
	if c == Black {
		return "b"
	}
	return "r"
}

func other(c int8) int8 {
	if c == Red {
		return Black
	}
	return Red
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// distRed[id] is the shortest step distance from hole id to Red's goal
// (the black home corner); distBlack is the mirror.
var (
	distRed   [Holes]int
	distBlack [Holes]int
)

func init() {
	distRed = bfsDistances(homeBlack)
	distBlack = bfsDistances(homeRed)
}

func bfsDistances(goal map[int]bool) [Holes]int {
	var dist [Holes]int
	for i := range dist {
		dist[i] = 1 << 20
	}
	queue := make([]int, 0, len(goal))
	for id := range goal {
		dist[id] = 0
		queue = append(queue, id)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range neighbors[cur] {
			if dist[n] > dist[cur]+1 {
				dist[n] = dist[cur] + 1
				queue = append(queue, n)
			}
		}
	}
	return dist
}
