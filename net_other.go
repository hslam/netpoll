// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build !linux,!darwin,!dragonfly,!freebsd,!netbsd,!openbsd

package netpoll

import (
	"net"
)

//Listener is a generic network listener for stream-oriented protocols.
type Listener struct {
	//Listener is a net.Listener.
	Listener net.Listener
	//Event is a handler event.
	Event *Event
}

// Serve serves with event on incoming connections.
func (l *Listener) Serve() (err error) {
	listener := &listener{Listener: l.Listener, Event: l.Event}
	return listener.Serve()
}

// Close closes the listener.
func (l *Listener) Close() error {
	return listener.Close()
}
