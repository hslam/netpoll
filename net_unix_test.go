// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

package netpoll

import (
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServerListenAndServe(t *testing.T) {
	network := "tcp"
	addr := ":8881"
	server := &Server{
		Network: network,
		Address: addr,
		NoAsync: false,
	}
	go server.ListenAndServe()
	time.Sleep(time.Millisecond * 10)
	server.Close()
	err := server.ListenAndServe()
	if err != ErrServerClosed {
		t.Error(err)
	}
}

func TestServerServe(t *testing.T) {
	network := "tcp"
	addr := ":8882"
	l, _ := net.Listen(network, addr)
	server := &Server{
		NoAsync: false,
	}
	err := server.Serve(nil)
	if err != ErrListener {
		t.Error(err)
	}
	err = server.Serve(l)
	if err != ErrHandler {
		t.Error(err)
	}
	server.Handler = nil
	server.Close()
	err = server.Serve(l)
	if err != ErrServerClosed {
		t.Error(err)
	}
}

func TestServerPoll(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler: handler,
		NoAsync: false,
	}
	network := "tcp"
	addr := ":9999"
	l, _ := net.Listen(network, addr)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Serve(l)
	}()
	time.Sleep(time.Millisecond * 10)
	server.accept()
	time.Sleep(time.Millisecond * 10)
	server.Close()
	time.Sleep(time.Millisecond * 10)
	server.accept()
	time.Sleep(time.Millisecond * 10)
	wg.Wait()
}

func TestServerClose(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler: handler,
		NoAsync: false,
	}
	network := "tcp"
	addr := ":9999"
	l, _ := net.Listen(network, addr)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Serve(l)
	}()
	time.Sleep(time.Millisecond * 10)
	server.Close()
	server.Close()
	wg.Wait()
}

func TestServerNumCPU(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	temp := numCPU
	numCPU = 17
	server := &Server{
		Handler: handler,
		NoAsync: false,
	}
	network := "tcp"
	addr := ":9999"
	l, _ := net.Listen(network, addr)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Serve(l)
	}()
	time.Sleep(time.Millisecond * 10)
	server.Close()
	wg.Wait()
	numCPU = temp
}

func TestServerTCPListener(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler: handler,
		NoAsync: false,
	}
	network := "tcp"
	addr := ":9999"
	l, _ := net.Listen(network, addr)
	time.Sleep(time.Millisecond * 10)
	l.Close()
	server.Serve(l)
	server.Close()
}

func TestServerUNIXListener(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler: handler,
		NoAsync: false,
	}
	network := "unix"
	addr := ":9999"
	os.Remove(addr)
	defer os.Remove(addr)
	l, _ := net.Listen(network, addr)
	l.Close()
	server.Serve(l)
}

