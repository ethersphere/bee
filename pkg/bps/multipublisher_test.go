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
	"github.com/ethersphere/bee/v2/pkg/cac"
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

// grant publishes a roster and waits until the broker has accepted it.
func (e *env) grant(admin bps.Session, index uint64, seats ...seat) {
	e.t.Helper()
	probe := e.subscribe()
	e.roster(admin, index, seats...)
	for {
		select {
		case m := <-probe.Messages():
			if m.Kind == bps.KindRoster && m.Index == index {
				return
			}
		case <-probe.Done():
			e.t.Fatalf("waiting for roster %d: %v", index, probe.Err())
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
	payload, err := bps.EncodeRosterMsg(addrs)
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
	case m := <-s.Messages():
		if m.Kind != kind || !bytes.Equal(m.Owner, owner) || m.Index != index {
			t.Fatalf("got kind %d owner %x index %d, want kind %d owner %x index %d", m.Kind, m.Owner, m.Index, kind, owner, index)
		}
		return m
	case <-s.Done():
		t.Fatalf("stream ended: %v", s.Err())
	case <-time.After(waitFor):
		t.Fatalf("timed out waiting for kind %d index %d", kind, index)
	}
	return bps.Message{}
}

// drain discards a session's deliveries, so that its stream keeps reading.
func drain(s bps.Session) {
	go func() {
		for {
			select {
			case <-s.Messages():
			case <-s.Done():
				return
			}
		}
	}()
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
	// the roster is delivered back to the admin: it admits by it from there
	expectKind(t, admin, bps.KindRoster, e.principal, 0)

	// the seat joins as a spectator; the latest roster, naming it, is its cue
	seatSess, _ := e.joinAs(st)
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

// TestSeatBeforeRoster is the startup order the review found: the seat joins
// before the broker holds any roster, and is upgraded in place by its AUTH
// once the first roster names it.
func TestSeatBeforeRoster(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	sub := e.subscribe()

	seatSess, overlay := e.joinAs(st)
	admin, _ := e.publisher()
	drain(admin)
	e.roster(admin, 0, st)
	expectKind(t, seatSess, bps.KindRoster, e.principal, 0)
	waitGranted(t, seatSess)
	e.publishAs(st, seatSess, bps.KindAuth, 0, nil)
	c := e.publishAs(st, seatSess, bps.KindData, 5, []byte("upgraded in place"))
	expectKind(t, sub, bps.KindRoster, e.principal, 0)
	got := expectKind(t, sub, bps.KindData, st.addr, 5)
	if !bytes.Equal(got.SOC, c) {
		t.Fatal("wrong chunk")
	}
	if e.bl.blocklisted(overlay) || e.counters().WrongStream != 0 {
		t.Fatal("an upgrade in place is not a violation")
	}
}

// TestRosteredListens: a rostered address that never authenticates is a
// spectator like any other, and is not disconnected at the auth deadline.
func TestRosteredListens(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{AuthWaitTimeout: 100 * time.Millisecond})
	st := e.newSeat()
	admin, _ := e.publisher()
	drain(admin)
	e.grant(admin, 0, st)

	listener, overlay := e.joinAs(st)
	expectKind(t, listener, bps.KindRoster, e.principal, 0)
	time.Sleep(300 * time.Millisecond)
	e.publish(admin, bps.KindData, 0, []byte("heard"))
	expectKind(t, listener, bps.KindData, e.principal, 0)
	if e.bl.blocklisted(overlay) || e.counters().AuthTimeout != 0 {
		t.Fatal("a listening rostered address is not timed out")
	}
}

func TestUnrosteredAuth(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	_, _ = e.publisher()
	overlay := swarm.RandAddress(t)
	w, _, ack := e.rawFrom(overlay, &pb.Join{Cohort: e.spec(), Owner: st.addr})
	if ack.Status != pb.Status_OK {
		t.Fatalf("status %s", ack.Status)
	}
	auth := e.chunk(st.signer, bps.KindAuth, ack.Challenge, 0, nil)
	if err := w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_AUTH, Challenge: ack.Challenge, Soc: auth}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return e.bl.blocklisted(overlay) && e.counters().WrongStream == 1 })
}

