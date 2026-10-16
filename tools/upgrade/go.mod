module github.com/permitio/permit-golang/v2/tools/upgrade

go 1.26.0

toolchain go1.27.2

replace (
	github.com/permitio/permit-golang/v2 => ../../
	github.com/permitio/permit-golang/v2/tools => ../
)

require (
	github.com/permitio/permit-golang v1.3.0
	github.com/permitio/permit-golang/v2 v2.0.0-00010101000000-000000000000
	github.com/permitio/permit-golang/v2/tools v0.0.0-00010101000000-000000000000
)

require (
	github.com/google/uuid v1.4.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)
