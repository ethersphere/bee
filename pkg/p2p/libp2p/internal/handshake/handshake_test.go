package handshake

import (
	"bytes"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"io/ioutil"
	"testing"
)

type StreamMock struct {
	ReadBuffer  *bytes.Buffer
	WriteBuffer *bytes.Buffer
}

func (s *StreamMock) Read(p []byte) (n int, err error) {
	return s.ReadBuffer.Read(p)
}

func (s *StreamMock) Write(p []byte) (n int, err error) {
	return s.WriteBuffer.Write(p)
}

func (s *StreamMock) Close() error {
	return nil
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
		expectedInfo := Info{
			Address:   "node2",
			NetworkID: 1,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := &StreamMock{ReadBuffer: &buffer1, WriteBuffer: &buffer2}
		stream2 := &StreamMock{ReadBuffer: &buffer2, WriteBuffer: &buffer1}

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&ShakeHand{
			Address:   expectedInfo.Address,
			NetworkID: expectedInfo.NetworkID,
			Light:     expectedInfo.Light,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(stream1)
		if err != nil {
			t.Fatal(err)
		}

		if *res != expectedInfo {
			t.Fatalf("got %+v, expected %+v", res, info)
		}
	})
}

func TestHandle(t *testing.T) {
	nodeInfo := Info{
		Address:   "node1",
		NetworkID: 0,
		Light:     false,
	}

	logger := logging.New(ioutil.Discard)
	handshakeService := New(nodeInfo.Address, nodeInfo.NetworkID, logger)

	t.Run("OK", func(t *testing.T) {
		node2Info := Info{
			Address:   "node2",
			NetworkID: 1,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := &StreamMock{ReadBuffer: &buffer1, WriteBuffer: &buffer2}
		stream2 := &StreamMock{ReadBuffer: &buffer2, WriteBuffer: &buffer1}

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&ShakeHand{
			Address:   node2Info.Address,
			NetworkID: node2Info.NetworkID,
			Light:     node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream1)
		if err != nil {
			t.Fatal(err)
		}

		if *res != node2Info {
			t.Fatalf("got %+v, expected %+v", res, node2Info)
		}

		_, r := protobuf.NewWriterAndReader(stream2)
		var got ShakeHand
		if err := r.ReadMsg(&got); err != nil {
			t.Fatal(err)
		}

		if nodeInfo != Info(got) {
			t.Fatalf("got %+v, expected %+v", got, node2Info)
		}
	})
}
