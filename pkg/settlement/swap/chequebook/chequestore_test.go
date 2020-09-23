package chequebook_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	storemock "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
)

func newTestChequeStore(
	store storage.StateStorer,
	backend chequebook.Backend,
	factory chequebook.Factory,
	chainID int64,
	beneficiary common.Address,
	simpleSwapBinding chequebook.SimpleSwapBinding,
	recoverChequeFunc chequebook.RecoverChequeFunc) chequebook.ChequeStore {
	return chequebook.NewChequeStore(
		store,
		backend,
		factory,
		chainID,
		beneficiary,
		func(common.Address, bind.ContractBackend) (chequebook.SimpleSwapBinding, error) {
			return simpleSwapBinding, nil
		},
		recoverChequeFunc)
}

func TestReceiveCheque(t *testing.T) {
	store := storemock.NewStateStore()
	beneficiary := common.HexToAddress("0xffff")
	amount := big.NewInt(10)
	chequebookAddress := common.HexToAddress("0xeeee")
	sig := make([]byte, 65)
	chainID := int64(1)

	chequestore := newTestChequeStore(
		store,
		&backendMock{},
		&factoryMock{
			verifyChequebook: func(ctx context.Context, address common.Address) error {
				return nil
			},
		},
		chainID,
		beneficiary,
		&simpleSwapBindingMock{
			balance: func(*bind.CallOpts) (*big.Int, error) {
				return amount, nil
			},
		},
		func(cheque *chequebook.SignedCheque, chainID int64) (common.Address, error) {
			return common.Address{}, nil
		})

	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      beneficiary,
			CumulativePayout: amount,
			Chequebook:       chequebookAddress,
		},
		Signature: sig,
	}

	received, err := chequestore.ReceiveCheque(context.Background(), cheque)
	if err != nil {
		t.Fatal(err)
	}

	if received.Cmp(amount) != 0 {
		t.Fatalf("calculated wrong received amount. wanted %d, got %d", amount, received)
	}
}
