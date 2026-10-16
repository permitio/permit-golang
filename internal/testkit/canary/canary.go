// Package canary makes fake Permit API keys and finds them in output, so that a test can prove a
// key never reaches a log, an error or a printed value. It is for tests only.
package canary

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// Prefix starts every fake Permit API key. The repository's secret scan ignores keys that start
// with it.
const Prefix = "permit_key_FAKE"

// keyLength is the length of every key that Key returns.
const keyLength = 64

// maxLabel is the longest label Key takes: the key holds Prefix, the label and at least one zero.
const maxLabel = keyLength - len(Prefix) - 1

// Key returns a fake Permit API key for label: Prefix, then label, then zeros up to 64
// characters. The label tells the keys of a test apart, so it holds 1 to 48 ASCII letters and
// digits; Key panics on any other label, as regexp.MustCompile does on a bad pattern, because the
// label is a literal in a test.
func Key(label string) string {
	if !validLabel(label) {
		panic(fmt.Sprintf("canary.Key(%q): the label must be 1 to %d ASCII letters and digits",
			label, maxLabel))
	}
	return Prefix + label + strings.Repeat("0", keyLength-len(Prefix)-len(label))
}

func validLabel(label string) bool {
	if label == "" || len(label) > maxLabel {
		return false
	}
	for _, c := range []byte(label) {
		letter := 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
		digit := '0' <= c && c <= '9'
		if !letter && !digit {
			return false
		}
	}
	return true
}

// Scanner is an io.Writer that looks for secrets in everything written to it, also a secret that
// one write starts and a later write ends. Write to it what a test must keep free of secrets,
// such as a logger's output or an error formatted with %+v, then call Check. It is safe for
// concurrent use.
type Scanner struct {
	secrets []string

	mu      sync.Mutex
	tail    []byte
	written int64
	found   map[string]bool
}

// NewScanner returns a Scanner that looks for each of secrets.
func NewScanner(secrets ...string) *Scanner {
	return &Scanner{secrets: secrets, found: map[string]bool{}}
}

// Write looks for the secrets in p and in the bytes before it that could start one. It never
// fails.
func (s *Scanner) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.written += int64(len(p))
	window := append(s.tail, p...)
	longest := 0
	for _, secret := range s.secrets {
		if secret != "" && bytes.Contains(window, []byte(secret)) {
			s.found[secret] = true
		}
		longest = max(longest, len(secret))
	}
	// The tail keeps enough of the stream to hold a secret that the next write ends.
	keep := min(len(window), longest)
	s.tail = bytes.Clone(window[len(window)-keep:])
	return len(p), nil
}

// Check fails the test for each secret that was written to the scanner. So that a test cannot
// pass without checking anything, it also fails the test when the scanner has no secrets, has
// an empty secret, or was given no bytes to scan.
func (s *Scanner) Check(tb testing.TB) {
	tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.secrets) == 0 {
		tb.Errorf("canary: the scanner has no secrets to look for")
	}
	for i, secret := range s.secrets {
		switch {
		case secret == "":
			tb.Errorf("canary: secret %d of the scanner is empty", i)
		case s.found[secret]:
			tb.Errorf("canary: the output has secret %d, %q", i, secret)
		}
	}
	if s.written == 0 {
		tb.Errorf("canary: nothing was written to the scanner, so it found no secret")
	}
}
