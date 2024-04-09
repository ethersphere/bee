package dynamicaccess_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/dynamicaccess"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/storage"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
)

func TestHistoryAdd(t *testing.T) {
	h, err := dynamicaccess.NewHistory(nil, nil)
	assert.NoError(t, err)

	addr := swarm.NewAddress([]byte("addr"))

	ctx := context.Background()

	err = h.Add(ctx, addr, nil)
	assert.NoError(t, err)
}

func TestSingleNodeHistoryLookup(t *testing.T) {
	storer := mockstorer.New()
	ctx := context.Background()
	ls := loadsave.New(storer.ChunkStore(), storer.Cache(), pipelineFactory(storer.Cache(), false))

	h, err := dynamicaccess.NewHistory(ls, nil)
	assert.NoError(t, err)

	testActRef := swarm.RandAddress(t)
	err = h.Add(ctx, testActRef, nil)
	assert.NoError(t, err)

	_, err = h.Store(ctx)
	assert.NoError(t, err)

	searchedTime := time.Now().Unix()
	actRef, err := h.Lookup(ctx, searchedTime)
	assert.NoError(t, err)
	assert.True(t, actRef.Equal(testActRef))
}

func TestMultiNodeHistoryLookup(t *testing.T) {
	storer := mockstorer.New()
	ctx := context.Background()
	ls := loadsave.New(storer.ChunkStore(), storer.Cache(), pipelineFactory(storer.Cache(), false))

	h, _ := dynamicaccess.NewHistory(ls, nil)

	testActRef1 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd891"))
	firstTime := time.Date(1994, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	h.Add(ctx, testActRef1, &firstTime)

	testActRef2 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd892"))
	secondTime := time.Date(2000, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	h.Add(ctx, testActRef2, &secondTime)

	testActRef3 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd893"))
	thirdTime := time.Date(2015, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	h.Add(ctx, testActRef3, &thirdTime)

	testActRef4 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd894"))
	fourthTime := time.Date(2020, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	h.Add(ctx, testActRef4, &fourthTime)

	testActRef5 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd895"))
	fifthTime := time.Date(2030, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	h.Add(ctx, testActRef5, &fifthTime)

	// latest
	searchedTime := time.Date(1980, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	actRef, err := h.Lookup(ctx, searchedTime)
	assert.NoError(t, err)
	assert.True(t, actRef.Equal(testActRef1))

	// before first time
	searchedTime = time.Date(2021, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	actRef, err = h.Lookup(ctx, searchedTime)
	assert.NoError(t, err)
	assert.True(t, actRef.Equal(testActRef4))

	// same time
	searchedTime = time.Date(2000, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	actRef, err = h.Lookup(ctx, searchedTime)
	assert.NoError(t, err)
	assert.True(t, actRef.Equal(testActRef2))

	// after time
	searchedTime = time.Date(2045, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	actRef, err = h.Lookup(ctx, searchedTime)
	assert.NoError(t, err)
	assert.True(t, actRef.Equal(testActRef5))
}

func TestHistoryStore(t *testing.T) {
	storer := mockstorer.New()
	ctx := context.Background()
	ls := loadsave.New(storer.ChunkStore(), storer.Cache(), pipelineFactory(storer.Cache(), false))

	h, _ := dynamicaccess.NewHistory(ls, nil)

	testActRef1 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd891"))
	firstTime := time.Date(1994, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	h.Add(ctx, testActRef1, &firstTime)

	_, err := h.Store(ctx)
	assert.NoError(t, err)
}

func pipelineFactory(s storage.Putter, encrypt bool) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(context.Background(), s, encrypt, 0)
	}
}
