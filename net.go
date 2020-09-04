// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

package poll

import (
	"errors"
	"net"
)

type Messages interface {
	SetBatch(batch func() int)
	ReadMessage() ([]byte, error)
	WriteMessage([]byte) error
	Close() error
}

type Event struct {
	Buffer  int
	NoCopy  bool
	Shared  bool
	NoAsync bool
	Batch   func() int
	Upgrade func(conn net.Conn) (net.Conn, Messages, error)
	Handle  func(req []byte) (res []byte)
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
		go func(c net.Conn) {
			defer func() {
				if e := recover(); e != nil {
				}
			}()
			var messages Messages
			if l.Event.Upgrade != nil {
				if upgrade, m, err := l.Event.Upgrade(c); err != nil {
					return
				} else {
					if m != nil {
						messages = m
						if l.Event.Batch != nil {
							messages.SetBatch(l.Event.Batch)
						}
					}
					if upgrade != nil && upgrade != c {
						c = upgrade
					}
				}
			}
			var n int
			var err error
			if messages != nil {
				var req []byte
				for err == nil {
					req, err = messages.ReadMessage()
					if err != nil {
						break
					}
					res := l.Event.Handle(req)
					err = messages.WriteMessage(res)
				}
				messages.Close()
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
