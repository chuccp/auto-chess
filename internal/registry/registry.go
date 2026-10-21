// Package registry wires every supported game engine into one lookup table.
package registry

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/chuccp/auto-chess/internal/game"
	"github.com/chuccp/auto-chess/internal/games/checkers"
	"github.com/chuccp/auto-chess/internal/games/chess"
	"github.com/chuccp/auto-chess/internal/games/connect4"
	"github.com/chuccp/auto-chess/internal/games/gomoku"
	"github.com/chuccp/auto-chess/internal/games/jungle"
	"github.com/chuccp/auto-chess/internal/games/reversi"
	"github.com/chuccp/auto-chess/internal/games/xiangqi"
)

// Registry is an immutable catalog of game engines.
type Registry struct {
	games map[string]game.Game
	metas []game.Meta
	byID  map[string]game.Meta
	order []string
}

// New builds the default registry with all supported games. If nnuePath is
// provided, xiangqi uses the Pikafish NNUE evaluator instead of godogpaw.
func New(nnuePath ...string) *Registry {
	r := &Registry{
		games: make(map[string]game.Game),
		byID:  make(map[string]game.Meta),
	}
	xiangqiGame := game.Game(xiangqi.New().WithNativeEngine())
	if len(nnuePath) > 0 && strings.TrimSpace(nnuePath[0]) != "" {
		if withNNUE, err := xiangqi.NewWithNNUE(nnuePath[0]); err == nil {
			xiangqiGame = withNNUE
		} else {
			fmt.Fprintf(os.Stderr, "加载 NNUE 失败，回退 godogpaw：%v\n", err)
		}
	}
	for _, g := range []game.Game{
		chess.New().WithNativeEngine(),
		xiangqiGame,
		gomoku.New(),
		reversi.New(),
		connect4.New(),
		jungle.New(),
		checkers.New(),
	} {
		meta := g.Meta()
		meta.InitialPosition = g.Encode(g.Initial(), true)
		r.games[meta.ID] = g
		r.metas = append(r.metas, meta)
		r.byID[meta.ID] = meta
		r.order = append(r.order, meta.ID)
		for _, alias := range meta.Aliases {
			r.games[strings.ToLower(alias)] = g
			r.byID[strings.ToLower(alias)] = meta
		}
	}
	sort.SliceStable(r.metas, func(i, j int) bool {
		return categoryRank(r.metas[i].Category) < categoryRank(r.metas[j].Category)
	})
	return r
}

func categoryRank(c string) int {
	switch c {
	case "经典棋类":
		return 0
	case "连珠类":
		return 1
	case "翻转类":
		return 2
	case "策略类":
		return 3
	case "跳棋类":
		return 4
	}
	return 9
}

// Get resolves an id or alias.
func (r *Registry) Get(id string) (game.Game, bool) {
	g, ok := r.games[strings.ToLower(strings.TrimSpace(id))]
	return g, ok
}

// Meta resolves an id or alias to its metadata.
func (r *Registry) Meta(id string) (game.Meta, bool) {
	m, ok := r.byID[strings.ToLower(strings.TrimSpace(id))]
	return m, ok
}

// List returns game metadata in display order.
func (r *Registry) List() []game.Meta {
	out := make([]game.Meta, len(r.metas))
	copy(out, r.metas)
	return out
}

// IDs returns the canonical game ids in display order.
func (r *Registry) IDs() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}
