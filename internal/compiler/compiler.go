// Package compiler turns validated packages into schedule-free scene ASTs.
package compiler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/shaping"
	"github.com/kuny/glypha/internal/content"
	"github.com/kuny/glypha/internal/schedule"
	"golang.org/x/image/math/fixed"
)

const Profile = "noto-sans-jp-2.004-regular-400-v1"
const FontHash = "c2f3b4d463500a2ddcd3849cded1fceeb9fd6d1c32e6cbecd568453ba50fc68f"
const MaxEnvelope = 48 << 20

type AST struct {
	Version    int                `json:"version"`
	Profile    string             `json:"profile"`
	Canvas     content.Canvas     `json:"canvas"`
	Background content.Background `json:"background"`
	Elements   []content.Element  `json:"elements"`
}
type Asset struct {
	MediaType string `json:"mediaType"`
	SHA256    string `json:"sha256"`
	Data      []byte `json:"base64"`
}
type Envelope struct {
	Protocol   int              `json:"protocol"`
	Generation string           `json:"generation"`
	Scene      string           `json:"scene"`
	AST        AST              `json:"ast"`
	Assets     map[string]Asset `json:"assets"`
}
type stored struct {
	Profile string            `json:"profile"`
	Source  json.RawMessage   `json:"source"`
	Assets  map[string][]byte `json:"assets"`
	Scenes  map[string]AST    `json:"scenes"`
}

// Candidate exposes no mutable content to callers.
type Candidate struct {
	encoded []byte
	daily   *schedule.Daily
	scenes  map[string]AST
	assets  map[string]Asset
}

func (c *Candidate) Bytes() []byte { return bytes.Clone(c.encoded) }
func (c *Candidate) ApplicableAt(n int64) (schedule.Occurrence, error) {
	return c.daily.ApplicableAt(n)
}
func (c *Candidate) LatestBetween(a, b int64) (schedule.Occurrence, bool, error) {
	return c.daily.LatestBetween(a, b)
}
func (c *Candidate) Snapshot(generation, scene string) ([]byte, string, error) {
	ast, ok := c.scenes[scene]
	if !ok {
		return nil, "", fmt.Errorf("unknown stored target")
	}
	refs := map[string]Asset{}
	if ast.Background.Image != "" {
		refs[ast.Background.Image] = c.assets[ast.Background.Image]
	}
	for _, e := range ast.Elements {
		if e.Type == "image" {
			refs[e.Asset] = c.assets[e.Asset]
		}
	}
	b, err := json.Marshal(Envelope{1, generation, scene, ast, refs})
	if err != nil {
		return nil, "", err
	}
	if len(b) > MaxEnvelope {
		return nil, "", fmt.Errorf("snapshot exceeds limit")
	}
	return b, fmt.Sprintf(`"%x"`, sha256.Sum256(b)), nil
}

type Compiler struct {
	mu     sync.Mutex
	face   *font.Face
	canvas content.Canvas
}

func New(path string, canvas content.Canvas) (*Compiler, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != FontHash {
		return nil, fmt.Errorf("font checksum mismatch")
	}
	face, err := font.ParseTTF(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	face.SetVariations([]font.Variation{{Tag: font.Tag(0x77676874), Value: 400}})
	return &Compiler{face: face, canvas: canvas}, nil
}
func (c *Compiler) Compile(source []byte, images map[string][]byte) (*Candidate, []content.Issue) {
	v, issues := content.Validate(source, images, c.canvas)
	if len(issues) > 0 {
		return nil, issues
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	scenes := map[string]AST{}
	ids := make([]string, 0, len(v.Document.Scenes))
	for id := range v.Document.Scenes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		scene := v.Document.Scenes[id]
		for i, e := range scene.Elements {
			if e.Type == "text" {
				if err := c.textFits(e); err != nil {
					issues = append(issues, content.Issue{Path: fmt.Sprintf("/scenes/%s/elements/%d/text", id, i), Code: "invalid_text_layout", Message: err.Error()})
				}
			}
		}
		scenes[id] = AST{1, Profile, v.Document.Canvas, scene.Background, scene.Elements}
	}
	if len(issues) > 0 {
		return nil, issues
	}
	raw, _ := json.Marshal(v.Document)
	cloned := map[string][]byte{}
	assets := map[string]Asset{}
	for id, data := range images {
		data = bytes.Clone(data)
		cloned[id] = data
		media := "image/jpeg"
		if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
			media = "image/png"
		}
		assets[id] = Asset{media, fmt.Sprintf("%x", sha256.Sum256(data)), data}
	}
	encoded, err := json.Marshal(stored{Profile, raw, cloned, scenes})
	if err != nil {
		return nil, []content.Issue{{Code: "compile_failed", Message: "Cannot serialize content."}}
	}
	return &Candidate{encoded, v.Schedule, scenes, assets}, nil
}

// Restore revalidates durable state rather than silently repairing or replacing it.
func (c *Compiler) Restore(data []byte) (*Candidate, error) {
	var s stored
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Profile != Profile {
		return nil, fmt.Errorf("unsupported stored profile")
	}
	candidate, issues := c.Compile(s.Source, s.Assets)
	if len(issues) > 0 {
		return nil, fmt.Errorf("stored content failed validation: %v", issues)
	}
	if !bytes.Equal(candidate.encoded, data) {
		return nil, fmt.Errorf("stored content is not canonical or compiled AST differs")
	}
	return candidate, nil
}

type oneFace struct{ face *font.Face }

func (f oneFace) ResolveFace(rune) *font.Face { return f.face }
func (c *Compiler) textFits(e content.Element) error {
	if e.FontSize > 4096 || len([]rune(e.Text)) > 16384 {
		return fmt.Errorf("Text exceeds the supported size or length.")
	}
	for _, r := range e.Text {
		if r == '\n' {
			continue
		}
		if unicode.IsControl(r) {
			return fmt.Errorf("Control characters other than newline are unsupported.")
		}
		if _, ok := c.face.NominalGlyph(r); !ok {
			return fmt.Errorf("The bundled font has no glyph for U+%04X.", r)
		}
	}
	lines := strings.Split(e.Text, "\n")
	if float64(len(lines))*float64(e.FontSize)*1.2 > float64(e.Height) {
		return fmt.Errorf("Text lines exceed the element height.")
	}
	var segmenter shaping.Segmenter
	var shaper shaping.HarfbuzzShaper
	for _, line := range lines {
		runes := []rune(line)
		if len(runes) == 0 {
			continue
		}
		runs := segmenter.Split(shaping.Input{Text: runes, RunEnd: len(runes), Direction: di.DirectionLTR, Size: fixed.I(int(e.FontSize))}, oneFace{c.face})
		var advance, minX, maxX, ascent, descent float64
		for _, run := range runs {
			out := shaper.Shape(run)
			for _, g := range out.Glyphs {
				x := advance + float64(g.XOffset+g.XBearing)/64
				minX = math.Min(minX, x)
				maxX = math.Max(maxX, x+float64(g.Width)/64)
				ascent = math.Max(ascent, float64(g.YOffset+g.YBearing)/64)
				descent = math.Max(descent, -float64(g.YOffset+g.YBearing+g.Height)/64)
				advance += float64(g.Advance) / 64
			}
		}
		width := math.Max(advance, maxX) - math.Min(0, minX)
		if width > float64(e.Width) || ascent+descent > float64(e.FontSize)*1.2 {
			return fmt.Errorf("Text glyphs do not fit the element rectangle.")
		}
	}
	return nil
}
