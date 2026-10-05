package content

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
)

var display = Canvas{1920, 1080}

func example(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../examples/daily/content.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func fixture(t *testing.T) map[string]any {
	t.Helper()
	var d map[string]any
	if err := json.Unmarshal(example(t), &d); err != nil {
		t.Fatal(err)
	}
	return d
}
func encode(t *testing.T, d any) []byte {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func has(issues []Issue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}
func TestDailyExample(t *testing.T) {
	result, issues := Validate(example(t), nil, display)
	if len(issues) != 0 || result == nil {
		t.Fatalf("%+v", issues)
	}
	if o, err := result.Schedule.ApplicableAt(0); err != nil || o.Scene != "closed" {
		t.Fatalf("%+v %v", o, err)
	}
}
func TestStrictJSON(t *testing.T) {
	valid := string(example(t))
	for _, tc := range []struct{ name, raw, code string }{
		{"duplicate", strings.Replace(valid, `"format": 1`, `"format": 1, "format": 1`, 1), "invalid_json"},
		{"escaped duplicate", strings.Replace(valid, `"format": 1`, `"format": 1, "\u0066ormat": 1`, 1), "invalid_json"},
		{"trailing", valid + ` {}`, "invalid_json"},
		{"case", strings.Replace(valid, `"format"`, `"Format"`, 1), "unknown_field"},
		{"unknown", strings.Replace(valid, `"format": 1`, `"format": 1, "draft": true`, 1), "unknown_field"},
		{"null", strings.Replace(valid, `"format": 1`, `"format": null`, 1), "invalid_type"},
		{"fraction", strings.Replace(valid, `"format": 1`, `"format": 1.0`, 1), "invalid_type"},
		{"missing", strings.Replace(valid, `"x": 100, `, ``, 1), "required"},
		{"null element", strings.Replace(valid, `"elements": [`, `"elements": [null,`, 1), "invalid_type"},
		{"invalid utf8", "\xff", "invalid_json"},
		{"depth", strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66), "invalid_json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, issues := Validate([]byte(tc.raw), nil, display)
			if result != nil || !has(issues, tc.code) {
				t.Fatalf("%+v", issues)
			}
		})
	}
}
func TestSemanticRejections(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		edit       func(map[string]any)
	}{
		{"version", "unsupported_version", func(d map[string]any) { d["format"] = 2 }},
		{"canvas", "invalid_canvas", func(d map[string]any) { d["canvas"].(map[string]any)["width"] = 1 }},
		{"offset", "invalid_offset", func(d map[string]any) { d["schedule"].(map[string]any)["utcOffset"] = "Asia/Tokyo" }},
		{"missing scene", "unknown_scene", func(d map[string]any) { d["schedule"].(map[string]any)["defaultScene"] = "missing" }},
		{"duplicate time", "duplicate_time", func(d map[string]any) {
			a := d["schedule"].(map[string]any)["daily"].([]any)
			a[1].(map[string]any)["at"] = a[0].(map[string]any)["at"]
		}},
		{"midnight", "invalid_time", func(d map[string]any) {
			d["schedule"].(map[string]any)["daily"].([]any)[0].(map[string]any)["at"] = "00:00:00"
		}},
		{"overflow rectangle", "invalid_rectangle", func(d map[string]any) { element(d)["x"] = json.Number("9223372036854775807") }},
		{"negative width", "invalid_rectangle", func(d map[string]any) { element(d)["width"] = -1 }},
		{"color", "invalid_color", func(d map[string]any) { element(d)["color"] = "red" }},
		{"font size", "invalid_font_size", func(d map[string]any) { element(d)["fontSize"] = 0 }},
		{"alignment", "invalid_align", func(d map[string]any) { element(d)["align"] = "justify" }},
		{"mixed element", "unknown_field", func(d map[string]any) { element(d)["asset"] = "logo.png" }},
		{"unsupported element", "invalid_element", func(d map[string]any) { element(d)["type"] = "video" }},
		{"background pair", "invalid_background", func(d map[string]any) { background(d)["fit"] = "cover" }},
		{"empty background image", "invalid_asset_id", func(d map[string]any) { background(d)["image"] = ""; background(d)["fit"] = "contain" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := fixture(t)
			tc.edit(d)
			result, issues := Validate(encode(t, d), nil, display)
			if result != nil || !has(issues, tc.code) {
				t.Fatalf("%+v", issues)
			}
		})
	}
}
func element(d map[string]any) map[string]any {
	return d["scenes"].(map[string]any)["closed"].(map[string]any)["elements"].([]any)[0].(map[string]any)
}
func background(d map[string]any) map[string]any {
	return d["scenes"].(map[string]any)["closed"].(map[string]any)["background"].(map[string]any)
}
func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func chunk(kind string, payload []byte) []byte {
	b := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint32(b, uint32(len(payload)))
	copy(b[4:], kind)
	copy(b[8:], payload)
	binary.BigEndian.PutUint32(b[len(b)-4:], crc32.ChecksumIEEE(b[4:len(b)-4]))
	return b
}
func TestAssets(t *testing.T) {
	d := fixture(t)
	background(d)["image"] = "logo.png"
	background(d)["fit"] = "contain"
	raw := encode(t, d)
	good := pngBytes(t)
	animation := append([]byte{}, good[:33]...)
	animation = append(animation, chunk("acTL", []byte{0, 0, 0, 2, 0, 0, 0, 0})...)
	animation = append(animation, good[33:]...)
	oversized := append([]byte{}, good...)
	binary.BigEndian.PutUint32(oversized[16:20], 32000001)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	for _, tc := range []struct {
		name, code string
		assets     map[string][]byte
	}{
		{"valid", "", map[string][]byte{"logo.png": good}},
		{"missing", "missing_asset", nil},
		{"unused", "unused_asset", map[string][]byte{"logo.png": good, "extra.png": good}},
		{"invalid", "invalid_image", map[string][]byte{"logo.png": []byte("not an image")}},
		{"truncated", "invalid_image", map[string][]byte{"logo.png": good[:33]}},
		{"animated", "animated_image", map[string][]byte{"logo.png": animation}},
		{"oversized dimensions", "resource_limit", map[string][]byte{"logo.png": oversized}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, issues := Validate(raw, tc.assets, display)
			if tc.code == "" {
				if result == nil || len(issues) > 0 {
					t.Fatalf("%+v", issues)
				}
			} else if result != nil || !has(issues, tc.code) {
				t.Fatalf("%+v", issues)
			}
		})
	}
}
func TestResourceLimits(t *testing.T) {
	if _, issues := Validate(bytes.Repeat([]byte(" "), MaxDocumentBytes+1), nil, display); !has(issues, "resource_limit") {
		t.Fatal(issues)
	}
	if _, issues := Validate(example(t), map[string][]byte{"big": make([]byte, MaxPackageBytes)}, display); !has(issues, "resource_limit") {
		t.Fatal(issues)
	}
}
func FuzzValidateJSON(f *testing.F) {
	seed, err := os.ReadFile("../../examples/daily/content.json")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte(`{"format":1}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"scenes":{"a":{"elements":[null]}}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		result, issues := Validate(raw, nil, display)
		if (result == nil) != (len(issues) > 0) {
			t.Fatal("result and issues disagree")
		}
	})
}

func TestImageElementAndJPEG(t *testing.T) {
	d := fixture(t)
	d["scenes"].(map[string]any)["closed"].(map[string]any)["elements"] = []any{map[string]any{
		"type": "image", "x": 0, "y": 0, "width": 1920, "height": 1080, "asset": "photo.jpg", "fit": "cover",
	}}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	if result, issues := Validate(encode(t, d), map[string][]byte{"photo.jpg": encoded.Bytes()}, display); result == nil || len(issues) > 0 {
		t.Fatal(issues)
	}
}
func TestIssuePathsAndStaticContent(t *testing.T) {
	d := fixture(t)
	d["schedule"].(map[string]any)["daily"] = []any{}
	if result, issues := Validate(encode(t, d), nil, display); result == nil || len(issues) > 0 {
		t.Fatal(issues)
	}
	d["schedule"].(map[string]any)["defaultScene"] = "absent"
	_, issues := Validate(encode(t, d), nil, display)
	if len(issues) != 1 || issues[0].Path != "/schedule/defaultScene" || issues[0].Code != "unknown_scene" {
		t.Fatal(issues)
	}
}
