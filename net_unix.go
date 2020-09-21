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

// Server defines parameters for running a server.
type Server struct {
	Network string
	Address string
	// Handler responds to a single request.
	Handler Handler
	// NoAsync disables async workers.
	NoAsync      bool
	listener     net.Listener
	netServer    *netServer
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
	if atomic.LoadInt32(&s.closed) != 0 {
		return ErrServerClosed
	}
	s.listener = l
	if s.listener == nil {
		return errors.New("Listener is nil")
	} else if s.Handler == nil {
		return errors.New("Handler is nil")
	}
	switch netListener := s.listener.(type) {
	case *net.TCPListener:
		if s.file, err = netListener.File(); err != nil {
			s.listener.Close()
			return err
		}
	case *net.UnixListener:
		if s.file, err = netListener.File(); err != nil {
			s.listener.Close()
			return err
		}
	default:
		s.netServer = &netServer{Handler: s.Handler}
		return s.netServer.Serve(l)
	}
	s.fd = int(s.file.Fd())
	if err := syscall.SetNonblock(s.fd, true); err != nil {
		s.listener.Close()
		return err
	}
	s.syncWorkers = 16
	if numCPU > 16 {
		s.syncWorkers = uint(numCPU)
	}
	if s.poll, err = Create(); err != nil {
		return err
	}
	s.poll.Register(s.fd)
	if !s.NoAsync && s.syncWorkers > 0 {
		s.rescheduled = true
	}
	for i := 0; i < int(s.syncWorkers)+numCPU; i++ {
		p, err := Create()
		if err != nil {
			return err
		}
		var async bool
		if i >= int(s.syncWorkers) && !s.NoAsync {
			async = true
		}
		w := &worker{
			index:  i,
			server: s,
			conns:  make(map[int]*conn),
			poll:   p,
			events: make([]Event, 0x400),
			async:  async,
			done:   make(chan struct{}, 1),
			jobs:   make(chan func()),
			tasks:  make(chan struct{}, numCPU),
		}
		s.workers = append(s.workers, w)
	}
	s.done = make(chan struct{}, 1)
	var n int
	var events = make([]Event, 1)
	for err == nil {
		if n, err = s.poll.Wait(events); n > 0 {
			if events[0].Fd == s.fd {
				err = s.accept()
			}
			runtime.Gosched()
		}
	}
	s.wg.Wait()
	return nil
}

func (s *Server) accept() (err error) {
	nfd, sa, err := syscall.Accept(s.fd)
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
	w := s.assignWorker()
	w.Wake(&s.wg)
	return w.register(&conn{w: w, fd: nfd, raddr: raddr, laddr: s.listener.Addr()})
}

func (s *Server) assignWorker() (w *worker) {
	if w := s.idleSyncWorker(); w != nil {
		return w
	}
	return s.leastConnectedAsyncWorker()
}

func (s *Server) idleSyncWorker() (w *worker) {
	if s.syncWorkers > 0 {
		for i := 0; i < int(s.syncWorkers); i++ {
			if s.workers[i].count < 1 {
				return s.workers[i]
			}
		}
	}
	return nil
}

func (s *Server) leastConnectedAsyncWorker() (w *worker) {
	min := s.workers[s.syncWorkers].count
	index := int(s.syncWorkers)
	if len(s.workers) > int(s.syncWorkers) {
		for i := int(s.syncWorkers) + 1; i < len(s.workers); i++ {
			if s.workers[i].count < min {
				min = s.workers[i].count
				index = i
			}
		}
	}
	return s.workers[index]
}

func (s *Server) wakeReschedule() {
	if !s.rescheduled {
		return
	}
	s.lock.Lock()
	if !s.wake {
		s.wake = true
		s.lock.Unlock()
		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			for {
				select {
				case <-ticker.C:
					stop := s.reschedule()
					if stop {
						s.lock.Lock()
						s.wake = false
						ticker.Stop()
						s.lock.Unlock()
						return
					}
				case <-s.done:
					ticker.Stop()
					return
				}
			}
		}()
	} else {
		s.lock.Unlock()
	}
}

