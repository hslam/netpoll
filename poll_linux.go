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
		event:  syscall.EpollEvent{Events: syscall.EPOLLIN},
	}, nil
}

func (p *Poll) Add(fd int) (err error) {
	return p.ctl(fd,syscall.EPOLL_CTL_ADD)
}

func (p *Poll) Delete(fd int) (err error) {
	return p.ctl(fd,syscall.EPOLL_CTL_DEL)
}

func (p *Poll) ctl(fd int, op int) error {
	p.event.Fd = int32(fd)
	return syscall.EpollCtl(p.fd, op, fd, &p.event)
}

func (p *Poll) Wait(events []int) (n int, err error) {
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
		events[i] = int(p.events[i].Fd)
	}
	return
}

func (p *Poll) Close() error {
	return syscall.Close(p.fd)
}
