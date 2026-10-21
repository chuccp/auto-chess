// Package jungle implements Dou Shou Qi (Jungle Chess) on a 7x9 board with a
// negamax AI.
package jungle

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

const (
	Rows = 9
	Cols = 7
	Size = Rows * Cols

	Red   int8 = 1
	Black int8 = 2

	Elephant int8 = 1
	Lion     int8 = 2
	Tiger    int8 = 3
	Leopard  int8 = 4
	Wolf     int8 = 5
	Dog      int8 = 6
	Cat      int8 = 7
	Rat      int8 = 8

	mateScore = 1_000_000
)

var animalLetter = map[int8]string{
	Elephant: "E", Lion: "L", Tiger: "T", Leopard: "P",
	Wolf: "W", Dog: "D", Cat: "C", Rat: "R",
}

var letterAnimal = map[byte]int8{
	'E': Elephant, 'L': Lion, 'T': Tiger, 'P': Leopard,
	'W': Wolf, 'D': Dog, 'C': Cat, 'R': Rat,
}

var power = map[int8]int{
	Elephant: 8, Lion: 7, Tiger: 6, Leopard: 5,
	Wolf: 4, Dog: 3, Cat: 2, Rat: 1,
}

var value = map[int8]int{
	Elephant: 800, Lion: 750, Tiger: 650, Leopard: 550,
	Wolf: 450, Dog: 350, Cat: 280, Rat: 300,
}

var dens = map[int8]int{Red: 8*Cols + 3, Black: 3}

var traps = map[int8]map[int]bool{
	Black: set(2, 4, 3+Cols),
	Red:   set(2+8*Cols, 4+8*Cols, 3+7*Cols),
}

var water = func() map[int]bool {
	m := map[int]bool{}
	for _, row := range []int{3, 4, 5} {
		for _, col := range []int{1, 2, 4, 5} {
			m[row*Cols+col] = true
		}
	}
	return m
}()

func set(items ...int) map[int]bool {
	m := make(map[int]bool, len(items))
	for _, v := range items {
		m[v] = true
	}
	return m
}

// State is a jungle position.
type State struct {
	Board     [Size]int8
	Turn      int8
	LastFrom  int
	LastTo    int
	Winner    int8
	WinReason string
	Moves     int
}

// Game implements game.Game.
type Game struct{}

// New returns a jungle engine.
func New() *Game { return &Game{} }

func piece(color, animal int8) int8 { return color*16 + animal }
func colorOf(p int8) int8           { return p >> 4 }
func animalOf(p int8) int8          { return p & 15 }
func other(c int8) int8 {
	if c == Red {
		return Black
	}
	return Red
}

// Meta implements game.Game.
func (g *Game) Meta() game.Meta {
	return game.Meta{
		ID:          "jungle",
		Aliases:     []string{"dou-shou-qi", "animal-chess"},
		Name:        "斗兽棋",
		EnglishName: "Jungle Chess (Dou Shou Qi)",
		Tagline:     "象狮虎豹狼狗猫鼠，踏入对方兽穴即胜。",
		Category:    "策略类",
		Players:     2,
		Board: game.BoardMeta{
			Rows: Rows, Cols: Cols, Cells: Size, Type: "grid",
			Origin: "row 0 为黑方底线（上方），index = row*7 + col；红方在下方先行",
			Legend: map[string]string{
				"": "空地", "rE": "红象", "rL": "红狮", "rT": "红虎", "rP": "红豹",
				"rW": "红狼", "rD": "红狗", "rC": "红猫", "rR": "红鼠",
				"bE": "黑象", "bL": "黑狮", "bT": "黑虎", "bP": "黑豹",
				"bW": "黑狼", "bD": "黑狗", "bC": "黑猫", "bR": "黑鼠",
			},
		},
		Colors:         []game.ColorMeta{{ID: "r", Name: "红方"}, {ID: "b", Name: "黑方"}},
		PositionFormat: `{"board":["rE","","", ...63格], "turn":"r"}`,
		MoveFormat:     `{"from": 起点下标, "to": 终点下标}`,
		Difficulties:   game.DifficultyOrder,
		Features:       []string{"完整斗兽棋规则", "狮虎跳河", "鼠象克制", "陷阱与兽穴", "Alpha-Beta 搜索"},
		Rules: []string{
			"红方先行，每次将一枚兽横向或纵向移动一格，或吃子。",
			"战力：象>狮>虎>豹>狼>狗>猫>鼠；鼠可吃象，象不能吃鼠。",
			"鼠可入水，水中鼠不能被陆上兽吃；狮、虎可跳过整片水域，但水中有鼠时不能跳。",
			"进入对方陷阱的兽战力变为 0。",
			"进入对方兽穴立即获胜；一方无子可走也判负。",
		},
	}
}

