// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build linux

package poll

import (
	"sync"
	"syscall"
)

var Tag = "epoll"

type Poll struct {
	fd      int
	events  []syscall.EpollEvent
	pool    *sync.Pool
	timeout int
}

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
		timeout: 1000,
	}, nil
}

func (p *Poll) Register(fd int) (err error) {
	event := p.pool.Get().(syscall.EpollEvent)
	defer p.pool.Put(event)
	event.Fd, event.Events = int32(fd), syscall.EPOLLIN
	return syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_ADD, fd, &event)
}

func (p *Poll) Write(fd int) (err error) {
	event := p.pool.Get().(syscall.EpollEvent)
	defer p.pool.Put(event)
	event.Fd, event.Events = int32(fd), syscall.EPOLLIN|syscall.EPOLLOUT
	return syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_MOD, fd, &event)
}

func (p *Poll) Unregister(fd int) (err error) {
	event := p.pool.Get().(syscall.EpollEvent)
	defer p.pool.Put(event)
	event.Fd, event.Events = int32(fd), syscall.EPOLLIN|syscall.EPOLLOUT
	return syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_DEL, fd, &event)
}

func (p *Poll) Wait(events []PollEvent) (n int, err error) {
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

func (p *Poll) Close() error {
	return syscall.Close(p.fd)
}
