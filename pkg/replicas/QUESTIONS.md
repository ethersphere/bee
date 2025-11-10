# Replicas Package - Research Questions

Questions about the design and implementation of the `replicas` package.

## 1. Error Handling in Put Operations

**Question:** What is the expected behavior when some replica `Put` operations succeed and others fail?

**Why this question matters:**

- In `putter.go:36-60` and `putter_soc.go:37-65`, all errors are collected and joined
- The function returns `errors.Join(errs...)` which includes all errors
- If 15 out of 16 replicas succeed, is this considered a success or failure?
- The caller receives all errors but may not know which replicas were successfully stored

**Current Behavior:**

- All errors are collected and returned together
- No distinction between partial success and complete failure
- Caller must inspect the joined error to determine success rate

**Answer/Suggestion:**

- Is partial replication acceptable? **This must be validated against Book of Swarm probability calculations**
- Consider returning a structured result with success count and errors
- If partial replication is acceptable, document the impact on reliability guarantees

**Viktor**: Return apprpriate answer, and how many was expected/succeeded? We should decide if error is appropriate!

---

## 2. Context Cancellation in Put Operations

**Question:** Are `Put` operations properly respecting context cancellation?

**Why this question matters:**

- In `putter.go` and `putter_soc.go`, the context is passed to `Put` calls
- However, if the context is cancelled, all goroutines continue running until completion
- There's no early termination when context is cancelled
- This could lead to wasted resources if the caller cancels the operation

**Current Implementation:**

- Context is passed to `p.putter.Put(ctx, sch)` but cancellation is not checked
- `wg.Wait()` waits for all goroutines regardless of context state

**Answer/Suggestion:**

- Check `ctx.Done()` in the goroutine loop
- Cancel remaining operations when context is cancelled
- **Important**: If cancellation is allowed, document the impact on replica count and reliability guarantees

**Viktor**: User should be able to cancel.

## 3. Swarmageddon Error Strategy

**Question:** Is the "Swarmageddon" error approach the right way to handle complete replica retrieval failure?

**Why this question matters:**

- `ErrSwarmageddon` is returned when all replicas fail to retrieve
- The error message suggests this is an extremely rare event
- However, the error handling doesn't distinguish between temporary network issues and permanent data loss
- **Error Message Clarity**: The "Swarmageddon" term is not clear to users. The error message "swarmageddon has begun" doesn't explain what happened or what it means
- **Semantic Confusion**: The term "Swarmageddon" historically refers to complete data loss on the entire network, but the code uses it when all replicas of a single chunk fail to retrieve
- **Scope Question**: Is the extremely rare event (all replicas of one chunk failing) equivalent to assuming that data on the whole network is lost? Or is it just a local retrieval failure for that specific chunk?
- **User Experience**: Users receiving this error may not understand:
  - Whether this is a temporary issue or permanent data loss
  - Whether it affects just their chunk or the entire network
  - What actions they can take (retry? report? accept loss?)

The question should be validated with the research team to:

- Clarify the intended meaning of "Swarmageddon"
- Determine if the error message should be more descriptive
- Decide if the term should be changed to avoid confusion
- Establish whether retry logic should be implemented

**Viktor**: Return apprpriate answer, and how many was expected/succeeded? We should decide if error is appropriate!

---

## 4. Concurrent Put Operations with Disk I/O

**Question:** Does it make sense to use concurrent `Put` operations when the underlying storage layer performs disk I/O operations that are serialized?

**Why this question matters:**

- The `putter.go` and `putter_soc.go` implementations use `sync.WaitGroup` to concurrently call `Put` for all replicas
- However, the underlying storage layer has multiple serialization points:
  - **Upload Store Global Lock**: `pkg/storer/uploadstore.go:74` uses `db.Lock(uploadsLock)` which serializes all upload operations
  - **Sharky Serialization**: `pkg/sharky/shard.go` processes writes sequentially per shard through channels
  - **Transaction Locking**: `pkg/storer/internal/transaction/transaction.go:237` locks per chunk address

**Current Behavior:**

- Multiple goroutines are spawned to call `Put` concurrently
- All goroutines serialize at the upload store lock
- No actual parallelism is achieved
- Overhead of goroutine creation and context switching without benefit

**Answer/Suggestion:**

- If the global lock is intentional for consistency, consider making `Put` operations sequential to reduce overhead

**Viktor**: Use sequential approach.

---

## 5. Goroutine Explosion with Multiple Chunks

**Question:** Is there a risk of goroutine explosion when processing multiple chunks concurrently, and should there be a limit on concurrent replica operations?

**Why this question matters:**

- Both `Put` and `Get` operations spawn multiple goroutines per chunk
- Both CAC and SOC implementations have the same goroutine spawning pattern
- Multiple chunks can be processed concurrently from various sources
- `Get` and `Put` operations can happen simultaneously, compounding the goroutine count

**Concurrent Scenarios:**

**PUT Operations:**

