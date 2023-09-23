# firecracker-containerd-agent-client
CLI client for interacting with the [Firecracker containerd agent](https://github.com/firecracker-microvm/firecracker-containerd/tree/main/agent).

## Build
```bash
# build directly
CGO_ENABLED=0 go build

# build with docker
docker run -it --rm -v $(pwd):/project -w /project golang:1.21 CGO_ENABLED=0 go build
```