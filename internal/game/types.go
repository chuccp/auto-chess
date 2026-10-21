// Package game defines the common contracts shared by every board-game AI
// engine exposed by auto-chess.
//
// Each engine owns its own internal position representation and implements the
// Game interface. The HTTP layer only works with the neutral JSON types in this
// file, which keeps the API identical for FEN based games (chess, xiangqi) and
// grid based games (gomoku, reversi, connect4, jungle, checkers).
package game

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Position is an opaque, engine specific position value.
type Position any

// PositionInput is the neutral position payload accepted by every engine.
//
// FEN based games read FEN. Grid based games read Board and Turn. The board is
// a flat row-major array; empty intersections are "" (or null from JSON).
type PositionInput struct {
	FEN   string   `json:"fen,omitempty"`
	Board []string `json:"board,omitempty"`
	Turn  string   `json:"turn,omitempty"`
}

// MoveInput is the neutral move payload accepted by Apply.
//
// FEN based games use UCI (for example "e2e4"). Grid based placement games use
// Index. All other games use From/To flat board indexes.
type MoveInput struct {
	UCI       string `json:"uci,omitempty"`
	From      *int   `json:"from,omitempty"`
	To        *int   `json:"to,omitempty"`
	Index     *int   `json:"index,omitempty"`
	Promotion string `json:"promotion,omitempty"`
}

// Coord locates a square or intersection on a board.
type Coord struct {
	Index int `json:"index"`
	Row   int `json:"row"`
	Col   int `json:"col"`
}

// Move is the neutral, human friendly move representation returned by the API.
type Move struct {
	UCI       string `json:"uci,omitempty"`
	From      *Coord `json:"from,omitempty"`
	To        *Coord `json:"to,omitempty"`
	Index     *Coord `json:"index,omitempty"`
	Promotion string `json:"promotion,omitempty"`
	Notation  string `json:"notation,omitempty"`
	Captured  string `json:"captured,omitempty"`
	Flipped   int    `json:"flipped,omitempty"`
	Jump      bool   `json:"jump,omitempty"`
	Check     bool   `json:"check,omitempty"`
}

// Status describes the state of a position after the last move.
type Status struct {
	State   string `json:"state"` // playing, check, checkmate, stalemate, win, draw
	Winner  string `json:"winner,omitempty"`
	Reason  string `json:"reason,omitempty"`
	InCheck bool   `json:"inCheck,omitempty"`
}

// Finished reports whether the game has ended.
func (s Status) Finished() bool {
	switch s.State {
	case "checkmate", "stalemate", "win", "draw":
		return true
	}
	return false
}

// PositionView is the neutral position representation returned by the API.
type PositionView struct {
	FEN            string   `json:"fen,omitempty"`
	Board          []string `json:"board"`
	Turn           string   `json:"turn"`
	MoveNumber     int      `json:"moveNumber,omitempty"`
	Status         Status   `json:"status"`
	LegalMoveCount int      `json:"legalMoveCount"`
	LegalMoves     []Move   `json:"legalMoves,omitempty"`
	LastMove       *Move    `json:"lastMove,omitempty"`
}

// BoardMeta documents the geometry of an engine's board.
type BoardMeta struct {
	Rows   int               `json:"rows"`
	Cols   int               `json:"cols"`
	Cells  int               `json:"cells"`
	Type   string            `json:"type"` // "fen" or "grid"
	Origin string            `json:"origin,omitempty"`
	Legend map[string]string `json:"legend,omitempty"`
}

// ColorMeta documents the two sides of a game.
type ColorMeta struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Meta is the public description of a game.
type Meta struct {
	ID              string       `json:"id"`
	Aliases         []string     `json:"aliases,omitempty"`
	Name            string       `json:"name"`
	EnglishName     string       `json:"englishName"`
	Tagline         string       `json:"tagline"`
	Category        string       `json:"category"`
	Players         int          `json:"players"`
	Board           BoardMeta    `json:"board"`
	Colors          []ColorMeta  `json:"colors"`
	PositionFormat  string       `json:"positionFormat"`
	MoveFormat      string       `json:"moveFormat"`
	Difficulties    []string     `json:"difficulties"`
	Features        []string     `json:"features"`
	Rules           []string     `json:"rules"`
	Engines         []string     `json:"engines,omitempty"`
	InitialPosition PositionView `json:"initialPosition"`
}

// Game is implemented by every board-game engine.
type Game interface {
	// Meta returns the static description of the game.
	Meta() Meta
	// Initial returns the engine specific starting position.
	Initial() Position
	// Decode parses the neutral position input.
	Decode(input PositionInput) (Position, error)
	// Encode converts an engine position to the neutral representation.
	// When includeLegalMoves is true the full legal move list is populated.
	Encode(pos Position, includeLegalMoves bool) PositionView
	// LegalMoves returns every legal move in a position.
	LegalMoves(pos Position) []Move
	// Apply validates and plays a move.
	Apply(pos Position, input MoveInput) (Position, Move, error)
	// Search chooses the best move for the side to move.
	Search(ctx context.Context, pos Position, opts SearchOptions) (SearchResult, error)
}

