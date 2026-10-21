// Package xiangqi implements Chinese chess (Xiangqi) with full move legality,
// check detection, FEN/UCCI support and a negamax AI.
package xiangqi

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
	"github.com/chuccp/auto-chess/internal/games/xiangqi/nnue"
)

const (
	Files = 9
	Ranks = 10
	Size  = Files * Ranks

	Red   int8 = 1
	Black int8 = -1

	King    int8 = 1
	Advisor int8 = 2
	Bishop  int8 = 3
	Knight  int8 = 4
	Rook    int8 = 5
	Cannon  int8 = 6
	Pawn    int8 = 7

	mateScore = 100_000
)

var letterPiece = map[byte]int8{
	'k': King, 'a': Advisor, 'b': Bishop, 'n': Knight, 'r': Rook, 'c': Cannon, 'p': Pawn,
}

// State is a xiangqi position. Square index = rank*9 + file, rank 0 is Red's
// baseline and file 0 is the a-file.
type State struct {
	Board    [Size]int8
	Turn     int8
	Halfmove int
	Fullmove int
	LastFrom int
	LastTo   int
	hash     uint64
}

type move struct{ From, To int }

// Game implements game.Game.
type Game struct {
	native  bool
	nnue    *nnue.Network
	workers int
}

// New returns a xiangqi engine backed by the built-in search.
func New() *Game { return &Game{workers: 1} }

// NewWithNNUE loads a Pikafish 2024 .nnue file and uses it for leaf evaluation.
func NewWithNNUE(path string) (*Game, error) {
	net, err := nnue.Load(path)
	if err != nil {
		return nil, err
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	g := &Game{nnue: net, workers: workers}
	return g, nil
}

// WithNativeEngine switches this game to the pure-Go godogpaw engine.
func (g *Game) WithNativeEngine() *Game {
	g.native = true
	g.nnue = nil
	return g
}

// Meta implements game.Game.
func (g *Game) Meta() game.Meta {
	return game.Meta{
		ID:          "xiangqi",
		Aliases:     []string{"chinese-chess", "xiang-qi"},
		Name:        "中国象棋",
		EnglishName: "Xiangqi",
		Tagline:     "楚河汉界，车马炮全规则，将帅不可照面。",
		Category:    "经典棋类",
		Players:     2,
		Board: game.BoardMeta{
			Rows: Ranks, Cols: Files, Cells: Size, Type: "fen",
			Origin: "内部 index = rank*9 + file；FEN 按标准从上到下；UCCI 坐标从红方底线起算 a0-i9",
			Legend: map[string]string{
				"K": "帅", "A": "仕", "B": "相", "N": "马", "R": "车", "C": "炮", "P": "兵",
				"k": "将", "a": "士", "b": "象", "n": "馬", "r": "車", "c": "砲", "p": "卒",
			},
		},
		Colors:         []game.ColorMeta{{ID: "w", Name: "红方"}, {ID: "b", Name: "黑方"}},
		PositionFormat: "标准象棋 FEN，例如 rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
		MoveFormat:     "UCCI，例如 h2e2、b0c2",
		Difficulties:   game.DifficultyOrder,
		Features:       []string{"完整中国象棋规则", "蹩马腿 / 塞象眼", "炮架吃子", "将帅照面检测", "将杀与困毙", "godogpaw / NNUE 引擎"},
		Rules: []string{
			"红方先行，目标是将死对方将帅。",
			"帅/将只能在九宫内横竖走一格；士/仕在九宫内斜走一格。",
			"相/象走田字，不能过河；马走日字，蹩马腿时不能走。",
			"车走直线；炮走直线，但吃子必须隔一个炮架。",
			"兵/卒过河前只能向前，过河后可左右；不能后退。",
			"双方将帅不能在同一直线上无子相隔（照面）。",
			"无合法走法时，被将军为将杀，否则为困毙，均判负。",
		},
	}
}

// Initial implements game.Game.
func (g *Game) Initial() game.Position {
	st, _ := parseFEN("rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1")
	return st
}

// Decode implements game.Game.
func (g *Game) Decode(in game.PositionInput) (game.Position, error) {
	if in.FEN == "" {
		return nil, game.NewError(game.CodeInvalidPosition, "xiangqi 需要提供 position.fen")
	}
	st, err := parseFEN(in.FEN)
	if err != nil {
		return nil, err
	}
	st.LastFrom, st.LastTo = -1, -1
	return st, nil
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
		letter := pieceLetter(absPiece(p))
		if p > 0 {
			letter = strings.ToUpper(letter)
		}
		board[i] = colorLetter(colorOf(p)) + letter
	}
	status := g.status(st)
	view := game.PositionView{
		FEN:        st.fen(),
		Board:      board,
		Turn:       colorString(st.Turn),
		MoveNumber: st.Fullmove,
		Status:     status,
	}
	if st.LastFrom >= 0 {
		m := g.moveView(move{From: st.LastFrom, To: st.LastTo})
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
	ms := legalMoves(st)
	out := make([]game.Move, 0, len(ms))
	for _, m := range ms {
		out = append(out, g.moveView(m))
	}
	return out
}

// Apply implements game.Game.
func (g *Game) Apply(pos game.Position, in game.MoveInput) (game.Position, game.Move, error) {
	st := pos.(*State)
	if g.status(st).Finished() {
		return nil, game.Move{}, game.NewError(game.CodeGameOver, "对局已经结束")
	}
	m, err := parseMoveInput(in)
	if err != nil {
		return nil, game.Move{}, err
	}
	legal := false
	for _, candidate := range legalMoves(st) {
		if candidate.From == m.From && candidate.To == m.To {
			legal = true
			break
		}
	}
	if !legal {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "走法 %s 不合法", m.uci())
	}
	next := makeMove(st, m)
	next.LastFrom, next.LastTo = m.From, m.To
	mv := g.moveView(m)
	mv.Check = inCheck(next, next.Turn)
	return next, mv, nil
}

