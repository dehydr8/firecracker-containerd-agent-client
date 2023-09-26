package command

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"path/filepath"

	shim "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/containerd/api/types"
	"github.com/dehydr8/firecracker-containerd-agent-client/client"
	"github.com/dehydr8/firecracker-containerd-agent-client/proto"
	ptypes "github.com/gogo/protobuf/types"
	"github.com/google/subcommands"
	"github.com/google/uuid"
	"github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	createMethodName  = "Create"
	rwm               = "rwm"
	defaultRootfsPath = "rootfs"
)

type CreateCmd struct {
	cid          int
	port         int
	bundle       string
	rootFSConfig string
	mountsConfig string
	namespace    string
	pid          string
	priv         bool
}

func (*CreateCmd) Name() string     { return "create" }
func (*CreateCmd) Synopsis() string { return "Create a new container" }
func (*CreateCmd) Usage() string {
	return `create [-ref id] <command>:
	Create a new container.
  `
}

func (p *CreateCmd) SetFlags(f *flag.FlagSet) {
	f.IntVar(&p.cid, "cid", 0, "Vsock Context ID")
	f.IntVar(&p.port, "port", 10789, "Vsock Port")
	f.StringVar(&p.rootFSConfig, "rootfs-config", "{}", "RootFS Config JSON")
	f.StringVar(&p.mountsConfig, "mounts-config", "[]", "Mounts Config JSON")
	f.StringVar(&p.bundle, "bundle", "", "Bundle")
	f.StringVar(&p.namespace, "examplens", "", "cgroup Namespace")
	f.StringVar(&p.pid, "pid", "", "PID NS Path")
	f.BoolVar(&p.priv, "priv", false, "All Capabilities")

}

func defaultUnixCaps() []string {
	return []string{
		"CAP_CHOWN",
		"CAP_DAC_OVERRIDE",
		"CAP_FSETID",
		"CAP_FOWNER",
		"CAP_MKNOD",
		"CAP_NET_RAW",
		"CAP_SETGID",
		"CAP_SETUID",
		"CAP_SETFCAP",
		"CAP_SETPCAP",
		"CAP_NET_BIND_SERVICE",
		"CAP_SYS_CHROOT",
		"CAP_KILL",
		"CAP_AUDIT_WRITE",
	}
}

func privUnixCaps() []string {
	return []string{
		"CAP_CHOWN",
		"CAP_DAC_OVERRIDE",
		"CAP_DAC_READ_SEARCH",
		"CAP_FOWNER",
		"CAP_FSETID",
		"CAP_KILL",
		"CAP_SETGID",
		"CAP_SETUID",
		"CAP_SETPCAP",
		"CAP_LINUX_IMMUTABLE",
		"CAP_NET_BIND_SERVICE",
		"CAP_NET_BROADCAST",
		"CAP_NET_ADMIN",
		"CAP_NET_RAW",
		"CAP_IPC_LOCK",
		"CAP_IPC_OWNER",
		"CAP_SYS_MODULE",
		"CAP_SYS_RAWIO",
		"CAP_SYS_CHROOT",
		"CAP_SYS_PTRACE",
		"CAP_SYS_PACCT",
		"CAP_SYS_ADMIN",
		"CAP_SYS_BOOT",
		"CAP_SYS_NICE",
		"CAP_SYS_RESOURCE",
		"CAP_SYS_TIME",
		"CAP_SYS_TTY_CONFIG",
		"CAP_MKNOD",
		"CAP_LEASE",
		"CAP_AUDIT_WRITE",
		"CAP_AUDIT_CONTROL",
		"CAP_SETFCAP",
		"CAP_MAC_OVERRIDE",
		"CAP_MAC_ADMIN",
		"CAP_SYSLOG",
		"CAP_WAKE_ALARM",
		"CAP_BLOCK_SUSPEND",
		"CAP_AUDIT_READ",
	}
}

