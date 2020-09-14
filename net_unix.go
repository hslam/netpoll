// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build linux darwin dragonfly freebsd netbsd openbsd

package netpoll

import (
	"errors"
	"net"
	"os"
	"runtime"
	"sort"
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
	Listener     net.Listener
	Event        *Event
	file         *os.File
	fd           int
	poll         *Poll
	workers      []*worker
	rescheduled  bool
	lock         sync.Mutex
	wake         bool
	rescheduling int32
	list         list
	adjust       list
	syncWorkers  uint
	wg           sync.WaitGroup
	closed       int32
	done         chan struct{}
}

func (l *Listener) Serve() (err error) {
	if l.Listener == nil {
		return errors.New("listener is nil")
	}
	if l.Event == nil {
		return errors.New("event is nil")
	} else if l.Event.Handle == nil && l.Event.UpgradeHandle == nil {
		return errors.New("need Handle or UpgradeHandle")
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
	if numCPU > 16 {
		l.syncWorkers = uint(numCPU)
	}
	if l.poll, err = Create(); err != nil {
		return err
	}
	l.poll.Register(l.fd)
	if !l.Event.NoAsync && l.syncWorkers > 0 {
		l.rescheduled = true
	}
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
			jobs:     make(chan func()),
			tasks:    make(chan struct{}, numCPU),
		}
		if w.listener.Event.UpgradeHandle == nil {
			if w.async {
				w.pool = &sync.Pool{New: func() interface{} {
					return make([]byte, l.Event.Buffer)
				}}
			} else {
				w.buf = make([]byte, l.Event.Buffer)
			}
		}
		l.workers = append(l.workers, w)
	}
	l.done = make(chan struct{}, 1)
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

func (l *Listener) wakeReschedule() {
	if !l.rescheduled {
		return
	}
	l.lock.Lock()
	if !l.wake {
		l.wake = true
		l.lock.Unlock()
		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			for {
				select {
				case <-ticker.C:
					stop := l.reschedule()
					if stop {
						l.lock.Lock()
						l.wake = false
						ticker.Stop()
						l.lock.Unlock()
						return
					}
				case <-l.done:
					ticker.Stop()
					return
				}
			}
		}()
	} else {
		l.lock.Unlock()
	}
}

func (l *Listener) reschedule() (stop bool) {
	if !l.rescheduled {
		return
	}
	if !atomic.CompareAndSwapInt32(&l.rescheduling, 0, 1) {
		return false
	}
	defer atomic.StoreInt32(&l.rescheduling, 0)
	l.adjust = l.adjust[:0]
	l.list = l.list[:0]
	sum := int64(0)
	for idx, w := range l.workers {
		w.lock.Lock()
		running := w.running
		w.lock.Unlock()
		if !running {
			continue
		}
		w.mu.Lock()
		for _, conn := range w.conns {
			if uint(idx) < l.syncWorkers {
				l.adjust = append(l.adjust, conn)
			}
			conn.score = atomic.LoadInt64(&conn.count)
			atomic.StoreInt64(&conn.count, 0)
			sum += conn.score
			l.list = append(l.list, conn)
		}
		w.mu.Unlock()
	}
	if len(l.list) == 0 || sum == 0 {
		return true
	}
	sort.Sort(l.list)
	syncWorkers := l.syncWorkers
	if uint(len(l.list)) < l.syncWorkers {
		syncWorkers = uint(len(l.list))
	}
	index := 0
	for _, conn := range l.list[:syncWorkers] {
		conn.lock.Lock()
		if conn.w.async {
			conn.lock.Unlock()
			l.list[index] = conn
			index++
		} else {
			conn.lock.Unlock()
			if len(l.adjust) > 0 {
				for i := 0; i < len(l.adjust); i++ {
					if conn == l.adjust[i] {
						if i < len(l.adjust)-1 {
							copy(l.adjust[i:], l.adjust[i+1:])
						}
						l.adjust = l.adjust[:len(l.adjust)-1]
						break
					}
				}
			}
		}
	}
	var reschedules = l.list[:index]
	if len(reschedules) == 0 || len(reschedules) != len(l.adjust) {
		return false
	}
	for i := 0; i < len(reschedules); i++ {
		l.adjust[i].lock.Lock()
		reschedules[i].lock.Lock()
		syncWorker := l.adjust[i].w
		asyncWorker := reschedules[i].w
		syncWorker.mu.Lock()
		asyncWorker.mu.Lock()
		syncWorker.decrease(l.adjust[i])
		l.adjust[i].w = asyncWorker
		asyncWorker.increase(l.adjust[i])
		asyncWorker.decrease(reschedules[i])
		reschedules[i].w = syncWorker
		syncWorker.increase(reschedules[i])
		asyncWorker.mu.Unlock()
		syncWorker.mu.Unlock()
		l.adjust[i].lock.Unlock()
		reschedules[i].lock.Unlock()
	}
	return false
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
	close(l.done)
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
	pool     *sync.Pool
	buf      []byte
	handle   func(req []byte) (res []byte)
	async    bool
	jobs     chan func()
	tasks    chan struct{}
	done     chan struct{}
	lock     sync.Mutex
	running  bool
	slept    int32
	closed   int32
}

