package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	// rulesFile lists the validation rules of the SDK, one per line as <ID> <message>.
	rulesFile = "testdata/validation-rules.txt"
	// modelMapFile maps each models type to its spec schema, with a required flag per field.
	modelMapFile = "tools/drift/model-map.yaml"
)

// breakingChange is a change of the pull request that breaks code written against its base.
type breakingChange struct {
	// source is what found it: apidiff, the validation rules or the model map.
	source string
	text   string
}

// diff finds the breaking changes between the commit base and the working tree: the
// incompatible changes apidiff reports for the module's packages but the internal ones, each
// validation rule whose ID is new in testdata/validation-rules.txt, and each field of an
// existing models type that tools/drift/model-map.yaml makes required, new fields included. It
// fails with an error, for exit 2, when the base cannot be read, apidiff cannot load either
// side, or a file cannot be read or parsed.
func diff(ctx context.Context, opts options, sys system, modulePath string,
) ([]breakingChange, []string, error) {
	var notes []string
	changes, err := apidiffChanges(ctx, opts, sys, modulePath)
	if err != nil {
		return nil, nil, err
	}
	rules, err := addedRules(ctx, opts, sys)
	if err != nil {
		return nil, nil, err
	}
	changes = append(changes, rules...)
	required, note, err := newlyRequired(ctx, opts, sys)
	if err != nil {
		return nil, nil, err
	}
	if note != "" {
		notes = append(notes, note)
	}
	return append(changes, required...), notes, nil
}

// apidiffChanges writes the export data of the module at the commit base with apidiff -m -w,
// compares the working tree with it, and returns the incompatible changes apidiff reports.
// apidiff exits 0 when it finds incompatible changes, so they are read from its output.
func apidiffChanges(ctx context.Context, opts options, sys system, modulePath string,
) ([]breakingChange, error) {
	if opts.apidiff == "" {
		return nil, errors.New("-apidiff names no apidiff binary; build it with go -C tools " +
			"build -o ../bin/apidiff golang.org/x/exp/cmd/apidiff")
	}
	apidiff, err := filepath.Abs(opts.apidiff)
	if err != nil {
		return nil, err
	}
	opts.apidiff = apidiff
	work, err := os.MkdirTemp("", "apisurface-base-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	export, err := baseExport(ctx, opts, sys, modulePath, work)
	if err != nil {
		return nil, err
	}
	out, stderr, err := sys.runIn(ctx, opts.root, sys.environ(), apidiff, "-m", export,
		modulePath)
	if err != nil {
		return nil, fmt.Errorf("apidiff could not compare %s with %s: %w: %s", modulePath,
			opts.base, err, strings.TrimSpace(string(stderr)))
	}
	incompatible, err := parseAPIDiff(out)
	if err != nil {
		return nil, err
	}
	var changes []breakingChange
	for _, text := range incompatible {
		changes = append(changes, breakingChange{source: "apidiff", text: text})
	}
	return changes, nil
}

// baseExport writes the files of the commit opts.base under work, and the export data of its
// module there with apidiff -m -w, and returns the export data's path. opts.apidiff must be an
// absolute path. It fails when apidiff cannot load the module or writes no export data.
func baseExport(ctx context.Context, opts options, sys system, modulePath, work string,
) (string, error) {
	tree := filepath.Join(work, "tree")
	if err := sys.extractCommit(ctx, opts.root, opts.base, tree); err != nil {
		return "", err
	}
	export := filepath.Join(work, "base.export")
	_, stderr, err := sys.runIn(ctx, tree, sys.environ(), opts.apidiff, "-m", "-w", export,
		modulePath)
	if err != nil {
		return "", fmt.Errorf("apidiff could not load %s at %s: %w: %s", modulePath, opts.base,
			err, strings.TrimSpace(string(stderr)))
	}
	if info, statErr := os.Stat(export); statErr != nil || info.Size() == 0 {
		return "", fmt.Errorf("apidiff wrote no export data for %s at %s", modulePath, opts.base)
	}
	return export, nil
}

// parseAPIDiff returns the incompatible changes in apidiff's output: the "- " lines under
// "Incompatible changes:", up to "Compatible changes:". It fails on any other line, so that a
// change in apidiff's output format cannot pass as no change.
func parseAPIDiff(out []byte) ([]string, error) {
	var incompatible []string
	section := ""
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "Incompatible changes:" || line == "Compatible changes:":
			section = line
		case strings.HasPrefix(line, "Ignoring internal package "):
		case strings.HasPrefix(line, "- ") && section != "":
			if section == "Incompatible changes:" {
				incompatible = append(incompatible, strings.TrimPrefix(line, "- "))
			}
		default:
			return nil, fmt.Errorf("apidiff printed a line apisurface does not know: %q", line)
		}
	}
	return incompatible, sc.Err()
}

