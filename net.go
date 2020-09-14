// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

package netpoll

import (
	"errors"
	"io"
	"net"
	"syscall"
)

var EOF = io.EOF
var EAGAIN = syscall.EAGAIN

type Event struct {
	Buffer        int
	NoCopy        bool
	NoAsync       bool
	UpgradeConn   func(conn net.Conn) (upgrade net.Conn, err error)
	Handle        func(req []byte) (res []byte)
	UpgradeHandle func(conn net.Conn) (handle func() error, err error)
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
	} else if l.Event.Handle == nil && l.Event.UpgradeHandle == nil {
		return errors.New("need Handle or UpgradeHandle")
	}
	if l.Event.Buffer < 1 {
		l.Event.Buffer = 0x10000
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
			var handle func() error
			if l.Event.UpgradeConn != nil {
				if upgrade, err := l.Event.UpgradeConn(c); err != nil {
					return
				} else if upgrade != nil && upgrade != c {
					c = upgrade
				}
			}
			if l.Event.UpgradeHandle != nil {
				if h, err := l.Event.UpgradeHandle(c); err != nil {
					return
				} else {
					handle = h
				}
			}
			var n int
			var err error
			if handle != nil {
				for err == nil {
					err = handle()
				}
			} else {
				var buf = make([]byte, l.Event.Buffer)
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
					res := l.Event.Handle(req)
					n, err = c.Write(res)
				}
			}
			c.Close()
		}(conn)
	}
	return nil
}
