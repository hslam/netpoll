# netpoll
[![PkgGoDev](https://pkg.go.dev/badge/github.com/hslam/netpoll)](https://pkg.go.dev/github.com/hslam/netpoll)
[![Build Status](https://travis-ci.org/hslam/netpoll.svg?branch=master)](https://travis-ci.org/hslam/netpoll)
[![codecov](https://codecov.io/gh/hslam/netpoll/branch/master/graph/badge.svg)](https://codecov.io/gh/hslam/netpoll)
[![Go Report Card](https://goreportcard.com/badge/github.com/hslam/netpoll)](https://goreportcard.com/report/github.com/hslam/netpoll)
[![LICENSE](https://img.shields.io/github/license/hslam/netpoll.svg?style=flat-square)](https://github.com/hslam/netpoll/blob/master/LICENSE)

Package netpoll implements a network poller based on epoll/kqueue.

## Features

* Epoll/kqueue
* TCP/UNIX
* Compatible with the net.Conn interface
* Upgrade connection
* Non-blocking I/O
* [Splice](https://github.com/hslam/splice "splice")/[sendfile](https://github.com/hslam/sendfile "sendfile")
* Sync/async workers
* Rescheduling workers

**Comparison to other packages.**

|Package| [net](https://github.com/golang/go/tree/master/src/net "net")| [netpoll](https://github.com/hslam/netpoll "netpoll")|[gnet](https://github.com/panjf2000/gnet "gnet")|[evio](https://github.com/tidwall/evio "evio")|
|:--:|:--|:--|:--|:--|
|Sync handler|Yes|Yes|Yes|Yes|
|Async handler|Yes|Yes|Yes|Yes|
|Low memory usage|No|Yes|Yes|Yes|
|Non-blocking I/O|No|Yes|Yes|Yes|
|Splice/sendfile|Yes|Yes|No|No|
|Rescheduling|Yes|Yes|No|No|
|Compatible with the net.Conn interface|Yes|Yes|No|No|

## [Benchmark](http://github.com/hslam/netpoll-benchmark "netpoll-benchmark")

<img src="https://raw.githubusercontent.com/hslam/netpoll-benchmark/master/netpoll-qps.png" width = "400" height = "300" alt="mock 0ms" align=center>
<img src="https://raw.githubusercontent.com/hslam/netpoll-benchmark/master/netpoll-mock-time-qps.png" width = "400" height = "300" alt="mock 1ms" align=center>

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
#### Simple Example
```go
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
#### [Websocket](http://github.com/hslam/websocket "websocket") Example
```go
package main

import (
	"github.com/hslam/netpoll"
	"github.com/hslam/websocket"
	"net"
)

func main() {
	var handler = &netpoll.ConnHandler{}
	handler.SetUpgrade(func(conn net.Conn) (netpoll.Context, error) {
		return websocket.Upgrade(conn, nil)
	})
	handler.SetServe(func(context netpoll.Context) error {
		ws := context.(*websocket.Conn)
		msg, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		return ws.WriteMessage(msg)
	})
	if err := netpoll.ListenAndServe("tcp", ":9999", handler); err != nil {
		panic(err)
	}
}
```


#### [HTTP](http://github.com/hslam/rum "rum") Example
```go
package main

import (
	"bufio"
	"github.com/hslam/mux"
	"github.com/hslam/netpoll"
	"github.com/hslam/response"
	"net"
	"net/http"
	"sync"
)

func main() {
	m := mux.New()
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello World"))
	})
	ListenAndServe(":8080", m)
}

func ListenAndServe(addr string, handler http.Handler) error {
	var h = &netpoll.ConnHandler{}
	type Context struct {
		reader  *bufio.Reader
		rw      *bufio.ReadWriter
		conn    net.Conn
		serving sync.Mutex
	}
	h.SetUpgrade(func(conn net.Conn) (netpoll.Context, error) {
		reader := bufio.NewReader(conn)
		rw := bufio.NewReadWriter(reader, bufio.NewWriter(conn))
		return &Context{reader: reader, conn: conn, rw: rw}, nil
	})
	h.SetServe(func(context netpoll.Context) error {
		ctx := context.(*Context)
		ctx.serving.Lock()
		req, err := http.ReadRequest(ctx.reader)
		if err != nil {
			ctx.serving.Unlock()
			return err
		}
		res := response.NewResponse(req, ctx.conn, ctx.rw)
		handler.ServeHTTP(res, req)
		res.FinishRequest()
		ctx.serving.Unlock()
		response.FreeResponse(res)
		return nil
	})
	return netpoll.ListenAndServe("tcp", addr, h)
}
```

### License
This package is licensed under a MIT license (Copyright (c) 2020 Meng Huang)


### Author
netpoll was written by Meng Huang.


