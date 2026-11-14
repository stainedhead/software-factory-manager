package process

import "testing"

func TestResultSuccess(t *testing.T) {
	if !(Result{}).Success() || (Result{ExitCode: 1}).Success() {
		t.Error("Success wrong")
	}
}
