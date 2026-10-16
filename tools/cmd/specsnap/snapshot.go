package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// lockFile is the name of the lock in the snapshot directory.
const lockFile = "lock.json"

// lockEntry is what lock.json records about a snapshot.
type lockEntry struct {
	// File is the snapshot's file name in the snapshot directory.
	File string `json:"file"`
	// URL is where the spec is published.
	URL string `json:"url,omitempty"`
	// Image is the container PDP image the spec was read from, as permitio/pdp-v2:<tag>.
	Image string `json:"image,omitempty"`
	// Digest is the image's digest, as sha256:<hex>.
	Digest string `json:"digest,omitempty"`
	// Note is a line of text about the spec, such as who took it.
	Note string `json:"note,omitempty"`
	// Fetched is the day the spec was fetched, as YYYY-MM-DD.
	Fetched string `json:"fetched"`
	// SourceSHA256 is the sha256 of the spec as fetched.
	SourceSHA256 string `json:"source_sha256"`
	// SHA256 is the sha256 of the snapshot.
	SHA256 string `json:"sha256"`
	// Operations is the number of operations in the snapshot.
	Operations int `json:"operations"`
}

// snapshot is a spec in canonical form.
type snapshot struct {
	data       []byte
	sourceSHA  string
	operations int
	fixes      []string
}

// makeSnapshot pre-fixes and canonicalizes a spec. Its error is a failure when a pre-fix patch
// failed; any other error means the spec cannot be used.
func makeSnapshot(src source, raw []byte) (snapshot, error) {
	doc, err := parse(raw)
	if err != nil {
		return snapshot{}, err
	}
	operations := countOperations(doc)
	if operations < src.minOperations {
		return snapshot{}, fmt.Errorf("it has %s, and a complete %s spec has at least %d: "+
			"it is cut short, or not an OpenAPI document", count(operations, "operation"),
			src.name, src.minOperations)
	}
	fixes, err := prefix(doc, src.patches)
	if err != nil {
		return snapshot{}, err
	}
	data, err := canonical(doc)
	if err != nil {
		return snapshot{}, err
	}
	return snapshot{data: data, sourceSHA: sha256Hex(raw), operations: operations, fixes: fixes},
		nil
}

// refresh writes the snapshot of the spec that -refresh names and records it in lock.json.
func refresh(ctx context.Context, opts options, stdout io.Writer, sys system) int {
	src, _ := findSource(sys.sources, opts.refresh)
	lock, err := readLock(opts.dir)
	if errors.Is(err, fs.ErrNotExist) {
		lock, err = map[string]lockEntry{}, nil
	}
	if err != nil {
		return verdict(stdout, exitDidNotRun, err.Error())
	}
	var raw []byte
	if opts.from != "" {
		raw, err = os.ReadFile(opts.from)
	} else {
		raw, err = fetch(ctx, sys, src.url)
	}
	if err != nil {
		return verdict(stdout, exitDidNotRun, fmt.Sprintf("reading the %s spec: %v", src.name,
			err))
	}
	snap, err := makeSnapshot(src, raw)
	if err != nil {
		return verdict(stdout, exitCode(err), fmt.Sprintf("the %s spec: %v", src.name, err))
	}
	for _, line := range snap.fixes {
		fmt.Fprintf(stdout, "%s: %s\n", src.name, line)
	}
	entry := lockEntry{
		File:         src.file(),
		URL:          src.url,
		Note:         opts.note,
		Fetched:      opts.fetched,
		SourceSHA256: snap.sourceSHA,
		SHA256:       sha256Hex(snap.data),
		Operations:   snap.operations,
	}
	if entry.Fetched == "" {
		entry.Fetched = sys.now().UTC().Format(time.DateOnly)
	}
	if src.url == "" {
		entry.Image, entry.Digest, _ = strings.Cut(opts.image, "@")
	}
	lock[src.name] = entry
	lockData, err := encodeLock(lock)
	if err != nil {
		return verdict(stdout, exitDidNotRun, err.Error())
	}
	err = replaceFiles(opts.dir, []pendingFile{
		{name: entry.File, data: snap.data},
		{name: lockFile, data: lockData},
	})
	if err != nil {
		return verdict(stdout, exitDidNotRun, err.Error())
	}
	return verdict(stdout, exitPassed, fmt.Sprintf("wrote %s: %s, sha256 %s",
		filepath.Join(opts.dir, entry.File), count(entry.Operations, "operation"), entry.SHA256))
}

// pendingFile is a file that refresh writes in the snapshot directory.
type pendingFile struct {
	name string
	data []byte
}

// replaceFiles writes the files in dir so that a failed write replaces none of them: it writes
// each to <name>.tmp beside it, and renames those over the files only once all of them are
// written. When it fails, it removes the .tmp files it wrote.
func replaceFiles(dir string, files []pendingFile) error {
	var temps []string
	for _, file := range files {
		temp := filepath.Join(dir, file.name+".tmp")
		//nolint:gosec // the snapshots and lock.json are committed and public.
		if err := os.WriteFile(temp, file.data, 0o644); err != nil {
			removeFiles(temps)
			return fmt.Errorf("writing %s: %w", file.name, err)
		}
		temps = append(temps, temp)
	}
	for i, file := range files {
		if err := os.Rename(temps[i], filepath.Join(dir, file.name)); err != nil {
			removeFiles(temps[i:])
			return fmt.Errorf("writing %s: %w", file.name, err)
		}
	}
	return nil
}

// removeFiles removes the files at the paths. It is the cleanup after a failed write, whose
// error is the one to report, so it ignores its own errors.
func removeFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

