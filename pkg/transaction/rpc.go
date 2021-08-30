package transaction

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

type RPCBackend interface {
	BlockHashAt(ctx context.Context, blockNumber *big.Int) (common.Hash, error)
}

type rpcBackend struct {
	client *rpc.Client
}

func NewRPCBackend(client *rpc.Client) RPCBackend {
	return &rpcBackend{
		client: client,
	}
}

type blockHeaderResponse struct {
	Hash common.Hash `json:"hash"`
}

func (c rpcBackend) BlockHashAt(ctx context.Context, blockNumber *big.Int) (common.Hash, error) {
	var resp blockHeaderResponse
	err := c.client.CallContext(ctx, &resp, "eth_getBlockByNumber", hexutil.EncodeBig(blockNumber), false)
	if err != nil {
		return common.Hash{}, err
	}
	return resp.Hash, nil
}
