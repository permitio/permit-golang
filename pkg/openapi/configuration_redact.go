// This file is written by hand, not generated: keep it when the client is generated again.

package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/permitio/permit-golang/internal/redact"
)

// redactedConfiguration has the fields of Configuration and none of its methods, so fmt prints it
// field by field and encoding/json encodes it field by field.
type redactedConfiguration Configuration

func (c Configuration) redacted() redactedConfiguration {
	redacted := redactedConfiguration(c)
	redacted.DefaultHeader = redact.Headers(c.DefaultHeader)
	return redacted
}

// Format prints c as fmt prints a struct, with every verb, but shows the value of each header in
// DefaultHeader that carries a credential, such as Authorization, as [REDACTED]. It does not
// change DefaultHeader. A struct that embeds Configuration gets this method, String and GoString
// too, so it prints as its Configuration alone.
func (c Configuration) Format(f fmt.State, verb rune) {
	printed := fmt.Sprintf(redact.FormatDirective(f, verb), c.redacted())
	if verb == 'v' && f.Flag('#') {
		printed = "openapi.Configuration" +
			strings.TrimPrefix(printed, "openapi.redactedConfiguration")
	}
	_, _ = io.WriteString(f, printed)
}

// String returns c as fmt.Sprint prints it, with credential header values shown as [REDACTED].
func (c Configuration) String() string {
	return fmt.Sprint(c)
}

// GoString returns c as the %#v verb prints it, with credential header values shown as
// [REDACTED].
func (c Configuration) GoString() string {
	return fmt.Sprintf("%#v", c)
}

// MarshalJSON encodes c as encoding/json encodes its fields, with the value of each header in
// DefaultHeader that carries a credential shown as [REDACTED]. A struct that embeds Configuration
// gets this method too, so it encodes as its Configuration alone.
func (c Configuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.redacted())
}

// printedAPIClient has the fields of APIClient and none of its methods, so fmt prints it field by
// field.
type printedAPIClient APIClient

// Format prints c as fmt prints a struct, with every verb, but with the credential header values
// of its configuration shown as [REDACTED]. Without it, a verb that does not apply to pointers,
// such as %s, prints the configuration that c points to in full. A struct that embeds APIClient
// or *APIClient gets this method too, so it prints as its APIClient alone.
func (c APIClient) Format(f fmt.State, verb rune) {
	printed := printedAPIClient(c)
	if c.cfg != nil {
		cfg := *c.cfg
		cfg.DefaultHeader = redact.Headers(c.cfg.DefaultHeader)
		printed.cfg = &cfg
	}
	out := fmt.Sprintf(redact.FormatDirective(f, verb), printed)
	if verb == 'v' && f.Flag('#') {
		out = "openapi.APIClient" + strings.TrimPrefix(out, "openapi.printedAPIClient")
	}
	_, _ = io.WriteString(f, out)
}
