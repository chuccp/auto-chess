package webui

import (
	"strings"
	"testing"
)

func TestRelativeBase(t *testing.T) {
	cases := []struct {
		path    string
		context string
		want    string
	}{
		{"/auto-chess", "/auto-chess", "./"},
		{"/auto-chess/", "/auto-chess", "./"},
		{"/auto-chess/play", "/auto-chess", "./"},
		{"/auto-chess/docs/", "/auto-chess", "../"},
		{"/auto-chess/games/xiangqi", "/auto-chess", "../"},
		{"/auto-chess/games/xiangqi/", "/auto-chess", "../../"},
		{"/play", "", "./"},
		{"/games/xiangqi", "", "../"},
		{"/foo/bar/games/xiangqi", "/foo/bar", "../"},
	}
	for _, tc := range cases {
		if got := RelativeBase(tc.path, tc.context); got != tc.want {
			t.Errorf("RelativeBase(%q, %q) = %q, want %q", tc.path, tc.context, got, tc.want)
		}
	}
}

func TestRelativizeHTML(t *testing.T) {
	page := `<a href="/play">play</a><img src="/a.png"><a href="#api">api</a><a href="https://example.com">x</a>`
	got := relativizeHTML(page, "../")
	for _, want := range []string{`href="../play"`, `src="../a.png"`, `href="#api"`, `href="https://example.com"`} {
		if !strings.Contains(got, want) {
			t.Errorf("relativized page does not contain %q: %s", want, got)
		}
	}
}
