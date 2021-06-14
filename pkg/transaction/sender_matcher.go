package transaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Matcher struct {
	backend Backend
	signer  types.Signer
}

var (
	ErrTransactionNotFound      = errors.New("transaction not found")
	ErrTransactionPending       = errors.New("transaction in pending status")
	ErrTransactionSenderInvalid = errors.New("invalid transaction sender")
)

func NewMatcher(backend Backend, signer types.Signer) *Matcher {
	return &Matcher{
		backend: backend,
		signer:  signer,
	}
}

func (m *Matcher) Matches(ctx context.Context, tx []byte, networkID uint64, senderOverlay swarm.Address) (bool, []byte, error) {

	incomingTx := common.BytesToHash(tx)

	nTx, isPending, err := m.backend.TransactionByHash(ctx, incomingTx)
	if err != nil {
		return false, nil, fmt.Errorf("%v: %w", err, ErrTransactionNotFound)
	}

	if isPending {
		return false, nil, ErrTransactionPending
	}

	sender, err := types.Sender(m.signer, nTx)
	if err != nil {
		return false, nil, fmt.Errorf("%v: %w", err, ErrTransactionSenderInvalid)
	}

	receipt, err := m.backend.TransactionReceipt(ctx, incomingTx)
	if err != nil {
		return false, nil, err
	}

	nextBlock, err := m.backend.HeaderByNumber(ctx, big.NewInt(0).Add(receipt.BlockNumber, big.NewInt(1)))
	if err != nil {
		return false, nil, err
	}

	if !bytes.Equal(receipt.BlockHash.Bytes(), nextBlock.ParentHash.Bytes()) {
		return false, nil, nil
	}

	expectedRemoteBzzAddress := crypto.NewOverlayFromEthereumAddress(sender.Bytes(), networkID, tx)

	return expectedRemoteBzzAddress.Equal(senderOverlay), nextBlock.Hash().Bytes(), nil
}
