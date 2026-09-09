// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/stampcarrier"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestDemo_RecoverableStamps is a narrated walk-through of the recoverable-stamps
// design, meant to be run with -v as a live demo:
//
//	go test -v -run TestDemo_RecoverableStamps ./pkg/file/joiner/
//
// It is not a new assertion surface — the behavior it shows is already covered by
// the TestStampCarrier* / TestStampRecovery* tests. Here the same machinery is
// arranged as one story so the log output reads top-to-bottom:
//
//	1. the original stamp of a chunk really is packed into a carrier;
//	2. lose that chunk entirely, and its ORIGINAL stamp comes back on read;
//	3. the carriers are themselves erasure coded — lose two of the group, still fine;
//	4. lose too many, and it degrades cleanly to "recovered, unstamped".
func TestDemo_RecoverableStamps(t *testing.T) {
	// Not parallel: the acts are meant to print in order for the demo.

	m, k, c := redundancy.MEDIUM.Composition(false) // 114 data, 9 parity, 3 carriers
	group := c + stampcarrier.GroupParities         // carriers + 2 carrier-parities

	banner(t,
		"THE PROBLEM",
		"Erasure coding rebuilds a lost chunk's 4096 bytes — but not its 113-byte",
		"postage stamp. The stamp is owner-signed and stored next to the chunk, so a",
		"reconstructed chunk normally comes back UNSTAMPED: it cannot re-enter the",
		"reserve or be pushsynced. This design makes the ORIGINAL stamp recoverable.",
	)
	t.Logf("at redundancy MEDIUM one full parent packs its 128 references as:")
	t.Logf("   %d data  +  %d parity  +  %d carriers  +  %d carrier-parities  = 128",
		m, k, c, stampcarrier.GroupParities)
	t.Logf("the carriers hold every child's stamp and form their own RS(%d,%d) group.", c, stampcarrier.GroupParities)

	// ── ACT 1 ───────────────────────────────────────────────────────────────
	t.Run("1_the_stamp_lives_in_a_carrier", func(t *testing.T) {
		banner(t,
			"ACT 1 — the original stamp really is carried",
			"Upload one full parent, then look inside its carriers.",
		)
		st, _ := newStampingStore(t)
		// UPLOAD: build one full parent; every chunk is stamped as it is stored.
		root, _ := uploadFile(t, st, m*swarm.ChunkSize, false)
		payload := parentPayload(t, st, root)
		t.Logf("uploaded a %d-chunk file; root parent %s", m, short(root))

		const slot = 5 // an ordinary data chunk
		dataAddr := refAt(payload, m, swarm.HashSize, slot)
		original := st.originalStamp(dataAddr)
		t.Logf("data chunk #%d is %s", slot, short(dataAddr))
		t.Logf("   its stamp, as written at upload : %s…  (%d bytes)", hex.EncodeToString(original[:16]), len(original))

		// the stamp for slot i lives in carrier i/48 — here, carrier #0
		carrierAddr := refAt(payload, m, swarm.HashSize, m+k) // first carrier ref
		carrierCh, err := st.Get(context.Background(), carrierAddr)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := stampcarrier.Unpack(carrierCh.Data()[swarm.SpanSize:])
		if err != nil {
			t.Fatal(err)
		}
		carried := entries[uint16(slot)]
		t.Logf("carrier #0 is %s and holds %d stamps (slots 0..47)", short(carrierAddr), len(entries))
		t.Logf("   the entry it carries for slot #%d   : %s…", slot, hex.EncodeToString(carried[:16]))

		if !bytes.Equal(carried, original) {
			t.Fatal("carried stamp differs from the original")
		}
		t.Log("=> byte-identical. The carrier holds the chunk's real, owner-signed stamp. ✓")
	})

	// ── ACT 2 ───────────────────────────────────────────────────────────────
	t.Run("2_lose_a_chunk_recover_its_original_stamp", func(t *testing.T) {
		banner(t,
			"ACT 2 — lose a chunk, get its ORIGINAL stamp back",
			"Delete a data chunk entirely, as if its whole neighborhood vanished,",
			"then read the file back.",
		)
		st, owner := newStampingStore(t)
		// UPLOAD the file — one full parent.
		root, data := uploadFile(t, st, m*swarm.ChunkSize, false)
		payload := parentPayload(t, st, root)

		const slot = 5
		victim := refAt(payload, m, swarm.HashSize, slot)
		original := st.originalStamp(victim) // remember the stamp so we can compare later
		// LOSE: delete the data chunk from storage — it now exists nowhere.
		if err := st.Delete(context.Background(), victim); err != nil {
			t.Fatal(err)
		}
		t.Logf("deleted data chunk #%d (%s) — it now exists nowhere.", slot, short(victim))

		// capturePutter records whatever the decoder rebuilds and saves back.
		caps := newCapturePutter(st.ChunkStore)
		ctx := ownerCtx(t, owner, getter.DATA) // plain read; falls back to RS recovery on the miss
		// RECOVER: reading the file triggers RS reconstruction of the deleted chunk.
		got := readAll(t, ctx, st, caps, root)
		if !bytes.Equal(got, data) {
			t.Fatal("read-back differs from the uploaded data")
		}
		t.Logf("read the whole file back: %d bytes, byte-identical to the upload. ✓", len(got))

		// REBUILT & SAVED: the decoder rebuilt the chunk and saved it with its recovered stamp.
		ch := waitSaved(t, caps, victim)
		recovered := marshalStamp(t, ch.Stamp())
		t.Logf("the erasure decoder rebuilt the chunk AND recovered its stamp from a carrier:")
		t.Logf("   original stamp  : %s…", hex.EncodeToString(original[:16]))
		t.Logf("   recovered stamp : %s…", hex.EncodeToString(recovered[:16]))

		// full check: byte-identical original + signature validates against the batch owner
		assertRecoveredStamp(t, st, ch, owner)
		t.Log("=> byte-identical to the original, and its signature validates against the")
		t.Log("   batch owner (ValidBinding). The chunk is whole again — data AND stamp. ✓")
	})

	// ── ACT 3 ───────────────────────────────────────────────────────────────
	t.Run("3_the_carriers_are_themselves_erasure_coded", func(t *testing.T) {
		banner(t,
			"ACT 3 — no single point of failure",
			"The carrier set is its own RS group. Lose a chunk AND two of the five",
			"carrier-group members at once — the group still rebuilds every stamp.",
		)
		st, owner := newStampingStore(t)
		// UPLOAD the file — one full parent.
		root, data := uploadFile(t, st, m*swarm.ChunkSize, false)
		payload := parentPayload(t, st, root)

		// LOSE (carriers): drop one carrier and one carrier-parity — 2 of the 5 group members.
		for _, slot := range []int{m + k, m + k + 3} {
			if err := st.Delete(context.Background(), refAt(payload, m, swarm.HashSize, slot)); err != nil {
				t.Fatal(err)
			}
		}
		// LOSE (data): also delete a data chunk.
		const dataSlot = 3
		victim := refAt(payload, m, swarm.HashSize, dataSlot)
		if err := st.Delete(context.Background(), victim); err != nil {
			t.Fatal(err)
		}
		t.Logf("deleted data chunk #%d AND 2 of the %d carrier-group members.", dataSlot, group)

		// RECOVER: the carrier RS group rebuilds the 2 lost carriers, so the stamp still comes back.
		caps := newCapturePutter(st.ChunkStore)
		ctx := ownerCtx(t, owner, getter.DATA)
		if got := readAll(t, ctx, st, caps, root); !bytes.Equal(got, data) {
			t.Fatal("read-back differs from the uploaded data")
		}
		assertRecoveredStamp(t, st, waitSaved(t, caps, victim), owner)
		t.Logf("=> the carrier RS(%d,%d) group rebuilt the lost carriers from the survivors,", c, stampcarrier.GroupParities)
		t.Log("   and the original stamp came back anyway. ✓")
	})

	// ── ACT 4 ───────────────────────────────────────────────────────────────
	t.Run("4_graceful_degradation", func(t *testing.T) {
		banner(t,
			"ACT 4 — never worse than today",
			"Now lose THREE of the five carrier-group members — past what RS(3,2) can",
			"recover. The data still comes back; only the stamp is gone.",
		)
		st, owner := newStampingStore(t)
		// UPLOAD the file — one full parent.
		root, data := uploadFile(t, st, m*swarm.ChunkSize, false)
		payload := parentPayload(t, st, root)

		// LOSE (carriers): drop THREE of the five group members — past what RS(3,2) can rebuild.
		for _, slot := range []int{m + k, m + k + 1, m + k + 2} {
			if err := st.Delete(context.Background(), refAt(payload, m, swarm.HashSize, slot)); err != nil {
				t.Fatal(err)
			}
		}
		// LOSE (data): also delete a data chunk.
		const dataSlot = 7
		victim := refAt(payload, m, swarm.HashSize, dataSlot)
		if err := st.Delete(context.Background(), victim); err != nil {
			t.Fatal(err)
		}
		t.Logf("deleted data chunk #%d AND 3 of the %d carrier-group members.", dataSlot, group)

		// RECOVER (data only): RS still rebuilds the data chunk from the main data+parity set...
		caps := newCapturePutter(st.ChunkStore)
		ctx := ownerCtx(t, owner, getter.DATA)
		if got := readAll(t, ctx, st, caps, root); !bytes.Equal(got, data) {
			t.Fatal("read-back differs from the uploaded data")
		}
		t.Logf("read the whole file back: %d bytes, still byte-identical. ✓", len(data))

		// UNSTAMPED: ...but the carrier group is unrecoverable, so no stamp comes back.
		ch := waitSaved(t, caps, victim)
		if ch.Stamp() != nil {
			t.Fatal("expected the rebuilt chunk to come back unstamped")
		}
		t.Log("=> the carrier group is unrecoverable, so the rebuilt chunk comes back")
		t.Log("   UNSTAMPED — exactly today's behavior. The feature never makes things worse. ✓")
	})

	banner(t,
		"IN SHORT",
		"Original stamps are recoverable for every data, parity and intermediate",
		"chunk; the carrier set is itself erasure coded; and every failure mode",
		"degrades to today's behavior — for ~4% storage and no mining.",
	)
}

// marshalStamp renders a recovered chunk's stamp back to its 113-byte wire form.
func marshalStamp(t *testing.T, s swarm.Stamp) []byte {
	t.Helper()
	b, err := postage.NewStamp(s.BatchID(), s.Index(), s.Timestamp(), s.Sig()).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// banner prints a titled, boxed narration block so the -v output reads as a story.
func banner(t *testing.T, title string, lines ...string) {
	t.Helper()
	const rule = "──────────────────────────────────────────────────────────────────────────"
	t.Log("")
	t.Log("┌" + rule)
	t.Log("│ " + title)
	if len(lines) > 0 {
		t.Log("├" + rule)
		for _, l := range lines {
			t.Log("│ " + l)
		}
	}
	t.Log("└" + rule)
}

func short(a swarm.Address) string { return hex.EncodeToString(a.Bytes()[:6]) + "…" }
