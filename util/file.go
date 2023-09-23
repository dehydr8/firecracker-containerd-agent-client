package util

import (
	"context"
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

type ReadWriteNopCloserWrapper struct {
	io.Reader
	io.Writer
}

func (r *ReadWriteNopCloserWrapper) Close() error {
	return nil
}

func FileConnector(file *os.File) IOConnector {
	return func(procCtx context.Context, logger *logrus.Entry) <-chan IOConnectorResult {
		returnCh := make(chan IOConnectorResult, 1)
		defer close(returnCh)

		returnCh <- IOConnectorResult{
			ReadWriteCloser: &ReadWriteNopCloserWrapper{
				Reader: file,
				Writer: file,
			},
			Err: nil,
		}
		return returnCh
	}
}
