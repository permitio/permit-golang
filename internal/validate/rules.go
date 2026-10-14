// Package validate holds the rules that the SDK checks a configuration or an argument against
// before it sends a request. Each rule has an ID that never changes and the message of the error
// it returns. testdata/validation-rules.txt lists the rules; TestValidationRules
// (validation_rules_test.go) keeps it in step with this package, and apisurface's diff mode
// (tools/cmd/apisurface) counts an ID that is new there as a breaking change, because a new rule
// rejects input that the SDK accepted before.
package validate

import "slices"

// Rule is one check of a configuration or an argument.
type Rule struct {
	// ID names the rule: lower-case words, each joined to the next by a dot, such as
	// config.api-key.missing. It never changes, so a new ID is a new rule.
	ID string

	// Message is the message of the error the rule returns, on one line.
	Message string
}

// The rules of a client's configuration, which permit.New checks. The error of a rule that an
// option or an environment variable breaks names the option or the variable before the message.
var (
	// APIKeyEmpty rejects an API key that an option gives as the empty string.
	APIKeyEmpty = Rule{
		ID: "config.api-key.empty",
		Message: "the API key is empty; pass a Permit API key, or leave the option out to " +
			"read PERMIT_API_KEY",
	}

	// APIKeyMissing rejects a configuration with no API key, from an option or from the
	// environment.
	APIKeyMissing = Rule{
		ID:      "config.api-key.missing",
		Message: "no API key; pass WithAPIKey, or set PERMIT_API_KEY",
	}

	// HTTPClientNil rejects a nil *http.Client.
	HTTPClientNil = Rule{
		ID: "config.http-client.nil",
		Message: "the *http.Client is nil; pass a client, or leave the option out to use one " +
			"of the SDK's own",
	}

	// OptionZero rejects an Option that no With function built, such as the zero Option.
	OptionZero = Rule{
		ID:      "config.option.zero",
		Message: "the Option is the zero value; build options with the With functions",
	}

	// RetryMaxBackoffNegative rejects a RetryPolicy whose MaxBackoff is less than zero.
	RetryMaxBackoffNegative = Rule{
		ID: "config.retry.max-backoff.negative",
		Message: "MaxBackoff is less than zero; give 0 or more, where 0 is the default, " +
			"5 seconds",
	}

	// RetryMaxRetriesNegative rejects a RetryPolicy whose MaxRetries is less than zero.
	RetryMaxRetriesNegative = Rule{
		ID:      "config.retry.max-retries.negative",
		Message: "MaxRetries is less than zero; give 0 or more, where 0 sends each request once",
	}

	// TimeoutNotPositive rejects a timeout of zero or less.
	TimeoutNotPositive = Rule{
		ID:      "config.timeout.not-positive",
		Message: "the timeout must be more than zero, such as 30 * time.Second",
	}

	// URLEmpty rejects a URL that an option gives as the empty string.
	URLEmpty = Rule{
		ID: "config.url.empty",
		Message: "the URL is empty; leave the option out to read the environment variable or " +
			"to use the default",
	}

	// URLFragment rejects a URL with a fragment, the part from a #.
	URLFragment = Rule{
		ID:      "config.url.fragment",
		Message: "the URL has a fragment (#...); give the URL without it",
	}

	// URLHost rejects a URL without a host, such as http:///v2 or http:localhost.
	URLHost = Rule{
		ID:      "config.url.host",
		Message: "the URL has no host; give one, such as api.permit.io in https://api.permit.io",
	}

	// URLParse rejects a URL that net/url cannot parse.
	URLParse = Rule{
		ID: "config.url.parse",
		Message: "the URL does not parse; check that it starts with http:// or https://, that " +
			"its port is a number, and that it has no spaces and no % without two hex digits " +
			"after it",
	}

	// URLQuery rejects a URL with a query, the part from a ?.
	URLQuery = Rule{
		ID:      "config.url.query",
		Message: "the URL has a query (?...); give the URL without it",
	}

	// URLScheme rejects a URL whose scheme is not http or https, or that has none.
	URLScheme = Rule{
		ID:      "config.url.scheme",
		Message: "the URL must start with http:// or https://",
	}

	// URLUserinfo rejects a URL with a user name or a password before its host.
	URLUserinfo = Rule{
		ID: "config.url.userinfo",
		Message: "the URL has a user name or a password; give the API key with WithAPIKey or " +
			"PERMIT_API_KEY instead",
	}
)

// The rules of the arguments of a call, which the SDK checks before it sends the request. The
// error of such a rule names the argument before the message.
var (
	// PathSegmentDot rejects a path parameter that is . or .., which a server or a proxy on the
	// way may resolve as a step in the path, so that the request reaches another route.
	PathSegmentDot = Rule{
		ID: "path.segment.dot",
		Message: "the path parameter is . or .., which would change the route; pass the key or " +
			"ID of the object",
	}

	// PathSegmentEmpty rejects an empty path parameter, which would leave an empty segment in
	// the path, so that the request reaches another route or none.
	PathSegmentEmpty = Rule{
		ID:      "path.segment.empty",
		Message: "the path parameter is empty; pass the key or ID of the object",
	}
)

// rules lists every rule, sorted by ID.
var rules = []Rule{
	APIKeyEmpty,
	APIKeyMissing,
	HTTPClientNil,
	OptionZero,
	RetryMaxBackoffNegative,
	RetryMaxRetriesNegative,
	TimeoutNotPositive,
	URLEmpty,
	URLFragment,
	URLHost,
	URLParse,
	URLQuery,
	URLScheme,
	URLUserinfo,
	PathSegmentDot,
	PathSegmentEmpty,
}

// Rules returns every rule, sorted by ID.
func Rules() []Rule {
	return slices.Clone(rules)
}
