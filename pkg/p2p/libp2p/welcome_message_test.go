package libp2p_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/p2p/libp2p"
)

func TestDynamicWelcomeMessage(t *testing.T) {
	const TestWelcomeMessage = "Hello World!"
	svc, _ := newService(t, 1, libp2p.Options{WelcomeMessage: TestWelcomeMessage})

	t.Run("Get current message - OK", func(t *testing.T) {
		got := svc.WelcomeMessageSynced()
		if got != TestWelcomeMessage {
			t.Fatalf("expected %s, got %s", TestWelcomeMessage, got)
		}
	})

	t.Run("Set new message - OK", func(t *testing.T) {
		const NewTestMessage = "I'm the new message!"

		err := svc.SetWelcomeMessage(NewTestMessage)
		if err != nil {
			t.Fatal("got error:", err)
		}
		got := svc.WelcomeMessageSynced()
		if got != NewTestMessage {
			t.Fatalf("expected: %s. got %s", NewTestMessage, got)
		}
	})
}