func (w *worker) task(job func()) {
	defer func() { <-w.tasks }()
	for {
		job()
		t := time.NewTimer(time.Second)
		runtime.Gosched()
		select {
		case job = <-w.jobs:
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

func (w *worker) Sleep() {
	if !atomic.CompareAndSwapInt32(&w.slept, 0, 1) {
		return
	}
	close(w.done)
}

func (w *worker) run(wg *sync.WaitGroup) {
	defer wg.Done()
	var n int
	var err error
	for err == nil {
		n, err = w.poll.Wait(w.events)
		for i := range w.events[:n] {
			ev := w.events[i]
			if w.async {
				job := func() {
					w.serve(ev)
				}
				select {
				case w.jobs <- job:
				case w.tasks <- struct{}{}:
					go w.task(job)
				default:
					go job()
				}
			} else {
				w.serve(ev)
			}
		}
		if n > 0 && w.async {
			w.listener.wakeReschedule()
		}
		if atomic.LoadInt64(&w.count) < 1 {
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
			if c.handle != nil {
				w.handleConn(c)
			} else {
				w.read(c)
			}
		}
		return nil
	}
}

func (w *worker) write(c *conn) error {
	c.writing.Lock()
	retain, err := c.flush()
	c.writing.Unlock()
	if err != nil {
		return err
	} else if retain > 0 {
		w.poll.Write(c.fd)
	}
	return nil
}

func (w *worker) handleConn(c *conn) error {
	for {
		err := c.handle()
		if err != nil {
			if err == syscall.EAGAIN {
				return nil
			}
			if !atomic.CompareAndSwapInt32(&c.closing, 0, 1) {
				return nil
			}
			w.decrease(c)
			c.Close()
			return nil
		}
	}
	return nil
}

func (w *worker) read(c *conn) error {
	for {
		var n int
		var err error
		var buf []byte
		var req []byte
		if w.async {
			buf = w.pool.Get().([]byte)
			buf = buf[:cap(buf)]
			defer w.pool.Put(buf)
		} else {
			buf = w.buf
		}
		c.reading.Lock()
		if c.upgrade != nil {
			n, err = c.upgrade.Read(buf)
		} else {
			n, err = c.Read(buf)
		}
		c.reading.Unlock()
		if n == 0 || err != nil {
			if err == syscall.EAGAIN {
				return nil
			}
			if !atomic.CompareAndSwapInt32(&c.closing, 0, 1) {
				return nil
			}
			w.Decrease(c)
			c.Close()
			return nil
		}
		req = buf[:n]
		if w.async {
			req = make([]byte, n)
			copy(req, buf[:n])
		} else {
			if !w.listener.Event.NoCopy {
				req = make([]byte, n)
				copy(req, buf[:n])
			}
		}
		w.do(c, req)
	}
	return nil
}

func (w *worker) do(c *conn, req []byte) {
	res := w.handle(req)
	c.writing.Lock()
	defer c.writing.Unlock()
	if c.upgrade != nil {
		c.upgrade.Write(res)
	} else {
		c.Write(res)
	}
}

func (w *worker) register(c *conn) error {
	w.Increase(c)
	if w.listener.Event.UpgradeConn != nil || w.listener.Event.UpgradeHandle != nil {
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
			if w.listener.Event.UpgradeConn != nil {
				if upgrade, err := w.listener.Event.UpgradeConn(c); err != nil {
					return
				} else if upgrade != nil && upgrade != c {
					c.upgrade = upgrade
				}
			}
			if w.listener.Event.UpgradeHandle != nil {
				if handle, err := w.listener.Event.UpgradeHandle(c); err != nil {
					return
				} else {
					c.handle = handle
				}
			}
			if err := syscall.SetNonblock(c.fd, true); err != nil {
				return
			}
			atomic.StoreInt32(&c.ready, 1)
			c.sendRetain()
		}(w, c)
		return nil
	}
	atomic.StoreInt32(&c.ready, 1)
	c.sendRetain()
	return nil
}

func (w *worker) Increase(c *conn) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.increase(c)
}

func (w *worker) increase(c *conn) {
	w.conns[c.fd] = c
	atomic.AddInt64(&w.count, 1)
	w.poll.Register(c.fd)
}

func (w *worker) Decrease(c *conn) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.decrease(c)
}

