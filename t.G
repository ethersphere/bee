=== RUN   TestCacheStore
=== PAUSE TestCacheStore
=== RUN   TestDebugInfo
=== PAUSE TestDebugInfo
=== RUN   TestEpochMigration
=== PAUSE TestEpochMigration
=== RUN   TestNetStore
=== PAUSE TestNetStore
=== RUN   TestPinStore
=== PAUSE TestPinStore
=== RUN   TestReporter
=== PAUSE TestReporter
=== RUN   TestIndexCollision
=== PAUSE TestIndexCollision
=== RUN   TestReplaceOldIndex
=== PAUSE TestReplaceOldIndex
=== RUN   TestEvictBatch
=== PAUSE TestEvictBatch
=== RUN   TestUnreserveCap
=== PAUSE TestUnreserveCap
=== RUN   TestNetworkRadius
=== PAUSE TestNetworkRadius
=== RUN   TestRadiusManager
=== PAUSE TestRadiusManager
=== RUN   TestSubscribeBin
=== PAUSE TestSubscribeBin
=== RUN   TestSubscribeBinTrigger
=== PAUSE TestSubscribeBinTrigger
=== RUN   TestReserveSampler
=== RUN   TestReserveSampler/disk
=== PAUSE TestReserveSampler/disk
=== RUN   TestReserveSampler/mem
=== PAUSE TestReserveSampler/mem
=== CONT  TestReserveSampler/disk
=== CONT  TestReserveSampler/mem
=== RUN   TestReserveSampler/disk/reserve_size
=== RUN   TestReserveSampler/disk/reserve_sample_1
=== RUN   TestReserveSampler/mem/reserve_size
=== RUN   TestReserveSampler/mem/reserve_sample_1
=== RUN   TestReserveSampler/disk/reserve_size#01
=== RUN   TestReserveSampler/disk/reserve_sample_2
=== RUN   TestReserveSampler/mem/reserve_size#01
=== RUN   TestReserveSampler/mem/reserve_sample_2
--- PASS: TestReserveSampler (0.00s)
    --- PASS: TestReserveSampler/disk (26.79s)
        --- PASS: TestReserveSampler/disk/reserve_size (0.00s)
        --- PASS: TestReserveSampler/disk/reserve_sample_1 (0.00s)
        --- PASS: TestReserveSampler/disk/reserve_size#01 (0.00s)
        --- PASS: TestReserveSampler/disk/reserve_sample_2 (0.38s)
    --- PASS: TestReserveSampler/mem (27.71s)
        --- PASS: TestReserveSampler/mem/reserve_size (0.00s)
        --- PASS: TestReserveSampler/mem/reserve_sample_1 (0.00s)
        --- PASS: TestReserveSampler/mem/reserve_size#01 (0.00s)
        --- PASS: TestReserveSampler/mem/reserve_sample_2 (0.41s)
=== RUN   TestRandSample
=== PAUSE TestRandSample
=== RUN   TestNew
=== PAUSE TestNew
=== RUN   TestPushSubscriber
=== PAUSE TestPushSubscriber
=== RUN   TestListDeleteSessions
=== PAUSE TestListDeleteSessions
=== RUN   TestUploadStore
=== PAUSE TestUploadStore
=== CONT  TestEpochMigration
=== CONT  TestUploadStore
=== CONT  TestListDeleteSessions
=== CONT  TestUnreserveCap
=== RUN   TestListDeleteSessions/inmem
=== RUN   TestUnreserveCap/disk
=== PAUSE TestListDeleteSessions/inmem
=== CONT  TestSubscribeBinTrigger
=== PAUSE TestUnreserveCap/disk
=== RUN   TestSubscribeBinTrigger/disk
=== RUN   TestUnreserveCap/mem
=== PAUSE TestSubscribeBinTrigger/disk
=== RUN   TestSubscribeBinTrigger/mem
=== PAUSE TestSubscribeBinTrigger/mem
=== PAUSE TestUnreserveCap/mem
=== CONT  TestSubscribeBin
=== RUN   TestSubscribeBin/disk
=== PAUSE TestSubscribeBin/disk
=== CONT  TestRadiusManager
=== RUN   TestRadiusManager/radius_decrease_due_to_under_utilization
=== RUN   TestSubscribeBin/mem
=== CONT  TestNew
=== RUN   TestNew/inmem_with_options
=== PAUSE TestNew/inmem_with_options
=== RUN   TestNew/disk_default_options
=== CONT  TestReporter
=== RUN   TestReporter/inmem
=== RUN   TestUploadStore/inmem
=== PAUSE TestUploadStore/inmem
=== PAUSE TestRadiusManager/radius_decrease_due_to_under_utilization
=== PAUSE TestReporter/inmem
=== PAUSE TestSubscribeBin/mem
=== RUN   TestUploadStore/disk
=== PAUSE TestNew/disk_default_options
=== RUN   TestReporter/disk
=== CONT  TestRandSample
=== RUN   TestRadiusManager/radius_doesnt_change_due_to_non-zero_pull_rate
=== PAUSE TestReporter/disk
=== RUN   TestListDeleteSessions/disk
=== CONT  TestEvictBatch
=== PAUSE TestUploadStore/disk
=== PAUSE TestRadiusManager/radius_doesnt_change_due_to_non-zero_pull_rate
=== PAUSE TestListDeleteSessions/disk
=== CONT  TestNetworkRadius
=== RUN   TestNetworkRadius/disk
=== PAUSE TestNetworkRadius/disk
=== RUN   TestNetworkRadius/mem
=== PAUSE TestNetworkRadius/mem
=== CONT  TestPushSubscriber
=== RUN   TestPushSubscriber/inmem
=== PAUSE TestPushSubscriber/inmem
=== RUN   TestPushSubscriber/disk
=== PAUSE TestPushSubscriber/disk
=== CONT  TestDebugInfo
=== RUN   TestDebugInfo/inmem
=== PAUSE TestDebugInfo/inmem
=== RUN   TestDebugInfo/disk
=== PAUSE TestDebugInfo/disk
=== CONT  TestNetStore
=== RUN   TestNetStore/inmem
=== PAUSE TestNetStore/inmem
=== RUN   TestNetStore/disk
=== PAUSE TestNetStore/disk
=== CONT  TestPinStore
=== RUN   TestPinStore/inmem
=== PAUSE TestPinStore/inmem
=== RUN   TestPinStore/disk
=== PAUSE TestPinStore/disk
=== CONT  TestCacheStore
=== RUN   TestCacheStore/inmem
=== PAUSE TestCacheStore/inmem
=== RUN   TestCacheStore/disk
=== PAUSE TestCacheStore/disk
=== CONT  TestSubscribeBinTrigger/mem
=== RUN   TestNew/disk_with_options
=== CONT  TestIndexCollision
=== CONT  TestReplaceOldIndex
=== RUN   TestIndexCollision/disk
=== CONT  TestUnreserveCap/disk
=== PAUSE TestIndexCollision/disk
=== RUN   TestIndexCollision/mem
=== PAUSE TestIndexCollision/mem
=== RUN   TestReplaceOldIndex/disk
=== PAUSE TestReplaceOldIndex/disk
=== RUN   TestReplaceOldIndex/mem
=== PAUSE TestNew/disk_with_options
=== RUN   TestNew/migration_on_latest_version
=== PAUSE TestNew/migration_on_latest_version
=== CONT  TestSubscribeBinTrigger/disk
=== CONT  TestUnreserveCap/mem
=== PAUSE TestReplaceOldIndex/mem
=== CONT  TestSubscribeBin/mem
--- PASS: TestRandSample (0.28s)
=== CONT  TestSubscribeBin/disk
=== RUN   TestEvictBatch/reserve_size
--- PASS: TestEpochMigration (0.46s)
=== CONT  TestReporter/inmem
--- PASS: TestEvictBatch (0.46s)
    --- PASS: TestEvictBatch/reserve_size (0.00s)
