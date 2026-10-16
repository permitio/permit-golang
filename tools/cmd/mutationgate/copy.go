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

	"golang.org/x/mod/modfile"
)

// checkCopy runs a package's tests without a mutant in a copy of its module, made
// as gremlins 0.6.0 makes the copies it tests mutants in: the module's directories
// and regular files, and nothing above its root. A test that reads a file outside
// the module, such as one of the repository's workflows, fails there for every
// mutant, which then counts as killed. checkCopy returns an error when the tests
// fail in the copy, so such a run does not pass. It runs with env, gremlins'
// environment, so the same tests are skipped and exec bounds them, and with
// modFile, the module file the go shim builds the mutants with.
func (r *runner) checkCopy(m goModule, pkg goPackage, modFile string, env []string) error {
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
	args := []string{"test", "-modfile=" + modFile}
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

// moduleFile returns the module file that go builds the module m with in a copy of
// it, where a replacement by a relative directory, such as the tools module's
// replacement of the SDK with ../, points nowhere. It writes the file, once for
// each module, to the work directory.
func (r *runner) moduleFile(m goModule) (string, error) {
	if p, ok := r.modFiles[m.dir]; ok {
		return p, nil
	}
	r.modDirs++
	dir := filepath.Join(r.dir, "module-"+strconv.Itoa(r.modDirs))
	p := filepath.Join(dir, "go.mod")
	err := os.Mkdir(dir, 0o700)
	if err == nil {
		err = writeModuleFile(m.dir, p)
	}
	if err != nil {
		return "", fmt.Errorf("writing the module file of %s for the copies of the module: %w",
			m.path, err)
	}
	if r.modFiles == nil {
		r.modFiles = map[string]string{}
	}
	r.modFiles[m.dir] = p
	return p, nil
}

// writeModuleFile writes the go.mod of the module whose root is dir to dst, which
// must end in .mod, with each replacement by a relative directory made absolute,
// and copies the module's go.sum, when it has one, beside dst, where go -modfile
// reads it. go resolves a relative directory from the module root, not from the
// module file, so the copy of a module built with dst uses the directory the
// module itself uses.
func writeModuleFile(dir, dst string) error {
	src := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, err := modfile.Parse(src, data, nil)
	if err != nil {
		return err
	}
	for _, r := range f.Replace {
		if r.New.Version != "" || filepath.IsAbs(r.New.Path) {
			continue
		}
		// A replacement by a directory ends with the directory.
		tokens := r.Syntax.Token
		tokens[len(tokens)-1] = modfile.AutoQuote(filepath.Join(dir, r.New.Path))
	}
	//nolint:gosec // G703: dst is a file the gate names in its own work directory.
	if err := os.WriteFile(dst, modfile.Format(f.Syntax), 0o600); err != nil {
		return err
	}
	sum := filepath.Join(dir, "go.sum")
	if _, err := os.Stat(sum); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return copyFile(sum, strings.TrimSuffix(dst, ".mod")+".sum", 0o600)
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
