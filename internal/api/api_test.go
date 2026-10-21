package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/chuccp/auto-chess/internal/api"
	"github.com/chuccp/auto-chess/internal/game"
	"github.com/chuccp/auto-chess/internal/registry"
	"github.com/chuccp/auto-chess/internal/webui"
	wf "github.com/chuccp/go-web-frame"
	"github.com/chuccp/go-web-frame/config"
)

// The framework's test handler dispatches by the Host header's port, so all
// test requests target the configured server port while the socket itself is a
// random httptest port.
const testPort = "19009"

func newTestServer(t *testing.T) *httptest.Server {
	return newTestServerWithContext(t, "")
}

func newTestServerWithContext(t *testing.T, contextPath string) *httptest.Server {
	t.Helper()
	cfg := config.LoadAutoConfig()
	cfg.Put("web.server.port", 19009)
	if contextPath != "" {
		cfg.Put("web.server.contextPath", contextPath)
	}
	renderer, err := webui.New(contextPath)
	if err != nil {
		t.Fatal(err)
	}
	app := wf.NewBuilder(cfg).
		Filter(&api.CORSFilter{}).
		Rest(&api.Controller{Registry: registry.New(), Renderer: renderer}).
		Build()
	handler := app.GetHandler()
	if handler == nil {
		t.Fatal("GetHandler returned nil")
	}
	return httptest.NewServer(handler)
}

