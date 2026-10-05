package content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type validation struct{ issues []Issue }

func (v *validation) add(path, code, message string) {
	v.issues = append(v.issues, Issue{path, code, message})
}
func pointer(path, key string) string {
	return path + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}
func keys[M ~map[string]V, V any](m M) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Scan before map decoding so repeated keys, including escaped equivalents, are rejected.
func scanJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 64 {
			return fmt.Errorf("JSON nesting exceeds 64 levels")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				token, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := token.(string)
				if !ok {
					return fmt.Errorf("expected object key")
				}
				if seen[key] {
					return fmt.Errorf("duplicate object key %q", key)
				}
				seen[key] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON document")
	}
	return nil
}
func (v *validation) object(raw json.RawMessage, path string, required, optional []string, dst any) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		v.add(path, "invalid_type", "Expected an object.")
		return nil
	}
	allowed := map[string]bool{}
	for _, key := range required {
		allowed[key] = true
		if _, ok := fields[key]; !ok {
			v.add(pointer(path, key), "required", "This field is required.")
		}
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for _, key := range keys(fields) {
		if !allowed[key] {
			v.add(pointer(path, key), "unknown_field", "Unknown field.")
		}
		if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			v.add(pointer(path, key), "invalid_type", "Null is not allowed.")
		}
	}
	if dst != nil {
		if err := json.Unmarshal(raw, dst); err != nil {
			v.add(path, "invalid_type", "Field values have incorrect types or integer ranges.")
		}
	}
	return fields
}
func (v *validation) array(raw json.RawMessage, path string) []json.RawMessage {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		v.add(path, "invalid_type", "Expected an array.")
	}
	return items
}
func (v *validation) shape(raw []byte) *Document {
	var d Document
	root := v.object(raw, "", []string{"format", "canvas", "schedule", "scenes"}, nil, &d)
	if root == nil {
		return nil
	}
	v.object(root["canvas"], "/canvas", []string{"width", "height"}, nil, nil)
	sched := v.object(root["schedule"], "/schedule", []string{"utcOffset", "defaultScene", "daily"}, nil, nil)
	if sched != nil {
		for i, e := range v.array(sched["daily"], "/schedule/daily") {
			v.object(e, fmt.Sprintf("/schedule/daily/%d", i), []string{"at", "scene"}, nil, nil)
		}
	}
	var scenes map[string]json.RawMessage
	if err := json.Unmarshal(root["scenes"], &scenes); err != nil || scenes == nil {
		v.add("/scenes", "invalid_type", "Expected a scene map.")
		return &d
	}
	for _, id := range keys(scenes) {
		path := pointer("/scenes", id)
		fields := v.object(scenes[id], path, []string{"background", "elements"}, nil, nil)
		if fields == nil {
			continue
		}
		bg := v.object(fields["background"], path+"/background", []string{"color"}, []string{"image", "fit"}, nil)
		_, hasImage := bg["image"]
		_, hasFit := bg["fit"]
		if hasImage != hasFit {
			v.add(path+"/background", "invalid_background", "Image and fit must be specified together.")
		}
		if hasImage && d.Scenes[id].Background.Image == "" {
			v.add(path+"/background/image", "invalid_asset_id", "Image ID cannot be empty.")
		}
		for i, e := range v.array(fields["elements"], path+"/elements") {
			ep := fmt.Sprintf("%s/elements/%d", path, i)
			var kind struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(e, &kind)
			required := []string{"type", "x", "y", "width", "height"}
			switch kind.Type {
			case "text":
				required = append(required, "text", "fontSize", "color", "align")
			case "image":
				required = append(required, "asset", "fit")
			default:
				v.add(ep+"/type", "invalid_element", "Expected text or image.")
			}
			v.object(e, ep, required, nil, nil)
		}
	}
	return &d
}
