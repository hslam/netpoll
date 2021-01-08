// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd netbsd openbsd

package netpoll

import (
	"errors"
	"sync"
	"syscall"
	"time"
)

// Tag is the poll type.
var Tag = "kqueue"

// ErrTimeout is the error returned by SetTimeout when time.Duration d < time.Millisecond.
var ErrTimeout = errors.New("non-positive interval for SetTimeout")

// Poll represents the poll that supports non-blocking I/O on file descriptors with polling.
type Poll struct {
	fd      int
	events  []syscall.Kevent_t
	pool    *sync.Pool
	timeout *syscall.Timespec
}

// Create creates a new poll.
func Create() (*Poll, error) {
	fd, err := syscall.Kqueue()
	if err != nil {
		return nil, err
	}
	return &Poll{
		fd:     fd,
		events: make([]syscall.Kevent_t, 1024),
		pool: &sync.Pool{New: func() interface{} {
			return []syscall.Kevent_t{{Filter: syscall.EVFILT_READ}, {Filter: syscall.EVFILT_WRITE}}
		}},
		timeout: &syscall.Timespec{Sec: 1},
	}, nil
}

// SetTimeout sets the wait timeout.
func (p *Poll) SetTimeout(d time.Duration) (err error) {
	if d < time.Millisecond {
		return ErrTimeout
	}
	p.timeout.Sec = int64(d / time.Second)
	p.timeout.Nsec = int64(d % time.Second)
	return nil
}

// Register registers a file descriptor.
func (p *Poll) Register(fd int) (err error) {
	changes := p.pool.Get().([]syscall.Kevent_t)
	changes[0].Ident, changes[0].Flags = uint64(fd), syscall.EV_ADD
	_, err = syscall.Kevent(p.fd, changes[:1], nil, nil)
	p.pool.Put(changes)
	return
}

// Write adds a write event.
func (p *Poll) Write(fd int) (err error) {
	changes := p.pool.Get().([]syscall.Kevent_t)
	changes[1].Ident, changes[1].Flags = uint64(fd), syscall.EV_ADD
	_, err = syscall.Kevent(p.fd, changes[1:], nil, nil)
	p.pool.Put(changes)
	return
}

// Unregister unregisters a file descriptor.
func (p *Poll) Unregister(fd int) (err error) {
	changes := p.pool.Get().([]syscall.Kevent_t)
	changes[0].Ident, changes[0].Flags = uint64(fd), syscall.EV_DELETE
	changes[1].Ident, changes[1].Flags = uint64(fd), syscall.EV_DELETE
	_, err = syscall.Kevent(p.fd, changes, nil, nil)
	p.pool.Put(changes)
	return
}

// Wait waits events.
func (p *Poll) Wait(events []Event) (n int, err error) {
	if cap(p.events) >= len(events) {
		p.events = p.events[:len(events)]
	} else {
		p.events = make([]syscall.Kevent_t, len(events))
	}
	n, err = syscall.Kevent(p.fd, nil, p.events, p.timeout)
	if err != nil {
		if err != syscall.EINTR {
			return 0, err
		}
		err = nil
	}
	for i := 0; i < n; i++ {
		ev := p.events[i]
		events[i].Fd = int(ev.Ident)
		switch ev.Filter {
		case syscall.EVFILT_READ:
			events[i].Mode = READ
		case syscall.EVFILT_WRITE:
			events[i].Mode = WRITE
			changes := p.pool.Get().([]syscall.Kevent_t)
			changes[1].Ident, changes[1].Flags = ev.Ident, syscall.EV_DELETE
			syscall.Kevent(p.fd, changes[1:], nil, nil)
			p.pool.Put(changes)
		}
	}
	return
}

// Close closes the poll fd. The underlying file descriptor is closed by the
// destroy method when there are no remaining references.
func (p *Poll) Close() error {
	return syscall.Close(p.fd)
}
