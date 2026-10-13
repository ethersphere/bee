// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// seat is a key the admin may roster.
type seat struct {
	signer crypto.Signer
	addr   []byte
}

func (e *env) newSeat() seat {
	e.t.Helper()
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		e.t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	addr, err := signer.EthereumAddress()
	if err != nil {
		e.t.Fatal(err)
	}
	return seat{signer: signer, addr: addr.Bytes()}
}

// joinAs joins as the seat's address from a fresh overlay.
func (e *env) joinAs(st seat) (bps.Session, swarm.Address) {
	e.t.Helper()
	overlay := swarm.RandAddress(e.t)
	c, _ := e.client(overlay)
	return e.join(c, st.addr), overlay
}

// joinSeated joins as the seat's address knowing it rostered — as a grantee
// that saw itself on a roster rejoins with the Position of that session.
func (e *env) joinSeated(st seat) (bps.Session, swarm.Address) {
	e.t.Helper()
	overlay := swarm.RandAddress(e.t)
	c, _ := e.client(overlay)
	s, err := c.Join(context.Background(), bps.JoinRequest{
		Broker:   e.brokerAddr,
		Spec:     bps.CohortSpec{Topic: e.topic, Principal: e.principal},
		Identity: st.addr,
		Position: bps.Position{Roster: [][]byte{st.addr}},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = s.Close() })
	return s, overlay
}

// grant publishes a roster and waits until the broker has accepted it.
func (e *env) grant(admin bps.Session, index uint64, seats ...seat) {
	e.t.Helper()
	probe := e.subscribe()
	e.roster(admin, index, seats...)
	for {
		m := <-probe.Messages()
		if m.Kind == bps.KindRoster && m.Index == index {
			return
		}
	}
}

// publishAs signs a chunk with the seat's key under the session's challenge
// and publishes it.
func (e *env) publishAs(st seat, s bps.Session, kind bps.Kind, index uint64, payload []byte) []byte {
	e.t.Helper()
	c := e.chunk(st.signer, kind, s.Challenge(), index, payload)
	if err := s.Publish(context.Background(), kind, index, c); err != nil {
		e.t.Fatal(err)
	}
	return c
}

// roster publishes the admin's roster at index on the admin's publisher stream.
func (e *env) roster(admin bps.Session, index uint64, seats ...seat) []byte {
	e.t.Helper()
	addrs := make([][]byte, 0, len(seats))
	for _, st := range seats {
		addrs = append(addrs, st.addr)
	}
	payload, err := bps.RosterPayload(addrs)
	if err != nil {
		e.t.Fatal(err)
	}
	return e.publish(admin, bps.KindRoster, index, payload)
}

func waitGranted(t *testing.T, s bps.Session) {
	t.Helper()
	select {
	case <-s.Granted():
	case <-time.After(waitFor):
		t.Fatal("not granted")
	}
}

func expectKind(t *testing.T, s bps.Session, kind bps.Kind, owner []byte, index uint64) bps.Message {
	t.Helper()
	select {
	case m, ok := <-s.Messages():
		if !ok {
			t.Fatalf("stream ended: %v", s.Err())
		}
		if m.Kind != kind || !bytes.Equal(m.Owner, owner) || m.Index != index {
			t.Fatalf("got kind %d owner %x index %d, want kind %d owner %x index %d", m.Kind, m.Owner, m.Index, kind, owner, index)
		}
		return m
	case <-time.After(waitFor):
		t.Fatalf("timed out waiting for kind %d index %d", kind, index)
	}
	return bps.Message{}
}

