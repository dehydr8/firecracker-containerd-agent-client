package client

import (
	"log"
	"net"

	"github.com/containerd/ttrpc"
)

func New(addr string, opts ...ttrpc.ClientOpts) (*ttrpc.Client, func()) {
	conn, err := net.Dial("tcp", addr)

	if err != nil {
		log.Fatalf("Failure dialing: %s", err)
	}

	client := ttrpc.NewClient(conn, opts...)
	return client, func() {
		conn.Close()
		client.Close()
	}
}
