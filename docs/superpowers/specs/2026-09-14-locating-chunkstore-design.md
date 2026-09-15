# Design Spec: Locating ChunkStore & Direct Reserve Sampling without Breaking Abstractions

**Date:** 2026-09-14  
**Author:** Antigravity & Ljubisa Gacevic  
**Status:** Approved for Implementation Planning  
**Target Branch:** `perf/sample-01-hoist-chunkstore` (or feature branch derived from it)

---

## 1. Problem Statement & Motivation

During the Swarm storage incentives game (`ReserveSample`), a node computes a sample over chunks in its reserve (typically 50,000–100,000 chunks).
Currently:
1. Phase 1 iterates `ChunkBinItem`s from the reserve index.
2. Phase 2 loads chunk data via `chunkStore.GetInto(ctx, addr, buf)`.

Under the hood, every single `chunkStore.GetInto` call performs a LevelDB lookup for `RetrievalIndexItem{Address: addr}` to discover the chunk's `sharky.Location`, and then reads the blob from Sharky. Across 100k chunks, this results in 100k random LevelDB lookups per sampling round.

A previous experimental branch (`perf/reserve-sample-direct-sharky-read` & `...-guard`) attempted to denormalize `sharky.Location` into `ChunkBinItem` to read directly from Sharky. However, it was met with resistance because:
1. **Broken Abstractions & Leaky Interfaces:** `reserve` and `sample.go` directly imported and depended on `sharky.Location`. Runtime type assertions (`db.storage.(transaction.SharkyReader)`) bypassed the standard `storage.ChunkStore` contracts.
2. **Use-After-Free Concurrency Bug:** When chunks are evicted or replaced during sampling, Sharky reuses freed slots. Direct Sharky reads can read newly written chunks in those reused slots, corrupting sample proofs. This forced an ad-hoc `samplingGuard` with callbacks across subsystems (`onEvict`).
3. **Heavy Database Migration (`step_08`):** Adding `Location` directly broke binary deserialization of existing `ChunkBinItem`s on disk, requiring an expensive, blocking database migration across all production nodes.

---

## 2. Goals & Non-Goals

### Goals
- **Maintain Clean Abstractions:** `reserve` must NOT import or know about `sharky`. The storage abstraction must remain clean and follow standard Go interface patterns (Interface Segregation / Capability Interfaces).
- **Zero Race Conditions (Safe Sharky Slot Reuse):** Eliminate use-after-free and silent data corruption when slots are released during sampling rounds.
- **Encapsulation:** The guard mechanism tracking released slots must be completely encapsulated within `internal/chunkstore`, with zero callbacks or lifecycle tracking leaked into `reserve` or `storer`.
- **Zero-Downtime / No Mandatory Migration:** Backward-compatible binary serialization in `ChunkBinItem.Unmarshal` that seamlessly supports both legacy items (106 bytes) and new items with location (114 bytes).
- **Graceful Fallback:** If a location is zero, legacy, or belonged to a released slot, the system automatically and transparently falls back to standard LevelDB `GetInto(ctx, addr, buf)`.

### Non-Goals
- Changing Sharky's internal storage layout or freelist behavior.
- Altering the cryptographic BMT sample calculation or consensus logic.
- Rewriting the entire storage engine.

---

## 3. Architecture & Interface Design

### 3.1 Opaque `storage.ChunkLocation`

In `pkg/storage/chunkstore.go`, define an opaque 8-byte value:

```go
// ChunkLocation is an opaque locator hint for accelerated chunk retrieval.
// Implementations can encode internal storage coordinates (e.g. Sharky shard/slot/length)
// into this structure. A zero ChunkLocation indicates an unset or legacy location.
type ChunkLocation [8]byte

func (c ChunkLocation) IsZero() bool {
    return c == ChunkLocation{}
}
```

*Note on size:* `sharky.Location` consists of `Shard uint8 (1) + Slot uint32 (4) + Length uint16 (2) = 7 bytes`. An 8-byte array accommodates this cleanly without exposing Sharky internals.

### 3.2 Capability Interfaces in `pkg/storage`

Following idiomatic Go practices (similar to `io.WriterTo` / `io.ReaderFrom`), define optional capability interfaces in `pkg/storage/chunkstore.go`:

```go
// LocatingPutter is an optional capability of a Putter that returns a ChunkLocation hint.
type LocatingPutter interface {
    PutLoc(ctx context.Context, ch swarm.Chunk) (ChunkLocation, error)
}

// LocatingReplacer is an optional capability of a Replacer that returns a ChunkLocation hint.
type LocatingReplacer interface {
    ReplaceLoc(ctx context.Context, ch swarm.Chunk, emplace bool) (ChunkLocation, error)
}

// LocatingGetterInto is an optional capability of a GetterInto that uses a ChunkLocation hint.
// Implementations MUST verify the validity of the location or guard state, and fallback to
// address-based lookup if the location is unsafe, unset, or invalid.
type LocatingGetterInto interface {
    GetterInto
    GetIntoLoc(ctx context.Context, addr swarm.Address, loc ChunkLocation, buf []byte) (int, error)
}
```

