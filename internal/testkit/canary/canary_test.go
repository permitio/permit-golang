package canary

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/faketb"
)

// gitleaksKey is a copy of the regex of the permit-api-key rule in .gitleaks.toml, and
// gitleaksFake a copy of the regex of its allowlist for fake keys, so that TestKey runs where the
// repository's files are not, as in a mutation test. TestRepositoryGitleaksRules checks that the
// copies are the same as the file's.
var (
	gitleaksKey  = regexp.MustCompile(`permit_key_[A-Za-z0-9]{20,}`)
	gitleaksFake = regexp.MustCompile(`^permit_key_FAKE[A-Za-z0-9_]*$`)
)

func TestKey(t *testing.T) {
	for _, label := range []string{"a", "API", "pdp2", strings.Repeat("x", maxLabel)} {
		key := Key(label)
		if len(key) != keyLength {
			t.Errorf("Key(%q) = %q, %d characters long, want %d", label, key, len(key), keyLength)
		}
		if want := Prefix + label; !strings.HasPrefix(key, want) {
			t.Errorf("Key(%q) = %q, want it to start with %q", label, key, want)
		}
		if rest := strings.TrimPrefix(key, Prefix+label); strings.Trim(rest, "0") != "" ||
			rest == "" {
			t.Errorf("Key(%q) = %q, want zeros after the label", label, key)
		}
		if match := gitleaksKey.FindString(key); match != key || !gitleaksFake.MatchString(key) {
			t.Errorf("Key(%q) = %q: the secret scan's key pattern matches %q and its fake key "+
				"allowlist matches %t, want the whole key for both", label, key, match,
				gitleaksFake.MatchString(key))
		}
	}
	if Key("one") == Key("two") {
		t.Error("Key returns the same key for two labels")
	}
}

// TestRepositoryGitleaksRules checks that gitleaksKey and gitleaksFake are the regexes of the
// permit-api-key rule and of the allowlist in this repository's .gitleaks.toml.
func TestRepositoryGitleaksRules(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", ".gitleaks.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		// find finds the regex in the file; its first group is the regex.
		find  *regexp.Regexp
		local *regexp.Regexp
	}{
		{
			name: "the regex of the permit-api-key rule",
			find: regexp.MustCompile(`(?m)^id = "permit-api-key"\n(?:\w+ = .*\n)*?` +
				`regex = '''(.*)'''$`),
			local: gitleaksKey,
		},
		{
			name: "the regex of the allowlist",
			find: regexp.MustCompile(`(?m)^\[\[allowlists\]\]\n(?:\w+ = .*\n)*?` +
				`regexes = \['''(.*)'''\]$`),
			local: gitleaksFake,
		},
	} {
		match := tt.find.FindSubmatch(data)
		if match == nil {
			t.Errorf(".gitleaks.toml has no %s that the pattern %s finds", tt.name, tt.find)
			continue
		}
		if got := string(match[1]); got != tt.local.String() {
			t.Errorf(".gitleaks.toml has %s %s, and the test's copy is %s; make them the same",
				tt.name, got, tt.local)
		}
	}
}

func TestKeyPanicsOnABadLabel(t *testing.T) {
	bad := []string{
		"", "with space", "dash-ed", "under_score", "\u00fcn\u00efcode", "a/b", "0:1", "@",
		strings.Repeat("x", maxLabel+1),
	}
	for _, label := range bad {
		t.Run(fmt.Sprintf("%q", label), func(t *testing.T) {
			defer func() {
				got := recover()
				if got == nil || !strings.Contains(fmt.Sprint(got), "the label must be") {
					t.Errorf("Key(%q) panicked with %v, want a panic about the label", label, got)
				}
			}()
			key := Key(label)
			t.Errorf("Key(%q) = %q, want a panic", label, key)
		})
	}
}

func TestValidLabelBoundaries(t *testing.T) {
	for _, c := range []byte("09azAZ") {
		if !validLabel(string(c)) {
			t.Errorf("validLabel(%q) = false, want true", c)
		}
	}
	for _, c := range []byte("/:@[`{") {
		if validLabel(string(c)) {
			t.Errorf("validLabel(%q) = true, want false", c)
		}
	}
}

