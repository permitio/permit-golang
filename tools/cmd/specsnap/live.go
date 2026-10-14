package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// digestPattern matches an image digest.
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// checkLive compares the live specs and Docker Hub's latest container PDP with the snapshots.
func checkLive(ctx context.Context, opts options, stdout io.Writer, sys system) int {
	lock, err := readLock(opts.dir)
	if err != nil {
		return verdict(stdout, exitDidNotRun, err.Error())
	}
	if err := checkLockNames(lock, sys.sources); err != nil {
		return verdict(stdout, exitDidNotRun, err.Error())
	}
	if opts.out != "" {
		if err := os.MkdirAll(opts.out, 0o750); err != nil {
			return verdict(stdout, exitDidNotRun, fmt.Sprintf("creating -out: %v", err))
		}
	}
	var results []result
	for _, src := range sys.sources {
		entry := lock[src.name]
		if src.url == "" {
			err := checkImage(ctx, sys, entry)
			results = append(results, report(stdout, src.name, err,
				"Docker Hub's latest "+pdpRepository+" is "+entry.Image))
			continue
		}
		err := checkSpec(ctx, sys, src, entry, opts.out)
		results = append(results, report(stdout, src.name, err,
			"the live spec matches the snapshot"))
	}
	return summarize(stdout, results, fmt.Sprintf("the %d live specs and images match %s",
		len(results), lockFile), "drift in")
}

// checkSpec fetches a published spec, writes its snapshot to the out directory when one is
// given, and compares the snapshot with the lock entry.
func checkSpec(ctx context.Context, sys system, src source, entry lockEntry, out string) error {
	raw, err := fetch(ctx, sys, src.url)
	if err != nil {
		return err
	}
	snap, err := makeSnapshot(src, raw)
	if err != nil {
		return err
	}
	if out != "" {
		//nolint:gosec // the snapshot of a public spec, for the job's artifact.
		if err := os.WriteFile(filepath.Join(out, src.file()), snap.data, 0o644); err != nil {
			return fmt.Errorf("writing the live snapshot: %w", err)
		}
	}
	if sum := sha256Hex(snap.data); sum != entry.SHA256 {
		return failure{fmt.Errorf("the live spec changed: its snapshot has sha256 %s and %s; "+
			"%s has sha256 %s and %s, fetched %s", sum, count(snap.operations, "operation"),
			entry.File, entry.SHA256, count(entry.Operations, "operation"), entry.Fetched)}
	}
	return nil
}

// hubTags is the part of Docker Hub's tag list that specsnap reads.
type hubTags struct {
	Results []struct {
		Name   string `json:"name"`
		Digest string `json:"digest"`
	} `json:"results"`
}

// checkImage compares the digest of Docker Hub's latest container PDP with the image the
// snapshot was taken from.
func checkImage(ctx context.Context, sys system, entry lockEntry) error {
	if !digestPattern.MatchString(entry.Digest) {
		return fmt.Errorf("%s records the digest %q, which is not sha256:<64 hex digits>",
			lockFile, entry.Digest)
	}
	data, err := fetch(ctx, sys, sys.hubURL)
	if err != nil {
		return err
	}
	var tags hubTags
	if err := json.Unmarshal(data, &tags); err != nil {
		return fmt.Errorf("parsing Docker Hub's tags of %s: %w", pdpRepository, err)
	}
	latest := ""
	for _, tag := range tags.Results {
		if tag.Name == "latest" {
			latest = tag.Digest
		}
	}
	switch {
	case latest == "":
		return errors.New("the newest 100 tags of " + pdpRepository + " on Docker Hub have no " +
			"latest")
	case !digestPattern.MatchString(latest):
		return fmt.Errorf("the latest tag on Docker Hub has the digest %q, which is not "+
			"sha256:<64 hex digits>", latest)
	case latest != entry.Digest:
		var same []string
		for _, tag := range tags.Results {
			if tag.Digest == latest && tag.Name != "latest" {
				same = append(same, tag.Name)
			}
		}
		slices.Sort(same)
		named := "no other tag"
		if len(same) > 0 {
			named = "also tagged " + strings.Join(same, ", ")
		}
		return failure{fmt.Errorf("latest %s on Docker Hub is %s (%s), and the snapshot was "+
			"taken from %s@%s", pdpRepository, latest, named, entry.Image, entry.Digest)}
	}
	return nil
}
