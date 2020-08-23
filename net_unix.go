// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build linux darwin dragonfly freebsd netbsd openbsd

package poll

import (
	"errors"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Event struct {
	Buffer  int
	Upgrade func(conn Conn) error
	Handle  func(req []byte) (res []byte)
}

func Serve(lis net.Listener, event *Event) error {
	l := &Listener{Listener: lis, Event: event}
	return l.Serve()
}

type Listener struct {
	Listener net.Listener
	Event    *Event
	file     *os.File
	fd       int
	workers  []*worker
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
	switch netListener := l.Listener.(type) {
	case *net.TCPListener:
		if l.file, err = netListener.File(); err != nil {
			l.Listener.Close()
			return err
		}
	case *net.UnixListener:
		if l.file, err = netListener.File(); err != nil {
			l.Listener.Close()
			return err
		}
	default:
		return errors.New("listener not supported")
	}
	l.fd = int(l.file.Fd())
	if err := syscall.SetNonblock(l.fd, true); err != nil {
		l.Listener.Close()
		return err
	}
	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU()*2; i++ {
		p, err := Create()
		if err != nil {
			panic(err)
		}
		w := &worker{
			index:    i,
			listener: l,
			conns:    make(map[int]*conn),
			poll:     p,
			events:   make([]PollEvent, 0x400),
			handle:   l.Event.Handle,
			done:     make(chan bool, 0x10),
			tasks:    runtime.NumCPU() * 4,
		}
		w.jobs = make(chan *conn, w.tasks)
		w.poll.Register(l.fd)
		l.workers = append(l.workers, w)
		wg.Add(1)
		go w.run(&wg)
	}
	wg.Wait()
	return nil
}

func (l *Listener) min() int64 {
	min := l.workers[0].count
	if len(l.workers) > 1 {
		for i := 1; i < len(l.workers); i++ {
			if l.workers[i].count < min {
				min = l.workers[i].count
			}
		}
	}
	return min
}

type worker struct {
	index    int
	listener *Listener
	count    int64
	mu       sync.Mutex
	conns    map[int]*conn
	poll     *Poll
	events   []PollEvent
	handle   func(req []byte) (res []byte)
	tasks    int
	idle     int64
	jobs     chan *conn
	done     chan bool
}

func (w *worker) run(wg *sync.WaitGroup) {
	defer wg.Done()
	defer w.Close()
	if w.handle != nil {
		for i := 0; i < w.tasks; i++ {
			w.idle += 1
			go func(w *worker) {
				buf := make([]byte, w.listener.Event.Buffer)
				for fd := range w.jobs {
					w.read(fd, buf)
					atomic.AddInt64(&w.idle, 1)
				}
			}(w)
		}
	}
	var n int
	var err error
	for err == nil {
		n, err = w.poll.Wait(w.events)
		for i := 0; i < n; i++ {
			w.serve(w.events[i])
		}
	}
}

func (w *worker) serve(ev PollEvent) error {
	fd := ev.Fd
	if fd == 0 {
		return nil
	}
	if fd == w.listener.fd {
		w.accept()
		return nil
	}
	w.mu.Lock()
	if c, ok := w.conns[fd]; !ok {
		w.mu.Unlock()
		return nil
	} else {
		w.mu.Unlock()
		switch ev.Mode {
		case WRITE:
			w.write(c)
			return nil
		case READ:
			if atomic.LoadInt64(&w.count) > 1 {
				if atomic.LoadInt64(&w.idle) > 0 {
					atomic.AddInt64(&w.idle, -1)
					w.jobs <- c
				} else {
					go w.read(c, []byte{})
				}
				return nil
			} else {
				return w.read(c, nil)
			}
		}
		return nil
	}
}

func (w *worker) accept() error {
	if atomic.LoadInt64(&w.count) < 1 || atomic.LoadInt64(&w.count) <= w.listener.min() {
		nfd, sa, err := syscall.Accept(w.listener.fd)
		if err != nil {
			if err == syscall.EAGAIN {
				return nil
			}
			return err
		}
		if err := syscall.SetNonblock(nfd, true); err != nil {
			return err
		}
		var raddr net.Addr
		switch sockaddr := sa.(type) {
		case *syscall.SockaddrUnix:
			raddr = &net.UnixAddr{Net: "unix", Name: sockaddr.Name}
		case *syscall.SockaddrInet4:
			raddr = &net.TCPAddr{
				IP:   append([]byte{}, sockaddr.Addr[:]...),
				Port: sockaddr.Port,
			}
		case *syscall.SockaddrInet6:
			var zone string
			if ifi, err := net.InterfaceByIndex(int(sockaddr.ZoneId)); err == nil {
				zone = ifi.Name
			}
			raddr = &net.TCPAddr{
				IP:   append([]byte{}, sockaddr.Addr[:]...),
				Port: sockaddr.Port,
				Zone: zone,
			}
		}
		c := &conn{w: w, fd: nfd, raddr: raddr, laddr: w.listener.Listener.Addr(), buf: make([]byte, w.listener.Event.Buffer)}
		if w.listener.Event.Upgrade != nil {
			go func(w *worker, c *conn) {
				defer func() {
					if e := recover(); e != nil {
					}
				}()
				if err := syscall.SetNonblock(c.fd, false); err != nil {
					return
				}
				if err := w.listener.Event.Upgrade(c); err != nil {
					return
				}
				if err := syscall.SetNonblock(c.fd, true); err != nil {
					return
				}
				w.poll.Register(c.fd)
				w.increase(c)
				c.ready = true
			}(w, c)
			return nil
		}
		w.poll.Register(c.fd)
		w.increase(c)
		c.ready = true
	}
	return nil
}

func (w *worker) write(c *conn) error {
	if !c.ready {
		return nil
	}
	if retain, err := c.flush(); err != nil {
		return err
	} else if retain > 0 {
		w.poll.Write(c.fd)
	}
	return nil
}

func (w *worker) read(c *conn, buf []byte) error {
	if !c.ready {
		return nil
	}
	n, err := c.Read(c.buf)
	if n == 0 || err != nil {
		if err == syscall.EAGAIN || c.closed {
			return nil
		}
		w.decrease(c)
		c.Close()
		return nil
	}
	var req []byte
	req = c.buf[:n]
	if buf != nil {
		if cap(buf) >= n {
			req = buf[:n]
			copy(req, c.buf[:n])
		} else {
			req = make([]byte, n)
			copy(req, c.buf[:n])
		}
	}
	res := w.handle(req)
	if len(res) > 0 {
		c.write(res)
		w.write(c)
	}
	return nil
}

func (w *worker) increase(c *conn) {
	w.mu.Lock()
	w.conns[c.fd] = c
	w.mu.Unlock()
	atomic.AddInt64(&w.count, 1)
}

func (w *worker) decrease(c *conn) {
	atomic.AddInt64(&w.count, -1)
	w.mu.Lock()
	delete(w.conns, c.fd)
	w.mu.Unlock()
}

func (w *worker) Close() {
	if w.jobs != nil {
		close(w.jobs)
		w.jobs = nil
	}
	w.poll.Close()
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

type Writer interface {
	Write(p []byte) (n int, err error)
}

type writer struct {
	c *conn
}

func (w *writer) Write(b []byte) (n int, err error) {
	defer w.c.w.poll.Write(w.c.fd)
	return w.c.write(b)
}

type conn struct {
	w       *worker
	rMu     sync.Mutex
	wMu     sync.Mutex
	fd      int
	buf     []byte
	send    []byte
	laddr   net.Addr
	raddr   net.Addr
	upgrade Conn
	ready   bool
	closed  bool
}

func (c *conn) Read(b []byte) (n int, err error) {
	c.rMu.Lock()
	defer c.rMu.Unlock()
	n, err = syscall.Read(c.fd, b)
	if n == 0 {
		err = syscall.EINVAL
	}
	return
}

func (c *conn) Write(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}
	c.wMu.Lock()
	defer c.wMu.Unlock()
	var retain = len(b)
	for retain > 0 {
		n, err = syscall.Write(c.fd, b[len(b)-retain:])
		if err != nil {
			return len(b) - retain, err
		}
		retain -= n
	}
	return len(b), nil
}

func (c *conn) write(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}
	c.wMu.Lock()
	defer c.wMu.Unlock()
	c.send = append(c.send, b...)
	return len(b), nil
}

func (c *conn) flush() (retain int, err error) {
	c.wMu.Lock()
	defer c.wMu.Unlock()
	if len(c.send) == 0 {
		return 0, nil
	}
	if n, err := syscall.Write(c.fd, c.send); err != nil || n < 1 {
		return len(c.send), err
	} else if n < len(c.send) {
		num := copy(c.send, c.send[n:])
		c.send = c.send[:num]
		return num, nil
	} else {
		c.send = c.send[:0]
		return 0, nil
	}
	return
}

func (c *conn) Close() (err error) {
	if c.closed {
		return nil
	}
	c.closed = true
	return syscall.Close(c.fd)
}
func (c *conn) LocalAddr() net.Addr {
	return c.laddr
}

func (c *conn) RemoteAddr() net.Addr {
	return c.raddr
}

func (c *conn) SetDeadline(t time.Time) error {
	return errors.New("not supported")
}

func (c *conn) SetReadDeadline(t time.Time) error {
	return errors.New("not supported")
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	return errors.New("not supported")
}
