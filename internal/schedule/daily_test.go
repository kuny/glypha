package schedule

import (
	"errors"
	"math"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func template(t *testing.T) *Daily {
	t.Helper()
	d, err := New("+09:00", "closed", []Transition{{"18:00:00", "closed"}, {"10:00:00", "open"}, {"13:00:00", "afternoon"}})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func second(t *testing.T, value string) int64 {
	t.Helper()
	n, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return n.Unix()
}
func TestDailyBoundaries(t *testing.T) {
	d := template(t)
	for _, tc := range []struct{ at, want string }{
		{"2026-10-05T00:00:00+09:00", "closed"},
		{"2026-10-05T09:59:59+09:00", "closed"},
		{"2026-10-05T10:00:00+09:00", "open"},
		{"2026-10-05T12:59:59+09:00", "open"},
		{"2026-10-05T13:00:00+09:00", "afternoon"},
		{"2026-10-05T18:00:00+09:00", "closed"},
		{"1969-12-31T13:00:00+09:00", "afternoon"},
	} {
		o, err := d.ApplicableAt(second(t, tc.at))
		if err != nil || o.Scene != tc.want {
			t.Fatalf("%s: %+v %v", tc.at, o, err)
		}
	}
}
func TestCatchUpRollbackAndRestart(t *testing.T) {
	d := template(t)
	cursor := second(t, "2026-10-05T09:00:00+09:00")
	o, ok, err := d.LatestBetween(cursor, second(t, "2026-10-05T14:00:00+09:00"))
	if err != nil || !ok || o.Scene != "afternoon" || o.Second != second(t, "2026-10-05T13:00:00+09:00") {
		t.Fatalf("catch-up: %+v %v %v", o, ok, err)
	}
	cursor = o.Second
	// Reconstruct only the template; the persistence layer supplies the saved cursor.
	d = template(t)
	for _, now := range []int64{cursor, cursor - 1, cursor - 86400} {
		if _, ok, err := d.LatestBetween(cursor, now); ok || err != nil {
			t.Fatalf("rollback replay: %v %v", ok, err)
		}
	}
	o, ok, err = d.LatestBetween(cursor, second(t, "2027-03-05T14:00:00+09:00"))
	if !ok || err != nil || o.Scene != "afternoon" || o.Second <= cursor {
		t.Fatalf("long jump: %+v %v", o, err)
	}
}
func TestConsecutiveBatches(t *testing.T) {
	d := template(t)
	start := int64(-12345)
	a, err := d.Generate(start, start+DaySeconds)
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.Generate(start+DaySeconds, start+2*DaySeconds)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 4 || len(b) != 4 {
		t.Fatalf("batch counts: %d %d", len(a), len(b))
	}
	all := append(a, b...)
	for i, o := range all {
		if o.Second <= start || o.Second > start+2*DaySeconds || (i > 0 && o.Second <= all[i-1].Second) {
			t.Fatal("duplicate or out-of-range occurrence")
		}
	}
	// Every explicit occurrence is exactly one day apart from its counterpart.
	for i := range a {
		if b[i].Second-a[i].Second != DaySeconds || b[i].Scene != a[i].Scene {
			t.Fatal("missing daily occurrence")
		}
	}
	empty, err := d.Generate(start, start)
	if err != nil || len(empty) != 0 {
		t.Fatal("empty interval")
	}
}
func TestStaticScheduleStillConsumesMidnight(t *testing.T) {
	d, _ := New("+00:00", "same", nil)
	o, ok, err := d.LatestBetween(1, DaySeconds)
	if err != nil || !ok || o != (Occurrence{DaySeconds, "same"}) {
		t.Fatalf("%+v %v %v", o, ok, err)
	}
}
func TestInvalidInputs(t *testing.T) {
	for _, offset := range []string{"UTC", "+9:00", "+14:01", "-15:00", "+00:60", "+aa:00"} {
		if _, err := New(offset, "x", nil); err == nil {
			t.Errorf("accepted %s", offset)
		}
	}
	for _, at := range []string{"00:00:00", "24:00:00", "12:60:00", "12:00:60", "1:00:00", "12:00:00.1", "12:00:0x"} {
		if _, err := New("+00:00", "x", []Transition{{at, "x"}}); err == nil {
			t.Errorf("accepted %s", at)
		}
	}
	if _, err := New("+00:00", "x", []Transition{{"01:00:00", "a"}, {"01:00:00", "b"}}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := New("+00:00", "", nil); err == nil {
		t.Fatal("empty default")
	}
	if _, err := New("+00:00", "x", make([]Transition, 65)); err == nil {
		t.Fatal("too many entries")
	}
	d := template(t)
	for _, n := range []int64{math.MinInt64, math.MaxInt64, MinSecond - 1, MaxSecond + 1} {
		if _, err := d.ApplicableAt(n); !errors.Is(err, ErrTimeRange) {
			t.Fatal(err)
		}
		if _, _, err := d.LatestBetween(0, n); !errors.Is(err, ErrTimeRange) {
			t.Fatal(err)
		}
	}
	if _, err := d.Generate(0, DaySeconds+1); err == nil {
		t.Fatal("unbounded generation")
	}
	if _, err := d.Generate(1, 0); err == nil {
		t.Fatal("inverted interval")
	}
	var zero Daily
	if _, err := zero.ApplicableAt(0); err == nil {
		t.Fatal("zero template")
	}
}

// The oracle iterates calendar days with time.Date, independent of floorDay and Generate.
func TestLookupMatchesEnumeration(t *testing.T) {
	rng := rand.New(rand.NewSource(20261005))
	for trial := 0; trial < 1000; trial++ {
		offsets := []string{"-14:00", "-03:30", "+00:00", "+05:30", "+09:00", "+14:00"}
		offset := offsets[rng.Intn(len(offsets))]
		count := 1 + rng.Intn(10)
		seen := map[int]bool{}
		transitions := []Transition{}
		slots := []int{0}
		for len(transitions) < count {
			n := 1 + rng.Intn(86399)
			if seen[n] {
				continue
			}
			seen[n] = true
			slots = append(slots, n)
			transitions = append(transitions, Transition{time.Date(2000, 1, 1, n/3600, n/60%60, n%60, 0, time.UTC).Format("15:04:05"), "scene"})
		}
		d, err := New(offset, "default", transitions)
		if err != nil {
			t.Fatal(err)
		}
		offsetSeconds, _ := ParseOffset(offset)
		loc := time.FixedZone("test", int(offsetSeconds))
		cursor := int64(rng.Intn(8*86400) - 4*86400)
		now := int64(rng.Intn(8*86400) - 4*86400)
		var want Occurrence
		found := false
		for day := -6; day <= 6; day++ {
			for _, s := range slots {
				at := time.Date(1970, 1, 1+day, s/3600, s/60%60, s%60, 0, loc).Unix()
				if at > cursor && at <= now && (!found || at > want.Second) {
					scene := "scene"
					if s == 0 {
						scene = "default"
					}
					want = Occurrence{at, scene}
					found = true
				}
			}
		}
		got, ok, err := d.LatestBetween(cursor, now)
		if err != nil || ok != found || (ok && !reflect.DeepEqual(got, want)) {
			t.Fatalf("trial %d: got %+v/%v want %+v/%v error %v", trial, got, ok, want, found, err)
		}
	}
}
