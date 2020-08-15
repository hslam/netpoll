// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd netbsd openbsd

package poll

import (
	"syscall"
)

type Poll struct {
	fd      int
	events  []syscall.Kevent_t
	changes []syscall.Kevent_t
}

func Create() (*Poll, error) {
	fd, err := syscall.Kqueue()
	if err != nil {
		return nil, err
	}
	return &Poll{
		fd:      fd,
		events:  make([]syscall.Kevent_t, 1024),
		changes: []syscall.Kevent_t{{Filter: syscall.EVFILT_READ}},
	}, nil
}

func (p *Poll) Add(fd int) (err error) {
	return p.kevent(fd, syscall.EV_ADD)
}

func (p *Poll) Delete(fd int) (err error) {
	return p.kevent(fd, syscall.EV_DELETE)
}

func (p *Poll) kevent(fd int, op uint16) (err error) {
	p.changes[0].Ident = uint64(fd)
	p.changes[0].Flags = op
	_, err = syscall.Kevent(p.fd, p.changes, nil, nil)
	return
}

func (p *Poll) Wait(events []int) (n int, err error) {
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
		events[i] = int(p.events[i].Ident)
	}
	return
}

func (p *Poll) Close() error {
	return syscall.Close(p.fd)
}
