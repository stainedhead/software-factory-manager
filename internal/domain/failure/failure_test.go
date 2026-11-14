package failure

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"usage", Usage("bad flag"), 64},
		{"validation", Validation("bad catalog"), 3},
		{"bootstrap", Bootstrap("running as root"), 4},
		{"operation", Operation("install failed"), 1},
		{"drift", Drift("conflict found"), 2},
		{"nil", nil, 0},
		{"plain error", errors.New("boom"), 1},
		{"wrapped usage", fmt.Errorf("parsing: %w", Usage("bad flag")), 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Errorf("ExitCode = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestErrorExitCodeMethod(t *testing.T) {
	if got := Bootstrap("x").ExitCode(); got != 4 {
		t.Errorf("ExitCode() = %d", got)
	}
}

func TestKindExitCodeUnknown(t *testing.T) {
	if got := Kind("other").ExitCode(); got != 1 {
		t.Errorf("unknown kind exit = %d, want 1", got)
	}
}

func TestErrorsAsThroughWrapping(t *testing.T) {
	inner := io.ErrUnexpectedEOF
	err := fmt.Errorf("loading catalog: %w", Validation("reading %s: %w", "catalog.yaml", inner).WithFix("fix the file"))
	var fe *Error
	if !errors.As(err, &fe) {
		t.Fatal("errors.As did not find *Error")
	}
	if fe.Kind != KindValidation || fe.Fix != "fix the file" {
		t.Errorf("got %+v", fe)
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is did not reach the wrapped cause")
	}
	if !strings.Contains(fe.Msg, "reading catalog.yaml") {
		t.Errorf("Msg = %q", fe.Msg)
	}
	if k, ok := KindOf(err); !ok || k != KindValidation {
		t.Errorf("KindOf = %v, %v", k, ok)
	}
	if _, ok := KindOf(errors.New("x")); ok {
		t.Error("KindOf plain error should be false")
	}
}

func TestErrorText(t *testing.T) {
	e := Validation("catalog is invalid").WithDetails("duplicate id jq", "unknown requires foo")
	want := "catalog is invalid: duplicate id jq; unknown requires foo"
	if e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
	if got := Usage("x").Error(); got != "x" {
		t.Errorf("Error() = %q", got)
	}
	wrapped := Operation("brew failed").Wrap(io.EOF)
	if !errors.Is(wrapped, io.EOF) || wrapped.Error() != "brew failed: EOF" {
		t.Errorf("Wrap: %q", wrapped.Error())
	}
}

func TestWithDoesNotMutate(t *testing.T) {
	base := Usage("x")
	_ = base.WithFix("f")
	_ = base.WithDetails("d")
	_ = base.Wrap(io.EOF)
	if base.Fix != "" || base.Details != nil || base.Err != nil {
		t.Errorf("base mutated: %+v", base)
	}
}

func TestWorst(t *testing.T) {
	op := Operation("op")
	val := Validation("val")
	boot := Bootstrap("boot")
	use := Usage("use")
	drift := Drift("drift")
	plain := errors.New("plain")
	tests := []struct {
		name string
		errs []error
		want error
	}{
		{"none", nil, nil},
		{"all nil", []error{nil, nil}, nil},
		{"single", []error{op}, op},
		{"usage beats all", []error{op, val, use, boot}, use},
		{"bootstrap beats validation", []error{val, boot, op}, boot},
		{"validation beats operation", []error{op, val}, val},
		{"operation beats drift", []error{drift, op}, op},
		{"plain counts as operation, first wins", []error{plain, op}, plain},
		{"drift alone", []error{nil, drift}, drift},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Worst(tt.errs...); !errors.Is(got, tt.want) || (got == nil) != (tt.want == nil) {
				t.Errorf("Worst = %v, want %v", got, tt.want)
			}
		})
	}
}
