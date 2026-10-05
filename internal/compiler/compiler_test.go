package compiler

import (
	"encoding/json"
	"github.com/kuny/glypha/internal/content"
	"os"
	"strings"
	"testing"
)

func testCompiler(t *testing.T) *Compiler {
	t.Helper()
	c, err := New("../../renderer/public/fonts/noto-sans-jp/NotoSansJP.ttf", content.Canvas{Width: 1920, Height: 1080})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestCompileAndRestore(t *testing.T) {
	c := testCompiler(t)
	raw, err := os.ReadFile("../../examples/daily/content.json")
	if err != nil {
		t.Fatal(err)
	}
	candidate, issues := c.Compile(raw, nil)
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	body, etag, err := candidate.Snapshot("generation", "open")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "schedule") || etag == "" {
		t.Fatal(string(body))
	}
	restored, err := c.Restore(candidate.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	body2, tag2, _ := restored.Snapshot("generation", "open")
	if string(body) != string(body2) || tag2 != etag {
		t.Fatal("non-deterministic snapshot")
	}
	source := candidate.Bytes()
	source[0] = 'x'
	if _, err := c.Restore(source); err == nil {
		t.Fatal("corrupt state accepted")
	}
}
func TestFontLayout(t *testing.T) {
	c := testCompiler(t)
	for _, tc := range []struct {
		text  string
		width int64
		valid bool
	}{
		{"English and \u65e5\u672c\u8a9e", 1000, true},
		{"A combining e\u0301", 1000, true},
		{"A very wide line", 1, false},
		{"\U0001f680", 1000, false},
		{"a\tb", 1000, false},
	} {
		e := content.Element{Type: "text", Text: tc.text, FontSize: 32, Width: tc.width, Height: 100}
		err := c.textFits(e)
		if (err == nil) != tc.valid {
			t.Errorf("%q: %v", tc.text, err)
		}
	}
}
func TestCompileRejectsTextBeforePublication(t *testing.T) {
	c := testCompiler(t)
	raw, _ := os.ReadFile("../../examples/daily/content.json")
	var d content.Document
	_ = json.Unmarshal(raw, &d)
	scene := d.Scenes["open"]
	scene.Elements[0].Width = 1
	d.Scenes["open"] = scene
	raw, _ = json.Marshal(d)
	result, issues := c.Compile(raw, nil)
	if result != nil || len(issues) == 0 || issues[0].Code != "invalid_text_layout" {
		t.Fatal(issues)
	}
}

func TestEmptyTextRoundTrip(t *testing.T) {
	c := testCompiler(t)
	raw, _ := os.ReadFile("../../examples/daily/content.json")
	var d content.Document
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	scene := d.Scenes["open"]
	scene.Elements[0].Text = ""
	d.Scenes["open"] = scene
	raw, _ = json.Marshal(d)
	candidate, issues := c.Compile(raw, nil)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	if _, err := c.Restore(candidate.Bytes()); err != nil {
		t.Fatal(err)
	}
	body, _, err := candidate.Snapshot("test", "open")
	if err != nil || !strings.Contains(string(body), `"text":""`) {
		t.Fatalf("missing empty text: %s, %v", body, err)
	}
}
