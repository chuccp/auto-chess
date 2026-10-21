package webui_test

import (
	"strings"
	"testing"

	"github.com/chuccp/auto-chess/internal/webui"
)

func TestLocalizeHTML(t *testing.T) {
	out := webui.LocalizeHTML(`<!doctype html><html lang="zh-CN"><head><title>auto-chess · AI 走子接口</title></head><body><p>首页</p><p>开始对弈</p></body></html>`)
	for _, want := range []string{`lang="en"`, "auto-chess · AI Move API", ">Home<", ">Start playing<"} {
		if !strings.Contains(out, want) {
			t.Fatalf("localized page missing %q:\n%s", want, out)
		}
	}
}
