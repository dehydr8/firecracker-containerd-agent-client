// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package util

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	// By default, once the task exits, wait defaultIOFlushTimeout for
	// the IO streams to close on their own before forcibly closing them.
	defaultIOFlushTimeout = 5 * time.Second
	defaultBufferSize     = 1024
)

type IOProxy interface {
	Start(procCtx context.Context, logger *logrus.Logger) (ioInitDone <-chan error, ioCopyDone <-chan error)
	Close()
	IsOpen() bool
}

type IOConnector func(procCtx context.Context, logger *logrus.Entry) <-chan IOConnectorResult

type IOConnectorResult struct {
	io.ReadWriteCloser
	Err error
}

type IOConnectorPair struct {
	ReadConnector  IOConnector
	WriteConnector IOConnector
}

type ioConnectorSet struct {
	stdin  *IOConnectorPair
	stdout *IOConnectorPair
	stderr *IOConnectorPair

	// closeMu is needed since Close() will be called from different goroutines.
	closeMu sync.Mutex
	closed  bool
}

func NewIOConnectorProxy(stdin, stdout, stderr *IOConnectorPair) IOProxy {
	return &ioConnectorSet{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		closed: false,
	}
}

func (ioConnectorSet *ioConnectorSet) Close() {
	ioConnectorSet.closeMu.Lock()
	defer ioConnectorSet.closeMu.Unlock()

	ioConnectorSet.closed = true
}

func (ioConnectorSet *ioConnectorSet) IsOpen() bool {
	ioConnectorSet.closeMu.Lock()
	defer ioConnectorSet.closeMu.Unlock()

	return !ioConnectorSet.closed
}

func (connectorPair *IOConnectorPair) proxy(
	ctx context.Context,
	logger *logrus.Entry,
	timeoutAfterExit time.Duration,
) (ioInitDone <-chan error, ioCopyDone <-chan error) {
	// initDone might not have to be buffered. We only send ioInitErr once.
	initDone := make(chan error, 2)
	copyDone := make(chan error)

	ioCtx, ioCancel := context.WithCancel(context.Background())

	// Start the initialization process. Any synchronous setup made by the connectors will
	// be completed after these lines. Async setup will be done once initDone is closed in
	// the goroutine below.
	readerResultCh := connectorPair.ReadConnector(ioCtx, logger.WithField("direction", "read"))
	writerResultCh := connectorPair.WriteConnector(ioCtx, logger.WithField("direction", "write"))

	go func() {
		defer ioCancel()
		defer close(copyDone)

		var reader io.ReadCloser
		var writer io.WriteCloser
		var ioInitErr error

		// Send the first error we get to initDone, but consume both so we can ensure both
		// end up closed in the case of an error
		for readerResultCh != nil || writerResultCh != nil {
			var err error
			select {
			case readerResult := <-readerResultCh:
				readerResultCh = nil
				if err = readerResult.Err; err == nil {
					reader = readerResult.ReadWriteCloser
				}
			case writerResult := <-writerResultCh:
				writerResultCh = nil
				if err = writerResult.Err; err == nil {
					writer = writerResult.ReadWriteCloser
				}
			}

			if err != nil {
				ioInitErr = fmt.Errorf("error initializing io: %w", err)
				logger.WithError(ioInitErr).Error()
				initDone <- ioInitErr
			}
		}

		close(initDone)
		if ioInitErr != nil {
			logClose(logger, reader, writer)
			return
		}

		// IO streams have been initialized successfully

		// Once the proc exits, wait the provided time before forcibly closing io streams.
		// If the io streams close on their own before the timeout, the Close calls here
		// should just be no-ops.
		go func() {
			<-ctx.Done()
			time.AfterFunc(timeoutAfterExit, func() {
				logClose(logger, reader, writer)
			})
		}()

		logger.Debug("begin copying io")
		defer logger.Debug("end copying io")

		size, err := io.CopyBuffer(writer, reader, make([]byte, defaultBufferSize))
		logger.Debugf("copied %d", size)
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") ||
				strings.Contains(err.Error(), "file already closed") {
				logger.Infof("connection was closed: %v", err)
			} else {
				logger.WithError(err).Error("error copying io")
			}
			copyDone <- err
		}
		defer logClose(logger, reader, writer)
	}()

	return initDone, copyDone
}

func logClose(logger *logrus.Entry, streams ...io.Closer) {
	var closeErr error
	for _, stream := range streams {
		if stream == nil {
			continue
		}

		err := stream.Close()
		if err != nil {
			closeErr = multierror.Append(closeErr, err)
		}
	}

	if closeErr != nil {
		logger.WithError(closeErr).Error("error closing io stream")
	}
}

// start starts goroutines to copy stdio and returns two channels.
// The first channel returns its initialization error. The second channel returns errors from copying.
func (ioConnectorSet *ioConnectorSet) Start(procCtx context.Context, logger *logrus.Logger) (ioInitDone <-chan error, ioCopyDone <-chan error) {
	var initErrG errgroup.Group

	// When one of the goroutines returns an error, we will cancel
	// the rest of goroutines through the ctx below.
	copyErrG, ctx := errgroup.WithContext(procCtx)

	waitErrs := func(initErrCh, copyErrCh <-chan error) {
		initErrG.Go(func() error { return <-initErrCh })
		copyErrG.Go(func() error { return <-copyErrCh })
	}

	if ioConnectorSet.stdin != nil {
		// For Stdin only, provide 0 as the timeout to wait after the proc exits before closing IO streams.
		// There's no reason to send stdin data to a proc that's already dead.
		waitErrs(ioConnectorSet.stdin.proxy(ctx, logger.WithField("stream", "stdin"), 0))
	} else {
		logger.Debug("skipping proxy io for unset stdin")
	}

	if ioConnectorSet.stdout != nil {
		waitErrs(ioConnectorSet.stdout.proxy(ctx, logger.WithField("stream", "stdout"), defaultIOFlushTimeout))
	} else {
		logger.Debug("skipping proxy io for unset stdout")
	}

	if ioConnectorSet.stderr != nil {
		waitErrs(ioConnectorSet.stderr.proxy(ctx, logger.WithField("stream", "stderr"), defaultIOFlushTimeout))
	} else {
		logger.Debug("skipping proxy io for unset stderr")
	}

	// These channels are not buffered, since we will close them right after having one error.
	// Callers must read the channels.
	initDone := make(chan error)
	go func() {
		defer close(initDone)
		initDone <- initErrG.Wait()
	}()

	copyDone := make(chan error)
	go func() {
		defer close(copyDone)
		copyDone <- copyErrG.Wait()
	}()

	return initDone, copyDone
}
