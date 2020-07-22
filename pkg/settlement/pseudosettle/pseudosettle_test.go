package pseudosettle_test

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle/pb"
	"github.com/ethersphere/bee/pkg/swarm"
)

type testObserver struct{}

func (*testObserver) NotifyPayment(peer swarm.Address, amount uint64) error {
	return nil
}

func TestPayment(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	recipient := pseudosettle.New(pseudosettle.Options{
		Logger: logger,
	})
	recipient.SetPaymentObserver(&testObserver{})

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
	)

	payer := pseudosettle.New(pseudosettle.Options{
		Streamer: recorder,
		Logger:   logger,
	})

	peerID := swarm.MustParseHexAddress("9ee7add7")
	amount := uint64(10000)

	err := payer.Pay(peerID, amount)
	if err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}

	record := records[0]

	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.Payment) },
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 1 {
		t.Fatalf("got %v messages, want %v", len(messages), 1)
	}

	sentAmount := messages[0].(*pb.Payment).Amount
	if sentAmount != amount {
		t.Fatalf("got message with amount %v, want %v", sentAmount, amount)
	}
}
