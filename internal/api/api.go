// Package api exposes the stateless AI move API and the documentation page.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
	"github.com/chuccp/auto-chess/internal/registry"
	"github.com/chuccp/auto-chess/internal/webui"
	"github.com/chuccp/go-web-frame/core"
	"github.com/chuccp/go-web-frame/log"
	"github.com/chuccp/go-web-frame/web"
	"go.uber.org/zap"
)

// Controller registers all HTTP routes.
type Controller struct {
	Registry  *registry.Registry
	Renderer  *webui.Renderer
	StartedAt time.Time
}

// Init implements core.IService.
func (c *Controller) Init(ctx *core.Context) error {
	if c.StartedAt.IsZero() {
		c.StartedAt = time.Now()
	}
	ctx.Get("/api/v1/health", c.health)
	ctx.Get("/api/v1/games", c.listGames)
	ctx.Get("/api/v1/games/:game", c.gameDetail)
	ctx.Post("/api/v1/move", c.move)
	ctx.Post("/api/v1/games/:game/move", c.move)
	ctx.Post("/api/v1/apply-move", c.applyMove)
	ctx.Post("/api/v1/games/:game/apply-move", c.applyMove)
	ctx.Get("/api/v1/move/stream", c.moveStream)
	ctx.Post("/api/v1/move/stream", c.moveStream)
	ctx.Get("/api/v1/games/:game/move/stream", c.moveStream)
	ctx.Post("/api/v1/games/:game/move/stream", c.moveStream)
	ctx.Post("/api/v1/legal-moves", c.legalMoves)
	ctx.Post("/api/v1/games/:game/legal-moves", c.legalMoves)

	ctx.Get("/", c.homePage)
	ctx.Get("/guide", c.guidePage)
	ctx.Get("/docs", c.apiPage)
	ctx.Get("/docs/", c.apiPage)
	ctx.Get("/docs/stream", c.streamPage)
	ctx.Get("/games", c.gamesPage)
	ctx.Get("/games/:game", c.gamePage)
	ctx.Get("/play", c.playPage)
	ctx.Get("/playground", c.playgroundPage)
	ctx.Get("/assets/app.css", c.asset("app.css", "text/css; charset=utf-8"))
	ctx.Get("/assets/site.js", c.asset("site.js", "application/javascript; charset=utf-8"))
	ctx.Get("/assets/play.js", c.asset("play.js", "application/javascript; charset=utf-8"))
	ctx.Get("/assets/playground.js", c.asset("playground.js", "application/javascript; charset=utf-8"))
	return nil
}

type successBody struct {
	OK   bool `json:"ok"`
	Data any  `json:"data"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorBody struct {
	OK    bool        `json:"ok"`
	Error errorDetail `json:"error"`
}

func writeJSON(req *web.Request, status int, value any) (any, error) {
	req.Response().Header().Set("Content-Type", "application/json; charset=utf-8")
	req.Response().JSON(status, value)
	return nil, nil
}

func ok(req *web.Request, data any) (any, error) {
	return writeJSON(req, http.StatusOK, successBody{OK: true, Data: data})
}

func fail(req *web.Request, status int, code, message string) (any, error) {
	return writeJSON(req, status, errorBody{OK: false, Error: errorDetail{Code: code, Message: message}})
}

func failErr(req *web.Request, err error) (any, error) {
	var de *game.Error
	if errors.As(err, &de) {
		status := http.StatusBadRequest
		switch de.Code {
		case game.CodeUnknownGame:
			status = http.StatusNotFound
		case game.CodeIllegalMove:
			status = http.StatusUnprocessableEntity
		case game.CodeGameOver:
			status = http.StatusConflict
		case game.CodeTimeout:
			status = http.StatusGatewayTimeout
		}
		return fail(req, status, de.Code, de.Message)
	}
	return fail(req, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
}

type healthData struct {
	Status        string `json:"status"`
	Service       string `json:"service"`
	Version       string `json:"version"`
	Games         int    `json:"games"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
	Time          string `json:"time"`
}

func (c *Controller) health(req *web.Request) (any, error) {
	return ok(req, healthData{
		Status:        "ok",
		Service:       "auto-chess",
		Version:       "0.1.0",
		Games:         len(c.Registry.List()),
		UptimeSeconds: int64(time.Since(c.StartedAt).Seconds()),
		Time:          time.Now().Format(time.RFC3339),
	})
}

type gamesData struct {
	Count int         `json:"count"`
	Games []game.Meta `json:"games"`
}

