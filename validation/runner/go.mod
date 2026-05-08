module github.com/danigoland/btf2go/validation/runner

go 1.25.5

require (
	github.com/cilium/ebpf v0.21.0
	github.com/danigoland/btf2go v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.37.0 // indirect
)

replace github.com/danigoland/btf2go => ../../