=== CONT  TestListDeleteSessions/inmem
=== RUN   TestListDeleteSessions/inmem/list_sessions
=== PAUSE TestListDeleteSessions/inmem/list_sessions
=== RUN   TestListDeleteSessions/inmem/delete_sessions
=== PAUSE TestListDeleteSessions/inmem/delete_sessions
=== CONT  TestNetworkRadius/disk
=== CONT  TestNetworkRadius/mem
=== RUN   TestSubscribeBin/mem/subscribe_full_range
=== PAUSE TestSubscribeBin/mem/subscribe_full_range
=== RUN   TestSubscribeBin/mem/subscribe_unsub
=== PAUSE TestSubscribeBin/mem/subscribe_unsub
=== RUN   TestSubscribeBin/mem/subscribe_range_higher_bin
=== PAUSE TestSubscribeBin/mem/subscribe_range_higher_bin
=== RUN   TestSubscribeBin/mem/subscribe_beyond_range
=== PAUSE TestSubscribeBin/mem/subscribe_beyond_range
=== CONT  TestPushSubscriber/disk
=== RUN   TestReporter/inmem/report
=== RUN   TestReporter/inmem/report/commit
=== RUN   TestReporter/inmem/report/rollback
=== CONT  TestPushSubscriber/inmem
=== CONT  TestDebugInfo/inmem
=== RUN   TestDebugInfo/inmem/upload_and_pin
=== PAUSE TestDebugInfo/inmem/upload_and_pin
=== RUN   TestDebugInfo/inmem/cache
=== PAUSE TestDebugInfo/inmem/cache
=== RUN   TestDebugInfo/inmem/reserve
=== PAUSE TestDebugInfo/inmem/reserve
=== CONT  TestDebugInfo/disk
=== RUN   TestDebugInfo/disk/upload_and_pin
=== PAUSE TestDebugInfo/disk/upload_and_pin
=== RUN   TestDebugInfo/disk/cache
=== PAUSE TestDebugInfo/disk/cache
=== RUN   TestDebugInfo/disk/reserve
=== PAUSE TestDebugInfo/disk/reserve
=== CONT  TestNetStore/inmem
=== RUN   TestNetStore/inmem/direct_upload
=== PAUSE TestNetStore/inmem/direct_upload
=== RUN   TestNetStore/inmem/download
=== PAUSE TestNetStore/inmem/download
=== CONT  TestNetStore/disk
=== RUN   TestNetStore/disk/direct_upload
=== PAUSE TestNetStore/disk/direct_upload
=== RUN   TestNetStore/disk/download
=== PAUSE TestNetStore/disk/download
=== CONT  TestPinStore/inmem
=== CONT  TestPinStore/disk
--- PASS: TestUnreserveCap (0.00s)
    --- PASS: TestUnreserveCap/mem (0.46s)
    --- PASS: TestUnreserveCap/disk (0.60s)
--- PASS: TestSubscribeBinTrigger (0.00s)
    --- PASS: TestSubscribeBinTrigger/mem (0.60s)
    --- PASS: TestSubscribeBinTrigger/disk (0.77s)
=== CONT  TestReporter/disk
=== RUN   TestSubscribeBin/disk/subscribe_full_range
=== PAUSE TestSubscribeBin/disk/subscribe_full_range
=== RUN   TestSubscribeBin/disk/subscribe_unsub
=== PAUSE TestSubscribeBin/disk/subscribe_unsub
=== RUN   TestSubscribeBin/disk/subscribe_range_higher_bin
=== PAUSE TestSubscribeBin/disk/subscribe_range_higher_bin
=== RUN   TestSubscribeBin/disk/subscribe_beyond_range
=== PAUSE TestSubscribeBin/disk/subscribe_beyond_range
=== CONT  TestCacheStore/disk
=== RUN   TestReporter/disk/report
=== RUN   TestReporter/disk/report/commit
=== RUN   TestReporter/disk/report/rollback
=== CONT  TestCacheStore/inmem
--- PASS: TestReporter (0.00s)
    --- PASS: TestReporter/inmem (0.07s)
        --- PASS: TestReporter/inmem/report (0.00s)
            --- PASS: TestReporter/inmem/report/commit (0.00s)
            --- PASS: TestReporter/inmem/report/rollback (0.00s)
    --- PASS: TestReporter/disk (0.21s)
        --- PASS: TestReporter/disk/report (0.00s)
            --- PASS: TestReporter/disk/report/commit (0.00s)
            --- PASS: TestReporter/disk/report/rollback (0.00s)
=== RUN   TestCacheStore/inmem/cache_chunks
=== RUN   TestCacheStore/inmem/cache_chunks/commit
=== RUN   TestCacheStore/inmem/cache_chunks/rollback
=== RUN   TestCacheStore/inmem/lookup
=== RUN   TestCacheStore/inmem/lookup/commit
=== RUN   TestCacheStore/disk/cache_chunks
=== RUN   TestCacheStore/disk/cache_chunks/commit
=== RUN   TestCacheStore/inmem/lookup/rollback
=== RUN   TestCacheStore/inmem/cache_chunks_beyond_capacity
=== RUN   TestCacheStore/disk/cache_chunks/rollback
=== RUN   TestCacheStore/disk/lookup
=== RUN   TestCacheStore/disk/lookup/commit
=== RUN   TestCacheStore/disk/lookup/rollback
=== RUN   TestCacheStore/disk/cache_chunks_beyond_capacity
=== CONT  TestUploadStore/inmem
=== RUN   TestUploadStore/inmem/new_session
=== PAUSE TestUploadStore/inmem/new_session
=== RUN   TestUploadStore/inmem/no_tag
=== PAUSE TestUploadStore/inmem/no_tag
=== CONT  TestUploadStore/disk
=== RUN   TestUploadStore/disk/new_session
=== PAUSE TestUploadStore/disk/new_session
=== RUN   TestUploadStore/disk/no_tag
=== PAUSE TestUploadStore/disk/no_tag
=== CONT  TestRadiusManager/radius_decrease_due_to_under_utilization
--- PASS: TestPushSubscriber (0.00s)
    --- PASS: TestPushSubscriber/inmem (0.78s)
    --- PASS: TestPushSubscriber/disk (0.87s)
--- PASS: TestCacheStore (0.00s)
    --- PASS: TestCacheStore/inmem (0.30s)
        --- PASS: TestCacheStore/inmem/cache_chunks (0.02s)
            --- PASS: TestCacheStore/inmem/cache_chunks/commit (0.01s)
            --- PASS: TestCacheStore/inmem/cache_chunks/rollback (0.02s)
        --- PASS: TestCacheStore/inmem/lookup (0.01s)
            --- PASS: TestCacheStore/inmem/lookup/commit (0.00s)
            --- PASS: TestCacheStore/inmem/lookup/rollback (0.00s)
        --- PASS: TestCacheStore/inmem/cache_chunks_beyond_capacity (0.08s)
    --- PASS: TestCacheStore/disk (0.52s)
        --- PASS: TestCacheStore/disk/cache_chunks (0.02s)
            --- PASS: TestCacheStore/disk/cache_chunks/commit (0.01s)
            --- PASS: TestCacheStore/disk/cache_chunks/rollback (0.02s)
        --- PASS: TestCacheStore/disk/lookup (0.01s)
            --- PASS: TestCacheStore/disk/lookup/commit (0.01s)
            --- PASS: TestCacheStore/disk/lookup/rollback (0.00s)
        --- PASS: TestCacheStore/disk/cache_chunks_beyond_capacity (0.16s)
