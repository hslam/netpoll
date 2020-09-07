// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd netbsd openbsd

package poll

import (
	"sync"
	"syscall"
)

type Poll struct {
	fd     int
	events []syscall.Kevent_t
	pool   *sync.Pool
}

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
	}, nil
}

func (p *Poll) Register(fd int) (err error) {
	changes := p.pool.Get().([]syscall.Kevent_t)
	defer p.pool.Put(changes)
	changes[0].Ident, changes[0].Flags = uint64(fd), syscall.EV_ADD
	_, err = syscall.Kevent(p.fd, changes[:1], nil, nil)
	return
}

func (p *Poll) Write(fd int) (err error) {
	changes := p.pool.Get().([]syscall.Kevent_t)
	defer p.pool.Put(changes)
	changes[1].Ident, changes[1].Flags = uint64(fd), syscall.EV_ADD
	_, err = syscall.Kevent(p.fd, changes[1:], nil, nil)
	return
}

func (p *Poll) Unregister(fd int) (err error) {
	changes := p.pool.Get().([]syscall.Kevent_t)
	defer p.pool.Put(changes)
	changes[0].Ident, changes[0].Flags = uint64(fd), syscall.EV_DELETE
	changes[1].Ident, changes[1].Flags = uint64(fd), syscall.EV_DELETE
	_, err = syscall.Kevent(p.fd, changes, nil, nil)
	return
}

func (p *Poll) Wait(events []PollEvent) (n int, err error) {
	if cap(p.events) >= len(events) {
		p.events = p.events[:len(events)]
	} else {
		p.events = make([]syscall.Kevent_t, len(events))
	}
	n, err = syscall.Kevent(p.fd, nil, p.events, nil)
	if err != nil && err != syscall.EINTR {
		return 0, err
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

func (p *Poll) Close() error {
	return syscall.Close(p.fd)
}
