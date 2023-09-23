package util

import (
	"context"

	"github.com/mdlayher/vsock"
	"github.com/sirupsen/logrus"
)

func VSockDial(cid uint32, port uint32) (*vsock.Conn, error) {
	return vsock.Dial(cid, port, &vsock.Config{})
}

func VSockDialConnector(cid uint32, port uint32) IOConnector {
	return func(procCtx context.Context, logger *logrus.Entry) <-chan IOConnectorResult {
		returnCh := make(chan IOConnectorResult)

		go func() {
			defer close(returnCh)

			conn, err := VSockDial(cid, port)
			returnCh <- IOConnectorResult{
				ReadWriteCloser: conn,
				Err:             err,
			}
		}()

		return returnCh
	}
}
