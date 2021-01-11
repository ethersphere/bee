package listener_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/postage/listener"
)

var hash common.Hash = common.HexToHash("ff6ec1ed9250a6952fabac07c6eb103550dc65175373eea432fd115ce8bb2246")
var addr common.Address = common.HexToAddress("abcdef")
var createdTopic = common.HexToHash("3f6ec1ed9250a6952fabac07c6eb103550dc65175373eea432fd115ce8bb2246")

func init() {
	//a := make([]byte, 20)
	//a[0] = 20
	//copy(addr[:], a)
	//h := make([]byte, 32)
	//copy(hash[:], h)
}

func TestListener(t *testing.T) {
	// test that when the listener gets a certain event
	// then we would like to assert the appropriate EventUpdater method was called
	//mockId := make([]byte, 32)
	//mockOwner := make([]byte, 32)
	c := createArgs{
		id:               hash[:],
		owner:            addr[:],
		amount:           big.NewInt(42),
		normalisedAmount: big.NewInt(43),
		depth:            100,
	}

	ev, evC := newEventUpdaterMock()
	mf := newMockFilterer(
		newCreateEvent(c.id, c.amount, c.normalisedAmount, c.depth),
	)
	listener := listener.New(mf)
	listener.Listen(0, ev)

	select {
	case <-evC:

	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func newEventUpdaterMock() (*updater, chan interface{}) {
	c := make(chan interface{})
	return &updater{
		eventC: c,
	}, c
}

type updater struct {
	eventC            chan interface{}
	createCalled      bool
	topupCalled       bool
	updateDepthCalled bool
	updatePriceCalled bool
}

func (u *updater) Create(id []byte, owner []byte, amount *big.Int, normalisedAmount *big.Int, depth uint8) error {
	u.createCalled = true
	return nil
}

func (u *updater) TopUp(id []byte, amount *big.Int) error {
	u.topupCalled = true
	return nil
}

func (u *updater) UpdateDepth(id []byte, depth uint8) error {
	u.updateDepthCalled = true
	return nil
}

func (u *updater) UpdatePrice(price *big.Int) error {
	u.updatePriceCalled = true
	return nil
}

type mockFilterer struct {
	events []types.Log
	sub    *sub
}

func newMockFilterer(logs ...types.Log) *mockFilterer {
	return &mockFilterer{
		events: logs,
	}
}

func (m *mockFilterer) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return m.events, nil
}

func (m *mockFilterer) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	go func() {
		for _, ev := range m.events {
			ch <- ev
		}
	}()
	s := newSub()
	return s, nil
}

func (m *mockFilterer) Close() {
	close(m.sub.c)
}

func (m *mockFilterer) BlockHeight(context.Context) (uint64, error) {
	return 0, nil
}

func parseABI(json string) abi.ABI {
	cabi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Sprintf("error creating ABI for postage contract: %v", err))
	}
	return cabi
}

func newCreateEvent(batchID common.Hash, totalAmount *big.Int, normalisedBalance *big.Int, depth uint8) types.Log {
	a := parseABI(listener.Abi)
	b, err := a.Events["BatchCreated"].Inputs[1:].NonIndexed().Pack(totalAmount, normalisedBalance, addr, depth)
	if err != nil {
		panic(err)
	}
	fmt.Println(b)
	l := types.Log{
		Data:   b,
		Topics: []common.Hash{createdTopic, batchID}, // 1st item is the function sig digest, 2nd is always the batch id
	}

	return l

}

type sub struct {
	c chan error
}

func newSub() *sub {
	return &sub{
		c: make(chan error),
	}
}

func (s *sub) Unsubscribe() {}
func (s *sub) Err() <-chan error {
	return s.c
}

func TestT(t *testing.T) {
	t.Skip()
	eventSignatures := []string{
		"BatchCreated(bytes32,uint256,uint256,address,uint8)",
		"BatchTopUp(bytes32,uint256)",
		"BatchDepthIncrease(bytes32,uint256)",
		"PriceUpdate(uint256)",
	}

	t.Fatal(digestSig(eventSignatures))
}

func digestSig(sigs []string) []string {
	digests := make([]string, 4)
	for i, s := range sigs {
		h, err := crypto.LegacyKeccak256([]byte(s))
		if err != nil {
			panic(fmt.Sprintf("error digesting signatures: %v", err))
		}
		digests[i] = hex.EncodeToString(h)
	}
	return digests
}

type createArgs struct {
	id               []byte
	owner            []byte
	amount           *big.Int
	normalisedAmount *big.Int
	depth            uint8
}

func (c createArgs) compare(cc createArgs) {
	if !bytes.Equal(c.id, cc.id) {
		t.Fatal("id mismatch")
	}

	if !bytes.Equal(c.owner, cc.owner) {
		t.Fatal("owner mismatch")
	}

}

type topupArgs struct {
	id     []byte
	amount *big.Int
}

type depthArgs struct {
	id    []byte
	depth uint8
}

type priceArgs struct {
	price *big.Int
}
