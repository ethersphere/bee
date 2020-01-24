// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package protobuf

import (
	"github.com/ethersphere/bee/pkg/p2p"
	ggio "github.com/gogo/protobuf/io"
	"github.com/gogo/protobuf/proto"
	"io"
)

const delimitedReaderMaxSize = 128 * 1024 // max message size

type Message = proto.Message

func NewWriterAndReader(s p2p.Stream) (w ggio.Writer, r ggio.Reader) {
	r = ggio.NewDelimitedReader(s, delimitedReaderMaxSize)
	w = ggio.NewDelimitedWriter(s)
	return w, r
}

func NewReader(r io.Reader) ggio.Reader {
	return ggio.NewDelimitedReader(r, delimitedReaderMaxSize)
}

func NewWriter(w io.Writer) ggio.Writer {
	return ggio.NewDelimitedWriter(w)
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
