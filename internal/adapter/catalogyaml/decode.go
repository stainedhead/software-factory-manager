package catalogyaml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

// decoder walks YAML nodes strictly, collecting every problem with its
// file:line:col position.
type decoder struct {
	file string
	errs []string
}

func (d *decoder) errf(n *yaml.Node, path, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if path != "" {
		msg = path + ": " + msg
	}
	d.errs = append(d.errs, fmt.Sprintf("%s:%d:%d %s", d.file, n.Line, n.Column, msg))
}

func (d *decoder) err() error {
	if len(d.errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(d.errs, "\n"))
}

// document parses data and returns its root node (a mapping), or nil.
func (d *decoder) document(data []byte) *yaml.Node {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			d.errs = append(d.errs, d.file+":1:1 empty file")
			return nil
		}
		d.errs = append(d.errs, fmt.Sprintf("%s: %s", d.file, strings.TrimPrefix(err.Error(), "yaml: ")))
		return nil
	}
	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		d.errf(root, "", "expected a mapping at the top level")
		return nil
	}
	return root
}

type field struct {
	key   string
	keyN  *yaml.Node
	value *yaml.Node
}

// fields returns the mapping's key/value pairs in order.
func (d *decoder) fields(n *yaml.Node, path string) []field {
	n = resolve(n)
	if n.Kind != yaml.MappingNode {
		d.errf(n, path, "expected a mapping, got %s", kindName(n))
		return nil
	}
	out := make([]field, 0, len(n.Content)/2)
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if seen[k.Value] {
			d.errf(k, join(path, k.Value), "duplicate key")
			continue
		}
		seen[k.Value] = true
		out = append(out, field{key: k.Value, keyN: k, value: v})
	}
	return out
}

func (d *decoder) unknown(f field, path string) {
	d.errf(f.keyN, join(path, f.key), "unknown field")
}

func (d *decoder) str(n *yaml.Node, path string) string {
	n = resolve(n)
	if n.Kind != yaml.ScalarNode || n.Tag == "!!null" {
		d.errf(n, path, "expected a string, got %s", kindName(n))
		return ""
	}
	return n.Value
}

func (d *decoder) boolean(n *yaml.Node, path string) bool {
	n = resolve(n)
	if n.Kind != yaml.ScalarNode || n.Tag != "!!bool" {
		d.errf(n, path, "expected true or false, got %s", kindName(n))
		return false
	}
	return n.Value == "true"
}

func (d *decoder) strList(n *yaml.Node, path string) []string {
	n = resolve(n)
	if n.Kind != yaml.SequenceNode {
		d.errf(n, path, "expected a list, got %s", kindName(n))
		return nil
	}
	out := make([]string, 0, len(n.Content))
	for i, item := range n.Content {
		out = append(out, d.str(item, fmt.Sprintf("%s[%d]", path, i)))
	}
	return out
}

// argv accepts a list or a string split on whitespace (never a shell).
func (d *decoder) argv(n *yaml.Node, path string) []string {
	if r := resolve(n); r.Kind == yaml.ScalarNode {
		return strings.Fields(d.str(r, path))
	}
	return d.strList(n, path)
}

func (d *decoder) keys(n *yaml.Node, path string) []platform.Key {
	var out []platform.Key
	for _, s := range d.strList(n, path) {
		out = append(out, platform.Key(s))
	}
	return out
}

func (d *decoder) opaque(n *yaml.Node, path string) map[string]any {
	var v map[string]any
	if err := resolve(n).Decode(&v); err != nil {
		d.errf(n, path, "expected a mapping: %s", strings.TrimPrefix(err.Error(), "yaml: "))
	}
	return v
}

func resolve(n *yaml.Node) *yaml.Node {
	for n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	return n
}

func kindName(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		return "a mapping"
	case yaml.SequenceNode:
		return "a list"
	case yaml.ScalarNode:
		if n.Tag == "!!null" {
			return "null"
		}
		return fmt.Sprintf("%q", n.Value)
	}
	return "an unsupported node"
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// decodeCatalog decodes catalog.yaml into apiVersion and ordered entries.
func decodeCatalog(file string, data []byte) (string, []catalog.Entry, error) {
	d := &decoder{file: file}
	root := d.document(data)
	if root == nil {
		return "", nil, d.err()
	}
	var api string
	var entries []catalog.Entry
	sawPackages := false
	for _, f := range d.fields(root, "") {
		switch f.key {
		case "apiVersion":
			api = d.str(f.value, f.key)
		case "packages":
			sawPackages = true
			for _, p := range d.fields(f.value, "packages") {
				entries = append(entries, d.entry(p.key, p.value, "packages."+p.key))
			}
		default:
			d.unknown(f, "")
		}
	}
	if !sawPackages {
		d.errf(root, "", "missing packages")
	}
	return api, entries, d.err()
}

