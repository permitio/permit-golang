// Command specsnap keeps the snapshots of the API specs that the SDK is checked against, in
// tools/specs: the control plane's (control-plane.json), the cloud PDP's (pdp-cloud.json) and the
// container PDP's (pdp-container.json), with lock.json, which records where each came from.
//
// Usage:
//
//	specsnap [-dir dir] -refresh name [-from file] [-image ref] [-fetched date] [-note text]
//	specsnap [-dir dir] -verify
//	specsnap [-dir dir] -check-live [-out dir]
//	specsnap [-dir dir] -check-pdp url [-image ref]
//
// -dir names the snapshot directory, tools/specs by default, relative to the directory specsnap
// runs in.
//
// -refresh writes the snapshot of one spec: control-plane, pdp-cloud or pdp-container. It reads
// the spec from the URL where it is published, or from -from, a file that holds it. It then
// applies the pre-fix, writes the spec in canonical form to <name>.json and records it in
// lock.json: the URL, or for pdp-container the image, the day the spec was fetched (-fetched,
// today in UTC by default), the sha256 of the spec as fetched and of the snapshot, and its
// number of operations. The container PDP publishes its spec only from a running container, at
// GET /openapi.json, so pdp-container needs -from, the file saved from there, and -image, the
// container's image as permitio/pdp-v2:<tag>@sha256:<digest>. -note adds a line of text to the
// spec's entry, such as who took the spec. It writes the snapshot and lock.json to .tmp files
// beside them first, and replaces the two only once both are written.
//
// -verify checks that every snapshot matches lock.json: its sha256 and number of operations, and
// that it is in canonical form.
//
// -check-live fetches the control plane's and the cloud PDP's specs where they are published,
// pre-fixes and canonicalizes them, and compares each with its snapshot's sha256 in lock.json.
// It also reads Docker Hub's newest 100 tags of permitio/pdp-v2 and compares the digest of
// latest with the image that pdp-container.json was taken from. -out names a directory to write
// the snapshots of the live specs to, for a pull request that refreshes them.
//
// -check-pdp fetches the spec that a running container PDP serves, at GET <url>/openapi.json,
// such as http://127.0.0.1:7766/openapi.json, makes its snapshot as -refresh does, and compares
// it with pdp-container.json byte for byte. It fails on any difference, and says where the two
// first differ. -image names the container's image, as permitio/pdp-v2:<tag>@sha256:<digest>,
// and fails the check when it is not the image that lock.json records for pdp-container.json.
//
// The canonical form is the spec without its prose and examples: every description, summary,
// title, example and examples keyword is left out, wherever it is a keyword and not a name, such
// as a property called title. The values of default, enum and const are kept as they are, and
// so is the value of an x- extension where its key is a keyword. In an object keyed by names,
// such as paths or properties, an x- key is a name, and its value loses its prose like any
// other. Numbers are kept as written. The object keys are sorted, the indent is two spaces,
// lines end in LF and the file ends in a newline, so a spec gives the same snapshot every time,
// and a change in its prose alone does not change it. OpenAPI requires a description on each
// response and a title in info, which the canonical form leaves out too.
//
// The pre-fix patches the three constructs of the published control-plane spec that are not
// valid OpenAPI 3.1, so that a tool that compiles the spec's schemas accepts the snapshot:
//
//   - the anyOf branches of the secret of ProxyConfigCreate, ProxyConfigRead and
//     ProxyConfigUpdate have the types HeadersAuth, BasicAuth and BearerAuth, which are not JSON
//     Schema types. HeadersAuth becomes an object of strings, and the other two strings.
//   - OPALUpdateCallback's callbacks use a draft-04 tuple, an items array, which becomes
//     prefixItems.
//   - the tag Bulk Operations is declared twice; the second is left out.
//
// Each patch first checks that its defect is still there, in exactly the places it fixes. When
// it is gone, specsnap fails with "fix no longer needed", and the patch can be removed; when it
// is in other places, it fails with "the defect changed".
//
// A spec with fewer operations than a complete one has is cut short: at least 250 for the
// control plane, 30 for the container PDP and 8 for the cloud PDP.
//
// The last line specsnap prints is its verdict: PASS, FAIL or DID NOT RUN, then a colon and what
// it found. The exit codes:
//
//	0  -refresh wrote the snapshot, every snapshot matches lock.json, no live spec or image
//	   differs from its snapshot, or the container PDP serves the spec of its snapshot
//	1  a snapshot does not match lock.json, a live spec or Docker Hub's latest permitio/pdp-v2
//	   differs from its snapshot, the spec or the image of the container PDP that -check-pdp
//	   asks is not the snapshot's, or a pre-fix patch is no longer needed or its defect changed
//	2  specsnap did not run: the command line is wrong; a spec, lock.json or Docker Hub's answer
//	   cannot be fetched, read, parsed or written; a spec has fewer operations than a complete
//	   one; lock.json has no entry for a spec or one for a spec specsnap does not know; or
//	   Docker Hub's newest tags do not include latest
//
// Build it before use, because go run reports every non-zero exit as 1:
//
//	go -C tools build -o ../bin/specsnap ./cmd/specsnap
//	bin/specsnap -verify
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