// Initial implements game.Game.
func (g *Game) Initial() game.Position {
	st := &State{Turn: Red, LastFrom: -1, LastTo: -1}
	place := func(color, animal int8, row, col int) {
		st.Board[row*Cols+col] = piece(color, animal)
	}
	// Black (top).
	place(Black, Lion, 0, 0)
	place(Black, Tiger, 0, 6)
	place(Black, Dog, 1, 1)
	place(Black, Cat, 1, 5)
	place(Black, Rat, 2, 0)
	place(Black, Leopard, 2, 2)
	place(Black, Wolf, 2, 4)
	place(Black, Elephant, 2, 6)
	// Red (bottom).
	place(Red, Tiger, 8, 0)
	place(Red, Lion, 8, 6)
	place(Red, Cat, 7, 1)
	place(Red, Dog, 7, 5)
	place(Red, Elephant, 6, 0)
	place(Red, Wolf, 6, 2)
	place(Red, Leopard, 6, 4)
	place(Red, Rat, 6, 6)
	return st
}

// Decode implements game.Game.
func (g *Game) Decode(in game.PositionInput) (game.Position, error) {
	if len(in.Board) != Size {
		return nil, game.NewError(game.CodeInvalidPosition, "jungle 棋盘必须包含 %d 个格子，实际 %d 个", Size, len(in.Board))
	}
	st := &State{Turn: Red, LastFrom: -1, LastTo: -1}
	for i, cell := range in.Board {
		if cell == "" || cell == "." || cell == "-" {
			continue
		}
		s := strings.TrimSpace(cell)
		if len(s) != 2 {
			return nil, game.NewError(game.CodeInvalidPosition, "第 %d 格的值 %q 无效，应形如 rE / bR", i, cell)
		}
		var color int8
		switch s[0] {
		case 'r', 'R':
			color = Red
		case 'b', 'B':
			color = Black
		default:
			return nil, game.NewError(game.CodeInvalidPosition, "第 %d 格的颜色 %q 无效，只支持 r / b", i, s[0])
		}
		animal, ok := letterAnimal[byte(strings.ToUpper(s[1:])[0])]
		if !ok {
			return nil, game.NewError(game.CodeInvalidPosition, "第 %d 格的兽种 %q 无效", i, s[1:])
		}
		st.Board[i] = piece(color, animal)
	}
	switch in.Turn {
	case "", "r", "R", "red":
		st.Turn = Red
	case "b", "B", "black":
		st.Turn = Black
	default:
		return nil, game.NewError(game.CodeInvalidPosition, "turn 必须是 r 或 b，实际 %q", in.Turn)
	}
	st.recompute()
	return st, nil
}

func (s *State) recompute() {
	s.Winner = 0
	s.WinReason = ""
	// If a den is occupied, that side has already won.
	for color := int8(Red); color <= Black; color++ {
		for idx, p := range s.Board {
			if p != 0 && colorOf(p) != color && idx == dens[color] {
				s.Winner = colorOf(p)
				s.WinReason = "den"
			}
		}
	}
	if s.Winner == 0 && len(genMoves(s)) == 0 {
		s.Winner = other(s.Turn)
		s.WinReason = "blocked"
	}
}

