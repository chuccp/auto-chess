// Package webui provides the server-rendered multi-page documentation site.
package webui

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"regexp"
	"strings"

	"github.com/chuccp/auto-chess/internal/game"
)

//go:embed templates/*.html templates/partials/*.html static/*
var assets embed.FS

// PageData is the shared data model for every page.
type PageData struct {
	Title       string
	Description string
	Active      string
	Page        string
	Version     string
	RelBase     string
	Games       []game.Meta
	Game        *game.Meta
	GameDoc     *GameDoc
	Lang        string
	OtherLang   string
	LangURL     string
	GamesJSON   template.JS
	Body        template.HTML
}

// Renderer owns the parsed template set and embedded assets.
type Renderer struct {
	tpl      *template.Template
	basePath string
}

// New parses every embedded template. The optional basePath is the context
// path configured for the server, such as "/auto-chess".
func New(basePath ...string) (*Renderer, error) {
	r := &Renderer{basePath: normalizeBasePath(firstString(basePath))}
	funcs := template.FuncMap{
		"icon":        gameIcon,
		"compactJSON": compactJSON,
		"example":     ExampleRequest,
		"join":        strings.Join,
		"first": func(n int, items []string) []string {
			if len(items) <= n {
				return items
			}
			return items[:n]
		},
	}
	tpl, err := template.New("site").Funcs(funcs).ParseFS(assets,
		"templates/*.html",
		"templates/partials/*.html",
	)
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	r.tpl = tpl
	return r, nil
}

// BasePath returns the normalized context path, without a trailing slash.
func (r *Renderer) BasePath() string {
	return r.basePath
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func normalizeBasePath(base string) string {
	base = strings.TrimSpace(base)
	if base == "" || base == "/" {
		return ""
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	return strings.TrimRight(base, "/")
}

// Render executes a page template and then the shared layout.
func (r *Renderer) Render(w io.Writer, page string, data PageData) error {
	if data.Version == "" {
		data.Version = "0.1.0"
	}
	if data.Lang == "" {
		data.Lang = "zh"
	}
	if data.RelBase == "" {
		data.RelBase = "./"
	}
	var body bytes.Buffer
	if err := r.tpl.ExecuteTemplate(&body, page, data); err != nil {
		return fmt.Errorf("render page %s: %w", page, err)
	}
	data.Body = template.HTML(body.String())
	var full bytes.Buffer
	if err := r.tpl.ExecuteTemplate(&full, "layout", data); err != nil {
		return fmt.Errorf("render layout for %s: %w", page, err)
	}
	rendered := relativizeHTML(full.String(), data.RelBase)
	if NormalizeLang(data.Lang) == "en" {
		_, err := io.WriteString(w, LocalizeHTML(rendered))
		return err
	}
	_, err := io.WriteString(w, rendered)
	return err
}

// relativizeHTML turns root-relative page links into paths relative to the
// current page, so the site keeps working when the server context path changes.
func relativizeHTML(rendered, relBase string) string {
	if relBase == "" {
		relBase = "./"
	}
	rendered = strings.ReplaceAll(rendered, `href="/`, `href="`+relBase)
	rendered = strings.ReplaceAll(rendered, `src="/`, `src="`+relBase)
	rendered = strings.ReplaceAll(rendered, `action="/`, `action="`+relBase)
	return rendered
}

// RelativeBase returns the prefix that points from a page path back to the
// application root, for example "./" for /play and "../" for /games/xiangqi.
func RelativeBase(pagePath, contextPath string) string {
	path := pagePath
	if contextPath = normalizeBasePath(contextPath); contextPath != "" {
		switch {
		case path == contextPath:
			path = ""
		case strings.HasPrefix(path, contextPath+"/"):
			path = strings.TrimPrefix(path, contextPath)
		}
	}
	trailing := strings.HasSuffix(path, "/")
	path = strings.Trim(path, "/")
	segments := 0
	if path != "" {
		segments = strings.Count(path, "/") + 1
		if trailing {
			segments++
		}
	}
	up := segments - 1
	if up <= 0 {
		return "./"
	}
	return strings.Repeat("../", up)
}

// Asset reads an embedded static file.
func (r *Renderer) Asset(name string) ([]byte, error) {
	return assets.ReadFile("static/" + name)
}

// JSONScript safely embeds a JSON value in a script element.
func JSONScript(v any) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return template.JS("[]")
	}
	// Prevent a composed string from closing the script element.
	s := strings.ReplaceAll(string(b), "</", "<\\/")
	return template.JS(s)
}

// ExampleRequest builds a ready-to-edit request body for a game page.
func ExampleRequest(m *game.Meta) string {
	if m == nil {
		return ""
	}
	req := map[string]any{
		"game":        m.ID,
		"difficulty":  "normal",
		"timeLimitMs": 1000,
	}
	if m.Board.Type == "fen" {
		req["fen"] = m.InitialPosition.FEN
	} else {
		req["board"] = m.InitialPosition.Board
		req["turn"] = m.InitialPosition.Turn
	}
	return prettyJSON(req)
}

func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

var primitiveArray = regexp.MustCompile(`(?s)\[\n\s+((?:"(?:[^"\\]|\\.)*"|-?\d+(?:\.\d+)?|true|false|null)(?:,\n\s+(?:"(?:[^"\\]|\\.)*"|-?\d+(?:\.\d+)?|true|false|null))*)\n\s+\]`)
var manySpaces = regexp.MustCompile(`[ \t]+`)

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return primitiveArray.ReplaceAllStringFunc(string(b), func(s string) string {
		inner := primitiveArray.FindStringSubmatch(s)[1]
		inner = strings.ReplaceAll(inner, "\n", " ")
		inner = manySpaces.ReplaceAllString(inner, " ")
		return "[" + inner + "]"
	})
}

func gameIcon(id string) string {
	switch id {
	case "chess":
		return "♞"
	case "xiangqi":
		return "帥"
	case "gomoku":
		return "五"
	case "reversi":
		return "◐"
	case "connect4":
		return "⬤"
	case "jungle":
		return "獸"
	case "checkers":
		return "✦"
	}
	return "◆"
}
