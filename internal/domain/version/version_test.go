package version

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantParts []int
		wantPre   string
		wantErr   bool
	}{
		{"plain", "2.1.4", []int{2, 1, 4}, "", false},
		{"leading v", "v24.11.0", []int{24, 11, 0}, "", false},
		{"pre-release", "1.7.1-rc1", []int{1, 7, 1}, "rc1", false},
		{"after regex", "2.17.0", []int{2, 17, 0}, "", false},
		{"single", "3", []int{3}, "", false},
		{"build suffix", "1.2.3+build5", []int{1, 2, 3}, "build5", false},
		{"letters suffix", "2.45.2a", []int{2, 45, 2}, "a", false},
		{"surrounding space", " 1.0 \n", []int{1, 0}, "", false},
		{"garbage", "garbage", nil, "", true},
		{"empty", "", nil, "", true},
		{"unparsed prefix", "aws-cli/2.17.0", nil, "", true},
		{"trailing dot", "1.", nil, "", true},
		{"overflow", "99999999999999999999999", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) = %+v, want error", tt.in, got)
				}
				if !errors.Is(err, ErrInvalid) {
					t.Errorf("error %v does not wrap ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			if !equalInts(got.Parts, tt.wantParts) || got.Pre != tt.wantPre {
				t.Errorf("Parse(%q) = %+v, want parts %v pre %q", tt.in, got, tt.wantParts, tt.wantPre)
			}
			if got.Raw != tt.in {
				t.Errorf("Raw = %q, want %q", got.Raw, tt.in)
			}
			if !got.Known() {
				t.Error("Known() = false for a parsed version")
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2", "1.2.0", 0},
		{"1.2.4", "1.2.3", 1},
		{"1.10", "1.9", 1},
		{"v24.11.0", "24.11", 0},
		{"24.10.9", "24.11", -1},
		{"1.7.1-rc1", "1.7.1", -1},
		{"1.7.1", "1.7.1-rc1", 1},
		{"1.7.1-rc1", "1.7.1-rc2", -1},
		{"1.7.1-rc2", "1.7.1-rc1", 1},
		{"2", "1.99.99", 1},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			if got := Compare(MustParse(t, tt.a), MustParse(t, tt.b)); got != tt.want {
				t.Errorf("Compare(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestAtLeast(t *testing.T) {
	floor := Version{Raw: "24.11", Parts: []int{24, 11}}
	tests := []struct {
		name string
		v    Version
		want bool
	}{
		{"equal", Version{Parts: []int{24, 11, 0}}, true},
		{"above", Version{Parts: []int{25}}, true},
		{"below", Version{Parts: []int{22, 9}}, false},
		{"unknown never satisfies", Version{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AtLeast(tt.v, floor); got != tt.want {
				t.Errorf("AtLeast(%v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
	if AtLeast(Version{Parts: []int{1}}, Version{}) {
		t.Error("an unknown minimum must not be satisfied")
	}
}

func TestSatisfies(t *testing.T) {
	tests := []struct {
		raw, min string
		want     bool
	}{
		{"24.11.0", "24.11", true},
		{"v22.1.0", "24.11", false},
		{"", "24.11", false},
		{"garbage", "1", false},
		{"1.0", "garbage", false},
	}
	for _, tt := range tests {
		t.Run(tt.raw+">="+tt.min, func(t *testing.T) {
			if got := Satisfies(tt.raw, tt.min); got != tt.want {
				t.Errorf("Satisfies(%q, %q) = %v, want %v", tt.raw, tt.min, got, tt.want)
			}
		})
	}
}

func TestString(t *testing.T) {
	if got := MustParse(t, "v1.2.3-rc1").String(); got != "v1.2.3-rc1" {
		t.Errorf("String() = %q", got)
	}
	if got := (Version{}).String(); got != "unknown" {
		t.Errorf("unknown String() = %q", got)
	}
}

// MustParse parses s or fails the test.
func MustParse(t *testing.T, s string) Version {
	t.Helper()
	v, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return v
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
