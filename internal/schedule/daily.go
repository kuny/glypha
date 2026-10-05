// Package schedule evaluates daily templates using explicit Unix seconds.
package schedule

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
)

const DaySeconds int64 = 86400

// The supported interval leaves room for offset and batch arithmetic.
const MinSecond int64 = -62135596800 // 0001-01-01 UTC
const MaxSecond int64 = 253402300799 // 9999-12-31 UTC

var ErrTimeRange = errors.New("timestamp is outside the supported range")

type Transition struct {
	At    string `json:"at"`
	Scene string `json:"scene"`
}
type Occurrence struct {
	Second int64
	Scene  string
}
type entry struct {
	second int64
	scene  string
}

// Daily is immutable after construction and safe for concurrent evaluation.
type Daily struct {
	offset  int64
	entries []entry
}

func ParseOffset(value string) (int64, error) {
	if len(value) != 6 || (value[0] != '+' && value[0] != '-') || value[3] != ':' {
		return 0, errors.New("expected a fixed offset in ±HH:MM")
	}
	h, okH := digits(value[1:3])
	m, okM := digits(value[4:6])
	if !okH || !okM || h > 14 || m > 59 || (h == 14 && m != 0) {
		return 0, errors.New("offset must be between -14:00 and +14:00")
	}
	result := int64(h*3600 + m*60)
	if value[0] == '-' {
		result = -result
	}
	return result, nil
}
func ParseTime(value string) (int64, error) {
	if len(value) != 8 || value[2] != ':' || value[5] != ':' {
		return 0, errors.New("expected HH:MM:SS")
	}
	h, a := digits(value[:2])
	m, b := digits(value[3:5])
	s, c := digits(value[6:])
	if !a || !b || !c || h > 23 || m > 59 || s > 59 {
		return 0, errors.New("invalid time of day")
	}
	n := int64(h*3600 + m*60 + s)
	if n == 0 {
		return 0, errors.New("midnight is reserved for the default scene")
	}
	return n, nil
}
func digits(s string) (int, bool) {
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}
func New(offset, defaultScene string, transitions []Transition) (*Daily, error) {
	o, err := ParseOffset(offset)
	if err != nil {
		return nil, err
	}
	if defaultScene == "" {
		return nil, errors.New("default scene is required")
	}
	if len(transitions) > 64 {
		return nil, errors.New("at most 64 daily transitions are allowed")
	}
	es := []entry{{0, defaultScene}}
	seen := map[int64]bool{0: true}
	for _, t := range transitions {
		second, err := ParseTime(t.At)
		if err != nil {
			return nil, fmt.Errorf("time %q: %w", t.At, err)
		}
		if seen[second] {
			return nil, fmt.Errorf("duplicate transition time %q", t.At)
		}
		if t.Scene == "" {
			return nil, errors.New("transition scene is required")
		}
		seen[second] = true
		es = append(es, entry{second, t.Scene})
	}
	sort.Slice(es, func(i, j int) bool { return es[i].second < es[j].second })
	return &Daily{offset: o, entries: es}, nil
}
func inRange(t int64) bool { return t >= MinSecond && t <= MaxSecond }
func floorDay(t int64) int64 {
	q := t / DaySeconds
	if t%DaySeconds < 0 {
		q--
	}
	return q
}
func (d *Daily) valid() bool { return d != nil && len(d.entries) > 0 }
func (d *Daily) ApplicableAt(now int64) (Occurrence, error) {
	if !d.valid() {
		return Occurrence{}, errors.New("daily template is uninitialized")
	}
	if !inRange(now) {
		return Occurrence{}, ErrTimeRange
	}
	midnight := floorDay(now+d.offset)*DaySeconds - d.offset
	at := now - midnight
	i := sort.Search(len(d.entries), func(i int) bool { return d.entries[i].second > at }) - 1
	selected := d.entries[i]
	return Occurrence{midnight + selected.second, selected.scene}, nil
}

// LatestBetween selects one occurrence in (cursor, now], skipping intermediate scenes.
func (d *Daily) LatestBetween(cursor, now int64) (Occurrence, bool, error) {
	if !d.valid() {
		return Occurrence{}, false, errors.New("daily template is uninitialized")
	}
	if !inRange(cursor) || !inRange(now) {
		return Occurrence{}, false, ErrTimeRange
	}
	if now <= cursor {
		return Occurrence{}, false, nil
	}
	o, err := d.ApplicableAt(now)
	if err != nil {
		return Occurrence{}, false, err
	}
	if o.Second <= cursor {
		return Occurrence{}, false, nil
	}
	return o, true, nil
}

// Generate materializes at most one day, with boundaries (start, end].
func (d *Daily) Generate(start, end int64) ([]Occurrence, error) {
	if !d.valid() {
		return nil, errors.New("daily template is uninitialized")
	}
	if !inRange(start) || !inRange(end) {
		return nil, ErrTimeRange
	}
	if end < start || end-start > DaySeconds {
		return nil, errors.New("generation interval must be between zero and 86400 seconds")
	}
	out := make([]Occurrence, 0, len(d.entries))
	for day := floorDay(start + d.offset); day <= floorDay(end+d.offset); day++ {
		for _, e := range d.entries {
			second := day*DaySeconds - d.offset + e.second
			if second > start && second <= end {
				out = append(out, Occurrence{second, e.scene})
			}
		}
	}
	return out, nil
}
