// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build linux

package poll

import (
	"syscall"
)

type Poll struct {
	fd     int
	events []syscall.EpollEvent
	event  syscall.EpollEvent
}

func Create() (*Poll, error) {
	fd, err := syscall.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	return &Poll{
		fd:     fd,
		events: make([]syscall.EpollEvent, 1024),
		event:  syscall.EpollEvent{},
	}, nil
}

func (p *Poll) Register(fd int) (err error) {
	p.event.Fd, p.event.Events = int32(fd), syscall.EPOLLIN
	return syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_ADD, fd, &p.event)
}

func (p *Poll) Write(fd int) (err error) {
	p.event.Fd, p.event.Events = int32(fd), syscall.EPOLLIN|syscall.EPOLLOUT
	return syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_MOD, fd, &p.event)
}

func (p *Poll) Unregister(fd int) (err error) {
	p.event.Fd, p.event.Events = int32(fd), syscall.EPOLLIN|syscall.EPOLLOUT
	return syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_DEL, fd, &p.event)
}

func (p *Poll) Wait(events []PollEvent) (n int, err error) {
	if cap(p.events) >= len(events) {
		p.events = p.events[:len(events)]
	} else {
		p.events = make([]syscall.EpollEvent, len(events))
	}
	n, err = syscall.EpollWait(p.fd, p.events, -1)
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
			p.event.Fd, p.event.Events = ev.Fd, syscall.EPOLLIN
			syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_MOD, int(ev.Fd), &p.event)
		}
	}
	return
}

func (p *Poll) Close() error {
	return syscall.Close(p.fd)
}
