# netpoll
[![PkgGoDev](https://pkg.go.dev/badge/github.com/hslam/netpoll)](https://pkg.go.dev/github.com/hslam/netpoll)
[![Build Status](https://travis-ci.org/hslam/netpoll.svg?branch=master)](https://travis-ci.org/hslam/netpoll)
[![codecov](https://codecov.io/gh/hslam/netpoll/branch/master/graph/badge.svg)](https://codecov.io/gh/hslam/netpoll)
[![Go Report Card](https://goreportcard.com/badge/github.com/hslam/netpoll?v=7e100)](https://goreportcard.com/report/github.com/hslam/netpoll)
[![LICENSE](https://img.shields.io/github/license/hslam/netpoll.svg?style=flat-square)](https://github.com/hslam/netpoll/blob/master/LICENSE)

Package netpoll implements a network poller based on epoll/kqueue.

## Features

* Epoll/Kqueue
* TCP/UNIX
* Compatible with the net.Conn
* Upgrade Connection
* Non-Blocking I/O
* Sync/Async Workers
* Rescheduling Workers

**Comparison to other packages.**

|Package| [net](https://github.com/golang/go/tree/master/src/net "net")| [netpoll](https://github.com/hslam/netpoll "netpoll")|[gnet](https://github.com/panjf2000/gnet "gnet")|[evio](https://github.com/tidwall/evio "evio")|
|:--:|:--|:--|:--|:--|
|Sync Handler|Yes|Yes|Yes|Yes|
|Async Handler|Yes|Yes|Yes|No|
|Low Memory Usage|No|Yes|Yes|Yes|
|Non-Blocking I/O|No|Yes|Yes|Yes|
|Rescheduling|Yes|Yes|No|No|
|Compatible With The net.Conn|Yes|Yes|No|No|

## [Benchmarks](http://github.com/hslam/netpoll-benchmark "netpoll-benchmark")

##### Network QPS

<img src="https://raw.githubusercontent.com/hslam/netpoll/master/netpoll-qps.png"  alt="mock 0ms" align=center>

##### Mocking 1ms QPS

<img src="https://raw.githubusercontent.com/hslam/netpoll/master/netpoll-mock-time-qps.png" alt="mock 1ms" align=center>


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
#### Websocket Example
```
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

### License
This package is licensed under a MIT license (Copyright (c) 2020 Meng Huang)


### Author
netpoll was written by Meng Huang.


