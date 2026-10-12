//go:build integration

// Command cleanup deletes what one run of the integration tests in pkg/tests created: every
// tenant, user, role, resource, resource instance, condition set and proxy config whose key is
// the run prefix, a hyphen and more characters. The tests delete what they create; CI also runs
// this command after them, including when they fail or are cancelled, for what a test could not
// delete.
//
//	go run -tags integration ./pkg/tests/cleanup
//
// It reads the run prefix from PERMIT_TEST_RUN_PREFIX, and PROJECT, ENV, PDP_API_KEY, PDP_URL
// and API_URL from the environment, like the tests. It exits with status 1 when it could not
// delete an object, and 2 when a variable is missing or the prefix is not valid.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/permitio/permit-golang/internal/testcleanup"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/permit"
	"go.uber.org/zap"
)

// timeout bounds the whole cleanup, so that a hung API cannot hold the CI job. It is shorter
// than the timeout of the CI step that runs it.
const timeout = 7 * time.Minute

func main() {
	os.Exit(run())
}

func run() int {
	names := []string{testcleanup.PrefixEnvVar, "PROJECT", "ENV", "PDP_API_KEY", "PDP_URL",
		"API_URL"}
	values := map[string]string{}
	var missing []string
	for _, name := range names {
		values[name] = os.Getenv(name)
		if values[name] == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "cleanup: set %s\n", strings.Join(missing, ", "))
		return 2
	}
	prefix := values[testcleanup.PrefixEnvVar]
	if err := testcleanup.CheckPrefix(prefix); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup: %s: %v\n", testcleanup.PrefixEnvVar, err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// The SDK's own error log lines would repeat what Run reports.
	client := permit.New(config.NewConfigBuilder(values["PDP_API_KEY"]).
		WithPdpUrl(values["PDP_URL"]).
		WithApiUrl(values["API_URL"]).
		WithContext(config.NewPermitContext(config.EnvironmentAPIKeyLevel,
			values["PROJECT"], values["ENV"])).
		WithLogger(zap.NewNop()).
		Build())
	if err := testcleanup.Run(ctx, client.Api, prefix, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup: %v\n", err)
		return 1
	}
	return 0
}
