module github.com/permitio/permit-golang/v2/tools

go 1.26.0

toolchain go1.27.1

replace github.com/permitio/permit-golang/v2 => ../

tool (
	golang.org/x/exp/cmd/apidiff
	golang.org/x/exp/cmd/gorelease
	golang.org/x/vuln/cmd/govulncheck
)

require (
	golang.org/x/exp v0.0.0-20261005173118-76772065c9b0 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/telemetry v0.0.0-20260924152758-ed294f943157 // indirect
	golang.org/x/tools v0.51.0 // indirect
	golang.org/x/vuln v1.8.0 // indirect
)