=== CONT  TestRadiusManager/radius_doesnt_change_due_to_non-zero_pull_rate
=== CONT  TestIndexCollision/mem
=== RUN   TestIndexCollision/mem/reserve_size
=== CONT  TestNew/inmem_with_options
=== CONT  TestListDeleteSessions/disk
=== RUN   TestListDeleteSessions/disk/list_sessions
=== PAUSE TestListDeleteSessions/disk/list_sessions
=== RUN   TestListDeleteSessions/disk/delete_sessions
=== PAUSE TestListDeleteSessions/disk/delete_sessions
=== CONT  TestIndexCollision/disk
=== CONT  TestNew/disk_default_options
=== CONT  TestNew/migration_on_latest_version
=== RUN   TestNew/migration_on_latest_version/inmem
--- PASS: TestNetworkRadius (0.00s)
    --- PASS: TestNetworkRadius/mem (1.10s)
    --- PASS: TestNetworkRadius/disk (1.23s)
=== PAUSE TestNew/migration_on_latest_version/inmem
=== RUN   TestNew/migration_on_latest_version/disk
=== PAUSE TestNew/migration_on_latest_version/disk
=== CONT  TestReplaceOldIndex/mem
=== RUN   TestReplaceOldIndex/mem/#00
=== RUN   TestReplaceOldIndex/mem/#00/reserve_size
=== CONT  TestReplaceOldIndex/disk
=== RUN   TestIndexCollision/disk/reserve_size
=== CONT  TestNew/disk_with_options
=== CONT  TestListDeleteSessions/inmem/delete_sessions
--- PASS: TestIndexCollision (0.00s)
    --- PASS: TestIndexCollision/mem (0.11s)
        --- PASS: TestIndexCollision/mem/reserve_size (0.00s)
    --- PASS: TestIndexCollision/disk (0.20s)
        --- PASS: TestIndexCollision/disk/reserve_size (0.00s)
=== CONT  TestListDeleteSessions/inmem/list_sessions
=== CONT  TestSubscribeBin/mem/subscribe_full_range
=== CONT  TestSubscribeBin/mem/subscribe_beyond_range
=== RUN   TestReplaceOldIndex/disk/#00
=== RUN   TestReplaceOldIndex/disk/#00/reserve_size
=== CONT  TestSubscribeBin/mem/subscribe_range_higher_bin
=== CONT  TestSubscribeBin/mem/subscribe_unsub
=== CONT  TestDebugInfo/inmem/reserve
=== CONT  TestDebugInfo/inmem/cache
--- PASS: TestReplaceOldIndex (0.00s)
    --- PASS: TestReplaceOldIndex/mem (0.02s)
        --- PASS: TestReplaceOldIndex/mem/#00 (0.01s)
            --- PASS: TestReplaceOldIndex/mem/#00/reserve_size (0.00s)
    --- PASS: TestReplaceOldIndex/disk (0.26s)
        --- PASS: TestReplaceOldIndex/disk/#00 (0.01s)
            --- PASS: TestReplaceOldIndex/disk/#00/reserve_size (0.00s)
=== RUN   TestPinStore/inmem/pin_10_chunks
=== RUN   TestPinStore/inmem/pin_20_chunks_rollback
=== RUN   TestPinStore/inmem/pin_30_chunks
=== RUN   TestPinStore/inmem/has_ce5219d802c740b40ea8cf502a219e687da3e6e2dd5c9186354084566b9a6d27
=== RUN   TestPinStore/inmem/has_3fa49d390d3d9daa79a150f13e93fc7b2eeaadb4d423dd8531b6d9bf034243ec
=== RUN   TestPinStore/inmem/has_29a47b9fff792db0331feb10c596f709779658545501ce4d8569b9156832f7a1
=== RUN   TestPinStore/inmem/pins
=== RUN   TestPinStore/inmem/delete_pin
=== RUN   TestPinStore/inmem/delete_pin/commit
=== RUN   TestPinStore/inmem/delete_pin/rollback
=== CONT  TestDebugInfo/inmem/upload_and_pin
=== RUN   TestPinStore/disk/pin_10_chunks
=== RUN   TestPinStore/disk/pin_20_chunks_rollback
=== RUN   TestPinStore/disk/pin_30_chunks
=== CONT  TestDebugInfo/disk/upload_and_pin
=== RUN   TestPinStore/disk/has_733adb5609dd15e419656e49f3e63813c91b89482f87043b54b961e24f1b1e20
=== RUN   TestPinStore/disk/has_06bd5cf3bcc1dcc69511c867f8a0f830038381dd07414146b617aca3e6c0e833
=== RUN   TestPinStore/disk/has_7f6da78dcc018ac45f3885a945e9b50cdee3ef20c37b485f04b66d31a921f0c9
=== RUN   TestPinStore/disk/pins
=== RUN   TestPinStore/disk/delete_pin
=== RUN   TestPinStore/disk/delete_pin/commit
=== RUN   TestPinStore/disk/delete_pin/rollback
=== CONT  TestDebugInfo/disk/reserve
panic: test timed out after 30s
running tests:
	TestDebugInfo (2s)
	TestDebugInfo/disk (2s)
	TestDebugInfo/disk/reserve (0s)
	TestDebugInfo/disk/upload_and_pin (0s)
	TestDebugInfo/inmem (2s)
	TestDebugInfo/inmem/upload_and_pin (0s)
	TestListDeleteSessions (2s)
	TestListDeleteSessions/disk (1s)
	TestNetStore (2s)
	TestNetStore/disk (2s)
	TestNetStore/inmem (2s)
	TestNew (2s)
	TestNew/migration_on_latest_version (1s)
	TestPinStore (2s)
	TestPinStore/disk (2s)
	TestRadiusManager (2s)
	TestRadiusManager/radius_decrease_due_to_under_utilization (1s)
	TestSubscribeBin (2s)
	TestSubscribeBin/disk (2s)
	TestSubscribeBin/mem (2s)
	TestSubscribeBin/mem/subscribe_beyond_range (0s)
	TestUploadStore (2s)
	TestUploadStore/disk (1s)
	TestUploadStore/inmem (1s)

goroutine 5421200 [running]:
testing.(*M).startAlarm.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:2241 +0x219
created by time.goFunc
	/home/anatol/sdk/go1.20.1/src/time/sleep.go:176 +0x48

goroutine 1 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc000502b60, 0xc0001fbac0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
testing.runTests(0xc0000c0960?, {0x171fb00, 0x14, 0x14}, {0x4?, 0x2?, 0x176d320?})
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:2034 +0x87d
testing.(*M).Run(0xc0000c0960)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1906 +0xb45
github.com/ethersphere/bee/pkg/storer_test.TestMain(0x0?)
	/home/anatol/swarm/bee/pkg/storer/storer_test.go:98 +0x47
main.main()
	_testmain.go:93 +0x325