func doRequest(t *testing.T, method, url string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Host = "127.0.0.1:" + testPort
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func postJSON(t *testing.T, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	res := doRequest(t, http.MethodPost, url, bytes.NewReader(raw))
	defer res.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return res, out
}

func TestHealthAndGames(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	res := doRequest(t, http.MethodGet, srv.URL+"/api/v1/health", nil)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("health status = %d", res.StatusCode)
	}
	res = doRequest(t, http.MethodGet, srv.URL+"/api/v1/games", nil)
	defer res.Body.Close()
	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			Count int `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Data.Count != 7 {
		t.Fatalf("games response = %+v", body)
	}
}

func TestMoveXiangqi(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	body := map[string]any{
		"game":        "xiangqi",
		"fen":         "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
		"difficulty":  "easy",
		"timeLimitMs": 300,
	}
	res, out := postJSON(t, srv.URL+"/api/v1/move", body)
	if res.StatusCode != 200 || out["ok"] != true {
		t.Fatalf("move response: status=%d body=%v", res.StatusCode, out)
	}
	data := out["data"].(map[string]any)
	move := data["move"].(map[string]any)
	if move["uci"] == "" || move["uci"] == nil {
		t.Fatalf("missing uci in move: %v", move)
	}
	if data["engine"] != "godogpaw" {
		t.Fatalf("engine = %v, want godogpaw", data["engine"])
	}
	if _, hasPosition := data["position"]; hasPosition {
		t.Fatalf("stateless response must not include a position: %v", data)
	}
}

func TestApplyMove(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	res, out := postJSON(t, srv.URL+"/api/v1/apply-move", map[string]any{
		"game": "xiangqi",
		"fen":  "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
		"move": map[string]any{"uci": "h2e2"},
	})
	if res.StatusCode != 200 || out["ok"] != true {
		t.Fatalf("apply response: status=%d body=%v", res.StatusCode, out)
	}
	data := out["data"].(map[string]any)
	applied := data["move"].(map[string]any)
	if applied["uci"] != "h2e2" {
		t.Fatalf("applied move = %v", applied)
	}
	position := data["position"].(map[string]any)
	if position["turn"] != "b" {
		t.Fatalf("turn after apply = %v, want b", position["turn"])
	}
	if position["fen"] == "" || position["fen"] == nil {
		t.Fatalf("missing fen after apply: %v", position)
	}
	if moves, ok := position["legalMoves"].([]any); !ok || len(moves) == 0 {
		t.Fatalf("missing legalMoves after apply: %v", position["legalMoves"])
	}
}

func TestMoveErrorsAndAlias(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	res, out := postJSON(t, srv.URL+"/api/v1/games/chinese-chess/move", map[string]any{
		"fen":        "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
		"difficulty": "easy",
	})
	if res.StatusCode != 200 {
		t.Fatalf("alias move status = %d: %v", res.StatusCode, out)
	}
	if got := out["data"].(map[string]any)["game"]; got != "xiangqi" {
		t.Fatalf("alias response game = %v, want xiangqi", got)
	}

	res, out = postJSON(t, srv.URL+"/api/v1/move", map[string]any{"game": "nope", "fen": "x"})
	if res.StatusCode != 404 {
		t.Fatalf("unknown game status = %d, want 404", res.StatusCode)
	}
	errObj := out["error"].(map[string]any)
	if errObj["code"] != "UNKNOWN_GAME" {
		t.Fatalf("unknown game code = %v", errObj["code"])
	}

	res, out = postJSON(t, srv.URL+"/api/v1/move", map[string]any{
		"game": "xiangqi",
		"fen":  "not-a-fen",
	})
	if res.StatusCode != 400 {
		t.Fatalf("invalid position status = %d, want 400", res.StatusCode)
	}
}

func TestLegalMoves(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	res, out := postJSON(t, srv.URL+"/api/v1/legal-moves", map[string]any{
		"game": "xiangqi",
		"fen":  "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
	})
	if res.StatusCode != 200 {
		t.Fatalf("legal moves status = %d", res.StatusCode)
	}
	data := out["data"].(map[string]any)
	if data["count"].(float64) != 44 {
		t.Fatalf("legal move count = %v, want 44", data["count"])
	}
}

const xiangqiStartFEN = "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1"

func legalMovesOf(t *testing.T, srv *httptest.Server, gameID, fen string) []any {
	t.Helper()
	res, out := postJSON(t, srv.URL+"/api/v1/legal-moves", map[string]any{"game": gameID, "fen": fen})
	if res.StatusCode != 200 {
		t.Fatalf("legal moves status = %d: %v", res.StatusCode, out)
	}
	data := out["data"].(map[string]any)
	position := data["position"].(map[string]any)
	moves, ok := position["legalMoves"].([]any)
	if !ok || len(moves) == 0 {
		t.Fatalf("missing legalMoves: %v", position)
	}
	return moves
}

func TestMoveForbiddenMoves(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// A placement game must not play a banned point: gomoku's opening shortcut
	// plays the center (index 115), so ban exactly that.
	res, out := postJSON(t, srv.URL+"/api/v1/move", map[string]any{
		"game":           "gomoku",
		"board":          make([]string, 225),
		"turn":           "b",
		"difficulty":     "easy",
		"timeLimitMs":    500,
		"forbiddenMoves": []map[string]any{{"index": 115}},
	})
	if res.StatusCode != 200 || out["ok"] != true {
		t.Fatalf("gomoku move response: status=%d body=%v", res.StatusCode, out)
	}
	move := out["data"].(map[string]any)["move"].(map[string]any)
	idx := move["index"].(map[string]any)["index"].(float64)
	if int(idx) == 115 {
		t.Fatalf("engine played the banned point 115: %v", move)
	}

	// Ban every legal move but one: the engine must return the survivor.
	moves := legalMovesOf(t, srv, "xiangqi", xiangqiStartFEN)
	keep := moves[0].(map[string]any)["uci"].(string)
	var banned []map[string]any
	for _, raw := range moves[1:] {
		banned = append(banned, map[string]any{"uci": raw.(map[string]any)["uci"].(string)})
	}
	res, out = postJSON(t, srv.URL+"/api/v1/move", map[string]any{
		"game":           "xiangqi",
		"fen":            xiangqiStartFEN,
		"difficulty":     "easy",
		"timeLimitMs":    300,
		"forbiddenMoves": banned,
	})
	if res.StatusCode != 200 || out["ok"] != true {
		t.Fatalf("xiangqi move response: status=%d body=%v", res.StatusCode, out)
	}
	move = out["data"].(map[string]any)["move"].(map[string]any)
	if move["uci"] != keep {
		t.Fatalf("engine move = %v, want the only un-banned %q", move["uci"], keep)
	}

	// Banning every legal move is a request error.
	all := append([]map[string]any{}, banned...)
	all = append(all, map[string]any{"uci": keep})
	res, out = postJSON(t, srv.URL+"/api/v1/move", map[string]any{
		"game":           "xiangqi",
		"fen":            xiangqiStartFEN,
		"difficulty":     "easy",
		"forbiddenMoves": all,
	})
	if res.StatusCode != 400 {
		t.Fatalf("all-banned status = %d, want 400: %v", res.StatusCode, out)
	}
	if code := out["error"].(map[string]any)["code"]; code != "BAD_REQUEST" {
		t.Fatalf("all-banned code = %v, want BAD_REQUEST", code)
	}

	// An empty entry names nothing and is rejected instead of silently ignored.
	res, out = postJSON(t, srv.URL+"/api/v1/move", map[string]any{
		"game":           "xiangqi",
		"fen":            xiangqiStartFEN,
		"forbiddenMoves": []map[string]any{{}},
	})
	if res.StatusCode != 400 {
		t.Fatalf("empty entry status = %d, want 400: %v", res.StatusCode, out)
	}
}

func TestLegalMovesForbiddenFlags(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// {"to": N} is a position ban: every move landing on N is flagged.
	moves := legalMovesOf(t, srv, "xiangqi", xiangqiStartFEN)
	first := moves[0].(map[string]any)
	toIndex := first["to"].(map[string]any)["index"].(float64)

	res, out := postJSON(t, srv.URL+"/api/v1/legal-moves", map[string]any{
		"game":           "xiangqi",
		"fen":            xiangqiStartFEN,
		"forbiddenMoves": []map[string]any{{"to": toIndex}},
	})
	if res.StatusCode != 200 {
		t.Fatalf("legal moves status = %d: %v", res.StatusCode, out)
	}
	position := out["data"].(map[string]any)["position"].(map[string]any)
	flagged := position["legalMoves"].([]any)
	sawFlag := false
	for _, raw := range flagged {
		m := raw.(map[string]any)
		isTarget := m["to"].(map[string]any)["index"].(float64) == toIndex
		if isTarget != (m["forbidden"] == true) {
			t.Fatalf("move %v forbidden flag = %v, want %v", m["uci"], m["forbidden"], isTarget)
		}
		if isTarget {
			sawFlag = true
		}
	}
	if !sawFlag {
		t.Fatalf("no move matched the banned destination %v", toIndex)
	}
}

func TestMoveTerminalPosition(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	res, out := postJSON(t, srv.URL+"/api/v1/move", map[string]any{
		"game": "chess",
		"fen":  "rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3",
	})
	if res.StatusCode != 200 {
		t.Fatalf("terminal position status = %d, want 200", res.StatusCode)
	}
	data := out["data"].(map[string]any)
	if data["gameOver"] != true {
		t.Fatalf("terminal position should report gameOver: %v", data)
	}
	if data["move"] != nil {
		t.Fatalf("terminal position should not return a move: %v", data["move"])
	}
	// Early game-over returns must still carry a complete evaluation shape.
	eval, _ := data["evaluation"].(map[string]any)
	if eval["unit"] != "centipawn" || eval["perspective"] != "w" {
		t.Fatalf("terminal evaluation shape = %v", eval)
	}
}

func TestMoveStreamEvents(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	raw, _ := json.Marshal(map[string]any{
		"game":        "xiangqi",
		"fen":         "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
		"difficulty":  "hard",
		"timeLimitMs": 700,
	})
	res := doRequest(t, http.MethodPost, srv.URL+"/api/v1/move/stream", bytes.NewReader(raw))
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("stream status = %d", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q", got)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("stream CORS origin = %q", got)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, event := range []string{"event: start", "event: depth", "event: result", "event: done"} {
		if !strings.Contains(text, event) {
			t.Fatalf("stream missing %q:\n%s", event, text)
		}
	}
	if !strings.Contains(text, `"move"`) {
		t.Fatalf("stream result has no move:\n%s", text)
	}
}

func TestMoveStreamGET(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	query := url.Values{}
	query.Set("game", "chess")
	query.Set("fen", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
	query.Set("difficulty", "easy")
	query.Set("timeLimitMs", "300")
	res := doRequest(t, http.MethodGet, srv.URL+"/api/v1/move/stream?"+query.Encode(), nil)
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET stream status = %d", res.StatusCode)
	}
	data, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(data), "event: result") {
		t.Fatalf("GET stream missing result:\n%s", data)
	}
}

func TestDefaultThemeAndLanguageSwitch(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	res := doRequest(t, http.MethodGet, srv.URL+"/", nil)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	page := string(body)
	for _, want := range []string{`data-theme="light"`, `id="theme-toggle"`, `class="lang-btn"`, `href="./?lang=en"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("default page missing %q", want)
		}
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/?lang=en", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "127.0.0.1:" + testPort
	enRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	enBody, _ := io.ReadAll(enRes.Body)
	enRes.Body.Close()
	enPage := string(enBody)
	if enRes.StatusCode != 200 {
		t.Fatalf("english page status = %d", enRes.StatusCode)
	}
	if got := enRes.Header.Get("Content-Language"); got != "en" {
		t.Fatalf("content language = %q", got)
	}
	for _, want := range []string{`<html lang="en"`, "AI Move API", `href="./?lang=zh"`} {
		if !strings.Contains(enPage, want) {
			t.Fatalf("english page missing %q", want)
		}
	}
	if strings.Contains(enPage, "只传棋盘") {
		t.Fatal("english page still contains the Chinese hero copy")
	}
}

