// Package store atomically publishes content and durable scheduling progress.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	"github.com/kuny/glypha/internal/compiler"
	_ "modernc.org/sqlite"
)

type Clock interface{ Now() time.Time }
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

var ErrUnavailable = errors.New("store unavailable; restart required")

type Publication struct {
	Generation string `json:"generation"`
	Scene      string `json:"scene"`
}
type Snapshot struct {
	Body []byte
	ETag string
}
type Store struct {
	mu                 sync.Mutex
	db                 *sql.DB
	clock              Clock
	candidate          *compiler.Candidate
	generation, target string
	cursor             int64
	failed             bool
}

func Open(path string, c *compiler.Compiler, clock Clock) (*Store, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: absolute}
	q := u.Query()
	q.Add("_pragma", "journal_mode(DELETE)")
	q.Add("_pragma", "synchronous(EXTRA)")
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, clock: clock}
	if err = s.initialize(c); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) initialize(c *compiler.Compiler) error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != 0 && version != 1 {
		return fmt.Errorf("unsupported database version %d", version)
	}
	if version == 0 {
		var tables int
		if err := s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
			return err
		}
		if tables != 0 {
			return fmt.Errorf("unrecognized database schema")
		}
		if _, err := s.db.Exec(`BEGIN; CREATE TABLE current (id INTEGER PRIMARY KEY CHECK(id=1), generation TEXT NOT NULL, package BLOB NOT NULL, cursor INTEGER NOT NULL, target TEXT NOT NULL); PRAGMA user_version=1; COMMIT;`); err != nil {
			return err
		}
	}
	var journal string
	var synchronous, foreign int
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		return err
	}
	if err := s.db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		return err
	}
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&foreign); err != nil {
		return err
	}
	if journal != "delete" || synchronous != 3 || foreign != 1 {
		return fmt.Errorf("unsafe database settings")
	}
	var raw []byte
	err := s.db.QueryRow("SELECT generation,package,cursor,target FROM current WHERE id=1").Scan(&s.generation, &raw, &s.cursor, &s.target)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	candidate, err := c.Restore(raw)
	if err != nil {
		return err
	}
	occurrence, err := candidate.ApplicableAt(s.cursor)
	if err != nil {
		return err
	}
	if occurrence.Scene != s.target || s.generation == "" {
		return fmt.Errorf("stored target and cursor disagree")
	}
	s.candidate = candidate
	return nil
}
func (s *Store) Close() error  { s.mu.Lock(); defer s.mu.Unlock(); s.failed = true; return s.db.Close() }
func (s *Store) Healthy() bool { s.mu.Lock(); defer s.mu.Unlock(); return !s.failed }
func (s *Store) Publish(ctx context.Context, c *compiler.Candidate) (Publication, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed {
		return Publication{}, ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Publication{}, err
	}
	defer tx.Rollback()
	now := s.clock.Now().Unix()
	o, err := c.ApplicableAt(now)
	if err != nil {
		return Publication{}, err
	}
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		return Publication{}, err
	}
	generation := fmt.Sprintf("%x", token)
	_, err = tx.ExecContext(ctx, `INSERT INTO current VALUES(1,?,?,?,?) ON CONFLICT(id) DO UPDATE SET generation=excluded.generation,package=excluded.package,cursor=excluded.cursor,target=excluded.target`, generation, c.Bytes(), now, o.Scene)
	if err != nil {
		return Publication{}, err
	}
	if err = tx.Commit(); err != nil {
		s.failed = true
		s.candidate = nil
		return Publication{}, err
	}
	s.candidate = c
	s.generation = generation
	s.target = o.Scene
	s.cursor = now
	return Publication{generation, o.Scene}, nil
}
func (s *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed {
		return Snapshot{}, ErrUnavailable
	}
	if s.candidate == nil {
		return Snapshot{}, nil
	}
	o, due, err := s.candidate.LatestBetween(s.cursor, s.clock.Now().Unix())
	if err != nil {
		return Snapshot{}, err
	}
	if due {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return Snapshot{}, err
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(ctx, "UPDATE current SET cursor=?,target=? WHERE id=1", o.Second, o.Scene); err != nil {
			return Snapshot{}, err
		}
		if err = tx.Commit(); err != nil {
			s.failed = true
			s.candidate = nil
			return Snapshot{}, err
		}
		s.cursor = o.Second
		s.target = o.Scene
	}
	body, etag, err := s.candidate.Snapshot(s.generation, s.target)
	return Snapshot{body, etag}, err
}
