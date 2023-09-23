package util

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"
)

func FileConnector(file *os.File) IOConnector {
	return func(procCtx context.Context, logger *logrus.Entry) <-chan IOConnectorResult {
		returnCh := make(chan IOConnectorResult, 1)
		defer close(returnCh)
		returnCh <- IOConnectorResult{
			ReadWriteCloser: file,
			Err:             nil,
		}
		return returnCh
	}
}
