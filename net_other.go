// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

//go:build !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd
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
	NoAsync bool
	// UnsharedWorkers do not work for consisted with other system.
	UnsharedWorkers int
	// SharedWorkers do not work for consisted with other system.
	SharedWorkers int
	// TasksPerWorker do not work for consisted with other system.
	TasksPerWorker int
	netServer      *netServer
	closed         int32
}

// ListenAndServe listens on the network address and then calls
// Serve with handler to handle requests on incoming connections.
//
// ListenAndServe always returns a non-nil error.
// After Close the returned error is ErrServerClosed.
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

// Serve accepts incoming connections on the listener l,
// and registers the conn fd to poll. The poll will trigger the fd to
// read requests and then call handler to reply to them.
//
// Serve always returns a non-nil error.
// After Close the returned error is ErrServerClosed.
func (s *Server) Serve(l net.Listener) (err error) {
	if l == nil {
		return ErrListener
	}
	if s.Handler == nil {
		return ErrHandler
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
