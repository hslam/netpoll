// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

package netpoll

import (
	"errors"
	"io"
	"net"
	"syscall"
)

const (
	bufferSize = 0x10000
)

// EOF is the error returned by Read when no more input is available.
var EOF = io.EOF

// EAGAIN is the error when resource temporarily unavailable
var EAGAIN = syscall.EAGAIN

type listener struct {
	Listener net.Listener
	Handler  Handler
}

func (l *listener) Serve() (err error) {
	if l.Listener == nil {
		return errors.New("Listener is nil")
	}
	if l.Handler == nil {
		return errors.New("Handler is nil")
	}
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer func() {
				if e := recover(); e != nil {
				}
			}()
			var err error
			var context Context
			if context, err = l.Handler.Upgrade(c); err != nil {
				c.Close()
				return
			}
			for err == nil {
				err = l.Handler.Serve(context)
			}
			c.Close()
		}(conn)
	}
	return nil
}

func (l *listener) Close() error {
	return l.Listener.Close()
}