func TestTCPServer(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	handler.SetUpgrade(func(c net.Conn) (net.Conn, error) {
		var u = &conn{}
		*u = *(c.(*conn))
		return u, nil
	})
	server := &Server{
		Handler: handler,
		NoAsync: false,
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

func TestUNIXServer(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler: handler,
		NoAsync: false,
	}
	network := "unix"
	addr := ":9999"
	os.Remove(addr)
	defer os.Remove(addr)
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

func TestOtherServer(t *testing.T) {
	type testListener struct {
		net.Listener
	}

	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler: handler,
		NoAsync: false,
	}
	network := "tcp"
	addr := ":9999"
	l, _ := net.Listen(network, addr)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Serve(&testListener{l}); err != nil {
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

func TestShared(t *testing.T) {
	var handler = &DataHandler{
		Shared:     true,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler: handler,
		NoAsync: false,
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

func TestNoCopy(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     true,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler: handler,
		NoAsync: false,
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

func TestServerUpgrade(t *testing.T) {
	var handler = &ConnHandler{}
	server := &Server{
		Handler: handler,
		NoAsync: false,
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

func TestWorker(t *testing.T) {
	var handler = &ConnHandler{}
	server := &Server{
		Handler: handler,
		NoAsync: false,
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
	time.Sleep(time.Millisecond * 10)
	server.workers[0].serve(Event{Fd: 0})
	server.workers[0].register(&conn{})
	server.workers[0].Close()
	server.Close()
	wg.Wait()
}

func TestNoAsync(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     true,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler: handler,
		NoAsync: true,
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
	time.Sleep(time.Millisecond * 500)
	conn.Close()
	server.Close()
	wg.Wait()
}

func TestSharedWorkers(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     true,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler:         handler,
		NoAsync:         true,
		UnsharedWorkers: 16,
		SharedWorkers:   numCPU,
		TasksPerWorker:  numCPU,
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
	time.Sleep(time.Millisecond * 500)
	conn.Close()
	server.Close()
	wg.Wait()
}

func TestSharedWorkersPanic(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     true,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler:         handler,
		NoAsync:         true,
		UnsharedWorkers: 16,
		SharedWorkers:   -1,
	}
	defer func() {
		if p := recover(); p == nil {
			t.Error()
		}
	}()
	if err := server.Serve(nil); err != nil {
		t.Error(err)
	}

}

func TestReschedule(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler: handler,
		NoAsync: false,
	}
	network := "tcp"
	addr := ":9999"
	l, _ := net.Listen(network, addr)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Serve(l)
	}()
	connWG := sync.WaitGroup{}
	for i := 0; i < 20; i++ {
		connWG.Add(1)
		go func() {
			defer connWG.Done()
			conn, _ := net.Dial(network, addr)
			msg := "Hello World"
			msg = strings.Repeat(msg, 50)
			when := time.Now().Add(time.Second)
			for time.Now().Before(when) {
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
			}
			conn.Close()
		}()
	}

	connWG.Wait()
	for i := 0; i < 512; i++ {
		go server.reschedule()
	}
	time.Sleep(time.Millisecond * 1010)
	server.Close()
	time.Sleep(time.Millisecond * 10)
	server.rescheduled = false
	server.reschedule()
	wg.Wait()
}

func TestRescheduleDone(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	server := &Server{
		Handler: handler,
		NoAsync: false,
	}
	network := "tcp"
	addr := ":9999"
	l, _ := net.Listen(network, addr)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Serve(l)
	}()
	connWG := sync.WaitGroup{}
	for i := 0; i < 32; i++ {
		connWG.Add(1)
		go func() {
			defer connWG.Done()
			conn, _ := net.Dial(network, addr)
			msg := "Hello World"
			msg = strings.Repeat(msg, 50)
			when := time.Now().Add(time.Second)
			if i > 20 {
				for j := 0; j < 4; j++ {
					for time.Now().Before(when) {
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
						time.Sleep(time.Millisecond)
					}
				}
			}
			for time.Now().Before(when) {
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
			}
			conn.Close()
		}()
	}

	connWG.Wait()
	server.Close()
	server.rescheduled = false
	server.wakeReschedule()
	go server.wakeReschedule()
	wg.Wait()
}

func TestRawConn(t *testing.T) {
	conn := &conn{fd: 1}
	rawConn, _ := conn.SyscallConn()
	rawConn.Control(func(fd uintptr) {
		if fd != 1 {
			t.Error(fd)
		}
	})
	rawConn.Read(func(fd uintptr) (done bool) {
		if fd != 1 {
			t.Error(fd)
		}
		return true
	})
	rawConn.Write(func(fd uintptr) (done bool) {
		if fd != 1 {
			t.Error(fd)
		}
		return true
	})
	conn.closed = 1
	rawConn.Control(func(fd uintptr) {
		if fd != 1 {
			t.Error(fd)
		}
	})
	rawConn.Read(func(fd uintptr) (done bool) {
		if fd != 1 {
			t.Error(fd)
		}
		return true
	})
	rawConn.Write(func(fd uintptr) (done bool) {
		if fd != 1 {
			t.Error(fd)
		}
		return true
	})
}

func TestSplice(t *testing.T) {
	var handler = &ConnHandler{}
	handler.SetUpgrade(func(conn net.Conn) (Context, error) {
		return conn, nil
	})
	handler.SetServe(func(context Context) error {
		conn := context.(net.Conn)
		_, err := io.Copy(conn, conn)
		return err
	})
	server := &Server{
		Handler: handler,
		NoAsync: false,
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

func TestReadFromLimitedReader(t *testing.T) {
	var handler = &ConnHandler{}
	handler.SetUpgrade(func(conn net.Conn) (Context, error) {
		return conn, nil
	})
	handler.SetServe(func(context Context) error {
		conn := context.(net.Conn)
		io.CopyN(conn, conn, 0)
		_, err := io.CopyN(conn, conn, bufferSize)
		return err
	})
	server := &Server{
		Handler: handler,
		NoAsync: false,
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

func TestGenericReadFrom(t *testing.T) {
	var handler = &ConnHandler{}
	handler.SetUpgrade(func(conn net.Conn) (Context, error) {
		return conn, nil
	})
	handler.SetServe(func(context Context) error {
		conn := context.(net.Conn)
		buf := make([]byte, bufferSize)
		r, w := io.Pipe()
		n, _ := conn.Read(buf)
		go w.Write(buf[:n])
		conn.(io.ReaderFrom).ReadFrom(r)
		return EAGAIN
	})
	server := &Server{
		Handler: handler,
		NoAsync: false,
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

func TestGenericReadFromRemain(t *testing.T) {
	genericReadFrom(nil, nil, -1)
	var handler = &ConnHandler{}
	handler.SetUpgrade(func(conn net.Conn) (Context, error) {
		return conn, nil
	})
	handler.SetServe(func(context Context) error {
		conn := context.(net.Conn)
		_, err := genericReadFrom(conn, conn, bufferSize+1)
		return err
	})
	server := &Server{
		Handler: handler,
		NoAsync: false,
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

func TestTopK(t *testing.T) {
	{
		l := list{&conn{score: 10}, &conn{score: 7}, &conn{score: 2}, &conn{score: 5}, &conn{score: 1}}
		k := 3
		topK(l, k)
		for j := k; j < l.Len(); j++ {
			for i := 0; i < k; i++ {
				if l.Less(i, j) {
					t.Error("top error")
				}
			}
		}
	}
	{
		l := list{&conn{score: 10}, &conn{score: 7}, &conn{score: 2}, &conn{score: 5}, &conn{score: 1}}
		k := 9
		topK(l, k)
		n := l.Len()
		for i := 1; i < n; i++ {
			if l.Less(i, 0) {
				t.Error("heap error")
			}
		}
	}
}
