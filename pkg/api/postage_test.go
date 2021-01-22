package api_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	contractMock "github.com/ethersphere/bee/pkg/postage/postagecontract/mock"
)

func TestPostageCreateStamp(t *testing.T) {
	batchId := []byte{1, 2, 3, 4}
	initialBalance := int64(1000)
	depth := uint8(1)

	contract := contractMock.New(
		contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8) ([]byte, error) {
			if ib.Cmp(big.NewInt(initialBalance)) != 0 {
				return nil, fmt.Errorf("called with wrong initial balance. wanted %d, got %d", initialBalance, ib)
			}
			if d != depth {
				return nil, fmt.Errorf("called with wrong depth. wanted %d, got %d", depth, d)
			}
			return batchId, nil
		}),
	)
	createBatch := func(amount int64, depth uint8) string { return fmt.Sprintf("/stamps/%d/%d", amount, depth) }
	client, _, _ := newTestServer(t, testServerOptions{
		PostageContract: contract,
	})

	jsonhttptest.Request(t, client, http.MethodPost, createBatch(initialBalance, depth), http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: hex.EncodeToString(batchId),
			Code:    http.StatusOK,
		}))
}
