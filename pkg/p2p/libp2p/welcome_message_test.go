// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package libp2p_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake"
)

func TestDynamicWelcomeMessage(t *testing.T) {
	const TestWelcomeMessage = "Hello World!"
	svc, _ := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{WelcomeMessage: TestWelcomeMessage}})

	t.Run("Get current message - OK", func(t *testing.T) {
		got := svc.GetWelcomeMessage()
		if got != TestWelcomeMessage {
			t.Fatalf("expected %s, got %s", TestWelcomeMessage, got)
		}
	})

	t.Run("Set new message", func(t *testing.T) {
		t.Run("OK", func(t *testing.T) {
			const testMessage = "I'm the new message!"

			err := svc.SetWelcomeMessage(testMessage)
			if err != nil {
				t.Fatal("got error:", err)
			}
			got := svc.GetWelcomeMessage()
			if got != testMessage {
				t.Fatalf("expected: %s. got %s", testMessage, got)
			}
		})
		t.Run("error - message too long", func(t *testing.T) {
			const testMessage = `Lorem ipsum dolor sit amet, consectetur adipiscing elit.
			Maecenas eu aliquam enim. Nulla tincidunt arcu nec nulla condimentum nullam sodales` // 141 characters

			want := handshake.ErrWelcomeMessageLength
			got := svc.SetWelcomeMessage(testMessage)
			if got != want {
				t.Fatalf("wrong error: want %v, got %v", want, got)
			}
		})

	})
}