// Search implements game.Game.
func (g *Game) Search(ctx context.Context, pos game.Position, opts game.SearchOptions) (game.SearchResult, error) {
	if g.native {
		st := pos.(*State)
		if status := g.status(st); status.Finished() {
			return game.SearchResult{
				Game:       "xiangqi",
				Difficulty: game.NormalizeDifficulty(opts.Difficulty),
				Engine:     "godogpaw",
				GameOver:   true,
				Status:     status,
			}, nil
		}
		return g.searchNative(ctx, pos, opts)
	}
	return g.searchBuiltin(ctx, pos, opts)
}

func (g *Game) searchBuiltin(ctx context.Context, pos game.Position, opts game.SearchOptions) (game.SearchResult, error) {
	st := pos.(*State)
	start := time.Now()
	result := game.SearchResult{Game: "xiangqi", Difficulty: game.NormalizeDifficulty(opts.Difficulty)}
	if g.nnue != nil {
		result.Engine = "pikafish-nnue"
	}
	rootMoves := legalMoves(st)
	if len(rootMoves) == 0 {
		result.GameOver = true
		result.Status = g.status(st)
		return result, nil
	}
	depth := game.DepthFor(opts.Difficulty, opts.MaxDepth, 7)
	if opts.MaxDepth == 0 {
		switch game.NormalizeDifficulty(opts.Difficulty) {
		case "easy":
			depth = 2
		case "normal":
			depth = 4
		case "hard":
			depth = 5
		case "master":
			depth = 6
		}
	}
	limit := game.TimeFor(opts.Difficulty, opts.TimeLimit)
	deadline := start.Add(limit)
	nodes := int64(0)
	timedOut := false
	var rootAcc *nnue.Accumulator
	if g.nnue != nil {
		rootAcc, _ = g.nnue.NewAccumulator(nnuePieces(st.Board))
	}
	evalCache := make(map[uint64]int)
	tables := &searchTables{eval: evalCache, tt: make(map[uint64]ttEntry)}
	orderMoves(st, rootMoves, &tables.history)
	best := rootMoves[0]
	bestScore := 0
	reached := 0
	for d := 1; d <= depth; d++ {
		if time.Now().After(deadline) || ctx.Err() != nil {
			timedOut = true
			break
		}
		alpha, beta := -mateScore*2, mateScore*2
		localBest := rootMoves[0]
		localScore := 0
		complete := true
		if g.workers > 1 && d >= 3 && len(rootMoves) > 1 {
			var pn int64
			localBest, localScore, pn, complete = g.searchRootSplit(rootMoves, st, rootAcc, d, deadline, ctx)
			nodes += pn
			if !complete {
				timedOut = true
				break
			}
		} else {
			for i, m := range rootMoves {
				child := makeMove(st, m)
				childAcc := nextAccumulator(g.nnue, rootAcc, st.Board, m)
				score := -g.negamax(child, d-1, -beta, -alpha, 1, &nodes, deadline, ctx, &timedOut, childAcc, tables)
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
		}
		if !complete {
			break
		}
		best = localBest
		bestScore = localScore
		reached = d
		bestView := g.moveView(best)
		opts.Report(game.SearchProgress{
			Depth:  d,
			Score:  bestScore,
			Nodes:  nodes,
			TimeMs: time.Since(start).Milliseconds(),
			Best:   &bestView,
			PV:     []game.Move{bestView},
		})
		rootMoves = moveToFront(rootMoves, best)
		if abs(bestScore) >= mateScore-1000 {
			break
		}
	}
	next := makeMove(st, best)
	next.LastFrom, next.LastTo = best.From, best.To
	mv := g.moveView(best)
	mv.Check = inCheck(next, next.Turn)
	nv := g.Encode(next, false)
	result.Move = &mv
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
	result.Search = game.SearchInfo{Depth: reached, Nodes: nodes, TimeMs: time.Since(start).Milliseconds(), TimedOut: timedOut, PV: []game.Move{mv}}
	return result, nil
}

func (g *Game) searchRootSplit(rootMoves []move, st *State, rootAcc *nnue.Accumulator, depth int, deadline time.Time, ctx context.Context) (move, int, int64, bool) {
	first := rootMoves[0]
	firstChild := makeMove(st, first)
	firstAcc := nextAccumulator(g.nnue, rootAcc, st.Board, first)
	firstTables := &searchTables{eval: make(map[uint64]int), tt: make(map[uint64]ttEntry)}
	var nodes int64
	var timed bool
	bestScore := -g.negamax(firstChild, depth-1, -mateScore*2, mateScore*2, 1, &nodes, deadline, ctx, &timed, firstAcc, firstTables)
	if timed || time.Now().After(deadline) || ctx.Err() != nil {
		return first, bestScore, nodes, false
	}
	best := first
	alpha := bestScore

	type result struct {
		move  move
		score int
		nodes int64
		done  bool
	}
	results := make([]result, len(rootMoves))
	results[0] = result{move: first, score: bestScore, nodes: nodes, done: true}
	jobs := make(chan int)
	workers := g.workers
	if workers > len(rootMoves)-1 {
		workers = len(rootMoves) - 1
	}
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tables := &searchTables{eval: make(map[uint64]int), tt: make(map[uint64]ttEntry)}
			for idx := range jobs {
				r := result{move: rootMoves[idx], done: true}
				child := makeMove(st, rootMoves[idx])
				childAcc := nextAccumulator(g.nnue, rootAcc, st.Board, rootMoves[idx])
				var localTimed bool
				if time.Now().After(deadline) || ctx.Err() != nil {
					r.done = false
				} else {
					r.score = -g.negamax(child, depth-1, -mateScore*2, -alpha, 1, &r.nodes, deadline, ctx, &localTimed, childAcc, tables)
					if localTimed {
						r.done = false
					}
				}
				results[idx] = r
			}
		}()
	}
	for i := 1; i < len(rootMoves); i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	complete := true
	for i := 1; i < len(results); i++ {
		r := results[i]
		nodes += r.nodes
		if !r.done {
			complete = false
			continue
		}
		if r.score > bestScore {
			best = r.move
			bestScore = r.score
		}
	}
	if !complete {
		return best, bestScore, nodes, false
	}
	return best, bestScore, nodes, true
}

