package netpoll

import (
	"errors"
	"net"
	"sync/atomic"
)

// ErrServerClosed is returned by the Server's Serve, ServeTLS, ListenAndServe,
// and ListenAndServeTLS methods after a call to Shutdown or Close.
var ErrServerClosed = errors.New("Server closed")

// Server defines parameters for running a server.
type Server struct {
	Network  string
	Address  string
	Handler  Handler
	listener *Listener
	closed   int32
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

// Serve accepts incoming connections on the listener l,
func (s *Server) Serve(l net.Listener) error {
	s.listener = &Listener{Listener: l, Handler: s.Handler}
	return s.listener.Serve()
}

// Close immediately closes the Server.
func (s *Server) Close() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return nil
	}
	return s.listener.Close()
}

// ListenAndServe listens on the network address and then serves
// incoming connections with event.
func ListenAndServe(network, address string, handler Handler) error {
	server := &Server{Network: network, Address: address, Handler: handler}
	return server.ListenAndServe()
}

// Serve serves incoming connections with event on the listener lis.
func Serve(lis net.Listener, handler Handler) error {
	server := &Server{Handler: handler}
	return server.Serve(lis)
}
