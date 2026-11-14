package conflicts

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
)

type linkBackend struct {
	id    catalog.BackendID
	paths map[string][]string
	err   error
}

func (b linkBackend) ID() catalog.BackendID { return b.id }
func (b linkBackend) LinkPaths(_ context.Context, e catalog.Entry, _ catalog.InstallStep) ([]string, error) {
	return b.paths[e.ID], b.err
}

type owned struct {
	class  plan.OwnerClass
	detail string
}

type fakeProbe struct {
	files     map[string]owned
	inspectE  error
	ownerE    error
	inspected []string
}

func (p *fakeProbe) Inspect(_ context.Context, path string) (*plan.ConflictEvidence, error) {
	p.inspected = append(p.inspected, path)
	if p.inspectE != nil {
		return nil, p.inspectE
	}
	if _, ok := p.files[path]; !ok {
		return nil, nil
	}
	return &plan.ConflictEvidence{LsLine: "lrwxr-xr-x 1 j admin 52 " + path, IsSymlink: true, Dangling: true}, nil
}

func (p *fakeProbe) Owner(_ context.Context, path string, _ catalog.Entry) (plan.OwnerClass, string, error) {
	if p.ownerE != nil {
		return "", "", p.ownerE
	}
	o := p.files[path]
	return o.class, o.detail, nil
}

var (
	cask    = catalog.InstallStep{Backend: catalog.BackendBrewCask, ID: "claude-code"}
	formula = catalog.InstallStep{Backend: catalog.BackendBrewFormula, ID: "gh"}
	orca    = catalog.InstallStep{Backend: catalog.BackendBrewCask, ID: "stablyai/orca/orca", Tap: "stablyai/orca", Trust: catalog.TrustItem}
)

func service(p *fakeProbe, paths map[string][]string) *Service {
	return &Service{Probe: p, Backends: map[catalog.BackendID]Backend{
		catalog.BackendBrewCask:    linkBackend{id: catalog.BackendBrewCask, paths: paths},
		catalog.BackendBrewFormula: linkBackend{id: catalog.BackendBrewFormula, paths: paths},
	}}
}

func TestCheck(t *testing.T) {
	paths := map[string][]string{
		"claude-code": {"/opt/homebrew/bin/claude"},
		"gh":          {"/opt/homebrew/bin/gh"},
		"orca":        {"/opt/homebrew/bin/orca", "/Applications/Orca.app"},
	}
	tests := []struct {
		name     string
		files    map[string]owned
		entry    string
		step     catalog.InstallStep
		owner    plan.OwnerClass
		failing  []string
		hintPart string
	}{
		{"nothing there", nil, "claude-code", cask, "", nil, ""},
		{"this package", map[string]owned{"/opt/homebrew/bin/claude": {plan.OwnerThisPackage, "brew cask claude-code"}}, "claude-code", cask, "", nil, ""},
		{"orphan", map[string]owned{"/opt/homebrew/bin/claude": {plan.OwnerOrphan, "dangling symlink"}}, "claude-code", cask, plan.OwnerOrphan,
			[]string{"brew", "install", "--cask", "claude-code"}, "nothing owns /opt/homebrew/bin/claude"},
		{"npm", map[string]owned{"/opt/homebrew/bin/claude": {plan.OwnerNPM, "npm @anthropic-ai/claude-code"}}, "claude-code", cask, plan.OwnerNPM,
			[]string{"brew", "install", "--cask", "claude-code"}, "npm uninstall -g @anthropic-ai/claude-code"},
		{"other brew", map[string]owned{"/opt/homebrew/bin/gh": {plan.OwnerOtherBrew, "brew formula gh-legacy"}}, "gh", formula, plan.OwnerOtherBrew,
			[]string{"brew", "install", "gh"}, "belongs to brew formula gh-legacy"},
		{"other installer", map[string]owned{"/opt/homebrew/bin/claude": {plan.OwnerOtherInstaller, "native install"}}, "claude-code", cask, plan.OwnerOtherInstaller,
			[]string{"brew", "install", "--cask", "claude-code"}, "its own uninstaller"},
		{"orca before its tap: detect.commands path", map[string]owned{"/opt/homebrew/bin/orca": {plan.OwnerOrphan, ""}}, "orca", orca, plan.OwnerOrphan,
			[]string{"brew", "install", "--cask", "stablyai/orca/orca"}, "nothing owns"},
		{"second path conflicts", map[string]owned{"/Applications/Orca.app": {plan.OwnerOtherInstaller, "app bundle"}}, "orca", orca, plan.OwnerOtherInstaller,
			[]string{"brew", "install", "--cask", "stablyai/orca/orca"}, "app bundle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := service(&fakeProbe{files: tt.files}, paths)
			c, err := s.Check(context.Background(), catalog.Entry{ID: tt.entry}, tt.step)
			if err != nil {
				t.Fatal(err)
			}
			if tt.owner == "" {
				if c != nil {
					t.Errorf("unexpected conflict %+v", c)
				}
				return
			}
			if c == nil || c.Owner != tt.owner || c.PackageID != tt.entry || !slices.Equal(c.FailingCommand, tt.failing) ||
				!strings.Contains(c.FixHint, tt.hintPart) || c.Evidence.LsLine == "" || !c.Evidence.Dangling {
				t.Errorf("conflict = %+v", c)
			}
		})
	}
}

func TestCheckAllAndErrors(t *testing.T) {
	ctx := context.Background()
	s := service(&fakeProbe{files: map[string]owned{"/b/claude": {plan.OwnerOrphan, ""}}}, map[string][]string{"claude-code": {"/b/claude"}})
	s.Backends["broken"] = linkBackend{id: "broken", err: errors.New("brew info failed")}
	res := s.CheckAll(ctx, []Item{
		{Entry: catalog.Entry{ID: "claude-code"}, Step: cask},
		{Entry: catalog.Entry{ID: "gh"}, Step: formula},
		{Entry: catalog.Entry{ID: "x"}, Step: catalog.InstallStep{Backend: "broken", ID: "x"}},
		{Entry: catalog.Entry{ID: "pc"}, Step: catalog.InstallStep{Backend: catalog.BackendNPM, ID: "paperclipai"}},
	})
	if len(res.Conflicts) != 1 || res.Conflicts["claude-code"] == nil || len(res.Failed) != 1 || !strings.Contains(res.Failed["x"], "brew info failed") {
		t.Errorf("result = %+v", res)
	}
	for _, p := range []*fakeProbe{{inspectE: errors.New("eperm")}, {files: map[string]owned{"/b/claude": {}}, ownerE: errors.New("lstat")}} {
		s := service(p, map[string][]string{"claude-code": {"/b/claude"}})
		if _, err := s.Check(ctx, catalog.Entry{ID: "claude-code"}, cask); err == nil {
			t.Error("probe error must be returned")
		}
	}
}
