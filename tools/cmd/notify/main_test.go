package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	runURL = "https://github.com/octo/repo/actions/runs/7"
	// hookPath is the secret part of a webhook URL, which notify must never print.
	hookPath = "/services/T0FAKE/B0FAKE/FAKEsecretPATH"
)

// webhook is an httptest server that records each request it gets and answers with status.
type webhook struct {
	server *httptest.Server
	mu     sync.Mutex
	posts  []request
}

// request is what the webhook got.
type request struct {
	method, path, contentType string
	text                      string
}

func newWebhook(t *testing.T, status int) *webhook {
	t.Helper()
	w := &webhook{}
	w.server = httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter,
		r *http.Request,
	) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Errorf("the payload is not JSON with a text: %v: %s", err, data)
		}
		w.mu.Lock()
		w.posts = append(w.posts, request{
			method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type"),
			text: payload.Text,
		})
		w.mu.Unlock()
		if status == http.StatusFound {
			http.Redirect(rw, r, w.server.URL+hookPath+"/again", http.StatusFound)
			return
		}
		rw.WriteHeader(status)
		_, _ = io.WriteString(rw, "no_service for "+r.URL.Path)
	}))
	t.Cleanup(w.server.Close)
	return w
}

// requests returns what the webhook got so far.
func (w *webhook) requests() []request {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]request(nil), w.posts...)
}

// sys returns a system whose environment holds env and whose transport trusts the webhook.
func (w *webhook) sys(env map[string]string) system {
	return system{
		getenv:    func(name string) string { return env[name] },
		transport: w.server.Client().Transport,
	}
}

func runNotify(t *testing.T, sys system, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), args, &stdout, &stderr, sys)
	t.Logf("exit %d, stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	return code, stdout.String()
}

// noticeArgs returns the command line of a notice to channel, with extra flags after it.
func noticeArgs(channel string, extra ...string) []string {
	return append([]string{
		"-channel", channel, "-repo", "octo/repo", "-check", "vuln", "-result", "FAIL",
		"-run-url", runURL,
	}, extra...)
}

// checkSecretHidden fails the test when stdout holds the webhook or its secret path.
func checkSecretHidden(t *testing.T, stdout, hook string) {
	t.Helper()
	for _, secret := range []string{hook, hookPath, strings.TrimPrefix(hookPath, "/")} {
		if strings.Contains(stdout, secret) {
			t.Errorf("stdout holds %q:\n%s", secret, stdout)
		}
	}
	if strings.Contains(stdout, "FAKEsecretPATH") {
		t.Errorf("stdout holds a part of the webhook's path:\n%s", stdout)
	}
}

func TestPost(t *testing.T) {
	tests := []struct {
		name    string
		channel string
		// variable is where the webhook is; other is set to a closed server, so a post to the
		// wrong channel fails.
		variable, other string
		extra           []string
		wantText        string
		wantStdout      string
	}{
		{
			name: "the security channel", channel: "security",
			variable: "SECURITY_SLACK_WEBHOOK_URL", other: "SLACK_WEBHOOK_URL",
			extra:    []string{"-details", "2 findings"},
			wantText: "octo/repo vuln: FAIL. 2 findings. " + runURL,
			wantStdout: "notify: posted to the security channel (HTTP 200): octo/repo vuln: " +
				"FAIL. 2 findings. " + runURL + "\n",
		},
		{
			name: "the ops channel, without details", channel: "ops",
			variable: "SLACK_WEBHOOK_URL", other: "SECURITY_SLACK_WEBHOOK_URL",
			wantText: "octo/repo vuln: FAIL. " + runURL,
		},
		{
			name: "fields escaped once", channel: "ops",
			variable: "SLACK_WEBHOOK_URL", other: "SECURITY_SLACK_WEBHOOK_URL",
			extra:    []string{"-details", "a <b> & c &amp; d"},
			wantText: "octo/repo vuln: FAIL. a &lt;b&gt; &amp; c &amp;amp; d. " + runURL,
		},
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := newWebhook(t, http.StatusOK)
			sys := hook.sys(map[string]string{
				tt.variable: hook.server.URL + hookPath, tt.other: closed.URL + hookPath,
			})

			code, stdout := runNotify(t, sys, noticeArgs(tt.channel, tt.extra...)...)

			if code != exitPosted {
				t.Errorf("exit = %d, want %d", code, exitPosted)
			}
			posts := hook.requests()
			if len(posts) != 1 {
				t.Fatalf("the webhook got %d requests, want 1", len(posts))
			}
			got := posts[0]
			if got.method != http.MethodPost || got.path != hookPath ||
				got.contentType != "application/json" || got.text != tt.wantText {
				t.Errorf("the webhook got %s %s (%s) with the text:\n%s\nwant POST %s "+
					"(application/json) with:\n%s", got.method, got.path, got.contentType,
					got.text, hookPath, tt.wantText)
			}
			if tt.wantStdout != "" && stdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tt.wantStdout)
			}
			checkSecretHidden(t, stdout, hook.server.URL+hookPath)
		})
	}
}