func (d *decoder) entry(id string, n *yaml.Node, path string) catalog.Entry {
	e := catalog.Entry{ID: id}
	for _, f := range d.fields(n, path) {
		fp := join(path, f.key)
		switch f.key {
		case "display":
			e.Display = d.str(f.value, fp)
		case "category":
			e.Category = d.str(f.value, fp)
		case "kind":
			e.Kind = catalog.Kind(d.str(f.value, fp))
		case "platforms":
			e.Platforms = d.keys(f.value, fp)
		case "detect":
			e.Detect = d.detect(f.value, fp)
		case "install":
			e.Install = d.install(f.value, fp)
		case "version_policy":
			e.VersionPolicy = catalog.VersionPolicy(d.str(f.value, fp))
		case "requires":
			e.Requires = d.requires(f.value, fp)
		case "satisfies_if":
			e.SatisfiesIf = &catalog.Satisfaction{}
			for _, g := range d.fields(f.value, fp) {
				if g.key != "min_version" {
					d.unknown(g, fp)
					continue
				}
				e.SatisfiesIf.MinVersion = d.str(g.value, join(fp, g.key))
			}
		case "reject_if":
			e.RejectIf = &catalog.Rejection{}
			for _, g := range d.fields(f.value, fp) {
				if g.key != "version_lt" {
					d.unknown(g, fp)
					continue
				}
				e.RejectIf.VersionLT = d.str(g.value, join(fp, g.key))
			}
		case "choice":
			e.Choice = d.choice(f.value, fp)
		case "satisfied_by_any_present":
			e.SatisfiedByAnyPresent = d.boolean(f.value, fp)
		case "sign_in_required":
			e.SignInRequired = d.boolean(f.value, fp)
		case "service":
			e.Service = &catalog.ServiceSpec{Raw: d.opaque(f.value, fp)}
		default:
			d.unknown(f, path)
		}
	}
	if e.VersionPolicy == "" {
		e.VersionPolicy = catalog.PolicyTrack
	}
	return e
}

func (d *decoder) detect(n *yaml.Node, path string) catalog.DetectSpec {
	var s catalog.DetectSpec
	for _, f := range d.fields(n, path) {
		fp := join(path, f.key)
		switch f.key {
		case "commands":
			s.Commands = d.strList(f.value, fp)
		case "bundle_ids":
			s.BundleIDs = d.strList(f.value, fp)
		case "winget_id":
			s.WingetID = d.str(f.value, fp)
		case "npm_package":
			s.NPMPackage = d.str(f.value, fp)
		case "version":
			s.Version = &catalog.VersionProbe{}
			for _, g := range d.fields(f.value, fp) {
				gp := join(fp, g.key)
				switch g.key {
				case "cmd":
					s.Version.Cmd = d.argv(g.value, gp)
				case "regex":
					s.Version.Regex = d.str(g.value, gp)
				default:
					d.unknown(g, fp)
				}
			}
		case "provenance":
			s.Provenance = map[catalog.Provenance]string{}
			for _, g := range d.fields(f.value, fp) {
				s.Provenance[catalog.Provenance(g.key)] = d.str(g.value, join(fp, g.key))
			}
		default:
			d.unknown(f, path)
		}
	}
	return s
}

func (d *decoder) install(n *yaml.Node, path string) map[platform.Key][]catalog.InstallStep {
	out := map[platform.Key][]catalog.InstallStep{}
	for _, f := range d.fields(n, path) {
		fp := join(path, f.key)
		seq := resolve(f.value)
		if seq.Kind != yaml.SequenceNode {
			d.errf(seq, fp, "expected a list of install steps, got %s", kindName(seq))
			continue
		}
		steps := []catalog.InstallStep{}
		for i, item := range seq.Content {
			steps = append(steps, d.step(item, fmt.Sprintf("%s[%d]", fp, i)))
		}
		out[platform.Key(f.key)] = steps
	}
	return out
}

