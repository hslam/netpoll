// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build linux darwin dragonfly freebsd netbsd openbsd

package netpoll

import (
	"errors"
	"github.com/hslam/sendfile"
	"github.com/hslam/splice"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	idleTime = time.Second
)

var numCPU = runtime.NumCPU()

// Server defines parameters for running a server.
type Server struct {
	Network string
	Address string
	// Handler responds to a single request.
	Handler Handler
	// NoAsync disables async.
	NoAsync         bool
	UnsharedWorkers int
	SharedWorkers   int
	TasksPerWorker  int
	addr            net.Addr
	netServer       *netServer
	file            *os.File
	fd              int
	poll            *Poll
	workers         []*worker
	rescheduled     bool
	lock            sync.Mutex
	wake            bool
	rescheduling    int32
	list            list
	adjust          list
	unsharedWorkers uint
	sharedWorkers   uint
	tasksPerWorker  uint
	wg              sync.WaitGroup
	closed          int32
	done            chan struct{}
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
// The handler must be not nil.
//
// Serve always returns a non-nil error.
// After Close the returned error is ErrServerClosed.
func (s *Server) Serve(l net.Listener) (err error) {
	if atomic.LoadInt32(&s.closed) != 0 {
		return ErrServerClosed
	}
	if s.UnsharedWorkers == 0 {
		s.unsharedWorkers = 16
	} else if s.UnsharedWorkers > 0 {
		s.unsharedWorkers = uint(s.UnsharedWorkers)
	}
	if s.SharedWorkers == 0 {
		s.sharedWorkers = uint(numCPU)
	} else if s.SharedWorkers > 0 {
		s.sharedWorkers = uint(s.SharedWorkers)
	} else {
		panic("SharedWorkers < 0")
	}
	if s.TasksPerWorker == 0 {
		s.tasksPerWorker = uint(numCPU)
	} else if s.TasksPerWorker > 0 {
		s.tasksPerWorker = uint(s.TasksPerWorker)
	}
	if l == nil {
		return ErrListener
	} else if s.Handler == nil {
		return ErrHandler
	}
	switch netListener := l.(type) {
	case *net.TCPListener:
		if s.file, err = netListener.File(); err != nil {
			l.Close()
			return err
		}
	case *net.UnixListener:
		if s.file, err = netListener.File(); err != nil {
			l.Close()
			return err
		}
	default:
		s.netServer = &netServer{Handler: s.Handler}
		return s.netServer.Serve(l)
	}
	s.fd = int(s.file.Fd())
	s.addr = l.Addr()
	l.Close()
	if err := syscall.SetNonblock(s.fd, true); err != nil {
		return err
	}
	if s.poll, err = Create(); err != nil {
		return err
	}
	s.poll.Register(s.fd)
	if !s.NoAsync && s.unsharedWorkers > 0 {
		s.rescheduled = true
	}
	for i := 0; i < int(s.unsharedWorkers+s.sharedWorkers); i++ {
		p, err := Create()
		if err != nil {
			return err
		}
		var async bool
		if i >= int(s.unsharedWorkers) && !s.NoAsync {
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
			tasks:  make(chan struct{}, s.tasksPerWorker),
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
			s.wakeReschedule()
		}
		runtime.Gosched()
	}
	s.wg.Wait()
	return err
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
	return w.register(&conn{w: w, fd: nfd, raddr: raddr, laddr: s.addr})
}

func (s *Server) assignWorker() (w *worker) {
	if w := s.idleUnsharedWorkers(); w != nil {
		return w
	}
	return s.leastConnectedSharedWorkers()
}

func (s *Server) idleUnsharedWorkers() (w *worker) {
	if s.unsharedWorkers > 0 {
		for i := 0; i < int(s.unsharedWorkers); i++ {
			if s.workers[i].count < 1 {
				return s.workers[i]
			}
		}
	}
	return nil
}

func (s *Server) leastConnectedSharedWorkers() (w *worker) {
	min := s.workers[s.unsharedWorkers].count
	index := int(s.unsharedWorkers)
	if len(s.workers) > int(s.unsharedWorkers) {
		for i := int(s.unsharedWorkers) + 1; i < len(s.workers); i++ {
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
					s.lock.Lock()
					stop := s.reschedule()
					if stop {
						s.wake = false
						ticker.Stop()
						s.lock.Unlock()
						return
					}
					s.lock.Unlock()
				case <-s.done:
					ticker.Stop()
					return
				}
				runtime.Gosched()
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
		if !w.running {
			w.lock.Unlock()
			continue
		}
		for _, conn := range w.conns {
			if uint(idx) < s.unsharedWorkers {
				s.adjust = append(s.adjust, conn)
			}
			conn.score = atomic.LoadInt64(&conn.count)
			atomic.StoreInt64(&conn.count, 0)
			sum += conn.score
			s.list = append(s.list, conn)
		}
		w.lock.Unlock()
	}
	if len(s.list) == 0 || sum == 0 {
		return true
	}
	unsharedWorkers := s.unsharedWorkers
	if uint(len(s.list)) < s.unsharedWorkers {
		unsharedWorkers = uint(len(s.list))
	}
	topK(s.list, int(unsharedWorkers))
	index := 0
	for _, conn := range s.list[:unsharedWorkers] {
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
		if atomic.LoadInt32(&s.adjust[i].ready) == 0 || atomic.LoadInt32(&reschedules[i].ready) == 0 {
			continue
		}
		s.adjust[i].lock.Lock()
		reschedules[i].lock.Lock()
		unsharedWorker := s.adjust[i].w
		sharedWorker := reschedules[i].w
		unsharedWorker.lock.Lock()
		sharedWorker.lock.Lock()
		unsharedWorker.decrease(s.adjust[i])
		s.adjust[i].w = sharedWorker
		sharedWorker.increase(s.adjust[i])
		sharedWorker.decrease(reschedules[i])
		reschedules[i].w = unsharedWorker
		unsharedWorker.increase(reschedules[i])
		sharedWorker.lock.Unlock()
		unsharedWorker.lock.Unlock()
		s.adjust[i].lock.Unlock()
		reschedules[i].lock.Unlock()
	}
	return false
}

// Close closes the server.
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
	if err := s.file.Close(); err != nil {
		return err
	}
	if s.done != nil {
		close(s.done)
	}
	return s.poll.Close()
}