func TestGrant(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	sub := e.subscribe()
	admin, _ := e.publisher()

	e.roster(admin, 0, st)
	m := expectKind(t, sub, bps.KindRoster, e.principal, 0)
	if len(m.Roster) != 1 || !bytes.Equal(m.Roster[0], st.addr) {
		t.Fatalf("roster %x", m.Roster)
	}

	// the seat's stream is pending: the roster naming it is its cue
	seatSess, _ := e.joinSeated(st)
	expectKind(t, seatSess, bps.KindRoster, e.principal, 0)
	waitGranted(t, seatSess)
	e.publishAs(st, seatSess, bps.KindAuth, 0, nil)
	c0 := e.publishAs(st, seatSess, bps.KindData, 0, []byte("from the seat"))

	got := expectKind(t, sub, bps.KindData, st.addr, 0)
	if !bytes.Equal(got.SOC, c0) {
		t.Fatal("wrong chunk")
	}
	// the admin's publisher stream receives the seat's update; the seat not its own
	expectKind(t, admin, bps.KindData, st.addr, 0)
	a0 := e.publish(admin, bps.KindData, 0, []byte("from the admin"))
	got = expectKind(t, seatSess, bps.KindData, e.principal, 0)
	if !bytes.Equal(got.SOC, a0) {
		t.Fatal("wrong chunk")
	}
	expectKind(t, sub, bps.KindData, e.principal, 0)
	expectNoMessage(t, admin)
}

func TestSpectatorUpgrade(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()
	sub := e.subscribe()

	// not rostered: a spectator, which may not publish
	spectator, _ := e.joinAs(st)
	if err := spectator.Publish(context.Background(), bps.KindAuth, 0, nil); !errors.Is(err, bps.ErrNotPublisher) {
		t.Fatalf("got %v, want %v", err, bps.ErrNotPublisher)
	}

	// a roster naming it reaches the spectator; it stays a spectator, and
	// rejoins to publish
	e.roster(admin, 0, st)
	m := expectKind(t, spectator, bps.KindRoster, e.principal, 0)
	if len(m.Roster) != 1 || !bytes.Equal(m.Roster[0], st.addr) {
		t.Fatalf("roster %x", m.Roster)
	}
	select {
	case <-spectator.Granted():
		t.Fatal("a spectator stream is never granted")
	default:
	}

	seatSess, _ := e.joinSeated(st)
	expectKind(t, seatSess, bps.KindRoster, e.principal, 0)
	waitGranted(t, seatSess)
	e.publishAs(st, seatSess, bps.KindAuth, 0, nil)
	c := e.publishAs(st, seatSess, bps.KindData, 5, []byte("upgraded"))
	expectKind(t, sub, bps.KindRoster, e.principal, 0)
	got := expectKind(t, sub, bps.KindData, st.addr, 5)
	if !bytes.Equal(got.SOC, c) {
		t.Fatal("wrong chunk")
	}
	// the old spectator stream is bound to the same identity: its own update
	// is not delivered there either
	expectNoMessage(t, spectator)
}

func TestNamedSpectatorAuth(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()

	// joined before the grant: a spectator, which never authenticates in place
	overlay := swarm.RandAddress(t)
	w, r, ack := e.rawFrom(overlay, &pb.Join{Cohort: e.spec(), Identity: st.addr})
	if ack.Status != pb.Status_OK {
		t.Fatalf("status %s", ack.Status)
	}
	e.roster(admin, 0, st)
	var ros pb.Broadcast
	if err := r.ReadMsg(&ros); err != nil || ros.Kind != pb.Kind_ROSTER {
		t.Fatalf("want the roster, got %s %v", ros.Kind, err)
	}
	auth := e.chunk(st.signer, bps.KindAuth, ack.Challenge, 0, nil)
	if err := w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_AUTH, Challenge: ack.Challenge, Soc: auth}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return e.bl.blocklisted(overlay) && e.counters().WrongStream == 1 })
}

func TestUnrosteredAuth(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	_, _ = e.publisher()
	overlay := swarm.RandAddress(t)
	w, _, ack := e.rawFrom(overlay, &pb.Join{Cohort: e.spec(), Identity: st.addr})
	if ack.Status != pb.Status_OK {
		t.Fatalf("status %s", ack.Status)
	}
	auth := e.chunk(st.signer, bps.KindAuth, ack.Challenge, 0, nil)
	if err := w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_AUTH, Challenge: ack.Challenge, Soc: auth}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return e.bl.blocklisted(overlay) && e.counters().WrongStream == 1 })
}

