// Package content validates format-1 packages without publishing them.
package content

import "github.com/kuny/glypha/internal/schedule"

const MaxDocumentBytes = 1 << 20
const MaxPackageBytes = 32 << 20
const MaxImagePixels int64 = 32_000_000

type Canvas struct {
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
}
type Schedule struct {
	UTCOffset    string                `json:"utcOffset"`
	DefaultScene string                `json:"defaultScene"`
	Daily        []schedule.Transition `json:"daily"`
}
type Background struct {
	Color string `json:"color"`
	Image string `json:"image,omitempty"`
	Fit   string `json:"fit,omitempty"`
}
type Element struct {
	Type     string `json:"type"`
	X        int64  `json:"x"`
	Y        int64  `json:"y"`
	Width    int64  `json:"width"`
	Height   int64  `json:"height"`
	Text     string `json:"text,omitempty"`
	FontSize int64  `json:"fontSize,omitempty"`
	Color    string `json:"color,omitempty"`
	Align    string `json:"align,omitempty"`
	Asset    string `json:"asset,omitempty"`
	Fit      string `json:"fit,omitempty"`
}
type Scene struct {
	Background Background `json:"background"`
	Elements   []Element  `json:"elements"`
}
type Document struct {
	Format   int              `json:"format"`
	Canvas   Canvas           `json:"canvas"`
	Schedule Schedule         `json:"schedule"`
	Scenes   map[string]Scene `json:"scenes"`
}
type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Validated describes structurally valid content, not a compiled or publishable AST.
// Exact font metrics, glyph coverage, and renderer-profile validation are a later stage.
type Validated struct {
	Document Document
	Schedule *schedule.Daily
}
