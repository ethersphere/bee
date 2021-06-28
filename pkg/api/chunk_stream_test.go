package api_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/logging"
	pinning "github.com/ethersphere/bee/pkg/pinning/mock"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/gorilla/websocket"
)

func TestChunkUploadStream(t *testing.T) {

	wsHeaders := http.Header{}
	wsHeaders.Set("Content-Type", "application/octet-stream")
	wsHeaders.Set("Swarm-Postage-Batch-Id", batchOkStr)

	var (
		statestoreMock = statestore.NewStateStore()
		logger         = logging.New(ioutil.Discard, 0)
		tag            = tags.NewTags(statestoreMock, logger)
		storerMock     = mock.NewStorer()
		pinningMock    = pinning.NewServiceMock()
		_, wsConn, _   = newTestServer(t, testServerOptions{
			Storer:    storerMock,
			Pinning:   pinningMock,
			Tags:      tag,
			Post:      mockpost.New(mockpost.WithAcceptAll()),
			WsPath:    "/stream/chunks",
			WsHeaders: wsHeaders,
		})
	)

	t.Run("upload and verify", func(t *testing.T) {
		chsToGet := []swarm.Chunk{}
		for i := 0; i < 5; i++ {
			ch := testingc.GenerateTestRandomChunk()

			err := wsConn.SetWriteDeadline(time.Now().Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}

			err = wsConn.WriteMessage(websocket.BinaryMessage, ch.Data())
			if err != nil {
				t.Fatal(err)
			}

			err = wsConn.SetReadDeadline(time.Now().Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}

			mt, msg, err := wsConn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}

			if mt != websocket.TextMessage || !bytes.Equal(msg, api.SuccessWsMsg) {
				t.Fatal("invalid response", mt, string(msg))
			}

			chsToGet = append(chsToGet, ch)
		}

		for _, c := range chsToGet {
			ch, err := storerMock.Get(context.Background(), storage.ModeGetRequest, c.Address())
			if err != nil {
				t.Fatal("failed to get chunk after upload", err)
			}
			if !ch.Equal(c) {
				t.Fatal("invalid chunk read")
			}
		}
	})

	t.Run("close on incorrect msg", func(t *testing.T) {
		err := wsConn.SetWriteDeadline(time.Now().Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}

		err = wsConn.WriteMessage(websocket.TextMessage, []byte("incorrect msg"))
		if err != nil {
			t.Fatal(err)
		}

		err = wsConn.SetReadDeadline(time.Now().Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = wsConn.ReadMessage()
		if err == nil {
			t.Fatal("expected failure on read")
		}
		if cerr, ok := err.(*websocket.CloseError); !ok {
			t.Fatal("invalid error on read")
		} else {
			if cerr.Text != "invalid message" {
				t.Fatalf("incorrect response on error, exp: (invalid message) got (%s)", cerr.Text)
			}
		}
	})
}
