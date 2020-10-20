// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package protobuf

import (
	"context"
	"errors"
	"io"

	"github.com/ethersphere/bee/pkg/p2p"
	ggio "github.com/gogo/protobuf/io"
	"github.com/gogo/protobuf/proto"
)

const delimitedReaderMaxSize = 128 * 1024 // max message size

var ErrTimeout = errors.New("timeout")

type Message = proto.Message

func NewWriterAndReader(s p2p.Stream) (Writer, Reader) {
	return NewWriter(s), NewReader(s)
}

func NewReader(r io.Reader) Reader {
	return newReader(ggio.NewDelimitedReader(r, delimitedReaderMaxSize))
}

func NewWriter(w io.Writer) Writer {
	return newWriter(ggio.NewDelimitedWriter(w))
}

func ReadMessages(r io.Reader, newMessage func() Message) (m []Message, err error) {
	pr := NewReader(r)
	for {
		msg := newMessage()
		if err := pr.ReadMsg(msg); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		m = append(m, msg)
	}
	return m, nil
}

type Reader struct {
	ggio.Reader
}

func newReader(r ggio.Reader) Reader {
	return Reader{Reader: r}
}

func (r Reader) ReadMsgWithContext(ctx context.Context, msg proto.Message) error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- r.ReadMsg(msg)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type Writer struct {
	ggio.Writer
}

func newWriter(r ggio.Writer) Writer {
	return Writer{Writer: r}
}

func (w Writer) WriteMsgWithContext(ctx context.Context, msg proto.Message) error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- w.WriteMsg(msg)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