func TestRevocation(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()
	sub := e.subscribe()
	e.roster(admin, 0, st)
	expectKind(t, sub, bps.KindRoster, e.principal, 0)

	// the seat authenticates on a raw stream, to send after its revocation
	overlay := swarm.RandAddress(t)
	w, r, ack := e.rawFrom(overlay, &pb.Join{Cohort: e.spec(), Identity: st.addr})
	if ack.Status != pb.Status_OK {
		t.Fatalf("status %s", ack.Status)
	}
	var cue pb.Broadcast
	if err := r.ReadMsg(&cue); err != nil || cue.Kind != pb.Kind_ROSTER {
		t.Fatalf("want the roster as the cue, got %s %v", cue.Kind, err)
	}
	send := func(kind bps.Kind, index uint64, payload []byte) {
		t.Helper()
		c := e.chunk(st.signer, kind, ack.Challenge, index, payload)
		f := &pb.Broadcast{Kind: pb.Kind(kind), Challenge: ack.Challenge, Index: index, Soc: c}
		_ = w.WriteMsg(f)
	}
	send(bps.KindAuth, 0, nil)
	send(bps.KindData, 0, []byte("before"))
	expectKind(t, sub, bps.KindData, st.addr, 0)

	// the reduced roster revokes the seat: the stream is written that roster
	// and closed; what it sends meanwhile is dropped, unpunished
	e.grant(admin, 1)
	send(bps.KindData, 1, []byte("in flight"))
	m := expectKind(t, sub, bps.KindRoster, e.principal, 1)
	if len(m.Roster) != 0 {
		t.Fatalf("roster %x", m.Roster)
	}
	var next pb.Broadcast
	if err := r.ReadMsg(&next); err != nil || next.Kind != pb.Kind_ROSTER || next.Index != 1 {
		t.Fatalf("want the reduced roster, got %s %d %v", next.Kind, next.Index, err)
	}
	if err := r.ReadMsg(&next); err == nil {
		t.Fatalf("want the stream closed, got %s", next.Kind)
	}
	expectNoMessage(t, sub)
	if e.bl.blocklisted(overlay) || e.counters().WrongStream != 0 {
		t.Fatal("a revoked peer is not punished")
	}

	// the subscriber no longer admits the seat
	if pos := sub.Position(); len(pos.Roster) != 0 || pos.RosterCursor != 2 {
		t.Fatalf("position %+v", pos)
	}

	// rejoining, the seat is a spectator
	spectator, _ := e.joinAs(st)
	expectKind(t, spectator, bps.KindRoster, e.principal, 1)
	select {
	case <-spectator.Granted():
		t.Fatal("a revoked identity is not granted")
	default:
	}
}

func TestRevokedSession(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()
	e.grant(admin, 0, st)
	seatSess, _ := e.joinSeated(st)
	expectKind(t, seatSess, bps.KindRoster, e.principal, 0)
	waitGranted(t, seatSess)
	e.publishAs(st, seatSess, bps.KindAuth, 0, nil)

	e.roster(admin, 1)
	expectKind(t, seatSess, bps.KindRoster, e.principal, 1)
	expectDone(t, seatSess)
	if err := seatSess.Publish(context.Background(), bps.KindData, 0, nil); !errors.Is(err, bps.ErrNotPublisher) {
		t.Fatalf("got %v, want %v", err, bps.ErrNotPublisher)
	}
}

func TestCursorsPerPublisher(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()
	sub := e.subscribe()
	e.roster(admin, 0, st)
	expectKind(t, sub, bps.KindRoster, e.principal, 0)
	seatSess, _ := e.joinSeated(st)
	waitGranted(t, seatSess)
	e.publishAs(st, seatSess, bps.KindAuth, 0, nil)

	// each publisher has its own cursor
	e.publish(admin, bps.KindData, 3, []byte("a3"))
	expectKind(t, sub, bps.KindData, e.principal, 3)
	e.publishAs(st, seatSess, bps.KindData, 0, []byte("s0"))
	expectKind(t, sub, bps.KindData, st.addr, 0)
	// below its own cursor: a retransmit
	e.publishAs(st, seatSess, bps.KindData, 0, []byte("s0"))
	eventually(t, func() bool { return e.counters().Retransmit == 1 })
	expectNoMessage(t, sub)

	pos := sub.Position()
	if pos.Cursors[string(e.principal)] != 4 || pos.Cursors[string(st.addr)] != 1 {
		t.Fatalf("cursors %v", pos.Cursors)
	}
}