func TestCORSAllRoutes(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	paths := []string{
		"/",
		"/guide",
		"/docs",
		"/docs/stream",
		"/games",
		"/games/xiangqi",
		"/play",
		"/playground",
		"/assets/app.css",
		"/assets/site.js",
		"/assets/play.js",
		"/assets/playground.js",
		"/api/v1/health",
		"/api/v1/games",
		"/api/v1/games/xiangqi",
		"/api/v1/move",
		"/api/v1/apply-move",
		"/api/v1/move/stream",
		"/api/v1/games/xiangqi/move",
		"/api/v1/games/xiangqi/apply-move",
		"/api/v1/games/xiangqi/move/stream",
		"/api/v1/legal-moves",
	}
	for _, path := range paths {
		req, err := http.NewRequest(http.MethodOptions, srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "127.0.0.1:" + testPort
		req.Header.Set("Origin", "https://client.example")
		req.Header.Set("Access-Control-Request-Method", "POST")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("%s OPTIONS status = %d", path, res.StatusCode)
		}
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Fatalf("%s CORS origin = %q", path, got)
		}
		if got := res.Header.Get("Access-Control-Allow-Methods"); got == "" {
			t.Fatalf("%s missing allow methods", path)
		}
	}
}

func TestServerRenderedPages(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	cases := []struct {
		path string
		want string
	}{
		{"/", "只传棋盘"},
		{"/guide", "三步接入"},
		{"/docs", "接口参考"},
		{"/docs", "按棋种查看接口"},
		{"/docs/stream", "SSE 实时思考过程"},
		{"/games", "支持的棋种"},
		{"/games/xiangqi", "中国象棋"},
		{"/games/chess", "国际象棋接口详解"},
		{"/games/gomoku", "五子棋接口详解"},
		{"/play", "直接和引擎下一盘"},
		{"/play", "play-api-mode"},
		{"/play", "SSE · 思考过程"},
		{"/play", "普通 · 一次返回"},
		{"/playground", "请求调试器"},
	}
	for _, tc := range cases {
		res := doRequest(t, http.MethodGet, srv.URL+tc.path, nil)
		data, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 {
			t.Fatalf("%s status = %d", tc.path, res.StatusCode)
		}
		if !strings.Contains(string(data), tc.want) {
			t.Fatalf("%s does not contain %q", tc.path, tc.want)
		}
		if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Fatalf("%s content type = %q", tc.path, got)
		}
	}

	res := doRequest(t, http.MethodGet, srv.URL+"/games/nope", nil)
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound || !strings.Contains(string(data), "页面不存在") {
		t.Fatalf("notfound status=%d body=%s", res.StatusCode, data)
	}

	for _, asset := range []string{"/assets/app.css", "/assets/site.js", "/assets/play.js", "/assets/playground.js"} {
		res := doRequest(t, http.MethodGet, srv.URL+asset, nil)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("%s status = %d", asset, res.StatusCode)
		}
	}
}

