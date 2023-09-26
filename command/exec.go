package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	shim "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/dehydr8/firecracker-containerd-agent-client/client"
	"github.com/dehydr8/firecracker-containerd-agent-client/proto"
	"github.com/dehydr8/firecracker-containerd-agent-client/util"
	"github.com/gogo/protobuf/types"
	"github.com/google/subcommands"
	"github.com/google/uuid"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/term"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	serviceName     = "containerd.task.v2.Task"
	execMethodName  = "Exec"
	startMethodName = "Start"

	minVsockIOPort = uint32(12000)
)

type ExecCmd struct {
	cid         int
	port        int
	containerId string
	execId      string
	cwd         string
	stdout      string
	stderr      string
	tty         bool
	io          bool
	uid         int
	gid         int
	priv        bool

	vsockPortMu      sync.Mutex
	vsockIOPortCount uint32
}

func (s *ExecCmd) nextVSockPort() uint32 {
	s.vsockPortMu.Lock()
	defer s.vsockPortMu.Unlock()

	port := minVsockIOPort + s.vsockIOPortCount
	if port == math.MaxUint32 {
		// given we use 3 ports per container, there would need to
		// be about 1431652098 containers spawned in this VM for
		// this to actually happen in practice.
		panic("overflow of vsock ports")
	}

	s.vsockIOPortCount++
	return port
}

func (*ExecCmd) Name() string     { return "exec" }
func (*ExecCmd) Synopsis() string { return "Execute a command in a container" }
func (*ExecCmd) Usage() string {
	return `exec [-container_id id] <command>:
	Execute a command in the specified container.
  `
}

func (p *ExecCmd) SetFlags(f *flag.FlagSet) {
	f.IntVar(&p.cid, "cid", 0, "Vsock Context ID")
	f.IntVar(&p.port, "port", 10789, "Vsock Port")
	f.StringVar(&p.containerId, "container_id", "", "Container ID")
	f.StringVar(&p.execId, "exec_id", "", "Execution ID")
	f.StringVar(&p.stdout, "stdout", "", "Standard Output")
	f.StringVar(&p.stderr, "stderr", "", "Standard Error")
	f.BoolVar(&p.tty, "tty", false, "Terminal")
	f.BoolVar(&p.io, "io", false, "IO Proxy")
	f.IntVar(&p.uid, "uid", 0, "User")
	f.IntVar(&p.gid, "gid", 0, "Group")
	f.StringVar(&p.cwd, "cwd", "/", "Current working directory")
	f.BoolVar(&p.priv, "priv", false, "All Capabilities")
}