goroutine 9 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc000503040, 0x11b4718)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 11 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc0005036c0, 0x11b4770)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 12 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc000503a00, 0x11b47d8)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5418910 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000e736c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 52 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc000103860, 0x11b47f8)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 53 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc000103ba0, 0x11b4860)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5368460 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc002f480f0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5346301 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc000b49520, 0x11b47c0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5411981 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc000978000)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.testListDeleteSessions.func1(0xc000978000)
	/home/anatol/swarm/bee/pkg/storer/uploadstore_test.go:256 +0x57
testing.tRunner(0xc000978000, 0xc001472450)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5412577 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000130c40)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5414833 [select]:
github.com/ethersphere/bee/pkg/storer.(*DB).SubscribeBin.func1()
	/home/anatol/swarm/bee/pkg/storer/reserve.go:699 +0x5a9
created by github.com/ethersphere/bee/pkg/storer.(*DB).SubscribeBin
	/home/anatol/swarm/bee/pkg/storer/reserve.go:670 +0x2cf

goroutine 5346382 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc001610340, 0x11b4750)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5346304 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc000b49a00, 0x11b4880)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5401234 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001904410)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5414820 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc001a76870)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5384318 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc00280e380)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5416627 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc002e20b90)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5387400 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc00283da40)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5418387 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002709c80, 0xc0004b5260)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5399673 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc001a765a0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5384319 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc00280e380)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5385463 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001f9ef50)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5377026 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc000598cc0, 0xc000e406c0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5419129 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc00222b980, 0xc000845020)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5418913 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5384321 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001f9ee60)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5403664 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc00241b540)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5403663 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc001e475c0, 0xc0010c73e0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5401719 [chan receive]:
testing.(*T).Parallel(0xc000830000)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1384 +0x569
github.com/ethersphere/bee/pkg/storer_test.testUploadStore.func1(0xc000830000)
	/home/anatol/swarm/bee/pkg/storer/uploadstore_test.go:25 +0x54
testing.tRunner(0xc000830000, 0xc001c887a0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5416643 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002f59020, 0xc001de1980)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5418063 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc000bd36c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5399681 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc00258c840, 0xc001365260)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5413831 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000dbbdc0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5399676 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc0004921c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5346025 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc002e6fc00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5404000 [chan receive]:
github.com/ethersphere/bee/pkg/storer.(*DB).startReserveWorkers.func1()
	/home/anatol/swarm/bee/pkg/storer/reserve.go:56 +0x59
created by github.com/ethersphere/bee/pkg/storer.(*DB).startReserveWorkers
	/home/anatol/swarm/bee/pkg/storer/reserve.go:55 +0x166

goroutine 5419131 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc00222b9e0, 0xc000845440)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5399674 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc0004921c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5414822 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc00089fa40)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5414821 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc00089fa40)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5412643 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc000e84000)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.TestNew.func4.1(0x0?)
	/home/anatol/swarm/bee/pkg/storer/storer_test.go:140 +0x31
testing.tRunner(0xc000e84000, 0x11b47a8)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5386181 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc002746a80)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5415641 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc002e6f500)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5414832 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc0023114a0, 0xc0012cf3e0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5401104 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc0001ea8c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5346408 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc001616340, 0x11b4760)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5399678 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001904370)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5375931 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000b4a1c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5414823 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc00089fa40)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5419124 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001b3c140)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5416431 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000792380)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5416560 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc001dbff80, 0xc0017cdb60)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5414225 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002310660, 0xc001821d40)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5414831 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0006b8f50)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5346303 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc000b49860, 0x11b4758)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5412572 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000ea6380)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5346332 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc001610b60, 0xc001c3e5b0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5416432 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc000792380)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5346031 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0019047d0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5403860 [select]:
github.com/ethersphere/bee/pkg/storer.(*DB).radiusWorker(0xc0012ac960, {0x12a9e50, 0xc001f9fea0}, 0xe388d0?)
	/home/anatol/swarm/bee/pkg/storer/reserve.go:101 +0x22c
created by github.com/ethersphere/bee/pkg/storer.(*DB).startReserveWorkers
	/home/anatol/swarm/bee/pkg/storer/reserve.go:91 +0x62a

goroutine 5418591 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc000f3a4b0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5388326 [runnable]:
github.com/btcsuite/btcd/btcec.(*fieldVal).SquareVal(0xc000ce5800, 0xc000ce5800)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/field.go:919 +0x17d4
github.com/btcsuite/btcd/btcec.(*fieldVal).Square(...)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/field.go:909
github.com/btcsuite/btcd/btcec.(*fieldVal).Inverse(0xc000ce5800)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/field.go:1182 +0x5e5
github.com/btcsuite/btcd/btcec.(*KoblitzCurve).fieldJacobianToBigAffine(0xc000ce5900?, 0xc000ae7780?, 0xc000ce5950?, 0xc000ce5900)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/btcec.go:98 +0xd0
github.com/btcsuite/btcd/btcec.(*KoblitzCurve).ScalarBaseMult(0x176d2a0, {0xc000ae7780, 0x20, 0x20})
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/btcec.go:886 +0x23e
crypto/ecdsa.generateLegacy({0x12ab998?, 0x176d2a0}, {0x12a45c0, 0xc0001ca000})
	/home/anatol/sdk/go1.20.1/src/crypto/ecdsa/ecdsa_legacy.go:29 +0x122
crypto/ecdsa.GenerateKey({0x12ab998, 0x176d2a0}, {0x12a45c0, 0xc0001ca000})
	/home/anatol/sdk/go1.20.1/src/crypto/ecdsa/ecdsa.go:176 +0x1ab
github.com/ethersphere/bee/pkg/crypto.GenerateSecp256k1Key()
	/home/anatol/swarm/bee/pkg/crypto/crypto.go:64 +0x66
github.com/ethersphere/bee/pkg/storage/testing.GenerateTestRandomChunk()
	/home/anatol/swarm/bee/pkg/storage/testing/chunk.go:34 +0x2a
github.com/ethersphere/bee/pkg/storage/testing.GenerateTestRandomChunks(0xa)
	/home/anatol/swarm/bee/pkg/storage/testing/chunk.go:80 +0x65
github.com/ethersphere/bee/pkg/storer_test.testDebugInfo.func1(0xc002f62ea0)
	/home/anatol/swarm/bee/pkg/storer/debug_test.go:39 +0x2fa
testing.tRunner(0xc002f62ea0, 0xc00195c670)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5416630 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc00258c0c0, 0xc000182c00)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5415583 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc002e6f6c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5384320 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc00280e380)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5401236 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001904460)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5418064 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc002090730)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5356250 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5418061 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000bd36c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5388334 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc002f63ba0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.testNetStore.func1(0x0?)
	/home/anatol/swarm/bee/pkg/storer/netstore_test.go:34 +0x3a
testing.tRunner(0xc002f63ba0, 0xc00195ca80)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5346022 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc002e5c3c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5397161 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc0010fe820)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.TestSubscribeBin.func1.4(0xc0010fe820)
	/home/anatol/swarm/bee/pkg/storer/reserve_test.go:532 +0x72
testing.tRunner(0xc0010fe820, 0xc0000b3830)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5346033 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001904820)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5416554 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc002e5c4b0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5346406 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc001616000, 0x11b4708)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5418400 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000e73500)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5346028 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc0002572c0, 0xc0018201e0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5401235 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc00258c900, 0xc001365440)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5419126 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001b3c190)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5346024 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc002e6fc00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5360059 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5346029 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001904780)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5346026 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc002e6fc00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5384999 [select]:
github.com/ethersphere/bee/pkg/storer_test.TestSubscribeBin.func1.4(0xc001e744e0)
	/home/anatol/swarm/bee/pkg/storer/reserve_test.go:540 +0x169