func TestContextPath(t *testing.T) {
	srv := newTestServerWithContext(t, "/auto-chess")
	defer srv.Close()

	res := doRequest(t, http.MethodGet, srv.URL+"/auto-chess/api/v1/health", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("prefixed health status = %d", res.StatusCode)
	}
	res = doRequest(t, http.MethodGet, srv.URL+"/api/v1/health", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("root health status = %d, want 404", res.StatusCode)
	}

	res = doRequest(t, http.MethodGet, srv.URL+"/auto-chess/play", nil)
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("prefixed play status = %d", res.StatusCode)
	}
	page := string(body)
	for _, want := range []string{
		`href="./assets/app.css`,
		`href="./play`,
		`window.__REL_BASE__`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("prefixed page does not contain %q", want)
		}
	}
	if strings.Contains(page, `href="/auto-chess`) {
		t.Fatal("prefixed page hard-codes the context path in an href")
	}
	if strings.Contains(page, "/auto-chess/auto-chess") {
		t.Fatal("prefixed page contains a double context path")
	}

	res = doRequest(t, http.MethodGet, srv.URL+"/auto-chess/assets/app.css", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("prefixed asset status = %d", res.StatusCode)
	}
}

func TestCORSAndDocs(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	res := doRequest(t, http.MethodOptions, srv.URL+"/api/v1/move", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, want 204", res.StatusCode)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("CORS origin = %q", got)
	}
	res = doRequest(t, http.MethodGet, srv.URL+"/docs", nil)
	defer res.Body.Close()
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(buf), "auto-chess") {
		t.Fatal("docs page does not contain the service name")
	}
}

