// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package kademlia provides an implementation of the topology.Driver interface
in a way that a kademlia connectivity is actively maintained by the node.

A thorough explanation of the logic in the `manage()` forever loop:
The `manageC` channel gets triggered every time there's a change in the
information regarding peers we know about. This can be a result of: (1) A peer
has disconnected from us (2) A peer has been added to the list of
known peers (from discovery, debugapi, bootnode flag or just because it
was persisted in the address book and the node has been restarted).

So the information has been changed, and potentially upon disconnection,
the depth can travel to a shallower depth in result.
If a peer gets added through AddPeers, this does not necessarily infer
an immediate depth change, since the peer might end up in the backlog for
a long time until we actually need to connect to her.

The `manage()` forever-loop will connect to peers in order from shallower
to deeper depths. This is because of depth calculation method that prioritizes empty bins
That are shallower than depth. An in-depth look at `recalcDepth()` method
will clarify this (more below). So if we will connect to peers from deeper
to shallower depths, all peers in all bins will qualify as peers we'd like
to connect to (see `binSaturated` method), ending up connecting to everyone we know about.

Another important notion one must observe while inspecting how `manage()`
works, is that when we connect to peers depth can only move in one direction,
which is deeper. So this becomes our strategy and we operate with this in mind,
this is also why we iterate from shallower to deeper - since additional
connections to peers for whatever reason can only result in increasing depth.

Empty intermediate bins should be eliminated by the `binSaturated` method indicating
a bin size too short, which in turn means that connections should be established
within this bin. Empty bins have special status in terms of depth calculation
and as mentioned before they are prioritized over deeper, non empty bins and
they constitute as the node's depth when the latter is recalculated.
For the rationale behind this please refer to the appropriate chapters in the book of Swarm.

A special case of the `manage()` functionality is that when we iterate over
peers and we come across a peer that has PO >= depth, we would always like
to connect to that peer. This should always be enforced within the bounds of
the `binSaturated` function and guarantees an ever increasing kademlia depth
in an ever-increasing size of Swarm, resulting in smaller areas of responsibility
for the nodes, maintaining a general upper bound of the assigned nominal
area of responsibility in terms of actual storage requirement. See book of Swarm for more details.

Worth to note is that `manage()` will always try to initiate connections when
a bin is not saturated, however currently it will not try to eliminate connections
on bins which might be over-saturated. Ideally it should be very cheap to maintain a
connection to a peer in a bin, so we should theoretically not aspire to eliminate connections prematurely.
It is also safe to assume we will always have more than the lower bound of peers in a bin, why?
(1) Initially, we will always try to satisfy our own connectivity requirement to saturate the bin
(2) Later on, other peers will get notified about our advertised address and
will try to connect to us in order to satisfy their own connectivity thresholds

We should allow other nodes to dial in, in order to help them maintain a healthy topolgy.
It could be, however, that we would need to mark-and-sweep certain connections once a
theorical upper bound has been reached.

Depth calculation explained:
When we calculate depth we must keep in mind the following constraints:
(1) A nearest-neighborhood constitutes of an arbitrary lower bound of the
closest peers we know about, this is defined in `nnLowWatermark` and is currently set to `2`
(2) Empty bins which are shallower than depth constitute as the node's area of responsibility

As of such, we would calculate depth in the following manner:
(1) Iterate over all peers we know about, from deepest (closest) to shallowest, and count until we reach `nnLowWatermark`
(2) Once we reach `nnLowWatermark`, mark current bin as depth candidate
(3) Iterate over all bins from shallowest to deepest, and look for the shallowest empty bin
(4) If the shallowest empty bin is shallower than the depth candidate - select shallowest bin as depth, otherwise select the candidate

Note: when we are connected to less or equal to `nnLowWatermark` peers, the
depth will always be considered `0`, thus a short-circuit is handling this edge
case explicitly in the `recalcDepth` method.

TODO: add pseudo-code how to calculate depth.

A few examples to depth calculation:

1. empty kademlia
bin | nodes
-------------
==DEPTH==
0			0
1			0
2			0
3			0
4			0
depth: 0


2. less or equal to two peers (nnLowWatermark=2) (a)
bin | nodes
-------------
==DEPTH==
0			1
1			1
2			0
3			0
4			0
depth: 0

3. less or equal to two peers (nnLowWatermark=2) (b)
bin | nodes
-------------
==DEPTH==
0			1
1			0
2			1
3			0
4			0
depth: 0

4. less or equal to two peers (nnLowWatermark=2) (c)
bin | nodes
-------------
==DEPTH==
0			2
1			0
2			0
3			0
4			0
depth: 0

5. empty shallow bin
bin | nodes
-------------
0			1
==DEPTH==
1			0
2			1
3			1
4			0
depth: 1 (depth candidate is 2, but 1 is shallower and empty)

6. no empty shallower bin, depth after nnLowerWatermark found
bin | nodes
-------------
0			1
1			1
==DEPTH==
2			1
3			1
4			0
depth: 2 (depth candidate is 2, shallowest empty bin is 4)

7. last bin size >= nnLowWatermark
bin | nodes
-------------
0			1
1			1
2			1
==DEPTH==
3			3
4			0
depth: 3 (depth candidate is 3, shallowest empty bin is 4)

8. all bins full
bin | nodes
-------------
0			1
1			1
2			1
3			3
==DEPTH==
4			2
depth: 4 (depth candidate is 4, no empty bins)
*/
package kademlia
