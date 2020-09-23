// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

package netpoll

import (
	"net"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestPoll(t *testing.T) {
	p, err := Create()
	defer p.Close()
	if err != nil {
		t.Error(err)
	}
	err = p.SetTimeout(0)
	if err != ErrTimeout {
		t.Error(err)
	}
	err = p.SetTimeout(time.Millisecond * 1)
	if err != nil {
		t.Error(err)
	}

	if n, err := p.Wait(make([]Event, len(p.events)+1)); err != nil {
		t.Error(err)
	} else if n != 0 {
		t.Error(n)
	}
	network := "tcp"
	addr := ":9999"
	l, _ := net.Listen(network, addr)
	ln, _ := l.(*net.TCPListener)
	f, _ := ln.File()
	epfd := int(f.Fd())
	if err := syscall.SetNonblock(epfd, false); err != nil {
		panic(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, _ := net.Dial(network, addr)
		for {
			conn.Read(make([]byte, 64))
			break
		}
	}()
	fd, _, err := syscall.Accept(epfd)
	if err != nil {
		t.Error(err)
	}
	p.Register(fd)
	err = p.Write(fd)
	if err != nil {
		t.Error(err)
	}
	events := make([]Event, 128)
	if n, err := p.Wait(events); err != nil {
		t.Error(err)
	} else if n != 1 {
		t.Error(n)
	} else if events[0].Mode != WRITE {
		t.Error(events)
	}
	p.Unregister(fd)
	syscall.Close(fd)
	f.Close()
	l.Close()
	wg.Wait()
}
