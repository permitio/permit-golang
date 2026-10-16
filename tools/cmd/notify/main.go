// Command notify posts a notice to a Slack channel through the channel's incoming webhook: one
// line, and optionally a report under it.
//
// Usage:
//
//	notify -channel security|ops -repo owner/name -check name -result text [-details text]
//	    [-run-url url] [-body file]
//
// The line is "<repo> <check>: <result>. <details>. <run URL>"; without -details or -run-url,
// that part and its ". " are left out. notify escapes &, < and > in the line's fields, once, as
// Slack's text format asks. -body names a file of Slack text that the program that wrote it,
// such as secreport or .github/scripts/digest.sh, escaped already; notify appends it under the
// line as it is. When the message is longer than Slack shows, notify cuts the body after a line
// and says so.
//
// -channel security posts to the webhook in SECURITY_SLACK_WEBHOOK_URL, and -channel ops to the
// one in SLACK_WEBHOOK_URL. When that variable is empty or not set, notify prints a ::warning::
// workflow command that says the notice was not posted, and exits 0. notify never prints the
// webhook, or any part of it, and does not follow a redirect.
//
// Exit codes:
//
//	0  the webhook answered with a 2xx status, or the webhook variable is not set
//	1  the notice was not delivered: the webhook answered with another status, or the request
//	   failed
//	2  notify did not run: the command line is wrong, the webhook is not an https URL, or the
//	   body cannot be read
//
// Build it before use, because go run reports every non-zero exit as 1:
//
//	go -C tools build -o ../bin/notify ./cmd/notify
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	exitPosted    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

const (
	// textLimit is the most bytes of text notify posts. Slack cuts a message's text after 40,000
	// characters, and a character is at least one byte.
	textLimit = 39000
	// timeout bounds the whole request.
	timeout = 30 * time.Second
	// replyLimit is the most bytes of the webhook's reply that notify prints.
	replyLimit = 200
)

// webhookVars names the variable that holds each channel's webhook.
var webhookVars = map[string]string{
	"security": "SECURITY_SLACK_WEBHOOK_URL",
	"ops":      "SLACK_WEBHOOK_URL",
}

var (
	// repoPattern matches a repository's owner/name.
	repoPattern = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9_.-]+$`)
	// runURLPattern matches the URL of a workflow run: https, without spaces or the characters
	// that end a link in Slack's text.
	runURLPattern = regexp.MustCompile(`^https://[^\s<>|]+$`)
	// slackEscaper escapes the characters that Slack's text format gives a meaning.
	slackEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
)

// system is what notify uses from the machine it runs on: its environment and the transport
// that carries the request.
type system struct {
	getenv    func(string) string
	transport http.RoundTripper
}

func main() {
	sys := system{
		getenv:    os.Getenv, //nolint:forbidigo // notify reads its webhook from the environment.
		transport: http.DefaultTransport,
	}
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, sys))
}

// options is the command line.
type options struct {
	channel, repo, check, result, details, runURL, body string
}

