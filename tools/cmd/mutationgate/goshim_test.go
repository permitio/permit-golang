package main

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMutantTestRun(t *testing.T) {
	tests := []struct {
		args      []string
		wantFlags []string
		wantOK    bool
	}{
		{args: []string{"test", "-timeout", "5m2s", "-failfast", "m/pkg"}, wantOK: true},
		{
			args: []string{
				"test",
				"-tags",
				"e2e",
				"-timeout",
				"5m2s",
				"-failfast",
				"-cpu 2",
				"m/pkg",
			},
			wantFlags: []string{"-tags", "e2e"},
			wantOK:    true,
		},
		{args: []string{"test", "-timeout", "5m2s", "-failfast", "./..."}},
		{args: []string{"test", "-cover", "-coverprofile", "/w/coverage", "./..."}},
		{args: []string{"test", "m/pkg"}},
		{args: []string{"test", "-failfast"}},
		// -tags with no value before the package builds with no tags.
		{args: []string{"test", "-failfast", "-tags", "m/pkg"}, wantOK: true},
		{args: []string{"mod", "download"}},
		{args: []string{"test"}},
		{},
	}
	for _, tt := range tests {
		flags, ok := mutantTestRun(tt.args)
		if !slices.Equal(flags, tt.wantFlags) || ok != tt.wantOK {
			t.Errorf("%q: got %q %v", tt.args, flags, ok)
		}
	}
}

func TestParseGoShimArgs(t *testing.T) {
	realGo := filepath.Join(t.TempDir(), "go")
	opts, err := parseGoShimArgs(
		[]string{
			"-real-go=" + realGo,
			"-log=/w/events.log",
			"-package=m/cmd/x",
			"--",
			"test",
			"-v",
		},
		&bytes.Buffer{},
	)
	if err != nil || opts.realGo != realGo || opts.logPath != "/w/events.log" ||
		opts.pkg != "m/cmd/x" || strings.Join(opts.args, " ") != "test -v" {
		t.Errorf("got %+v, %v", opts, err)
	}
	for _, args := range [][]string{
		{"-package=m", "--", "env"},
		{"-real-go=go", "-package=m", "--", "env"},
		{"-real-go=" + realGo, "--", "env"},
		{"-real-go=" + realGo, "-package=-v", "--", "env"},
		{"-nope"},
	} {
		if _, err := parseGoShimArgs(args, &bytes.Buffer{}); err == nil {
			t.Errorf("%q: no error", args)
		}
	}
}

func TestParseGitShimArgs(t *testing.T) {
	dir := t.TempDir()
	realGit, diff := filepath.Join(dir, "git"), filepath.Join(dir, "scope.diff")
	opts, err := parseGitShimArgs([]string{
		"-real-git=" + realGit, "-log=/w/events.log",
		"-diff=" + diff, "--", "status", "-s",
	}, &bytes.Buffer{})
	if err != nil || opts.realGit != realGit || opts.logPath != "/w/events.log" ||
		opts.diffPath != diff || strings.Join(opts.args, " ") != "status -s" {
		t.Errorf("got %+v, %v", opts, err)
	}
	for _, args := range [][]string{
		{"-diff=" + diff, "--", "status"},
		{"-real-git=git", "-diff=" + diff, "--", "status"},
		{"-real-git=" + realGit, "--", "status"},
		{"-real-git=" + realGit, "-diff=x", "--", "status"},
		{"-nope"},
	} {
		if _, err := parseGitShimArgs(args, &bytes.Buffer{}); err == nil {
			t.Errorf("%q: no error", args)
		}
	}
}