func (s *Server) reschedule() (stop bool) {
	if !s.rescheduled {
		return
	}
	if !atomic.CompareAndSwapInt32(&s.rescheduling, 0, 1) {
		return false
	}
	defer atomic.StoreInt32(&s.rescheduling, 0)
	s.adjust = s.adjust[:0]
	s.list = s.list[:0]
	sum := int64(0)
	for idx, w := range s.workers {
		w.lock.Lock()
		running := w.running
		w.lock.Unlock()
		if !running {
			continue
		}
		w.mu.Lock()
		for _, conn := range w.conns {
			if uint(idx) < s.syncWorkers {
				s.adjust = append(s.adjust, conn)
			}
			conn.score = atomic.LoadInt64(&conn.count)
			atomic.StoreInt64(&conn.count, 0)
			sum += conn.score
			s.list = append(s.list, conn)
		}
		w.mu.Unlock()
	}
	if len(s.list) == 0 || sum == 0 {
		return true
	}
	sort.Sort(s.list)
	syncWorkers := s.syncWorkers
	if uint(len(s.list)) < s.syncWorkers {
		syncWorkers = uint(len(s.list))
	}
	index := 0
	for _, conn := range s.list[:syncWorkers] {
		conn.lock.Lock()
		if conn.w.async {
			conn.lock.Unlock()
			s.list[index] = conn
			index++
		} else {
			conn.lock.Unlock()
			if len(s.adjust) > 0 {
				for i := 0; i < len(s.adjust); i++ {
					if conn == s.adjust[i] {
						if i < len(s.adjust)-1 {
							copy(s.adjust[i:], s.adjust[i+1:])
						}
						s.adjust = s.adjust[:len(s.adjust)-1]
						break
					}
				}
			}
		}
	}
	var reschedules = s.list[:index]
	if len(reschedules) == 0 || len(reschedules) != len(s.adjust) {
		return false
	}
	for i := 0; i < len(reschedules); i++ {
		s.adjust[i].lock.Lock()
		reschedules[i].lock.Lock()
		syncWorker := s.adjust[i].w
		asyncWorker := reschedules[i].w
		syncWorker.mu.Lock()
		asyncWorker.mu.Lock()
		syncWorker.decrease(s.adjust[i])
		s.adjust[i].w = asyncWorker
		asyncWorker.increase(s.adjust[i])
		asyncWorker.decrease(reschedules[i])
		reschedules[i].w = syncWorker
		syncWorker.increase(reschedules[i])
		asyncWorker.mu.Unlock()
		syncWorker.mu.Unlock()
		s.adjust[i].lock.Unlock()
		reschedules[i].lock.Unlock()
	}
	return false
}

// Close closes the listener.
func (s *Server) Close() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return nil
	}
	if s.netServer != nil {
		return s.netServer.Close()
	}
	for i := 0; i < len(s.workers); i++ {
		s.workers[i].Close()
	}
	if err := s.listener.Close(); err != nil {
		return err
	}
	if err := s.file.Close(); err != nil {
		return err
	}
	close(s.done)
	return s.poll.Close()
}

type worker struct {
	index   int
	server  *Server
	count   int64
	mu      sync.Mutex
	conns   map[int]*conn
	poll    *Poll
	events  []Event
	async   bool
	jobs    chan func()
	tasks   chan struct{}
	done    chan struct{}
	lock    sync.Mutex
	running bool
	slept   int32
	closed  int32
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
			w.server.wakeReschedule()
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

func (w *worker) serve(ev Event) error {
	fd := ev.Fd
	if fd == 0 {
		return nil
	}
	w.mu.Lock()
	c, ok := w.conns[fd]
	if !ok {
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()
	if atomic.LoadInt32(&c.ready) == 0 {
		return nil
	}
	switch ev.Mode {
	case WRITE:
	case READ:
		w.serveConn(c)
	}
	return nil
}

func (w *worker) serveConn(c *conn) error {
	for {
		err := w.server.Handler.Serve(c.context)
		if err != nil {
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
	}
}

func (w *worker) register(c *conn) error {
	w.Increase(c)
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
		ctx, err := w.server.Handler.Upgrade(c)
		if err != nil {
			w.Decrease(c)
			c.Close()
			return
		}
		c.context = ctx
		if err := syscall.SetNonblock(c.fd, true); err != nil {
			return
		}
		atomic.StoreInt32(&c.ready, 1)
	}(w, c)
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
	rlock    sync.Mutex
	wlock    sync.Mutex
	fd       int
	laddr    net.Addr
	raddr    net.Addr
	upgraded int32
	context  Context
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
	if c.w.server.rescheduled {
		c.lock.Unlock()
		atomic.AddInt64(&c.count, 1)
	} else {
		c.lock.Unlock()
	}
	c.rlock.Lock()
	defer c.rlock.Unlock()
	n, err = syscall.Read(c.fd, b)
	if n == 0 || err == syscall.EBADF || err == syscall.ECONNRESET {
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
	var retain = len(b)
	for retain > 0 {
		n, err = syscall.Write(c.fd, b[len(b)-retain:])
		if n < 1 || err != nil {
			return len(b) - retain, err
		}
		retain -= n
	}
	return len(b), nil
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
	}
	return false
}

func (l list) Swap(i, j int) {
	var temp = l[i]
	l[i] = l[j]
	l[j] = temp
}
