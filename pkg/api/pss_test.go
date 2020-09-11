// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pss"
	resolverMock "github.com/ethersphere/bee/pkg/resolver/mock"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
	"github.com/gorilla/websocket"
)

func newTestWsServer(t *testing.T, o testServerOptions) *httptest.Server {

	if o.Logger == nil {
		o.Logger = logging.New(ioutil.Discard, 0)
	}
	if o.Resolver == nil {
		o.Resolver = resolverMock.NewResolver()
	}
	s := api.New(o.Tags, o.Storer, o.Resolver, o.Pss, o.Logger, nil, api.Options{
		GatewayMode: o.GatewayMode,
	})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)

	return ts
}

// creates a single websocket handler for an arbitrary topic, and receives a message
func TestPssWebsocketSingleHandler(t *testing.T) {
	// create a new pss instance, register a handle through ws, call
	// pss.TryUnwrap with a chunk designated for this handler and expect
	// the handler to be notified
	var (
		logger = logging.New(ioutil.Discard, 0)
		pss    = pss.New(logger)

		server = newTestWsServer(t, testServerOptions{
			Pss:    pss,
			Storer: mock.NewStorer(),
			Logger: logger,
		})

		target     = trojan.Target([]byte{1})
		targets    = trojan.Targets([]trojan.Target{target})
		payload    = []byte("testdata")
		topic      = trojan.NewTopic("testtopic")
		msgContent = make([]byte, len(payload))
		tc         swarm.Chunk
		mtx        sync.Mutex
		cl         = newWsClient(t, server.Listener.Addr().String())
		timeout    = 5 * time.Second
		done       = make(chan struct{})
	)

	cl.SetReadLimit(4096)
	cl.SetReadDeadline(time.Now().Add(timeout))
	cl.SetReadLimit(swarm.ChunkSize)
	cl.SetReadDeadline(time.Now().Add(pongWait))
	cl.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}

			_, message, err := cl.ReadMessage()
			if err != nil {
				return
			}
			fmt.Println("got msg", message, msgContent)

			if message != nil {
				mtx.Lock()
				copy(msgContent, message)
				mtx.Unlock()
			}
		}
	}()
	m, err := trojan.NewMessage(topic, payload)
	if err != nil {
		t.Fatal(err)
	}

	tc, err = m.Wrap(targets)
	if err != nil {
		t.Fatal(err)
	}

	err = pss.TryUnwrap(context.Background(), tc)
	if err != nil {
		t.Fatal(err)
	}
	waitMessage(t, msgContent, payload, timeout, &mtx)
}

func waitMessage(t *testing.T, data, expData []byte, timeout time.Duration, mtx *sync.Mutex) {
	ttl := time.After(timeout)
	for {
		select {
		case <-ttl:
			t.Fatal("timed out waiting for pss message")
		default:
		}
		mtx.Lock()
		if bytes.Equal(data, expData) {
			return
		}
		mtx.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
}

//func TestPssWebsocketSingleHandlerDeregister(t *testing.T) {

//}

//func TestPssWebsocketMultiHandler(t *testing.T) {

//}

//func TestPssPostWebsocket(t *testing.T) {

//}

func newWsClient(t *testing.T, addr string) *websocket.Conn {
	u := url.URL{Scheme: "ws", Host: addr, Path: "/pss/subscribe/testtopic"}

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	return c

	//done := make(chan struct{})

	//go func() {
	//defer close(done)
	//for {
	//_, message, err := c.ReadMessage()
	//if err != nil {
	//log.Println("read:", err)
	//return
	//}
	//log.Printf("recv: %s", message)
	//}
	//}()

	//ticker := time.NewTicker(time.Second)
	//defer ticker.Stop()

	//for {
	//select {
	//case <-done:
	//return
	//case t := <-ticker.C:
	//err := c.WriteMessage(websocket.TextMessage, []byte(t.String()))
	//if err != nil {
	//log.Println("write:", err)
	//return
	//}
	//case <-interrupt:
	//log.Println("interrupt")

	//// Cleanly close the connection by sending a close message and then
	//// waiting (with timeout) for the server to close the connection.
	//err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	//if err != nil {
	//log.Println("write close:", err)
	//return
	//}
	//select {
	//case <-done:
	//case <-time.After(time.Second):
	//}
	//return
	//}
	//}

}
