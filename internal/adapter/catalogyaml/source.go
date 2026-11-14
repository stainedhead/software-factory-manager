// Package catalogyaml loads the catalog and profiles from YAML: the catalog
// v1 embedded in the binary, or a --catalog directory with catalog.yaml and
// profiles/*.yaml. Decoding is strict (unknown keys and wrong types are
// errors with file:line:col), except under service: and policy:, which are
// kept opaque for later increments. Every load is validated by
// catalog.Validate.
package catalogyaml

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/usecase/doctor"
	"github.com/stainedhead/software-factory-manager/internal/usecase/planning"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
)

//go:embed embedded/catalog.yaml embedded/profiles/*.yaml
var embedded embed.FS

// File layout inside a catalog directory.
const (
	CatalogFile = "catalog.yaml"
	ProfilesDir = "profiles"
)

// Source loads one catalog. It implements the CatalogSource ports.
type Source struct {
	fsys     fs.FS
	root     string // directory inside fsys holding catalog.yaml
	label    string // shown in errors: "embedded catalog" or the directory
	revision string // fixed revision (embedded); "" means hash the files
	dir      string // absolute directory for dir sources
}

var (
	_ scan.CatalogSource     = (*Source)(nil)
	_ planning.CatalogSource = (*Source)(nil)
	_ doctor.CatalogSource   = (*Source)(nil)
)

// Embedded returns the catalog v1 built into this binary; its revision is
// embedded:<version>.
func Embedded(version string) *Source {
	if version == "" {
		version = "dev"
	}
	return &Source{fsys: embedded, root: "embedded", label: "embedded catalog", revision: "embedded:" + version}
}

// remotePrefixes mark catalog locations that are git URLs.
var remotePrefixes = []string{"https://", "http://", "git@", "ssh://", "git://", "file://"}

// Dir returns a source for --catalog <dir>. A git URL is a usage error
// (exit 64); a missing or invalid directory is reported by Load (exit 3).
// The revision is dir:<abs path>@<sha256 of the files>.
func Dir(dir string) (*Source, error) {
	for _, p := range remotePrefixes {
		if strings.HasPrefix(dir, p) {
			return nil, failure.Usage("--catalog %s: git catalogs arrive in a later increment", dir).
				WithFix("check the catalog out locally and pass its directory to --catalog")
		}
	}
	if strings.TrimSpace(dir) == "" {
		return nil, failure.Usage("--catalog needs a directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, failure.Validation("--catalog %s: %w", dir, err)
	}
	return &Source{fsys: os.DirFS(abs), root: ".", label: abs, dir: abs}, nil
}

// Load reads, decodes and validates the catalog and its profiles.
func (s *Source) Load(ctx context.Context) (catalog.Catalog, error) {
	if err := ctx.Err(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("loading catalog: %w", err)
	}
	files, err := s.files()
	if err != nil {
		return catalog.Catalog{}, err
	}
	var problems []string
	api, entries, err := decodeCatalog(CatalogFile, files[CatalogFile])
	if err != nil {
		problems = append(problems, strings.Split(err.Error(), "\n")...)
	}
	c := catalog.Catalog{APIVersion: api, Entries: entries, Profiles: map[string]catalog.Profile{}, Revision: s.revisionOf(files)}
	for _, name := range sortedNames(files) {
		if name == CatalogFile {
			continue
		}
		papi, p, err := decodeProfile(name, files[name])
		if err != nil {
			problems = append(problems, strings.Split(err.Error(), "\n")...)
			continue
		}
		want := strings.TrimSuffix(path.Base(name), ".yaml")
		switch {
		case papi != catalog.APIVersion:
			problems = append(problems, fmt.Sprintf("%s: apiVersion is %q, want %q", name, papi, catalog.APIVersion))
		case p.Name != want:
			problems = append(problems, fmt.Sprintf("%s: profile is %q, but the file is named for %q", name, p.Name, want))
		default:
			c.Profiles[p.Name] = p
		}
	}
	if len(problems) > 0 {
		return catalog.Catalog{}, failure.Validation("%s is invalid", s.label).
			WithDetails(problems...).
			WithFix("fix the files at the positions shown and run again")
	}
	if err := c.Validate(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("%s: %w", s.label, err)
	}
	return c, nil
}

// files reads catalog.yaml and profiles/*.yaml, keyed by slash path.
func (s *Source) files() (map[string][]byte, error) {
	out := map[string][]byte{}
	b, err := fs.ReadFile(s.fsys, path.Join(s.root, CatalogFile))
	if err != nil {
		return nil, s.missing(CatalogFile, err)
	}
	out[CatalogFile] = b
	entries, err := fs.ReadDir(s.fsys, path.Join(s.root, ProfilesDir))
	if err != nil {
		return nil, s.missing(ProfilesDir+"/", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := ProfilesDir + "/" + e.Name()
		b, err := fs.ReadFile(s.fsys, path.Join(s.root, name))
		if err != nil {
			return nil, s.missing(name, err)
		}
		out[name] = b
	}
	return out, nil
}

func (s *Source) missing(name string, err error) error {
	where := s.label
	if s.dir != "" {
		where = filepath.Join(s.dir, filepath.FromSlash(name))
	}
	what := "cannot read"
	if errors.Is(err, fs.ErrNotExist) {
		what = "missing"
	}
	return failure.Validation("catalog %s: %s %s", s.label, what, where).Wrap(err).
		WithFix("--catalog expects a directory with catalog.yaml and profiles/*.yaml")
}

func (s *Source) revisionOf(files map[string][]byte) string {
	if s.revision != "" {
		return s.revision
	}
	h := sha256.New()
	for _, name := range sortedNames(files) {
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(files[name])
		h.Write([]byte{0})
	}
	return "dir:" + s.dir + "@" + hex.EncodeToString(h.Sum(nil))
}

func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
