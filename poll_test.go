// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

package netpoll

import (
	"os"
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
	tmpfile := "tmppollfile"
	file, _ := os.Create(tmpfile)
	defer os.Remove(tmpfile)
	fd := int(file.Fd())
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
}
