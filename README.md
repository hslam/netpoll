# netpoll
Package netpoll implements a network poller base on epoll/kqueue.

## Features

* Epoll/Kqueue
* TCP/UNIX
* Fully compatible with the net.Conn
* Upgrade Connection
* Non-Blocking I/O
* Sync/Async Workers
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

import "github.com/hslam/netpoll"

func main() {
	var handler = &netpoll.DataHandler{
		Shared:     false,
		NoCopy:     true,
		BufferSize: 1024,
		HandlerFunc: func(req []byte) (res []byte) {
			res = req
			return
		},
	}
	if err := netpoll.ListenAndServe("tcp", ":9999", handler); err != nil {
		panic(err)
	}
}
```

### License
This package is licensed under a MIT license (Copyright (c) 2020 Meng Huang)


### Authors
netpoll was written by Meng Huang.