testing.tRunner(0xc001e744e0, 0xc001e42c00)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5346331 [select]:
github.com/ethersphere/bee/pkg/spinlock.WaitWithInterval(0xc0016109c0?, 0xc001acdd48?, 0xc000f09d30)
	/home/anatol/swarm/bee/pkg/spinlock/wait.go:29 +0x205
github.com/ethersphere/bee/pkg/spinlock.Wait(...)
	/home/anatol/swarm/bee/pkg/spinlock/wait.go:16
github.com/ethersphere/bee/pkg/storer_test.TestRadiusManager.func1(0xc0016109c0, 0xc00241b720, 0x0)
	/home/anatol/swarm/bee/pkg/storer/reserve_test.go:391 +0x8e
github.com/ethersphere/bee/pkg/storer_test.TestRadiusManager.func3(0xc0016109c0)
	/home/anatol/swarm/bee/pkg/storer/reserve_test.go:446 +0x4fb
testing.tRunner(0xc0016109c0, 0xc00144c4e0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5403657 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc0009ae8c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5377027 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001b3df40)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5420804 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc000f3a2d0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5416559 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc002e20af0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5397470 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5401237 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc00258c960, 0xc001365620)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5416556 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc0005bf500)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5346364 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc0031464e0, 0x11b47b8)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5415962 [select]:
github.com/ethersphere/bee/pkg/storer.(*DB).SubscribeBin.func1()
	/home/anatol/swarm/bee/pkg/storer/reserve.go:699 +0x5a9
created by github.com/ethersphere/bee/pkg/storer.(*DB).SubscribeBin
	/home/anatol/swarm/bee/pkg/storer/reserve.go:670 +0x2cf

goroutine 5385458 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002814720, 0xc00282a240)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5346409 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc0016164e0, 0x11b4768)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5346411 [select]:
github.com/ethersphere/bee/pkg/storer.(*DB).Close(0xc000146b40)
	/home/anatol/swarm/bee/pkg/storer/storer.go:626 +0x5a5
github.com/ethersphere/bee/pkg/storer_test.newStorer.func1()
	/home/anatol/swarm/bee/pkg/storer/storer_test.go:233 +0x47
testing.(*common).Cleanup.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1150 +0x194
testing.(*common).runCleanup(0xc001616820, 0x0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1328 +0x1ea
testing.tRunner.func2()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1570 +0x53
testing.tRunner(0xc001616820, 0x11b47d0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 295 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5375935 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001b3dea0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5402684 [chan receive]:
testing.(*T).Parallel(0xc0008304e0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1384 +0x569
github.com/ethersphere/bee/pkg/storer_test.testUploadStore.func2(0xc0008304e0)
	/home/anatol/swarm/bee/pkg/storer/uploadstore_test.go:44 +0x3a
testing.tRunner(0xc0008304e0, 0xc001c88ef0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5346023 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc002e6fc00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5420624 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc00081b500)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5385461 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001f9ef00)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5419128 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001b3c1e0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5416648 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0001809b0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5346642 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc0002573e0, 0xc001820780)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5346330 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc001610820, 0xc001c3e4e0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5421058 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc0024c84b0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5420625 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc00081b500)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5397160 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc0030b7d40)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.TestSubscribeBin.func1.3(0xc0030b7d40)
	/home/anatol/swarm/bee/pkg/storer/reserve_test.go:515 +0x6c
testing.tRunner(0xc0030b7d40, 0xc0000b37d0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5381048 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc001f9ac40)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5416645 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002f590e0, 0xc001de1b60)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5417356 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc0002444b0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5418391 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002709d40, 0xc0004b5e00)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5416629 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc002e20be0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5402683 [chan receive]:
testing.(*T).Parallel(0xc000830340)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1384 +0x569
github.com/ethersphere/bee/pkg/storer_test.testUploadStore.func1(0xc000830340)
	/home/anatol/swarm/bee/pkg/storer/uploadstore_test.go:25 +0x54
testing.tRunner(0xc000830340, 0xc001c88ce0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5416644 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc000180910)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5404003 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc001e476e0, 0xc0010c77a0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5418593 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000fcee00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5418065 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002709c20, 0xc0004b5080)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5412723 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000e94000)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5401507 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5404002 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc00241b590)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5389046 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000724e00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5403665 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc001e47680, 0xc0010c75c0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5388329 [runnable]:
syscall.Syscall(0x8?, 0x1?, 0x488fa5?, 0x4dbc3f?)
	/home/anatol/sdk/go1.20.1/src/syscall/syscall_linux.go:69 +0x31
syscall.Fsync(0xc001440f60?)
	/home/anatol/sdk/go1.20.1/src/syscall/zsyscall_linux_amd64.go:438 +0x3b
internal/poll.(*FD).Fsync.func1(...)
	/home/anatol/sdk/go1.20.1/src/internal/poll/fd_fsync_posix.go:18
internal/poll.ignoringEINTR(...)
	/home/anatol/sdk/go1.20.1/src/internal/poll/fd_posix.go:74
internal/poll.(*FD).Fsync(0xc001440f60)
	/home/anatol/sdk/go1.20.1/src/internal/poll/fd_fsync_posix.go:17 +0x17e
os.(*File).Sync(0xc0000142e0)
	/home/anatol/sdk/go1.20.1/src/os/file_posix.go:168 +0x97
github.com/syndtr/goleveldb/leveldb/storage.syncDir({0xc001e4ba70, 0x24})
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/storage/file_storage_unix.go:94 +0xbf
github.com/syndtr/goleveldb/leveldb/storage.(*fileWrap).Sync(0xc001e21ef0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/storage/file_storage.go:591 +0xb6
github.com/syndtr/goleveldb/leveldb.(*session).newManifest(0xc0024c84b0, 0x0, 0x0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:457 +0x705
github.com/syndtr/goleveldb/leveldb.(*session).create(...)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:125
github.com/syndtr/goleveldb/leveldb.Open({0x12b01c8, 0xc00082a700}, 0xc000fc6100?)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:194 +0x288
github.com/syndtr/goleveldb/leveldb.OpenFile({0xc001e4ba70, 0x24}, 0xc000b12f00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:225 +0x88
github.com/ethersphere/bee/pkg/shed.NewDB({0xc001e4ba70, 0x24}, 0x0)
	/home/anatol/swarm/bee/pkg/shed/db.go:75 +0x237
github.com/ethersphere/bee/pkg/storer.epochMigration({0x12a9e88, 0xc0001ac008}, {0xc001e4ba70, 0x24}, {0x0?, 0x0}, {0x12af938, 0xc001f5cc90}, {0x12a8020, 0xc001c801e0}, ...)
	/home/anatol/swarm/bee/pkg/storer/epoch_migration.go:117 +0x36c
github.com/ethersphere/bee/pkg/storer.performEpochMigration({0x12a9e88, 0xc0001ac008}, {0xc001e4ba70, 0x24}, 0xc0027f60b0)
	/home/anatol/swarm/bee/pkg/storer/storer.go:412 +0x449
github.com/ethersphere/bee/pkg/storer.New({0x12a9e88, 0xc0001ac008}, {0xc001e4ba70, 0x24}, 0xc0027f60b0)
	/home/anatol/swarm/bee/pkg/storer/storer.go:509 +0x41c
github.com/ethersphere/bee/pkg/storer_test.newStorer({0x12b2880, 0xc0016161a0}, {0xc001e4ba70, 0x24}, 0xc001e4ab40?)
	/home/anatol/swarm/bee/pkg/storer/storer_test.go:230 +0x7f
github.com/ethersphere/bee/pkg/storer_test.diskStorer.func1()
	/home/anatol/swarm/bee/pkg/storer/storer_test.go:246 +0x6a
github.com/ethersphere/bee/pkg/storer_test.testDebugInfo.func1(0xc002f63380)
	/home/anatol/swarm/bee/pkg/storer/debug_test.go:24 +0x64
testing.tRunner(0xc002f63380, 0xc00195c7e0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5377029 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001b3df90)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5414826 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc0023111a0, 0xc0012ceba0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5418062 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc000bd36c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5385460 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002814780, 0xc00282a420)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5418386 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc002090780)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5375932 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc000b4a1c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5399675 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc0004921c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5419127 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc00222b8c0, 0xc000844e40)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5414824 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc00089fa40)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5406998 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc0010cae00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5399677 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc0004921c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5416429 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc00117e2d0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5408705 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5377028 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc000598ea0, 0xc000e408a0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5414223 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002310600, 0xc001821920)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5418592 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc000fcee00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5399680 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0019043c0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5420827 [runnable]:
syscall.Syscall(0x8?, 0x1?, 0x488fa5?, 0x4dbc3f?)
	/home/anatol/sdk/go1.20.1/src/syscall/syscall_linux.go:69 +0x31