// run parses the command line, builds the message and posts it.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, sys system) int {
	opts, err := parseArgs(args, stderr)
	if err != nil {
		fmt.Fprintf(stdout, "notify: DID NOT RUN: %v\n", err)
		return exitDidNotRun
	}
	line := opts.line()
	text := line
	if opts.body != "" {
		body, readErr := os.ReadFile(opts.body)
		if readErr != nil {
			fmt.Fprintf(stdout, "notify: DID NOT RUN: reading the body: %v\n", readErr)
			return exitDidNotRun
		}
		text = fitText(line, string(body))
	}
	variable := webhookVars[opts.channel]
	webhook := sys.getenv(variable)
	if webhook == "" {
		fmt.Fprintf(stdout, "::warning title=Slack::%s\n", escapeData(variable+" is not set, "+
			"so this notice was not posted to the "+opts.channel+" channel: "+line))
		return exitPosted
	}
	target, err := url.Parse(webhook)
	if err != nil || target.Scheme != "https" || target.Host == "" {
		fmt.Fprintf(stdout, "notify: DID NOT RUN: %s is not an https URL\n", variable)
		return exitDidNotRun
	}
	status, reply, err := post(ctx, sys.transport, webhook, text)
	redact := redacter(webhook, target)
	switch {
	case err != nil:
		fmt.Fprintf(stdout, "notify: FAIL: posting to the %s channel failed: %s\n", opts.channel,
			redact.Replace(err.Error()))
		return exitFailed
	case status < 200 || status > 299:
		fmt.Fprintf(stdout, "notify: FAIL: the %s channel's webhook answered HTTP %d: %q\n",
			opts.channel, status, redact.Replace(reply))
		return exitFailed
	}
	fmt.Fprintf(stdout, "notify: posted to the %s channel (HTTP %d): %s\n", opts.channel, status,
		line)
	return exitPosted
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("notify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.channel, "channel", "", "the `channel` to post to: security or ops")
	flags.StringVar(&opts.repo, "repo", "", "the `owner/name` of the repository")
	flags.StringVar(&opts.check, "check", "", "the `name` of the check or job")
	flags.StringVar(&opts.result, "result", "", "the check's `result`, such as PASS")
	flags.StringVar(&opts.details, "details", "", "one line of `details`")
	flags.StringVar(&opts.runURL, "run-url", "", "the `URL` of the workflow run")
	flags.StringVar(&opts.body, "body", "", "a `file` of Slack text to post under the line")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	switch {
	case flags.NArg() > 0:
		return options{}, fmt.Errorf("unexpected arguments %q", flags.Args())
	case webhookVars[opts.channel] == "":
		return options{}, fmt.Errorf("-channel %q is not security or ops", opts.channel)
	case !repoPattern.MatchString(opts.repo):
		return options{}, fmt.Errorf("-repo %q is not owner/name", opts.repo)
	case opts.runURL != "" && !runURLPattern.MatchString(opts.runURL):
		return options{}, fmt.Errorf("-run-url %q is not an https URL", opts.runURL)
	}
	for _, field := range []struct{ name, value string }{
		{"-check", opts.check}, {"-result", opts.result}, {"-details", opts.details},
	} {
		if field.name != "-details" && strings.TrimSpace(field.value) == "" {
			return options{}, fmt.Errorf("%s is missing", field.name)
		}
		if strings.ContainsAny(field.value, "\r\n") || !utf8.ValidString(field.value) {
			return options{}, fmt.Errorf("%s must be one line of text", field.name)
		}
	}
	return opts, nil
}

// line returns the notice's line, with its fields escaped for Slack.
func (o options) line() string {
	parts := []string{slackEscaper.Replace(o.repo + " " + o.check + ": " + o.result)}
	if o.details != "" {
		parts = append(parts, slackEscaper.Replace(o.details))
	}
	if o.runURL != "" {
		parts = append(parts, o.runURL)
	}
	return strings.Join(parts, ". ")
}

// redacter returns a replacer that hides the webhook, its path and its query, the parts that hold
// its secret, in text that notify prints.
func redacter(webhook string, target *url.URL) *strings.Replacer {
	pairs := []string{webhook, "[webhook]"}
	for _, part := range []string{target.EscapedPath(), target.Path, target.RawQuery} {
		if strings.Trim(part, "/") != "" {
			pairs = append(pairs, part, "[webhook part]")
		}
	}
	return strings.NewReplacer(pairs...)
}

// fitText returns the line and the body under it, with the body cut after a line when the text
// would be longer than textLimit bytes.
func fitText(line, body string) string {
	body = strings.TrimRight(body, "\n")
	text := line + "\n" + body
	if len(text) <= textLimit {
		return text
	}
	const note = "\n... the rest is cut to fit Slack; the run has the full report"
	kept := body[:max(textLimit-len(line)-1-len(note), 0)]
	if i := strings.LastIndexByte(kept, '\n'); i >= 0 {
		kept = kept[:i]
	}
	for kept != "" && !utf8.ValidString(kept) {
		kept = kept[:len(kept)-1]
	}
	return line + "\n" + kept + note
}

// post sends the text to the webhook and returns the status and the start of the reply.
func post(ctx context.Context, transport http.RoundTripper, webhook, text string,
) (int, string, error) {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook,
		bytes.NewReader(payload))
	if err != nil {
		return 0, "", errors.New("the request could not be built")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		// A url.Error names the URL; what it wraps does not.
		if urlErr := (*url.Error)(nil); errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	// The status decides the result; a reply that cannot be read is printed as far as it was.
	reply, _ := io.ReadAll(io.LimitReader(resp.Body, replyLimit))
	return resp.StatusCode, string(reply), nil
}

// escapeData escapes the message of a workflow command.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}