func (g *Game) negamax(st *State, depth, alpha, beta, ply int, nodes *int64, deadline time.Time, ctx context.Context, timedOut *bool, acc *nnue.Accumulator, tables *searchTables) int {
	*nodes++
	key := stateKey(st)
	alpha0 := alpha
	if score, ok := tables.probe(key, depth, alpha, beta); ok {
		return score
	}
	if *nodes&1023 == 0 && (time.Now().After(deadline) || ctx.Err() != nil) {
		*timedOut = true
		return 0
	}
	if st.Halfmove >= 120 {
		return 0
	}
	moves := legalMoves(st)
	if len(moves) == 0 {
		return -mateScore + ply
	}
	if depth <= 0 {
		return g.quiesce(st, alpha, beta, ply, nodes, deadline, ctx, timedOut, acc, tables)
	}
	if entry, ok := tables.tt[key]; ok && (entry.best != move{}) {
		moves = moveToFront(moves, entry.best)
	}
	orderMoves(st, moves, &tables.history)
	if depth >= 3 && !inCheck(st, st.Turn) && hasNonPawnMaterial(st, st.Turn) {
		nullState := makeNull(st)
		score := -g.negamax(nullState, depth-3, -beta, -beta+1, ply+1, nodes, deadline, ctx, timedOut, acc, tables)
		if *timedOut {
			return 0
		}
		if score >= beta {
			return beta
		}
	}
	best := -mateScore * 2
	bestMove := move{}
	for i, m := range moves {
		child := makeMove(st, m)
		childAcc := nextAccumulator(g.nnue, acc, st.Board, m)
		reduction := 0
		if depth >= 3 && i >= 3 && st.Board[m.To] == 0 {
			reduction = 1
			if depth >= 5 && i >= 6 {
				reduction = 2
			}
		}
		score := -g.negamax(child, depth-1-reduction, -beta, -alpha, ply+1, nodes, deadline, ctx, timedOut, childAcc, tables)
		if reduction > 0 && score > alpha && !*timedOut {
			score = -g.negamax(child, depth-1, -beta, -alpha, ply+1, nodes, deadline, ctx, timedOut, childAcc, tables)
		}
		if score > best {
			best = score
			bestMove = m
		}
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			if st.Board[m.To] == 0 {
				side := 0
				if st.Turn == Black {
					side = 1
				}
				tables.history[side][m.From][m.To] += int32(depth * depth)
			}
			break
		}
		if *timedOut {
			break
		}
	}
	if !*timedOut {
		tables.store(key, depth, best, alpha0, beta, bestMove)
	}
	return best
}

