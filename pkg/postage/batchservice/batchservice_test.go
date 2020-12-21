package batchservice_test

import (
	"io/ioutil"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchservice"
	batchstoremock "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
)

func newTestBatchService() postage.EventUpdater {
	log := logging.New(ioutil.Discard, 0)
	store := batchstoremock.New()

	return batchservice.NewBatchService(store, log)
}
