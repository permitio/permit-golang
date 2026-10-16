package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// runner runs gremlins for the units of a plan.
type runner struct {
	opts   gateOptions
	tools  tools
	dir    string
	stdout io.Writer
	runs   int
	// copies are the copies of the modules that checkCopy made, by module root.
	copies map[string]string
	// modFiles are the module files that moduleFile wrote, by module root.
	modFiles map[string]string
	// modDirs counts the directories moduleFile made, including those of module
	// files it failed to write, so each attempt gets a directory of its own.
	modDirs int
}

// unitResult is what gremlins' runs found in one unit.
type unitResult struct {
	// ran reports whether the unit was measured: false when its module stopped at
	// another unit's failure first.
	ran bool
	// mutations have file names relative to the repository root.
	mutations  []mutation
	mismatched []mutation
	events     []event
	runs       int
	elapsed    time.Duration
	failure    error
	// candidates are the lines whose mutants would run, found by the dry run.
	candidates []candidate
	// sampledOut counts the mutants on candidates the sample left out.
	sampledOut int
}

// runResults are the results of every unit, and the sample when there was one.
type runResults struct {
	units   []unitResult
	sampled bool
	chosen  map[fileLine]bool
	head    string
}

// runUnits runs gremlins on every unit. With -max-mutants it first finds the
// mutants of each unit without testing them, chooses a sample, and then tests only
// the sampled lines. A unit that does not run stops the rest of its module.
func (r *runner) runUnits(p plan, head string) runResults {
	res := runResults{units: make([]unitResult, len(p.units)), head: head}
	scopes := make([]map[string][]lineRange, len(p.units))
	for i, u := range p.units {
		scopes[i] = u.lines
	}
	failed := map[int]bool{}
	final := make([]bool, len(p.units))
	if r.opts.maxMutants > 0 {
		var candidates []candidate
		for i, u := range p.units {
			if failed[u.module] {
				continue
			}
			res.units[i] = r.run(p, u, u.lines, true)
			if res.units[i].failure != nil {
				failed[u.module] = true
				continue
			}
			res.units[i].candidates = dryRunCandidates(res.units[i].mutations)
			candidates = append(candidates, res.units[i].candidates...)
		}
		res.sampled = true
		res.chosen = chooseSample(candidates, head, r.opts.maxMutants)
		for i, u := range p.units {
			if failed[u.module] {
				continue
			}
			scopes[i], final[i] = r.applySample(u, &res.units[i], res.chosen)
		}
	}
	for i, u := range p.units {
		if failed[u.module] || final[i] {
			continue
		}
		dry := res.units[i]
		res.units[i] = r.run(p, u, scopes[i], false)
		res.units[i].candidates, res.units[i].sampledOut = dry.candidates, dry.sampledOut
		res.units[i].runs += dry.runs
		res.units[i].elapsed += dry.elapsed
		res.units[i].events = append(dry.events, res.units[i].events...)
		res.units[i].mismatched = append(dry.mismatched, res.units[i].mismatched...)
		if res.units[i].failure != nil {
			failed[u.module] = true
		}
	}
	return res
}

// dryRunCandidates returns the lines on which gremlins' dry run found mutants that
// would run.
func dryRunCandidates(mutations []mutation) []candidate {
	var out []candidate
	index := map[fileLine]int{}
	for _, m := range mutations {
		if m.status != statusRunnable {
			continue
		}
		key := fileLine{m.file, m.line}
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, candidate{file: m.file, line: m.line})
		}
		out[i].mutants++
	}
	return out
}

// applySample returns the lines of a unit to test after the sample, and whether
// its dry run is already its result: when the sample holds none of its lines,
// there is nothing for gremlins to test, so the mutants on the lines it left out
// count as skipped.
func (r *runner) applySample(u unit, dry *unitResult, chosen map[fileLine]bool) (
	map[string][]lineRange, bool,
) {
	drop := map[string][]int{}
	kept := 0
	for _, c := range dry.candidates {
		if chosen[fileLine{c.file, c.line}] {
			kept++
			continue
		}
		name := path.Base(c.file)
		drop[name] = append(drop[name], c.line)
		dry.sampledOut += c.mutants
	}
	lines := withoutLines(u.lines, drop)
	if kept > 0 {
		return lines, false
	}
	for i, m := range dry.mutations {
		if m.status == statusRunnable {
			dry.mutations[i].status = statusSkipped
		}
	}
	return lines, true
}

