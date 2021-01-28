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
	"io/ioutil"
	"testing"

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
}