- **Pusher Service**: `pkg/pusher/pusher.go:66` allows `ConcurrentPushes = swarm.Branches = 128` concurrent chunk pushes
- **API SOC Uploads**: `pkg/api/soc.go:112` - SOC chunk uploads via API (multiple concurrent clients)
- **API Chunk Stream**: `pkg/api/chunk_stream.go:200` - WebSocket chunk stream uploads (multiple concurrent clients)
- **File Uploads**: `pkg/file/pipeline/hashtrie/hashtrie.go:53` - File upload pipeline (root chunk replicas)

**GET Operations:**

- **File Joiner**: `pkg/file/joiner/joiner.go:135` - Uses `replicas.NewGetter` for root chunk retrieval
- **API Feed Retrieval**: `pkg/api/feed.go:80` - Uses `replicas.NewSocGetter` for feed chunk retrieval
- **API SOC Retrieval**: `pkg/api/soc.go:279` - Uses `replicas.NewSocGetter` for SOC chunk retrieval
- **Puller Service**: Chunks being pulled from network (multiple concurrent pulls)
- **Multiple concurrent API clients** requesting chunks simultaneously

**Goroutine Calculation Examples:**

**PUT Operations (Worst Case - PARANOID level):**

- 128 concurrent chunks (from pusher) × (16 replicas + 1 replicator) = **2,176 goroutines**
- Additional concurrent uploads from API clients can add more

**GET Operations (Worst Case - PARANOID level):**

- 128 concurrent Get calls × (1 original + 16 replicas + 1 replicator) = **2,304 goroutines**
- Note: Get operations spawn goroutines in batches, but if early batches fail, all goroutines can accumulate

**Combined Worst Case:**

- 128 concurrent Put + 128 concurrent Get = **4,480+ goroutines** just from the replicas package
- Plus goroutines from:
  - Other system components
  - Network I/O operations
  - Storage layer operations

**Current Behavior:**

- No limit on concurrent `Put` operations across chunks
- No limit on concurrent `Get` operations across chunks
- No limit on total goroutines spawned by the replicas package
- Each chunk upload spawns all replicas concurrently (no batching)
- Each chunk retrieval spawns replicas in batches, but batches can accumulate
- No backpressure mechanism to prevent goroutine explosion
- The upload store's global lock (`uploadsLock`) serializes Put operations but doesn't prevent goroutine accumulation

**Answer/Suggestion:**

- Consider implementing a semaphore or worker pool to limit concurrent replica operations globally
- Add a global limit on concurrent `Put` operations across all chunks
- Add a global limit on concurrent `Get` operations across all chunks
- Consider sequential `Put` operations per chunk to reduce goroutine count (though this may impact performance)
- Consider limiting the number of concurrent Get batches that can be in-flight
- Monitor goroutine count in production to validate if this is a real issue
- Consider if the upload store's global lock already provides sufficient backpressure (it serializes but doesn't limit goroutine count)

**Viktor** - Invesigate limits before we introduce limitations.

---

## 6. Goroutine Usage in socReplicator

**Question:** Is the goroutine in `socReplicator` necessary, and could the replica address generation functionality be exported for external verification tools like beekeeper?

**Why this question matters:**

- The `socReplicator` (see `pkg/replicas/replicas_soc.go:25-36`) uses a goroutine to generate replica addresses
- The computation is trivial: simple bit manipulation operations that generate at most 16 addresses
- The goroutine overhead may exceed the computation time for such simple operations
- The address generation is deterministic and could be exported for external verification tools

**Benchmark Results:**

Benchmark tests comparing synchronous vs asynchronous implementations show:

- **4.8x faster**: 299 ns/op (sync) vs 1,427 ns/op (async)
- **33% less memory**: 896 B/op (sync) vs 1,328 B/op (async)
- **53% fewer allocations**: 17 allocs/op (sync) vs 36 allocs/op (async)

The goroutine overhead significantly outweighs the trivial bit manipulation work.

**Answer/Suggestion:**

- Make `socReplicator` address generation synchronous (remove goroutine)
- Export a function like `GenerateSocReplicaAddresses(addr swarm.Address, level redundancy.Level) []swarm.Address` for external use
- This would allow beekeeper and other verification tools to independently calculate and verify replica addresses

**Viktor** Improve to use it without worker.

---

## 7. Exponential Backoff Strategy in Get Operations

**Question:** Is the exponential doubling strategy (2, 4, 8, 16 replicas per batch) optimal for retrieval?

**Why this question matters:**

- In `getter.go:79` and `getter_soc.go:70`, the number of replicas attempted doubles each `RetryInterval`
- This means: try 2, wait 300ms, try 4 more, wait 300ms, try 8 more, etc.
- The strategy assumes that if early replicas fail, more should be tried
- However, this might delay successful retrieval if early batches fail due to temporary issues

**Viktor**: If Redundancy Level is specified on PUT, most efficient is to use that exact one on GET. Leave it as is.

---
