// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build linux

package netpoll

import (
	"errors"
	"sync"
	"syscall"
	"time"
)

// Tag is the poll type.
var Tag = "epoll"

// Poll represents the poll that supports non-blocking I/O on file descriptors with polling.
type Poll struct {
	fd      int
	events  []syscall.EpollEvent
	pool    *sync.Pool
	timeout int
}

// Create creates a new poll.
func Create() (*Poll, error) {
	fd, err := syscall.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	return &Poll{
		fd:     fd,
		events: make([]syscall.EpollEvent, 1024),
		pool: &sync.Pool{New: func() interface{} {
			return syscall.EpollEvent{}
		}},
		timeout: 60000,
	}, nil
}

// SetTimeout sets the wait timeout.
func (p *Poll) SetTimeout(d time.Duration) (err error) {
	if d < time.Millisecond {
		return errors.New("non-positive interval for SetTimeout")
	}
	p.timeout = int(d / time.Millisecond)
	return nil
}

// Register registers a file descriptor.
func (p *Poll) Register(fd int) (err error) {
	event := p.pool.Get().(syscall.EpollEvent)
	defer p.pool.Put(event)
	event.Fd, event.Events = int32(fd), syscall.EPOLLIN
	return syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_ADD, fd, &event)
}

// Write adds a write event.
func (p *Poll) Write(fd int) (err error) {
	event := p.pool.Get().(syscall.EpollEvent)
	defer p.pool.Put(event)
	event.Fd, event.Events = int32(fd), syscall.EPOLLIN|syscall.EPOLLOUT
	return syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_MOD, fd, &event)
}

// Unregister unregisters a file descriptor.
func (p *Poll) Unregister(fd int) (err error) {
	event := p.pool.Get().(syscall.EpollEvent)
	defer p.pool.Put(event)
	event.Fd, event.Events = int32(fd), syscall.EPOLLIN|syscall.EPOLLOUT
	return syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_DEL, fd, &event)
}

// Wait waits events.
func (p *Poll) Wait(events []Event) (n int, err error) {
	if cap(p.events) >= len(events) {
		p.events = p.events[:len(events)]
	} else {
		p.events = make([]syscall.EpollEvent, len(events))
	}
	n, err = syscall.EpollWait(p.fd, p.events, p.timeout)
	if err != nil && err != syscall.EINTR {
		return 0, err
	}
	for i := 0; i < n; i++ {
		ev := p.events[i]
		events[i].Fd = int(ev.Fd)
		if ev.Events&syscall.EPOLLIN != 0 {
			events[i].Mode = READ
		} else if ev.Events&syscall.EPOLLOUT != 0 {
			events[i].Mode = WRITE
			event := p.pool.Get().(syscall.EpollEvent)
			event.Fd, event.Events = ev.Fd, syscall.EPOLLIN
			syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_MOD, int(ev.Fd), &event)
			p.pool.Put(event)
		}
	}
	return
}

// Close closes the poll fd. The underlying file descriptor is closed by the
// destroy method when there are no remaining references.
func (p *Poll) Close() error {
	return syscall.Close(p.fd)
}
