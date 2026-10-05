package store

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuny/glypha/internal/compiler"
)

// Distinct image bytes make mixed package recovery observable, not just a scene-ID check.
func crashCandidate(t *testing.T, c *compiler.Compiler, replacement bool) *compiler.Candidate {
	t.Helper()
	raw, err := os.ReadFile("../../examples/daily/content.json")
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.ReplaceAll(raw, []byte(`"background": {`), []byte(`"background": {"image":"tile.png","fit":"cover",`))
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	shade := color.NRGBA{R: 255, A: 255}
	if replacement {
		shade = color.NRGBA{B: 255, A: 255}
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(x, y, shade)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	candidate, issues := c.Compile(raw, map[string][]byte{"tile.png": encoded.Bytes()})
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	return candidate
}

// The child blocks at an actual service boundary until the parent kills it.
// No Close, rollback defer, or graceful shutdown runs after the checkpoint.
func TestStoreCrashHelper(t *testing.T) {
	path := os.Getenv("GLYPHA_TEST_CRASH_DB")
	if path == "" {
		t.Skip("subprocess only")
	}
	c, _ := setup(t)
	mode := os.Getenv("GLYPHA_TEST_CRASH_MODE")
	now := at("2026-10-05T14:00:00+09:00")
	if mode == "same_scene" {
		now = at("2026-10-06T00:30:00+09:00")
	}
	s, err := Open(path, c, &fakeClock{now})
	if err != nil {
		t.Fatal(err)
	}
	operation := "consume"
	if mode == "publish" {
		operation = "publish"
	}
	s.checkpoint = func(op, phase string) {
		if op != operation || phase != os.Getenv("GLYPHA_TEST_CRASH_PHASE") {
			return
		}
		fmt.Fprintln(os.Stdout, "READY")
		var input [1]byte
		_, _ = os.Stdin.Read(input[:])
		t.Fatal("checkpoint unexpectedly resumed")
	}
	if mode == "publish" {
		_, err = s.Publish(context.Background(), crashCandidate(t, c, true))
	} else {
		_, err = s.Snapshot(context.Background())
	}
	t.Fatalf("checkpoint was not reached: %v", err)
}

func killAtCheckpoint(t *testing.T, path, mode, phase string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStoreCrashHelper$")
	cmd.Env = append(os.Environ(), "GLYPHA_TEST_CRASH_DB="+path, "GLYPHA_TEST_CRASH_MODE="+mode, "GLYPHA_TEST_CRASH_PHASE="+phase)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("checkpoint unavailable: %q %v", line, err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	waited = true
	if ctx.Err() != nil {
		t.Fatal("checkpoint timed out")
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != -1 {
		t.Fatalf("helper was not killed: %v %s", err, stderr.String())
	}
}

func TestCrashRecoveryBoundaries(t *testing.T) {
	for _, mode := range []string{"publish", "advance", "same_scene"} {
		for _, phase := range []string{"before_begin", "after_write", "after_commit", "after_runtime"} {
			t.Run(mode+"/"+phase, func(t *testing.T) {
				c, _ := setup(t)
				old := crashCandidate(t, c, false)
				initialTime := at("2026-10-05T09:00:00+09:00")
				if mode == "same_scene" {
					initialTime = at("2026-10-05T19:00:00+09:00")
				}
				clock := &fakeClock{initialTime}
				path := filepath.Join(t.TempDir(), "crash.db")
				s, err := Open(path, c, clock)
				if err != nil {
					t.Fatal(err)
				}
				initial, err := s.Publish(context.Background(), old)
				if err != nil {
					s.Close()
					t.Fatal(err)
				}
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				killAtCheckpoint(t, path, mode, phase)
				// Restart behind both possible cursors so recovery cannot hide errors by catching up.
				clock.now = initialTime.Add(-time.Hour)
				s, err = Open(path, c, clock)
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				committed := phase == "after_commit" || phase == "after_runtime"
				wantCandidate, wantScene, wantCursor := old, "closed", initialTime.Unix()
				if committed {
					if mode == "publish" {
						wantCandidate = crashCandidate(t, c, true)
						wantScene = "afternoon"
						wantCursor = at("2026-10-05T14:00:00+09:00").Unix()
					}
					if mode == "advance" {
						wantScene = "afternoon"
						wantCursor = at("2026-10-05T13:00:00+09:00").Unix()
					}
					if mode == "same_scene" {
						wantCursor = at("2026-10-06T00:00:00+09:00").Unix()
					}
				}
				changed := s.generation != initial.Generation
				if changed != (committed && mode == "publish") {
					t.Fatal("wrong recovered generation")
				}
				if s.cursor != wantCursor || s.target != wantScene || !bytes.Equal(s.candidate.Bytes(), wantCandidate.Bytes()) {
					t.Fatal("recovered package, target, and cursor are not coherent")
				}
				snapshot, err := s.Snapshot(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				wantBody, wantETag, err := wantCandidate.Snapshot(s.generation, wantScene)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(snapshot.Body, wantBody) || snapshot.ETag != wantETag || s.cursor != wantCursor {
					t.Fatal("snapshot replayed or mixed content after restart")
				}
				var integrity string
				if err := s.db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
					t.Fatalf("SQLite integrity: %q %v", integrity, err)
				}
			})
		}
	}
}