func (c *Controller) listGames(req *web.Request) (any, error) {
	games := c.Registry.List()
	return ok(req, gamesData{Count: len(games), Games: games})
}

func (c *Controller) gameDetail(req *web.Request) (any, error) {
	id := req.Param("game")
	meta, found := c.Registry.Meta(id)
	if !found {
		return fail(req, http.StatusNotFound, game.CodeUnknownGame, "不支持的棋类："+id)
	}
	return ok(req, meta)
}

type positionInput struct {
	FEN   string   `json:"fen,omitempty"`
	Board []string `json:"board,omitempty"`
	Turn  string   `json:"turn,omitempty"`
}

type moveRequest struct {
	Game        string         `json:"game,omitempty"`
	Position    *positionInput `json:"position,omitempty"`
	FEN         string         `json:"fen,omitempty"`
	Board       []string       `json:"board,omitempty"`
	Turn        string         `json:"turn,omitempty"`
	Difficulty  string         `json:"difficulty,omitempty"`
	TimeLimitMs int            `json:"timeLimitMs,omitempty"`
	MaxDepth    int            `json:"maxDepth,omitempty"`
	Seed        int64          `json:"seed,omitempty"`
}

func (r *moveRequest) position() game.PositionInput {
	in := game.PositionInput{FEN: r.FEN, Board: r.Board, Turn: r.Turn}
	if r.Position != nil {
		if r.Position.FEN != "" {
			in.FEN = r.Position.FEN
		}
		if len(r.Position.Board) > 0 {
			in.Board = r.Position.Board
		}
		if r.Position.Turn != "" {
			in.Turn = r.Position.Turn
		}
	}
	return in
}

func (c *Controller) resolve(req *web.Request, bodyGame string) (game.Game, string, error) {
	id := req.Param("game")
	if id == "" {
		id = bodyGame
	}
	if id == "" {
		return nil, "", game.NewError(game.CodeBadRequest, "缺少 game 参数，可选值：%s", joinIDs(c.Registry.IDs()))
	}
	g, found := c.Registry.Get(id)
	if !found {
		return nil, "", game.NewError(game.CodeUnknownGame, "不支持的棋类：%s，可选值：%s", id, joinIDs(c.Registry.IDs()))
	}
	meta, _ := c.Registry.Meta(id)
	return g, meta.ID, nil
}

func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ", "
		}
		out += id
	}
	return out
}

func bindMoveRequest(req *web.Request) (moveRequest, error) {
	var body moveRequest
	if !req.IsGet() {
		if err := req.BindJSON(&body); err != nil {
			return body, fmt.Errorf("请求体不是合法 JSON：%w", err)
		}
		return body, nil
	}
	if raw := req.Query("payload"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return body, fmt.Errorf("payload 不是合法 JSON：%w", err)
		}
	}
	if raw := req.Query("position"); raw != "" {
		var p positionInput
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return body, fmt.Errorf("position 不是合法 JSON：%w", err)
		}
		body.Position = &p
	}
	if v := req.Query("game"); v != "" {
		body.Game = v
	}
	if v := req.Query("fen"); v != "" {
		body.FEN = v
	}
	if v := req.Query("turn"); v != "" {
		body.Turn = v
	}
	if v := req.Query("difficulty"); v != "" {
		body.Difficulty = v
	}
	if v := req.Query("timeLimitMs"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			body.TimeLimitMs = n
		}
	}
	if v := req.Query("maxDepth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			body.MaxDepth = n
		}
	}
	if v := req.Query("seed"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			body.Seed = n
		}
	}
	if raw := req.Query("board"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &body.Board); err != nil {
			return body, fmt.Errorf("board 参数不是 JSON 数组：%w", err)
		}
	}
	return body, nil
}

type preparedSearch struct {
	game       game.Game
	gameID     string
	position   game.Position
	options    game.SearchOptions
	limit      time.Duration
	difficulty string
}

