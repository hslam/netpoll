package netpoll

import (
	"net"
	"testing"
	"time"
)

func TestAssignPool(t *testing.T) {
	p := assignPool(1024)
	b := p.Get().([]byte)
	if len(b) < 1024 {
		t.Error(len(b))
	}
	assignPool(1024)
}

func TestNewHandler(t *testing.T) {
	var handler = NewHandler(func(conn net.Conn) (Context, error) {
		if n, err := conn.Write([]byte("")); err != nil {
			t.Error(err)
		} else if n != 0 {
			t.Error(n)
		}
		if n, err := conn.Write([]byte("12345678")); err == nil {
			t.Error("Unexpected")
		} else if n != 0 {
			t.Error(n)
		}
		if n, err := conn.Read([]byte("")); err != nil {
			t.Error(err)
		} else if n != 0 {
			t.Error(n)
		}
		conn.LocalAddr()
		conn.RemoteAddr()
		conn.SetDeadline(time.Now().Add(time.Second))
		conn.SetReadDeadline(time.Now().Add(time.Second))
		conn.SetWriteDeadline(time.Now().Add(time.Second))
		conn.Close()
		conn.Close()
		return conn, nil
	}, func(context Context) error {
		return nil
	})
	ctx, err := handler.Upgrade(&conn{})
	if err != nil {
		t.Error(err)
	}
	err = handler.Serve(ctx)
	if err != nil {
		t.Error(err)
	}
}

func TestConnHandler(t *testing.T) {
	var handler = &ConnHandler{}
	ctx, err := handler.Upgrade(&conn{})
	if err != ErrUpgradeFunc {
		t.Error(err)
	}
	err = handler.Serve(ctx)
	if err != ErrServeFunc {
		t.Error(err)
	}
	handler.SetUpgrade(func(conn net.Conn) (Context, error) {
		return conn, nil
	})
	handler.SetServe(func(context Context) error {
		return nil
	})
}

func TestDataHandler(t *testing.T) {
	var handler = &DataHandler{
		Shared:     false,
		NoCopy:     false,
		BufferSize: 0,
	}
	_, err := handler.Upgrade(&conn{})
	if err != ErrHandlerFunc {
		t.Error(err)
	}
}