// TestRevokedSpectatorAuth: a spectator sends its AUTH after the roster naming
// it, but a roster revoking it reaches the broker first: the stream is closed,
// nothing is punished.
func TestRevokedSpectatorAuth(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()
	drain(admin)
	e.grant(admin, 0, st)

	overlay := swarm.RandAddress(t)
	w, r, ack := e.rawFrom(overlay, &pb.Join{Cohort: e.spec(), Owner: st.addr})
	var cue pb.Broadcast
	if err := r.ReadMsg(&cue); err != nil || cue.Kind != pb.Kind_ROSTER {
		t.Fatalf("want the roster as the cue, got %s %v", cue.Kind, err)
	}
	e.grant(admin, 1)
	auth := e.chunk(st.signer, bps.KindAuth, ack.Challenge, 0, nil)
	_ = w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_AUTH, Challenge: ack.Challenge, Soc: auth})
	eventually(t, func() bool { return e.counters().Revoked == 1 })
	if e.bl.blocklisted(overlay) || e.counters().WrongStream != 0 {
		t.Fatal("an AUTH racing its revocation is not punished")
	}
}

func TestRevocation(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()
	drain(admin)
	sub := e.subscribe()
	e.roster(admin, 0, st)
	expectKind(t, sub, bps.KindRoster, e.principal, 0)

	// the seat authenticates on a raw stream, to send after its revocation
	overlay := swarm.RandAddress(t)
	w, r, ack := e.rawFrom(overlay, &pb.Join{Cohort: e.spec(), Owner: st.addr})
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
	if pos := sub.Position(); len(pos.Roster) != 0 || pos.RosterCursor != 2 {
		t.Fatalf("position %+v", pos)
	}

	// rejoining, the seat is a spectator that may not authenticate
	spectator, _ := e.joinAs(st)
	expectKind(t, spectator, bps.KindRoster, e.principal, 1)
	select {
	case <-spectator.Granted():
		t.Fatal("a revoked owner is not granted")
	default:
	}
}

// TestRevokeInFlight: the admin revokes a seat whose update the broker accepted
// just before the roster; the admin's own session, admitting by the echoed
// roster, takes the update and is not reset.
func TestRevokeInFlight(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()
	e.roster(admin, 0, st)
	expectKind(t, admin, bps.KindRoster, e.principal, 0)
	seatSess, _ := e.joinAs(st)
	drain(seatSess)
	waitGranted(t, seatSess)
	e.publishAs(st, seatSess, bps.KindAuth, 0, nil)
	e.publishAs(st, seatSess, bps.KindData, 0, []byte("last"))
	time.Sleep(50 * time.Millisecond) // the broker accepts the update first
	e.roster(admin, 1)
	expectKind(t, admin, bps.KindData, st.addr, 0)
	expectKind(t, admin, bps.KindRoster, e.principal, 1)
	select {
	case <-admin.Done():
		t.Fatalf("the admin's stream ended: %v", admin.Err())
	default:
	}
}

