// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package addressbook provides persisted mapping between overlay (topology) address
and bzz.Address address, which contains underlay (physical) address.
The underlay address contains both physical and p2p addresses.

It is single point of truth about known peers and relations of their
overlay and underlay addresses.
*/
package addressbook
