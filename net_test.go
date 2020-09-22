// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

package netpoll

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestListenAndServe(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	err := ListenAndServe("", "", handler)
	if err == nil {
		t.Error("Unexpected")
	}
	network := "tcp"
	addr := ":8888"
	go ListenAndServe(network, addr, handler)
	time.Sleep(time.Millisecond * 10)
}

func TestServe(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	err := Serve(nil, handler)
	if err == nil {
		t.Error("Unexpected")
	}
	network := "tcp"
	addr := ":8889"
	l, _ := net.Listen(network, addr)
	go Serve(l, handler)
	time.Sleep(time.Millisecond * 10)
}

func TestNetServer(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     true,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &netServer{
		Handler: handler,
	}
	network := "tcp"
	addr := ":9999"
	l, _ := net.Listen(network, addr)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Serve(l); err != nil {
			t.Error(err)
		}
	}()
	conn, _ := net.Dial(network, addr)
	msg := "Hello World"
	msg = strings.Repeat(msg, 50)
	if n, err := conn.Write([]byte(msg)); err != nil {
		t.Error(err)
	} else if n != len(msg) {
		t.Error(n)
	}
	buf := make([]byte, len(msg))
	if n, err := conn.Read(buf); err != nil {
		t.Error(err)
	} else if n != len(msg) {
		t.Error(n)
	} else if string(buf) != msg {
		t.Error(string(buf))
	}
	conn.Close()
	server.Close()
	wg.Wait()
}

func TestNetServerUpgrade(t *testing.T) {
	var handler = &ConnHandler{}
	server := &netServer{
		Handler: handler,
	}
	network := "tcp"
	addr := ":9999"
	l, _ := net.Listen(network, addr)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Serve(l); err != nil {
			t.Error(err)
		}
	}()
	conn, _ := net.Dial(network, addr)
	time.Sleep(time.Millisecond * 10)
	conn.Close()
	server.Close()
	wg.Wait()
}