func TestRosterFeed(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()
	sub := e.subscribe()

	e.roster(admin, 0, st)
	expectKind(t, sub, bps.KindRoster, e.principal, 0)
	// a roster below the roster cursor is a retransmit
	e.roster(admin, 0, st)
	eventually(t, func() bool { return e.counters().Retransmit == 1 })

	// a valid chunk whose payload is not a roster changes nothing
	e.publish(admin, bps.KindRoster, 1, []byte{0x0a, 0x02, 0x01, 0x02})
	eventually(t, func() bool { return e.counters().InvalidRoster == 1 })
	e.roster(admin, 1)
	expectKind(t, sub, bps.KindRoster, e.principal, 1)

	// a late joiner receives the latest roster first
	late := e.subscribe()
	m := expectKind(t, late, bps.KindRoster, e.principal, 1)
	if len(m.Roster) != 0 {
		t.Fatalf("roster %x", m.Roster)
	}
}

func TestServiceKindsAdminOnly(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()
	e.grant(admin, 0, st)

	overlay := swarm.RandAddress(t)
	w, r, ack := e.rawFrom(overlay, &pb.Join{Cohort: e.spec(), Identity: st.addr})
	var cue pb.Broadcast
	if err := r.ReadMsg(&cue); err != nil {
		t.Fatal(err)
	}
	auth := e.chunk(st.signer, bps.KindAuth, ack.Challenge, 0, nil)
	if err := w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_AUTH, Challenge: ack.Challenge, Soc: auth}); err != nil {
		t.Fatal(err)
	}
	payload, _ := bps.RosterPayload([][]byte{st.addr})
	ros := e.chunk(st.signer, bps.KindRoster, ack.Challenge, 1, payload)
	if err := w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_ROSTER, Challenge: ack.Challenge, Index: 1, Soc: ros}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return e.bl.blocklisted(overlay) && e.counters().WrongStream == 1 })
}

func TestEndOfStream(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	admin, _ := e.publisher()
	sub := e.subscribe()
	e.publish(admin, bps.KindData, 0, []byte("last words"))
	expectKind(t, sub, bps.KindData, e.principal, 0)

	e.publish(admin, bps.KindEOS, 0, nil)
	expectKind(t, sub, bps.KindEOS, e.principal, 0)
	expectDone(t, sub)
	if !errors.Is(sub.Err(), bps.ErrEndOfStream) {
		t.Fatalf("got %v, want %v", sub.Err(), bps.ErrEndOfStream)
	}
	expectDone(t, admin)
	eventually(t, func() bool {
		_, ok := e.broker.Counters(bps.CohortSpec{Topic: e.topic, Principal: e.principal})
		return !ok
	})

	// a channel the subscriber has seen end is not rejoined
	pos := sub.Position()
	if !pos.Ended {
		t.Fatal("not ended")
	}
	c, _ := e.client(swarm.RandAddress(t))
	addr := make([]byte, 20)
	_, err := c.Join(context.Background(), bps.JoinRequest{Broker: e.brokerAddr, Spec: bps.CohortSpec{Topic: e.topic, Principal: e.principal}, Identity: addr, Position: pos})
	if !errors.Is(err, bps.ErrChannelEnded) {
		t.Fatalf("got %v, want %v", err, bps.ErrChannelEnded)
	}
}

func TestPublisherStreamsBound(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{MaxPublisherStreams: 2})
	st := e.newSeat()
	admin, _ := e.publisher()
	e.grant(admin, 0, st)

	sess := make([]bps.Session, 0, 3)
	for range 3 {
		s, _ := e.joinSeated(st)
		go func() {
			for range s.Messages() {
			}
		}()
		waitGranted(t, s)
		e.publishAs(st, s, bps.KindAuth, 0, nil)
		sess = append(sess, s)
		time.Sleep(50 * time.Millisecond)
	}
	// the third auth resets the oldest
	expectDone(t, sess[0])
	select {
	case <-sess[1].Done():
		t.Fatal("second stream reset")
	case <-sess[2].Done():
		t.Fatal("third stream reset")
	default:
	}
}