// Encode implements game.Game.
func (g *Game) Encode(pos game.Position, includeLegalMoves bool) game.PositionView {
	st := pos.(*State)
	board := make([]string, Size)
	for i, p := range st.Board {
		if p == 0 {
			board[i] = ""
			continue
		}
		board[i] = colorLetter(colorOf(p)) + animalLetter[animalOf(p)]
	}
	status := game.Status{State: "playing"}
	if st.Winner != 0 {
		reason := st.WinReason
		if reason == "" {
			reason = "den"
		}
		status = game.Status{State: "win", Winner: colorString(st.Winner), Reason: reason}
	}
	view := game.PositionView{
		Board:      board,
		Turn:       colorString(st.Turn),
		MoveNumber: st.Moves + 1,
		Status:     status,
	}
	if st.LastFrom >= 0 {
		m := moveView(st.LastFrom, st.LastTo, st.Board[st.LastTo])
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
	moves := genMoves(st)
	out := make([]game.Move, 0, len(moves))
	for _, m := range moves {
		out = append(out, moveView(m.from, m.to, st.Board[m.to]))
	}
	return out
}

type rawMove struct{ from, to int }

func genMoves(st *State) []rawMove {
	var moves []rawMove
	for from, p := range st.Board {
		if p == 0 || colorOf(p) != st.Turn {
			continue
		}
		row, col := from/Cols, from%Cols
		for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			r, c := row+d[0], col+d[1]
			if r < 0 || r >= Rows || c < 0 || c >= Cols {
				continue
			}
			to := r*Cols + c
			if canEnter(p, to) && canCapture(st.Board, from, p, to) {
				moves = append(moves, rawMove{from, to})
			}
		}
		if animalOf(p) == Lion || animalOf(p) == Tiger {
			moves = append(moves, jumpMoves(st.Board, from, p)...)
		}
	}
	return moves
}

func canEnter(p int8, to int) bool {
	if to == dens[colorOf(p)] {
		return false
	}
	if water[to] && animalOf(p) != Rat {
		return false
	}
	return true
}

func canCapture(board [Size]int8, from int, attacker int8, to int) bool {
	target := board[to]
	if target == 0 {
		return true
	}
	if colorOf(target) == colorOf(attacker) {
		return false
	}
	// A rat in the water and a piece on land cannot capture each other; only
	// rats in the water may fight there.
	if water[from] != water[to] && (animalOf(attacker) == Rat || animalOf(target) == Rat) {
		return false
	}
	if animalOf(attacker) == Rat && animalOf(target) == Elephant && !water[from] {
		return true
	}
	if animalOf(attacker) == Elephant && animalOf(target) == Rat {
		return false
	}
	return effectivePower(attacker, from) >= effectivePower(target, to)
}

func effectivePower(p int8, at int) int {
	if traps[other(colorOf(p))][at] {
		return 0
	}
	return power[animalOf(p)]
}

func jumpMoves(board [Size]int8, from int, p int8) []rawMove {
	var moves []rawMove
	row, col := from/Cols, from%Cols
	for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		r, c := row+d[0], col+d[1]
		crossed := false
		blocked := false
		for r >= 0 && r < Rows && c >= 0 && c < Cols && water[r*Cols+c] {
			crossed = true
			if board[r*Cols+c] != 0 {
				blocked = true
				break
			}
			r += d[0]
			c += d[1]
		}
		if !crossed || blocked {
			continue
		}
		if r < 0 || r >= Rows || c < 0 || c >= Cols {
			continue
		}
		to := r*Cols + c
		if canEnter(p, to) && canCapture(board, from, p, to) {
			moves = append(moves, rawMove{from, to})
		}
	}
	return moves
}

