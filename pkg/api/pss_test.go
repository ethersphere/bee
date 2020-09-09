package api_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"testing"

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

	//return &http.Client{
	//Transport: web.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
	//u, err := url.Parse(ts.URL + r.URL.String())
	//if err != nil {
	//return nil, err
	//}
	//r.URL = u
	//return ts.Client().Transport.RoundTrip(r)
	//}),
	//}
	return ts
}

// creates a single websocket handler for an arbitrary topic, and receives a message
func TestPssWebsocketSingleHandler(t *testing.T) {
	// create a new pss instance, register a handle through ws, call
	// pss.TryUnwrap with a chunk designated for this handler and expect
	// the handler to be notified
	var (
		logger = logging.New(ioutil.Discard, 0)
		pss    = pss.New(logger, nil)

		//mockStatestore = statestore.NewStateStore()
		server = newTestWsServer(t, testServerOptions{
			Pss:    pss,
			Storer: mock.NewStorer(),
			Logger: logger,
		})
	)

	url := server.URL
	fmt.Println(url)
	fmt.Println(server.Listener.Addr().String())
	cl := newWsClient(t, server.Listener.Addr().String())
	fmt.Println(cl)
	t.Fatal(1)

	target := trojan.Target([]byte{1}) // arbitrary test target
	targets := trojan.Targets([]trojan.Target{target})
	payload := []byte("testdata")
	topic := trojan.NewTopic("testtopic")

	m, err := trojan.NewMessage(topic, payload)
	if err != nil {
		t.Fatal(err)
	}

	var tc swarm.Chunk
	tc, err = m.Wrap(targets)
	if err != nil {
		t.Fatal(err)
	}

	err = pss.TryUnwrap(context.Background(), tc)
	if err != nil {
		t.Fatal(err)
	}

	// assert the websocket got the correct data
}

//func TestPssWebsocketSingleHandlerDeregister(t *testing.T) {

//}

//func TestPssWebsocketMultiHandler(t *testing.T) {

//}

//func TestPssPostWebsocket(t *testing.T) {

//}

func newWsClient(t *testing.T, addr string) *websocket.Conn {
	u := url.URL{Scheme: "ws", Host: addr, Path: "/pss/ws"}

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
