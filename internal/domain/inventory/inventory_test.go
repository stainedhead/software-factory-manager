package inventory

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
)

var now = time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC)

func TestCanTransition(t *testing.T) {
	all := []OperationStatus{StatusInProgress, StatusSucceeded, StatusFailed, StatusInterrupted}
	for _, from := range all {
		for _, to := range all {
			want := from == StatusInProgress && to != StatusInProgress
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
	if StatusInProgress.Terminal() || !StatusFailed.Terminal() {
		t.Error("Terminal wrong")
	}
}

func TestOperationFinish(t *testing.T) {
	op := Operation{ID: 7, Status: StatusInProgress, LogPath: "a.log"}
	done, err := op.Finish(now, 0, StatusSucceeded, "")
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != StatusSucceeded || *done.ExitCode != 0 || !done.FinishedAt.Equal(now) || done.LogPath != "a.log" {
		t.Errorf("done = %+v", done)
	}
	if op.FinishedAt != nil {
		t.Error("Finish mutated its receiver")
	}
	failed, err := op.Finish(now, 1, StatusFailed, "b.log")
	if err != nil || failed.LogPath != "b.log" {
		t.Errorf("failed = %+v, %v", failed, err)
	}
	if _, err := done.Finish(now, 1, StatusFailed, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("finishing a terminal op: %v", err)
	}
	if _, err := op.Finish(now, 0, StatusInProgress, ""); err == nil {
		t.Error("in_progress -> in_progress allowed")
	}
}

func TestRecordValidate(t *testing.T) {
	good := Record{ID: "jq", State: StateManaged, Backend: "brew-formula", InstalledAt: now, InstalledBy: "j@2026-11-14T19:00:00Z", Scope: ScopeUser}
	adoptedAt := now
	tests := []struct {
		name   string
		mutate func(r *Record)
		want   string
	}{
		{"managed ok", func(*Record) {}, ""},
		{"empty scope ok", func(r *Record) { r.Scope = "" }, ""},
		{"adopted ok", func(r *Record) { r.State, r.AdoptedAt = StateAdopted, &adoptedAt }, ""},
		{"adopted without time", func(r *Record) { r.State = StateAdopted }, "without adopted_at"},
		{"foreign cannot be stored", func(r *Record) { r.State = "foreign" }, `state "foreign" cannot be stored`},
		{"drifted cannot be stored", func(r *Record) { r.State = "drifted" }, "cannot be stored"},
		{"missing id", func(r *Record) { r.ID = "" }, "missing id"},
		{"missing backend", func(r *Record) { r.Backend = "" }, "missing backend"},
		{"missing time", func(r *Record) { r.InstalledAt = time.Time{} }, "missing installed_at"},
		{"missing by", func(r *Record) { r.InstalledBy = "" }, "missing installed_by"},
		{"bad scope", func(r *Record) { r.Scope = "system" }, "unknown scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := good
			tt.mutate(&r)
			err := r.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestOpFor(t *testing.T) {
	tests := map[plan.ActionKind]OperationOp{
		plan.ActionBootstrapHomebrew: OpBootstrap,
		plan.ActionTap:               OpTap,
		plan.ActionTrust:             OpTrust,
		plan.ActionInstall:           OpInstall,
	}
	for k, want := range tests {
		if got := OpFor(k); got != want {
			t.Errorf("OpFor(%s) = %s, want %s", k, got, want)
		}
	}
}

func TestInstallRef(t *testing.T) {
	tests := []struct {
		name    string
		cmds    []plan.Command
		backend string
		ref     string
	}{
		{"cask", []plan.Command{{Argv: []string{"/opt/homebrew/bin/brew", "install", "--cask", "claude-code"}}}, "brew-cask", "brew-cask:claude-code"},
		{"formula after tap", []plan.Command{{Argv: []string{"brew", "tap", "a/b"}}, {Argv: []string{"brew", "install", "gh"}}}, "brew-formula", "brew-formula:gh"},
		{"none", []plan.Command{{Argv: []string{"/bin/bash", "x.sh"}}, {Argv: []string{"npm", "install", "-g", "x"}}}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, ref, ok := InstallRef(tt.cmds)
			if string(b) != tt.backend || ref != tt.ref || ok != (tt.ref != "") {
				t.Errorf("InstallRef = %q %q %v", b, ref, ok)
			}
		})
	}
}

// Review FR-005b: a lock holder whose PID could not be read is "PID unknown".
func TestHolderLabel(t *testing.T) {
	for pid, want := range map[int]string{0: "PID unknown", -1: "PID unknown", 4242: "PID 4242"} {
		if got := HolderLabel(pid); got != want {
			t.Errorf("HolderLabel(%d) = %q, want %q", pid, got, want)
		}
	}
}