// Apply implements game.Game.
func (g *Game) Apply(pos game.Position, in game.MoveInput) (game.Position, game.Move, error) {
	st := pos.(*State)
	if st.Winner != 0 {
		return nil, game.Move{}, game.NewError(game.CodeGameOver, "对局已经结束")
	}
	if in.From == nil || in.To == nil {
		return nil, game.Move{}, game.NewError(game.CodeInvalidMove, "jungle 需要使用 from 与 to 指定起点和终点")
	}
	from, to := *in.From, *in.To
	if from < 0 || from >= Size || to < 0 || to >= Size {
		return nil, game.Move{}, game.NewError(game.CodeInvalidMove, "坐标超出棋盘")
	}
	p := st.Board[from]
	if p == 0 || colorOf(p) != st.Turn {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "起点没有轮到行动的一方棋子")
	}
	legal := false
	for _, m := range genMoves(st) {
		if m.from == from && m.to == to {
			legal = true
			break
		}
	}
	if !legal {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "该走法不合法")
	}
	captured := st.Board[to]
	next := *st
	next.Board[from] = 0
	next.Board[to] = p
	next.LastFrom, next.LastTo = from, to
	next.Moves++
	next.Winner = 0
	next.WinReason = ""
	next.Turn = other(st.Turn)
	if to == dens[other(colorOf(p))] {
		next.Winner = colorOf(p)
		next.WinReason = "den"
	} else if len(genMoves(&next)) == 0 {
		next.Winner = colorOf(p)
		next.WinReason = "blocked"
	}
	m := moveView(from, to, captured)
	return &next, m, nil
}

// Search implements game.Game.
func (g *Game) Search(ctx context.Context, pos game.Position, opts game.SearchOptions) (game.SearchResult, error) {
	st := pos.(*State)
	start := time.Now()
	result := game.SearchResult{Game: "jungle", Difficulty: game.NormalizeDifficulty(opts.Difficulty)}
	if st.Winner != 0 {
		result.GameOver = true
		result.Status = g.Encode(st, false).Status
		return result, nil
	}
	depth := game.DepthFor(opts.Difficulty, opts.MaxDepth, 9)
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

	moves := genMoves(st)
	if len(moves) == 0 {
		result.GameOver = true
		result.Status = g.Encode(st, false).Status
		return result, nil
	}
	allowed := make([]rawMove, 0, len(moves))
	for _, m := range moves {
		if !opts.Forbids(moveView(m.from, m.to, 0)) {
			allowed = append(allowed, m)
		}
	}
	moves = allowed
	if len(moves) == 0 {
		return game.SearchResult{}, game.NewError(game.CodeBadRequest, "forbiddenMoves 覆盖了全部合法着法，没有可走的棋")
	}
	orderMoves(st, moves)
	best := moves[0]
	bestScore := 0
	reached := 0
	for d := 1; d <= depth; d++ {
		if time.Now().After(deadline) || ctx.Err() != nil {
			timedOut = true
			break
		}
		alpha, beta := -mateScore*2, mateScore*2
		localBest := moves[0]
		localScore := 0
		complete := true
		for i, m := range moves {
			child := applyRaw(st, m)
			var score int
			if child.Winner != 0 {
				score = mateScore - 1
			} else {
				score = -negamax(child, d-1, -beta, -alpha, 1, &nodes, deadline, ctx, &timedOut)
			}
			if timedOut {
				complete = false
				break
			}
			if i == 0 || score > localScore {
				localBest = m
				localScore = score
			}
			if score > alpha {
				alpha = score
			}
		}
		if !complete {
			break
		}
		best = localBest
		bestScore = localScore
		reached = d
		bestMove := moveView(best.from, best.to, st.Board[best.to])
		opts.Report(game.SearchProgress{
			Depth: d, Score: bestScore, Nodes: nodes, TimeMs: time.Since(start).Milliseconds(),
			Best: &bestMove, PV: []game.Move{bestMove},
		})
		moves = rawToFront(moves, best)
		if bestScore >= mateScore-1000 {
			break
		}
	}
	next, move, _ := g.Apply(st, game.MoveInput{From: &best.from, To: &best.to})
	nv := g.Encode(next, false)
	result.Move = &move
	result.Status = nv.Status
	result.GameOver = nv.Status.Finished()
	result.Position = &nv
	result.Evaluation = game.Evaluation{Score: bestScore, Unit: "centipawn", Perspective: colorString(st.Turn)}
	if abs(bestScore) >= mateScore-1000 {
		result.Evaluation.MateIn = (mateScore - abs(bestScore) + 1) / 2
		if bestScore < 0 {
			result.Evaluation.MateIn = -result.Evaluation.MateIn
		}
	}
	result.Search = game.SearchInfo{Depth: reached, Nodes: nodes, TimeMs: time.Since(start).Milliseconds(), TimedOut: timedOut, PV: []game.Move{move}}
	return result, nil
}