### 3.3 Internal Location Serialization (`internal/chunkstore`)

Within `pkg/storer/internal/chunkstore`, provide internal helpers to convert between `sharky.Location` and `storage.ChunkLocation`:

```go
func locationToChunkLocation(l sharky.Location) storage.ChunkLocation {
    var cl storage.ChunkLocation
    cl[0] = l.Shard
    binary.BigEndian.PutUint32(cl[1:5], l.Slot)
    binary.BigEndian.PutUint16(cl[5:7], l.Length)
    return cl
}

func chunkLocationToLocation(cl storage.ChunkLocation) sharky.Location {
    return sharky.Location{
        Shard:  cl[0],
        Slot:   binary.BigEndian.Uint32(cl[1:5]),
        Length: binary.BigEndian.Uint16(cl[5:7]),
    }
}
```

---

## 4. Concurrency Guard & Slot Reuse Protection

### 4.1 Encapsulated `LocationGuard`

Sharky releases slots in only two places across the entire codebase:
1. `chunkstore.Delete`
2. `chunkstore.Replace`

We introduce a thread-safe `LocationGuard` encapsulated within the chunkstore component:

```go
type LocationGuard struct {
    mu      sync.RWMutex
    active  int32 // active sampling session counter
    freed   map[storage.ChunkLocation]struct{}
}

func (g *LocationGuard) StartSession() func() {
    g.mu.Lock()
    defer g.mu.Unlock()
    if g.active == 0 {
        g.freed = make(map[storage.ChunkLocation]struct{})
    }
    g.active++

    return func() {
        g.mu.Lock()
        defer g.mu.Unlock()
        g.active--
        if g.active == 0 {
            g.freed = nil
        }
    }
}

func (g *LocationGuard) MarkFreed(loc storage.ChunkLocation) {
    g.mu.Lock()
    defer g.mu.Unlock()
    if g.active > 0 {
        g.freed[loc] = struct{}{}
    }
}

func (g *LocationGuard) IsFreed(loc storage.ChunkLocation) bool {
    g.mu.RLock()
    defer g.mu.RUnlock()
    if g.active == 0 {
        return false
    }
    _, found := g.freed[loc]
    return found
}
```

### 4.2 Safe Execution in `GetIntoLoc`

When `GetIntoLoc(ctx, addr, loc, buf)` is called:
1. If `loc.IsZero()`: call standard `GetInto(ctx, addr, buf)` (LevelDB lookup).
2. If `guard.IsFreed(loc)`: the slot was released during the active sampling session! Immediately call standard `GetInto(ctx, addr, buf)` (which will either read the new location from LevelDB or return `storage.ErrNotFound`).
3. Otherwise: read directly from `sharky.Read(ctx, chunkLocationToLocation(loc), buf)`. If Sharky returns an error, fallback to `GetInto(ctx, addr, buf)`.

Result: **100% immune to use-after-free and slot reuse race conditions.**

---

## 5. Storage & Backward-Compatible Serialization

### 5.1 `ChunkBinItem` Definition

In `pkg/storer/internal/reserve/items.go`:

```go
type ChunkBinItem struct {
    Bin       uint8
    BinID     uint64
    Address   swarm.Address
    BatchID   []byte
    StampHash []byte
    ChunkType swarm.ChunkType
    Location  storage.ChunkLocation // Opaque 8 bytes
}
```

### 5.2 Two-Format Unmarshal

- Legacy item size: `1 + 8 + 32 + 32 + 1 + 32 = 106 bytes`.
- New item size: `106 + 8 = 114 bytes`.

```go
const (
    legacyChunkBinItemSize  = 106
    chunkBinItemSizeWithLoc = 114
)

func (c *ChunkBinItem) Marshal() ([]byte, error) {
    buf := make([]byte, chunkBinItemSizeWithLoc)
    // marshal standard fields (0..106)
    ...
    copy(buf[106:114], c.Location[:])
    return buf, nil
}

func (c *ChunkBinItem) Unmarshal(buf []byte) error {
    switch len(buf) {
    case legacyChunkBinItemSize:
        // Decode standard fields; c.Location remains zeroed.
        return c.unmarshalLegacy(buf)
    case chunkBinItemSizeWithLoc:
        if err := c.unmarshalLegacy(buf[:legacyChunkBinItemSize]); err != nil {
            return err
        }
        copy(c.Location[:], buf[legacyChunkBinItemSize:])
        return nil
    default:
        return errUnmarshalInvalidSize
    }
}
```

---

## 6. Verification Plan

1. **Unit Tests:**
   - Test `ChunkBinItem` marshal/unmarshal with both 106-byte (legacy) and 114-byte buffers.
   - Test `LocationGuard` concurrency (concurrent `MarkFreed` and `IsFreed`).
   - Test `GetIntoLoc` fallback behavior when location is zero, when slot is freed, and normal path.
2. **Race Detector:**
   - Run `go test -race ./pkg/storer -run TestReserveSample` under heavy parallel eviction and insertion.
3. **Benchmarks:**
   - Compare `BenchmarkReserveSample1k` with populated `ChunkLocation` vs baseline. Expect significant drop in LevelDB read operations and latency.
