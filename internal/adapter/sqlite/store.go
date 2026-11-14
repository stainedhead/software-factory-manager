// Package sqlite is facman's inventory store: an embedded SQLite database
// (modernc.org/sqlite, no cgo) at <state dir>/state.db with numbered
// migrations tracked in meta.schema_version. It implements the Inventory
// ports of usecase/scan, usecase/execute and usecase/status. Times are
// stored as RFC 3339 UTC text so the file is readable with the system
// sqlite3 (FR-062).
package sqlite

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver

	"github.com/stainedhead/software-factory-manager/internal/domain/appinfo"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
)

// SchemaVersion is the newest migration this build knows.
const SchemaVersion = 1

// sqliteMagic starts every SQLite database file.
var sqliteMagic = []byte("SQLite format 3\x00")

// migrations[i] moves the schema from version i to i+1.
var migrations = []string{
	`CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE packages(
  id TEXT PRIMARY KEY, state TEXT NOT NULL CHECK(state IN ('managed','adopted')),
  backend TEXT NOT NULL, backend_ref TEXT NOT NULL,
  installed_version TEXT, install_path TEXT,
  installed_at TEXT NOT NULL, installed_by TEXT NOT NULL, adopted_at TEXT,
  manifest_rev TEXT, scope TEXT NOT NULL DEFAULT 'user', bootstrap INTEGER NOT NULL DEFAULT 0);
CREATE TABLE operations(
  id INTEGER PRIMARY KEY AUTOINCREMENT, started_at TEXT NOT NULL, finished_at TEXT,
  package_id TEXT NOT NULL, op TEXT NOT NULL, commands_json TEXT NOT NULL,
  exit_code INTEGER, status TEXT NOT NULL, log_path TEXT, consent_by TEXT NOT NULL);
CREATE TABLE detections(
  scanned_at TEXT NOT NULL, package_id TEXT NOT NULL, found INTEGER NOT NULL,
  version TEXT, provenance TEXT, path TEXT);
CREATE TABLE services(
  package_id TEXT PRIMARY KEY, manager TEXT, unit_path TEXT, port INTEGER,
  exposure TEXT, last_health TEXT, last_backup_path TEXT);
CREATE TABLE tap_trust(
  package_id TEXT NOT NULL, tap TEXT NOT NULL, item TEXT NOT NULL,
  granted_at TEXT NOT NULL, operation_id INTEGER REFERENCES operations(id));
CREATE INDEX operations_status ON operations(status);
CREATE INDEX detections_pkg ON detections(package_id, scanned_at);`,
}

// Store is an open inventory database.
type Store struct {
	db   *sql.DB
	path string
	now  func() time.Time
}

// Open opens (creating if needed) and migrates the database at path. now
// stamps finished_at and interrupted rows; nil means time.Now. A file that
// is not a SQLite database, or one written by a newer facman, is a
// bootstrap error naming the path, and the file is left untouched.
func Open(ctx context.Context, path string, now func() time.Time) (*Store, error) {
	if now == nil {
		now = time.Now
	}
	if err := checkHeader(path); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, bootstrapErr(path, err)
	}
	db.SetMaxOpenConns(1) // one writer; also keeps an in-process view consistent
	s := &Store{db: db, path: path, now: now}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func bootstrapErr(path string, err error) error {
	return failure.Bootstrap("state database %s cannot be opened: %w", path, err).
		WithFix("move the file aside (facman never deletes it) and run again")
}

// checkHeader refuses to touch a non-empty file that is not SQLite.
func checkHeader(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return bootstrapErr(path, err)
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, len(sqliteMagic))
	n, err := io.ReadFull(f, head)
	if n == 0 && (errors.Is(err, io.EOF) || err == nil) {
		return nil // empty file: SQLite initialises it
	}
	if !bytes.Equal(head[:n], sqliteMagic) {
		return failure.Bootstrap("state database %s is not a SQLite database (corrupt?)", path).
			WithFix("move the file aside (facman never deletes it) and run again")
	}
	return nil
}

// querier is what schemaVersion reads through: the pool, or the one
// connection that holds a migration's write lock.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// migrate brings the schema to SchemaVersion. Each step runs in its own
// BEGIN IMMEDIATE transaction that re-reads the version inside the write
// lock, so two first launches at the same moment never both create the
// tables (review FR-005a): the second waits (busy_timeout), then sees the
// work done and skips it.
func (s *Store) migrate(ctx context.Context) error {
	current, err := schemaVersion(ctx, s.db)
	if err != nil {
		return bootstrapErr(s.path, err)
	}
	for {
		if current > SchemaVersion {
			return failure.Bootstrap("state database %s was written by a newer %s (schema %d; this build knows %d)",
				s.path, appinfo.Name, current, SchemaVersion).
				WithFix("upgrade " + appinfo.Name + "; it never migrates state down")
		}
		if current == SchemaVersion {
			return nil
		}
		if current, err = s.step(ctx); err != nil {
			return bootstrapErr(s.path, err)
		}
	}
}

// step runs the next migration under BEGIN IMMEDIATE and returns the schema
// version it leaves. A version that is already current (or newer) when the
// lock is taken is returned unchanged, with nothing applied.
func (s *Store) step(ctx context.Context) (int, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("starting migration: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return 0, fmt.Errorf("starting migration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `ROLLBACK`)
		}
	}()
	from, err := schemaVersion(ctx, conn)
	if err != nil {
		return 0, err
	}
	if from >= SchemaVersion {
		return from, nil // another process finished it while we waited
	}
	if _, err := conn.ExecContext(ctx, migrations[from]); err != nil {
		return 0, fmt.Errorf("migration %d: %w", from+1, err)
	}
	if from == 0 {
		id, err := newUUID()
		if err != nil {
			return 0, err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES ('machine_id', ?), ('last_manifest_rev', '')`, id); err != nil {
			return 0, fmt.Errorf("seeding meta: %w", err)
		}
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES ('schema_version', ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, strconv.Itoa(from+1)); err != nil {
		return 0, fmt.Errorf("recording schema version: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return 0, fmt.Errorf("committing migration %d: %w", from+1, err)
	}
	committed = true
	return from + 1, nil
}

// schemaVersion is 0 for a fresh database.
func schemaVersion(ctx context.Context, q querier) (int, error) {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'meta'`).Scan(&n); err != nil {
		return 0, fmt.Errorf("reading schema: %w", err)
	}
	if n == 0 {
		return 0, nil
	}
	var v string
	err := q.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading schema version: %w", err)
	}
	version, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("bad schema_version %q: %w", v, err)
	}
	return version, nil
}

// Meta returns a meta value ("" when absent).
func (s *Store) Meta(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading meta %s: %w", key, err)
	}
	return v, nil
}

// SetMeta writes a meta value (for example last_manifest_rev).
func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
		return fmt.Errorf("writing meta %s: %w", key, err)
	}
	return nil
}

// Path returns the database file.
func (s *Store) Path() string { return s.path }

// Close closes the database.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", s.path, err)
	}
	return nil
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generating machine id: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
