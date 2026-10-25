package feedback

import (
	crand "crypto/rand"
	"errors"
	"fmt"
	"html"
	"math/big"
	mrand "math/rand/v2"
	"strings"
	"sync"
	"time"
)

const (
	// CaptchaWidth and CaptchaHeight are the SVG image dimensions in pixels.
	CaptchaWidth  = 360
	CaptchaHeight = 208
	// CaptchaCount is the number of characters a visitor must click, in order.
	CaptchaCount = 3

	captchaTileCount   = 8
	captchaTTL         = 5 * time.Minute
	captchaMaxPending  = 5000
	captchaMargin      = 8
	captchaGap         = 8
	captchaPromptBand  = 36
	captchaCols        = 4
	captchaRows        = 2
	captchaTileW       = (CaptchaWidth - 2*captchaMargin - (captchaCols-1)*captchaGap) / captchaCols
	captchaTileH       = (CaptchaHeight - captchaPromptBand - 2*captchaMargin - (captchaRows-1)*captchaGap) / captchaRows
	captchaFontFamily  = "'KaiTi','STKaiti','Noto Serif SC','Songti SC',serif"
	captchaTileFillA   = "#f9f1e0"
	captchaTileFillB   = "#f4ead4"
	captchaTileStroke  = "#d6c39d"
	captchaPromptFill  = "#4a3a22"
	captchaPromptBandC = "#e8d9b9"
	captchaPaper       = "#f3e9d2"
)

var (
	// ErrCaptchaExpired means the challenge id is unknown, already used or past
	// its lifetime. Challenges are always consumed by a verification attempt.
	ErrCaptchaExpired = errors.New("captcha expired or already used")
	// ErrCaptchaInvalid means the clicked order does not match the challenge.
	ErrCaptchaInvalid = errors.New("captcha answer mismatch")
	// ErrCaptchaBusy means too many pending challenges are in memory.
	ErrCaptchaBusy = errors.New("too many pending captcha challenges")
)

// captchaChars is the pool of visually distinct characters shown by the
// captcha. Targets and distractors are drawn from it for every challenge.
var captchaChars = []string{
	"山", "水", "风", "雨", "云", "花", "木", "火", "日", "月",
	"天", "地", "人", "心", "明", "光", "石", "田", "春", "秋",
	"江", "海", "林", "鸟", "鱼", "龙", "马", "羊", "星", "草",
}

var captchaGlyphColors = []string{
	"#26384a", "#3d4a2c", "#593326", "#1f3d55", "#4a3a24", "#31443d",
}

var captchaNoiseColors = []string{
	"#8f7a57", "#6f7f65", "#9a6d55", "#5d7180", "#7c6a4a",
}