func TestMoveEngineSelection(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	play := func(engine string) (int, map[string]any) {
		t.Helper()
		body := map[string]any{
			"game":        "chess",
			"fen":         "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
			"difficulty":  "easy",
			"timeLimitMs": 300,
		}
		if engine != "" {
			body["engine"] = engine
		}
		res, out := postJSON(t, srv.URL+"/api/v1/move", body)
		return res.StatusCode, out
	}

	status, out := play("builtin")
	if status != 200 || out["ok"] != true {
		t.Fatalf("builtin move: status=%d body=%v", status, out)
	}
	if got := out["data"].(map[string]any)["engine"]; got != "builtin" {
		t.Fatalf("builtin engine = %v", got)
	}

	status, out = play("chess-3")
	if status != 200 || out["ok"] != true {
		t.Fatalf("chess-3 move: status=%d body=%v", status, out)
	}
	if got := out["data"].(map[string]any)["engine"]; got != "chess-3" {
		t.Fatalf("chess-3 engine = %v", got)
	}

	status, out = play("nope")
	if out["ok"] != false {
		t.Fatalf("unknown engine should fail: status=%d body=%v", status, out)
	}
	_ = status
	errBody := out["error"].(map[string]any)
	if errBody["code"] != game.CodeUnknownEngine {
		t.Fatalf("unknown engine code = %v", errBody["code"])
	}

	// xiangqi without a loaded NNUE network must reject pikafish-nnue and
	// accept godogpaw / builtin.
	res, out := postJSON(t, srv.URL+"/api/v1/move", map[string]any{
		"game":        "xiangqi",
		"fen":         "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
		"difficulty":  "easy",
		"timeLimitMs": 300,
		"engine":      "builtin",
	})
	if res.StatusCode != 200 || out["ok"] != true {
		t.Fatalf("xiangqi builtin move: status=%d body=%v", res.StatusCode, out)
	}
	if got := out["data"].(map[string]any)["engine"]; got != "builtin" {
		t.Fatalf("xiangqi builtin engine = %v", got)
	}

	_, out = postJSON(t, srv.URL+"/api/v1/move", map[string]any{
		"game":        "xiangqi",
		"fen":         "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
		"difficulty":  "easy",
		"timeLimitMs": 300,
		"engine":      "pikafish-nnue",
	})
	if out["ok"] != false {
		t.Fatalf("missing NNUE should reject pikafish-nnue: %v", out)
	}
	if got := out["error"].(map[string]any)["code"]; got != game.CodeUnknownEngine {
		t.Fatalf("pikafish-nnue error code = %v", got)
	}

	// non-search games only accept builtin.
	_, out = postJSON(t, srv.URL+"/api/v1/move", map[string]any{
		"game":        "gomoku",
		"difficulty":  "easy",
		"timeLimitMs": 300,
		"engine":      "godogpaw",
	})
	if out["ok"] != false {
		t.Fatalf("gomoku must reject godogpaw: %v", out)
	}
	if got := out["error"].(map[string]any)["code"]; got != game.CodeUnknownEngine {
		t.Fatalf("gomoku engine error code = %v", got)
	}
}

func TestGamesListEngines(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	res := doRequest(t, http.MethodGet, srv.URL+"/api/v1/games", nil)
	defer res.Body.Close()
	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			Games []struct {
				ID      string   `json:"id"`
				Engines []string `json:"engines"`
			} `json:"games"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	byID := map[string][]string{}
	for _, g := range body.Data.Games {
		byID[g.ID] = g.Engines
	}
	if got := byID["xiangqi"]; len(got) == 0 || got[0] != "godogpaw" {
		t.Fatalf("xiangqi engines = %v, want godogpaw first", got)
	}
	if got := byID["chess"]; len(got) == 0 || got[0] != "chess-3" {
		t.Fatalf("chess engines = %v, want chess-3 first", got)
	}
	if got := byID["gomoku"]; len(got) != 1 || got[0] != "builtin" {
		t.Fatalf("gomoku engines = %v, want [builtin]", got)
	}
}

func TestApplyMoveAcceptsReturnedMove(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	position := map[string]any{
		"game": "xiangqi",
		"fen":  "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
	}
	_, legal := postJSON(t, srv.URL+"/api/v1/legal-moves", position)
	moves := legal["data"].(map[string]any)["position"].(map[string]any)["legalMoves"].([]any)
	// Send the first legal move back exactly as the API returned it.
	body := map[string]any{
		"game":     "xiangqi",
		"fen":      position["fen"],
		"move":     moves[0],
		"gameOver": false,
	}
	res, out := postJSON(t, srv.URL+"/api/v1/apply-move", body)
	if res.StatusCode != 200 || out["ok"] != true {
		t.Fatalf("applying a returned move failed: status=%d body=%v", res.StatusCode, out)
	}
}
