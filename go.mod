module github.com/dehydr8/firecracker-containerd-agent-client

go 1.21

toolchain go1.21.0

require (
	github.com/containerd/containerd v1.7.2
	github.com/containerd/ttrpc v1.2.2
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.3
	github.com/google/subcommands v1.2.0
	github.com/google/uuid v1.3.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/mdlayher/vsock v1.2.1
	github.com/opencontainers/runtime-spec v1.1.0
	github.com/sirupsen/logrus v1.9.3
	golang.org/x/sync v0.3.0
	golang.org/x/term v0.12.0
	google.golang.org/protobuf v1.31.0
)

require (
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/mdlayher/socket v0.5.0 // indirect
	golang.org/x/net v0.15.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230726155614-23370e0ffb3e // indirect
	google.golang.org/grpc v1.57.0 // indirect
)
