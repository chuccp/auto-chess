package chess

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
	"github.com/paulsonkoly/chess-3/board"
	c3 "github.com/paulsonkoly/chess-3/chess"
	c3search "github.com/paulsonkoly/chess-3/search"
	"github.com/paulsonkoly/chess-3/transp"
)

// searchNative runs the pure-Go Chess-3 engine. Chess-3 is MIT licensed and
// searches at roughly 3000 CCRL Elo.
func (g *Game) searchNative(ctx context.Context, pos game.Position, opts game.SearchOptions) (game.SearchResult, error) {
	st := pos.(*State)
	start := time.Now()
	b, err := board.FromFEN(st.fen())
	if err != nil {
		return game.SearchResult{}, game.NewError("ENGINE_ERROR", "chess-3 无法解析局面：%v", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeLimit := opts.TimeLimit
	if timeLimit <= 0 {
		timeLimit = time.Second
	}
	depth := nativeDepth(opts)

	sink := &c3Sink{}
	sink.onInfo = func(info c3Info) {
		var best *game.Move
		if len(info.pv) > 0 {
			if mm, err := parseUCI(info.pv[0]); err == nil {
				mv := g.moveView(st, mm)
				best = &mv
			}
		}
		opts.Report(game.SearchProgress{
			Depth:  info.depth,
			Score:  info.score,
			Mate:   info.mate,
			Nodes:  info.nodes,
			TimeMs: info.timeMs,
			Best:   best,
		})
	}

	counters := c3search.Counters{}
	s := c3search.New(16 * transp.MegaBytes)
	score, best, _ := s.Go(b,
		c3search.WithSoftTime(timeLimit.Milliseconds()),
		c3search.WithDepth(c3.Depth(depth)),
		c3search.WithStop(ctx.Done()),
		c3search.WithOutput(sink),
		c3search.WithCounters(&counters),
	)
	if best == 0 {
		return game.SearchResult{}, game.NewError(game.CodeTimeout, "chess-3 没有找到合法走法")
	}
	uci := best.String()
	m, err := parseUCI(uci)
	if err != nil {
		return game.SearchResult{}, game.NewError("ENGINE_ERROR", "chess-3 返回了无效走法 %q：%v", uci, err)
	}
	legal := false
	for _, candidate := range legalMoves(st) {
		if candidate.From == m.From && candidate.To == m.To &&
			(m.Promotion == 0 || candidate.Promotion == m.Promotion) {
			legal = true
			break
		}
	}
	if !legal {
		return game.SearchResult{}, game.NewError("ENGINE_ERROR", "chess-3 返回了非法走法 %q", uci)
	}
	if opts.Forbids(g.moveView(st, m)) {
		// chess-3 cannot exclude root moves; the builtin search honours the
		// caller's ban list.
		return g.searchBuiltin(ctx, pos, opts)
	}

	next := makeMove(st, m)
	next.LastFrom, next.LastTo, next.LastPromo = m.From, m.To, m.Promotion
	mv := g.moveView(st, m)
	mv.Check = inCheck(next, next.Turn)
	nv := g.Encode(next, false)

	mateIn := 0
	if score.IsMate() {
		abs := score
		if abs < 0 {
			abs = -abs
		}
		mateIn = (int(c3.Inf-abs) + 1) / 2
		if score < 0 {
			mateIn = -mateIn
		}
	}
	pv := make([]game.Move, 0, len(sink.pv))
	for _, pvUCI := range sink.pv {
		if pm, err := parseUCI(pvUCI); err == nil {
			pv = append(pv, g.moveView(st, pm))
		}
	}
	if len(pv) == 0 {
		pv = []game.Move{mv}
	}
	reached := sink.depth
	if reached < 1 {
		reached = 1
	}
	return game.SearchResult{
		Game:       "chess",
		Difficulty: game.NormalizeDifficulty(opts.Difficulty),
		Engine:     "chess-3",
		Move:       &mv,
		GameOver:   nv.Status.Finished(),
		Status:     nv.Status,
		Position:   &nv,
		Evaluation: game.Evaluation{
			Score:       int(score),
			Unit:        "centipawn",
			Perspective: colorString(st.Turn),
			MateIn:      mateIn,
		},
		Search: game.SearchInfo{
			Depth:  reached,
			Nodes:  int64(counters.Nodes),
			TimeMs: time.Since(start).Milliseconds(),
			PV:     pv,
		},
	}, nil
}

func nativeDepth(opts game.SearchOptions) int {
	if opts.MaxDepth > 0 {
		d := opts.MaxDepth
		if d > 20 {
			d = 20
		}
		if d < 1 {
			d = 1
		}
		return d
	}
	switch game.NormalizeDifficulty(opts.Difficulty) {
	case "easy":
		return 4
	case "normal":
		return 8
	case "hard":
		return 12
	case "master":
		return 20
	}
	return 8
}

type c3Info struct {
	depth  int
	score  int
	mate   int
	nodes  int64
	timeMs int64
	pv     []string
}

type c3Sink struct {
	buf    strings.Builder
	depth  int
	pv     []string
	onInfo func(c3Info)
}

func (w *c3Sink) Write(p []byte) (int, error) {
	_, _ = w.buf.Write(p)
	for {
		s := w.buf.String()
		idx := strings.IndexByte(s, '\n')
		if idx < 0 {
			break
		}
		line := s[:idx]
		w.buf.Reset()
		_, _ = w.buf.WriteString(s[idx+1:])
		w.parse(line)
	}
	return len(p), nil
}

func (w *c3Sink) parse(line string) {
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "info" {
		return
	}
	info := c3Info{}
	for i := 1; i < len(fields); i++ {
		switch fields[i] {
		case "depth":
			if i+1 < len(fields) {
				info.depth, _ = strconv.Atoi(fields[i+1])
			}
		case "score":
			if i+2 < len(fields) {
				value, _ := strconv.Atoi(fields[i+2])
				switch fields[i+1] {
				case "cp":
					info.score = value
				case "mate":
					info.mate = value
				}
			}
		case "nodes":
			if i+1 < len(fields) {
				info.nodes, _ = strconv.ParseInt(fields[i+1], 10, 64)
			}
		case "time":
			if i+1 < len(fields) {
				info.timeMs, _ = strconv.ParseInt(fields[i+1], 10, 64)
			}
		case "pv":
			info.pv = append(info.pv, fields[i+1:]...)
			i = len(fields)
		}
	}
	if info.depth == 0 || len(info.pv) == 0 {
		return
	}
	w.depth = info.depth
	w.pv = info.pv
	if w.onInfo != nil {
		w.onInfo(info)
	}
}
