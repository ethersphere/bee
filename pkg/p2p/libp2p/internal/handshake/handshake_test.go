package handshake

import (
	"bytes"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"io/ioutil"
	"testing"
)

type StreamMock struct {
	Buffer bytes.Buffer
}

func (s *StreamMock) Read(p []byte) (n int, err error) {
	return s.Buffer.Read(p)
}

func (s *StreamMock) Write(p []byte) (n int, err error) {
	return s.Buffer.Write(p)
}

func (s *StreamMock) Close() error {
	panic("stream.Close() should not be called")
}

func TestHandshake(t *testing.T) {
	logger := logging.New(ioutil.Discard)
	info := Info{
		Address:   "node1",
		NetworkID: 0,
		Light:     false,
	}
	handshakeService := New(info.Address, info.NetworkID, logger)

	t.Run("OK", func(t *testing.T) {
		stream := &StreamMock{}
		res, err := handshakeService.Handshake(stream)
		if err != nil {
			t.Fatal(err)
		}

		if *res != info {
			t.Fatalf("got %+v, expected %+v", res, info)
		}
	})
}

func TestHandle(t *testing.T) {
	logger := logging.New(ioutil.Discard)
	handshakeService := New("node1", 0, logger)

	t.Run("OK", func(t *testing.T) {
		stream := &StreamMock{}
		w, _ := protobuf.NewWriterAndReader(stream)
		info := Info{
			Address:   "node2",
			NetworkID: 1,
			Light:     false,
		}

		w.WriteMsg(&ShakeHand{
			Address:   info.Address,
			NetworkID: info.NetworkID,
			Light:     info.Light,
		})

		res, err := handshakeService.Handshake(stream)
		if err != nil {
			t.Fatal(err)
		}

		if *res != info {
			t.Fatalf("got %+v, expected %+v", res, info)
		}
	})
}
