// Package chess implements standard international chess with full legal move
// generation (castling, en passant, promotion) and a negamax AI.
package chess

import (
	"context"
	"strings"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

const (
	Empty  int8 = 0
	Pawn   int8 = 1
	Knight int8 = 2
	Bishop int8 = 3
	Rook   int8 = 4
	Queen  int8 = 5
	King   int8 = 6

	White int8 = 1
	Black int8 = -1

	mateScore = 100_000
)

const (
	castleWK uint8 = 1 << iota
	castleWQ
	castleBK
	castleBQ
)

// State is a complete chess position.
type State struct {
	Board     [64]int8
	Turn      int8
	Castling  uint8
	EP        int
	Halfmove  int
	Fullmove  int
	LastFrom  int
	LastTo    int
	LastPromo int8
}

type move struct {
	From      int
	To        int
	Promotion int8
}

// Game implements game.Game.
type Game struct {
	native bool
}

// New returns a chess engine backed by the built-in search. Tests and callers
// that need a self-contained implementation use this.
func New() *Game { return &Game{} }

// WithNativeEngine switches this game to the pure-Go Chess-3 engine.
func (g *Game) WithNativeEngine() *Game {
	g.native = true
	return g
}

// Meta implements game.Game.
func (g *Game) Meta() game.Meta {
	return game.Meta{
		ID:          "chess",
		Aliases:     []string{"international-chess", "western-chess"},
		Name:        "国际象棋",
		EnglishName: "Chess",
		Tagline:     "标准 FIDE 规则，王车易位、吃过路兵、升变全部支持。",
		Category:    "经典棋类",
		Players:     2,
		Board: game.BoardMeta{
			Rows: 8, Cols: 8, Cells: 64, Type: "fen",
			Origin: "FEN 标准坐标，a1 = 0，h8 = 63；白方在下",
			Legend: map[string]string{
				"P": "白兵", "N": "白马", "B": "白象", "R": "白车", "Q": "白后", "K": "白王",
				"p": "黑兵", "n": "黑马", "b": "黑象", "r": "黑车", "q": "黑后", "k": "黑王",
			},
		},
		Colors:         []game.ColorMeta{{ID: "w", Name: "白方"}, {ID: "b", Name: "黑方"}},
		PositionFormat: "标准 FEN，例如 rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
		MoveFormat:     "UCI，例如 e2e4、e7e8q；升变时在末尾加 q/r/b/n",
		Difficulties:   game.DifficultyOrder,
		Features:       []string{"完整 FIDE 走子规则", "王车易位", "吃过路兵", "兵升变", "将杀与逼和", "Chess-3 引擎"},
		Rules: []string{
			"白方先行，目标是将死对方王。",
			"王车易位要求王与车均未移动、之间无子、王不在被将军状态且经过的格子不受攻击。",
			"兵首次可前进两格；吃子斜走一格；到达底线必须升变为后、车、象或马。",
			"吃过路兵只能在对方兵首次前进两格的下一步立即执行。",
			"无合法走法时：被将军为将杀，否则为逼和。",
			"50 回合无吃子无兵动、或子力不足时判和。",
		},
	}
}

// Initial implements game.Game.
func (g *Game) Initial() game.Position {
	st, _ := parseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
	return st
}

// Decode implements game.Game.
func (g *Game) Decode(in game.PositionInput) (game.Position, error) {
	if in.FEN == "" {
		return nil, game.NewError(game.CodeInvalidPosition, "chess 需要提供 position.fen")
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
	board := make([]string, 64)
	for i, p := range st.Board {
		if p == 0 {
			board[i] = ""
			continue
		}
		letter := string(pieceLetter(absPiece(p)))
		if p > 0 {
			letter = string([]byte{letter[0] &^ 0x20})
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
		m := g.moveView(st, move{From: st.LastFrom, To: st.LastTo, Promotion: st.LastPromo})
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
		out = append(out, g.moveView(st, m))
	}
	return out
}

// Apply implements game.Game.
func (g *Game) Apply(pos game.Position, in game.MoveInput) (game.Position, game.Move, error) {
	st := pos.(*State)
	if g.status(st).Finished() {
		return nil, game.Move{}, game.NewError(game.CodeGameOver, "对局已经结束")
	}
	m, err := parseMoveInput(st, in)
	if err != nil {
		return nil, game.Move{}, err
	}
	var legal *move
	for _, candidate := range legalMoves(st) {
		if candidate.From == m.From && candidate.To == m.To {
			if m.Promotion == 0 || candidate.Promotion == m.Promotion {
				c := candidate
				legal = &c
				break
			}
		}
	}
	if legal == nil {
		return nil, game.Move{}, game.NewError(game.CodeIllegalMove, "走法 %s 不合法", m.uci())
	}
	next := makeMove(st, *legal)
	next.LastFrom, next.LastTo, next.LastPromo = legal.From, legal.To, legal.Promotion
	mv := g.moveView(st, *legal)
	mv.Check = inCheck(next, next.Turn)
	return next, mv, nil
}

// Search implements game.Game.
func (g *Game) Search(ctx context.Context, pos game.Position, opts game.SearchOptions) (game.SearchResult, error) {
	if g.native {
		st := pos.(*State)
		if status := g.status(st); status.Finished() {
			return game.SearchResult{
				Game:       "chess",
				Difficulty: game.NormalizeDifficulty(opts.Difficulty),
				Engine:     "chess-3",
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
	result := game.SearchResult{Game: "chess", Difficulty: game.NormalizeDifficulty(opts.Difficulty)}
	rootMoves := legalMoves(st)
	if len(rootMoves) == 0 {
		result.GameOver = true
		result.Status = g.status(st)
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
			depth = 5
		case "master":
			depth = 6
		}
	}
	limit := game.TimeFor(opts.Difficulty, opts.TimeLimit)
	deadline := start.Add(limit)
	s := &searcher{deadline: deadline, ctx: ctx}

	s.order(st, rootMoves, 0)
	best := rootMoves[0]
	bestScore := 0
	reached := 0
	var pv []move
	for d := 1; d <= depth; d++ {
		if time.Now().After(deadline) || ctx.Err() != nil {
			s.timedOut = true
			break
		}
		alpha, beta := -mateScore*2, mateScore*2
		localBest := rootMoves[0]
		localScore := 0
		var localPV []move
		complete := true
		for i, m := range rootMoves {
			child := makeMove(st, m)
			s.pv[1] = s.pv[1][:0]
			score := -s.negamax(child, d-1, -beta, -alpha, 1)
			if s.timedOut {
				complete = false
				break
			}
			if i == 0 || score > localScore {
				localBest = m
				localScore = score
				localPV = []move{m}
				if len(s.pv[1]) > 0 {
					localPV = append(localPV, s.pv[1]...)
				}
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
		pv = localPV
		bestView := g.moveView(st, best)
		pvView := make([]game.Move, 0, len(pv))
		for _, pm := range pv {
			pvView = append(pvView, g.moveView(st, pm))
		}
		opts.Report(game.SearchProgress{
			Depth:  d,
			Score:  bestScore,
			Nodes:  s.nodes,
			TimeMs: time.Since(start).Milliseconds(),
			Best:   &bestView,
			PV:     pvView,
		})
		rootMoves = moveToFront(rootMoves, best)
		if abs(bestScore) >= mateScore-1000 {
			break
		}
	}
	next := makeMove(st, best)
	next.LastFrom, next.LastTo, next.LastPromo = best.From, best.To, best.Promotion
	mv := g.moveView(st, best)
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
	result.Search = game.SearchInfo{Depth: reached, Nodes: s.nodes, TimeMs: time.Since(start).Milliseconds(), TimedOut: s.timedOut}
	for _, m := range pv {
		result.Search.PV = append(result.Search.PV, g.moveView(st, m))
	}
	return result, nil
}

func (g *Game) moveView(st *State, m move) game.Move {
	fromCopy, toCopy := m.From, m.To
	out := game.Move{
		UCI:  m.uci(),
		From: &game.Coord{Index: fromCopy, Row: fromCopy / 8, Col: fromCopy % 8},
		To:   &game.Coord{Index: toCopy, Row: toCopy / 8, Col: toCopy % 8},
	}
	if m.Promotion != 0 {
		out.Promotion = string(pieceLetter(m.Promotion))
	}
	if captured := st.Board[m.To]; captured != 0 {
		letter := string(pieceLetter(absPiece(captured)))
		if captured > 0 {
			letter = strings.ToUpper(letter)
		}
		out.Captured = colorLetter(colorOf(captured)) + letter
	}
	out.Notation = out.UCI
	return out
}

func parseMoveInput(st *State, in game.MoveInput) (move, error) {
	if in.UCI != "" {
		m, err := parseUCI(in.UCI)
		if err != nil {
			return move{}, err
		}
		if m.Promotion == 0 && absPiece(st.Board[m.From]) == Pawn && (m.To/8 == 7 || m.To/8 == 0) {
			m.Promotion = Queen
		}
		return m, nil
	}
	if in.From == nil || in.To == nil {
		return move{}, game.NewError(game.CodeInvalidMove, "chess 需要 uci 或 from/to 坐标")
	}
	from, to := *in.From, *in.To
	if from < 0 || from >= 64 || to < 0 || to >= 64 {
		return move{}, game.NewError(game.CodeInvalidMove, "棋盘坐标超出范围")
	}
	m := move{From: from, To: to}
	if in.Promotion != "" {
		p, ok := pieceFromLetter(in.Promotion[0])
		if !ok || p == King || p == Pawn {
			return move{}, game.NewError(game.CodeInvalidMove, "升变棋子必须是 q/r/b/n")
		}
		m.Promotion = p
	}
	// If this is a promotion and no piece was supplied, default to queen.
	if absPiece(st.Board[from]) == Pawn && (to/8 == 7 || to/8 == 0) && m.Promotion == 0 {
		m.Promotion = Queen
	}
	return m, nil
}

func squareName(sq int) string {
	return string([]byte{byte('a' + sq%8), byte('1' + sq/8)})
}

func parseUCI(s string) (move, error) {
	if len(s) != 4 && len(s) != 5 {
		return move{}, game.NewError(game.CodeInvalidMove, "UCI 走法长度必须为 4 或 5：%q", s)
	}
	from, ok1 := parseSquare(s[0:2])
	to, ok2 := parseSquare(s[2:4])
	if !ok1 || !ok2 {
		return move{}, game.NewError(game.CodeInvalidMove, "UCI 走法 %q 坐标无效", s)
	}
	m := move{From: from, To: to}
	if len(s) == 5 {
		p, ok := pieceFromLetter(s[4])
		if !ok || p == King || p == Pawn {
			return move{}, game.NewError(game.CodeInvalidMove, "升变棋子必须是 q/r/b/n")
		}
		m.Promotion = p
	}
	return m, nil
}

func parseSquare(s string) (int, bool) {
	if len(s) != 2 || s[0] < 'a' || s[0] > 'h' || s[1] < '1' || s[1] > '8' {
		return 0, false
	}
	return int(s[1]-'1')*8 + int(s[0]-'a'), true
}

func (m move) uci() string {
	s := squareName(m.From) + squareName(m.To)
	if m.Promotion != 0 {
		s += string(pieceLetter(m.Promotion))
	}
	return s
}

func pieceLetter(p int8) byte {
	switch p {
	case Pawn:
		return 'p'
	case Knight:
		return 'n'
	case Bishop:
		return 'b'
	case Rook:
		return 'r'
	case Queen:
		return 'q'
	case King:
		return 'k'
	}
	return '?'
}

func pieceFromLetter(c byte) (int8, bool) {
	switch c {
	case 'p', 'P':
		return Pawn, true
	case 'n', 'N':
		return Knight, true
	case 'b', 'B':
		return Bishop, true
	case 'r', 'R':
		return Rook, true
	case 'q', 'Q':
		return Queen, true
	case 'k', 'K':
		return King, true
	}
	return 0, false
}

func colorOf(p int8) int8 {
	if p > 0 {
		return White
	}
	if p < 0 {
		return Black
	}
	return 0
}

func colorLetter(c int8) string {
	if c == Black {
		return "b"
	}
	return "w"
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

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func moveToFront(moves []move, m move) []move {
	out := make([]move, 0, len(moves))
	out = append(out, m)
	for _, v := range moves {
		if v != m {
			out = append(out, v)
		}
	}
	return out
}
