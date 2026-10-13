module github.com/permitio/permit-golang/v2/tools

go 1.26.0

toolchain go1.27.1

replace github.com/permitio/permit-golang/v2 => ../

tool (
	github.com/editorconfig-checker/editorconfig-checker/v4/cmd/editorconfig-checker
	golang.org/x/exp/cmd/apidiff
	golang.org/x/exp/cmd/gorelease
	golang.org/x/vuln/cmd/govulncheck
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/editorconfig-checker/editorconfig-checker/v4 v4.0.2 // indirect
	github.com/editorconfig/editorconfig-core-go/v2 v2.6.4 // indirect
	github.com/gabriel-vasile/mimetype v1.4.15 // indirect
	github.com/gkampitakis/ciinfo v0.3.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/wlynxg/chardet v1.0.5 // indirect
	golang.org/x/exp v0.0.0-20261005173118-76772065c9b0 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/telemetry v0.0.0-20260924152758-ed294f943157 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/tools v0.51.0 // indirect
	golang.org/x/vuln v1.8.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
