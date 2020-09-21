// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build !linux,!darwin,!dragonfly,!freebsd,!netbsd,!openbsd

package netpoll

import (
	"errors"
	"net"
	"sync/atomic"
)

// Server defines parameters for running a server.
type Server struct {
	Network string
	Address string
	// Handler responds to a single request.
	Handler Handler
	// NoAsync do not work for consisted with other system.
	NoAsync   bool
	netServer *netServer
	closed    int32
}

// ListenAndServe listens and then calls Serve on incoming connections
func (s *Server) ListenAndServe() error {
	if atomic.LoadInt32(&s.closed) != 0 {
		return ErrServerClosed
	}
	ln, err := net.Listen(s.Network, s.Address)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Serve serves with handler on incoming connections.
func (s *Server) Serve(l net.Listener) (err error) {
	if l == nil {
		return errors.New("Listener is nil")
	}
	if s.Handler == nil {
		return errors.New("Handler is nil")
	}
	if atomic.LoadInt32(&s.closed) != 0 {
		return ErrServerClosed
	}
	s.netServer = &netServer{Handler: s.Handler}
	return s.netServer.Serve(l)
}

// Close closes the server.
func (s *Server) Close() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return nil
	}
	return s.netServer.Close()
}