func (g *Game) quiesce(st *State, alpha, beta, ply int, nodes *int64, deadline time.Time, ctx context.Context, timedOut *bool, acc *nnue.Accumulator, tables *searchTables) int {
	*nodes++
	if *nodes&1023 == 0 && (time.Now().After(deadline) || ctx.Err() != nil) {
		*timedOut = true
		return 0
	}
	if st.Halfmove >= 120 || ply >= 24 {
		return g.evaluateLeaf(st, acc, tables)
	}
	check := inCheck(st, st.Turn)
	stand := 0
	var moves []move
	if check {
		moves = legalMoves(st)
		if len(moves) == 0 {
			return -mateScore + ply
		}
	} else {
		stand = g.evaluateLeaf(st, acc, tables)
		if stand >= beta {
			return stand
		}
		if stand > alpha {
			alpha = stand
		}
		moves = legalCaptures(st)
		if len(moves) == 0 {
			return alpha
		}
	}
	orderMoves(st, moves, &tables.history)
	best := -mateScore * 2
	for _, m := range moves {
		if !check && st.Board[m.To] != 0 && stand+pieceValue[absPiece(st.Board[m.To])]+100 <= alpha {
			continue
		}
		child := makeMove(st, m)
		childAcc := nextAccumulator(g.nnue, acc, st.Board, m)
		score := -g.quiesce(child, -beta, -alpha, ply+1, nodes, deadline, ctx, timedOut, childAcc, tables)
		if *timedOut {
			break
		}
		if score > best {
			best = score
		}
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			if st.Board[m.To] == 0 {
				side := 0
				if st.Turn == Black {
					side = 1
				}
				tables.history[side][m.From][m.To]++
			}
			break
		}
	}
	if best == -mateScore*2 {
		return alpha
	}
	return best
}

