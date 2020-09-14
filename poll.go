// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// Package netpoll implements network poller base on epoll/kqueue.
package netpoll

type Mode int

const (
	READ Mode = 1 << iota
	WRITE
)

type PollEvent struct {
	Fd   int
	Mode Mode
}