func TestRevokedSession(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()
	drain(admin)
	e.grant(admin, 0, st)
	seatSess, _ := e.joinAs(st)
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
	drain(admin)
	sub := e.subscribe()
	e.roster(admin, 0, st)
	expectKind(t, sub, bps.KindRoster, e.principal, 0)
	seatSess, _ := e.joinAs(st)
	drain(seatSess)
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
	drain(admin)
	sub := e.subscribe()

	e.roster(admin, 0, st)
	expectKind(t, sub, bps.KindRoster, e.principal, 0)
	// a roster below the roster cursor is a retransmit
	e.roster(admin, 0, st)
	eventually(t, func() bool { return e.counters().Retransmit == 1 })

	// a valid chunk whose payload is not a roster changes nothing; a session
	// refuses to send one, so it comes from a raw stream of the admin's
	w, _, ack := e.raw(&pb.Join{Cohort: e.spec(), Owner: e.principal})
	auth := e.chunk(e.signer, bps.KindAuth, ack.Challenge, 0, nil)
	if err := w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_AUTH, Challenge: ack.Challenge, Soc: auth}); err != nil {
		t.Fatal(err)
	}
	bad := e.chunk(e.signer, bps.KindRoster, ack.Challenge, 1, []byte{0x0a, 0x02, 0x01, 0x02})
	if err := w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_ROSTER, Challenge: ack.Challenge, Index: 1, Soc: bad}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return e.counters().InvalidRoster == 1 })
	// nor does one with an empty entry, which would name a stream with no owner
	empty := e.chunk(e.signer, bps.KindRoster, ack.Challenge, 1, []byte{0x0a, 0x00})
	if err := w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_ROSTER, Challenge: ack.Challenge, Index: 1, Soc: empty}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return e.counters().InvalidRoster == 2 })
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
	drain(admin)
	e.grant(admin, 0, st)

	overlay := swarm.RandAddress(t)
	w, r, ack := e.rawFrom(overlay, &pb.Join{Cohort: e.spec(), Owner: st.addr})
	var cue pb.Broadcast
	if err := r.ReadMsg(&cue); err != nil {
		t.Fatal(err)
	}
	auth := e.chunk(st.signer, bps.KindAuth, ack.Challenge, 0, nil)
	if err := w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_AUTH, Challenge: ack.Challenge, Soc: auth}); err != nil {
		t.Fatal(err)
	}
	payload, _ := bps.EncodeRosterMsg([][]byte{st.addr})
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
	second, _ := e.pending()
	e.publish(admin, bps.KindData, 0, []byte("last words"))
	expectKind(t, sub, bps.KindData, e.principal, 0)

	e.publish(admin, bps.KindEOS, 0, nil)
	// every stream is told — the sender and a pending one included — and closed
	for _, s := range []bps.Session{sub, admin, second} {
		expectKind(t, s, bps.KindEOS, e.principal, 0)
		expectDone(t, s)
		if !errors.Is(s.Err(), bps.ErrEndOfStream) {
			t.Fatalf("got %v, want %v", s.Err(), bps.ErrEndOfStream)
		}
	}
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
	_, err := c.Join(context.Background(), bps.JoinRequest{Broker: e.brokerAddr, Spec: bps.CohortSpec{Topic: e.topic, Principal: e.principal}, Owner: addr, Position: pos})
	if !errors.Is(err, bps.ErrChannelEnded) {
		t.Fatalf("got %v, want %v", err, bps.ErrChannelEnded)
	}
	// a Join that does not know creates a fresh cohort
	fresh := e.subscribe()
	expectNoMessage(t, fresh)
	if _, ok := e.broker.Counters(bps.CohortSpec{Topic: e.topic, Principal: e.principal}); !ok {
		t.Fatal("no fresh cohort")
	}
}

func TestPublisherStreamsBound(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{MaxPublisherStreams: 2})
	st := e.newSeat()
	admin, _ := e.publisher()
	drain(admin)
	e.grant(admin, 0, st)

	sess := make([]bps.Session, 0, 3)
	for range 3 {
		s, _ := e.joinAs(st)
		drain(s)
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

// TestRosteredPastFullSet: a rostered owner that finds the fan-out set full
// is pending — the roster, nothing else — and authenticates as any grantee;
// one that does not is reset at the auth deadline, not blocklisted.
func TestRosteredPastFullSet(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{MaxSubsPerCohort: 1, AuthWaitTimeout: 300 * time.Millisecond})
	st, idle := e.newSeat(), e.newSeat()
	admin, _ := e.publisher()
	drain(admin)
	probe := e.subscribe() // fills the fan-out set
	e.roster(admin, 0, st, idle)
	expectKind(t, probe, bps.KindRoster, e.principal, 0)

	seatSess, _ := e.joinAs(st)
	expectKind(t, seatSess, bps.KindRoster, e.principal, 0)
	waitGranted(t, seatSess)
	e.publishAs(st, seatSess, bps.KindAuth, 0, nil)
	e.publishAs(st, seatSess, bps.KindData, 0, []byte("past the bound"))
	expectKind(t, probe, bps.KindData, st.addr, 0)

	listener, overlay := e.joinAs(idle)
	expectKind(t, listener, bps.KindRoster, e.principal, 0)
	e.publish(admin, bps.KindData, 0, []byte("not for a pending stream"))
	expectKind(t, probe, bps.KindData, e.principal, 0)
	expectDone(t, listener)
	if e.bl.blocklisted(overlay) || e.counters().AuthTimeout != 1 {
		t.Fatal("a rostered pending stream times out without a blocklist")
	}
}

