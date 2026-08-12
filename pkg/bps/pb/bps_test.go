// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pb_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/gogo/protobuf/proto"
)

func TestHelloDisambiguates(t *testing.T) {
	t.Parallel()

	topic := bytes.Repeat([]byte{0x2a}, 32)

	open := &pb.Hello{Handshake: &pb.Hello_Open{Open: &pb.Open{
		Cohort: &pb.CohortSpec{
			Topic:      topic,
			Binding:    pb.TopicBinding_ANCHOR,
			Publishers: pb.PublisherRegime_EXPLICIT_SINGLE,
		},
	}}}
	sub := &pb.Hello{Handshake: &pb.Hello_Subscribe{Subscribe: &pb.Subscribe{
		Topic: topic,
	}}}

	for _, tc := range []struct {
		name string
		msg  *pb.Hello
		open bool
	}{
		{name: "open", msg: open, open: true},
		{name: "subscribe", msg: sub, open: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, err := proto.Marshal(tc.msg)
			if err != nil {
				t.Fatal(err)
			}
			var got pb.Hello
			if err := proto.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			if tc.open {
				if got.GetOpen() == nil {
					t.Fatal("expected Open handshake")
				}
				if got.GetSubscribe() != nil {
					t.Fatal("unexpected Subscribe handshake")
				}
				if !bytes.Equal(got.GetOpen().GetCohort().GetTopic(), topic) {
					t.Fatal("topic mismatch")
				}
				return
			}
			if got.GetSubscribe() == nil {
				t.Fatal("expected Subscribe handshake")
			}
			if got.GetOpen() != nil {
				t.Fatal("unexpected Open handshake")
			}
			if !bytes.Equal(got.GetSubscribe().GetTopic(), topic) {
				t.Fatal("topic mismatch")
			}
		})
	}
}