const (
	// pdpRepository is the Docker Hub repository of the container PDP.
	pdpRepository = "permitio/pdp-v2"
	// hubTagsURL lists the newest 100 tags of the container PDP on Docker Hub.
	hubTagsURL = "https://hub.docker.com/v2/repositories/" + pdpRepository +
		"/tags?page_size=100&ordering=last_updated"
	// maxBody is the most bytes specsnap reads from a URL: about 20 times the control plane's spec.
	maxBody = 16 << 20
	// timeout bounds each request.
	timeout = time.Minute
)

// source is a spec that specsnap keeps.
type source struct {
	// name is the spec's name on the command line and in lock.json; its snapshot is <name>.json.
	name string
	// url is where the spec is published. The container PDP's spec has none: it is read from a
	// running container.
	url string
	// minOperations is the fewest operations a complete spec has.
	minOperations int
	// patches is the spec's pre-fix.
	patches []patch
}

// file returns the name of the source's snapshot.
func (s source) file() string {
	return s.name + ".json"
}

// sources are the specs specsnap keeps, in the order it reports them.
var sources = []source{
	{
		name:          "control-plane",
		url:           "https://api.permit.io/v2/openapi.json",
		minOperations: 250,
		patches:       controlPlanePatches,
	},
	{name: "pdp-cloud", url: "https://cloudpdp.api.permit.io/openapi.json", minOperations: 8},
	{name: "pdp-container", minOperations: 30},
}

// system is what specsnap uses from outside: the specs it keeps, Docker Hub, the network and the
// clock.
type system struct {
	sources []source
	hubURL  string
	client  *http.Client
	maxBody int64
	now     func() time.Time
}

func main() {
	sys := system{
		sources: sources,
		hubURL:  hubTagsURL,
		client:  &http.Client{Timeout: timeout},
		maxBody: maxBody,
		now:     time.Now,
	}
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, sys))
}

// options is the command line.
type options struct {
	dir, refresh, from, image, fetched, note, out, checkPDP string
	verify, checkLive                                       bool
}

// modeFlags lists the flags each mode takes.
var modeFlags = map[string][]string{
	"refresh":    {"dir", "refresh", "from", "image", "fetched", "note"},
	"verify":     {"dir", "verify"},
	"check-live": {"dir", "check-live", "out"},
	"check-pdp":  {"dir", "check-pdp", "image"},
}

// imagePattern matches the container PDP's image with its tag and digest.
var imagePattern = regexp.MustCompile(`^` + regexp.QuoteMeta(pdpRepository) +
	`:[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}@sha256:[0-9a-f]{64}$`)

// failure is an error that means a check failed, exit 1, rather than that specsnap did not run.
type failure struct {
	error
}

// exitCode returns the exit code for an error: 1 for a failure, 2 for any other error.
func exitCode(err error) int {
	if _, ok := errors.AsType[failure](err); ok {
		return exitFailed
	}
	return exitDidNotRun
}

// run parses the command line and runs its mode.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, sys system) int {
	opts, err := parseArgs(args, stderr, sys)
	if err != nil {
		fmt.Fprintf(stdout, "DID NOT RUN: %v\n", err)
		return exitDidNotRun
	}
	switch {
	case opts.verify:
		return verify(opts.dir, stdout, sys)
	case opts.checkLive:
		return checkLive(ctx, opts, stdout, sys)
	case opts.checkPDP != "":
		return checkPDP(ctx, opts, stdout, sys)
	}
	return refresh(ctx, opts, stdout, sys)
}