// TestSeatPublishChecks: a session refuses what its seat may not send yet, or at all.
func TestSeatPublishChecks(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	seatSess, overlay := e.joinAs(st)
	publish := func(kind bps.Kind, payload []byte) error {
		t.Helper()
		return seatSess.Publish(context.Background(), kind, 0, e.chunk(st.signer, kind, seatSess.Challenge(), 0, payload))
	}

	if err := publish(bps.KindAuth, nil); !errors.Is(err, bps.ErrNotPublisher) {
		t.Fatalf("auth before the grant: got %v, want %v", err, bps.ErrNotPublisher)
	}

	admin, _ := e.publisher()
	drain(admin)
	e.roster(admin, 0, st)
	expectKind(t, seatSess, bps.KindRoster, e.principal, 0)
	waitGranted(t, seatSess)

	if err := publish(bps.KindData, []byte("x")); !errors.Is(err, bps.ErrInvalidMessage) {
		t.Fatalf("data before the auth: got %v, want %v", err, bps.ErrInvalidMessage)
	}
	e.publishAs(st, seatSess, bps.KindAuth, 0, nil)
	if err := publish(bps.KindRoster, nil); !errors.Is(err, bps.ErrNotPublisher) {
		t.Fatalf("roster from a seat: got %v, want %v", err, bps.ErrNotPublisher)
	}
	if err := publish(bps.KindEOS, nil); !errors.Is(err, bps.ErrNotPublisher) {
		t.Fatalf("eos from a seat: got %v, want %v", err, bps.ErrNotPublisher)
	}
	// none of it reached the broker
	if e.bl.blocklisted(overlay) || e.counters().WrongStream != 0 {
		t.Fatal("a refused frame reached the broker")
	}
}

// TestGrantOnReconnect: a seat that passes back its position does not get the
// broker's re-sent roster again, which is below its roster cursor, but it is the
// cue on this stream all the same. So a dApp cannot go by the ROSTER delivery
// alone: Granted is what tells it.
func TestGrantOnReconnect(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	st := e.newSeat()
	admin, _ := e.publisher()
	drain(admin)
	e.grant(admin, 0, st)

	c, _ := e.client(swarm.RandAddress(t))
	seatSess, err := c.Join(context.Background(), bps.JoinRequest{
		Broker:   e.brokerAddr,
		Spec:     bps.CohortSpec{Topic: e.topic, Principal: e.principal},
		Owner:    st.addr,
		Position: bps.Position{Cursors: map[string]uint64{}, Roster: [][]byte{st.addr}, RosterCursor: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = seatSess.Close() })
	waitGranted(t, seatSess)
	expectNoMessage(t, seatSess)
	e.publishAs(st, seatSess, bps.KindAuth, 0, nil)
}

// TestRosterNamingTheAdmin: a session refuses to send one (see TestPublishChecks),
// but a broker and a subscriber take a roster that names the admin, or an address
// twice, as it comes, and it changes nothing: the admin stays a publisher, by that
// roster and by a later one that does not name it, and the rest of it holds.
func TestRosterNamingTheAdmin(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		roster func(admin, seat []byte) [][]byte
	}{
		{"the admin alone", func(admin, _ []byte) [][]byte { return [][]byte{admin} }},
		{"the admin with another", func(admin, seat []byte) [][]byte { return [][]byte{admin, seat} }},
		{"an address twice", func(_, seat []byte) [][]byte { return [][]byte{seat, seat} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, bps.Options{})
			st := e.newSeat()
			sub := e.subscribe()
			w, _, ack := e.raw(&pb.Join{Cohort: e.spec(), Owner: e.principal})
			write := func(f *pb.Broadcast) {
				t.Helper()
				f.Challenge = ack.Challenge
				if err := w.WriteMsg(f); err != nil {
					t.Fatal(err)
				}
			}
			roster := func(index uint64, addrs [][]byte) {
				t.Helper()
				payload, err := bps.EncodeRosterMsg(addrs)
				if err != nil {
					t.Fatal(err)
				}
				write(&pb.Broadcast{Kind: pb.Kind_ROSTER, Index: index, Soc: e.chunk(e.signer, bps.KindRoster, ack.Challenge, index, payload)})
				expectKind(t, sub, bps.KindRoster, e.principal, index)
			}
			data := func(index uint64) {
				t.Helper()
				write(&pb.Broadcast{Kind: pb.Kind_DATA, Index: index, Soc: e.chunk(e.signer, bps.KindData, ack.Challenge, index, []byte("x"))})
				expectKind(t, sub, bps.KindData, e.principal, index)
			}
			write(&pb.Broadcast{Kind: pb.Kind_AUTH, Soc: e.chunk(e.signer, bps.KindAuth, ack.Challenge, 0, nil)})

			roster(0, tc.roster(e.principal, st.addr))
			data(0)
			roster(1, nil) // names no one: the admin is a publisher all the same
			data(1)
			if got := e.counters(); got.InvalidRoster != 0 || got.Revoked != 0 || got.WrongStream != 0 || got.InvalidSOC != 0 {
				t.Fatalf("counters %+v", got)
			}
		})
	}
}

