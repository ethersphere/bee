package transaction_test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction/backendmock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestMatchesSender(t *testing.T) {
	t.Skip()

	txByHash := backendmock.WithTransactionByHashFunc(func(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
		return nil, false, nil
	})
	matcher := transaction.NewMatcher(backendmock.New(txByHash), &types.HomesteadSigner{})
	_, err := matcher.Matches(context.Background(), "0x123", 0, swarm.NewAddress([]byte{}))
	if err != nil {
		t.Fatal(err)
	}
	// WIP
}

// type mockSigner struct {
// }

// func (*mockSigner) Sender(tx *types.Transaction) (common.Address, error) {
// 	return common.Address{}, nil
// }