func (c *Controller) prepareSearch(req *web.Request, body moveRequest) (*preparedSearch, error) {
	g, gameID, err := c.resolve(req, body.Game)
	if err != nil {
		return nil, err
	}
	pos, err := g.Decode(body.position())
	if err != nil {
		return nil, err
	}
	difficulty := game.NormalizeDifficulty(body.Difficulty)
	limit := game.TimeFor(difficulty, time.Duration(body.TimeLimitMs)*time.Millisecond)
	if limit <= 0 {
		limit = time.Second
	}
	if limit > 15*time.Second {
		limit = 15 * time.Second
	}
	if limit < 50*time.Millisecond {
		limit = 50 * time.Millisecond
	}
	depth := body.MaxDepth
	if depth < 0 {
		depth = 0
	}
	if depth > 10 {
		depth = 10
	}
	seed := body.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &preparedSearch{
		game:       g,
		gameID:     gameID,
		position:   pos,
		limit:      limit,
		difficulty: difficulty,
		options: game.SearchOptions{
			Difficulty: difficulty,
			MaxDepth:   depth,
			TimeLimit:  limit,
			Seed:       seed,
		},
	}, nil
}

func (c *Controller) move(req *web.Request) (any, error) {
	body, err := bindMoveRequest(req)
	if err != nil {
		return fail(req, http.StatusBadRequest, game.CodeBadRequest, err.Error())
	}
	prep, err := c.prepareSearch(req, body)
	if err != nil {
		return failErr(req, err)
	}
	searchCtx, cancel := context.WithTimeout(req.Ctx(), prep.limit+1500*time.Millisecond)
	defer cancel()
	result, err := prep.game.Search(searchCtx, prep.position, prep.options)
	if err != nil {
		return failErr(req, err)
	}
	if result.Move == nil && !result.GameOver {
		return fail(req, http.StatusGatewayTimeout, game.CodeTimeout, "思考超时，请提高 timeLimitMs 或降低 maxDepth")
	}
	// The API is stateless: return only the chosen move, its evaluation and the
	// resulting status. The caller keeps the board and applies the move itself.
	result.Position = nil
	result.Game = prep.gameID
	result.Difficulty = prep.difficulty
	if result.Engine == "" {
		result.Engine = "builtin"
	}
	return ok(req, result)
}

func sseSend(stream *web.SSEStream, event string, value any) bool {
	data, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return stream.Send(event, string(data)) == nil
}

func sseError(err error) errorBody {
	code := "INTERNAL_ERROR"
	message := err.Error()
	var de *game.Error
	if errors.As(err, &de) {
		code = de.Code
		message = de.Message
	}
	return errorBody{OK: false, Error: errorDetail{Code: code, Message: message}}
}

func defaultEngineName(gameID string) string {
	switch gameID {
	case "chess":
		return "chess-3"
	case "xiangqi":
		return "godogpaw"
	}
	return "builtin"
}

// moveStream streams iterative-deepening progress over SSE and ends with a
// result event. POST carries a normal JSON body. GET supports EventSource:
//
//	GET /api/v1/move/stream?game=xiangqi&fen=...&difficulty=master
//
// For large grid boards, use POST with fetch() or put the whole request into the
// URL-encoded "payload" query parameter.
func (c *Controller) moveStream(req *web.Request) (any, error) {
	body, err := bindMoveRequest(req)
	if err != nil {
		return fail(req, http.StatusBadRequest, game.CodeBadRequest, err.Error())
	}
	prep, err := c.prepareSearch(req, body)
	if err != nil {
		return failErr(req, err)
	}
	return &web.SSEResponse{Handler: func(stream *web.SSEStream) error {
		stream.SetHeader("X-Accel-Buffering", "no")
		stream.StartHeartbeat(5 * time.Second)
		_ = stream.SendRetry(3000)

		start := time.Now()
		view := prep.game.Encode(prep.position, false)
		if !sseSend(stream, "start", map[string]any{
			"ok":          true,
			"game":        prep.gameID,
			"engine":      defaultEngineName(prep.gameID),
			"difficulty":  prep.difficulty,
			"timeLimitMs": prep.limit.Milliseconds(),
			"maxDepth":    prep.options.MaxDepth,
			"turn":        view.Turn,
		}) {
			return nil
		}

		progress := make(chan game.SearchProgress, 64)
		options := prep.options
		options.Progress = func(p game.SearchProgress) {
			select {
			case progress <- p:
			default:
				// The client is slower than the search; keep the search fast.
			}
		}
		searchCtx, cancel := context.WithTimeout(stream.Context(), prep.limit+2*time.Second)
		defer cancel()

		type searchDone struct {
			result game.SearchResult
			err    error
		}
		done := make(chan searchDone, 1)
		go func() {
			result, err := prep.game.Search(searchCtx, prep.position, options)
			done <- searchDone{result: result, err: err}
		}()

		for {
			select {
			case <-stream.Done():
				cancel()
				return nil
			case p := <-progress:
				if !sseSend(stream, "depth", p) {
					cancel()
					return nil
				}
			case out := <-done:
				if out.err != nil {
					_ = sseSend(stream, "error", sseError(out.err))
					_ = sseSend(stream, "done", map[string]any{"ok": false, "timeMs": time.Since(start).Milliseconds()})
					return nil
				}
				result := out.result
				if result.Move == nil && !result.GameOver {
					_ = sseSend(stream, "error", errorBody{
						OK:    false,
						Error: errorDetail{Code: game.CodeTimeout, Message: "思考超时，请提高 timeLimitMs 或降低 maxDepth"},
					})
					_ = sseSend(stream, "done", map[string]any{"ok": false, "timeMs": time.Since(start).Milliseconds()})
					return nil
				}
				result.Position = nil
				result.Game = prep.gameID
				result.Difficulty = prep.difficulty
				if result.Engine == "" {
					result.Engine = "builtin"
				}
				if !sseSend(stream, "result", result) {
					return nil
				}
				_ = sseSend(stream, "done", map[string]any{
					"ok":       true,
					"timeMs":   time.Since(start).Milliseconds(),
					"depth":    result.Search.Depth,
					"nodes":    result.Search.Nodes,
					"gameOver": result.GameOver,
				})
				return nil
			}
		}
	}}, nil
}