func TestPostWithABody(t *testing.T) {
	line := "octo/repo security-scan: PASS. " + runURL
	tests := []struct {
		name, body string
		wantText   string
	}{
		{
			name:     "a body that fits, escaped by its writer",
			body:     "*Clean* &lt;none&gt;\n• a\n\n",
			wantText: line + "\n*Clean* &lt;none&gt;\n• a",
		},
		{
			name:     "an empty body",
			wantText: line + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := newWebhook(t, http.StatusOK)
			body := filepath.Join(t.TempDir(), "body")
			if err := os.WriteFile(body, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			sys := hook.sys(map[string]string{"SECURITY_SLACK_WEBHOOK_URL": hook.server.URL})

			code, _ := runNotify(t, sys, "-channel", "security", "-repo", "octo/repo", "-check",
				"security-scan", "-result", "PASS", "-run-url", runURL, "-body", body)

			posts := hook.requests()
			if code != exitPosted || len(posts) != 1 || posts[0].text != tt.wantText {
				t.Errorf("exit %d, posts %d, want exit 0 and one post with:\n%s\ngot:\n%v",
					code, len(posts), tt.wantText, posts)
			}
		})
	}
}

func TestFitText(t *testing.T) {
	line := "octo/repo digest: sent"
	short := "a\nb"
	if got := fitText(line, short+"\n"); got != line+"\n"+short {
		t.Errorf("fitText(short) = %q", got)
	}
	exact := strings.Repeat("x", textLimit-len(line)-1)
	if got := fitText(line, exact); got != line+"\n"+exact {
		t.Errorf("a body that fits exactly was cut to %d bytes", len(got))
	}

	long := strings.Repeat("• a finding\n", textLimit/10)
	got := fitText(line, long)
	if len(got) > textLimit {
		t.Errorf("fitText returned %d bytes, more than %d", len(got), textLimit)
	}
	if !strings.HasPrefix(got, line+"\n• a finding\n") ||
		!strings.HasSuffix(got, "• a finding\n... the rest is cut to fit Slack; the run has "+
			"the full report") {
		t.Errorf("fitText did not cut the body after a line and say so: ...%q",
			got[max(len(got)-120, 0):])
	}
	if len(got) < textLimit-len("• a finding\n")-100 {
		t.Errorf("fitText kept %d bytes, much less than the limit %d", len(got), textLimit)
	}

	// A body whose only newline is its first byte keeps nothing of the line after it.
	got = fitText(line, "\n"+strings.Repeat("x", textLimit))
	if got != line+"\n\n... the rest is cut to fit Slack; the run has the full report" {
		t.Errorf("fitText kept part of a line it had to cut: %d bytes", len(got))
	}

	multibyte := strings.Repeat("é", textLimit)
	got = fitText(line, multibyte)
	if len(got) > textLimit || !strings.HasPrefix(got, line+"\né") ||
		!strings.Contains(got, "é\n... the rest is cut") {
		t.Errorf("fitText cut a body without newlines badly: %d bytes, ends %q", len(got),
			got[max(len(got)-80, 0):])
	}
}

func TestNotDelivered(t *testing.T) {
	tests := []struct {
		name   string
		status int
		closed bool
		want   string
	}{
		{
			name: "a 500", status: http.StatusInternalServerError,
			want: "notify: FAIL: the security channel's webhook answered HTTP 500: " +
				"\"no_service for [webhook part]\"\n",
		},
		{
			name: "a 404", status: http.StatusNotFound,
			want: "notify: FAIL: the security channel's webhook answered HTTP 404: ",
		},
		{
			name: "a redirect, not followed", status: http.StatusFound,
			want: "notify: FAIL: the security channel's webhook answered HTTP 302: ",
		},
		{
			name: "a webhook that refuses the connection", closed: true,
			want: "notify: FAIL: posting to the security channel failed: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := newWebhook(t, tt.status)
			url := hook.server.URL + hookPath
			if tt.closed {
				hook.server.Close()
			}
			sys := hook.sys(map[string]string{"SECURITY_SLACK_WEBHOOK_URL": url})

			code, stdout := runNotify(t, sys, noticeArgs("security")...)

			if code != exitFailed {
				t.Errorf("exit = %d, want %d", code, exitFailed)
			}
			if !strings.HasPrefix(stdout, tt.want) {
				t.Errorf("stdout = %q, want it to start with %q", stdout, tt.want)
			}
			if posts := hook.requests(); tt.status == http.StatusFound && len(posts) != 1 {
				t.Errorf("the webhook got %d requests, want 1: the redirect is not followed",
					len(posts))
			}
			checkSecretHidden(t, stdout, url)
		})
	}
}

func TestRedacter(t *testing.T) {
	hook := "https://hooks.example.com/services/T/B/hookSecret?token=querySecret"
	target, err := url.Parse(hook)
	if err != nil {
		t.Fatal(err)
	}

	got := redacter(hook, target).Replace("Post \"" + hook + "\": /services/T/B/hookSecret and " +
		"token=querySecret")

	want := "Post \"[webhook]\": [webhook part] and [webhook part]"
	if got != want {
		t.Errorf("redacted: %q, want %q", got, want)
	}

	root, err := url.Parse("https://hooks.example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if got := redacter("https://hooks.example.com/", root).Replace("a/b c"); got != "a/b c" {
		t.Errorf("a webhook without a path changed %q to %q", "a/b c", got)
	}
}

