package transaction

import (
	"context"
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

func NewMatcher(backend Backend, chainID int64) *Matcher {
	return &Matcher{
		backend: backend,
		signer:  types.NewEIP155Signer(big.NewInt(chainID)),
	}
}

func (s Matcher) Matches(ctx context.Context, tx string, networkID uint64, senderOverlay swarm.Address) (bool, error) {
	incomingTx := common.HexToHash(tx)

	nTx, isPending, err := s.backend.TransactionByHash(ctx, incomingTx)
	if err != nil {
		return false, err //TODO wrap error
	}

	if isPending {
		return false, fmt.Errorf("transaction still pending")
	}

	sender, err := types.Sender(nil, nTx)
	if err != nil {
		return false, err //TODO wrap error
	}

	expectedRemoteBzzAddress := crypto.NewOverlayFromEthereumAddress(sender.Bytes(), networkID)

	return expectedRemoteBzzAddress.Equal(senderOverlay), nil
}
