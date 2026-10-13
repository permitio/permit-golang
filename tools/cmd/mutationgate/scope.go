package main

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// goModule is a module the gate mutates packages of, as go list reports it.
type goModule struct {
	path string
	// dir is the module's root directory.
	dir  string
	pkgs []goPackage
}

// goPackage is one package of a module.
type goPackage struct {
	dir        string
	importPath string
	// files are the names of the non-test Go files go builds.
	files []string
	// rel is dir relative to the repository root, which the gate sets.
	rel string
}

// moduleScope is a module, the directories in it the gate was given, and what
// changed in it.
type moduleScope struct {
	module goModule
	// rel is the module's root, and dirs the directories given, relative to the
	// repository root.
	rel  string
	dirs []string
	// changedFiles counts the changed Go files of the module, or with -all the Go
	// files of its packages in the directories, mutated or not.
	changedFiles int
	notMutated   []notMutated
}

// notMutated is a changed Go file the gate does not mutate, and why.
type notMutated struct{ file, reason string }

// unit is one package to mutate. gremlins runs once per unit, in the package's
// directory with its subdirectories left out, and the go shim tests the package,
// so a mutant is always tested with its own package's tests.
type unit struct {
	module int
	pkg    goPackage
	// lines are the lines to mutate, by file name in the package's directory.
	lines map[string][]lineRange
	// exclude are gremlins --exclude-files patterns: every file in a subdirectory,
	// and the Go files in the directory that go does not build here.
	exclude []string
}

// plan is what a run mutates.
type plan struct {
	root string
	// realRoot is root with symbolic links resolved.
	realRoot string
	modules  []moduleScope
	units    []unit
}

// planRun loads the modules of the directories given and works out the units: the
// packages with changed lines, or with -all every package in the directories.
func planRun(t tools, opts gateOptions, root string) (plan, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return plan{}, err
	}
	p := plan{root: root, realRoot: realRoot}
	if err = p.loadModules(t, opts); err != nil {
		return p, err
	}
	scoped := map[string][]lineRange{}
	if opts.all {
		for i := range p.modules {
			if p.modules[i].changedFiles, err = p.allLines(i, scoped); err != nil {
				return p, err
			}
		}
	} else {
		diff, err := t.diff(root, opts.base)
		if err != nil {
			return p, err
		}
		changed, err := parseAddedLines(diff)
		if err != nil {
			return p, err
		}
		if err := p.classify(changed, scoped); err != nil {
			return p, err
		}
	}
	return p, p.makeUnits(scoped)
}

// loadModules finds the module of each directory given and lists its packages.
func (p *plan) loadModules(t tools, opts gateOptions) error {
	for _, given := range opts.dirs {
		abs, err := filepath.Abs(given)
		if err != nil {
			return err
		}
		if info, statErr := os.Stat(abs); statErr != nil || !info.IsDir() {
			return fmt.Errorf("%s is not a directory", given)
		}
		rel, err := p.relative(abs)
		if err != nil {
			return fmt.Errorf("directory %s: %w", given, err)
		}
		modRel, ok := p.moduleOfDir(rel)
		if !ok {
			return fmt.Errorf("directory %s is in no Go module of the repository", given)
		}
		i := slices.IndexFunc(p.modules, func(m moduleScope) bool { return m.rel == modRel })
		if i < 0 {
			m, err := p.loadModule(t, modRel, opts.tags)
			if err != nil {
				return err
			}
			p.modules = append(p.modules, moduleScope{module: m, rel: modRel})
			i = len(p.modules) - 1
		}
		if !slices.Contains(p.modules[i].dirs, rel) {
			p.modules[i].dirs = append(p.modules[i].dirs, rel)
		}
	}
	return nil
}

