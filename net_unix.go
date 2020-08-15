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
)

func Serve(lis net.Listener, handle func(req []byte) (res []byte)) error {
	l := &Listener{netListener: lis}
	return l.Serve(handle)
}

type Listener struct {
	netListener net.Listener
	file        *os.File
	fd          int
	workers     []*worker
}

func (l *Listener) Serve(handle func(req []byte) (res []byte)) error {
	var err error
	switch netListener := l.netListener.(type) {
	case *net.TCPListener:
		if l.file, err = netListener.File(); err != nil {
			l.netListener.Close()
			return err
		}
	case *net.UnixListener:
		if l.file, err = netListener.File(); err != nil {
			l.netListener.Close()
			return err
		}
	default:
		return errors.New("listener not supported")
	}
	l.fd = int(l.file.Fd())
	if err := syscall.SetNonblock(l.fd, true); err != nil {
		l.netListener.Close()
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
			events:   make([]int, 0x400),
			handle:   handle,
			done:     make(chan bool, 0x10),
			tasks:    runtime.NumCPU() * 4,
		}
		w.jobs = make(chan *conn, w.tasks)
		w.poll.Add(l.fd)
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
	events   []int
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
				buf := make([]byte, 0xFFFF)
				for fd := range w.jobs {
					atomic.AddInt64(&w.idle, -1)
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

func (w *worker) serve(fd int) error {
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
		if atomic.LoadInt64(&w.count) > 1 {
			if atomic.LoadInt64(&w.idle) > 0 {
				w.jobs <- c
			} else {
				go w.read(c, []byte{})
			}
			return nil
		} else {
			return w.read(c, nil)
		}
	}
}

func (w *worker) accept() error {
	if atomic.LoadInt64(&w.count) < 1 || atomic.LoadInt64(&w.count) <= w.listener.min() {
		nfd, _, err := syscall.Accept(w.listener.fd)
		if err != nil {
			if err == syscall.EAGAIN {
				return nil
			}
			return err
		}
		if err := syscall.SetNonblock(nfd, true); err != nil {
			return err
		}
		c := &conn{fd: nfd, buf: make([]byte, 0xFFFF)}
		w.poll.Add(nfd)
		w.increase(c)
	}
	return nil
}

func (w *worker) read(c *conn, buf []byte) error {
	n, err := c.Read(c.buf)
	if n == 0 || err != nil {
		if err == syscall.EAGAIN {
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
	c.Write(res)
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

type conn struct {
	rMu sync.Mutex
	wMu sync.Mutex
	fd  int
	buf []byte
}

func (c *conn) Read(p []byte) (n int, err error) {
	c.rMu.Lock()
	defer c.rMu.Unlock()
	return syscall.Read(c.fd, p)
}

func (c *conn) Write(p []byte) (n int, err error) {
	c.wMu.Lock()
	defer c.wMu.Unlock()
	return syscall.Write(c.fd, p)
}

func (c *conn) Close() (err error) {
	return syscall.Close(c.fd)
}
