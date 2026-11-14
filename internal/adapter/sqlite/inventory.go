package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/status"
)

var (
	_ scan.Inventory    = (*Store)(nil)
	_ execute.Inventory = (*Store)(nil)
	_ status.Inventory  = (*Store)(nil)
)

// DetectionScansKept is how many scans per package the detections table keeps.
const DetectionScansKept = 10

// ErrNotFound is returned for an unknown operation ID.
var ErrNotFound = errors.New("not found")

const timeFormat = time.RFC3339

func formatTime(t time.Time) string { return t.UTC().Format(timeFormat) }

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeFormat, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing time %q: %w", s, err)
	}
	return t, nil
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// commandJSON is the stored shape of plan.Command in operations.commands_json.
type commandJSON struct {
	Argv        []string `json:"argv"`
	EnvNames    []string `json:"env_names,omitempty"`
	NeedsSudo   bool     `json:"needs_sudo,omitempty"`
	Unverified  bool     `json:"unverified,omitempty"`
	Interactive bool     `json:"interactive,omitempty"`
	Mutating    bool     `json:"mutating,omitempty"`
}

func encodeCommands(cmds []plan.Command) (string, error) {
	out := make([]commandJSON, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, commandJSON(c))
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("encoding commands: %w", err)
	}
	return string(b), nil
}

func decodeCommands(s string) ([]plan.Command, error) {
	var in []commandJSON
	if err := json.Unmarshal([]byte(s), &in); err != nil {
		return nil, fmt.Errorf("decoding commands_json: %w", err)
	}
	out := make([]plan.Command, 0, len(in))
	for _, c := range in {
		out = append(out, plan.Command(c))
	}
	return out, nil
}

