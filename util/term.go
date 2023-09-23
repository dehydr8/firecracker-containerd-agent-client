// bits and pieces from https://github.com/superfly/flyctl/blob/master/ssh/io.go

package util

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	shim "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/ttrpc"
	"golang.org/x/term"
	"google.golang.org/protobuf/types/known/emptypb"
)

type FdReader interface {
	io.Reader
	Fd() uintptr
}

func GetFd(reader io.Reader) (fd int, ok bool) {
	fdthing, ok := reader.(FdReader)
	if !ok {
		return 0, false
	}

	fd = int(fdthing.Fd())
	return fd, term.IsTerminal(fd)
}

func ResizePty(ctx context.Context, containerId, executionId string, width, height int, client *ttrpc.Client) error {
	sizeReq := &shim.ResizePtyRequest{
		ID:     containerId,
		ExecID: executionId,
		Width:  uint32(width),
		Height: uint32(height),
	}

	sizeRes := &emptypb.Empty{}

	return client.Call(ctx, "containerd.task.v2.Task", "ResizePty", sizeReq, sizeRes)
}

func WatchWindowSize(ctx context.Context, fd int, containerId, executionId string, client *ttrpc.Client) error {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGWINCH)

	for {
		select {
		case <-sigc:
		case <-ctx.Done():
			return nil
		}

		width, height, err := term.GetSize(fd)
		if err != nil {
			return err
		}

		err = ResizePty(ctx, containerId, executionId, width, height, client)

		if err != nil {
			return err
		}
	}
}