func negamax(st *State, depth, alpha, beta, ply int, nodes *int64, deadline time.Time, ctx context.Context, timedOut *bool) int {
	*nodes++
	if *nodes&1023 == 0 && (time.Now().After(deadline) || ctx.Err() != nil) {
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
	orderMoves(st, moves)
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
	p := next.Board[m.from]
	captured := next.Board[m.to]
	next.Board[m.from] = 0
	next.Board[m.to] = p
	next.LastFrom, next.LastTo = m.from, m.to
	next.Moves++
	next.Winner = 0
	next.WinReason = ""
	next.Turn = other(st.Turn)
	if m.to == dens[other(colorOf(p))] {
		next.Winner = colorOf(p)
		next.WinReason = "den"
	} else if len(genMoves(&next)) == 0 {
		next.Winner = colorOf(p)
		next.WinReason = "blocked"
	}
	_ = captured
	return &next
}

func evaluate(st *State) int {
	score := 0
	for i, p := range st.Board {
		if p == 0 {
			continue
		}
		color := colorOf(p)
		v := value[animalOf(p)]
		// Advancement toward the enemy den.
		row := i / Cols
		if color == Red {
			v += (8 - row) * 12
		} else {
			v += row * 12
		}
		// Den proximity for the strongest pieces.
		denRow := 0
		if color == Red {
			denRow = 8
		}
		if animalOf(p) >= Lion {
			v += (7 - abs(row-denRow)) * 8
		}
		if water[i] && animalOf(p) == Rat {
			v += 40
		}
		if traps[other(color)][i] {
			v -= 60
		}
		if color == st.Turn {
			score += v
		} else {
			score -= v
		}
	}
	return score
}

func orderMoves(st *State, moves []rawMove) {
	sort.SliceStable(moves, func(i, j int) bool {
		return moveWeight(st, moves[i]) > moveWeight(st, moves[j])
	})
}

func moveWeight(st *State, m rawMove) int {
	w := 0
	target := st.Board[m.to]
	if target != 0 {
		w += value[animalOf(target)] * 2
	}
	if m.to == dens[other(st.Turn)] {
		w += mateScore
	}
	p := st.Board[m.from]
	if traps[other(st.Turn)][m.to] {
		w += 100
	}
	_ = p
	return w
}

func rawToFront(moves []rawMove, m rawMove) []rawMove {
	out := make([]rawMove, 0, len(moves))
	out = append(out, m)
	for _, v := range moves {
		if v != m {
			out = append(out, v)
		}
	}
	return out
}

func moveView(from, to int, captured int8) game.Move {
	f, t := from, to
	m := game.Move{
		From: &game.Coord{Index: f, Row: f / Cols, Col: f % Cols},
		To:   &game.Coord{Index: t, Row: t / Cols, Col: t % Cols},
	}
	if captured != 0 {
		m.Captured = colorLetter(colorOf(captured)) + animalLetter[animalOf(captured)]
	}
	return m
}

func coordMove(idx int) game.Move {
	i := idx
	return game.Move{Index: &game.Coord{Index: i, Row: i / Cols, Col: i % Cols}}
}

func colorLetter(c int8) string {
	if c == Black {
		return "b"
	}
	return "r"
}

func colorString(c int8) string { return colorLetter(c) }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
