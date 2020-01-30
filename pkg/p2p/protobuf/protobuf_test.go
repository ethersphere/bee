// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package protobuf_test

import (
	"fmt"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/protobuf/internal/pb"
)

func TestReadMessages(t *testing.T) {
	r, pipe := io.Pipe()

	w := protobuf.NewWriter(pipe)

	messages := []string{"first", "second", "third"}

	go func() {
		for _, m := range messages {
			if err := w.WriteMsg(&pb.Message{
				Text: m,
			}); err != nil {
				panic(err)
			}
		}
		if err := pipe.Close(); err != nil {
			panic(err)
		}
	}()

	got, err := protobuf.ReadMessages(r, func() protobuf.Message { return new(pb.Message) })
	if err != nil {
		t.Fatal(err)
	}

	var gotMessages []string
	for _, m := range got {
		gotMessages = append(gotMessages, m.(*pb.Message).Text)
	}

	if fmt.Sprint(gotMessages) != fmt.Sprint(messages) {
		t.Errorf("got messages %v, want %v", gotMessages, messages)
	}
}
