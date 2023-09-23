package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	shim "github.com/containerd/containerd/api/runtime/task/v2"
	events "github.com/containerd/containerd/api/services/events/v1"
	"github.com/dehydr8/firecracker-containerd-agent-client/client"
	"github.com/dehydr8/firecracker-containerd-agent-client/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/google/subcommands"
)

type Pair struct {
	req, res interface{}
}

var requestMapping = map[string]Pair{
	"aws.firecracker.containerd.eventbridge.getter/GetEvent": {&emptypb.Empty{}, &events.Envelope{}},

	"containerd.task.v2.Task/State":      {&shim.StateRequest{}, &shim.StateResponse{}},
	"containerd.task.v2.Task/Create":     {&shim.CreateTaskRequest{}, &shim.CreateTaskResponse{}},
	"containerd.task.v2.Task/Start":      {&shim.StartRequest{}, &shim.StartResponse{}},
	"containerd.task.v2.Task/Delete":     {&shim.DeleteRequest{}, &shim.DeleteResponse{}},
	"containerd.task.v2.Task/Pids":       {&shim.PidsRequest{}, &shim.PidsResponse{}},
	"containerd.task.v2.Task/Pause":      {&shim.PauseRequest{}, &emptypb.Empty{}},
	"containerd.task.v2.Task/Resume":     {&shim.ResumeRequest{}, &emptypb.Empty{}},
	"containerd.task.v2.Task/Checkpoint": {&shim.CheckpointTaskRequest{}, &emptypb.Empty{}},
	"containerd.task.v2.Task/Kill":       {&shim.KillRequest{}, &emptypb.Empty{}},
	"containerd.task.v2.Task/Exec":       {&shim.ExecProcessRequest{}, &emptypb.Empty{}},
	"containerd.task.v2.Task/ResizePty":  {&shim.ResizePtyRequest{}, &emptypb.Empty{}},
	"containerd.task.v2.Task/CloseIO":    {&shim.CloseIORequest{}, &emptypb.Empty{}},
	"containerd.task.v2.Task/Update":     {&shim.UpdateTaskRequest{}, &emptypb.Empty{}},
	"containerd.task.v2.Task/Wait":       {&shim.WaitRequest{}, &shim.WaitResponse{}},
	"containerd.task.v2.Task/Stats":      {&shim.StatsRequest{}, &shim.StatsResponse{}},
	"containerd.task.v2.Task/Connect":    {&shim.ConnectRequest{}, &shim.ConnectResponse{}},
	"containerd.task.v2.Task/Shutdown":   {&shim.ShutdownRequest{}, &emptypb.Empty{}},

	"IOProxy/State":  {&proto.StateRequest{}, &proto.StateResponse{}},
	"IOProxy/Attach": {&proto.AttachRequest{}, &emptypb.Empty{}},

	"DriveMounter/MountDrive":   {&proto.MountDriveRequest{}, &emptypb.Empty{}},
	"DriveMounter/UnmountDrive": {&proto.UnmountDriveRequest{}, &emptypb.Empty{}},
}

type CallCmd struct {
	cid     int
	port    int
	service string
	method  string
}

func (*CallCmd) Name() string     { return "call" }
func (*CallCmd) Synopsis() string { return "Call a TTRPC service method" }
func (*CallCmd) Usage() string {
	return `call --service <service> --method <method> <json>:
	Call TTRPC service.
  `
}

func (p *CallCmd) SetFlags(f *flag.FlagSet) {
	f.IntVar(&p.cid, "cid", 0, "Vsock Context ID")
	f.IntVar(&p.port, "port", 10789, "Vsock Port")
	f.StringVar(&p.service, "service", "", "Service name")
	f.StringVar(&p.method, "method", "", "Method name")
}

func (p *CallCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if len(p.service) <= 0 {
		log.Printf("No service defined")
		return subcommands.ExitFailure
	}

	if len(p.method) <= 0 {
		log.Printf("No method defined")
		return subcommands.ExitFailure
	}

	serviceKey := fmt.Sprintf("%s/%s", p.service, p.method)

	val, ok := requestMapping[serviceKey]

	if !ok {
		log.Printf("No request mapping defined for: %s\n", serviceKey)
		return subcommands.ExitFailure
	}

	c, cleanup := client.New(uint32(p.cid), uint32(p.port))
	defer cleanup()

	req := val.req

	if len(f.Args()) > 0 {
		input := f.Arg(0)
		e := json.Unmarshal([]byte(input), req)
		if e != nil {
			log.Printf("Failure unmarshalling input: %s\n", e)
			return subcommands.ExitFailure
		}
	}

	res := val.res

	err := c.Call(context.Background(), p.service, p.method, req, res)

	if err != nil {
		log.Printf("Failure in Call: %s\n", err)
		return subcommands.ExitFailure
	}

	a, _ := json.Marshal(res)

	log.Println(string(a))

	return subcommands.ExitSuccess
}
