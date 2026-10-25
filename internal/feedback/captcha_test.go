package feedback

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCaptchaIssueVerify(t *testing.T) {
	manager := NewCaptchaManager()
	view, err := manager.Issue("zh")
	if err != nil {
		t.Fatal(err)
	}
	if view.ID == "" || view.Count != CaptchaCount || len(view.Tiles) != captchaTileCount {
		t.Fatalf("unexpected view: %+v", view)
	}
	challenge := manager.items[view.ID]
	answer := append([]int(nil), challenge.answer...)
	if err := manager.Verify(view.ID, answer); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := manager.Verify(view.ID, answer); !errors.Is(err, ErrCaptchaExpired) {
		t.Fatalf("replay error = %v, want ErrCaptchaExpired", err)
	}
}

func TestCaptchaWrongOrderConsumesChallenge(t *testing.T) {
	manager := NewCaptchaManager()
	view, err := manager.Issue("zh")
	if err != nil {
		t.Fatal(err)
	}
	challenge := manager.items[view.ID]
	wrong := append([]int(nil), challenge.answer...)
	wrong[0], wrong[1] = wrong[1], wrong[0]
	if err := manager.Verify(view.ID, wrong); !errors.Is(err, ErrCaptchaInvalid) {
		t.Fatalf("wrong order error = %v, want ErrCaptchaInvalid", err)
	}
	if err := manager.Verify(view.ID, challenge.answer); !errors.Is(err, ErrCaptchaExpired) {
		t.Fatalf("challenge was not consumed: %v", err)
	}
}

func TestCaptchaExpiry(t *testing.T) {
	manager := NewCaptchaManager()
	view, err := manager.Issue("zh")
	if err != nil {
		t.Fatal(err)
	}
	challenge := manager.items[view.ID]
	answer := append([]int(nil), challenge.answer...)
	challenge.expires = time.Now().Add(-time.Second)
	if err := manager.Verify(view.ID, answer); !errors.Is(err, ErrCaptchaExpired) {
		t.Fatalf("expired error = %v, want ErrCaptchaExpired", err)
	}
	if _, err := manager.Render(view.ID); !errors.Is(err, ErrCaptchaExpired) {
		t.Fatalf("expired render error = %v", err)
	}
}

func TestCaptchaRender(t *testing.T) {
	manager := NewCaptchaManager()
	view, err := manager.Issue("zh")
	if err != nil {
		t.Fatal(err)
	}
	svg, err := manager.Render(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	text := string(svg)
	if !strings.HasPrefix(text, "<svg") || !strings.Contains(text, `<g id="chars"`) {
		t.Fatalf("unexpected svg:\n%s", text)
	}
	if !strings.Contains(text, "请依次点击：") {
		t.Fatalf("svg missing prompt:\n%s", text)
	}
	challenge := manager.items[view.ID]
	for _, target := range challenge.targets {
		if !strings.Contains(text, target) {
			t.Fatalf("svg missing target %q", target)
		}
	}
	for _, tile := range view.Tiles {
		if tile.W <= 0 || tile.H <= 0 {
			t.Fatalf("bad tile geometry: %+v", tile)
		}
		if tile.X < 0 || tile.Y < 0 || tile.X+tile.W > CaptchaWidth || tile.Y+tile.H > CaptchaHeight {
			t.Fatalf("tile outside image: %+v", tile)
		}
	}
}

func TestCaptchaEnglishPrompt(t *testing.T) {
	manager := NewCaptchaManager()
	view, err := manager.Issue("en")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view.Prompt, "Click the 3 characters") {
		t.Fatalf("prompt = %q", view.Prompt)
	}
	svg, err := manager.Render(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(svg), "Click in order:") {
		t.Fatalf("english svg prompt missing:\n%s", svg)
	}
}

func TestCaptchaViewHidesAnswer(t *testing.T) {
	manager := NewCaptchaManager()
	view, err := manager.Issue("zh")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"answer", "targets", "char\""} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("view leaks %q: %s", secret, raw)
		}
	}
}