func (p *ExecCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if len(p.containerId) <= 0 {
		log.Printf("No container ID defined")
		return subcommands.ExitFailure
	}

	if len(f.Args()) <= 0 {
		log.Printf("No command defined")
		return subcommands.ExitFailure
	}

	if len(p.execId) <= 0 {
		p.execId = uuid.NewString()
	}

	if len(p.stdout) <= 0 {
		p.stdout = fmt.Sprintf("file:///tmp/%s.stdout", p.execId)
	}

	if len(p.stderr) <= 0 {
		p.stderr = fmt.Sprintf("file:///tmp/%s.stderr", p.execId)
	}

	log.Printf("Execution ID: %s\n", p.execId)

	caps := defaultUnixCaps()

	if p.priv {
		caps = privUnixCaps()
	}

	cmd := &specs.Process{
		User: specs.User{
			UID: uint32(p.uid),
			GID: uint32(p.gid),
		},
		Args: f.Args(),
		Cwd:  p.cwd,
		Capabilities: &specs.LinuxCapabilities{
			Bounding:  caps,
			Permitted: caps,
			Effective: caps,
		},
		NoNewPrivileges: false,
	}

	if p.tty {
		cmd.Env = append(cmd.Env, "TERM=xterm")
	}

	a, _ := json.Marshal(cmd)

	// Firecracker agent expects the spec to be wrapped in ExtraData
	spec := &proto.ExtraData{
		RuncOptions: &anypb.Any{
			TypeUrl: "",
			Value:   a,
		},
		StdinPort:  p.nextVSockPort(),
		StdoutPort: p.nextVSockPort(),
		StderrPort: p.nextVSockPort(),
	}

	marshalled_spec, _ := types.MarshalAny(spec)

	req := &shim.ExecProcessRequest{
		ID:       p.containerId,
		ExecID:   p.execId,
		Terminal: p.tty,
		Spec: &anypb.Any{
			// force TypeUrl as we're using a different proto impl
			TypeUrl: "type.googleapis.com/ExtraData",
			Value:   marshalled_spec.Value,
		},
		Stdout: p.stdout,
		Stderr: p.stderr,
	}

	if p.io {
		req.Stdin = uuid.NewString()
		req.Stdout = uuid.NewString()
		req.Stderr = uuid.NewString()
	}

	client, cleanup := client.New(uint32(p.cid), uint32(p.port))
	defer cleanup()

	res := &emptypb.Empty{}

	execCallError := make(chan error)
	var copyDone <-chan error

	go func() {
		err := client.Call(ctx, serviceName, execMethodName, req, res)
		execCallError <- err
	}()

	// catch-22 in Exec, it won't finish until a connection is accepted for IOProxy
	time.Sleep(1 * time.Second)

	if p.io {
		proxy := util.NewIOConnectorProxy(
			&util.IOConnectorPair{
				ReadConnector:  util.FileConnector(os.Stdin),
				WriteConnector: util.VSockDialConnector(uint32(p.cid), spec.StdinPort),
			},
			&util.IOConnectorPair{
				ReadConnector:  util.VSockDialConnector(uint32(p.cid), spec.StdoutPort),
				WriteConnector: util.FileConnector(os.Stdout),
			},
			&util.IOConnectorPair{
				ReadConnector:  util.VSockDialConnector(uint32(p.cid), spec.StderrPort),
				WriteConnector: util.FileConnector(os.Stderr),
			},
		)

		logger := logrus.New()

		initDone, xcopyDone := proxy.Start(ctx, logger)

		copyDone = xcopyDone

		err := <-initDone
		if err != nil {
			log.Printf("Failure starting IOProxy: %s\n", err)
			return subcommands.ExitFailure
		}

		log.Printf("Proxy attached...\n")
	}

	err := <-execCallError

	if err != nil {
		log.Printf("Failure in exec call: %s\n", err)
		return subcommands.ExitFailure
	}

	log.Printf("Exec call successfull, starting process...\n")

	var termFd int

	if p.tty {
		if fd, ok := util.GetFd(os.Stdin); ok {
			termFd = fd
			state, err := term.MakeRaw(fd)
			if err != nil {
				log.Printf("Failure making terminal: %s\n", err)
				return subcommands.ExitFailure
			}

			defer term.Restore(fd, state)

			go util.WatchWindowSize(ctx, fd, p.containerId, p.execId, client)
		}
	}

	startReq := &shim.StartRequest{
		ID:     p.containerId,
		ExecID: p.execId,
	}

	startRes := &shim.StartResponse{}

	err = client.Call(ctx, serviceName, startMethodName, startReq, startRes)

	if err != nil {
		log.Printf("Failure in start call: %s\n", err)
		return subcommands.ExitFailure
	}

	log.Printf("Command executed with PID: %d\n", startRes.Pid)

	if p.tty {
		// update the initial terminal size
		width, height, _ := term.GetSize(termFd)
		err = util.ResizePty(ctx, p.containerId, p.execId, width, height, client)
	}

	if p.io {
		err = <-copyDone
		if err != nil {
			log.Printf("Failure in IOProxy: %s\n", err)
			return subcommands.ExitFailure
		}
	}

	return subcommands.ExitSuccess
}
