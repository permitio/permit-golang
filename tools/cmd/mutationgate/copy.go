package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// checkCopy runs a package's tests without a mutant in a copy of its module, made
// as gremlins 0.6.0 makes the copies it tests mutants in: the module's directories
// and regular files, and nothing above its root. A test that reads a file outside
// the module, such as one of the repository's workflows, fails there for every
// mutant, which then counts as killed. checkCopy returns an error when the tests
// fail in the copy, so such a run does not pass. It runs with env, gremlins'
// environment, so the same tests are skipped and exec bounds them.
func (r *runner) checkCopy(m goModule, pkg goPackage, env []string) error {
	root, ok := r.copies[m.dir]
	if !ok {
		root = filepath.Join(r.dir, "copy-"+strconv.Itoa(len(r.copies)+1))
		if err := copyModule(m.dir, root); err != nil {
			return fmt.Errorf("copying the module %s as gremlins does: %w", m.path, err)
		}
		if r.copies == nil {
			r.copies = map[string]string{}
		}
		r.copies[m.dir] = root
	}
	rel, err := filepath.Rel(m.dir, pkg.dir)
	if err != nil {
		return err
	}
	args := []string{"test"}
	if r.opts.tags != "" {
		args = append(args, "-tags", r.opts.tags)
	}
	output, err := r.tools.goTest(filepath.Join(root, rel), append(args, pkg.importPath), env)
	if err != nil {
		return fmt.Errorf("the tests of %s fail without a mutant in a copy of its module, where "+
			"gremlins tests the mutants, so every mutant would count as killed: a test reads a "+
			"file outside the module, or depends on where the module is; skip it with -skip "+
			"(%w): %s", pkg.importPath, err, lastLines(output, 5))
	}
	return nil
}

// copyModule copies the directories and regular files under src to dst, as
// gremlins copies a module.
func copyModule(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode().IsRegular():
			return copyFile(p, target, info.Mode().Perm())
		}
		return nil
	})
}

func copyFile(src, dst string, perm fs.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	_, err = io.Copy(out, in)
	return err
}

// runGoTest runs the go command at goPath with args in dir and env, and returns its
// output, and an error when it fails.
func runGoTest(goPath, dir string, args, env []string) (string, error) {
	//nolint:gosec // G204: the gate tests the package it mutates, with its own flags.
	cmd := exec.CommandContext(context.Background(), goPath, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
