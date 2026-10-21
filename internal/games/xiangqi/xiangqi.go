// Package xiangqi implements Chinese chess (Xiangqi) with full move legality,
// check detection, FEN/UCCI support and a negamax AI.
package xiangqi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
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
}

type move struct{ From, To int }

// Game implements game.Game.
type Game struct {
	native bool
}

// New returns a xiangqi engine backed by the built-in search.
func New() *Game { return &Game{} }

// WithNativeEngine switches this game to the pure-Go godogpaw engine.
func (g *Game) WithNativeEngine() *Game {
	g.native = true
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
		Features:       []string{"完整中国象棋规则", "蹩马腿 / 塞象眼", "炮架吃子", "将帅照面检测", "将杀与困毙", "godogpaw 引擎"},
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

	orderMoves(st, rootMoves)
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
		for i, m := range rootMoves {
			child := makeMove(st, m)
			score := -negamax(child, d-1, -beta, -alpha, 1, &nodes, deadline, ctx, &timedOut)
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

func negamax(st *State, depth, alpha, beta, ply int, nodes *int64, deadline time.Time, ctx context.Context, timedOut *bool) int {
	*nodes++
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
		return evaluate(st)
	}
	orderMoves(st, moves)
	best := -mateScore * 2
	for _, m := range moves {
		child := makeMove(st, m)
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
