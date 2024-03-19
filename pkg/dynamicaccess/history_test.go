package dynamicaccess_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/dynamicaccess"
	"github.com/ethersphere/bee/pkg/dynamicaccess/mock"
	"github.com/stretchr/testify/assert"
)

func TestHistoryLookup(t *testing.T) {
	h := prepareTestHistory()
	now := time.Now()

	tests := []struct {
		input    int64
		expected string
	}{
		{input: 0, expected: "value3"},
		{input: now.Unix(), expected: "value3"},
		{input: now.AddDate(0, -5, 0).Unix(), expected: "value3"},
		{input: now.AddDate(0, -6, 0).Unix(), expected: "value3"},
		{input: now.AddDate(-1, 0, 0).Unix(), expected: "value3"},
		{input: now.AddDate(-1, -6, 0).Unix(), expected: "value2"},
		{input: now.AddDate(-2, -0, 0).Unix(), expected: "value2"},
		{input: now.AddDate(-2, -6, 0).Unix(), expected: "value1"},
		{input: now.AddDate(-3, -0, 0).Unix(), expected: "value1"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			actAt, _ := h.Lookup(tt.input)
			output := actAt.Lookup([]byte("key1"))
			assert.Equal(t, output, hex.EncodeToString([]byte(tt.expected)))
		})
	}
}

func prepareTestHistory() dynamicaccess.History {
	var (
		h    = mock.NewHistory()
		now  = time.Now()
		act1 = dynamicaccess.NewDefaultAct()
		act2 = dynamicaccess.NewDefaultAct()
		act3 = dynamicaccess.NewDefaultAct()
	)
	act1.Add([]byte("key1"), []byte("value1"))
	act2.Add([]byte("key1"), []byte("value2"))
	act3.Add([]byte("key1"), []byte("value3"))

	h.Insert(now.AddDate(-3, 0, 0).Unix(), act1)
	h.Insert(now.AddDate(-2, 0, 0).Unix(), act2)
	h.Insert(now.AddDate(-1, 0, 0).Unix(), act3)

	return h
}