func (w *worker) decrease(c *conn) {
	w.poll.Unregister(c.fd)
	atomic.AddInt64(&w.count, -1)
	delete(w.conns, c.fd)
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
	lock     sync.Mutex
	w        *worker
	reading  sync.Mutex
	writing  sync.Mutex
	rlock    sync.Mutex
	wlock    sync.Mutex
	fd       int
	send     []byte
	laddr    net.Addr
	raddr    net.Addr
	upgrade  net.Conn
	upgraded int32
	handle   func() error
	ready    int32
	count    int64
	score    int64
	closing  int32
	closed   int32
}

func (c *conn) Read(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}
	c.lock.Lock()
	if c.w.listener.rescheduled {
		c.lock.Unlock()
		atomic.AddInt64(&c.count, 1)
	} else {
		c.lock.Unlock()
	}
	c.rlock.Lock()
	defer c.rlock.Unlock()
	n, err = syscall.Read(c.fd, b)
	if n == 0 {
		err = EOF
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
	c.wlock.Lock()
	defer c.wlock.Unlock()
	if atomic.LoadInt32(&c.ready) == 1 {
		if retain, err := c.flush(); err != nil || retain > 0 {
			c.write(b)
			return len(b), err
		}
	}
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
	defer func() {
		c.lock.Lock()
		c.w.poll.Write(c.fd)
		c.lock.Unlock()
	}()
	c.send = append(c.send, b...)
	return len(b), nil
}

func (c *conn) flush() (retain int, err error) {
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

func (c *conn) sendRetain() {
	c.lock.Lock()
	c.wlock.Lock()
	if len(c.send) > 0 {
		c.w.poll.Write(c.fd)
	}
	c.wlock.Unlock()
	c.lock.Unlock()
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

type list []*conn

func (l list) Len() int {
	return len(l)
}

func (l list) Less(i, j int) bool {
	if atomic.LoadInt64(&l[i].score) > atomic.LoadInt64(&l[j].score) {
		return true
	} else {
		return false
	}
}

func (l list) Swap(i, j int) {
	var temp = l[i]
	l[i] = l[j]
	l[j] = temp
}
