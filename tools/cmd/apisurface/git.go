package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// system is what apisurface uses from outside: the environment of the commands it runs.
type system struct {
	// extraEnv is added to the environment of every command apisurface runs. The tests set the Go
	// module proxy, the checksum database and GOPATH there.
	extraEnv []string
}

// environ returns the environment of the commands apisurface runs.
func (s system) environ() []string {
	return append(os.Environ(), s.extraEnv...)
}

// runIn runs a command in dir with env and returns what it printed on stdout and on stderr.
func (s system) runIn(ctx context.Context, dir string, env []string, name string,
	args ...string,
) ([]byte, []byte, error) {
	//nolint:gosec // G204: apisurface runs go, git and the tools named on its command line.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// git runs git with args in the repository at root, without the user's and the system's
// configuration, and returns what it printed on stdout.
func (s system) git(ctx context.Context, root string, args ...string) ([]byte, error) {
	env := append(s.environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, stderr, err := s.runIn(ctx, root, env, "git", args...)
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err,
			strings.TrimSpace(string(stderr)))
	}
	return out, nil
}

// fileAt returns the content of the file at the slash path name in the commit rev, and false
// when the commit has no such file.
func (s system) fileAt(ctx context.Context, root, rev, name string) ([]byte, bool, error) {
	listed, err := s.git(ctx, root, "ls-tree", "--name-only", rev, "--", name)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(string(listed)) != name {
		return nil, false, nil
	}
	content, err := s.git(ctx, root, "show", rev+":"+name)
	return content, err == nil, err
}

// modulePathAt returns the module path that the root go.mod names in the commit rev, or in the
// working tree when rev is "".
func (s system) modulePathAt(ctx context.Context, root, rev string) (string, error) {
	var content []byte
	var err error
	if rev == "" {
		content, err = os.ReadFile(filepath.Join(root, "go.mod"))
	} else {
		var ok bool
		content, ok, err = s.fileAt(ctx, root, rev, "go.mod")
		if err == nil && !ok {
			return "", fmt.Errorf("%s has no go.mod at the repository root", rev)
		}
	}
	if err != nil {
		return "", err
	}
	modulePath := modfile.ModulePath(content)
	if modulePath == "" {
		return "", fmt.Errorf("the go.mod of %s names no module path", nonEmpty(rev, "the "+
			"working tree"))
	}
	return modulePath, nil
}

// nonEmpty returns s, or otherwise when s is "".
func nonEmpty(s, otherwise string) string {
	if s == "" {
		return otherwise
	}
	return s
}

// idsAt returns the entry IDs that ids.txt lists in the commit rev, none when the commit has no
// ids.txt.
func (s system) idsAt(ctx context.Context, root, rev string) ([]string, error) {
	content, ok, err := s.fileAt(ctx, root, rev, idsFile)
	if err != nil || !ok {
		return nil, err
	}
	ids, problems, err := parseIDs(bytes.NewReader(content), rev+":"+idsFile)
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return ids, nil
}

// checkIDsGrow returns a problem for each entry ID that a commit after base, up to HEAD, drops
// from the ids.txt of its first parent, and for each that the working tree's IDs, current, drop
// from HEAD's: ids.txt may only grow.
func (s system) checkIDsGrow(ctx context.Context, root, base string, current []string,
) ([]string, error) {
	out, err := s.git(ctx, root, "rev-list", "--reverse", base+"..HEAD")
	if err != nil {
		return nil, err
	}
	var problems []string
	for commit := range strings.FieldsSeq(string(out)) {
		dropped, commitErr := s.droppedByCommit(ctx, root, commit)
		if commitErr != nil {
			return nil, commitErr
		}
		problems = append(problems, dropped...)
	}
	head, err := s.idsAt(ctx, root, "HEAD")
	if err != nil {
		return nil, err
	}
	return append(problems, droppedIDs(head, current, "the working tree")...), nil
}

// droppedByCommit returns a problem for each entry ID that the commit drops from the ids.txt of
// its first parent. A commit without a parent has none to drop.
func (s system) droppedByCommit(ctx context.Context, root, commit string) ([]string, error) {
	parents, err := s.git(ctx, root, "rev-list", "--parents", "-n", "1", commit)
	if err != nil {
		return nil, err
	}
	var before []string
	if fields := strings.Fields(string(parents)); len(fields) > 1 {
		if before, err = s.idsAt(ctx, root, fields[1]); err != nil {
			return nil, err
		}
	}
	after, err := s.idsAt(ctx, root, commit)
	if err != nil {
		return nil, err
	}
	return droppedIDs(before, after, "commit "+commit[:12]), nil
}

// droppedIDs returns a problem for each ID of before that after does not list.
func droppedIDs(before, after []string, where string) []string {
	kept := map[string]bool{}
	for _, id := range after {
		kept[id] = true
	}
	var problems []string
	for _, id := range before {
		if !kept[id] {
			problems = append(problems, fmt.Sprintf("%s removes %s from %s; an entry ID is "+
				"never removed or renumbered: restore it, and give a new entry a new ID", where,
				id, idsFile))
		}
	}
	return problems
}
