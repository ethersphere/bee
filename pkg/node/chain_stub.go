package node

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

type stubMatcher struct{}

func (m *stubMatcher) Matches(context.Context, []byte, uint64, swarm.Address, bool) ([]byte, error) {
	return nil, nil
}

func stubSwapBackend(log logging.Logger) *loggingSwapBackend {
	return &loggingSwapBackend{
		log: log,
	}
}

type loggingSwapBackend struct {
	log logging.Logger
}

func (m loggingSwapBackend) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	m.log.Debug("MockSwapBackend: CodeAt")
	return common.FromHex(sw3abi.SimpleSwapFactoryDeployedBinv0_4_0), nil
}
func (m loggingSwapBackend) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	panic("MockSwapBackend: CallContract")
}
func (m loggingSwapBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	m.log.Debug("MockSwapBackend: HeaderByNumber")
	h := new(types.Header)
	h.Time = uint64(time.Now().Unix())
	return h, nil
}
func (m loggingSwapBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	panic("MockSwapBackend: PendingNonceAt")
}
func (m loggingSwapBackend) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	panic("MockSwapBackend: SuggestGasPrice")
}
func (m loggingSwapBackend) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	panic("MockSwapBackend: EstimateGas")
}
func (m loggingSwapBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	panic("MockSwapBackend: SendTransaction")
}
func (m loggingSwapBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	m.log.Debug("MockSwapBackend: TransactionReceipt")
	r := new(types.Receipt)
	r.BlockNumber = big.NewInt(1)
	return r, nil
}
func (m loggingSwapBackend) TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error) {
	m.log.Debug("MockSwapBackend: TransactionByHash")
	return nil, false, nil
}
func (m loggingSwapBackend) BlockNumber(ctx context.Context) (uint64, error) {
	m.log.Debug("MockSwapBackend: BlockNumber")
	return 4, nil
}
func (m loggingSwapBackend) BalanceAt(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
	panic("MockSwapBackend: BalanceAt")
}
func (m loggingSwapBackend) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	panic("MockSwapBackend: NonceAt")
}
func (m loggingSwapBackend) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	panic("MockSwapBackend: FilterLogs")
}
func (m loggingSwapBackend) ChainID(ctx context.Context) (*big.Int, error) {
	panic("MockSwapBackend: ChainID")
}
func (m loggingSwapBackend) Close() {}

type stubPostageContract struct{}

func (m *stubPostageContract) CreateBatch(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) ([]byte, error) {
	return nil, postagecontract.ErrSwapBackendDisabled
}
func (m *stubPostageContract) TopUpBatch(ctx context.Context, batchID []byte, topupBalance *big.Int) error {
	return errors.New("chain backend disabled")
}
func (m *stubPostageContract) DiluteBatch(ctx context.Context, batchID []byte, newDepth uint8) error {
	return errors.New("chain backend disabled")
}

type stubChequebookService struct{}

func (m *stubChequebookService) Deposit(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
	return hash, errors.New("chain backend disabled")
}
func (m *stubChequebookService) Withdraw(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
	return hash, errors.New("chain backend disabled")
}
func (m *stubChequebookService) WaitForDeposit(ctx context.Context, txHash common.Hash) error {
	return errors.New("chain backend disabled")
}
func (m *stubChequebookService) Balance(ctx context.Context) (*big.Int, error) {
	return nil, errors.New("chain backend disabled")
}
func (m *stubChequebookService) AvailableBalance(ctx context.Context) (*big.Int, error) {
	return nil, errors.New("chain backend disabled")
}
func (m *stubChequebookService) Address() common.Address {
	return common.Address{}
}
func (m *stubChequebookService) Issue(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error) {
	return nil, errors.New("chain backend disabled")
}
func (m *stubChequebookService) LastCheque(beneficiary common.Address) (*chequebook.SignedCheque, error) {
	return nil, errors.New("chain backend disabled")
}
func (m *stubChequebookService) LastCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	return nil, errors.New("chain backend disabled")
}