func parseArgs(args []string, stderr io.Writer, sys system) (options, error) {
	var opts options
	flags := flag.NewFlagSet("specsnap", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.dir, "dir", "tools/specs", "the snapshot `directory`")
	flags.StringVar(&opts.refresh, "refresh", "", "the `name` of the spec to refresh")
	flags.BoolVar(&opts.verify, "verify", false, "check the snapshots against lock.json")
	flags.BoolVar(&opts.checkLive, "check-live", false,
		"compare the live specs and the PDP image with the snapshots")
	flags.StringVar(&opts.from, "from", "", "read the spec from this `file`")
	flags.StringVar(&opts.image, "image", "",
		"the container PDP's image, as permitio/pdp-v2:`tag@sha256:digest`")
	flags.StringVar(&opts.fetched, "fetched", "", "the `day` the spec was fetched, as YYYY-MM-DD")
	flags.StringVar(&opts.note, "note", "", "a line of `text` for the spec's entry in lock.json")
	flags.StringVar(&opts.out, "out", "", "a `directory` to write the live specs' snapshots to")
	flags.StringVar(&opts.checkPDP, "check-pdp", "",
		"compare the spec that the container PDP at this `url` serves with its snapshot")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments %q", flags.Args())
	}
	var modes []string
	if opts.refresh != "" {
		modes = append(modes, "refresh")
	}
	if opts.verify {
		modes = append(modes, "verify")
	}
	if opts.checkLive {
		modes = append(modes, "check-live")
	}
	if opts.checkPDP != "" {
		modes = append(modes, "check-pdp")
	}
	if len(modes) != 1 {
		return options{}, errors.New("give exactly one of -refresh, -verify, -check-live and " +
			"-check-pdp")
	}
	var extra []string
	flags.Visit(func(f *flag.Flag) {
		if !slices.Contains(modeFlags[modes[0]], f.Name) {
			extra = append(extra, "-"+f.Name)
		}
	})
	if len(extra) > 0 {
		return options{}, fmt.Errorf("-%s does not take %s", modes[0], strings.Join(extra, ", "))
	}
	if opts.dir == "" {
		return options{}, errors.New("-dir is empty")
	}
	switch {
	case opts.refresh != "":
		return opts, checkRefreshArgs(opts, sys)
	case opts.checkPDP != "":
		return opts, checkPDPArgs(opts)
	}
	return opts, nil
}

// checkRefreshArgs checks the flags that -refresh takes.
func checkRefreshArgs(opts options, sys system) error {
	src, ok := findSource(sys.sources, opts.refresh)
	switch {
	case !ok:
		return fmt.Errorf("-refresh %q is not one of %s", opts.refresh, sourceNames(sys.sources))
	case src.url == "" && (opts.from == "" || opts.image == ""):
		return fmt.Errorf("-refresh %s needs -from, the spec saved from the container's "+
			"GET /openapi.json, and -image, the container's image", src.name)
	case src.url == "" && !imagePattern.MatchString(opts.image):
		return fmt.Errorf("-image %q is not %s:<tag>@sha256:<64 hex digits>", opts.image,
			pdpRepository)
	case src.url != "" && opts.image != "":
		return fmt.Errorf("-image is only for the container PDP's spec, not %s", src.name)
	case strings.ContainsAny(opts.note, "\r\n"):
		return errors.New("-note must be one line of text")
	}
	if opts.fetched != "" {
		if _, err := time.Parse(time.DateOnly, opts.fetched); err != nil {
			return fmt.Errorf("-fetched %q is not a day as YYYY-MM-DD", opts.fetched)
		}
	}
	return nil
}

// findSource returns the source with the name.
func findSource(list []source, name string) (source, bool) {
	for _, src := range list {
		if src.name == name {
			return src, true
		}
	}
	return source{}, false
}

// sourceNames returns the names of the sources, for a message.
func sourceNames(list []source) string {
	names := make([]string, 0, len(list))
	for _, src := range list {
		names = append(names, src.name)
	}
	return strings.Join(names, ", ")
}