type applyMoveRequest struct {
	Game      string          `json:"game,omitempty"`
	Position  *positionInput  `json:"position,omitempty"`
	FEN       string          `json:"fen,omitempty"`
	Board     []string        `json:"board,omitempty"`
	Turn      string          `json:"turn,omitempty"`
	Move      *game.MoveInput `json:"move,omitempty"`
	UCI       string          `json:"uci,omitempty"`
	From      *int            `json:"from,omitempty"`
	To        *int            `json:"to,omitempty"`
	Index     *int            `json:"index,omitempty"`
	Promotion string          `json:"promotion,omitempty"`
}

func (r *applyMoveRequest) position() game.PositionInput {
	in := game.PositionInput{FEN: r.FEN, Board: r.Board, Turn: r.Turn}
	if r.Position != nil {
		if r.Position.FEN != "" {
			in.FEN = r.Position.FEN
		}
		if len(r.Position.Board) > 0 {
			in.Board = r.Position.Board
		}
		if r.Position.Turn != "" {
			in.Turn = r.Position.Turn
		}
	}
	return in
}

func (r *applyMoveRequest) move() game.MoveInput {
	var in game.MoveInput
	if r.Move != nil {
		in = *r.Move
	}
	if r.UCI != "" {
		in.UCI = r.UCI
	}
	if r.From != nil {
		in.From = r.From
	}
	if r.To != nil {
		in.To = r.To
	}
	if r.Index != nil {
		in.Index = r.Index
	}
	if r.Promotion != "" {
		in.Promotion = r.Promotion
	}
	return in
}

type applyMoveData struct {
	Game     string            `json:"game"`
	Move     game.Move         `json:"move"`
	Position game.PositionView `json:"position"`
}

func (c *Controller) applyMove(req *web.Request) (any, error) {
	var body applyMoveRequest
	if err := req.BindJSON(&body); err != nil {
		return fail(req, http.StatusBadRequest, game.CodeBadRequest, "请求体不是合法 JSON："+err.Error())
	}
	g, gameID, err := c.resolve(req, body.Game)
	if err != nil {
		return failErr(req, err)
	}
	pos, err := g.Decode(body.position())
	if err != nil {
		return failErr(req, err)
	}
	next, applied, err := g.Apply(pos, body.move())
	if err != nil {
		return failErr(req, err)
	}
	return ok(req, applyMoveData{
		Game:     gameID,
		Move:     applied,
		Position: g.Encode(next, true),
	})
}

type legalMovesRequest struct {
	Game     string         `json:"game,omitempty"`
	Position *positionInput `json:"position,omitempty"`
	FEN      string         `json:"fen,omitempty"`
	Board    []string       `json:"board,omitempty"`
	Turn     string         `json:"turn,omitempty"`
}

func (r *legalMovesRequest) position() game.PositionInput {
	in := game.PositionInput{FEN: r.FEN, Board: r.Board, Turn: r.Turn}
	if r.Position != nil {
		if r.Position.FEN != "" {
			in.FEN = r.Position.FEN
		}
		if len(r.Position.Board) > 0 {
			in.Board = r.Position.Board
		}
		if r.Position.Turn != "" {
			in.Turn = r.Position.Turn
		}
	}
	return in
}

