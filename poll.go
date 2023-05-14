// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// Package netpoll implements a network poller based on epoll/kqueue.
package netpoll

import (
	"errors"
)

// Mode represents the read/write mode.
type Mode int

const (
	// READ is the read mode.
	READ Mode = 1 << iota
	// WRITE is the write mode.
	WRITE
)

// Event represents the poll event for the poller.
type Event struct {
	// Fd is a file descriptor.
	Fd int
	// Mode represents the event mode.
	Mode Mode
}

// ErrTimeout is the error returned by SetTimeout when time.Duration d < 0.
var ErrTimeout = errors.New("negative interval for SetTimeout")
