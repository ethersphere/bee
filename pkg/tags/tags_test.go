// Copyright 2019 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package tags

import (
	"context"
	"errors"
	"io/ioutil"
	"sort"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestAll(t *testing.T) {
	mockStatestore := statestore.NewStateStore()
	logger := logging.New(ioutil.Discard, 0)
	ts := NewTags(mockStatestore, logger)
	if _, err := ts.Create(1); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Create(1); err != nil {
		t.Fatal(err)
	}

	all := ts.All()

	if len(all) != 2 {
		t.Fatalf("expected length to be 2 got %d", len(all))
	}

	if n := all[0].TotalCounter(); n != 1 {
		t.Fatalf("expected tag 0 Total to be 1 got %d", n)
	}

	if n := all[1].TotalCounter(); n != 1 {
		t.Fatalf("expected tag 1 Total to be 1 got %d", n)
	}

	if _, err := ts.Create(1); err != nil {
		t.Fatal(err)
	}
	all = ts.All()

	if len(all) != 3 {
		t.Fatalf("expected length to be 3 got %d", len(all))
	}
}

func TestListAll(t *testing.T) {
	mockStatestore := statestore.NewStateStore()
	logger := logging.New(ioutil.Discard, 0)

	ts1 := NewTags(mockStatestore, logger)

	// create few tags
	for i := 0; i < 5; i++ {
		if _, err := ts1.Create(1); err != nil {
			t.Fatal(err)
		}
	}

	// tags are from sync.Map
	tagList1, err := ts1.ListAll(context.Background(), 0, 5)
	if err != nil {
		t.Fatal(err)
	}

	if len(tagList1) != 5 {
		t.Fatalf("want %d tags but got %d", 5, len(tagList1))
	}

	// save all returned tags to statestore
	for _, tag := range tagList1 {
		err = tag.saveTag()
		if err != nil {
			t.Fatal(err)
		}
	}

	// This sleep is needed because otherwise in test the ts2 newtags gets the same seed as ts1 (happening in the same second in the test),
	// which in the test results in the same uids already existing in statestore that the "create few more tags" creates below in sync.Map
	// this highlights that upon tags.Create(), already existing values are only checked in sync.Map but not in statestore

	time.Sleep(1 * time.Second)

	// use new tags object
	ts2 := NewTags(mockStatestore, logger)

	// create few more tags in new tags object
	for i := 0; i < 5; i++ {
		if _, err := ts2.Create(1); err != nil {
			t.Fatal(err)
		}
	}

	// first tags are from sync.Map
	tagList2, err := ts2.ListAll(context.Background(), 0, 5)
	if err != nil {
		t.Fatal(err)
	}

	if len(tagList2) != 5 {
		t.Fatalf("want %d tags but got %d", 5, len(tagList2))
	}

	// now tags are returned from statestore
	tagList3, err := ts2.ListAll(context.Background(), 5, 5)
	if err != nil {
		t.Fatal(err)
	}

	if len(tagList3) != 5 {
		t.Fatalf("want %d tags but got %d", 5, len(tagList3))
	}

	// where they are not sorted
	sort.Slice(tagList3, func(i, j int) bool { return tagList3[i].Uid < tagList3[j].Uid })

	// and are the same as ones returned from first tags object
	for i := range tagList3 {
		if tagList1[i].Uid != tagList3[i].Uid {
			t.Fatalf("expected tag %d, but got %d", tagList1[i].Uid, tagList3[i].Uid)
		}
	}
}

func TestPersistence(t *testing.T) {
	mockStatestore := statestore.NewStateStore()
	logger := logging.New(ioutil.Discard, 0)
	ts := NewTags(mockStatestore, logger)
	ta, err := ts.Create(1)
	if err != nil {
		t.Fatal(err)
	}
	ta.Total = 10
	ta.Seen = 2
	ta.Split = 10
	ta.Stored = 8
	_, err = ta.DoneSplit(swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	// simulate node closing down and booting up
	err = ts.Close()
	if err != nil {
		t.Fatal(err)
	}
	ts = NewTags(mockStatestore, logger)

	// Get the tag after the node bootup
	rcvd1, err := ts.Get(ta.Uid)
	if err != nil {
		t.Fatal(err)
	}

	// check if the values ae intact after the bootup
	if ta.Uid != rcvd1.Uid {
		t.Fatalf("invalid uid: expected %d got %d", ta.Uid, rcvd1.Uid)
	}
	if ta.Total != rcvd1.Total {
		t.Fatalf("invalid total: expected %d got %d", ta.Total, rcvd1.Total)
	}

	// See if tag is saved after syncing is over
	for i := 0; i < 8; i++ {
		err := ta.Inc(StateSent)
		if err != nil {
			t.Fatal(err)
		}
		err = ta.Inc(StateSynced)
		if err != nil {
			t.Fatal(err)
		}
	}

	// simulate node closing down and booting up
	err = ts.Close()
	if err != nil {
		t.Fatal(err)
	}
	ts = NewTags(mockStatestore, logger)

	// get the tag after the node boot up
	rcvd2, err := ts.Get(ta.Uid)
	if err != nil {
		t.Fatal(err)
	}

	// check if the values ae intact after the bootup
	if ta.Uid != rcvd2.Uid {
		t.Fatalf("invalid uid: expected %d got %d", ta.Uid, rcvd2.Uid)
	}
	if ta.Total != rcvd2.Total {
		t.Fatalf("invalid total: expected %d got %d", ta.Total, rcvd2.Total)
	}
	if ta.Sent != rcvd2.Sent {
		t.Fatalf("invalid sent: expected %d got %d", ta.Sent, rcvd2.Sent)
	}
	if ta.Synced != rcvd2.Synced {
		t.Fatalf("invalid synced: expected %d got %d", ta.Synced, rcvd2.Synced)
	}

	// regression test case: make sure that a persisted tag
	// is flushed from statestore on Delete
	ts.Delete(ta.Uid)

	// simulate node closing down and booting up
	err = ts.Close()
	if err != nil {
		t.Fatal(err)
	}
	ts = NewTags(mockStatestore, logger)

	// get the tag after the node boot up
	_, err = ts.Get(ta.Uid)
	if !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