func TestMissingWebhook(t *testing.T) {
	for _, channel := range []string{"security", "ops"} {
		t.Run(channel, func(t *testing.T) {
			hook := newWebhook(t, http.StatusOK)
			other := map[string]string{
				"security": "SLACK_WEBHOOK_URL", "ops": "SECURITY_SLACK_WEBHOOK_URL",
			}[channel]
			sys := hook.sys(map[string]string{other: hook.server.URL + hookPath})

			code, _ := runNotify(t, sys, noticeArgs(channel, "-details", "50%\nx")...)

			if code != exitDidNotRun {
				t.Fatalf("a two-line -details: exit = %d, want %d", code, exitDidNotRun)
			}
			code, stdout := runNotify(t, sys, noticeArgs(channel, "-details", "50% done")...)

			variable := webhookVars[channel]
			want := "::warning title=Slack::" + variable + " is not set, so this notice was " +
				"not posted to the " + channel + " channel: octo/repo vuln: FAIL. 50%25 done. " +
				runURL + "\n"
			if code != exitPosted || stdout != want {
				t.Errorf("exit %d, stdout %q; want exit 0, stdout %q", code, stdout, want)
			}
			if posts := hook.requests(); len(posts) != 0 {
				t.Errorf("the other channel's webhook got %d requests, want none", len(posts))
			}
		})
	}
}

func TestBadArgs(t *testing.T) {
	missingBody := filepath.Join(t.TempDir(), "missing")
	tests := []struct {
		name string
		args []string
		// webhook is SECURITY_SLACK_WEBHOOK_URL; a valid one when empty.
		webhook string
		want    string
	}{
		{name: "no arguments", want: "-channel \"\" is not security or ops"},
		{
			name: "an unknown channel", args: noticeArgs("alerts"),
			want: "-channel \"alerts\" is not security or ops",
		},
		{
			name: "an argument", args: append(noticeArgs("security"), "extra"),
			want: "unexpected arguments [\"extra\"]",
		},
		{
			name: "a repository that is not owner/name",
			args: []string{"-channel", "ops", "-repo", "octo", "-check", "c", "-result", "r"},
			want: "-repo \"octo\" is not owner/name",
		},
		{
			name: "no check",
			args: []string{"-channel", "ops", "-repo", "octo/repo", "-result", "r"},
			want: "-check is missing",
		},
		{
			name: "a blank result",
			args: []string{"-channel", "ops", "-repo", "octo/repo", "-check", "c", "-result", " "},
			want: "-result is missing",
		},
		{
			name: "a check of two lines",
			args: []string{"-channel", "ops", "-repo", "o/r", "-check", "c\rd", "-result", "r"},
			want: "-check must be one line of text",
		},
		{
			name: "a result that is not UTF-8",
			args: []string{"-channel", "ops", "-repo", "o/r", "-check", "c", "-result", "\xff"},
			want: "-result must be one line of text",
		},
		{
			name: "an http run URL",
			args: noticeArgs("security", "-run-url", "http://github.com/octo/repo/actions/runs/7"),
			want: "-run-url \"http://github.com/octo/repo/actions/runs/7\" is not an https URL",
		},
		{
			name: "a run URL that would end a Slack link",
			args: noticeArgs("security", "-run-url", runURL+"|x"),
			want: "is not an https URL",
		},
		{
			name: "a body that cannot be read", args: noticeArgs("security", "-body", missingBody),
			want: "reading the body: open " + missingBody,
		},
		{
			name: "an http webhook", args: noticeArgs("security"),
			webhook: "http://hooks.example.com" + hookPath,
			want:    "SECURITY_SLACK_WEBHOOK_URL is not an https URL",
		},
		{
			name: "a webhook that is not a URL", args: noticeArgs("security"),
			webhook: "https://hooks example.com" + hookPath + "%zz",
			want:    "SECURITY_SLACK_WEBHOOK_URL is not an https URL",
		},
		{
			name: "a webhook without a host", args: noticeArgs("security"),
			webhook: "https://" + hookPath,
			want:    "SECURITY_SLACK_WEBHOOK_URL is not an https URL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := newWebhook(t, http.StatusOK)
			webhook := tt.webhook
			if webhook == "" {
				webhook = hook.server.URL + hookPath
			}
			sys := hook.sys(map[string]string{"SECURITY_SLACK_WEBHOOK_URL": webhook})

			code, stdout := runNotify(t, sys, tt.args...)

			if code != exitDidNotRun || !strings.HasPrefix(stdout, "notify: DID NOT RUN: ") ||
				!strings.Contains(stdout, tt.want) {
				t.Errorf("exit %d, stdout %q; want exit 2 and DID NOT RUN: ...%s", code, stdout,
					tt.want)
			}
			if posts := hook.requests(); len(posts) != 0 {
				t.Errorf("the webhook got %d requests, want none", len(posts))
			}
			checkSecretHidden(t, stdout, webhook)
		})
	}
}
