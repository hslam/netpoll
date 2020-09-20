// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build !linux,!darwin,!dragonfly,!freebsd,!netbsd,!openbsd

package netpoll

import (
	"errors"
	"time"
)

// Tag is the poll type.
var Tag = "none"

// Poll represents the poll that supports non-blocking I/O on file descriptors with polling.
type Poll struct {
}

// Create creates a new poll.
func Create() (*Poll, error) {
	return nil, errors.New("system not supported")
}

// SetTimeout sets the wait timeout.
func (p *Poll) SetTimeout(d time.Duration) (err error) {
	return nil
}

// Register registers a file descriptor.
func (p *Poll) Register(fd int) (err error) {
	return
}

// Write adds a write event.
func (p *Poll) Write(fd int) (err error) {
	return
}

// Unregister unregisters a file descriptor.
func (p *Poll) Unregister(fd int) (err error) {
	return
}

// Wait waits events.
func (p *Poll) Wait(events []Event) (n int, err error) {
	return
}

// Close closes the poll fd. The underlying file descriptor is closed by the
// destroy method when there are no remaining references.
func (p *Poll) Close() error {
	return nil
}
