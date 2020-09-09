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

var numCPU = runtime.NumCPU()

func Serve(lis net.Listener, event *Event) error {
	l := &Listener{Listener: lis, Event: event}
	return l.Serve()
}

type Listener struct {
	Listener    net.Listener
	Event       *Event
	file        *os.File
	fd          int
	poll        *Poll
	workers     []*worker
	syncWorkers uint
	wg          sync.WaitGroup
	closed      int32
}

func (l *Listener) Serve() (err error) {
	if l.Listener == nil {
		return errors.New("listener is nil")
	}
	if l.Event == nil {
		return errors.New("event is nil")
	} else if l.Event.Handle == nil {
		return errors.New("Handle is nil")
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
		listener := &listener{Listener: l.Listener, Event: l.Event}
		return listener.Serve()
	}
	l.fd = int(l.file.Fd())
	if err := syscall.SetNonblock(l.fd, true); err != nil {
		l.Listener.Close()
		return err
	}
	l.syncWorkers = 16
	if l.poll, err = Create(); err != nil {
		return err
	}
	l.poll.Register(l.fd)
	for i := 0; i < int(l.syncWorkers)+numCPU; i++ {
		p, err := Create()
		if err != nil {
			return err
		}
		var async bool
		if i >= int(l.syncWorkers) && !l.Event.NoAsync {
			async = true
		}
		w := &worker{
			index:    i,
			listener: l,
			conns:    make(map[int]*conn),
			poll:     p,
			events:   make([]PollEvent, 0x400),
			handle:   l.Event.Handle,
			async:    async,
			done:     make(chan struct{}, 1),
			jobs:     make(chan *job),
			tasks:    make(chan struct{}, numCPU),
		}
		if w.async {
			w.pool = &sync.Pool{New: func() interface{} {
				return make([]byte, l.Event.Buffer)
			}}
		} else {
			w.buf = make([]byte, l.Event.Buffer)
		}
		l.workers = append(l.workers, w)
	}
	var n int
	var events = make([]PollEvent, 1)
	for err == nil {
		if n, err = l.poll.Wait(events); n > 0 {
			if events[0].Fd == l.fd {
				err = l.accept()
			}
			runtime.Gosched()
		}
	}
	l.wg.Wait()
	return nil
}

func (l *Listener) accept() (err error) {
	nfd, sa, err := syscall.Accept(l.fd)
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
	w := l.assignWorker()
	w.Wake(&l.wg)
	return w.register(&conn{w: w, fd: nfd, raddr: raddr, laddr: l.Listener.Addr()})
}

func (l *Listener) assignWorker() (w *worker) {
	if l.syncWorkers > 0 {
		for i := 0; i < int(l.syncWorkers); i++ {
			if l.workers[i].count < 1 {
				return l.workers[i]
			}
		}
	}
	return l.leastConnectedWorker()
}

func (l *Listener) leastConnectedWorker() (w *worker) {
	min := l.workers[l.syncWorkers].count
	index := int(l.syncWorkers)
	if len(l.workers) > int(l.syncWorkers) {
		for i := int(l.syncWorkers) + 1; i < len(l.workers); i++ {
			if l.workers[i].count < min {
				min = l.workers[i].count
				index = i
			}
		}
	}
	return l.workers[index]
}

func (l *Listener) Close() error {
	if !atomic.CompareAndSwapInt32(&l.closed, 0, 1) {
		return nil
	}
	for i := 0; i < len(l.workers); i++ {
		l.workers[i].Close()
	}
	if err := l.Listener.Close(); err != nil {
		return err
	}
	if err := l.file.Close(); err != nil {
		return err
	}
	return l.poll.Close()
}

type worker struct {
	index    int
	listener *Listener
	count    int64
	mu       sync.Mutex
	conns    map[int]*conn
	poll     *Poll
	events   []PollEvent
	wg       sync.WaitGroup
	pool     *sync.Pool
	buf      []byte
	handle   func(req []byte) (res []byte)
	async    bool
	jobs     chan *job
	tasks    chan struct{}
	done     chan struct{}
	lock     sync.Mutex
	running  bool
	slept    int32
	closed   int32
}

type job struct {
	conn *conn
	req  []byte
}

func (w *worker) task(j *job) {
	defer func() { <-w.tasks }()
	for {
		w.do(j.conn, j.req)
		t := time.NewTimer(time.Second)
		runtime.Gosched()
		select {
		case j = <-w.jobs:
			t.Stop()
		case <-t.C:
			return
		case <-w.done:
			return
		}
	}
}
func (w *worker) Wake(wg *sync.WaitGroup) {
	w.lock.Lock()
	if !w.running {
		w.running = true
		w.done = make(chan struct{}, 1)
		atomic.StoreInt32(&w.slept, 0)
		w.lock.Unlock()
		wg.Add(1)
		go w.run(wg)
	} else {
		w.lock.Unlock()
	}
}
func (w *worker) run(wg *sync.WaitGroup) {
	defer wg.Done()
	var n int
	var err error
	for err == nil {
		n, err = w.poll.Wait(w.events)
		if w.async {
			for i := 0; i < n; i++ {
				w.wg.Add(1)
				go w.serve(w.events[i])
			}
			w.wg.Wait()
		} else {
			for i := 0; i < n; i++ {
				w.serve(w.events[i])
			}
		}
		if atomic.LoadInt64(&w.count) == 0 {
			w.lock.Lock()
			w.Sleep()
			w.running = false
			w.lock.Unlock()
			return
		}
		runtime.Gosched()
	}
}

