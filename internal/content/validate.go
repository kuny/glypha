package content

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"regexp"
	"unicode/utf8"

	"github.com/kuny/glypha/internal/schedule"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
var colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func validID(s string) bool { return idPattern.MatchString(s) && s != "." && s != ".." }
func fit(s string) bool     { return s == "contain" || s == "cover" }

// Validate checks structure, references, resource bounds, and image decodability.
// It deliberately does not claim exact text layout or AST publication readiness.
func Validate(raw []byte, assets map[string][]byte, profile Canvas) (*Validated, []Issue) {
	v := &validation{}
	if len(raw) > MaxDocumentBytes {
		v.add("", "resource_limit", "Content JSON exceeds 1 MiB.")
		return nil, v.issues
	}
	total := len(raw)
	for _, data := range assets {
		if len(data) > MaxPackageBytes-total {
			v.add("/assets", "resource_limit", "Package exceeds 32 MiB.")
			return nil, v.issues
		}
		total += len(data)
	}
	if !utf8.Valid(raw) {
		v.add("", "invalid_json", "JSON must be valid UTF-8.")
		return nil, v.issues
	}
	if err := scanJSON(raw); err != nil {
		v.add("", "invalid_json", err.Error())
		return nil, v.issues
	}
	d := v.shape(raw)
	if len(v.issues) > 0 {
		return nil, v.issues
	}
	if profile.Width <= 0 || profile.Height <= 0 {
		v.add("/canvas", "invalid_profile", "Configured display dimensions must be positive.")
	}
	if d.Format != 1 {
		v.add("/format", "unsupported_version", "Only content format 1 is supported.")
	}
	if d.Canvas.Width <= 0 || d.Canvas.Height <= 0 || d.Canvas != profile {
		v.add("/canvas", "invalid_canvas", "Canvas must match the configured positive display dimensions.")
	}
	if len(d.Scenes) < 1 || len(d.Scenes) > 64 {
		v.add("/scenes", "resource_limit", "Expected between one and 64 scenes.")
	}
	if len(d.Schedule.Daily) > 64 {
		v.add("/schedule/daily", "resource_limit", "At most 64 daily transitions are allowed.")
	}
	if _, err := schedule.ParseOffset(d.Schedule.UTCOffset); err != nil {
		v.add("/schedule/utcOffset", "invalid_offset", err.Error())
	}
	sceneRef := func(id, path string) {
		if _, ok := d.Scenes[id]; !ok {
			v.add(path, "unknown_scene", "The referenced scene does not exist.")
		}
	}
	sceneRef(d.Schedule.DefaultScene, "/schedule/defaultScene")
	seen := map[int64]bool{}
	for i, t := range d.Schedule.Daily {
		p := fmt.Sprintf("/schedule/daily/%d", i)
		n, err := schedule.ParseTime(t.At)
		if err != nil {
			v.add(p+"/at", "invalid_time", err.Error())
		} else if seen[n] {
			v.add(p+"/at", "duplicate_time", "Daily transition times must be unique.")
		} else {
			seen[n] = true
		}
		sceneRef(t.Scene, p+"/scene")
	}
	references := map[string]bool{}
	assetRef := func(id, path string) {
		references[id] = true
		if !validID(id) {
			v.add(path, "invalid_asset_id", "Expected a local asset ID.")
		}
		if _, ok := assets[id]; !ok {
			v.add(path, "missing_asset", "The referenced asset is missing.")
		}
	}
	for _, id := range keys(d.Scenes) {
		s := d.Scenes[id]
		p := pointer("/scenes", id)
		if !validID(id) {
			v.add(p, "invalid_scene_id", "Scene ID contains unsupported characters.")
		}
		if !colorPattern.MatchString(s.Background.Color) {
			v.add(p+"/background/color", "invalid_color", "Expected #RRGGBB.")
		}
		if s.Background.Image != "" {
			assetRef(s.Background.Image, p+"/background/image")
			if !fit(s.Background.Fit) {
				v.add(p+"/background/fit", "invalid_fit", "Expected contain or cover.")
			}
		}
		if len(s.Elements) > 128 {
			v.add(p+"/elements", "resource_limit", "At most 128 elements per scene are allowed.")
		}
		for i, e := range s.Elements {
			ep := fmt.Sprintf("%s/elements/%d", p, i)
			// Subtraction after dimension checks avoids overflowing x+width or y+height.
			if e.X < 0 || e.Y < 0 || e.Width <= 0 || e.Height <= 0 || e.Width > d.Canvas.Width || e.Height > d.Canvas.Height || e.X > d.Canvas.Width-e.Width || e.Y > d.Canvas.Height-e.Height {
				v.add(ep, "invalid_rectangle", "Element rectangle must lie inside the canvas.")
			}
			switch e.Type {
			case "text":
				if e.FontSize <= 0 {
					v.add(ep+"/fontSize", "invalid_font_size", "Font size must be positive.")
				}
				if !colorPattern.MatchString(e.Color) {
					v.add(ep+"/color", "invalid_color", "Expected #RRGGBB.")
				}
				if e.Align != "left" && e.Align != "center" && e.Align != "right" {
					v.add(ep+"/align", "invalid_align", "Expected left, center, or right.")
				}
			case "image":
				assetRef(e.Asset, ep+"/asset")
				if !fit(e.Fit) {
					v.add(ep+"/fit", "invalid_fit", "Expected contain or cover.")
				}
			}
		}
	}
	for _, id := range keys(assets) {
		p := pointer("/assets", id)
		if !validID(id) {
			v.add(p, "invalid_asset_id", "Asset ID contains unsupported characters.")
		}
		if !references[id] {
			v.add(p, "unused_asset", "Uploaded asset is not referenced.")
		}
	}
	if len(v.issues) > 0 {
		return nil, v.issues
	}
	// Validate all dimensions before decoding any full image.
	var pixels int64
	for _, id := range keys(assets) {
		p := pointer("/assets", id)
		cfg, format, err := image.DecodeConfig(bytes.NewReader(assets[id]))
		if err != nil || (format != "png" && format != "jpeg") {
			v.add(p, "invalid_image", "Expected a decodable static PNG or JPEG.")
			continue
		}
		w, h := int64(cfg.Width), int64(cfg.Height)
		if w <= 0 || h <= 0 || w > MaxImagePixels || h > MaxImagePixels || w > (MaxImagePixels-pixels)/h {
			v.add(p, "resource_limit", "Decoded package images exceed 32 million pixels.")
			continue
		}
		pixels += w * h
		if format == "png" && animatedPNG(assets[id]) {
			v.add(p, "animated_image", "Animated PNG is not supported.")
		}
	}
	if len(v.issues) > 0 {
		return nil, v.issues
	}
	for _, id := range keys(assets) {
		if _, _, err := image.Decode(bytes.NewReader(assets[id])); err != nil {
			v.add(pointer("/assets", id), "invalid_image", "Image data cannot be fully decoded.")
		}
	}
	if len(v.issues) > 0 {
		return nil, v.issues
	}
	daily, err := schedule.New(d.Schedule.UTCOffset, d.Schedule.DefaultScene, d.Schedule.Daily)
	if err != nil {
		v.add("/schedule", "invalid_schedule", err.Error())
		return nil, v.issues
	}
	return &Validated{Document: *d, Schedule: daily}, nil
}

func animatedPNG(data []byte) bool {
	// PNG chunks are length, type, payload, CRC. Decode verifies integrity separately.
	for at := 8; at <= len(data)-12; {
		size := uint64(binary.BigEndian.Uint32(data[at : at+4]))
		if size > uint64(len(data)-at-12) {
			return false
		}
		if string(data[at+4:at+8]) == "acTL" {
			return true
		}
		at += int(size) + 12
	}
	return false
}
