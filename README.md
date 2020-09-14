# netpoll
Package netpoll implements network poller base on epoll/kqueue.

## Features

* Epoll/Kqueue
* TCP/UNIX
* Compatible With The net.Conn
* Upgrade Connection
* Non-Blocking I/O
* Async Handle
* Rescheduling Workers

## Get started

### Install
```
go get github.com/hslam/netpoll
```
### Import
```
import "github.com/hslam/netpoll"
```
### Usage
#### Example
```
package main

import (
	"github.com/hslam/netpoll"
	"net"
)

func main() {
	lis, err := net.Listen("tcp", ":9999")
	if err != nil {
		panic(err)
	}
	netpoll.Serve(lis, &netpoll.Event{
		Handle: func(req []byte) (res []byte) {
			res = req
			return
		},
	})
}
```

### License
This package is licensed under a MIT license (Copyright (c) 2020 Meng Huang)


### Authors
netpoll was written by Meng Huang.


