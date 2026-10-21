package xiangqi

import (
	"context"
	"time"

	"github.com/chuccp/auto-chess/internal/engines/godogpaw"
	"github.com/chuccp/auto-chess/internal/game"
)

// searchNative runs the pure-Go godogpaw engine. Search state lives on the
// position (PV table, transposition table, stop flag), so concurrent searches
// do not need to be serialized.
func (g *Game) searchNative(ctx context.Context, pos game.Position, opts game.SearchOptions) (game.SearchResult, error) {
	st := pos.(*State)
	start := time.Now()
	if ctx == nil {
		ctx = context.Background()
	}
	limit := opts.TimeLimit
	if limit <= 0 {
		limit = time.Second
	}
	depth := nativeDepth(opts)

	enginePos := new(godogpaw.PositionNG)
	enginePos.Set(st.fen())

	var last godogpaw.SearchProgress
	progress := func(p godogpaw.SearchProgress) {
		last = p
		var best *game.Move
		if len(p.PV) > 0 {
			if m, err := parseUCCI(p.PV[0]); err == nil {
				mv := g.moveView(m)
				best = &mv
			}
		}
		opts.Report(game.SearchProgress{
			Depth:  p.Depth,
			Score:  int(p.Score),
			Nodes:  int64(p.Nodes),
			TimeMs: p.Time.Milliseconds(),
			Best:   best,
		})
	}

	stop := make(chan struct{})
	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		select {
		case <-ctx.Done():
			enginePos.StopSearch()
		case <-stop:
		}
	}()
	bestMove := enginePos.SearchWithProgress(godogpaw.SearchLimits{
		Depth:     uint8(depth),
		TimeLimit: limit,
	}, progress)
	close(stop)
	<-stopDone

	if bestMove == 0 {
		return game.SearchResult{}, game.NewError(game.CodeTimeout, "godogpaw 没有找到合法走法")
	}
	ucci := godogpaw.Move2Str(bestMove)
	m, err := parseUCCI(ucci)
	if err != nil {
		return game.SearchResult{}, game.NewError("ENGINE_ERROR", "godogpaw 返回了无效走法 %q：%v", ucci, err)
	}
	legal := false
	for _, candidate := range legalMoves(st) {
		if candidate.From == m.From && candidate.To == m.To {
			legal = true
			break
		}
	}
	if !legal {
		return game.SearchResult{}, game.NewError("ENGINE_ERROR", "godogpaw 返回了非法走法 %q", ucci)
	}
	if opts.Forbids(g.moveView(m)) {
		// godogpaw cannot exclude root moves; the bitboard search honours the
		// caller's ban list. Only the remaining budget is spent so the retry
		// cannot double the request latency.
		opts.TimeLimit = game.RemainingBudget(limit, time.Since(start))
		return g.searchPosition(ctx, pos, opts, g.nnue != nil)
	}

	next := makeMove(st, m)
	next.LastFrom, next.LastTo = m.From, m.To
	mv := g.moveView(m)
	mv.Check = inCheck(next, next.Turn)
	nv := g.Encode(next, false)

	score := int(last.Score)
	mateIn := 0
	if last.Score >= godogpaw.VALUE_MATE_IN_MAX_PLY {
		mateIn = (int(godogpaw.VALUE_MATE-last.Score) + 1) / 2
	} else if last.Score <= -godogpaw.VALUE_MATE_IN_MAX_PLY {
		mateIn = -((int(godogpaw.VALUE_MATE+last.Score) + 1) / 2)
	}
	pv := make([]game.Move, 0, len(last.PV))
	for _, pvUCCI := range last.PV {
		if pm, err := parseUCCI(pvUCCI); err == nil {
			pv = append(pv, g.moveView(pm))
		}
	}
	if len(pv) == 0 {
		pv = []game.Move{mv}
	}
	reached := last.Depth
	if reached < 1 {
		reached = 1
	}
	nodes := int64(last.Nodes)
	if nodes == 0 {
		nodes = int64(enginePos.Nodes)
	}
	return game.SearchResult{
		Game:       "xiangqi",
		Difficulty: game.NormalizeDifficulty(opts.Difficulty),
		Engine:     "godogpaw",
		Move:       &mv,
		GameOver:   nv.Status.Finished(),
		Status:     nv.Status,
		Position:   &nv,
		Evaluation: game.Evaluation{
			Score:       score,
			Unit:        "centipawn",
			Perspective: colorString(st.Turn),
			MateIn:      mateIn,
		},
		Search: game.SearchInfo{
			Depth:    reached,
			Nodes:    nodes,
			TimeMs:   time.Since(start).Milliseconds(),
			PV:       pv,
			TimedOut: ctx.Err() != nil,
		},
	}, nil
}

func nativeDepth(opts game.SearchOptions) int {
	if opts.MaxDepth > 0 {
		d := opts.MaxDepth
		if d > 12 {
			d = 12
		}
		if d < 1 {
			d = 1
		}
		return d
	}
	switch game.NormalizeDifficulty(opts.Difficulty) {
	case "easy":
		return 2
	case "normal":
		return 4
	case "hard":
		return 6
	case "master":
		return 0 // time-limited iterative deepening
	}
	return 4
}
