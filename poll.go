// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// Package poll supports non-blocking I/O on file descriptors with polling.
package poll

type Mode int

const (
	READ Mode = 1 << iota
	WRITE
)

type PollEvent struct {
	Fd   int
	Mode Mode
}