// TestNoOwner: a stream may declare no owner. It listens like any spectator, within
// the fan-out bound, and can never upgrade, even if the key behind it is on the
// roster: no roster can name it.
func TestNoOwner(t *testing.T) {
	t.Parallel()

	t.Run("it listens, and its session refuses to publish", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, bps.Options{})
		st := e.newSeat()
		admin, _ := e.publisher()
		drain(admin)
		e.grant(admin, 0, st)

		c, _ := e.client(swarm.RandAddress(t))
		listener := e.join(c, nil)
		expectKind(t, listener, bps.KindRoster, e.principal, 0)
		e.publish(admin, bps.KindData, 0, []byte("heard"))
		expectKind(t, listener, bps.KindData, e.principal, 0)

		select {
		case <-listener.Granted():
			t.Fatal("a stream with no owner is not granted")
		default:
		}
		auth := e.chunk(st.signer, bps.KindAuth, listener.Challenge(), 0, nil)
		if err := listener.Publish(context.Background(), bps.KindAuth, 0, auth); !errors.Is(err, bps.ErrNotPublisher) {
			t.Fatalf("got %v, want %v", err, bps.ErrNotPublisher)
		}

		// a later roster changes nothing for it
		e.grant(admin, 1)
		expectKind(t, listener, bps.KindRoster, e.principal, 1)
		e.publish(admin, bps.KindData, 1, []byte("still heard"))
		expectKind(t, listener, bps.KindData, e.principal, 1)
	})

	t.Run("at the broker, an AUTH on it is not what the stream may send", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, bps.Options{})
		st := e.newSeat()
		admin, _ := e.publisher()
		drain(admin)
		e.grant(admin, 0, st)

		overlay := swarm.RandAddress(t)
		w, _, ack := e.rawFrom(overlay, &pb.Join{Cohort: e.spec()})
		if ack.Status != pb.Status_OK {
			t.Fatalf("status %s", ack.Status)
		}
		auth := e.chunk(st.signer, bps.KindAuth, ack.Challenge, 0, nil)
		if err := w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_AUTH, Challenge: ack.Challenge, Soc: auth}); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() bool { return e.bl.blocklisted(overlay) && e.counters().WrongStream == 1 })
	})

	t.Run("it counts against the fan-out bound", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, bps.Options{MaxSubsPerCohort: 1})
		e.subscribe()
		if _, _, ack := e.raw(&pb.Join{Cohort: e.spec()}); ack.Status != pb.Status_FULL {
			t.Fatalf("got %s, want FULL", ack.Status)
		}
	})
}

// TestMaxRoster: the most addresses a roster can name is what fits in a chunk.
func TestMaxRoster(t *testing.T) {
	t.Parallel()
	addrs := make([][]byte, bps.MaxRosterSize+1)
	for i := range addrs {
		addrs[i] = make([]byte, crypto.AddressSize)
	}
	fits, err := bps.EncodeRosterMsg(addrs[:bps.MaxRosterSize])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cac.New(fits); err != nil {
		t.Fatalf("%d addresses, %d bytes: %v", bps.MaxRosterSize, len(fits), err)
	}
	over, err := bps.EncodeRosterMsg(addrs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cac.New(over); err == nil {
		t.Fatalf("%d addresses, %d bytes fit in a chunk", len(addrs), len(over))
	}
}
