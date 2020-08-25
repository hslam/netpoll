// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build !linux,!darwin,!dragonfly,!freebsd,!netbsd,!openbsd

package poll

import (
	"errors"
	"net"
	"time"
)

type Event struct {
	Buffer  int
	NoCopy  bool
	Shared  bool
	Upgrade func(conn Conn) (Conn, error)
	Handle  func(req []byte) (res []byte)
}

func Serve(lis net.Listener, event *Event) error {
	l := &Listener{Listener: lis, Event: event}
	return l.Serve()
}

type Listener struct {
	Listener net.Listener
	Event    *Event
}

func (l *Listener) Serve() (err error) {
	if l.Listener == nil {
		return errors.New("listener is nil")
	}
	if l.Event == nil {
		return errors.New("event is nil")
	} else if l.Event.Upgrade == nil && l.Event.Handle == nil {
		return errors.New("need Upgrade or Handle")
	}
	if l.Event.Buffer < 1 {
		l.Event.Buffer = 0x10000
	}
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			continue
		}
		go func(c Conn) {
			defer func() {
				if e := recover(); e != nil {
				}
			}()
			if l.Event.Upgrade != nil {
				if upgrade, err := l.Event.Upgrade(c); err != nil {
					return
				} else if upgrade != nil && upgrade != c {
					c = upgrade
				}
			}
			var n int
			var err error
			var buf = make([]byte, l.Event.Buffer)
			for err == nil {
				n, err = c.Read(buf)
				if err != nil {
					break
				}
				req := buf[:n]
				if !l.Event.NoCopy {
					req := make([]byte, n)
					copy(req, buf[:n])
				}
				res := l.Event.Handle(req)
				n, err = c.Write(res)
			}
			c.Close()
		}(conn)
	}
	return nil
}

type Conn interface {
	Read(b []byte) (n int, err error)
	Write(b []byte) (n int, err error)
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}