// loadModule lists the module whose root is modRel, relative to the repository
// root, and its packages.
func (p *plan) loadModule(t tools, modRel, tags string) (goModule, error) {
	modDir := filepath.Join(p.root, filepath.FromSlash(modRel))
	m, err := t.module(modDir, tags)
	if err != nil {
		return m, err
	}
	if !sameDir(m.dir, modDir) {
		return m, fmt.Errorf("go list in %s reports the module root %s", modDir, m.dir)
	}
	for i := range m.pkgs {
		if m.pkgs[i].rel, err = p.relative(m.pkgs[i].dir); err != nil {
			return m, fmt.Errorf("package %s: %w", m.pkgs[i].importPath, err)
		}
	}
	return m, nil
}

// relative returns abs relative to the repository root, with slashes.
func (p *plan) relative(abs string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	rel, err := filepath.Rel(p.realRoot, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%s is outside the repository %s", abs, p.root)
	}
	return filepath.ToSlash(rel), nil
}

// moduleOfDir returns the root, relative to the repository root, of the module a
// directory is in: the nearest directory at or above it with a go.mod, leaving out
// test data.
func (p *plan) moduleOfDir(dir string) (string, bool) {
	for ; ; dir = path.Dir(dir) {
		if !inTestdata(dir) {
			goMod := filepath.Join(p.root, filepath.FromSlash(dir), "go.mod")
			if info, err := os.Stat(goMod); err == nil && info.Mode().IsRegular() {
				return dir, true
			}
		}
		if dir == "." {
			return "", false
		}
	}
}

func inTestdata(file string) bool {
	return slices.Contains(strings.Split(file, "/"), "testdata")
}

// under reports whether rel, relative to the repository root, is dir or below it.
func under(rel, dir string) bool {
	return dir == "." || rel == dir || strings.HasPrefix(rel, dir+"/")
}

// inDirs reports whether rel, relative to the repository root, is in one of the
// module's directories.
func (m moduleScope) inDirs(rel string) bool {
	return slices.ContainsFunc(m.dirs, func(dir string) bool { return under(rel, dir) })
}

// isBuilt reports whether go builds a file, relative to the repository root, in
// one of the module's packages.
func (m moduleScope) isBuilt(file string) bool {
	dir, name := path.Dir(file), path.Base(file)
	return slices.ContainsFunc(m.module.pkgs, func(pkg goPackage) bool {
		return pkg.rel == dir && slices.Contains(pkg.files, name)
	})
}

// classify sorts the changed Go files: lines to mutate go in scoped, the files not
// mutated in their module's notMutated, and a file in a module that no directory
// given is in is an error.
func (p *plan) classify(changed, scoped map[string][]lineRange) error {
	var outside []string
	for _, file := range slices.Sorted(maps.Keys(changed)) {
		modRel, ok := p.moduleOfDir(path.Dir(file))
		i := slices.IndexFunc(p.modules, func(m moduleScope) bool { return m.rel == modRel })
		if !ok || i < 0 {
			outside = append(outside, file)
			continue
		}
		m := &p.modules[i]
		m.changedFiles++
		switch {
		case inTestdata(file):
			m.notMutated = append(m.notMutated, notMutated{file, "test data"})
		case !m.inDirs(path.Dir(file)):
			m.notMutated = append(m.notMutated, notMutated{file, "outside the directories given"})
		case !m.isBuilt(file):
			m.notMutated = append(m.notMutated, notMutated{file, "go does not build it here " +
				"(another platform, a build tag or an ignored name)"})
		default:
			scoped[file] = changed[file]
		}
	}
	if len(outside) > 0 {
		return fmt.Errorf("go lines changed in %s, in no module of the directories given; give "+
			"the directory of its module too", strings.Join(outside, ", "))
	}
	return nil
}

