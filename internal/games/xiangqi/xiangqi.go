// Package xiangqi implements Chinese chess (Xiangqi) with full move legality,
// check detection, FEN/UCCI support and a negamax AI.
package xiangqi

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"

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
	accPool sync.Pool
}

// New returns a xiangqi engine backed by the built-in search.
func New() *Game {
	g := &Game{workers: 1}
	g.accPool.New = func() any { return new(nnue.Accumulator) }
	return g
}

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
	g.accPool.New = func() any { return new(nnue.Accumulator) }
	return g, nil
}

// WithNativeEngine switches this game to the pure-Go godogpaw engine.
func (g *Game) WithNativeEngine() *Game {
	g.native = true
	g.nnue = nil
	return g
}

// WithNNUE loads a Pikafish 2024 .nnue network onto an existing game so callers
// can pick between godogpaw, the NNUE search and the builtin evaluator per
// request via SearchOptions.Engine.
func (g *Game) WithNNUE(path string) error {
	net, err := nnue.Load(path)
	if err != nil {
		return err
	}
	g.nnue = net
	if workers := runtime.GOMAXPROCS(0); workers > g.workers {
		g.workers = workers
	}
	return nil
}

// Engines lists the search backends Search accepts, default first. godogpaw is
// always available (compiled in); pikafish-nnue appears only when the network
// is loaded.
func (g *Game) Engines() []string {
	switch {
	case g.nnue != nil:
		return []string{"pikafish-nnue", "godogpaw", "builtin"}
	case g.native:
		return []string{"godogpaw", "builtin"}
	default:
		return []string{"builtin", "godogpaw"}
	}
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
	legal := legalMoves(st)
	status := g.statusFrom(st, legal)
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
		view.LegalMoves = g.moveViews(legal)
		view.LegalMoveCount = len(view.LegalMoves)
	}
	return view
}

// LegalMoves implements game.Game.
func (g *Game) LegalMoves(pos game.Position) []game.Move {
	return g.moveViews(legalMoves(pos.(*State)))
}

func (g *Game) moveViews(ms []move) []game.Move {
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

// Search implements game.Game. SearchOptions.Engine selects the backend:
// "godogpaw", "pikafish-nnue" or "builtin". Empty (or "auto") picks the
// default: pikafish-nnue when the network is loaded, godogpaw otherwise.
func (g *Game) Search(ctx context.Context, pos game.Position, opts game.SearchOptions) (game.SearchResult, error) {
	engine := strings.ToLower(strings.TrimSpace(opts.Engine))
	if engine == "" || engine == "auto" {
		switch {
		case g.nnue != nil:
			engine = "pikafish-nnue"
		case g.native:
			engine = "godogpaw"
		default:
			engine = "builtin"
		}
	}
	switch engine {
	case "godogpaw":
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
	case "pikafish-nnue":
		if g.nnue == nil {
			return game.SearchResult{}, game.NewError(game.CodeUnknownEngine,
				"引擎 pikafish-nnue 不可用：NNUE 网络未加载（请配置 engine.xiangqi.nnue 或 XIANGQI_NNUE）")
		}
		return g.searchPosition(ctx, pos, opts, true)
	case "builtin":
		return g.searchPosition(ctx, pos, opts, false)
	default:
		return game.SearchResult{}, game.NewError(game.CodeUnknownEngine,
			"xiangqi 不支持引擎 %q，可选：%s", opts.Engine, strings.Join(g.Engines(), ", "))
	}
}

type ttEntry struct {
	depth int
	score int
	flag  uint8 // 0 exact, 1 lower, 2 upper
	best  move
}

// maxPly bounds the per-ply tables (killers and move scratch buffers). The
// search never gets close to it: the API caps maxDepth at 10 and quiescence
// stops at ply 24.
const maxPly = 64

type searchTables struct {
	eval    map[uint64]int
	tt      map[uint64]ttEntry
	history [2][Size][Size]int32
	killers [maxPly][2]move
	// moves holds one reusable move buffer per ply. Move generation is the
	// hottest allocation in the search; reusing the buffers removes tens of
	// megabytes of garbage per request.
	moves [maxPly][]move
}

// scratch returns an empty move buffer for ply, reusing a previous allocation
// when possible. The caller stores the (possibly grown) result back with
// keepScratch.
func (t *searchTables) scratch(ply int) []move {
	if ply < 0 || ply >= len(t.moves) {
		return nil
	}
	return t.moves[ply][:0]
}

func (t *searchTables) keepScratch(ply int, moves []move) {
	if ply < 0 || ply >= len(t.moves) {
		return
	}
	t.moves[ply] = moves
}

// ttScore converts a search score into its ply-independent table form so a
// mate found at one depth still reports the right distance when the position is
// reached through a transposition at another depth.
func ttScore(score, ply int) int {
	if score > mateScore-maxPly {
		return score + ply
	}
	if score < -mateScore+maxPly {
		return score - ply
	}
	return score
}

// ttScoreFromPly is the inverse of ttScore.
func ttScoreFromPly(score, ply int) int {
	if score > mateScore-maxPly {
		return score - ply
	}
	if score < -mateScore+maxPly {
		return score + ply
	}
	return score
}

func (t *searchTables) probe(key uint64, depth, alpha, beta, ply int) (int, bool) {
	if t == nil {
		return 0, false
	}
	entry, ok := t.tt[key]
	if !ok || entry.depth < depth {
		return 0, false
	}
	score := ttScoreFromPly(entry.score, ply)
	switch entry.flag {
	case 0:
		return score, true
	case 1:
		if score >= beta {
			return score, true
		}
	case 2:
		if score <= alpha {
			return score, true
		}
	}
	return 0, false
}

func (t *searchTables) store(key uint64, depth, score, alpha, beta, ply int, best move) {
	if t == nil {
		return
	}
	flag := uint8(0)
	if score <= alpha {
		flag = 2
	} else if score >= beta {
		flag = 1
	}
	t.tt[key] = ttEntry{depth: depth, score: ttScore(score, ply), flag: flag, best: best}
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

func (g *Game) childAccumulator(acc *nnue.Accumulator, board [Size]int8, m move) *nnue.Accumulator {
	if g.nnue == nil || acc == nil {
		return nil
	}
	child := g.accPool.Get().(*nnue.Accumulator)
	g.nnue.UpdateMoveInto(child, acc, board, m.From, m.To)
	return child
}

func (g *Game) releaseAccumulator(acc *nnue.Accumulator) {
	if acc != nil {
		g.accPool.Put(acc)
	}
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

func (g *Game) status(st *State) game.Status {
	return g.statusFrom(st, legalMoves(st))
}

// statusFrom derives the position status from an already generated move list.
func (g *Game) statusFrom(st *State, ms []move) game.Status {
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