func (w *worker) serve(ev PollEvent) error {
	if w.async {
		defer w.wg.Done()
	}
	fd := ev.Fd
	if fd == 0 {
		return nil
	}
	w.mu.Lock()
	if c, ok := w.conns[fd]; !ok {
		w.mu.Unlock()
		return nil
	} else {
		w.mu.Unlock()
		if atomic.LoadInt32(&c.ready) == 0 {
			return nil
		}
		switch ev.Mode {
		case WRITE:
			w.write(c)
		case READ:
			w.read(c)
		}
		return nil
	}
}

func (w *worker) write(c *conn) error {
	if retain, err := c.flush(); err != nil {
		return err
	} else if retain > 0 {
		w.poll.Write(c.fd)
	}
	return nil
}

func (w *worker) read(c *conn) error {
	for {
		var n int
		var err error
		var buf []byte
		var msg []byte
		var req []byte
		if c.messages != nil {
			msg, err = c.messages.ReadMessage()
			n = len(msg)
		} else {
			if w.async {
				buf = w.pool.Get().([]byte)
				buf = buf[:cap(buf)]
				defer w.pool.Put(buf)
			} else {
				buf = w.buf
			}
			if c.upgrade != nil {
				n, err = c.upgrade.Read(buf)
			} else {
				n, err = c.Read(buf)
			}
		}
		if n == 0 || err != nil {
			if err == syscall.EAGAIN || atomic.LoadInt32(&c.closed) > 0 {
				return nil
			}
			w.decrease(c)
			c.Close()
			return nil
		}
		if c.messages == nil {
			msg = buf[:n]
		}
		req = msg
		if w.async {
			req = make([]byte, n)
			copy(req, msg)
			select {
			case w.jobs <- &job{c, req}:
			case w.tasks <- struct{}{}:
				go w.task(&job{c, req})
			default:
				go w.do(c, req)
			}
		} else {
			if !w.listener.Event.NoCopy {
				req = make([]byte, n)
				copy(req, msg)
			}
			w.do(c, req)
		}
		if c.upgrade == nil && c.messages == nil {
			break
		}
	}
	return nil
}

func (w *worker) do(c *conn, req []byte) {
	res := w.handle(req)
	if c.messages != nil {
		c.sending.Lock()
		c.messages.WriteMessage(res)
		c.sending.Unlock()
	} else if c.upgrade != nil {
		c.sending.Lock()
		c.upgrade.Write(res)
		c.sending.Unlock()
	} else {
		c.Write(res)
	}
}

func (w *worker) register(c *conn) error {
	w.increase(c)
	if w.listener.Event.Upgrade != nil {
		if !atomic.CompareAndSwapInt32(&c.upgraded, 0, 1) {
			return nil
		}
		go func(w *worker, c *conn) {
			defer func() {
				if e := recover(); e != nil {
				}
			}()
			if err := syscall.SetNonblock(c.fd, false); err != nil {
				return
			}
			if upgrade, m, err := w.listener.Event.Upgrade(c); err != nil {
				return
			} else {
				if m != nil {
					c.messages = m
					if w.listener.Event.Batch != nil {
						c.messages.SetBatch(w.listener.Event.Batch)
					}
				}
				if upgrade != nil && upgrade != c {
					c.upgrade = upgrade
				}
			}
			if err := syscall.SetNonblock(c.fd, true); err != nil {
				return
			}
			atomic.StoreInt32(&c.ready, 1)
		}(w, c)
		return nil
	}
	atomic.StoreInt32(&c.ready, 1)
	return nil
}

func (w *worker) increase(c *conn) {
	w.mu.Lock()
	w.conns[c.fd] = c
	w.mu.Unlock()
	atomic.AddInt64(&w.count, 1)
	w.poll.Register(c.fd)
}

func (w *worker) decrease(c *conn) {
	w.poll.Unregister(c.fd)
	atomic.AddInt64(&w.count, -1)
	w.mu.Lock()
	delete(w.conns, c.fd)
	w.mu.Unlock()
}

func (w *worker) Sleep() {
	if !atomic.CompareAndSwapInt32(&w.slept, 0, 1) {
		return
	}
	close(w.done)
}

func (w *worker) Close() {
	if !atomic.CompareAndSwapInt32(&w.closed, 0, 1) {
		return
	}
	w.mu.Lock()
	for _, c := range w.conns {
		c.Close()
	}
	w.mu.Unlock()
	w.Sleep()
	w.poll.Close()
}

type conn struct {
	w        *worker
	sending  sync.Mutex
	rMu      sync.Mutex
	wMu      sync.Mutex
	fd       int
	send     []byte
	laddr    net.Addr
	raddr    net.Addr
	upgrade  net.Conn
	upgraded int32
	messages Messages
	ready    int32
	closed   int32
}

func (c *conn) Read(b []byte) (n int, err error) {
	c.rMu.Lock()
	defer c.rMu.Unlock()
	n, err = syscall.Read(c.fd, b)
	if n == 0 {
		err = syscall.EINVAL
	}
	if n < 0 {
		n = 0
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
		if n < 1 || err != nil {
			if atomic.LoadInt32(&c.ready) == 0 {
				return len(b) - retain, err
			} else {
				c.write(b[len(b)-retain:])
				break
			}
		}
		retain -= n
	}
	return len(b), nil
}

func (c *conn) write(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}
	defer c.w.poll.Write(c.fd)
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
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return
	}
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