syscall.Fsync(0xc000139ce0?)
	/home/anatol/sdk/go1.20.1/src/syscall/zsyscall_linux_amd64.go:438 +0x3b
internal/poll.(*FD).Fsync.func1(...)
	/home/anatol/sdk/go1.20.1/src/internal/poll/fd_fsync_posix.go:18
internal/poll.ignoringEINTR(...)
	/home/anatol/sdk/go1.20.1/src/internal/poll/fd_posix.go:74
internal/poll.(*FD).Fsync(0xc000139ce0)
	/home/anatol/sdk/go1.20.1/src/internal/poll/fd_fsync_posix.go:17 +0x17e
os.(*File).Sync(0xc0001a42b8)
	/home/anatol/sdk/go1.20.1/src/os/file_posix.go:168 +0x97
github.com/ethersphere/bee/pkg/sharky.(*slots).save(0xc000139d40)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:54 +0x15b
github.com/ethersphere/bee/pkg/sharky.(*shard).close(0xc0030ae640)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:153 +0x85
github.com/ethersphere/bee/pkg/sharky.(*Store).Close(0xc0012a6680)
	/home/anatol/swarm/bee/pkg/sharky/store.go:71 +0xae
github.com/ethersphere/bee/pkg/storer.closer.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:208 +0xd0
github.com/ethersphere/bee/pkg/storer.closerFn.Close(0xc0013a0220)
	/home/anatol/swarm/bee/pkg/storer/storer.go:202 +0x2f
github.com/ethersphere/bee/pkg/storer.(*DB).Close.func3()
	/home/anatol/swarm/bee/pkg/storer/storer.go:615 +0xbe
created by github.com/ethersphere/bee/pkg/storer.(*DB).Close
	/home/anatol/swarm/bee/pkg/storer/storer.go:613 +0x3a6

goroutine 5414829 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0006b8f00)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5414221 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc0023105a0, 0xc000183920)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5385464 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002814840, 0xc00282a7e0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5414220 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0006b8640)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5416646 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc000180960)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5403659 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc0009ae8c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5403656 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc0009ae8c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5346337 [runnable]:
github.com/btcsuite/btcd/btcec.(*fieldVal).SquareVal(0xc0017d9438, 0xc0017d9438)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/field.go:919 +0x17d4
github.com/btcsuite/btcd/btcec.(*fieldVal).Square(...)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/field.go:909
github.com/btcsuite/btcd/btcec.(*fieldVal).Inverse(0xc0017d9438)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/field.go:1187 +0x6ae
github.com/btcsuite/btcd/btcec.(*KoblitzCurve).fieldJacobianToBigAffine(0xc0017d9548?, 0x1f?, 0xc002aa9b00?, 0xc0017d9548)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/btcec.go:98 +0xd0
github.com/btcsuite/btcd/btcec.(*KoblitzCurve).Add(0x176d2a0?, 0xc001ce7580, 0xc001ce75c0, 0xc001ce75e0, 0xc001ce7600)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/btcec.go:481 +0x2fd
github.com/btcsuite/btcd/btcec.recoverKeyFromSignature(0x176d2a0, 0xc001472620, {0xc002536ac0, 0x20, 0x20}, 0x0, 0x1)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/signature.go:354 +0xa6b
github.com/btcsuite/btcd/btcec.SignCompact(0x176d2a0, 0xc001644600, {0xc002536ac0, 0x20, 0x20}, 0x0)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/signature.go:381 +0x145
github.com/ethersphere/bee/pkg/crypto.(*defaultSigner).sign(0xc0001a4790, {0xc002536ac0, 0x20, 0x20}, 0x0?)
	/home/anatol/swarm/bee/pkg/crypto/signer.go:138 +0x87
github.com/ethersphere/bee/pkg/crypto.(*defaultSigner).Sign(0xc002536a40?, {0xc002536a80, 0x20, 0x20})
	/home/anatol/swarm/bee/pkg/crypto/signer.go:88 +0xa7
github.com/ethersphere/bee/pkg/postage/testing.MustNewValidSignature({0x12aa8a8, 0xc0001a4790}, {{0xc0025369e0?, 0xc0025369e0?, 0x4d6a8f?}}, {0xc002536a00, 0x20, 0x20}, {0xc002536a20, 0x8, ...}, ...)
	/home/anatol/swarm/bee/pkg/postage/testing/stamp.go:36 +0x117
github.com/ethersphere/bee/pkg/postage/testing.MustNewValidStamp({0x12aa8a8, 0xc0001a4790}, {{0xc0025369e0?, 0xc000501800?, 0xc0012acd20?}})
	/home/anatol/swarm/bee/pkg/postage/testing/stamp.go:54 +0x137
github.com/ethersphere/bee/pkg/storage/testing.GenerateTestRandomChunk()
	/home/anatol/swarm/bee/pkg/storage/testing/chunk.go:40 +0xd6
github.com/ethersphere/bee/pkg/storage/testing.GenerateTestRandomChunks(0x1e)
	/home/anatol/swarm/bee/pkg/storage/testing/chunk.go:80 +0x65
github.com/ethersphere/bee/pkg/storer_test.testUploadStore(0xc001611520, 0xc002eb1800)
	/home/anatol/swarm/bee/pkg/storer/uploadstore_test.go:70 +0x1cb
github.com/ethersphere/bee/pkg/storer_test.TestUploadStore.func2(0x0?)
	/home/anatol/swarm/bee/pkg/storer/uploadstore_test.go:342 +0x7e
testing.tRunner(0xc001611520, 0x11b4878)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5419123 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc000fcee00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5414218 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc00089f880)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5413334 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5384316 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc001a763c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5413331 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000d8b500)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5346030 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc000257320, 0xc0018203c0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5416647 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002f591a0, 0xc001de1d40)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5414978 [select]:
github.com/ethersphere/bee/pkg/storer.(*DB).SubscribeBin.func1()
	/home/anatol/swarm/bee/pkg/storer/reserve.go:699 +0x5a9