// CaptchaTile is the clickable rectangle for one character. Only the geometry
// is serialized; the character itself is rendered into the image.
type CaptchaTile struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`

	char     string
	rotation float64
	fontSize int
	color    string
}

// ChallengeView is the public part of a captcha challenge. It deliberately
// omits the answer and the characters at each tile: those are only available
// in the image.
type ChallengeView struct {
	ID        string        `json:"id"`
	Count     int           `json:"count"`
	Width     int           `json:"width"`
	Height    int           `json:"height"`
	Prompt    string        `json:"prompt"`
	ExpiresIn int           `json:"expiresIn"`
	Tiles     []CaptchaTile `json:"tiles"`
}

type challenge struct {
	view    ChallengeView
	targets []string
	answer  []int
	expires time.Time
	lang    string
}

// CaptchaManager keeps pending challenges in memory. The service is a single
// process, so a process-local store is enough; challenges are short lived and
// consumed by the first verification attempt.
type CaptchaManager struct {
	mu    sync.Mutex
	items map[string]*challenge
}

// NewCaptchaManager returns an empty challenge store.
func NewCaptchaManager() *CaptchaManager {
	return &CaptchaManager{items: make(map[string]*challenge)}
}

// Issue creates a new challenge and returns the data the browser needs to draw
// the click overlays and prompt.
func (m *CaptchaManager) Issue(lang string) (*ChallengeView, error) {
	lang = normalizeLang(lang)
	chars := pickChars(captchaTileCount)
	order := shuffleRange(captchaTileCount)[:CaptchaCount]
	targets := make([]string, CaptchaCount)
	answer := make([]int, CaptchaCount)
	for i, idx := range order {
		targets[i] = chars[idx]
		answer[i] = idx
	}

	tiles := make([]CaptchaTile, captchaTileCount)
	for i, char := range chars {
		col, row := i%captchaCols, i/captchaCols
		tiles[i] = CaptchaTile{
			X:        captchaMargin + col*(captchaTileW+captchaGap),
			Y:        captchaPromptBand + captchaMargin + row*(captchaTileH+captchaGap),
			W:        captchaTileW,
			H:        captchaTileH,
			char:     char,
			rotation: float64(secureInt(31) - 15),
			fontSize: 32 + secureInt(7),
			color:    captchaGlyphColors[secureInt(len(captchaGlyphColors))],
		}
	}

	ch := &challenge{
		view: ChallengeView{
			ID:        crand.Text(),
			Count:     CaptchaCount,
			Width:     CaptchaWidth,
			Height:    CaptchaHeight,
			Prompt:    viewPrompt(lang, CaptchaCount),
			ExpiresIn: int(captchaTTL / time.Second),
			Tiles:     tiles,
		},
		targets: targets,
		answer:  answer,
		expires: time.Now().Add(captchaTTL),
		lang:    lang,
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now())
	if len(m.items) >= captchaMaxPending {
		return nil, ErrCaptchaBusy
	}
	m.items[ch.view.ID] = ch
	view := ch.view
	return &view, nil
}

// Verify consumes the challenge and checks the clicked tile indices. The
// challenge is deleted even when the answer is wrong, so a challenge cannot be
// used for repeated guessing.
func (m *CaptchaManager) Verify(id string, answer []int) error {
	m.mu.Lock()
	ch, ok := m.items[id]
	if ok {
		delete(m.items, id)
	}
	m.mu.Unlock()
	if !ok || time.Now().After(ch.expires) {
		return ErrCaptchaExpired
	}
	if len(answer) != len(ch.answer) {
		return ErrCaptchaInvalid
	}
	for i := range answer {
		if answer[i] != ch.answer[i] {
			return ErrCaptchaInvalid
		}
	}
	return nil
}

// Render returns the SVG image for a pending challenge.
func (m *CaptchaManager) Render(id string) ([]byte, error) {
	m.mu.Lock()
	ch, ok := m.items[id]
	m.mu.Unlock()
	if !ok || time.Now().After(ch.expires) {
		return nil, ErrCaptchaExpired
	}
	return renderCaptcha(ch), nil
}

func (m *CaptchaManager) pruneLocked(now time.Time) {
	for id, ch := range m.items {
		if now.After(ch.expires) {
			delete(m.items, id)
		}
	}
}

func renderCaptcha(ch *challenge) []byte {
	var b strings.Builder
	b.Grow(4096)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`,
		CaptchaWidth, CaptchaHeight, CaptchaWidth, CaptchaHeight)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`, CaptchaWidth, CaptchaHeight, captchaPaper)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`, CaptchaWidth, captchaPromptBand, captchaPromptBandC)

	// Background scratches make the image harder to read mechanically without
	// hurting the characters themselves.
	for i := 0; i < 7; i++ {
		x1, y1 := secureInt(CaptchaWidth), secureInt(CaptchaHeight)
		cx, cy := secureInt(CaptchaWidth), secureInt(CaptchaHeight)
		x2, y2 := secureInt(CaptchaWidth), secureInt(CaptchaHeight)
		fmt.Fprintf(&b, `<path d="M%d %d Q%d %d %d %d" fill="none" stroke="%s" stroke-width="%d" opacity="0.16"/>`,
			x1, y1, cx, cy, x2, y2, captchaNoiseColors[secureInt(len(captchaNoiseColors))], 1+secureInt(2))
	}
	for i := 0; i < 36; i++ {
		fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%.1f" fill="%s" opacity="0.14"/>`,
			secureInt(CaptchaWidth), secureInt(CaptchaHeight), 0.8+float64(secureInt(20))/10,
			captchaNoiseColors[secureInt(len(captchaNoiseColors))])
	}

	prompt := promptText(ch.lang, ch.targets)
	fmt.Fprintf(&b, `<text id="prompt" x="%d" y="23" text-anchor="middle" font-family="%s" font-size="14" font-weight="600" fill="%s">%s</text>`,
		CaptchaWidth/2, captchaFontFamily, captchaPromptFill, html.EscapeString(prompt))

	for i, tile := range ch.view.Tiles {
		fill := captchaTileFillA
		if i%2 == 1 {
			fill = captchaTileFillB
		}
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" rx="8" fill="%s" stroke="%s"/>`,
			tile.X, tile.Y, tile.W, tile.H, fill, captchaTileStroke)
	}

	b.WriteString(`<g id="chars" font-family="` + captchaFontFamily + `">`)
	for _, tile := range ch.view.Tiles {
		cx := tile.X + tile.W/2 + secureInt(9) - 4
		cy := tile.Y + tile.H/2 + secureInt(7) - 3
		baseline := cy + tile.fontSize*36/100
		fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" font-size="%d" font-weight="600" fill="%s" transform="rotate(%.1f %d %d)">%s</text>`,
			cx, baseline, tile.fontSize, tile.color, tile.rotation, cx, cy, html.EscapeString(tile.char))
	}
	b.WriteString(`</g>`)

	// A few foreground specks cross the glyphs.
	for i := 0; i < 14; i++ {
		fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%.1f" fill="%s" opacity="0.18"/>`,
			secureInt(CaptchaWidth), secureInt(CaptchaHeight), 0.6+float64(secureInt(14))/10,
			captchaNoiseColors[secureInt(len(captchaNoiseColors))])
	}
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

func promptText(lang string, targets []string) string {
	if lang == "en" {
		return "Click in order: " + strings.Join(targets, " ")
	}
	return "请依次点击：" + strings.Join(targets, " ")
}

func viewPrompt(lang string, count int) string {
	if lang == "en" {
		return fmt.Sprintf("Click the %d characters shown in the image, in order", count)
	}
	return fmt.Sprintf("请按图片中的提示，依次点击 %d 个字", count)
}

func normalizeLang(value string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "en") {
		return "en"
	}
	return "zh"
}

func pickChars(n int) []string {
	pool := append([]string(nil), captchaChars...)
	for i := len(pool) - 1; i > 0; i-- {
		j := secureInt(i + 1)
		pool[i], pool[j] = pool[j], pool[i]
	}
	return pool[:n]
}

func shuffleRange(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	for i := n - 1; i > 0; i-- {
		j := secureInt(i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func secureInt(n int) int {
	if n <= 1 {
		return 0
	}
	v, err := crand.Int(crand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return mrand.IntN(n)
	}
	return int(v.Int64())
}