type legalMovesData struct {
	Game  string            `json:"game"`
	Count int               `json:"count"`
	View  game.PositionView `json:"position"`
}

func (c *Controller) legalMoves(req *web.Request) (any, error) {
	var body legalMovesRequest
	if err := req.BindJSON(&body); err != nil {
		return fail(req, http.StatusBadRequest, game.CodeBadRequest, "请求体不是合法 JSON："+err.Error())
	}
	g, gameID, err := c.resolve(req, body.Game)
	if err != nil {
		return failErr(req, err)
	}
	pos, err := g.Decode(body.position())
	if err != nil {
		return failErr(req, err)
	}
	view := g.Encode(pos, true)
	return ok(req, legalMovesData{Game: gameID, Count: view.LegalMoveCount, View: view})
}

func (c *Controller) renderPage(req *web.Request, status int, page string, data webui.PageData) (any, error) {
	if c.Renderer == nil {
		return fail(req, http.StatusInternalServerError, "INTERNAL_ERROR", "页面渲染器未初始化")
	}
	lang := pageLang(req)
	data.Lang = lang
	data.OtherLang = webui.OtherLang(lang)
	data.RelBase = webui.RelativeBase(req.URL().Path, c.Renderer.BasePath())
	langURL := *req.URL()
	if contextPath := c.Renderer.BasePath(); contextPath != "" {
		langURL.Path = strings.TrimPrefix(langURL.Path, contextPath)
	}
	langPath := strings.TrimPrefix(langURL.Path, "/")
	if langPath == "" {
		langPath = "./"
	} else {
		langPath = data.RelBase + langPath
	}
	langURL.Path = langPath
	data.LangURL = webui.LangSwitchURL(&langURL, lang)
	var buf bytes.Buffer
	if err := c.Renderer.Render(&buf, page, data); err != nil {
		log.Error("render page", zap.String("page", page), zap.Error(err))
		return fail(req, http.StatusInternalServerError, "INTERNAL_ERROR", "页面渲染失败")
	}
	req.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	req.Response().Header().Set("Cache-Control", "no-cache")
	req.Response().Header().Set("Content-Language", lang)
	req.Response().WriteHeader(status)
	_, _ = req.Response().Write(buf.Bytes())
	return nil, nil
}

func pageLang(req *web.Request) string {
	lang := webui.NormalizeLang(req.Cookie().Get("auto-chess-lang"))
	if queryLang := req.Query("lang"); queryLang != "" {
		lang = webui.NormalizeLang(queryLang)
		req.Cookie().Set("auto-chess-lang", lang, web.WithPath("/"), web.WithSameSite(http.SameSiteLaxMode))
	}
	return lang
}

func (c *Controller) homePage(req *web.Request) (any, error) {
	return c.renderPage(req, http.StatusOK, "home", webui.PageData{
		Title:       "auto-chess · AI 走子接口",
		Description: "无状态 AI 走子接口：传入当前棋盘，返回下一步最佳落子。支持中国象棋、国际象棋、五子棋、黑白棋、四子棋、斗兽棋、中国跳棋。",
		Active:      "home",
		Page:        "home",
		Games:       c.Registry.List(),
	})
}

func (c *Controller) guidePage(req *web.Request) (any, error) {
	return c.renderPage(req, http.StatusOK, "guide", webui.PageData{
		Title:       "快速开始 · auto-chess",
		Description: "三步接入无状态 AI 走子接口：读取棋种、提交当前局面、应用返回走法。",
		Active:      "guide",
		Page:        "guide",
	})
}

func (c *Controller) apiPage(req *web.Request) (any, error) {
	return c.renderPage(req, http.StatusOK, "api", webui.PageData{
		Title:       "接口参考 · auto-chess",
		Description: "auto-chess 接口参考：请求字段、响应字段、走法对象、错误码与 CORS。",
		Active:      "docs",
		Page:        "api",
		Games:       c.Registry.List(),
	})
}

func (c *Controller) streamPage(req *web.Request) (any, error) {
	return c.renderPage(req, http.StatusOK, "stream", webui.PageData{
		Title:       "SSE 流式思考 · auto-chess",
		Description: "通过 SSE 实时观察 AI 思考过程：深度、评分、节点数与当前最佳走法。",
		Active:      "stream",
		Page:        "stream",
	})
}