// Records returns every packages row, ordered by id.
func (s *Store) Records(ctx context.Context) ([]inventory.Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, state, backend, backend_ref, installed_version, install_path,
installed_at, installed_by, adopted_at, manifest_rev, scope, bootstrap FROM packages ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("reading packages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []inventory.Record
	for rows.Next() {
		var r inventory.Record
		var version, path, adopted, rev sql.NullString
		var installedAt string
		var bootstrap int
		if err := rows.Scan(&r.ID, &r.State, &r.Backend, &r.BackendRef, &version, &path, &installedAt,
			&r.InstalledBy, &adopted, &rev, &r.Scope, &bootstrap); err != nil {
			return nil, fmt.Errorf("reading packages row: %w", err)
		}
		r.InstalledVersion, r.InstallPath, r.ManifestRev = version.String, path.String, rev.String
		r.Bootstrap = bootstrap != 0
		if r.InstalledAt, err = parseTime(installedAt); err != nil {
			return nil, fmt.Errorf("packages %s: %w", r.ID, err)
		}
		if adopted.Valid {
			t, err := parseTime(adopted.String)
			if err != nil {
				return nil, fmt.Errorf("packages %s: %w", r.ID, err)
			}
			r.AdoptedAt = &t
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading packages: %w", err)
	}
	return out, nil
}

// UpsertRecord validates and writes a packages row.
func (s *Store) UpsertRecord(ctx context.Context, r inventory.Record) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("upserting record: %w", err)
	}
	if r.Scope == "" {
		r.Scope = inventory.ScopeUser
	}
	bootstrap := 0
	if r.Bootstrap {
		bootstrap = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO packages(id, state, backend, backend_ref, installed_version, install_path,
installed_at, installed_by, adopted_at, manifest_rev, scope, bootstrap) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET state=excluded.state, backend=excluded.backend, backend_ref=excluded.backend_ref,
installed_version=excluded.installed_version, install_path=excluded.install_path, installed_at=excluded.installed_at,
installed_by=excluded.installed_by, adopted_at=excluded.adopted_at, manifest_rev=excluded.manifest_rev,
scope=excluded.scope, bootstrap=excluded.bootstrap`,
		r.ID, string(r.State), string(r.Backend), r.BackendRef, nullString(r.InstalledVersion), nullString(r.InstallPath),
		formatTime(r.InstalledAt), r.InstalledBy, nullTime(r.AdoptedAt), nullString(r.ManifestRev), r.Scope, bootstrap)
	if err != nil {
		return fmt.Errorf("upserting record %s: %w", r.ID, err)
	}
	return nil
}

// StartOperation appends an in_progress operations row and returns its id.
func (s *Store) StartOperation(ctx context.Context, op inventory.Operation) (int64, error) {
	if op.Status == "" {
		op.Status = inventory.StatusInProgress
	}
	if op.Status != inventory.StatusInProgress {
		return 0, fmt.Errorf("starting operation: %w: status must be in_progress, got %s", inventory.ErrInvalid, op.Status)
	}
	if op.ConsentBy == "" || op.PackageID == "" || op.Op == "" || op.StartedAt.IsZero() {
		return 0, fmt.Errorf("starting operation: %w: package, op, consent and start time are required", inventory.ErrInvalid)
	}
	cmds, err := encodeCommands(op.Commands)
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO operations(started_at, package_id, op, commands_json, status, log_path, consent_by)
VALUES (?,?,?,?,?,?,?)`, formatTime(op.StartedAt), op.PackageID, string(op.Op), cmds, string(op.Status), nullString(op.LogPath), op.ConsentBy)
	if err != nil {
		return 0, fmt.Errorf("starting operation for %s: %w", op.PackageID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("starting operation for %s: %w", op.PackageID, err)
	}
	return id, nil
}

// FinishOperation moves an in_progress row to a terminal status with its
// exit code, finish time (from the store's clock) and log path ("" keeps the
// path written at start).
func (s *Store) FinishOperation(ctx context.Context, id int64, exitCode int, st inventory.OperationStatus, logPath string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("finishing operation %d: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	var current string
	err = tx.QueryRowContext(ctx, `SELECT status FROM operations WHERE id = ?`, id).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("finishing operation %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("finishing operation %d: %w", id, err)
	}
	if !inventory.CanTransition(inventory.OperationStatus(current), st) {
		return fmt.Errorf("finishing operation %d: %w: %s -> %s", id, inventory.ErrInvalid, current, st)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE operations SET finished_at = ?, exit_code = ?, status = ?,
log_path = COALESCE(?, log_path) WHERE id = ?`, formatTime(s.now()), exitCode, string(st), nullString(logPath), id); err != nil {
		return fmt.Errorf("finishing operation %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("finishing operation %d: %w", id, err)
	}
	return nil
}

const operationColumns = `id, started_at, finished_at, package_id, op, commands_json, exit_code, status, log_path, consent_by`

func scanOperation(sc interface{ Scan(...any) error }) (inventory.Operation, error) {
	var op inventory.Operation
	var started, cmds string
	var finished, logPath sql.NullString
	var exit sql.NullInt64
	if err := sc.Scan(&op.ID, &started, &finished, &op.PackageID, &op.Op, &cmds, &exit, &op.Status, &logPath, &op.ConsentBy); err != nil {
		return op, fmt.Errorf("reading operations row: %w", err)
	}
	var err error
	if op.StartedAt, err = parseTime(started); err != nil {
		return op, err
	}
	if finished.Valid {
		t, err := parseTime(finished.String)
		if err != nil {
			return op, err
		}
		op.FinishedAt = &t
	}
	if exit.Valid {
		code := int(exit.Int64)
		op.ExitCode = &code
	}
	op.LogPath = logPath.String
	if op.Commands, err = decodeCommands(cmds); err != nil {
		return op, err
	}
	return op, nil
}

// Operations returns every operations row, oldest first.
func (s *Store) Operations(ctx context.Context) ([]inventory.Operation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+operationColumns+` FROM operations ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("reading operations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []inventory.Operation
	for rows.Next() {
		op, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading operations: %w", err)
	}
	return out, nil
}

// MarkInterrupted moves every in_progress row to interrupted (finished now)
// and returns those rows as they are after the change. Only call it while
// holding the state lock.
func (s *Store) MarkInterrupted(ctx context.Context) ([]inventory.Operation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("marking interrupted operations: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT `+operationColumns+` FROM operations WHERE status = ? ORDER BY id`, string(inventory.StatusInProgress))
	if err != nil {
		return nil, fmt.Errorf("marking interrupted operations: %w", err)
	}
	var ops []inventory.Operation
	for rows.Next() {
		op, err := scanOperation(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		ops = append(ops, op)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("marking interrupted operations: %w", err)
	}
	now := s.now().UTC().Truncate(time.Second)
	for i := range ops {
		if _, err := tx.ExecContext(ctx, `UPDATE operations SET status = ?, finished_at = ? WHERE id = ?`,
			string(inventory.StatusInterrupted), formatTime(now), ops[i].ID); err != nil {
			return nil, fmt.Errorf("marking operation %d interrupted: %w", ops[i].ID, err)
		}
		ops[i].Status = inventory.StatusInterrupted
		ops[i].FinishedAt = &now
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("marking interrupted operations: %w", err)
	}
	return ops, nil
}

// SaveDetections appends a scan's snapshot rows and keeps only the last
// DetectionScansKept scans per package.
func (s *Store) SaveDetections(ctx context.Context, snaps []inventory.DetectionSnapshot) error {
	if len(snaps) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saving detections: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	pkgs := map[string]bool{}
	for _, d := range snaps {
		found := 0
		if d.Found {
			found = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO detections(scanned_at, package_id, found, version, provenance, path) VALUES (?,?,?,?,?,?)`,
			formatTime(d.ScannedAt), d.PackageID, found, nullString(d.Version), nullString(string(d.Provenance)), nullString(d.Path)); err != nil {
			return fmt.Errorf("saving detection for %s: %w", d.PackageID, err)
		}
		pkgs[d.PackageID] = true
	}
	for pkg := range pkgs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM detections WHERE package_id = ?1 AND scanned_at NOT IN (
SELECT DISTINCT scanned_at FROM detections WHERE package_id = ?1 ORDER BY scanned_at DESC LIMIT ?2)`, pkg, DetectionScansKept); err != nil {
			return fmt.Errorf("pruning detections for %s: %w", pkg, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("saving detections: %w", err)
	}
	return nil
}

// Detections returns the stored snapshot rows for a package, newest first.
func (s *Store) Detections(ctx context.Context, pkgID string) ([]inventory.DetectionSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT scanned_at, package_id, found, version, provenance, path FROM detections
WHERE package_id = ? ORDER BY scanned_at DESC, rowid`, pkgID)
	if err != nil {
		return nil, fmt.Errorf("reading detections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []inventory.DetectionSnapshot
	for rows.Next() {
		var d inventory.DetectionSnapshot
		var at string
		var found int
		var version, prov, path sql.NullString
		if err := rows.Scan(&at, &d.PackageID, &found, &version, &prov, &path); err != nil {
			return nil, fmt.Errorf("reading detections row: %w", err)
		}
		if d.ScannedAt, err = parseTime(at); err != nil {
			return nil, err
		}
		d.Found, d.Version, d.Provenance, d.Path = found != 0, version.String, catalog.Provenance(prov.String), path.String
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading detections: %w", err)
	}
	return out, nil
}

// RecordTapTrust appends a tap_trust row.
func (s *Store) RecordTapTrust(ctx context.Context, g inventory.TapTrustGrant) error {
	var opID any
	if g.OperationID != 0 {
		opID = g.OperationID
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO tap_trust(package_id, tap, item, granted_at, operation_id) VALUES (?,?,?,?,?)`,
		g.PackageID, g.Tap, g.Item, formatTime(g.GrantedAt), opID); err != nil {
		return fmt.Errorf("recording tap trust for %s: %w", g.Item, err)
	}
	return nil
}

// TapTrustGrants returns every tap_trust row in grant order.
func (s *Store) TapTrustGrants(ctx context.Context) ([]inventory.TapTrustGrant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT package_id, tap, item, granted_at, operation_id FROM tap_trust ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("reading tap_trust: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []inventory.TapTrustGrant
	for rows.Next() {
		var g inventory.TapTrustGrant
		var at string
		var opID sql.NullInt64
		if err := rows.Scan(&g.PackageID, &g.Tap, &g.Item, &at, &opID); err != nil {
			return nil, fmt.Errorf("reading tap_trust row: %w", err)
		}
		if g.GrantedAt, err = parseTime(at); err != nil {
			return nil, err
		}
		g.OperationID = opID.Int64
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading tap_trust: %w", err)
	}
	return out, nil
}