created by github.com/ethersphere/bee/pkg/storer.(*DB).SubscribeBin
	/home/anatol/swarm/bee/pkg/storer/reserve.go:670 +0x2cf

goroutine 5403662 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc00241b4f0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5403660 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc00241b4a0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5401720 [chan receive]:
testing.(*T).Parallel(0xc0008301a0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1384 +0x569
github.com/ethersphere/bee/pkg/storer_test.testUploadStore.func2(0xc0008301a0)
	/home/anatol/swarm/bee/pkg/storer/uploadstore_test.go:44 +0x3a
testing.tRunner(0xc0008301a0, 0xc001c88850)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5346032 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc000257380, 0xc0018205a0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5413835 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc0016e9880)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5346407 [chan receive]:
testing.tRunner.func1()
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1542 +0x8eb
testing.tRunner(0xc0016161a0, 0x11b4710)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1582 +0x255
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5413002 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000ea7c00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5375934 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5404390 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000d12a80)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5416430 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc000792380)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5414828 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002311260, 0xc0012ced80)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5418060 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc000bd36c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5419122 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc000fcee00)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5346336 [runnable]:
github.com/btcsuite/btcd/btcec.(*fieldVal).Add(0xc002fc7190, 0xc002fc71b8)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/field.go:548 +0x396
github.com/btcsuite/btcd/btcec.(*KoblitzCurve).addZ2EqualsOne(0xc002fc7538?, 0xc002fc7588, 0xc002fc7560, 0xc00140c878?, 0xc002fc74c0, 0xc002fc7470, 0xc002fc7588, 0xc002fc7560, 0xc002fc7538)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/btcec.go:323 +0xaab
github.com/btcsuite/btcd/btcec.(*KoblitzCurve).addJacobian(0x10?, 0xc002fc7588, 0xc002fc7560, 0xc002fc7538, 0xc002fc74c0, 0xc002fc7470, 0xc002fc7448, 0xc002fc7588, 0xc002fc7560, 0xc002fc7538)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/btcec.go:450 +0xb45
github.com/btcsuite/btcd/btcec.(*KoblitzCurve).ScalarMult(0x176d2a0, 0xc002fc77d0?, 0xc0006be040?, {0xc0025f0e60, 0x20, 0x20})
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/btcec.go:850 +0x8e5
github.com/btcsuite/btcd/btcec.recoverKeyFromSignature(0x176d2a0, 0xc00185eaa0, {0xc0025f0c60, 0x20, 0x20}, 0x0, 0x1)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/signature.go:343 +0x91c
github.com/btcsuite/btcd/btcec.SignCompact(0x176d2a0, 0xc0009aa000, {0xc0025f0c60, 0x20, 0x20}, 0x0)
	/home/anatol/go/pkg/mod/github.com/btcsuite/btcd@v0.22.1/btcec/signature.go:381 +0x145
github.com/ethersphere/bee/pkg/crypto.(*defaultSigner).sign(0xc000132840, {0xc0025f0c60, 0x20, 0x20}, 0x0?)
	/home/anatol/swarm/bee/pkg/crypto/signer.go:138 +0x87
github.com/ethersphere/bee/pkg/crypto.(*defaultSigner).Sign(0xc0025f0be0?, {0xc0025f0c20, 0x20, 0x20})
	/home/anatol/swarm/bee/pkg/crypto/signer.go:88 +0xa7
github.com/ethersphere/bee/pkg/postage/testing.MustNewValidSignature({0x12aa8a8, 0xc000132840}, {{0xc0025f0b80?, 0xc0025f0b80?, 0x4d6a8f?}}, {0xc0025f0ba0, 0x20, 0x20}, {0xc0025f0bc0, 0x8, ...}, ...)
	/home/anatol/swarm/bee/pkg/postage/testing/stamp.go:36 +0x117
github.com/ethersphere/bee/pkg/postage/testing.MustNewValidStamp({0x12aa8a8, 0xc000132840}, {{0xc0025f0b80?, 0xc00007ac00?, 0xc000146960?}})
	/home/anatol/swarm/bee/pkg/postage/testing/stamp.go:54 +0x137
github.com/ethersphere/bee/pkg/storage/testing.GenerateTestRandomChunk()
	/home/anatol/swarm/bee/pkg/storage/testing/chunk.go:40 +0xd6
github.com/ethersphere/bee/pkg/storage/testing.GenerateTestRandomChunks(0x1e)
	/home/anatol/swarm/bee/pkg/storage/testing/chunk.go:80 +0x65
github.com/ethersphere/bee/pkg/storer_test.testUploadStore(0xc001610000, 0xc001c88770)
	/home/anatol/swarm/bee/pkg/storer/uploadstore_test.go:70 +0x1cb
github.com/ethersphere/bee/pkg/storer_test.TestUploadStore.func1(0xc001610000)
	/home/anatol/swarm/bee/pkg/storer/uploadstore_test.go:335 +0x9b
testing.tRunner(0xc001610000, 0x11b4870)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5397159 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc0030b7ba0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.TestSubscribeBin.func1.2(0xc0030b7ba0)
	/home/anatol/swarm/bee/pkg/storer/reserve_test.go:500 +0x52
testing.tRunner(0xc0030b7ba0, 0xc001c88100)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5397158 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc0030b7a00)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.TestSubscribeBin.func1.1(0xc0030b7a00)
	/home/anatol/swarm/bee/pkg/storer/reserve_test.go:483 +0x6c
testing.tRunner(0xc0030b7a00, 0xc0000b3740)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5414219 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc00089f880)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5418395 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000e73340)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5385462 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc0028147e0, 0xc00282a600)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5415110 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc0007fc000)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5346027 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001904730)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5418389 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002709ce0, 0xc0004b5440)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5375930 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc000b4a1c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5388331 [runnable]:
github.com/syndtr/goleveldb/leveldb/memdb.New({0x12a4c00?, 0xc001a1bb80}, 0x2000000)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/memdb/memdb.go:474 +0x1a5
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolGet(0xc0007fefc0, 0x0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:90 +0x1c5
github.com/syndtr/goleveldb/leveldb.(*DB).newMem(0xc0007fefc0, 0x26d50e0?)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:147 +0x619
github.com/syndtr/goleveldb/leveldb.(*DB).recoverJournal(0xc0007fefc0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:619 +0x788
github.com/syndtr/goleveldb/leveldb.openDB(0xc000f3a2d0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:131 +0x89a
github.com/syndtr/goleveldb/leveldb.Open({0x12b01c8, 0xc000912cb0}, 0xc000883400?)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:203 +0x35d
github.com/syndtr/goleveldb/leveldb.OpenFile({0xc001c02c00, 0x2f}, 0xc0016843c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:225 +0x88
github.com/ethersphere/bee/pkg/storage/leveldbstore.New({0xc001c02c00, 0x2f}, 0xc0016843c0)
	/home/anatol/swarm/bee/pkg/storage/leveldbstore/store.go:68 +0x10b
github.com/ethersphere/bee/pkg/storer.initStore({0xc001c02900, 0x24}, 0xc0027f60b0)
	/home/anatol/swarm/bee/pkg/storer/storer.go:260 +0x3bd
github.com/ethersphere/bee/pkg/storer.performEpochMigration({0x12a9e88, 0xc0001ac008}, {0xc001c02900, 0x24}, 0xc0027f60b0)
	/home/anatol/swarm/bee/pkg/storer/storer.go:379 +0x85
github.com/ethersphere/bee/pkg/storer.New({0x12a9e88, 0xc0001ac008}, {0xc001c02900, 0x24}, 0xc0027f60b0)
	/home/anatol/swarm/bee/pkg/storer/storer.go:509 +0x41c
github.com/ethersphere/bee/pkg/storer_test.newStorer({0x12b2880, 0xc0016161a0}, {0xc001c02900, 0x24}, 0xc000adfd40?)
	/home/anatol/swarm/bee/pkg/storer/storer_test.go:230 +0x7f
github.com/ethersphere/bee/pkg/storer_test.diskStorer.func1()
	/home/anatol/swarm/bee/pkg/storer/storer_test.go:246 +0x6a
github.com/ethersphere/bee/pkg/storer_test.testDebugInfo.func3(0xc002f636c0)
	/home/anatol/swarm/bee/pkg/storer/debug_test.go:125 +0x64
testing.tRunner(0xc002f636c0, 0xc00195c8b0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5416628 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc00258c060, 0xc0017cdf20)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5411993 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000c90000)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5416561 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc002e20b40)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5414217 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc00089f880)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5354390 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5420622 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc00081b500)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5420623 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc00081b500)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:101 +0x12f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5388335 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc002f63d40)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.testNetStore.func2(0x0?)
	/home/anatol/swarm/bee/pkg/storer/netstore_test.go:236 +0x3a