type worker struct {
	index    int
	server   *Server
	count    int64
	lock     sync.Mutex
	conns    map[int]*conn
	lastIdle time.Time
	poll     *Poll
	events   []Event
	async    bool
	jobs     chan func()
	tasks    chan struct{}
	done     chan struct{}
	running  bool
	slept    int32
	closed   int32
}

func (w *worker) task(job func()) {
	defer func() { <-w.tasks }()
	for {
		job()
		t := time.NewTimer(idleTime)
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

func (w *worker) run(wg *sync.WaitGroup) {
	defer wg.Done()
	var n int
	var err error
	for err == nil {
		n, err = w.poll.Wait(w.events)
		if n > 0 {
			for i := range w.events[:n] {
				ev := w.events[i]
				if w.async {
					wg.Add(1)
					job := func() {
						w.serve(ev)
						wg.Done()
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
		}
		if atomic.LoadInt64(&w.count) < 1 {
			w.lock.Lock()
			if len(w.conns) == 0 && w.lastIdle.Add(idleTime).Before(time.Now()) {
				w.sleep()
				w.running = false
				w.lock.Unlock()
				return
			}
			w.lock.Unlock()
		}
		runtime.Gosched()
	}
}

func (w *worker) serve(ev Event) error {
	fd := ev.Fd
	if fd == 0 {
		return nil
	}
	w.lock.Lock()
	c, ok := w.conns[fd]
	if !ok {
		w.lock.Unlock()
		return nil
	}
	w.lock.Unlock()
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
				runtime.Gosched()
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
	go func(w *worker, c *conn) {
		var err error
		defer func() {
			if err != nil {
				w.Decrease(c)
				c.Close()
			}
		}()
		if err = syscall.SetNonblock(c.fd, false); err != nil {
			return
		}
		if c.context, err = w.server.Handler.Upgrade(c); err != nil {
			return
		}
		if err = syscall.SetNonblock(c.fd, true); err != nil {
			return
		}
		atomic.StoreInt32(&c.ready, 1)
		w.serveConn(c)
	}(w, c)
	return nil
}

func (w *worker) Increase(c *conn) {
	w.lock.Lock()
	w.increase(c)
	w.wake(&w.server.wg)
	w.lock.Unlock()
}

func (w *worker) increase(c *conn) {
	w.conns[c.fd] = c
	atomic.AddInt64(&w.count, 1)
	w.poll.Register(c.fd)
}

func (w *worker) Decrease(c *conn) {
	w.lock.Lock()
	w.decrease(c)
	if atomic.LoadInt64(&w.count) < 1 {
		w.lastIdle = time.Now()
	}
	w.lock.Unlock()
}

func (w *worker) decrease(c *conn) {
	w.poll.Unregister(c.fd)
	atomic.AddInt64(&w.count, -1)
	delete(w.conns, c.fd)
}

func (w *worker) wake(wg *sync.WaitGroup) {
	if !w.running {
		w.running = true
		w.done = make(chan struct{}, 1)
		atomic.StoreInt32(&w.slept, 0)
		wg.Add(1)
		go w.run(wg)
	}
}

func (w *worker) sleep() {
	if !atomic.CompareAndSwapInt32(&w.slept, 0, 1) {
		return
	}
	close(w.done)
}

func (w *worker) Close() {
	if !atomic.CompareAndSwapInt32(&w.closed, 0, 1) {
		return
	}
	w.lock.Lock()
	for _, c := range w.conns {
		c.Close()
		delete(w.conns, c.fd)
	}
	w.sleep()
	w.poll.Close()
	w.lock.Unlock()
}

type conn struct {
	lock    sync.Mutex
	w       *worker
	rlock   sync.Mutex
	wlock   sync.Mutex
	fd      int
	laddr   net.Addr
	raddr   net.Addr
	context Context
	ready   int32
	count   int64
	score   int64
	closing int32
	closed  int32
}

// Read reads data from the connection.
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
	n, err = syscall.Read(c.fd, b)
	c.rlock.Unlock()
	if err != nil && err != syscall.EAGAIN || err == nil && n == 0 {
		err = EOF
	}
	if n < 0 {
		n = 0
	}
	return
}

// Write writes data to the connection.
func (c *conn) Write(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}
	var retain = len(b)
	c.wlock.Lock()
	for retain > 0 {
		n, err = syscall.Write(c.fd, b[len(b)-retain:])
		if n > 0 {
			retain -= n
			continue
		}
		if err != syscall.EAGAIN {
			c.wlock.Unlock()
			return len(b) - retain, EOF
		}
	}
	c.wlock.Unlock()
	return len(b), nil
}

// Close closes the connection.
func (c *conn) Close() (err error) {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return
	}
	return syscall.Close(c.fd)
}

// LocalAddr returns the local network address.
func (c *conn) LocalAddr() net.Addr {
	return c.laddr
}

// RemoteAddr returns the remote network address.
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

func (c *conn) ok() bool { return c != nil && c.fd > 0 && atomic.LoadInt32(&c.closed) == 0 }

// SyscallConn returns a raw network connection.
// This implements the syscall.Conn interface.
func (c *conn) SyscallConn() (syscall.RawConn, error) {
	return &rawConn{uintptr(c.fd), c}, nil
}

// ReadFrom implements the io.ReaderFrom ReadFrom method.
func (c *conn) ReadFrom(r io.Reader) (int64, error) {
	var remain int64
	if lr, ok := r.(*io.LimitedReader); ok {
		remain, r = lr.N, lr.R
		if remain <= 0 {
			return 0, nil
		}
	}
	if syscallConn, ok := r.(syscall.Conn); ok {
		if src, ok := r.(net.Conn); ok {
			if remain <= 0 {
				remain = bufferSize
			}
			var n int64
			var err error
			n, err = splice.Splice(c, src, remain)
			if err != splice.ErrNotHandled {
				return n, err
			}
		}
		if raw, err := syscallConn.SyscallConn(); err == nil {
			var src int
			raw.Control(func(fd uintptr) {
				src = int(fd)
			})
			if pos, err := syscall.Seek(src, 0, io.SeekCurrent); err == nil {
				size, _ := syscall.Seek(src, 0, io.SeekEnd)
				syscall.Seek(src, pos, io.SeekStart)
				if remain <= 0 || remain > size-pos {
					remain = size - pos
				}
				if remain <= 0 {
					return 0, nil
				}
				return sendfile.SendFile(c, src, pos, remain)
			}
		}
	}
	return genericReadFrom(c, r, remain)
}

func genericReadFrom(w io.Writer, r io.Reader, remain int64) (n int64, err error) {
	if remain < 0 {
		return
	}
	if remain == 0 {
		remain = bufferSize
	} else if remain > bufferSize {
		remain = bufferSize
	}
	pool := assignPool(int(remain))
	buf := pool.Get().([]byte)
	defer pool.Put(buf)
	var retain int
	retain, err = r.Read(buf)
	if err != nil {
		return 0, err
	}
	var out int
	var pos int
	for retain > 0 {
		out, err = w.Write(buf[pos : pos+retain])
		if out > 0 {
			retain -= out
			n += int64(out)
			pos += out
			continue
		}
		if err != syscall.EAGAIN {
			return n, EOF
		}
	}
	return n, nil
}

type rawConn struct {
	fd uintptr
	c  *conn
}

func (c *rawConn) Control(f func(fd uintptr)) error {
	if !c.c.ok() {
		return syscall.EINVAL
	}
	f(c.fd)
	return nil
}

func (c *rawConn) Read(f func(fd uintptr) (done bool)) error {
	if !c.c.ok() {
		return syscall.EINVAL
	}
	f(c.fd)
	return nil
}

func (c *rawConn) Write(f func(fd uintptr) (done bool)) error {
	if !c.c.ok() {
		return syscall.EINVAL
	}
	f(c.fd)
	return nil
}

type list []*conn

func (l list) Len() int { return len(l) }
func (l list) Less(i, j int) bool {
	return atomic.LoadInt64(&l[i].score) < atomic.LoadInt64(&l[j].score)
}
func (l list) Swap(i, j int) { l[i], l[j] = l[j], l[i] }

func topK(h list, k int) {
	n := h.Len()
	if k > n {
		k = n
	}
	for i := k/2 - 1; i >= 0; i-- {
		heapDown(h, i, k)
	}
	if k < n {
		for i := k; i < n; i++ {
			if h.Less(0, i) {
				h.Swap(0, i)
				heapDown(h, 0, k)
			}
		}
	}
}

func heapDown(h list, i, n int) bool {
	parent := i
	for {
		leftChild := 2*parent + 1
		if leftChild >= n || leftChild < 0 { // leftChild < 0 after int overflow
			break
		}
		lessChild := leftChild
		if rightChild := leftChild + 1; rightChild < n && h.Less(rightChild, leftChild) {
			lessChild = rightChild
		}
		if !h.Less(lessChild, parent) {
			break
		}
		h.Swap(parent, lessChild)
		parent = lessChild
	}
	return parent > i
}
