package chequebook

import (
	"bytes"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/sw3-bindings/simpleswapfactory"
	"golang.org/x/net/context"
)

var (
	ErrInvalidFactory       = errors.New("not a valid factory contract")
	ErrNotDeployedByFactory = errors.New("chequebook not deployed by factory")
)

type Factory interface {
	Deploy(ctx context.Context, issuer common.Address, defaultHardDepositTimeoutDuration *big.Int) (common.Address, error)
	VerifyBytecode(ctx context.Context) error
	VerifyChequebook(ctx context.Context, chequebook common.Address) error
}

type factory struct {
	backend            Backend
	transactionService TransactionService
	address            common.Address

	ABI      abi.ABI
	instance *simpleswapfactory.SimpleSwapFactory
}

func NewFactory(backend Backend, transactionService TransactionService, address common.Address) (Factory, error) {
	ABI, err := abi.JSON(strings.NewReader(simpleswapfactory.SimpleSwapFactoryABI))
	if err != nil {
		return nil, err
	}

	instance, err := simpleswapfactory.NewSimpleSwapFactory(address, backend)
	if err != nil {
		return nil, err
	}

	return &factory{
		backend:            backend,
		transactionService: transactionService,
		address:            address,
		ABI:                ABI,
		instance:           instance,
	}, nil
}

func (c *factory) Deploy(ctx context.Context, issuer common.Address, defaultHardDepositTimeoutDuration *big.Int) (common.Address, error) {
	callData, err := c.ABI.Pack("deploySimpleSwap", issuer, big.NewInt(0).Set(defaultHardDepositTimeoutDuration))
	if err != nil {
		return common.Address{}, err
	}

	request := &TxRequest{
		To:       c.address,
		Data:     callData,
		GasPrice: nil,
		GasLimit: 0,
		Value:    big.NewInt(0),
	}

	txHash, err := c.transactionService.Send(ctx, request)
	if err != nil {
		return common.Address{}, err
	}

	receipt, err := c.transactionService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return common.Address{}, err
	}

	chequebookAddress, err := c.parseDeployReceipt(receipt)
	if err != nil {
		return common.Address{}, err
	}

	return chequebookAddress, nil
}

func (c *factory) parseDeployReceipt(receipt *types.Receipt) (address common.Address, err error) {
	for _, log := range receipt.Logs {
		if log.Address != c.address {
			continue
		}
		if event, err := c.instance.ParseSimpleSwapDeployed(*log); err == nil {
			address = event.ContractAddress
			break
		}
	}
	if (address == common.Address{}) {
		return common.Address{}, errors.New("contract deployment failed")
	}
	return address, nil
}

func (c *factory) VerifyBytecode(ctx context.Context) (err error) {
	code, err := c.backend.CodeAt(ctx, c.address, nil)
	if err != nil {
		return err
	}

	referenceCode := common.FromHex(simpleswapfactory.SimpleSwapFactoryDeployedCode)
	if !bytes.Equal(code, referenceCode) {
		return ErrInvalidFactory
	}
	return nil
}

func (c *factory) VerifyChequebook(ctx context.Context, chequebook common.Address) error {
	deployed, err := c.instance.DeployedContracts(&bind.CallOpts{
		Context: ctx,
	}, chequebook)
	if err != nil {
		return err
	}
	if !deployed {
		return ErrNotDeployedByFactory
	}
	return nil
}
