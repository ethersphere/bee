package balance

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/pkg/transaction"
)

type balance struct {
	erc20Token        erc20.Service
	swapBackend       transaction.Backend
	overlayEthAddress common.Address
}

func New(overlayEthAddress common.Address, erc20Token erc20.Service, swapBackend transaction.Backend) *balance {
	return &balance{
		erc20Token:        erc20Token,
		swapBackend:       swapBackend,
		overlayEthAddress: overlayEthAddress,
	}
}

func (b *balance) Erc20Balance(ctx context.Context) (*big.Int, error) {
	return b.erc20Token.BalanceOf(ctx, b.overlayEthAddress)
}

func (b *balance) EthBalance(ctx context.Context) (*big.Int, error) {
	return b.swapBackend.BalanceAt(ctx, b.overlayEthAddress, nil)
}
