// Package reversi implements Othello/Reversi on an 8x8 bitboard with a
// negamax AI.
package reversi

import (
	"context"
	"math/bits"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

const (
	Size = 64

	Black int8 = 1
	White int8 = 2

	mateScore = 1_000_000
)

const (
	fileA uint64 = 0x0101010101010101
	fileH uint64 = 0x8080808080808080
)

// State is an Othello position. Black and White are bitboards with bit 0 = a1.
type State struct {
	Black uint64
	White uint64
	Turn  int8
	Last  int
	Pass  bool
}

// Game implements game.Game.
type Game struct{}

// New returns a reversi engine.
func New() *Game { return &Game{} }

// Meta implements game.Game.
func (g *Game) Meta() game.Meta {
	return game.Meta{
		ID:          "reversi",
		Aliases:     []string{"othello", "black-white"},
		Name:        "黑白棋",
		EnglishName: "Reversi / Othello",
		Tagline:     "夹住对方棋子即可翻面，终局子多者胜。",
		Category:    "翻转类",
		Players:     2,
		Board: game.BoardMeta{
			Rows: 8, Cols: 8, Cells: 64, Type: "grid",
			Origin: "row 0 为最上行，index = row*8 + col，a1 = 0",
			Legend: map[string]string{"": "空位", "b": "黑方", "w": "白方"},
		},
		Colors:         []game.ColorMeta{{ID: "b", Name: "黑方"}, {ID: "w", Name: "白方"}},
		PositionFormat: `{"board":["","","", ...64格], "turn":"b"}`,
		MoveFormat:     `{"index": 落子格子下标（0-63）}`,
		Difficulties:   game.DifficultyOrder,
		Features:       []string{"完整翻转规则", "自动跳过无子可下的一方", "位棋盘", "Alpha-Beta 搜索"},
		Rules: []string{
			"黑方先行。落子后，在横、竖、斜任意方向夹住对方棋子，被夹住的棋子全部翻成己方颜色。",
			"必须至少翻转一枚对方棋子才能落子。",
			"一方无子可下时自动跳过，由另一方继续落子。",
			"双方都无子可下时结束，棋子多者获胜，相同为和棋。",
		},
	}
}

// Initial implements game.Game.
func (g *Game) Initial() game.Position {
	return &State{
		Black: (1 << 28) | (1 << 35),
		White: (1 << 27) | (1 << 36),
		Turn:  Black,
		Last:  -1,
	}
}

func (s *State) me() uint64 {
	if s.Turn == White {
		return s.White
	}
	return s.Black
}

func (s *State) opp() uint64 {
	if s.Turn == White {
		return s.Black
	}
	return s.White
}

func (s *State) legal() uint64 {
	return legalMoves(s.me(), s.opp())
}

func (s *State) legalFor(side int8) uint64 {
	if side == White {
		return legalMoves(s.White, s.Black)
	}
	return legalMoves(s.Black, s.White)
}

func (s *State) normalizeTurn() {
	if s.legal() == 0 && s.legalFor(other(s.Turn)) != 0 {
		s.Turn = other(s.Turn)
		s.Pass = true
	}
}

// Decode implements game.Game.
func (g *Game) Decode(in game.PositionInput) (game.Position, error) {
	if len(in.Board) != Size {
		return nil, game.NewError(game.CodeInvalidPosition, "reversi 棋盘必须包含 %d 个格子，实际 %d 个", Size, len(in.Board))
	}
	st := &State{Turn: Black, Last: -1}
	for i, cell := range in.Board {
		switch cell {
		case "", "0", ".", "-":
		case "b", "B", "black":
			st.Black |= 1 << uint(i)
		case "w", "W", "white":
			st.White |= 1 << uint(i)
		default:
			return nil, game.NewError(game.CodeInvalidPosition, "第 %d 格的值 %q 无效，只支持 b / w / 空", i, cell)
		}
	}
	switch in.Turn {
	case "", "b", "B", "black":
		st.Turn = Black
	case "w", "W", "white":
		st.Turn = White
	default:
		return nil, game.NewError(game.CodeInvalidPosition, "turn 必须是 b 或 w，实际 %q", in.Turn)
	}
	if in.Turn == "" && bits.OnesCount64(st.Black) > bits.OnesCount64(st.White) {
		st.Turn = White
	}
	st.normalizeTurn()
	return st, nil
}

// Encode implements game.Game.
func (g *Game) Encode(pos game.Position, includeLegalMoves bool) game.PositionView {
	st := pos.(*State)
	board := make([]string, Size)
	for i := 0; i < Size; i++ {
		bit := uint64(1) << uint(i)
		switch {
		case st.Black&bit != 0:
			board[i] = "b"
		case st.White&bit != 0:
			board[i] = "w"
		default:
			board[i] = ""
		}
	}
	status := game.Status{State: "playing"}
	if st.legal() == 0 && st.legalFor(other(st.Turn)) == 0 {
		b, w := bits.OnesCount64(st.Black), bits.OnesCount64(st.White)
		switch {
		case b > w:
			status = game.Status{State: "win", Winner: "b", Reason: "disc-count"}
		case w > b:
			status = game.Status{State: "win", Winner: "w", Reason: "disc-count"}
		default:
			status = game.Status{State: "draw", Reason: "disc-count"}
		}
	}
	view := game.PositionView{
		Board:      board,
		Turn:       turnString(st.Turn),
		MoveNumber: bits.OnesCount64(st.Black|st.White) - 3,
		Status:     status,
	}
	if view.MoveNumber < 1 {
		view.MoveNumber = 1
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
	mask := st.legal()
	out := make([]game.Move, 0, bits.OnesCount64(mask))
	for mask != 0 {
		i := bits.TrailingZeros64(mask)
		mask &= mask - 1
		out = append(out, coordMove(i))
	}
	return out
}

// Apply implements game.Game.
func (g *Game) Apply(pos game.Position, in game.MoveInput) (game.Position, game.Move, error) {
	st := pos.(*State)
	if in.Index == nil {
		return nil, game.Move{}, game.NewError(game.CodeInvalidMove, "reversi 需要使用 index 指定落子位置")
	}
	over := st.legal() == 0 && st.legalFor(other(st.Turn)) == 0
	if over {
		return nil, game.Move{}, game.NewError(game.CodeGameOver, "对局已经结束")
	}
	idx := *in.Index
	if idx < 0 || idx >= Size {
		return nil, game.Move{}, game.NewError(game.CodeInvalidMove, "落子位置 %d 超出棋盘", idx)
	}
	bit := uint64(1) << uint(idx)
	if (st.Black|st.White)&bit != 0 {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "位置 %s 已有棋子", notation(idx))
	}
	flipped := flips(st.me(), st.opp(), bit)
	if flipped == 0 {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "在 %s 落子不能翻转任何棋子", notation(idx))
	}
	next := *st
	next.Last = idx
	next.Pass = false
	if st.Turn == Black {
		next.Black |= bit | flipped
		next.White &^= flipped
	} else {
		next.White |= bit | flipped
		next.Black &^= flipped
	}
	next.Turn = other(st.Turn)
	next.normalizeTurn()
	m := coordMove(idx)
	m.Notation = notation(idx)
	m.Flipped = bits.OnesCount64(flipped)
	return &next, m, nil
}