// EngineLister is implemented by games that can report which search engines
// they accept in SearchOptions.Engine. The first entry is the default engine
// used when the caller does not select one. Games without this method are
// assumed to offer only "builtin".
type EngineLister interface {
	Engines() []string
}

// SearchOptions controls one AI search.
type SearchOptions struct {
	Difficulty string
	MaxDepth   int
	TimeLimit  time.Duration
	Seed       int64
	// Engine optionally selects a concrete search implementation, e.g.
	// "godogpaw", "pikafish-nnue", "chess-3" or "builtin". Empty (or "auto")
	// lets the game pick its default. Unsupported names return CodeUnknownEngine.
	Engine string
	// Progress, when set, is called after each completed iterative-deepening
	// level. Callbacks must be fast and non-blocking; the SSE API forwards them
	// through a buffered channel and drops stale progress if the client is slow.
	Progress ProgressFunc
}

// SearchProgress is one completed search level. It is streamed by the SSE API
// so a client can show that a high-difficulty search is still working.
type SearchProgress struct {
	Depth    int    `json:"depth"`
	Score    int    `json:"score"`
	Mate     int    `json:"mate,omitempty"`
	Nodes    int64  `json:"nodes"`
	TimeMs   int64  `json:"timeMs"`
	Best     *Move  `json:"best,omitempty"`
	PV       []Move `json:"pv,omitempty"`
	TimedOut bool   `json:"timedOut,omitempty"`
}

// ProgressFunc receives search progress.
type ProgressFunc func(SearchProgress)

// Report emits one progress record when a callback is configured.
func (o SearchOptions) Report(p SearchProgress) {
	if o.Progress != nil {
		o.Progress(p)
	}
}

// SearchInfo reports search statistics.
type SearchInfo struct {
	Depth    int    `json:"depth"`
	Nodes    int64  `json:"nodes"`
	TimeMs   int64  `json:"timeMs"`
	TimedOut bool   `json:"timedOut,omitempty"`
	PV       []Move `json:"pv,omitempty"`
}

// Evaluation is the engine score for the move that was played.
type Evaluation struct {
	Score       int    `json:"score"`
	Unit        string `json:"unit"` // centipawn
	Perspective string `json:"perspective"`
	MateIn      int    `json:"mateIn,omitempty"`
}

// SearchResult is everything the AI endpoint returns.
type SearchResult struct {
	Game       string        `json:"game"`
	Difficulty string        `json:"difficulty"`
	Engine     string        `json:"engine,omitempty"`
	Move       *Move         `json:"move"`
	GameOver   bool          `json:"gameOver"`
	Status     Status        `json:"status"`
	Evaluation Evaluation    `json:"evaluation"`
	Search     SearchInfo    `json:"search"`
	Position   *PositionView `json:"position,omitempty"`
}

// Error is a domain error with a stable machine readable code.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// NewError builds a domain error.
func NewError(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Error codes shared by the API and the engines.
const (
	CodeInvalidPosition = "INVALID_POSITION"
	CodeInvalidMove     = "INVALID_MOVE"
	CodeIllegalMove     = "ILLEGAL_MOVE"
	CodeGameOver        = "GAME_OVER"
	CodeUnknownGame     = "UNKNOWN_GAME"
	CodeUnknownEngine   = "UNKNOWN_ENGINE"
	CodeBadRequest      = "BAD_REQUEST"
	CodeTimeout         = "TIMEOUT"
)

// AsError extracts a domain error from an error chain.
func AsError(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// DifficultyPreset is the default time budget for a difficulty level.
type DifficultyPreset struct {
	Depth int
	Time  time.Duration
}

// Presets are the shared difficulty defaults. Engines may lower the depth when
// their branching factor requires it.
var Presets = map[string]DifficultyPreset{
	"easy":   {Depth: 2, Time: 300 * time.Millisecond},
	"normal": {Depth: 4, Time: time.Second},
	"hard":   {Depth: 5, Time: 2500 * time.Millisecond},
	"master": {Depth: 7, Time: 6000 * time.Millisecond},
}

// DifficultyOrder is the canonical ordering used by the docs and API.
var DifficultyOrder = []string{"easy", "normal", "hard", "master"}

// NormalizeDifficulty returns a known difficulty, defaulting to normal.
func NormalizeDifficulty(d string) string {
	if _, ok := Presets[d]; ok {
		return d
	}
	return "normal"
}

// DepthFor returns the effective depth for a difficulty, capped by maxDepth and
// an engine provided ceiling. A non-zero requested depth always wins.
func DepthFor(difficulty string, requested, engineMax int) int {
	if requested > 0 {
		if requested > engineMax {
			return engineMax
		}
		return requested
	}
	if engineMax <= 0 {
		engineMax = 10
	}
	d := Presets[NormalizeDifficulty(difficulty)].Depth
	if d > engineMax {
		return engineMax
	}
	return d
}

// TimeFor returns the effective time budget. A non-zero requested budget wins.
func TimeFor(difficulty string, requested time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}
	return Presets[NormalizeDifficulty(difficulty)].Time
}

// IsGridBoard validates the length of a decoded grid board.
func IsGridBoard(board []string, size int) bool {
	return len(board) == size
}

// Cell normalizes a JSON board cell: null decodes as "", so both mean empty.
func Cell(s string) string { return s }