type ttEntry struct {
	depth int
	score int
	flag  uint8 // 0 exact, 1 lower, 2 upper
	best  move
}

type searchTables struct {
	eval    map[uint64]int
	tt      map[uint64]ttEntry
	history [2][Size][Size]int32
}

func (t *searchTables) probe(key uint64, depth, alpha, beta int) (int, bool) {
	if t == nil {
		return 0, false
	}
	entry, ok := t.tt[key]
	if !ok || entry.depth < depth {
		return 0, false
	}
	switch entry.flag {
	case 0:
		return entry.score, true
	case 1:
		if entry.score >= beta {
			return entry.score, true
		}
	case 2:
		if entry.score <= alpha {
			return entry.score, true
		}
	}
	return 0, false
}

func (t *searchTables) store(key uint64, depth, score, alpha, beta int, best move) {
	if t == nil {
		return
	}
	flag := uint8(0)
	if score <= alpha {
		flag = 2
	} else if score >= beta {
		flag = 1
	}
	t.tt[key] = ttEntry{depth: depth, score: score, flag: flag, best: best}
}

func (g *Game) evaluateLeaf(st *State, acc *nnue.Accumulator, tables *searchTables) int {
	if g.nnue == nil || acc == nil {
		return evaluate(st)
	}
	var key uint64
	if tables != nil {
		key = stateKey(st)
		if score, ok := tables.eval[key]; ok {
			return score
		}
	}
	score, err := g.nnue.EvaluateAccumulator(acc, colorString(st.Turn))
	if err != nil {
		return evaluate(st)
	}
	if tables != nil {
		tables.eval[key] = score
	}
	return score
}

func stateKey(st *State) uint64 {
	if st.hash != 0 {
		return st.hash
	}
	return calcHash(st.Board, st.Turn)
}

func nnueCode(p int8) int {
	base := 8
	if p > 0 {
		base = 0
	}
	var code int
	switch absPiece(p) {
	case Rook:
		code = 1
	case Advisor:
		code = 2
	case Cannon:
		code = 3
	case Pawn:
		code = 4
	case Knight:
		code = 5
	case Bishop:
		code = 6
	case King:
		code = 7
	}
	return base + code
}

func nnuePieces(board [Size]int8) []int {
	out := make([]int, Size)
	for i, p := range board {
		if p != 0 {
			out[i] = nnueCode(p)
		}
	}
	return out
}

func nextAccumulator(n *nnue.Network, acc *nnue.Accumulator, board [Size]int8, m move) *nnue.Accumulator {
	if n == nil || acc == nil {
		return nil
	}
	return n.UpdateMove(acc, board, m.From, m.To)
}

func evaluate(st *State) int {
	score := 0
	for idx, p := range st.Board {
		if p == 0 {
			continue
		}
		piece := absPiece(p)
		value := pieceValue[piece]
		mirror := idx
		if p < 0 {
			mirror = (9-idx/9)*9 + (8 - idx%9)
		}
		value += evalTable(piece)[mirror]
		if p > 0 {
			score += value
		} else {
			score -= value
		}
	}
	if st.Turn == Black {
		return -score
	}
	return score
}

func (g *Game) status(st *State) game.Status {
	ms := legalMoves(st)
	check := inCheck(st, st.Turn)
	if len(ms) == 0 {
		if check {
			return game.Status{State: "checkmate", Winner: colorString(-st.Turn), Reason: "checkmate", InCheck: true}
		}
		return game.Status{State: "stalemate", Winner: colorString(-st.Turn), Reason: "stalemate"}
	}
	if st.Halfmove >= 120 {
		return game.Status{State: "draw", Reason: "sixty-move"}
	}
	if check {
		return game.Status{State: "check", InCheck: true}
	}
	return game.Status{State: "playing"}
}

