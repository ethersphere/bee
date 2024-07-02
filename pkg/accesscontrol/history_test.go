// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/storage"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
)

func TestHistoryAdd(t *testing.T) {
	t.Parallel()
	h, err := accesscontrol.NewHistory(nil)
	assertNoError(t, "create history", err)

	addr := swarm.NewAddress([]byte("addr"))

	ctx := context.Background()

	err = h.Add(ctx, addr, nil, nil)
	assertNoError(t, "history add", err)
}

func TestSingleNodeHistoryLookup(t *testing.T) {
	t.Parallel()
	storer := mockstorer.New()
	ctx := context.Background()
	ls := loadsave.New(storer.ChunkStore(), storer.Cache(), pipelineFactory(storer.Cache(), false))

	h, err := accesscontrol.NewHistory(ls)
	assertNoError(t, "create history", err)

	testActRef := swarm.RandAddress(t)
	err = h.Add(ctx, testActRef, nil, nil)
	assertNoError(t, "history add", err)

	_, err = h.Store(ctx)
	assertNoError(t, "store history", err)

	searchedTime := time.Now().Unix()
	entry, err := h.Lookup(ctx, searchedTime)
	assertNoError(t, "history lookup", err)
	actRef := entry.Reference()
	assert.True(t, actRef.Equal(testActRef))
	assert.Nil(t, entry.Metadata())
}

func TestMultiNodeHistoryLookup(t *testing.T) {
	t.Parallel()
	storer := mockstorer.New()
	ctx := context.Background()
	ls := loadsave.New(storer.ChunkStore(), storer.Cache(), pipelineFactory(storer.Cache(), false))

	h, err := accesscontrol.NewHistory(ls)
	assertNoError(t, "create history", err)

	testActRef1 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd891"))
	firstTime := time.Date(1994, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	mtdt1 := map[string]string{"firstTime": "1994-04-01"}
	err = h.Add(ctx, testActRef1, &firstTime, &mtdt1)
	assertNoError(t, "1st history add", err)

	testActRef2 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd892"))
	secondTime := time.Date(2000, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	mtdt2 := map[string]string{"secondTime": "2000-04-01"}
	err = h.Add(ctx, testActRef2, &secondTime, &mtdt2)
	assertNoError(t, "2nd history add", err)

	testActRef3 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd893"))
	thirdTime := time.Date(2015, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	mtdt3 := map[string]string{"thirdTime": "2015-04-01"}
	err = h.Add(ctx, testActRef3, &thirdTime, &mtdt3)
	assertNoError(t, "3rd history add", err)

	testActRef4 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd894"))
	fourthTime := time.Date(2020, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	mtdt4 := map[string]string{"fourthTime": "2020-04-01"}
	err = h.Add(ctx, testActRef4, &fourthTime, &mtdt4)
	assertNoError(t, "4th history add", err)

	testActRef5 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd895"))
	fifthTime := time.Date(2030, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	mtdt5 := map[string]string{"fifthTime": "2030-04-01"}
	err = h.Add(ctx, testActRef5, &fifthTime, &mtdt5)
	assertNoError(t, "5th history add", err)

	// latest
	searchedTime := time.Date(1980, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	entry, err := h.Lookup(ctx, searchedTime)
	assertNoError(t, "1st history lookup", err)
	actRef := entry.Reference()
	assert.True(t, actRef.Equal(testActRef1))
	assert.True(t, reflect.DeepEqual(mtdt1, entry.Metadata()))

	// before first time
	searchedTime = time.Date(2021, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	entry, err = h.Lookup(ctx, searchedTime)
	assertNoError(t, "2nd history lookup", err)
	actRef = entry.Reference()
	assert.True(t, actRef.Equal(testActRef4))
	assert.True(t, reflect.DeepEqual(mtdt4, entry.Metadata()))

	// same time
	searchedTime = time.Date(2000, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	entry, err = h.Lookup(ctx, searchedTime)
	assertNoError(t, "3rd history lookup", err)
	actRef = entry.Reference()
	assert.True(t, actRef.Equal(testActRef2))
	assert.True(t, reflect.DeepEqual(mtdt2, entry.Metadata()))

	// after time
	searchedTime = time.Date(2045, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	entry, err = h.Lookup(ctx, searchedTime)
	assertNoError(t, "4th history lookup", err)
	actRef = entry.Reference()
	assert.True(t, actRef.Equal(testActRef5))
	assert.True(t, reflect.DeepEqual(mtdt5, entry.Metadata()))
}

func TestHistoryStore(t *testing.T) {
	t.Parallel()
	storer := mockstorer.New()
	ctx := context.Background()
	ls := loadsave.New(storer.ChunkStore(), storer.Cache(), pipelineFactory(storer.Cache(), false))

	h1, err := accesscontrol.NewHistory(ls)
	assertNoError(t, "create history", err)

	testActRef1 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd891"))
	firstTime := time.Date(1994, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	mtdt1 := map[string]string{"firstTime": "1994-04-01"}
	err = h1.Add(ctx, testActRef1, &firstTime, &mtdt1)
	assertNoError(t, "history add", err)

	href1, err := h1.Store(ctx)
	assertNoError(t, "store history", err)

	h2, err := accesscontrol.NewHistoryReference(ls, href1)
	assertNoError(t, "create history ref", err)

	entry1, err := h2.Lookup(ctx, firstTime)
	assertNoError(t, "history lookup", err)
	actRef1 := entry1.Reference()
	assert.True(t, actRef1.Equal(testActRef1))
	assert.True(t, reflect.DeepEqual(mtdt1, entry1.Metadata()))
}

func pipelineFactory(s storage.Putter, encrypt bool) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(context.Background(), s, encrypt, 0)
	}
}
