package netpoll

import (
	"net"
	"os"
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
	server.Close()
	server.accept()
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
	for i := 0; i < 128; i++ {
		connWG.Add(1)
		go func() {
			defer connWG.Done()
			conn, _ := net.Dial(network, addr)
			msg := "Hello World"
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