// extractCommit writes the files of the commit rev of the repository at root into dir, with
// git archive.
func (s system) extractCommit(ctx context.Context, root, rev, dir string) error {
	out, err := s.git(ctx, root, "archive", "--format=tar", rev)
	if err != nil {
		return err
	}
	tr := tar.NewReader(bytes.NewReader(out))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading git archive %s: %w", rev, err)
		}
		if hdr.Typeflag != tar.TypeReg || !filepath.IsLocal(hdr.Name) {
			continue
		}
		if err := writeArchived(tr, filepath.Join(dir, filepath.FromSlash(hdr.Name))); err != nil {
			return fmt.Errorf("writing %s of git archive %s: %w", hdr.Name, rev, err)
		}
	}
}

// writeArchived writes the content of the current file of an archive to name.
func writeArchived(tr *tar.Reader, name string) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		return err
	}
	content, err := io.ReadAll(tr)
	if err != nil {
		return err
	}
	return os.WriteFile(name, content, 0o600)
}

// addedRules returns a breaking change for each validation rule whose ID is in the working
// tree's testdata/validation-rules.txt but not in the base's: a new rule rejects input that the
// base accepted.
func addedRules(ctx context.Context, opts options, sys system) ([]breakingChange, error) {
	base, ok, err := sys.fileAt(ctx, opts.root, opts.base, rulesFile)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s has no %s", opts.base, rulesFile)
	}
	head, err := os.ReadFile(filepath.Join(opts.root, rulesFile))
	if err != nil {
		return nil, err
	}
	before, err := ruleIDs(base)
	if err != nil {
		return nil, fmt.Errorf("%s:%s: %w", opts.base, rulesFile, err)
	}
	after, err := ruleIDs(head)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rulesFile, err)
	}
	var changes []breakingChange
	for _, id := range after {
		if !slices.Contains(before, id) {
			changes = append(changes, breakingChange{
				source: rulesFile,
				text:   "validation rule " + id + ": added; it rejects input that was accepted",
			})
		}
	}
	return changes, nil
}

// ruleIDs returns the IDs of the rules in a validation rules file: the first word of each line
// but blank lines and comments. It fails on a line with no message and on a repeated ID.
func ruleIDs(content []byte) ([]string, error) {
	var ids []string
	var problem error
	err := eachLine(bytes.NewReader(content), func(n int, line string) {
		id, message, _ := strings.Cut(line, " ")
		switch {
		case problem != nil:
		case strings.TrimSpace(message) == "":
			problem = fmt.Errorf("line %d, %q, is not <ID> <message>", n, line)
		case slices.Contains(ids, id):
			problem = fmt.Errorf("line %d repeats the rule ID %s", n, id)
		default:
			ids = append(ids, id)
		}
	})
	return ids, errors.Join(err, problem)
}

// modelMap is the part of tools/drift/model-map.yaml that apisurface reads: for each models
// type, its fields with their required flags. The other keys of the file are ignored.
type modelMap struct {
	Models map[string]struct {
		Fields map[string]struct {
			Required bool `yaml:"required"`
		} `yaml:"fields"`
	} `yaml:"models"`
}

// newlyRequired returns a breaking change for each field of a models type of the base's model
// map that the working tree's map marks required and the base's has not required, or does not
// have: a request model then rejects input that the base accepted. A models type that only the
// working tree's map has is new, and its required fields break nothing. When neither commit has
// the file it returns a note instead; when only the base has it, an error.
func newlyRequired(ctx context.Context, opts options, sys system,
) ([]breakingChange, string, error) {
	base, inBase, err := sys.fileAt(ctx, opts.root, opts.base, modelMapFile)
	if err != nil {
		return nil, "", err
	}
	head, err := os.ReadFile(filepath.Join(opts.root, modelMapFile))
	inHead := err == nil
	switch {
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return nil, "", err
	case !inHead && inBase:
		return nil, "", fmt.Errorf("%s is in %s but not in the working tree", modelMapFile,
			opts.base)
	case !inHead:
		return nil, "no " + modelMapFile + " yet: no field can turn required", nil
	case !inBase:
		return nil, modelMapFile + " is new: no field turned required", nil
	}
	before, err := parseModelMap(base)
	if err != nil {
		return nil, "", fmt.Errorf("%s:%s: %w", opts.base, modelMapFile, err)
	}
	after, err := parseModelMap(head)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", modelMapFile, err)
	}
	var changes []breakingChange
	for _, model := range slices.Sorted(maps.Keys(after.Models)) {
		baseModel, known := before.Models[model]
		if !known {
			continue
		}
		fields := after.Models[model].Fields
		for _, field := range slices.Sorted(maps.Keys(fields)) {
			old, existed := baseModel.Fields[field]
			if !fields[field].Required || old.Required {
				continue
			}
			change := "turned required"
			if !existed {
				change = "added as required"
			}
			changes = append(changes, breakingChange{
				source: modelMapFile,
				text:   "models." + model + " field " + field + ": " + change,
			})
		}
	}
	return changes, "", nil
}

// parseModelMap parses a model map. It fails when the map lists no models type.
func parseModelMap(content []byte) (modelMap, error) {
	var m modelMap
	if err := yaml.Unmarshal(content, &m); err != nil {
		return modelMap{}, err
	}
	if len(m.Models) == 0 {
		return modelMap{}, errors.New("its models key maps no models type")
	}
	return m, nil
}