func populateDefaultUnixSpec(ns, id, pidNsPath string, caps []string) *specs.Spec {
	return &specs.Spec{
		Version: specs.Version,
		Root: &specs.Root{
			Path: defaultRootfsPath,
		},
		Process: &specs.Process{
			Cwd:             "/",
			NoNewPrivileges: false,
			User: specs.User{
				UID: 0,
				GID: 0,
			},
			Capabilities: &specs.LinuxCapabilities{
				Bounding:  caps,
				Permitted: caps,
				Effective: caps,
			},
		},
		Linux: &specs.Linux{
			CgroupsPath: filepath.Join("/", ns, id),
			Resources: &specs.LinuxResources{
				Devices: []specs.LinuxDeviceCgroup{
					{
						Allow:  true,
						Access: rwm,
					},
				},
			},
			Namespaces: []specs.LinuxNamespace{
				{
					Type: specs.PIDNamespace,
					Path: pidNsPath,
				},
				{
					Type: specs.IPCNamespace,
				},
				{
					Type: specs.UTSNamespace,
				},
				{
					Type: specs.MountNamespace,
				},
				{
					Type: specs.NetworkNamespace,
				},
			},
		},
		Mounts: []specs.Mount{
			{
				Destination: "/proc",
				Type:        "proc",
				Source:      "proc",
				Options:     []string{"nosuid", "noexec", "nodev"},
			},
			{
				Destination: "/dev",
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
			},
			{
				Destination: "/dev/pts",
				Type:        "devpts",
				Source:      "devpts",
				Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"},
			},
			{
				Destination: "/dev/shm",
				Type:        "tmpfs",
				Source:      "shm",
				Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
			},
			{
				Destination: "/dev/mqueue",
				Type:        "mqueue",
				Source:      "mqueue",
				Options:     []string{"nosuid", "noexec", "nodev"},
			},
			{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "rw"},
			},
			{
				Destination: "/sys/fs/cgroup",
				Type:        "cgroup",
				Source:      "cgroup",
				Options:     []string{"nosuid", "noexec", "nodev", "rw"},
			},
		},
	}
}

func (p *CreateCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	id := uuid.NewString()
	caps := defaultUnixCaps()

	log.Printf("Creating container: %s\n", id)

	if p.priv {
		caps = privUnixCaps()
	}

	spec := populateDefaultUnixSpec(p.namespace, id, p.pid, caps)

	spec.Process.Args = f.Args()
	spec.Process.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}

	var inputMounts []specs.Mount
	if err := json.Unmarshal([]byte(p.mountsConfig), &inputMounts); err != nil {
		log.Printf("Failure parsing mounts JSON config: %s\n", err)
		return subcommands.ExitFailure
	}

	// join it with the defaults
	spec.Mounts = append(spec.Mounts, inputMounts...)

	a, _ := json.Marshal(spec)

	// Firecracker agent expects the spec to be wrapped in ExtraData
	wrapped := &proto.ExtraData{
		RuncOptions: &anypb.Any{
			TypeUrl: "",
			Value:   a,
		},
		JsonSpec: a,
	}

	marshalled_spec, _ := ptypes.MarshalAny(wrapped)

	var rootFSMount types.Mount
	if err := json.Unmarshal([]byte(p.rootFSConfig), &rootFSMount); err != nil {
		log.Printf("Failure parsing RootFS JSON config: %s\n", err)
		return subcommands.ExitFailure
	}

	req := &shim.CreateTaskRequest{
		ID:     id,
		Bundle: p.bundle,
		Rootfs: []*types.Mount{&rootFSMount},
		Options: &anypb.Any{
			TypeUrl: "type.googleapis.com/ExtraData",
			Value:   marshalled_spec.Value,
		},
	}

	client, cleanup := client.New(uint32(p.cid), uint32(p.port))
	defer cleanup()

	res := &shim.CreateTaskResponse{}

	err := client.Call(ctx, serviceName, createMethodName, req, res)

	if err != nil {
		log.Printf("Failure in create call: %s\n", err)
		return subcommands.ExitFailure
	}

	log.Printf("Create call successfull, started with PID: %d...\n", res.Pid)

	return subcommands.ExitSuccess
}
