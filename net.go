// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

package netpoll

import (
	"errors"
	"io"
	"net"
	"syscall"
)

// EOF is the error returned by Read when no more input is available.
var EOF = io.EOF

// EAGAIN is the error when resource temporarily unavailable
var EAGAIN = syscall.EAGAIN

// Event represents the handler event.
type Event struct {
	// BufferSize represents the buffer size.
	BufferSize int
	// NoCopy returns the bytes underlying buffer when NoCopy is true,
	// The bytes returned is shared by all invocations of Read, so do not modify it.
	// Default NoCopy is false to make a copy of data for every invocations of Read.
	NoCopy bool
	// NoAsync disables async workers.
	NoAsync bool
	// UpgradeConn upgrades connection.
	UpgradeConn func(conn net.Conn) (upgrade net.Conn, err error)
	// Handler represents the handler function.
	Handler func(req []byte) (res []byte)
	// UpgradeHandler upgrades the handler function.
	UpgradeHandler func(conn net.Conn) (handle func() error, err error)
}

// ListenAndServe listens on the network address and then calls
// Serve with event on incoming connections
func ListenAndServe(network, address string, event *Event) error {
	lis, err := net.Listen(network, address)
	if err != nil {
		panic(err)
	}
	return Serve(lis, event)
}

// Serve serves with event on incoming connections.
func Serve(lis net.Listener, event *Event) error {
	l := &Listener{Listener: lis, Event: event}
	return l.Serve()
}

type listener struct {
	Listener net.Listener
	Event    *Event
}

func (l *listener) Serve() (err error) {
	if l.Listener == nil {
		return errors.New("listener is nil")
	}
	if l.Event == nil {
		return errors.New("event is nil")
	} else if l.Event.Handler == nil && l.Event.UpgradeHandler == nil {
		return errors.New("need Handler or UpgradeHandler")
	}
	if l.Event.BufferSize < 1 {
		l.Event.BufferSize = 0x10000
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
			var handler func() error
			if l.Event.UpgradeConn != nil {
				upgrade, err := l.Event.UpgradeConn(c)
				if err != nil {
					return
				} else if upgrade != nil && upgrade != c {
					c = upgrade
				}
			}
			if l.Event.UpgradeHandler != nil {
				h, err := l.Event.UpgradeHandler(c)
				if err != nil {
					return
				}
				handler = h
			}
			var n int
			var err error
			if handler != nil {
				for err == nil {
					err = handler()
				}
			} else {
				var buf = make([]byte, l.Event.BufferSize)
				for err == nil {
					n, err = c.Read(buf)
					if err != nil {
						break
					}
					req := buf[:n]
					if !l.Event.NoCopy {
						req = make([]byte, n)
						copy(req, buf[:n])
					}
					res := l.Event.Handler(req)
					n, err = c.Write(res)
				}
			}
			c.Close()
		}(conn)
	}
	return nil
}

func (l *listener) Close() error {
	return l.Listener.Close()
}
