package catalogyaml

import (
	"os"
	"path/filepath"
	"testing"
)

// Fuzz targets for the catalog and profile decoders (review FR-014): a
// --catalog directory is input. Each checks no panic and that a decode
// that succeeds names every entry and profile.

func seedYAML(f *testing.F, paths ...string) {
	f.Helper()
	for _, p := range paths {
		b, err := os.ReadFile(p) //nolint:gosec // test fixture
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
}

func FuzzDecodeCatalog(f *testing.F) {
	seedYAML(f, filepath.Join("embedded", CatalogFile))
	f.Add([]byte("api_version: 1\npackages: []\n"))
	f.Add([]byte("packages:\n  - id: &a x\n    display: *a\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, entries, err := decodeCatalog(CatalogFile, data)
		if err != nil {
			return
		}
		for i, e := range entries {
			if e.ID == "" {
				t.Errorf("entry %d decoded without an id", i)
			}
		}
	})
}

func FuzzDecodeProfile(f *testing.F) {
	paths, err := filepath.Glob(filepath.Join("embedded", "profiles", "*.yaml"))
	if err != nil {
		f.Fatal(err)
	}
	seedYAML(f, paths...)
	f.Fuzz(func(t *testing.T, data []byte) {
		_, p, err := decodeProfile("profiles/fuzz.yaml", data)
		if err != nil {
			return
		}
		if p.Name == "" {
			t.Errorf("profile decoded without a name: %+v", p)
		}
	})
}