// verify checks every snapshot against lock.json.
func verify(dir string, stdout io.Writer, sys system) int {
	lock, err := readLock(dir)
	if err != nil {
		return verdict(stdout, exitDidNotRun, err.Error())
	}
	if err := checkLockNames(lock, sys.sources); err != nil {
		return verdict(stdout, exitDidNotRun, err.Error())
	}
	var results []result
	for _, src := range sys.sources {
		err := verifySnapshot(dir, src, lock[src.name])
		results = append(results, report(stdout, src.name, err, "matches "+lockFile))
	}
	return summarize(stdout, results, fmt.Sprintf("the %d snapshots match %s", len(results),
		lockFile), "failed for")
}

// verifySnapshot checks one snapshot against its lock entry.
func verifySnapshot(dir string, src source, entry lockEntry) error {
	if entry.File != src.file() {
		return fmt.Errorf("%s names the file %q, not %q", lockFile, entry.File, src.file())
	}
	data, err := os.ReadFile(filepath.Join(dir, entry.File))
	if err != nil {
		return fmt.Errorf("reading the snapshot: %w", err)
	}
	if sum := sha256Hex(data); sum != entry.SHA256 {
		return failure{fmt.Errorf("%s has sha256 %s, and %s records %s: refresh it with "+
			"specsnap -refresh, never by hand", entry.File, sum, lockFile, entry.SHA256)}
	}
	doc, err := parse(data)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", entry.File, err)
	}
	operations := countOperations(doc)
	if operations < src.minOperations {
		return fmt.Errorf("%s has %s, and a complete %s spec has at least %d", entry.File,
			count(operations, "operation"), src.name, src.minOperations)
	}
	if operations != entry.Operations {
		return failure{fmt.Errorf("%s has %s, and %s records %d", entry.File,
			count(operations, "operation"), lockFile, entry.Operations)}
	}
	want, err := canonical(doc)
	if err != nil {
		return err
	}
	if !bytes.Equal(want, data) {
		return failure{fmt.Errorf("%s is not in canonical form: refresh it with specsnap "+
			"-refresh", entry.File)}
	}
	return nil
}

// readLock reads lock.json from the snapshot directory.
func readLock(dir string) (map[string]lockEntry, error) {
	data, err := os.ReadFile(filepath.Join(dir, lockFile))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", lockFile, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var lock map[string]lockEntry
	if err := dec.Decode(&lock); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", lockFile, err)
	}
	if lock == nil {
		return nil, fmt.Errorf("parsing %s: it is not a JSON object", lockFile)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: it has more data after its JSON object", lockFile)
	}
	return lock, nil
}

// checkLockNames checks that the lock has an entry for every source and for nothing else.
func checkLockNames(lock map[string]lockEntry, list []source) error {
	for name := range lock {
		if _, ok := findSource(list, name); !ok {
			return fmt.Errorf("%s has an entry for %q, which is not one of %s", lockFile, name,
				sourceNames(list))
		}
	}
	for _, src := range list {
		if _, ok := lock[src.name]; !ok {
			return fmt.Errorf("%s has no entry for %s: write it with specsnap -refresh %s",
				lockFile, src.name, src.name)
		}
	}
	return nil
}

// encodeLock returns the content of lock.json: the lock with its keys sorted, indented by two
// spaces.
func encodeLock(lock map[string]lockEntry) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(lock); err != nil {
		return nil, fmt.Errorf("writing %s: %w", lockFile, err)
	}
	return buf.Bytes(), nil
}

// fetch reads a URL, which must answer 200 with at most sys.maxBody bytes.
func fetch(ctx context.Context, sys system, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("building the request for %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := sys.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, sys.maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	if int64(len(data)) > sys.maxBody {
		return nil, fmt.Errorf("%s answered with more than %d bytes", url, sys.maxBody)
	}
	return data, nil
}

// result is the outcome of one spec's check.
type result struct {
	name string
	code int
}

// report prints a spec's line and returns its result: PASS with the pass text, or the error.
func report(stdout io.Writer, name string, err error, pass string) result {
	if err == nil {
		fmt.Fprintf(stdout, "%s: PASS: %s\n", name, pass)
		return result{name: name, code: exitPassed}
	}
	code := exitCode(err)
	fmt.Fprintf(stdout, "%s: %s: %v\n", name, verdictWord(code), err)
	return result{name: name, code: code}
}

// summarize prints the verdict over the results and returns its exit code: 2 when a check did
// not run, else 1 when one failed, else 0. The verdict is pass when every check passed, and
// otherwise names the specs whose check did not complete, and the specs whose check failed
// after failed.
func summarize(stdout io.Writer, results []result, pass, failed string) int {
	var failedNames, didNotRun []string
	for _, r := range results {
		switch r.code {
		case exitFailed:
			failedNames = append(failedNames, r.name)
		case exitDidNotRun:
			didNotRun = append(didNotRun, r.name)
		}
	}
	var parts []string
	if len(didNotRun) > 0 {
		parts = append(parts, "did not complete for "+strings.Join(didNotRun, ", "))
	}
	if len(failedNames) > 0 {
		parts = append(parts, failed+" "+strings.Join(failedNames, ", "))
	}
	switch {
	case len(didNotRun) > 0:
		return verdict(stdout, exitDidNotRun, strings.Join(parts, "; "))
	case len(failedNames) > 0:
		return verdict(stdout, exitFailed, strings.Join(parts, "; "))
	}
	return verdict(stdout, exitPassed, pass)
}

// verdict prints the verdict line and returns its exit code.
func verdict(stdout io.Writer, code int, text string) int {
	fmt.Fprintf(stdout, "%s: %s\n", verdictWord(code), text)
	return code
}

// verdictWord returns the word for an exit code.
func verdictWord(code int) string {
	switch code {
	case exitPassed:
		return "PASS"
	case exitFailed:
		return "FAIL"
	}
	return "DID NOT RUN"
}
