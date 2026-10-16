# Security Policy

## Supported versions

| Version | Supported |
| --- | --- |
| 2.x (the latest minor release) | Yes |
| 1.x | No |

Fixes are released in a new 2.x version; update to the latest 2.x release to get them.

v1, the module `github.com/permitio/permit-golang`, is not supported. v1.4.0 is its last release,
and v1 receives no further fixes, security fixes included. Move to v2,
`github.com/permitio/permit-golang/v2`, to keep receiving fixes.

## Reporting a vulnerability

Report vulnerabilities privately through GitHub's private vulnerability reporting: open the
[Security tab](https://github.com/permitio/permit-golang/security) of this repository and choose
**Report a vulnerability**. Do not open a public issue, pull request or discussion for a
vulnerability.

Include what you can of:

- the SDK version, and the Go version;
- whether the SDK talks to a PDP container or to Permit's cloud PDP, and the PDP version if you
  run one;
- the steps to reproduce, and what an attacker gains.

Leave real API keys, tokens and user data out of the report. If a Permit API key appears in a
report, an issue, a log or anywhere else it should not be, rotate the key in the Permit dashboard.

## Disclosure timeline

- Permit.io acknowledges a report within 3 business days.
- Permit.io releases a fix, or publishes an advisory, within 90 days of the report.
