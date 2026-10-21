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
	t.Helper()
	cfg := config.LoadAutoConfig()
	cfg.Put("web.server.port", 19009)
	renderer, err := webui.New()
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
	for _, want := range []string{`data-theme="light"`, `id="theme-toggle"`, `class="lang-btn"`, `href="/?lang=en"`} {
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
	for _, want := range []string{`<html lang="en"`, "AI Move API", `href="/?lang=zh"`} {
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
