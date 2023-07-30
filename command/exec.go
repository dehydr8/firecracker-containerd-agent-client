package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	shim "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/dehydr8/firecracker-containerd-agent-client/client"
	"github.com/dehydr8/firecracker-containerd-agent-client/proto"
	"github.com/gogo/protobuf/types"
	"github.com/google/subcommands"
	"github.com/google/uuid"
	"github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	serviceName     = "containerd.task.v2.Task"
	execMethodName  = "Exec"
	startMethodName = "Start"
)

type ExecCmd struct {
	address     string
	containerId string
	execId      string
	cwd         string
	stdout      string
	stderr      string
}

func (*ExecCmd) Name() string     { return "exec" }
func (*ExecCmd) Synopsis() string { return "Execute a command in a container" }
func (*ExecCmd) Usage() string {
	return `exec [-address addr] [-container_id id] <command>:
	Execute a command in the specified container.
  `
}

func (p *ExecCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&p.address, "address", "", "Server address")
	f.StringVar(&p.containerId, "container_id", "", "Container ID")
	f.StringVar(&p.stdout, "exec_id", "", "Execution ID")
	f.StringVar(&p.stdout, "stdout", "", "Standard Output")
	f.StringVar(&p.stderr, "stderr", "", "Standard Error")
	f.StringVar(&p.cwd, "cwd", "/", "Current working directory")
}

func (p *ExecCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if len(p.address) <= 0 {
		log.Printf("No address defined")
		return subcommands.ExitFailure
	}

	if len(p.containerId) <= 0 {
		log.Printf("No container ID defined")
		return subcommands.ExitFailure
	}

	if len(f.Args()) <= 0 {
		log.Printf("No command defined")
		return subcommands.ExitFailure
	}

	if len(p.execId) <= 0 {
		p.execId = uuid.New().String()
	}

	if len(p.stdout) <= 0 {
		p.stdout = fmt.Sprintf("file:///tmp/%s.stdout", p.execId)
	}

	if len(p.stderr) <= 0 {
		p.stderr = fmt.Sprintf("file:///tmp/%s.stderr", p.execId)
	}

	log.Printf("Execution ID: %s\n", p.execId)

	cmd := &specs.Process{
		User: specs.User{
			UID: 0,
			GID: 0,
		},
		Args: f.Args(),
		Cwd:  p.cwd,
	}

	a, _ := json.Marshal(cmd)

	// Firecracker agent expects the spec to be wrapped in ExtraData
	spec := &proto.ExtraData{
		RuncOptions: &anypb.Any{
			TypeUrl: "",
			Value:   a,
		},
	}

	marshalled_spec, _ := types.MarshalAny(spec)

	req := &shim.ExecProcessRequest{
		ID:       p.containerId,
		ExecID:   p.execId,
		Terminal: false,
		Spec: &anypb.Any{
			// force TypeUrl as we're using a different proto impl
			TypeUrl: "type.googleapis.com/ExtraData",
			Value:   marshalled_spec.Value,
		},
		Stdout: p.stdout,
		Stderr: p.stderr,
	}

	client, cleanup := client.New(p.address)
	defer cleanup()

	res := &emptypb.Empty{}

	err := client.Call(context.Background(), serviceName, execMethodName, req, res)

	if err != nil {
		log.Printf("Failure in exec call: %s\n", err)
		return subcommands.ExitFailure
	}

	startReq := &shim.StartRequest{
		ID:     p.containerId,
		ExecID: p.execId,
	}

	startRes := &shim.StartResponse{}

	err = client.Call(context.Background(), serviceName, startMethodName, startReq, startRes)

	if err != nil {
		log.Printf("Failure in start call: %s\n", err)
		return subcommands.ExitFailure
	}

	log.Printf("Command executed with PID: %d\n", startRes.Pid)

	return subcommands.ExitSuccess
}
