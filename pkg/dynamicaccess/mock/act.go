package mock

import (
	"github.com/ethersphere/bee/pkg/dynamicaccess"
	"github.com/ethersphere/bee/pkg/manifest"
)

type ActMock struct {
	AddFunc    func(lookupKey []byte, encryptedAccessKey []byte) dynamicaccess.Act
	LookupFunc func(lookupKey []byte) []byte
	LoadFunc   func(lookupKey []byte) manifest.Entry
	StoreFunc  func(me manifest.Entry)
}

var _ dynamicaccess.Act = (*ActMock)(nil)

func (act *ActMock) Add(lookupKey []byte, encryptedAccessKey []byte) dynamicaccess.Act {
	if act.AddFunc == nil {
		return act
	}
	return act.AddFunc(lookupKey, encryptedAccessKey)
}

func (act *ActMock) Lookup(lookupKey []byte) []byte {
	if act.LookupFunc == nil {
		return make([]byte, 0)
	}
	return act.LookupFunc(lookupKey)
}

func (act *ActMock) Load(lookupKey []byte) manifest.Entry {
	if act.LoadFunc == nil {
		return nil
	}
	return act.LoadFunc(lookupKey)
}

func (act *ActMock) Store(me manifest.Entry) {
	if act.StoreFunc == nil {
		return
	}
	act.StoreFunc(me)
}

func NewActMock(addFunc func(lookupKey []byte, encryptedAccessKey []byte) dynamicaccess.Act, getFunc func(lookupKey []byte) []byte) dynamicaccess.Act {
	return &ActMock{
		AddFunc:    addFunc,
		LookupFunc: getFunc,
	}
}
