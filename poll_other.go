// Copyright (c) 2020 Meng Huang (mhboy@outlook.com)
// This package is licensed under a MIT license that can be found in the LICENSE file.

// +build !linux,!darwin,!dragonfly,!freebsd,!netbsd,!openbsd

package poll

import (
	"errors"
)

type Poll struct {
}

func Create() (*Poll, error) {
	return nil, errors.New("system not supported")
}

func (p *Poll) Add(fd int) (err error) {
	return
}

func (p *Poll) Delete(fd int) (err error) {
	return
}

func (p *Poll) Wait(events []int) (n int, err error) {
	return
}

func (p *Poll) Close() error {
	return nil
}