func (c *Controller) gamesPage(req *web.Request) (any, error) {
	return c.renderPage(req, http.StatusOK, "games", webui.PageData{
		Title:       "支持的棋种 · auto-chess",
		Description: "auto-chess 支持的七种棋类、棋盘尺寸、局面格式与走法格式。",
		Active:      "games",
		Page:        "games",
		Games:       c.Registry.List(),
	})
}

func (c *Controller) gamePage(req *web.Request) (any, error) {
	id := req.Param("game")
	meta, found := c.Registry.Meta(id)
	if !found {
		return c.renderPage(req, http.StatusNotFound, "notfound", webui.PageData{
			Title:       "棋种不存在 · auto-chess",
			Description: "没有找到对应的棋种。",
			Active:      "games",
			Page:        "notfound",
		})
	}
	engine, _ := c.Registry.Get(meta.ID)
	return c.renderPage(req, http.StatusOK, "game", webui.PageData{
		Title:       meta.Name + " · auto-chess",
		Description: meta.Tagline,
		Active:      "games",
		Page:        "game",
		Game:        &meta,
		GameDoc:     webui.BuildGameDoc(&meta, engine, pageLang(req)),
	})
}

func (c *Controller) playgroundPage(req *web.Request) (any, error) {
	games := c.Registry.List()
	data := webui.PageData{
		Title:       "请求调试器 · auto-chess",
		Description: "在线调试 auto-chess 的 AI 走子接口，支持普通 JSON 与 SSE 流式请求。",
		Active:      "playground",
		Page:        "playground",
		Games:       games,
	}
	data.GamesJSON = webui.JSONScript(games)
	return c.renderPage(req, http.StatusOK, "playground", data)
}

func (c *Controller) playPage(req *web.Request) (any, error) {
	games := c.Registry.List()
	data := webui.PageData{
		Title:       "在线对弈 · auto-chess",
		Description: "直接在浏览器里和内置引擎对弈：中国象棋、国际象棋、五子棋、黑白棋、四子棋、斗兽棋、中国跳棋。",
		Active:      "play",
		Page:        "play",
		Games:       games,
	}
	data.GamesJSON = webui.JSONScript(games)
	return c.renderPage(req, http.StatusOK, "play", data)
}

func (c *Controller) asset(name, contentType string) web.HandlerFunc {
	return func(req *web.Request) (any, error) {
		if c.Renderer == nil {
			return fail(req, http.StatusInternalServerError, "INTERNAL_ERROR", "静态资源未初始化")
		}
		data, err := c.Renderer.Asset(name)
		if err != nil {
			return fail(req, http.StatusNotFound, "NOT_FOUND", "资源不存在")
		}
		req.Response().Header().Set("Content-Type", contentType)
		req.Response().Header().Set("Cache-Control", "no-cache")
		_, _ = req.Response().Write(data)
		return nil, nil
	}
}

// CORSFilter allows browser based clients to call the API.
type CORSFilter struct{}

// Init implements core.IService.
func (f *CORSFilter) Init(ctx *core.Context) error { return nil }

// Handle implements web.Filter.
func (f *CORSFilter) Handle(fc web.FilterChain, req *web.Request) (any, error) {
	h := req.Response().Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin, Cache-Control, Last-Event-ID")
	h.Set("Access-Control-Expose-Headers", "Content-Length")
	h.Set("Access-Control-Max-Age", "86400")
	h.Set("Vary", "Origin")
	if req.Request().Method == http.MethodOptions {
		req.Response().WriteStatus(http.StatusNoContent)
		return nil, nil
	}
	return fc.Next()
}

// RecoveryFilter turns a panic into a JSON 500 instead of dropping the
// connection. It is intentionally thin: the search engines are pure functions,
// so a panic here is a bug worth logging with its stack.
type RecoveryFilter struct{}

// Init implements core.IService.
func (f *RecoveryFilter) Init(ctx *core.Context) error { return nil }

// Handle implements web.Filter.
func (f *RecoveryFilter) Handle(fc web.FilterChain, req *web.Request) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if recovered == http.ErrAbortHandler {
				panic(recovered)
			}
			log.Error("api panic", zap.Any("panic", recovered), zap.ByteString("stack", debug.Stack()))
			if !req.Response().Written() {
				req.Response().Header().Set("Content-Type", "application/json; charset=utf-8")
				req.Response().JSON(http.StatusInternalServerError, errorBody{
					OK:    false,
					Error: errorDetail{Code: "INTERNAL_ERROR", Message: "服务内部错误"},
				})
			} else {
				req.Response().Abort()
			}
			result, err = nil, nil
		}
	}()
	return fc.Next()
}
