package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/adapter/execrunner"
	domain "github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/testkit"
	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testkit.RemoveBuild()
	os.Exit(code)
}

const (
	ubuntu = "PRETTY_NAME=\"Ubuntu 24.04 LTS\"\nNAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\n"
	debian = "ID=debian\n"
	fedora = "NAME=Fedora\nID=fedora\n"
	rhel   = "ID=\"rhel\"\nID_LIKE=\"fedora\"\n"
	alpine = "ID=alpine\n"
	wslVer = "Linux version 5.15.153.1-microsoft-standard-WSL2 (root@x) #1 SMP\n"
	linVer = "Linux version 6.8.0-45-generic (buildd@lcy02) #45-Ubuntu SMP\n"
)

func fixtures(t *testing.T, procVersion, osRelease string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	pv, osr := filepath.Join(dir, "version"), filepath.Join(dir, "os-release")
	if procVersion != "" {
		if err := os.WriteFile(pv, []byte(procVersion), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if osRelease != "" {
		if err := os.WriteFile(osr, []byte(osRelease), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return pv, osr
}

func glibcWorld(getconf, ldd *fakeworld.Rule) fakeworld.World {
	var w fakeworld.World
	if getconf != nil {
		w.Rules = append(w.Rules, *getconf)
	}
	if ldd != nil {
		w.Rules = append(w.Rules, *ldd)
	}
	return w
}

func TestProbeLinux(t *testing.T) {
	getconf239 := &fakeworld.Rule{Cmd: "getconf", Args: []string{"GNU_LIBC_VERSION"}, Stdout: "glibc 2.39\n"}
	getconf235 := &fakeworld.Rule{Cmd: "getconf", Args: []string{"GNU_LIBC_VERSION"}, Stdout: "glibc 2.35\n"}
	getconfFail := &fakeworld.Rule{Cmd: "getconf", Exit: 1, Stderr: "getconf: Unrecognized variable\n"}
	lddGlibc := &fakeworld.Rule{Cmd: "ldd", Args: []string{"--version"}, Stdout: "ldd (Ubuntu GLIBC 2.39-0ubuntu8) 2.39\nCopyright (C) 2024\n"}
	lddMusl := &fakeworld.Rule{Cmd: "ldd", Exit: 1, Stderr: "musl libc (x86_64)\n"}
	lddOdd := &fakeworld.Rule{Cmd: "ldd", Stdout: "ldd (GNU libc) garbage\n"}
	tests := []struct {
		name        string
		procVersion string
		osRelease   string
		env         map[string]string
		world       fakeworld.World
		wantWSL     bool
		wantDisplay bool
		wantDistro  domain.Distro
		wantGlibc   string
	}{
		{"ubuntu desktop", linVer, ubuntu, map[string]string{"DISPLAY": ":0"}, glibcWorld(getconf239, nil), false, true,
			domain.Distro{ID: "ubuntu", IDLike: []string{"debian"}, PkgManager: "apt"}, "2.39"},
		{"debian headless glibc 2.35", linVer, debian, nil, glibcWorld(getconf235, nil), false, false,
			domain.Distro{ID: "debian", PkgManager: "apt"}, "2.35"},
		{"fedora wayland, ldd fallback", linVer, fedora, map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, glibcWorld(getconfFail, lddGlibc), false, true,
			domain.Distro{ID: "fedora", PkgManager: "dnf"}, "2.39"},
		{"rhel via ID_LIKE", linVer, rhel, nil, glibcWorld(getconf239, nil), false, false,
			domain.Distro{ID: "rhel", IDLike: []string{"fedora"}, PkgManager: "dnf"}, "2.39"},
		{"alpine musl is unknown", linVer, alpine, nil, glibcWorld(getconfFail, lddMusl), false, false,
			domain.Distro{ID: "alpine"}, ""},
		{"wsl from /proc/version", wslVer, ubuntu, nil, glibcWorld(getconf239, nil), true, false,
			domain.Distro{ID: "ubuntu", IDLike: []string{"debian"}, PkgManager: "apt"}, "2.39"},
		{"wsl from env", linVer, ubuntu, map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}, glibcWorld(getconf239, nil), true, false,
			domain.Distro{ID: "ubuntu", IDLike: []string{"debian"}, PkgManager: "apt"}, "2.39"},
		{"no glibc commands, no files", "", "", nil, fakeworld.World{}, false, false, domain.Distro{}, ""},
		{"ldd without a version", linVer, debian, nil, glibcWorld(getconfFail, lddOdd), false, false,
			domain.Distro{ID: "debian", PkgManager: "apt"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := testkit.NewFake(t, tt.world, "getconf", "ldd")
			pv, osr := fixtures(t, tt.procVersion, tt.osRelease)
			runner := execrunner.New(execrunner.Options{BaseEnv: []string{"PATH=" + f.Dir}})
			p := New(runner, Options{
				GOOS: "linux", GOARCH: "arm64", ProcVersionPath: pv, OSReleasePath: osr,
				Getenv: func(k string) string { return tt.env[k] },
				UID:    func() int { return 501 },
			})
			got, err := p.Probe(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got.OS != "linux" || got.Arch != "arm64" || got.UID != 501 {
				t.Errorf("basic facts = %+v", got)
			}
			if got.WSL != tt.wantWSL || got.Display != tt.wantDisplay {
				t.Errorf("wsl=%v display=%v, want %v %v", got.WSL, got.Display, tt.wantWSL, tt.wantDisplay)
			}
			if got.Distro.ID != tt.wantDistro.ID || got.Distro.PkgManager != tt.wantDistro.PkgManager || !slices.Equal(got.Distro.IDLike, tt.wantDistro.IDLike) {
				t.Errorf("distro = %+v, want %+v", got.Distro, tt.wantDistro)
			}
			gotGlibc := ""
			if got.Glibc != nil {
				gotGlibc = got.Glibc.String()
			}
			if gotGlibc != tt.wantGlibc {
				t.Errorf("glibc = %q, want %q", gotGlibc, tt.wantGlibc)
			}
			testkit.AssertSafeInvocations(t, f.Log(t))
		})
	}
}

func TestProbeDarwin(t *testing.T) {
	p := New(nil, Options{GOOS: "darwin", GOARCH: "arm64", UID: func() int { return 501 }})
	got, err := p.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Display || got.Glibc != nil || got.Key() != domain.Darwin || got.WSL {
		t.Errorf("darwin = %+v", got)
	}
}

func TestProbeUnreadableFile(t *testing.T) {
	for _, which := range []string{"proc", "osrelease"} {
		t.Run(which, func(t *testing.T) {
			boom := errors.New("permission denied")
			p := New(nil, Options{GOOS: "linux", ProcVersionPath: "/p", OSReleasePath: "/o",
				ReadFile: func(path string) ([]byte, error) {
					if (which == "proc") == (path == "/p") {
						return nil, boom
					}
					return nil, os.ErrNotExist
				}})
			if _, err := p.Probe(context.Background()); !errors.Is(err, boom) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestDefaults(t *testing.T) {
	p := New(nil, Options{})
	o := p.opts
	if o.GOOS == "" || o.ProcVersionPath != "/proc/version" || o.OSReleasePath != "/etc/os-release" || o.GetconfCmd != "getconf" || o.LddCmd != "ldd" || o.Getenv == nil || o.UID == nil || o.ReadFile == nil {
		t.Errorf("defaults = %+v", o)
	}
}

func TestParseOSRelease(t *testing.T) {
	d := ParseOSRelease([]byte("# comment\n\nID='ubuntu'\nnoequals\nID_LIKE=\"Debian\"\n"))
	if d.ID != "ubuntu" || !slices.Equal(d.IDLike, []string{"debian"}) || d.PkgManager != "apt" {
		t.Errorf("distro = %+v", d)
	}
	if ParseOSRelease([]byte("ID=centos\n")).PkgManager != "dnf" {
		t.Error("centos should use dnf")
	}
}