func TestScannerFindsSecrets(t *testing.T) {
	api, pdp, other := Key("api"), Key("pdp"), Key("other")
	secrets := []string{api, pdp, other}
	tests := []struct {
		name   string
		writes []string
		want   []int // the indexes of the secrets Check reports
	}{
		{name: "no secret", writes: []string{"Authorization: Bearer ***"}},
		{name: "one write", writes: []string{"key=" + api + " sent"}, want: []int{0}},
		{
			name:   "split across writes",
			writes: []string{"key=" + api[:20], api[20:40], api[40:] + "\n"},
			want:   []int{0},
		},
		{
			name:   "split one byte at a time",
			writes: strings.Split("x"+pdp+"y", ""),
			want:   []int{1},
		},
		{name: "two secrets", writes: []string{pdp, " and ", api}, want: []int{0, 1}},
		{name: "a secret split by an unrelated write", writes: []string{api[:30], "\n", api[30:]}},
		{name: "a prefix of the secret", writes: []string{api[:len(api)-1]}},
		{
			name:   "after more than a secret's length of output",
			writes: []string{strings.Repeat("a", 200), api[:10], api[10:]},
			want:   []int{0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScanner(secrets...)
			for _, w := range tt.writes {
				n, err := s.Write([]byte(w))
				if n != len(w) || err != nil {
					t.Fatalf("Write(%q) = %d, %v, want %d, nil", w, n, err, len(w))
				}
			}
			rec := faketb.New(t)
			s.Check(rec)
			got := rec.Take()
			var want []string
			for _, i := range tt.want {
				want = append(
					want,
					fmt.Sprintf("canary: the output has secret %d, %q", i, secrets[i]),
				)
			}
			if !slices.Equal(got, want) {
				t.Errorf("Check reported %q, want %q", got, want)
			}
		})
	}
}

func TestScannerIsSafeForConcurrentUse(t *testing.T) {
	key := Key("concurrent")
	s := NewScanner(key)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				_, _ = s.Write([]byte("log line without the key\n"))
			}
		})
	}
	wg.Go(func() { _, _ = fmt.Fprintf(s, "%s\n", key) })
	wg.Wait()
	rec := faketb.New(t)
	s.Check(rec)
	got := rec.Take()
	if len(got) != 1 || !strings.Contains(got[0], key) {
		t.Errorf("Check reported %q, want the key", got)
	}
}

func TestCheckFailsWhenItCannotCheck(t *testing.T) {
	tests := []struct {
		name    string
		secrets []string
		write   string
		want    []string
	}{
		{
			name:  "no secrets",
			write: "output",
			want:  []string{"canary: the scanner has no secrets to look for"},
		},
		{
			name:    "an empty secret",
			secrets: []string{Key("a"), ""},
			write:   "output",
			want:    []string{"canary: secret 1 of the scanner is empty"},
		},
		{
			name:    "nothing written",
			secrets: []string{Key("a")},
			want:    []string{"canary: nothing was written to the scanner, so it found no secret"},
		},
		{
			name:    "everything at once",
			secrets: []string{""},
			want: []string{
				"canary: secret 0 of the scanner is empty",
				"canary: nothing was written to the scanner, so it found no secret",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScanner(tt.secrets...)
			if tt.write != "" {
				_, _ = s.Write([]byte(tt.write))
			}
			rec := faketb.New(t)
			s.Check(rec)
			got := rec.Take()
			if !slices.Equal(got, tt.want) {
				t.Errorf("Check reported %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckPassesCleanOutput(t *testing.T) {
	s := NewScanner(Key("a"), Key("b"))
	_, _ = s.Write([]byte("Authorization: Bearer [redacted]"))
	rec := faketb.New(t)
	s.Check(rec)
	got := rec.Take()
	if len(got) != 0 {
		t.Errorf("Check reported %q for output without a secret, want nothing", got)
	}
}
