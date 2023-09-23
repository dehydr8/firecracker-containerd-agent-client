package client

import (
	"log"

	"github.com/containerd/ttrpc"
	"github.com/dehydr8/firecracker-containerd-agent-client/util"
)

func New(cid, port uint32, opts ...ttrpc.ClientOpts) (*ttrpc.Client, func()) {
	conn, err := util.VSockDial(cid, port)

	if err != nil {
		log.Fatalf("Failure dialing: %s", err)
	}

	client := ttrpc.NewClient(conn, opts...)
	return client, func() {
		conn.Close()
		client.Close()
	}
}
