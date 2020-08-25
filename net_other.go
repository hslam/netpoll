// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build !linux,!darwin,!dragonfly,!freebsd,!netbsd,!openbsd

package poll

import (
	"net"
)

func Serve(lis net.Listener, event *Event) error {
	l := &Listener{Listener: lis, Event: event}
	return l.Serve()
}

type Listener struct {
	Listener net.Listener
	Event    *Event
}

func (l *Listener) Serve() (err error) {
	listener := &listener{Listener: l.Listener, Event: l.Event}
	return listener.Serve()
}