// Search implements game.Game.
func (g *Game) Search(ctx context.Context, pos game.Position, opts game.SearchOptions) (game.SearchResult, error) {
	st := pos.(*State)
	start := time.Now()
	result := game.SearchResult{Game: "reversi", Difficulty: game.NormalizeDifficulty(opts.Difficulty)}
	if st.legal() == 0 {
		result.GameOver = true
		result.Status = g.Encode(st, false).Status
		return result, nil
	}
	depth := depthFor(opts)
	limit := game.TimeFor(opts.Difficulty, opts.TimeLimit)
	deadline := start.Add(limit)
	nodes := int64(0)
	timedOut := false

	moves := legalIndexes(st)
	order(st, moves)
	best := moves[0]
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
		for _, idx := range moves {
			child := applyBit(st, idx)
			score := -negamax(child.me(), child.opp(), d-1, -beta, -alpha, 1, &nodes, deadline, ctx, &timedOut)
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
		moves = moveToFront(moves, best)
		if abs(bestScore) >= mateScore-1000 {
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
	if abs(bestScore) >= mateScore-1000 {
		result.Evaluation.MateIn = (mateScore - abs(bestScore) + 1) / 2
		if bestScore < 0 {
			result.Evaluation.MateIn = -result.Evaluation.MateIn
		}
	}
	result.Search = game.SearchInfo{Depth: reached, Nodes: nodes, TimeMs: time.Since(start).Milliseconds(), TimedOut: timedOut, PV: []game.Move{move}}
	return result, nil
}

func depthFor(opts game.SearchOptions) int {
	if opts.MaxDepth > 0 {
		return game.DepthFor(opts.Difficulty, opts.MaxDepth, 12)
	}
	switch game.NormalizeDifficulty(opts.Difficulty) {
	case "easy":
		return 2
	case "normal":
		return 5
	case "hard":
		return 7
	case "master":
		return 9
	}
	return 5
}

func negamax(me, opp uint64, depth, alpha, beta, ply int, nodes *int64, deadline time.Time, ctx context.Context, timedOut *bool) int {
	*nodes++
	if *nodes&511 == 0 && (time.Now().After(deadline) || ctx.Err() != nil) {
		*timedOut = true
		return 0
	}
	moves := legalMoves(me, opp)
	if moves == 0 {
		if legalMoves(opp, me) != 0 {
			return negamax(opp, me, depth, alpha, beta, ply, nodes, deadline, ctx, timedOut)
		}
		return finalScore(me, opp)
	}
	if depth <= 0 {
		return evaluate(me, opp)
	}
	moveList := bitsToIndexes(moves)
	orderBits(opp, me, moveList)
	best := -mateScore * 2
	for _, idx := range moveList {
		flipped := flips(me, opp, uint64(1)<<uint(idx))
		newMe := opp &^ flipped
		newOpp := me | flipped | (uint64(1) << uint(idx))
		score := -negamax(newMe, newOpp, depth-1, -beta, -alpha, ply+1, nodes, deadline, ctx, timedOut)
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

func evaluate(me, opp uint64) int {
	discDiff := bits.OnesCount64(me) - bits.OnesCount64(opp)
	score := discDiff * 12
	for i := 0; i < Size; i++ {
		bit := uint64(1) << uint(i)
		if me&bit != 0 {
			score += positional[i]
		} else if opp&bit != 0 {
			score -= positional[i]
		}
	}
	mobility := bits.OnesCount64(legalMoves(me, opp)) - bits.OnesCount64(legalMoves(opp, me))
	score += mobility * 10
	if bits.OnesCount64(me|opp) > 48 {
		score += discDiff * 20
	}
	return score
}

func finalScore(me, opp uint64) int {
	diff := bits.OnesCount64(me) - bits.OnesCount64(opp)
	if diff > 0 {
		return mateScore - 64 + diff
	}
	if diff < 0 {
		return -mateScore + 64 + diff
	}
	return 0
}

func applyBit(st *State, idx int) *State {
	flipped := flips(st.me(), st.opp(), uint64(1)<<uint(idx))
	next := *st
	next.Last = idx
	next.Pass = false
	if st.Turn == Black {
		next.Black |= (uint64(1) << uint(idx)) | flipped
		next.White &^= flipped
	} else {
		next.White |= (uint64(1) << uint(idx)) | flipped
		next.Black &^= flipped
	}
	next.Turn = other(st.Turn)
	next.normalizeTurn()
	return &next
}

type direction struct {
	shift int
	mask  uint64
}

var directions = []direction{
	{1, ^fileH},
	{-1, ^fileA},
	{8, ^uint64(0)},
	{-8, ^uint64(0)},
	{9, ^fileH},
	{-9, ^fileA},
	{7, ^fileA},
	{-7, ^fileH},
}

func shift(bits uint64, d direction) uint64 {
	masked := bits & d.mask
	if d.shift > 0 {
		return masked << uint(d.shift)
	}
	return masked >> uint(-d.shift)
}

func legalMoves(me, opp uint64) uint64 {
	empty := ^(me | opp)
	var moves uint64
	for _, d := range directions {
		x := shift(me, d) & opp
		x |= shift(x, d) & opp
		x |= shift(x, d) & opp
		x |= shift(x, d) & opp
		x |= shift(x, d) & opp
		x |= shift(x, d) & opp
		moves |= shift(x, d) & empty
	}
	return moves
}

func flips(me, opp, move uint64) uint64 {
	var all uint64
	for _, d := range directions {
		var captured uint64
		x := shift(move, d)
		for x != 0 && x&opp != 0 {
			captured |= x
			x = shift(x, d)
		}
		if x != 0 && x&me != 0 {
			all |= captured
		}
	}
	return all
}

func bitsToIndexes(mask uint64) []int {
	out := make([]int, 0, bits.OnesCount64(mask))
	for mask != 0 {
		i := bits.TrailingZeros64(mask)
		mask &= mask - 1
		out = append(out, i)
	}
	return out
}

func legalIndexes(st *State) []int {
	return bitsToIndexes(st.legal())
}

func order(st *State, moves []int) {
	for i := 1; i < len(moves); i++ {
		for j := i; j > 0 && moveWeight(st, moves[j]) > moveWeight(st, moves[j-1]); j-- {
			moves[j], moves[j-1] = moves[j-1], moves[j]
		}
	}
}

func orderBits(opp, me uint64, moves []int) {
	for i := 1; i < len(moves); i++ {
		for j := i; j > 0 && squareWeight(moves[j]) > squareWeight(moves[j-1]); j-- {
			moves[j], moves[j-1] = moves[j-1], moves[j]
		}
	}
}

func squareWeight(idx int) int {
	row, col := idx/8, idx%8
	if (row == 0 || row == 7) && (col == 0 || col == 7) {
		return 100
	}
	if (row == 0 || row == 7) || (col == 0 || col == 7) {
		return 20
	}
	if (row == 1 || row == 6) && (col == 1 || col == 6) {
		return -60
	}
	if (row == 1 || row == 6) || (col == 1 || col == 6) {
		return -15
	}
	return 5
}

func moveWeight(st *State, idx int) int {
	w := squareWeight(idx)
	flipped := flips(st.me(), st.opp(), uint64(1)<<uint(idx))
	w += bits.OnesCount64(flipped) * 2
	return w
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
	return game.Move{Index: &game.Coord{Index: idxCopy, Row: idx / 8, Col: idx % 8}}
}

func notation(idx int) string {
	return string([]byte{byte('a' + idx%8), byte('1' + idx/8)})
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

var positional = [Size]int{
	100, -20, 10, 5, 5, 10, -20, 100,
	-20, -50, -2, -2, -2, -2, -50, -20,
	10, -2, 1, 1, 1, 1, -2, 10,
	5, -2, 1, 0, 0, 1, -2, 5,
	5, -2, 1, 0, 0, 1, -2, 5,
	10, -2, 1, 1, 1, 1, -2, 10,
	-20, -50, -2, -2, -2, -2, -50, -20,
	100, -20, 10, 5, 5, 10, -20, 100,
}