func (g *Game) moveView(m move) game.Move {
	from, to := m.From, m.To
	return game.Move{
		UCI:  m.uci(),
		From: &game.Coord{Index: from, Row: from / Files, Col: from % Files},
		To:   &game.Coord{Index: to, Row: to / Files, Col: to % Files},
	}
}

func parseMoveInput(in game.MoveInput) (move, error) {
	if in.UCI != "" {
		return parseUCCI(in.UCI)
	}
	if in.From == nil || in.To == nil {
		return move{}, game.NewError(game.CodeInvalidMove, "xiangqi 需要 uci 或 from/to 坐标")
	}
	from, to := *in.From, *in.To
	if from < 0 || from >= Size || to < 0 || to >= Size {
		return move{}, game.NewError(game.CodeInvalidMove, "棋盘坐标超出范围")
	}
	return move{From: from, To: to}, nil
}

func parseUCCI(s string) (move, error) {
	if len(s) != 4 || s[0] < 'a' || s[0] > 'i' || s[2] < 'a' || s[2] > 'i' ||
		s[1] < '0' || s[1] > '9' || s[3] < '0' || s[3] > '9' {
		return move{}, game.NewError(game.CodeInvalidMove, "UCCI 走法必须形如 a0a1：%q", s)
	}
	from := int(s[1]-'0')*Files + int(s[0]-'a')
	to := int(s[3]-'0')*Files + int(s[2]-'a')
	return move{From: from, To: to}, nil
}

func (m move) uci() string {
	return fmt.Sprintf("%c%d%c%d", 'a'+m.From%Files, m.From/Files, 'a'+m.To%Files, m.To/Files)
}

func (m move) String() string { return m.uci() }

func colorOf(p int8) int8 {
	if p > 0 {
		return Red
	}
	return Black
}

func colorLetter(c int8) string {
	if c == Black {
		return "b"
	}
	return "r"
}

func colorString(c int8) string {
	if c == Black {
		return "b"
	}
	return "w"
}

func absPiece(p int8) int8 {
	if p < 0 {
		return -p
	}
	return p
}

func other(c int8) int8 { return -c }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func pieceLetter(p int8) string {
	switch p {
	case King:
		return "k"
	case Advisor:
		return "a"
	case Bishop:
		return "b"
	case Knight:
		return "n"
	case Rook:
		return "r"
	case Cannon:
		return "c"
	case Pawn:
		return "p"
	}
	return "?"
}

var pieceValue = [8]int{0, 0, 200, 200, 400, 900, 450, 100}

func evalTable(piece int8) *[Size]int {
	switch piece {
	case Pawn:
		return &pawnEval
	case Knight:
		return &knightEval
	case Cannon:
		return &cannonEval
	case Rook:
		return &rookEval
	case Advisor:
		return &advisorEval
	case Bishop:
		return &bishopEval
	case King:
		return &kingEval
	}
	return &pawnEval
}

func buildTables() {
	for rank := 0; rank < Ranks; rank++ {
		for file := 0; file < Files; file++ {
			idx := rank*Files + file
			center := 4 - abs(file-4)
			if rank >= 5 {
				pawnEval[idx] += 25 + (rank-5)*12
				if file >= 3 && file <= 5 {
					pawnEval[idx] += 10
				}
			}
			if rank >= 7 && file >= 3 && file <= 5 {
				pawnEval[idx] += 20
			}
			knightEval[idx] = center*8 + rank*2
			cannonEval[idx] = center*6 + min(rank, 9-rank)*2
			rookEval[idx] = center*4 + rank*2
			if rank == 0 || rank == 9 {
				rookEval[idx] += 10
			}
			if rank <= 2 || rank >= 7 {
				advisorEval[idx] = 8
				bishopEval[idx] = 6
				kingEval[idx] = 10
			}
			if file < 3 || file > 5 {
				kingEval[idx] = -40
			}
		}
	}
}

var (
	pawnEval    [Size]int
	knightEval  [Size]int
	cannonEval  [Size]int
	rookEval    [Size]int
	advisorEval [Size]int
	bishopEval  [Size]int
	kingEval    [Size]int
)

func init() { buildTables() }
