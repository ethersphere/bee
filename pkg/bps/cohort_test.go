// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func addr(b byte) []byte  { return bytes.Repeat([]byte{b}, 20) }
func topic(b byte) []byte { return bytes.Repeat([]byte{b}, swarm.HashSize) }

func validSpec() *pb.CohortSpec {
	return &pb.CohortSpec{
		Topic:         topic(0xaa),
		Binding:       pb.TopicBinding_ANCHOR,
		Publishers:    pb.PublisherRegime_EXPLICIT_LIST,
		Admin:         addr(0x01),
		PublisherList: [][]byte{addr(0x02), addr(0x03)},
	}
}

func TestValidateSpec(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		spec func() *pb.CohortSpec
		want error
	}{
		{name: "valid explicit list", spec: validSpec},
		{name: "valid explicit single", spec: func() *pb.CohortSpec {
			s := validSpec()
			s.Publishers = pb.PublisherRegime_EXPLICIT_SINGLE
			s.PublisherList = nil
			return s
		}},
		{name: "nil spec", spec: func() *pb.CohortSpec { return nil }, want: bps.ErrInvalidSpec},
		{name: "unset binding", spec: func() *pb.CohortSpec {
			s := validSpec()
			s.Binding = pb.TopicBinding_TOPIC_BINDING_UNSPECIFIED
			return s
		}, want: bps.ErrInvalidSpec},
		{name: "unset regime", spec: func() *pb.CohortSpec {
			s := validSpec()
			s.Publishers = pb.PublisherRegime_PUBLISHER_REGIME_UNSPECIFIED
			return s
		}, want: bps.ErrInvalidSpec},
		{name: "short topic", spec: func() *pb.CohortSpec {
			s := validSpec()
			s.Topic = s.Topic[:16]
			return s
		}, want: bps.ErrInvalidSpec},
		{name: "missing admin under explicit regime", spec: func() *pb.CohortSpec {
			s := validSpec()
			s.Admin = nil
			return s
		}, want: bps.ErrInvalidSpec},
		{name: "publisher list under explicit single", spec: func() *pb.CohortSpec {
			s := validSpec()
			s.Publishers = pb.PublisherRegime_EXPLICIT_SINGLE
			return s
		}, want: bps.ErrInvalidSpec},
		{name: "malformed publisher address", spec: func() *pb.CohortSpec {
			s := validSpec()
			s.PublisherList = [][]byte{addr(0x02)[:10]}
			return s
		}, want: bps.ErrInvalidSpec},
		{name: "unsupported binding", spec: func() *pb.CohortSpec {
			s := validSpec()
			s.Binding = pb.TopicBinding_OWNER
			return s
		}, want: bps.ErrUnsupportedBinding},
		{name: "unsupported regime", spec: func() *pb.CohortSpec {
			s := validSpec()
			s.Publishers = pb.PublisherRegime_ALL
			s.Admin = nil
			s.PublisherList = nil
			return s
		}, want: bps.ErrUnsupportedRegime},
		{name: "history unsupported", spec: func() *pb.CohortSpec {
			s := validSpec()
			s.History = true
			return s
		}, want: bps.ErrInvalidSpec},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := bps.ValidateSpec(tc.spec())
			if tc.want == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

func TestSpecEqual(t *testing.T) {
	t.Parallel()

	reordered := validSpec()
	reordered.PublisherList = [][]byte{addr(0x03), addr(0x02)}

	differentAdmin := validSpec()
	differentAdmin.Admin = addr(0x09)

	differentClosed := validSpec()
	differentClosed.Closed = true

	shorterList := validSpec()
	shorterList.PublisherList = [][]byte{addr(0x02)}

	for _, tc := range []struct {
		name string
		a, b *pb.CohortSpec
		want bool
	}{
		{name: "identical", a: validSpec(), b: validSpec(), want: true},
		{name: "publisher list order insensitive", a: validSpec(), b: reordered, want: true},
		{name: "different admin", a: validSpec(), b: differentAdmin},
		{name: "different closed flag", a: validSpec(), b: differentClosed},
		{name: "different list length", a: validSpec(), b: shorterList},
		{name: "both nil", a: nil, b: nil, want: true},
		{name: "one nil", a: validSpec(), b: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := bps.SpecEqual(tc.a, tc.b); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
