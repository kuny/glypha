package store

import (
	"context"
	"encoding/json"
	"github.com/kuny/glypha/internal/compiler"
	"github.com/kuny/glypha/internal/content"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }
func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
func setup(t *testing.T) (*compiler.Compiler, *compiler.Candidate) {
	t.Helper()
	c, err := compiler.New("../../renderer/public/fonts/noto-sans-jp/NotoSansJP.ttf", content.Canvas{Width: 1920, Height: 1080})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../examples/daily/content.json")
	if err != nil {
		t.Fatal(err)
	}
	candidate, issues := c.Compile(raw, nil)
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	return c, candidate
}
func target(t *testing.T, snapshot Snapshot) string {
	t.Helper()
	var e compiler.Envelope
	if err := json.Unmarshal(snapshot.Body, &e); err != nil {
		t.Fatal(err)
	}
	return e.Scene
}
func TestRestartRollbackAndSnapshots(t *testing.T) {
	c, candidate := setup(t)
	path := filepath.Join(t.TempDir(), "glypha.db")
	clock := &fakeClock{at("2026-10-05T09:00:00+09:00")}
	s, err := Open(path, c, clock)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := s.Snapshot(context.Background())
	if err != nil || len(empty.Body) != 0 {
		t.Fatal("nonempty initial state")
	}
	pub, err := s.Publish(context.Background(), candidate)
	if err != nil || pub.Scene != "closed" {
		t.Fatalf("%+v %v", pub, err)
	}
	initial, _ := s.Snapshot(context.Background())
	clock.now = at("2026-10-05T14:00:00+09:00")
	latest, err := s.Snapshot(context.Background())
	if err != nil || target(t, latest) != "afternoon" {
		t.Fatal(err)
	}
	cursor := s.cursor
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	clock.now = at("2026-10-05T11:00:00+09:00")
	s, err = Open(path, c, clock)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, err := s.Snapshot(context.Background())
	if err != nil || restored.ETag != latest.ETag || s.cursor != cursor {
		t.Fatal("replayed after restart", err)
	}
	clock.now = at("2026-10-06T09:00:00+09:00")
	_, err = s.Publish(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	next, _ := s.Snapshot(context.Background())
	if next.ETag == initial.ETag {
		t.Fatal("generation was reused")
	}
	if target(t, initial) != "closed" || target(t, latest) != "afternoon" {
		t.Fatal("old response mutated")
	}
}
func TestSameSceneConsumesAndFailedWritePreservesDurableState(t *testing.T) {
	c, candidate := setup(t)
	path := filepath.Join(t.TempDir(), "test.db")
	clock := &fakeClock{at("2026-10-05T19:00:00+09:00")}
	s, err := Open(path, c, clock)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Publish(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot(context.Background())
	cursor := s.cursor
	clock.now = at("2026-10-06T00:00:00+09:00")
	after, err := s.Snapshot(context.Background())
	if err != nil || after.ETag != before.ETag || s.cursor <= cursor {
		t.Fatal("same-scene cursor not persisted")
	}
	_, err = s.db.Exec(`CREATE TRIGGER reject_update BEFORE UPDATE ON current BEGIN SELECT RAISE(ABORT, 'injected'); END`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Publish(context.Background(), candidate)
	if err == nil {
		t.Fatal("injected write succeeded")
	}
	s.Close()
	s, err = Open(path, c, clock)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	recovered, err := s.Snapshot(context.Background())
	if err != nil || recovered.ETag != after.ETag {
		t.Fatal("failed publication changed durable state")
	}
}
func TestUnknownSchemaFails(t *testing.T) {
	c, _ := setup(t)
	path := filepath.Join(t.TempDir(), "test.db")
	clock := &fakeClock{at("2026-10-05T09:00:00+09:00")}
	s, err := Open(path, c, clock)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec("PRAGMA user_version=99")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err = Open(path, c, clock); err == nil {
		t.Fatal("unknown schema accepted")
	}
}

func TestCommitFailureStopsServing(t *testing.T) {
	c, candidate := setup(t)
	clock := &fakeClock{at("2026-10-05T09:00:00+09:00")}
	s, err := Open(filepath.Join(t.TempDir(), "test.db"), c, clock)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Publish(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	// Deferred constraints fail at Commit, after the UPDATE itself succeeds.
	_, err = s.db.Exec(`CREATE TABLE parent(id INTEGER PRIMARY KEY);
 CREATE TABLE child(id INTEGER REFERENCES parent(id) DEFERRABLE INITIALLY DEFERRED);
 CREATE TRIGGER fail_commit AFTER UPDATE ON current BEGIN INSERT INTO child VALUES(1); END;`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(context.Background(), candidate); err == nil {
		t.Fatal("commit unexpectedly succeeded")
	}
	if s.Healthy() {
		t.Fatal("store remained healthy after commit failure")
	}
	if _, err := s.Snapshot(context.Background()); err != ErrUnavailable {
		t.Fatalf("state served after commit failure: %v", err)
	}
}

// The helper exits without closing SQLite, simulating process loss at a transaction boundary.
func TestCrashRecovery(t *testing.T) {
	if path := os.Getenv("GLYPHA_CRASH_TEST_DB"); path != "" {
		c, candidate := setup(t)
		clock := &fakeClock{at("2026-10-05T14:00:00+09:00")}
		s, err := Open(path, c, clock)
		if err != nil {
			t.Fatal(err)
		}
		if os.Getenv("GLYPHA_CRASH_TEST_PHASE") == "before" {
			tx, err := s.db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec("UPDATE current SET generation='uncommitted',cursor=?,target='afternoon' WHERE id=1", clock.now.Unix()); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := s.Publish(context.Background(), candidate); err != nil {
				t.Fatal(err)
			}
		}
		os.Exit(73)
	}
	for _, phase := range []string{"before", "after"} {
		t.Run(phase, func(t *testing.T) {
			c, candidate := setup(t)
			path := filepath.Join(t.TempDir(), "crash.db")
			clock := &fakeClock{at("2026-10-05T09:00:00+09:00")}
			s, err := Open(path, c, clock)
			if err != nil {
				t.Fatal(err)
			}
			initial, err := s.Publish(context.Background(), candidate)
			if err != nil {
				t.Fatal(err)
			}
			s.Close()
			cmd := exec.Command(os.Args[0], "-test.run=^TestCrashRecovery$")
			cmd.Env = append(os.Environ(), "GLYPHA_CRASH_TEST_DB="+path, "GLYPHA_CRASH_TEST_PHASE="+phase)
			output, err := cmd.CombinedOutput()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 73 {
				t.Fatalf("helper: %v %s", err, output)
			}
			s, err = Open(path, c, clock)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			snapshot, err := s.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if phase == "before" {
				if s.generation != initial.Generation || target(t, snapshot) != "closed" {
					t.Fatal("uncommitted state survived")
				}
			} else if s.generation == initial.Generation || target(t, snapshot) != "afternoon" {
				t.Fatal("committed state lost or replayed")
			}
		})
	}
}
