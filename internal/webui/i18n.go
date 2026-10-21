package webui

import (
	"bytes"
	"embed"
	"encoding/json"
	"log"
	"net/url"
	"strings"

	xhtml "golang.org/x/net/html"
)

//go:embed lang/*.json
var langFS embed.FS

var enText = map[string]string{}

func init() {
	entries, err := langFS.ReadDir("lang")
	if err != nil {
		log.Printf("webui: read lang files: %v", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := langFS.ReadFile("lang/" + entry.Name())
		if err != nil {
			log.Printf("webui: read lang/%s: %v", entry.Name(), err)
			continue
		}
		var page map[string]string
		if err := json.Unmarshal(data, &page); err != nil {
			log.Printf("webui: parse lang/%s: %v", entry.Name(), err)
			continue
		}
		for key, value := range page {
			if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
				continue
			}
			enText[key] = value
		}
	}
}

// NormalizeLang returns "en" or "zh". Chinese is the default.
func NormalizeLang(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "en") || strings.EqualFold(strings.TrimSpace(value), "english") {
		return "en"
	}
	return "zh"
}

// OtherLang returns the language a switch should lead to.
func OtherLang(current string) string {
	if NormalizeLang(current) == "en" {
		return "zh"
	}
	return "en"
}

// LangSwitchURL keeps the current path/query and changes the lang parameter.
func LangSwitchURL(u *url.URL, current string) string {
	if u == nil {
		return "?lang=" + OtherLang(current)
	}
	copyURL := *u
	q := copyURL.Query()
	q.Set("lang", OtherLang(current))
	copyURL.RawQuery = q.Encode()
	return copyURL.RequestURI()
}

// LocalizeHTML translates text nodes and a few user-facing attributes of an
// already rendered page. It deliberately skips code blocks and scripts so JSON
// examples and executable code stay untouched.
func LocalizeHTML(page string) string {
	doc, err := xhtml.Parse(strings.NewReader(page))
	if err != nil {
		return page
	}
	walkLocalize(doc, false)
	if root := findElement(doc, "html"); root != nil {
		setAttr(root, "lang", "en")
		setAttr(root, "data-lang", "en")
	}
	var buf bytes.Buffer
	if err := xhtml.Render(&buf, doc); err != nil {
		return page
	}
	return buf.String()
}

func walkLocalize(n *xhtml.Node, skip bool) {
	switch n.Type {
	case xhtml.ElementNode:
		switch n.Data {
		case "script", "style", "pre", "textarea":
			skip = true
		}
		if hasAttr(n, "data-i18n-ignore") {
			skip = true
		}
		for i := range n.Attr {
			key := n.Attr[i].Key
			switch key {
			case "title", "aria-label", "placeholder", "alt", "content":
				if translated, ok := enText[strings.TrimSpace(n.Attr[i].Val)]; ok {
					n.Attr[i].Val = translated
				}
			}
		}
	case xhtml.TextNode:
		if skip {
			return
		}
		raw := n.Data
		trimmed := strings.Join(strings.Fields(raw), " ")
		if trimmed == "" {
			return
		}
		translated, ok := enText[trimmed]
		if !ok {
			return
		}
		left := raw[:len(raw)-len(strings.TrimLeft(raw, " \t\r\n"))]
		right := raw[len(strings.TrimRight(raw, " \t\r\n")):]
		n.Data = left + translated + right
		return
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walkLocalize(child, skip)
	}
}

func findElement(n *xhtml.Node, tag string) *xhtml.Node {
	if n.Type == xhtml.ElementNode && n.Data == tag {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func setAttr(n *xhtml.Node, key, value string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, xhtml.Attribute{Key: key, Val: value})
}

func hasAttr(n *xhtml.Node, key string) bool {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			return true
		}
	}
	return false
}