testing.tRunner(0xc002f63d40, 0xc00195cae0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5403999 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc001a76690)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5411982 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc0009781a0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.testListDeleteSessions.func2(0xc0009781a0)
	/home/anatol/swarm/bee/pkg/storer/uploadstore_test.go:287 +0x57
testing.tRunner(0xc0009781a0, 0xc0014724b0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5388333 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc002f63a00)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.testNetStore.func2(0x0?)
	/home/anatol/swarm/bee/pkg/storer/netstore_test.go:236 +0x3a
testing.tRunner(0xc002f63a00, 0xc00195c9c0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5384317 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc00280e380)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5416555 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc0005bf500)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5413838 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5414216 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).compactionError(0xc00089f880)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:91 +0x1cf
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:148 +0x93f

goroutine 5420828 [chan receive]:
github.com/ethersphere/bee/pkg/storer.(*DB).Close.func4()
	/home/anatol/swarm/bee/pkg/storer/storer.go:621 +0x9a
created by github.com/ethersphere/bee/pkg/storer.(*DB).Close
	/home/anatol/swarm/bee/pkg/storer/storer.go:619 +0x515

goroutine 5387403 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5418390 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc002090870)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5375937 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001b3def0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5419125 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc00222b860, 0xc000844c60)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5414827 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0006b8eb0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5403661 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc001e47560, 0xc0010c7200)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5416642 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0001808c0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5414087 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc00062b180)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5420621 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc0024c83c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5414090 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5399679 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc00258c7e0, 0xc001365080)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5416558 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc0005bf500)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5412511 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc000cb68c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5411905 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc0009cbdc0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5416226 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5414222 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0006b86e0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5375933 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc000b4a1c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5419130 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001b3c230)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5388332 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc002f63860)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.testNetStore.func1(0x0?)
	/home/anatol/swarm/bee/pkg/storer/netstore_test.go:34 +0x3a
testing.tRunner(0xc002f63860, 0xc00195c960)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5389049 [select]:
github.com/ethersphere/bee/pkg/storer.initDiskRepository.func1()
	/home/anatol/swarm/bee/pkg/storer/storer.go:289 +0x1ef
created by github.com/ethersphere/bee/pkg/storer.initDiskRepository
	/home/anatol/swarm/bee/pkg/storer/storer.go:282 +0x2a5

goroutine 5414830 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002311320, 0xc0012cef60)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5403658 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc0009ae8c0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5416649 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc002f59260, 0xc001de1f20)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5377030 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc000599020, 0xc000e40a80)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5414215 [select]:
github.com/syndtr/goleveldb/leveldb.(*session).refLoop(0xc001a762d0)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session_util.go:189 +0x75a
created by github.com/syndtr/goleveldb/leveldb.newSession
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/session.go:93 +0x573

goroutine 5388330 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc002f63520)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.testDebugInfo.func2(0xc002f63520)
	/home/anatol/swarm/bee/pkg/storer/debug_test.go:84 +0x57
testing.tRunner(0xc002f63520, 0xc00195c840)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5410776 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc0005fc700)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5385459 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc001f9eeb0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5414818 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0006b8870)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5418388 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0020907d0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5416433 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction(0xc000792380)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:773 +0x1cd
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:156 +0xafa

goroutine 5408702 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain(0xc0014d1180)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_state.go:110 +0x205
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:149 +0x9bf

goroutine 5412644 [chan receive]:
testing.(*testContext).waitParallel(0xc000036230)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1696 +0x139
testing.(*T).Parallel(0xc000e841a0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1385 +0x599
github.com/ethersphere/bee/pkg/storer_test.TestNew.func4.2(0x0?)
	/home/anatol/swarm/bee/pkg/storer/storer_test.go:147 +0x31
testing.tRunner(0xc000e841a0, 0x11b47b0)
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
	/home/anatol/sdk/go1.20.1/src/testing/testing.go:1629 +0x806

goroutine 5416557 [select]:
github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction(0xc0005bf500)
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db_compaction.go:836 +0xa2f
created by github.com/syndtr/goleveldb/leveldb.openDB
	/home/anatol/go/pkg/mod/github.com/syndtr/goleveldb@v1.0.1-0.20210819022825-2ae1ddf74ef7/leveldb/db.go:155 +0xa7a

goroutine 5416626 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc00258c000, 0xc0017cdd40)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5414224 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0006b87d0)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705

goroutine 5404001 [select]:
github.com/ethersphere/bee/pkg/storer.(*DB).evictionWorker(0xc0012ac960, {0x12a9e50, 0xc001f9fea0})
	/home/anatol/swarm/bee/pkg/storer/reserve.go:247 +0x7ce
created by github.com/ethersphere/bee/pkg/storer.(*DB).startReserveWorkers
	/home/anatol/swarm/bee/pkg/storer/reserve.go:63 +0x26e

goroutine 5375936 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc000598ba0, 0xc000e404e0)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5414819 [select]:
github.com/ethersphere/bee/pkg/sharky.(*slots).process(0xc0023106c0, 0xc001821f20)
	/home/anatol/swarm/bee/pkg/sharky/slots.go:107 +0x17c
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func2()
	/home/anatol/swarm/bee/pkg/sharky/store.go:112 +0xd3
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:110 +0x848

goroutine 5414825 [select]:
github.com/ethersphere/bee/pkg/sharky.(*shard).process(0xc0006b8e60)
	/home/anatol/swarm/bee/pkg/sharky/shard.go:110 +0x2cd
github.com/ethersphere/bee/pkg/sharky.(*Store).create.func1()
	/home/anatol/swarm/bee/pkg/sharky/store.go:106 +0xa8
created by github.com/ethersphere/bee/pkg/sharky.(*Store).create
	/home/anatol/swarm/bee/pkg/sharky/store.go:104 +0x705
FAIL	github.com/ethersphere/bee/pkg/storer	31.131s
FAIL