// run runs gremlins once on a unit with lines in scope, and checks that gremlins
// skipped exactly the mutants outside them.
func (r *runner) run(p plan, u unit, lines map[string][]lineRange, dryRun bool) unitResult {
	r.runs++
	res := unitResult{ran: true, runs: 1}
	dir := filepath.Join(r.dir, strconv.Itoa(r.runs))
	if err := os.Mkdir(dir, 0o700); err != nil {
		res.failure = err
		return res
	}
	mod := p.modules[u.module].module
	modFile, err := r.moduleFile(mod)
	if err != nil {
		res.failure = err
		return res
	}
	w, err := newWorkFiles(dir, r.tools, u.pkg.importPath, modFile, scopeDiff(lines))
	if err != nil {
		res.failure = err
		return res
	}
	env := gremlinsEnv(os.Environ(), r.tools.self, r.opts, w)
	args := buildGremlinsArgs(r.opts, dryRun, w, u.pkg.dir, u.exclude)
	moduleDir := mod.dir
	_, _ = fmt.Fprintf(r.stdout, "mutationgate: in %s: %s %s\n", moduleDir, r.opts.gremlins,
		strings.Join(args, " "))
	start := time.Now()
	code, err := r.tools.gremlins(r.opts.gremlins, moduleDir, args, env)
	res.elapsed = time.Since(start)
	events, eventsErr := readEvents(w.events)
	res.events = events
	switch {
	case err != nil:
		res.failure = fmt.Errorf("gremlins on %s: %w", u.pkg.importPath, err)
	case code != 0:
		res.failure = fmt.Errorf("gremlins exited %d on %s%s", code, u.pkg.importPath,
			stopReasons(events))
	case eventsErr != nil:
		res.failure = eventsErr
	}
	if res.failure != nil {
		return res
	}
	mutations, err := readResults(w.results)
	if err != nil {
		res.failure = fmt.Errorf("%s: %w", u.pkg.importPath, err)
		return res
	}
	detected := false
	for _, m := range mutations {
		if (m.status == statusSkipped) == isChanged(lines, m.file, m.line) {
			res.mismatched = append(res.mismatched, m.in(u.pkg.rel))
		}
		res.mutations = append(res.mutations, m.in(u.pkg.rel))
		detected = detected || m.status == statusKilled || m.status == statusTimedOut
	}
	if detected {
		res.failure = r.checkCopy(mod, u.pkg, modFile, env)
	}
	return res
}

// workFiles are the files of one gremlins run in its work directory.
type workFiles struct {
	results string
	events  string
	config  string
	diff    string
	// bin holds the go and git shims, first on gremlins' PATH.
	bin string
}

// newWorkFiles creates gremlins' empty config file, the diff the git shim gives
// gremlins, and the shims in dir. The go shim tests the package importPath, built
// with the module file modFile.
func newWorkFiles(dir string, t tools, importPath, modFile string, diff []byte) (
	workFiles, error,
) {
	w := workFiles{
		results: filepath.Join(dir, "results.json"),
		events:  filepath.Join(dir, "events.log"),
		config:  filepath.Join(dir, "gremlins.yaml"),
		diff:    filepath.Join(dir, "scope.diff"),
		bin:     filepath.Join(dir, "bin"),
	}
	if err := checkPlainPath("import path", importPath); err != nil {
		return w, err
	}
	if err := os.WriteFile(w.config, nil, 0o600); err != nil {
		return w, err
	}
	if err := os.WriteFile(w.diff, diff, 0o600); err != nil {
		return w, err
	}
	if err := os.Mkdir(w.bin, 0o700); err != nil {
		return w, err
	}
	shim := goShimOptions{realGo: t.goPath, logPath: w.events, modFile: modFile, pkg: importPath}
	if err := writeGoShim(w.bin, t.self, shim); err != nil {
		return w, err
	}
	return w, writeGitShim(w.bin, t.self, t.gitPath, w.events, w.diff)
}
