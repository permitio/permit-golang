package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	// dependabotFile is Dependabot's configuration, and hooksFile the git hooks' configuration.
	dependabotFile = ".github/dependabot.yml"
	hooksFile      = ".pre-commit-config.yaml"
	// cooldownDays is the fewest days an entry of dependabotFile waits before it proposes a new
	// version, and majorCooldownDays the fewest a gomod entry waits for a new major version.
	cooldownDays      = 7
	majorCooldownDays = 14
	// tidyEntry is the entry of the hook that checks that the root module is tidy.
	tidyEntry = "go mod tidy -diff"
)

// moduleExceptions maps the directory of a Go module, relative to the repository root with
// slashes, to the reason it has no gomod entry in dependabotFile and needs no go mod tidy -diff
// hook in hooksFile.
var moduleExceptions = map[string]string{
	"tools/upgrade": "its test checks that v2 reads what a v1 release wrote, so it pins " +
		"github.com/permitio/permit-golang v1.3.0 on purpose and moves by hand to each new v1 " +
		"release",
}

// moduleSetup is what the module rules check: the Go modules, and the configurations that update
// and tidy them.
type moduleSetup struct {
	// dirs are the directories of the go.mod files that git tracks outside testdata directories,
	// relative to the repository root with slashes; the root is ".".
	dirs []string
	// dependabot and hooks are the top-level mappings of dependabotFile and hooksFile.
	dependabot, hooks *yaml.Node
	// exceptions are the moduleExceptions.
	exceptions map[string]string
}

// readModuleSetup finds the go.mod files among the committed files, and reads dependabotFile and
// hooksFile.
func readModuleSetup(root string, committed []string) (moduleSetup, error) {
	setup := moduleSetup{exceptions: moduleExceptions}
	for _, name := range committed {
		dir := path.Dir(name)
		if path.Base(name) == "go.mod" && !strings.Contains("/"+dir+"/", "/testdata/") {
			setup.dirs = append(setup.dirs, dir)
		}
	}
	if !slices.Contains(setup.dirs, ".") {
		return moduleSetup{}, errors.New("git tracks no go.mod at the repository root; add it " +
			"with git add go.mod")
	}
	var err error
	if setup.dependabot, err = readMapping(root, dependabotFile); err != nil {
		return moduleSetup{}, err
	}
	if setup.hooks, err = readMapping(root, hooksFile); err != nil {
		return moduleSetup{}, err
	}
	return setup, nil
}

// readMapping reads name, a slash path relative to root, as YAML whose top level must be a
// mapping, and returns the mapping.
func readMapping(root, name string) (*yaml.Node, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 ||
		resolve(doc.Content[0]).Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: its top level is not a mapping", name)
	}
	return resolve(doc.Content[0]), nil
}

// checkModules checks the cooldown of every entry of dependabotFile, and that every Go module
// has a gomod entry and a tidy hook unless it is an exception. It returns the number of entries.
func (c *checker) checkModules(setup moduleSetup) int {
	updates := items(field(setup.dependabot, "updates"))
	// entries maps each directory that a gomod entry for master names to the entry's line.
	entries := map[string]int{}
	for _, entry := range updates {
		ecosystem, _ := scalar(field(entry, "package-ecosystem"))
		dirs := entryDirs(entry)
		c.checkCooldown(entry, ecosystem, dirs)
		if ecosystem == "gomod" && field(entry, "target-branch") == nil {
			for _, dir := range dirs {
				entries[dir] = entry.Line
			}
		}
	}
	tidied := tidyHooks(setup.hooks)
	// modules holds the directory that the gomod entry of each module names, such as /tools.
	modules := map[string]bool{}
	for _, dir := range setup.dirs {
		modules[entryDir(dir)] = true
		if _, ok := setup.exceptions[dir]; ok {
			continue
		}
		goMod := path.Join(dir, "go.mod")
		if _, ok := entries[entryDir(dir)]; !ok {
			c.add(goMod, 0, "no gomod entry in %s without a target-branch has the directory %s, "+
				"so Dependabot never updates this module; add one, or add the module to "+
				"moduleExceptions in tools/cmd/cicheck with the reason it gets no updates",
				dependabotFile, entryDir(dir))
		}
		if !tidied[dir] {
			c.add(goMod, 0, "no hook in %s has the entry %s, so a go.mod or go.sum that is not "+
				"tidy passes; add one", hooksFile, tidyCommand(dir))
		}
	}
	for _, dir := range slices.Sorted(maps.Keys(setup.exceptions)) {
		goMod := path.Join(dir, "go.mod")
		if !modules[entryDir(dir)] {
			c.add(goMod, 0, "moduleExceptions in tools/cmd/cicheck lists %s, but git tracks no "+
				"go.mod there; remove the exception", dir)
			continue
		}
		if strings.TrimSpace(setup.exceptions[dir]) == "" {
			c.add(goMod, 0, "moduleExceptions in tools/cmd/cicheck lists this module without a "+
				"reason; give the reason it gets no Dependabot updates")
		}
		if line, ok := entries[entryDir(dir)]; ok {
			c.add(dependabotFile, line, "the gomod entry for %s updates a module that "+
				"moduleExceptions in tools/cmd/cicheck keeps out of Dependabot; remove the entry "+
				"or the exception", entryDir(dir))
		}
	}
	for _, dir := range slices.Sorted(maps.Keys(entries)) {
		if !modules[dir] {
			c.add(dependabotFile, entries[dir], "the gomod entry for %s names a directory without "+
				"a go.mod that git tracks; remove it", dir)
		}
	}
	return len(updates)
}