// allLines puts every line of every Go file of the packages in module i's
// directories in scoped, and returns how many files there are.
func (p *plan) allLines(i int, scoped map[string][]lineRange) (int, error) {
	m := p.modules[i]
	files := 0
	for _, pkg := range m.module.pkgs {
		if !m.inDirs(pkg.rel) {
			continue
		}
		for _, name := range pkg.files {
			data, err := os.ReadFile(filepath.Join(pkg.dir, name))
			if err != nil {
				return 0, err
			}
			files++
			if n := lineCount(data); n > 0 {
				scoped[path.Join(pkg.rel, name)] = []lineRange{{1, n}}
			}
		}
	}
	return files, nil
}

func lineCount(data []byte) int {
	n := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// makeUnits makes a unit of each package in the directories given that has lines
// in scoped, in the order of the modules and then of the import paths.
func (p *plan) makeUnits(scoped map[string][]lineRange) error {
	for i, m := range p.modules {
		pkgs := slices.SortedFunc(slices.Values(m.module.pkgs), func(a, b goPackage) int {
			return strings.Compare(a.importPath, b.importPath)
		})
		for _, pkg := range pkgs {
			if !m.inDirs(pkg.rel) {
				continue
			}
			u := unit{module: i, pkg: pkg, lines: map[string][]lineRange{}}
			for _, name := range pkg.files {
				if ranges, ok := scoped[path.Join(pkg.rel, name)]; ok {
					u.lines[name] = ranges
				}
			}
			if len(u.lines) == 0 {
				continue
			}
			var err error
			if u.exclude, err = excludedFiles(pkg); err != nil {
				return err
			}
			p.units = append(p.units, u)
		}
	}
	return nil
}

// excludedFiles returns the gremlins --exclude-files patterns of a package's
// directory: every file in a subdirectory, and each Go file go does not build here,
// whose mutants would otherwise live.
func excludedFiles(pkg goPackage) ([]string, error) {
	entries, err := os.ReadDir(pkg.dir)
	if err != nil {
		return nil, err
	}
	patterns := []string{"/"}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			slices.Contains(pkg.files, name) {
			continue
		}
		patterns = append(patterns, "^"+regexp.QuoteMeta(name)+"$")
	}
	return patterns, nil
}

// scopeRef is the ref the gate passes to gremlins' --diff. gremlins runs
// git diff --merge-base scopeRef, which the git shim answers with scopeDiff.
const scopeRef = "mutationgate-scope"

// scopeDiff returns the git diff that the git shim gives gremlins: one that adds
// exactly lines, by file name, with no context, so gremlins marks those lines as
// changed and no others.
func scopeDiff(lines map[string][]lineRange) []byte {
	var b bytes.Buffer
	for _, name := range slices.Sorted(maps.Keys(lines)) {
		ranges := slices.SortedFunc(slices.Values(lines[name]), func(x, y lineRange) int {
			return x.first - y.first
		})
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n", name, name, name, name)
		added := 0
		for _, r := range ranges {
			n := r.last - r.first + 1
			fmt.Fprintf(&b, "@@ -%d,0 +%d,%d @@\n%s", r.first-1-added, r.first, n,
				strings.Repeat("+\n", n))
			added += n
		}
	}
	return b.Bytes()
}

// isChanged reports whether line of file is in lines.
func isChanged(lines map[string][]lineRange, file string, line int) bool {
	for _, r := range lines[file] {
		if line >= r.first && line <= r.last {
			return true
		}
	}
	return false
}

// withoutLines returns lines less the lines in drop, both by file name.
func withoutLines(lines map[string][]lineRange, drop map[string][]int) map[string][]lineRange {
	kept := map[string][]lineRange{}
	for name, ranges := range lines {
		for _, r := range ranges {
			first := r.first
			for line := r.first; line <= r.last; line++ {
				if !slices.Contains(drop[name], line) {
					continue
				}
				if line > first {
					kept[name] = append(kept[name], lineRange{first, line - 1})
				}
				first = line + 1
			}
			if first <= r.last {
				kept[name] = append(kept[name], lineRange{first, r.last})
			}
		}
	}
	return kept
}
