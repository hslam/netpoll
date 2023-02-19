// Copyright (c) 2023 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

//go:build darwin || dragonfly || freebsd || netbsd || openbsd || linux
// +build darwin dragonfly freebsd netbsd openbsd linux

package netpoll

import (
	"github.com/hslam/reuse"
	"net"
	"sync"
	"testing"
	"time"
)

func TestReuseServerPort(t *testing.T) {
	network := "tcp"
	addr := ":9999"
	msg := "Hello World"
	wg := sync.WaitGroup{}
	servers := make([]*Server, 2)
	var handler = &DataHandler{
		NoShared:   false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	for i := 0; i < 2; i++ {
		server := &Server{
			Network: network,
			Address: addr,
			Handler: handler,
			NoAsync: false,
		}
		servers[i] = server
		wg.Add(1)
		go func() {
			defer wg.Done()
			server.ListenAndServe()
		}()
	}
	time.Sleep(time.Millisecond * 100)
	localPort := 8888
	d := net.Dialer{LocalAddr: &net.TCPAddr{Port: localPort}, Control: reuse.Control}
	conn, err := d.Dial(network, addr)
	if err != nil {
		t.Error("dial failed:", err)
		return
	}
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Error(err)
		return
	}
	buf := make([]byte, 1024)
	if n, err := conn.Read(buf); err != nil {
		t.Error(err)
		return
	} else if n != len(msg) {
		t.Errorf("%d %d", n, len(msg))
	}
	conn.Close()
	for i := 0; i < 2; i++ {
		servers[i].Close()
	}
	wg.Wait()
}

func TestReuseClientPort(t *testing.T) {
	network := "tcp"
	addr1 := ":9997"
	addr2 := ":9998"
	msg := "Hello World"
	wg := sync.WaitGroup{}
	var handler = &DataHandler{
		NoShared:   false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server1 := &Server{
		Network: network,
		Address: addr1,
		Handler: handler,
		NoAsync: false,
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		server1.ListenAndServe()
	}()
	server2 := &Server{
		Network: network,
		Address: addr2,
		Handler: handler,
		NoAsync: false,
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		server2.ListenAndServe()
	}()
	time.Sleep(time.Millisecond * 100)
	localPort := 8888
	d := net.Dialer{LocalAddr: &net.TCPAddr{Port: localPort}, Control: reuse.Control}
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := d.Dial(network, addr1)
		time.Sleep(time.Millisecond * 100)
		if err != nil {
			t.Error("dial failed:", err)
			return
		}
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Error(err)
			return
		}
		buf := make([]byte, 1024)
		if n, err := conn.Read(buf); err != nil {
			t.Error(err)
			return
		} else if n != len(msg) {
			t.Errorf("%d %d", n, len(msg))
		}
		time.Sleep(time.Millisecond * 500)
		conn.Close()
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := d.Dial(network, addr2)
		time.Sleep(time.Millisecond * 100)
		if err != nil {
			t.Error("dial failed:", err)
			return
		}
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Error(err)
			return
		}
		buf := make([]byte, 1024)
		if n, err := conn.Read(buf); err != nil {
			t.Error(err)
			return
		} else if n != len(msg) {
			t.Errorf("%d %d", n, len(msg))
		}
		time.Sleep(time.Millisecond * 500)
		conn.Close()
	}()
	time.Sleep(time.Second)
	server1.Close()
	server2.Close()
	wg.Wait()
}