// entryDirs returns the directories that an entry of dependabotFile names, in directory or
// directories, cleaned as entryDir writes them, so /tools/ is /tools.
func entryDirs(entry *yaml.Node) []string {
	var dirs []string
	for _, key := range []string{"directory", "directories"} {
		for _, item := range items(field(entry, key)) {
			if dir, ok := scalar(item); ok {
				dirs = append(dirs, path.Join("/", dir))
			}
		}
	}
	return dirs
}

// entryDir returns the directory that the gomod entry of the module in dir names: / for the root,
// /tools for tools.
func entryDir(dir string) string {
	return path.Join("/", dir)
}

// checkCooldown checks that an entry of dependabotFile waits cooldownDays before it proposes a
// new version, and majorCooldownDays for a new major version: default-days and, on a gomod entry,
// semver-major-days must be set, and a semver-minor-days, semver-patch-days or semver-major-days
// that is set must not be shorter. An include or exclude list fails, since a dependency it leaves
// out of the cooldown is updated as soon as it is released.
func (c *checker) checkCooldown(entry *yaml.Node, ecosystem string, dirs []string) {
	what := fmt.Sprintf("the %s entry for %s", ecosystem, strings.Join(dirs, " and "))
	cooldown := field(entry, "cooldown")
	c.checkDays(entry.Line, what, field(cooldown, "default-days"), "default-days", cooldownDays)
	major := field(cooldown, "semver-major-days")
	if ecosystem == "gomod" || major != nil {
		c.checkDays(entry.Line, what, major, "semver-major-days", majorCooldownDays)
	}
	for _, key := range []string{"semver-minor-days", "semver-patch-days"} {
		if days := field(cooldown, key); days != nil {
			c.checkDays(entry.Line, what, days, key, cooldownDays)
		}
	}
	for _, key := range []string{"include", "exclude"} {
		if field(cooldown, key) != nil {
			c.add(dependabotFile, entry.Line, "%s has cooldown.%s, which takes dependencies out "+
				"of the cooldown or limits it to some; remove it, so every dependency waits",
				what, key)
		}
	}
}

// checkDays checks that the number of days in the cooldown key of an entry is at least minDays.
func (c *checker) checkDays(line int, what string, days *yaml.Node, key string, minDays int) {
	value, ok := scalar(days)
	if !ok {
		c.add(dependabotFile, line, "%s has no cooldown.%s; set it to at least %d, so Dependabot "+
			"proposes a version only once it is %d days old", what, key, minDays, minDays)
		return
	}
	if n, err := strconv.Atoi(value); err != nil || n < minDays {
		c.add(dependabotFile, line, "%s has cooldown.%s: %s; set it to at least %d, so "+
			"Dependabot proposes a version only once it is %d days old", what, key, value, minDays,
			minDays)
	}
}

// tidyHooks returns the module directories that a hook in hooksFile runs go mod tidy -diff in:
// "." for a hook whose entry is go mod tidy -diff, and <dir> for go -C <dir> mod tidy -diff.
func tidyHooks(hooks *yaml.Node) map[string]bool {
	tidied := map[string]bool{}
	for _, repo := range items(field(hooks, "repos")) {
		for _, hook := range items(field(repo, "hooks")) {
			entry, _ := scalar(field(hook, "entry"))
			words := strings.Fields(entry)
			dir := "."
			if len(words) > 2 && words[1] == "-C" {
				dir, words = words[2], slices.Concat(words[:1], words[3:])
			}
			if strings.Join(words, " ") == tidyEntry {
				tidied[dir] = true
			}
		}
	}
	return tidied
}

// tidyCommand returns the entry of the hook that checks that the module in dir is tidy.
func tidyCommand(dir string) string {
	if dir == "." {
		return tidyEntry
	}
	return "go -C " + dir + " mod tidy -diff"
}