func (d *decoder) step(n *yaml.Node, path string) catalog.InstallStep {
	var s catalog.InstallStep
	for _, f := range d.fields(n, path) {
		fp := join(path, f.key)
		switch f.key {
		case "backend":
			s.Backend = catalog.BackendID(d.str(f.value, fp))
		case "id":
			s.ID = d.str(f.value, fp)
		case "tap":
			s.Tap = d.str(f.value, fp)
		case "trust":
			s.Trust = d.str(f.value, fp)
		case "version":
			s.Version = d.str(f.value, fp)
		case "hint":
			s.Hint = d.str(f.value, fp)
		case "post_install":
			s.PostInstall = d.strList(f.value, fp)
		case "link":
			d.errf(f.keyN, fp, "not supported: facman never links with --force; it puts the keg on PATH instead (D-3)")
		default:
			d.unknown(f, path)
		}
	}
	return s
}

func (d *decoder) requires(n *yaml.Node, path string) []catalog.Requirement {
	seq := resolve(n)
	if seq.Kind != yaml.SequenceNode {
		d.errf(seq, path, "expected a list, got %s", kindName(seq))
		return nil
	}
	var out []catalog.Requirement
	for i, item := range seq.Content {
		ip := fmt.Sprintf("%s[%d]", path, i)
		if r := resolve(item); r.Kind == yaml.ScalarNode {
			out = append(out, catalog.Requirement{Package: d.str(r, ip)})
			continue
		}
		var req catalog.Requirement
		for _, f := range d.fields(item, ip) {
			switch f.key {
			case "package":
				req.Package = d.str(f.value, join(ip, f.key))
			case "min_version":
				req.MinVersion = d.str(f.value, join(ip, f.key))
			default:
				d.unknown(f, ip)
			}
		}
		out = append(out, req)
	}
	return out
}

func (d *decoder) choice(n *yaml.Node, path string) []catalog.ChoiceMember {
	seq := resolve(n)
	if seq.Kind != yaml.SequenceNode {
		d.errf(seq, path, "expected a list, got %s", kindName(seq))
		return nil
	}
	var out []catalog.ChoiceMember
	for i, item := range seq.Content {
		ip := fmt.Sprintf("%s[%d]", path, i)
		var m catalog.ChoiceMember
		for _, f := range d.fields(item, ip) {
			fp := join(ip, f.key)
			switch f.key {
			case "id":
				m.ID = d.str(f.value, fp)
			case "platforms":
				m.Platforms = d.keys(f.value, fp)
			case "host_on":
				m.HostOn = d.keys(f.value, fp)
			default:
				d.unknown(f, ip)
			}
		}
		out = append(out, m)
	}
	return out
}

// decodeProfile decodes one profiles/<name>.yaml file.
func decodeProfile(file string, data []byte) (string, catalog.Profile, error) {
	d := &decoder{file: file}
	root := d.document(data)
	if root == nil {
		return "", catalog.Profile{}, d.err()
	}
	var api string
	var p catalog.Profile
	for _, f := range d.fields(root, "") {
		switch f.key {
		case "apiVersion":
			api = d.str(f.value, f.key)
		case "profile":
			p.Name = d.str(f.value, f.key)
		case "description":
			p.Description = d.str(f.value, f.key)
		case "include":
			p.Include = d.strList(f.value, f.key)
		case "packages":
			p.Packages = d.profilePackages(f.value, f.key)
		case "policy":
			p.Policy = d.opaque(f.value, f.key)
		default:
			d.unknown(f, "")
		}
	}
	if p.Name == "" {
		d.errf(root, "", "missing profile name")
	}
	return api, p, d.err()
}

// profilePackages accepts "id" or {id: {optional: bool, version: x}}.
func (d *decoder) profilePackages(n *yaml.Node, path string) []catalog.ProfilePackage {
	seq := resolve(n)
	if seq.Kind != yaml.SequenceNode {
		d.errf(seq, path, "expected a list, got %s", kindName(seq))
		return nil
	}
	var out []catalog.ProfilePackage
	for i, item := range seq.Content {
		ip := fmt.Sprintf("%s[%d]", path, i)
		if r := resolve(item); r.Kind == yaml.ScalarNode {
			out = append(out, catalog.ProfilePackage{ID: d.str(r, ip)})
			continue
		}
		fs := d.fields(item, ip)
		if len(fs) != 1 {
			if fs != nil {
				d.errf(item, ip, "expected one package id with its options")
			}
			continue
		}
		pkg := catalog.ProfilePackage{ID: fs[0].key}
		op := join(ip, fs[0].key)
		for _, g := range d.fields(fs[0].value, op) {
			switch g.key {
			case "optional":
				pkg.Optional = d.boolean(g.value, join(op, g.key))
			case "version":
				pkg.Version = d.str(g.value, join(op, g.key))
			default:
				d.unknown(g, op)
			}
		}
		out = append(out, pkg)
	}
	return out
}
