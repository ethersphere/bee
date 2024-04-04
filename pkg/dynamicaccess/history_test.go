package dynamicaccess_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/dynamicaccess"
	"github.com/ethersphere/bee/v2/pkg/dynamicaccess/mock"
	kvsmock "github.com/ethersphere/bee/v2/pkg/kvs/mock"
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
			sAt, _ := h.Lookup(tt.input)
			output, _ := sAt.Get([]byte("key1"))
			assert.Equal(t, output, hex.EncodeToString([]byte(tt.expected)))
		})
	}
}

func prepareTestHistory() dynamicaccess.History {
	var (
		h   = mock.NewHistory()
		now = time.Now()
		s1  = kvsmock.New()
		s2  = kvsmock.New()
		s3  = kvsmock.New()
	)
	s1.Put([]byte("key1"), []byte("value1"))
	s2.Put([]byte("key1"), []byte("value2"))
	s3.Put([]byte("key1"), []byte("value3"))

	h.Insert(now.AddDate(-3, 0, 0).Unix(), s1)
	h.Insert(now.AddDate(-2, 0, 0).Unix(), s2)
	h.Insert(now.AddDate(-1, 0, 0).Unix(), s3)

	return h
}
