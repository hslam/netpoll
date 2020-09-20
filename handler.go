// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

package netpoll

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
)

// ErrHandlerFunc is the error when the HandlerFunc is nil
var ErrHandlerFunc = errors.New("HandlerFunc must be not nil")

var (
	buffers = sync.Map{}
	assign  int32
)

func assignPool(size int) *sync.Pool {
	for {
		if p, ok := buffers.Load(size); ok {
			return p.(*sync.Pool)
		}
		if atomic.CompareAndSwapInt32(&assign, 0, 1) {
			var pool = &sync.Pool{New: func() interface{} {
				return make([]byte, size)
			}}
			buffers.Store(size, pool)
			atomic.StoreInt32(&assign, 0)
			return pool
		}
	}
}

// Context is returned by Upgrade for serving.
type Context interface{}

// Handler responds to a single request.
type Handler interface {
	// Upgrade upgrades the net.Conn to a Context.
	Upgrade(net.Conn) (Context, error)
	// Serve should serve a single request with the Context.
	Serve(Context) error
}

// NewHandler returns a new Handler.
func NewHandler(upgrade func(net.Conn) (Context, error), serve func(Context) error) Handler {
	return &handler{upgrade: upgrade, serve: serve}
}

type handler struct {
	upgrade func(net.Conn) (Context, error)
	serve   func(Context) error
}

func (h *handler) Upgrade(conn net.Conn) (Context, error) {
	return h.upgrade(conn)
}

func (h *handler) Serve(ctx Context) error {
	return h.serve(ctx)
}

// DataHandler implements the Handler interface.
type DataHandler struct {
	// Shared enables the DataHandler to use the buffer pool for low memory usage.
	Shared bool
	// NoCopy returns the bytes underlying buffer when NoCopy is true,
	// The bytes returned is shared by all invocations of Read, so do not modify it.
	// Default NoCopy is false to make a copy of data for every invocations of Read.
	NoCopy bool
	// BufferSize represents the buffer size.
	BufferSize int
	// HandlerFunc is the data Serve function.
	HandlerFunc func(req []byte) (res []byte)
}

type context struct {
	conn   net.Conn
	pool   *sync.Pool
	buffer []byte
}

// Upgrade sets the net.Conn to a Context.
func (h *DataHandler) Upgrade(conn net.Conn) (Context, error) {
	if h.BufferSize < 1 {
		h.BufferSize = bufferSize
	}
	if h.HandlerFunc == nil {
		return nil, ErrHandlerFunc
	}
	var ctx = &context{conn: conn}
	if h.Shared {
		ctx.pool = assignPool(h.BufferSize)
	} else {
		ctx.buffer = make([]byte, h.BufferSize)
	}
	return ctx, nil
}

// Serve should serve a single request with the Context ctx.
func (h *DataHandler) Serve(ctx Context) error {
	c := ctx.(*context)
	var conn = c.conn
	var n int
	var err error
	var buf []byte
	var req []byte
	if h.Shared {
		buf = c.pool.Get().([]byte)
		defer c.pool.Put(buf)
	} else {
		buf = c.buffer
	}
	n, err = conn.Read(buf)
	if err != nil {
		return err
	}
	req = buf[:n]
	if !h.NoCopy {
		req = make([]byte, n)
		copy(req, buf[:n])
	}
	res := h.HandlerFunc(req)
	if len(res) == 0 {
		return nil
	}
	_, err = conn.Write(res)
	return err
}
