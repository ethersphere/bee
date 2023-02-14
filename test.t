go test -failfast -v ./...
?   	github.com/ethersphere/bee	[no test files]
?   	github.com/ethersphere/bee/cmd/bee	[no test files]
=== RUN   TestVersionCmd
=== PAUSE TestVersionCmd
=== CONT  TestVersionCmd
--- PASS: TestVersionCmd (0.00s)
PASS
ok  	github.com/ethersphere/bee/cmd/bee/cmd	(cached)
?   	github.com/ethersphere/bee/pkg/accounting/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/auth/mock	[no test files]
=== RUN   TestMutex
=== PAUSE TestMutex
=== RUN   TestAccountingAddBalance
=== PAUSE TestAccountingAddBalance
=== RUN   TestAccountingAddOriginatedBalance
=== PAUSE TestAccountingAddOriginatedBalance
=== RUN   TestAccountingAdd_persistentBalances
=== PAUSE TestAccountingAdd_persistentBalances
=== RUN   TestAccountingReserve
=== PAUSE TestAccountingReserve
=== RUN   TestAccountingReserveAheadOfTime
=== PAUSE TestAccountingReserveAheadOfTime
=== RUN   TestAccountingDisconnect
=== PAUSE TestAccountingDisconnect
=== RUN   TestAccountingCallSettlement
=== PAUSE TestAccountingCallSettlement
=== RUN   TestAccountingCallSettlementMonetary
=== PAUSE TestAccountingCallSettlementMonetary
=== RUN   TestAccountingCallSettlementTooSoon
=== PAUSE TestAccountingCallSettlementTooSoon
=== RUN   TestAccountingCallSettlementEarly
=== PAUSE TestAccountingCallSettlementEarly
=== RUN   TestAccountingSurplusBalance
=== PAUSE TestAccountingSurplusBalance
=== RUN   TestAccountingNotifyPaymentReceived
=== PAUSE TestAccountingNotifyPaymentReceived
=== RUN   TestAccountingConnected
=== PAUSE TestAccountingConnected
=== RUN   TestAccountingNotifyPaymentThreshold
=== PAUSE TestAccountingNotifyPaymentThreshold
=== RUN   TestAccountingPeerDebt
=== PAUSE TestAccountingPeerDebt
=== RUN   TestAccountingCallPaymentErrorRetries
=== PAUSE TestAccountingCallPaymentErrorRetries
=== RUN   TestAccountingGhostOverdraft
=== PAUSE TestAccountingGhostOverdraft
=== RUN   TestAccountingReconnectBeforeAllowed
=== PAUSE TestAccountingReconnectBeforeAllowed
=== RUN   TestAccountingResetBalanceAfterReconnect
=== PAUSE TestAccountingResetBalanceAfterReconnect
=== RUN   TestAccountingRefreshGrowingThresholds
=== PAUSE TestAccountingRefreshGrowingThresholds
=== RUN   TestAccountingRefreshGrowingThresholdsLight
=== PAUSE TestAccountingRefreshGrowingThresholdsLight
=== RUN   TestAccountingSwapGrowingThresholds
=== PAUSE TestAccountingSwapGrowingThresholds
=== RUN   TestAccountingSwapGrowingThresholdsLight
=== PAUSE TestAccountingSwapGrowingThresholdsLight
=== CONT  TestMutex
=== CONT  TestAccountingNotifyPaymentReceived
=== RUN   TestMutex/locked_mutex_can_not_be_locked_again
=== PAUSE TestMutex/locked_mutex_can_not_be_locked_again
=== RUN   TestMutex/can_lock_after_release
=== PAUSE TestMutex/can_lock_after_release
=== RUN   TestMutex/locked_mutex_takes_context_into_account
=== PAUSE TestMutex/locked_mutex_takes_context_into_account
=== CONT  TestMutex/locked_mutex_can_not_be_locked_again
=== CONT  TestAccountingSurplusBalance
--- PASS: TestAccountingNotifyPaymentReceived (0.00s)
=== CONT  TestAccountingSwapGrowingThresholdsLight
--- PASS: TestAccountingSurplusBalance (0.00s)
=== CONT  TestAccountingCallSettlementEarly
--- PASS: TestAccountingCallSettlementEarly (0.00s)
=== CONT  TestAccountingCallSettlementTooSoon
--- PASS: TestAccountingCallSettlementTooSoon (0.00s)
=== CONT  TestAccountingCallSettlementMonetary
=== CONT  TestAccountingCallSettlement
--- PASS: TestAccountingCallSettlement (0.00s)
=== CONT  TestAccountingDisconnect
--- PASS: TestAccountingDisconnect (0.00s)
=== CONT  TestAccountingReserveAheadOfTime
=== CONT  TestAccountingReserve
--- PASS: TestAccountingReserve (0.00s)
=== CONT  TestAccountingAdd_persistentBalances
--- PASS: TestAccountingAdd_persistentBalances (0.00s)
=== CONT  TestAccountingAddOriginatedBalance
--- PASS: TestAccountingAddOriginatedBalance (0.00s)
=== CONT  TestAccountingAddBalance
--- PASS: TestAccountingAddBalance (0.00s)
=== CONT  TestMutex/locked_mutex_takes_context_into_account
=== CONT  TestMutex/can_lock_after_release
--- PASS: TestAccountingCallSettlementMonetary (0.00s)
=== CONT  TestAccountingReconnectBeforeAllowed
--- PASS: TestAccountingReconnectBeforeAllowed (0.00s)
=== CONT  TestAccountingSwapGrowingThresholds
=== CONT  TestAccountingGhostOverdraft
=== CONT  TestAccountingCallPaymentErrorRetries
=== CONT  TestAccountingPeerDebt
=== CONT  TestAccountingConnected
--- PASS: TestAccountingConnected (0.00s)
=== CONT  TestAccountingRefreshGrowingThresholds
=== CONT  TestAccountingRefreshGrowingThresholdsLight
--- PASS: TestAccountingGhostOverdraft (0.00s)
=== CONT  TestAccountingResetBalanceAfterReconnect
--- PASS: TestAccountingResetBalanceAfterReconnect (0.00s)
=== CONT  TestAccountingNotifyPaymentThreshold
--- PASS: TestAccountingNotifyPaymentThreshold (0.00s)
--- PASS: TestAccountingPeerDebt (0.01s)
--- PASS: TestMutex (0.00s)
    --- PASS: TestMutex/locked_mutex_can_not_be_locked_again (0.00s)
    --- PASS: TestMutex/can_lock_after_release (0.00s)
    --- PASS: TestMutex/locked_mutex_takes_context_into_account (0.00s)
--- PASS: TestAccountingCallPaymentErrorRetries (0.01s)
--- PASS: TestAccountingSwapGrowingThresholdsLight (0.10s)
--- PASS: TestAccountingSwapGrowingThresholds (0.14s)
--- PASS: TestAccountingRefreshGrowingThresholds (0.16s)
--- PASS: TestAccountingRefreshGrowingThresholdsLight (0.19s)
--- PASS: TestAccountingReserveAheadOfTime (1.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/accounting	(cached)
=== RUN   TestInMem
=== PAUSE TestInMem
=== CONT  TestInMem
--- PASS: TestInMem (0.03s)
PASS
ok  	github.com/ethersphere/bee/pkg/addressbook	(cached)
=== RUN   TestAccountingInfo
=== PAUSE TestAccountingInfo
=== RUN   TestAccountingInfoError
=== PAUSE TestAccountingInfoError
=== RUN   TestParseName
=== PAUSE TestParseName
=== RUN   TestCalculateNumberOfChunks
=== PAUSE TestCalculateNumberOfChunks
=== RUN   TestCalculateNumberOfChunksEncrypted
=== PAUSE TestCalculateNumberOfChunksEncrypted
=== RUN   TestPostageHeaderError
=== PAUSE TestPostageHeaderError
=== RUN   TestOptions
=== PAUSE TestOptions
=== RUN   TestPostageDirectAndDeferred_FLAKY
=== PAUSE TestPostageDirectAndDeferred_FLAKY
=== RUN   TestAuth
=== RUN   TestAuth/missing_authorization_header
=== RUN   TestAuth/missing_role
=== RUN   TestAuth/bad_authorization_header
=== RUN   TestAuth/bad_request_body
=== RUN   TestAuth/unauthorized
=== RUN   TestAuth/failed_to_add_key
=== RUN   TestAuth/success
--- PASS: TestAuth (0.04s)
    --- PASS: TestAuth/missing_authorization_header (0.00s)
    --- PASS: TestAuth/missing_role (0.01s)
    --- PASS: TestAuth/bad_authorization_header (0.00s)
    --- PASS: TestAuth/bad_request_body (0.00s)
    --- PASS: TestAuth/unauthorized (0.00s)
    --- PASS: TestAuth/failed_to_add_key (0.00s)
    --- PASS: TestAuth/success (0.00s)
=== RUN   TestBalances
=== PAUSE TestBalances
=== RUN   TestBalancesError
=== PAUSE TestBalancesError
=== RUN   TestBalancesPeers
=== PAUSE TestBalancesPeers
=== RUN   TestBalancesPeersError
=== PAUSE TestBalancesPeersError
=== RUN   TestBalancesPeersNoBalance
=== PAUSE TestBalancesPeersNoBalance
=== RUN   TestConsumedBalances
=== PAUSE TestConsumedBalances
=== RUN   TestConsumedError
=== PAUSE TestConsumedError
=== RUN   TestConsumedPeers
=== PAUSE TestConsumedPeers
=== RUN   TestConsumedPeersError
=== PAUSE TestConsumedPeersError
=== RUN   TestConsumedPeersNoBalance
=== PAUSE TestConsumedPeersNoBalance
=== RUN   Test_peerBalanceHandler_invalidInputs
=== PAUSE Test_peerBalanceHandler_invalidInputs
=== RUN   Test_compensatedPeerBalanceHandler_invalidInputs
=== PAUSE Test_compensatedPeerBalanceHandler_invalidInputs
=== RUN   TestBytes
=== RUN   TestBytes/upload
=== RUN   TestBytes/upload-with-pinning
=== RUN   TestBytes/download
=== RUN   TestBytes/head
=== RUN   TestBytes/head_with_compression
=== RUN   TestBytes/internal_error
--- PASS: TestBytes (0.05s)
    --- PASS: TestBytes/upload (0.02s)
    --- PASS: TestBytes/upload-with-pinning (0.00s)
    --- PASS: TestBytes/download (0.03s)
    --- PASS: TestBytes/head (0.00s)
    --- PASS: TestBytes/head_with_compression (0.00s)
    --- PASS: TestBytes/internal_error (0.00s)
=== RUN   TestBytesInvalidStamp
=== RUN   TestBytesInvalidStamp/upload,_batch_not_found
=== RUN   TestBytesInvalidStamp/upload,_batch_exists_error
=== RUN   TestBytesInvalidStamp/upload,_batch_unusable
=== RUN   TestBytesInvalidStamp/upload,_invalid_tag
=== RUN   TestBytesInvalidStamp/upload,_tag_not_found
--- PASS: TestBytesInvalidStamp (0.01s)
    --- PASS: TestBytesInvalidStamp/upload,_batch_not_found (0.00s)
    --- PASS: TestBytesInvalidStamp/upload,_batch_exists_error (0.00s)
    --- PASS: TestBytesInvalidStamp/upload,_batch_unusable (0.00s)
    --- PASS: TestBytesInvalidStamp/upload,_invalid_tag (0.00s)
    --- PASS: TestBytesInvalidStamp/upload,_tag_not_found (0.00s)
=== RUN   Test_bytesUploadHandler_invalidInputs
=== PAUSE Test_bytesUploadHandler_invalidInputs
=== RUN   Test_bytesGetHandler_invalidInputs
=== PAUSE Test_bytesGetHandler_invalidInputs
=== RUN   TestDirectUploadBytes
=== PAUSE TestDirectUploadBytes
=== RUN   TestBzzFiles
=== RUN   TestBzzFiles/tar-file-upload
=== RUN   TestBzzFiles/tar-file-upload-with-pinning
=== RUN   TestBzzFiles/encrypt-decrypt
=== RUN   TestBzzFiles/filter_out_filename_path
=== RUN   TestBzzFiles/check-content-type-detection
=== RUN   TestBzzFiles/upload-then-download-and-check-data
=== RUN   TestBzzFiles/upload-then-download-with-targets
--- PASS: TestBzzFiles (0.03s)
    --- PASS: TestBzzFiles/tar-file-upload (0.00s)
    --- PASS: TestBzzFiles/tar-file-upload-with-pinning (0.01s)
    --- PASS: TestBzzFiles/encrypt-decrypt (0.01s)
    --- PASS: TestBzzFiles/filter_out_filename_path (0.00s)
    --- PASS: TestBzzFiles/check-content-type-detection (0.00s)
    --- PASS: TestBzzFiles/upload-then-download-and-check-data (0.00s)
    --- PASS: TestBzzFiles/upload-then-download-with-targets (0.00s)
=== RUN   TestBzzFilesRangeRequests
=== PAUSE TestBzzFilesRangeRequests
=== RUN   TestFeedIndirection
=== PAUSE TestFeedIndirection
=== RUN   Test_bzzDownloadHandler_invalidInputs
=== PAUSE Test_bzzDownloadHandler_invalidInputs
=== RUN   TestInvalidBzzParams
=== PAUSE TestInvalidBzzParams
=== RUN   TestDirectUploadBzz
=== PAUSE TestDirectUploadBzz
=== RUN   TestChequebookBalance
=== PAUSE TestChequebookBalance
=== RUN   TestChequebookBalanceError
=== PAUSE TestChequebookBalanceError
=== RUN   TestChequebookAvailableBalanceError
=== PAUSE TestChequebookAvailableBalanceError
=== RUN   TestChequebookAddress
=== PAUSE TestChequebookAddress
=== RUN   TestChequebookWithdraw
=== PAUSE TestChequebookWithdraw
=== RUN   TestChequebookDeposit
=== PAUSE TestChequebookDeposit
=== RUN   TestChequebookLastCheques
=== PAUSE TestChequebookLastCheques
=== RUN   TestChequebookLastChequesPeer
=== PAUSE TestChequebookLastChequesPeer
=== RUN   TestChequebookCashout
=== PAUSE TestChequebookCashout
=== RUN   TestChequebookCashout_CustomGas
=== PAUSE TestChequebookCashout_CustomGas
=== RUN   TestChequebookCashoutStatus
=== PAUSE TestChequebookCashoutStatus
=== RUN   Test_chequebookLastPeerHandler_invalidInputs
=== PAUSE Test_chequebookLastPeerHandler_invalidInputs
=== RUN   TestChunkUploadStream
=== RUN   TestChunkUploadStream/upload_and_verify
=== RUN   TestChunkUploadStream/close_on_incorrect_msg
--- PASS: TestChunkUploadStream (0.01s)
    --- PASS: TestChunkUploadStream/upload_and_verify (0.01s)
    --- PASS: TestChunkUploadStream/close_on_incorrect_msg (0.00s)
=== RUN   TestChunkUploadDownload
=== RUN   TestChunkUploadDownload/empty_chunk
=== RUN   TestChunkUploadDownload/ok
=== RUN   TestChunkUploadDownload/pin-invalid-value
=== RUN   TestChunkUploadDownload/pin-header-missing
=== RUN   TestChunkUploadDownload/pin-ok
--- PASS: TestChunkUploadDownload (0.02s)
    --- PASS: TestChunkUploadDownload/empty_chunk (0.00s)
    --- PASS: TestChunkUploadDownload/ok (0.00s)
    --- PASS: TestChunkUploadDownload/pin-invalid-value (0.00s)
    --- PASS: TestChunkUploadDownload/pin-header-missing (0.00s)
    --- PASS: TestChunkUploadDownload/pin-ok (0.01s)
=== RUN   TestHasChunkHandler
=== RUN   TestHasChunkHandler/ok
=== RUN   TestHasChunkHandler/not_found
=== RUN   TestHasChunkHandler/bad_address
=== RUN   TestHasChunkHandler/remove-chunk
=== RUN   TestHasChunkHandler/remove-not-present-chunk
--- PASS: TestHasChunkHandler (0.01s)
    --- PASS: TestHasChunkHandler/ok (0.00s)
    --- PASS: TestHasChunkHandler/not_found (0.00s)
    --- PASS: TestHasChunkHandler/bad_address (0.00s)
    --- PASS: TestHasChunkHandler/remove-chunk (0.00s)
    --- PASS: TestHasChunkHandler/remove-not-present-chunk (0.00s)
=== RUN   Test_chunkHandlers_invalidInputs
=== PAUSE Test_chunkHandlers_invalidInputs
=== RUN   TestInvalidChunkParams
=== PAUSE TestInvalidChunkParams
=== RUN   TestDirectChunkUpload
=== PAUSE TestDirectChunkUpload
=== RUN   TestCORSHeaders
=== PAUSE TestCORSHeaders
=== RUN   TestCors
=== PAUSE TestCors
=== RUN   TestCorsStatus
=== PAUSE TestCorsStatus
=== RUN   TestDBIndices
=== PAUSE TestDBIndices
=== RUN   TestDirs
=== RUN   TestDirs/empty_request_body
=== RUN   TestDirs/non_tar_file
=== RUN   TestDirs/wrong_content_type
=== RUN   TestDirs/non-nested_files_without_extension
=== RUN   TestDirs/non-nested_files_without_extension/tar_upload
=== RUN   TestDirs/nested_files_with_extension
=== RUN   TestDirs/nested_files_with_extension/tar_upload
=== RUN   TestDirs/nested_files_with_extension/multipart_upload
=== RUN   TestDirs/no_index_filename
=== RUN   TestDirs/no_index_filename/tar_upload
=== RUN   TestDirs/no_index_filename/multipart_upload
=== RUN   TestDirs/explicit_index_filename
=== RUN   TestDirs/explicit_index_filename/tar_upload
=== RUN   TestDirs/explicit_index_filename/multipart_upload
=== RUN   TestDirs/nested_index_filename
=== RUN   TestDirs/nested_index_filename/tar_upload
=== RUN   TestDirs/explicit_index_and_error_filename
=== RUN   TestDirs/explicit_index_and_error_filename/tar_upload
=== RUN   TestDirs/explicit_index_and_error_filename/multipart_upload
=== RUN   TestDirs/invalid_archive_paths
=== RUN   TestDirs/invalid_archive_paths/tar_upload
=== RUN   TestDirs/encrypted
=== RUN   TestDirs/encrypted/tar_upload
=== RUN   TestDirs/upload,_invalid_tag
=== RUN   TestDirs/upload,_tag_not_found
--- PASS: TestDirs (0.08s)
    --- PASS: TestDirs/empty_request_body (0.00s)
    --- PASS: TestDirs/non_tar_file (0.00s)
    --- PASS: TestDirs/wrong_content_type (0.00s)
    --- PASS: TestDirs/non-nested_files_without_extension (0.02s)
        --- PASS: TestDirs/non-nested_files_without_extension/tar_upload (0.02s)
    --- PASS: TestDirs/nested_files_with_extension (0.01s)
        --- PASS: TestDirs/nested_files_with_extension/tar_upload (0.01s)
        --- PASS: TestDirs/nested_files_with_extension/multipart_upload (0.01s)
    --- PASS: TestDirs/no_index_filename (0.00s)
        --- PASS: TestDirs/no_index_filename/tar_upload (0.00s)
        --- PASS: TestDirs/no_index_filename/multipart_upload (0.00s)
    --- PASS: TestDirs/explicit_index_filename (0.01s)
        --- PASS: TestDirs/explicit_index_filename/tar_upload (0.00s)
        --- PASS: TestDirs/explicit_index_filename/multipart_upload (0.01s)
    --- PASS: TestDirs/nested_index_filename (0.00s)
        --- PASS: TestDirs/nested_index_filename/tar_upload (0.00s)
    --- PASS: TestDirs/explicit_index_and_error_filename (0.01s)
        --- PASS: TestDirs/explicit_index_and_error_filename/tar_upload (0.00s)
        --- PASS: TestDirs/explicit_index_and_error_filename/multipart_upload (0.00s)
    --- PASS: TestDirs/invalid_archive_paths (0.01s)
        --- PASS: TestDirs/invalid_archive_paths/tar_upload (0.01s)
    --- PASS: TestDirs/encrypted (0.00s)
        --- PASS: TestDirs/encrypted/tar_upload (0.00s)
    --- PASS: TestDirs/upload,_invalid_tag (0.00s)
    --- PASS: TestDirs/upload,_tag_not_found (0.01s)
=== RUN   TestEmtpyDir
=== PAUSE TestEmtpyDir
=== RUN   TestFeed_Get
=== PAUSE TestFeed_Get
=== RUN   TestFeed_Post
=== RUN   TestFeed_Post/ok
=== RUN   TestFeed_Post/postage
=== RUN   TestFeed_Post/postage/err_-_bad_batch
=== RUN   TestFeed_Post/postage/ok_-_batch_zeros
=== RUN   TestFeed_Post/postage/bad_request_-_batch_empty
--- PASS: TestFeed_Post (0.01s)
    --- PASS: TestFeed_Post/ok (0.00s)
    --- PASS: TestFeed_Post/postage (0.00s)
        --- PASS: TestFeed_Post/postage/err_-_bad_batch (0.00s)
        --- PASS: TestFeed_Post/postage/ok_-_batch_zeros (0.00s)
        --- PASS: TestFeed_Post/postage/bad_request_-_batch_empty (0.00s)
=== RUN   TestDirectUploadFeed
=== PAUSE TestDirectUploadFeed
=== RUN   TestHealth
=== PAUSE TestHealth
=== RUN   TestGetLoggers
--- PASS: TestGetLoggers (0.00s)
=== RUN   TestSetLoggerVerbosity
=== RUN   TestSetLoggerVerbosity/to=warning,exp=name
=== RUN   TestSetLoggerVerbosity/to=warning,exp=^name$
=== RUN   TestSetLoggerVerbosity/to=warning,exp=^name/\[0
=== RUN   TestSetLoggerVerbosity/to=warning,exp=^name/\[0\]\[
=== RUN   TestSetLoggerVerbosity/to=warning,exp=^name/\[0\]\[\"val\"=1\]
=== RUN   TestSetLoggerVerbosity/to=warning,exp=^name/\[0\]\[\"val\"=1\]>>824634860360
--- PASS: TestSetLoggerVerbosity (0.01s)
    --- PASS: TestSetLoggerVerbosity/to=warning,exp=name (0.00s)
    --- PASS: TestSetLoggerVerbosity/to=warning,exp=^name$ (0.00s)
    --- PASS: TestSetLoggerVerbosity/to=warning,exp=^name/\[0 (0.00s)
    --- PASS: TestSetLoggerVerbosity/to=warning,exp=^name/\[0\]\[ (0.00s)
    --- PASS: TestSetLoggerVerbosity/to=warning,exp=^name/\[0\]\[\"val\"=1\] (0.00s)
    --- PASS: TestSetLoggerVerbosity/to=warning,exp=^name/\[0\]\[\"val\"=1\]>>824634860360 (0.00s)
=== RUN   Test_loggerGetHandler_invalidInputs
=== PAUSE Test_loggerGetHandler_invalidInputs
=== RUN   Test_loggerSetVerbosityHandler_invalidInputs
=== PAUSE Test_loggerSetVerbosityHandler_invalidInputs
=== RUN   TestToFileSizeBucket
=== PAUSE TestToFileSizeBucket
=== RUN   TestBeeNodeMode_String
=== PAUSE TestBeeNodeMode_String
=== RUN   TestAddresses
=== PAUSE TestAddresses
=== RUN   TestAddresses_error
=== PAUSE TestAddresses_error
=== RUN   TestConnect
=== PAUSE TestConnect
=== RUN   TestDisconnect
=== PAUSE TestDisconnect
=== RUN   TestPeer
=== PAUSE TestPeer
=== RUN   TestBlocklistedPeers
=== PAUSE TestBlocklistedPeers
=== RUN   TestBlocklistedPeersErr
=== PAUSE TestBlocklistedPeersErr
=== RUN   Test_peerConnectHandler_invalidInputs
=== PAUSE Test_peerConnectHandler_invalidInputs
=== RUN   Test_peerDisconnectHandler_invalidInputs
=== PAUSE Test_peerDisconnectHandler_invalidInputs
=== RUN   TestPinHandlers
=== RUN   TestPinHandlers/bytes
=== RUN   TestPinHandlers/bzz
=== RUN   TestPinHandlers/chunk
--- PASS: TestPinHandlers (0.02s)
    --- PASS: TestPinHandlers/bytes (0.00s)
    --- PASS: TestPinHandlers/bzz (0.01s)
    --- PASS: TestPinHandlers/chunk (0.00s)
=== RUN   Test_pinHandlers_invalidInputs
=== PAUSE Test_pinHandlers_invalidInputs
=== RUN   TestPingpong
=== PAUSE TestPingpong
=== RUN   Test_pingpongHandler_invalidInputs
=== PAUSE Test_pingpongHandler_invalidInputs
=== RUN   TestPostageCreateStamp
=== PAUSE TestPostageCreateStamp
=== RUN   TestPostageGetStamps
=== PAUSE TestPostageGetStamps
=== RUN   TestGetAllBatches
=== PAUSE TestGetAllBatches
=== RUN   TestPostageGetStamp
=== PAUSE TestPostageGetStamp
=== RUN   TestPostageGetBuckets
=== PAUSE TestPostageGetBuckets
=== RUN   TestReserveState
=== PAUSE TestReserveState
=== RUN   TestChainState
=== PAUSE TestChainState
=== RUN   TestPostageTopUpStamp
=== PAUSE TestPostageTopUpStamp
=== RUN   TestPostageDiluteStamp
=== PAUSE TestPostageDiluteStamp
=== RUN   TestPostageAccessHandler
=== PAUSE TestPostageAccessHandler
=== RUN   Test_postageCreateHandler_invalidInputs
=== PAUSE Test_postageCreateHandler_invalidInputs
=== RUN   Test_postageGetStampsHandler_invalidInputs
=== PAUSE Test_postageGetStampsHandler_invalidInputs
=== RUN   Test_postageGetStampBucketsHandler_invalidInputs
=== PAUSE Test_postageGetStampBucketsHandler_invalidInputs
=== RUN   Test_postageGetStampHandler_invalidInputs
=== PAUSE Test_postageGetStampHandler_invalidInputs
=== RUN   Test_postageTopUpHandler_invalidInputs
=== PAUSE Test_postageTopUpHandler_invalidInputs
=== RUN   Test_postageDiluteHandler_invalidInputs
=== PAUSE Test_postageDiluteHandler_invalidInputs
=== RUN   TestPssWebsocketSingleHandler
=== PAUSE TestPssWebsocketSingleHandler
=== RUN   TestPssWebsocketSingleHandlerDeregister
=== PAUSE TestPssWebsocketSingleHandlerDeregister
=== RUN   TestPssWebsocketMultiHandler
=== PAUSE TestPssWebsocketMultiHandler
=== RUN   TestPssSend
=== RUN   TestPssSend/err_-_bad_batch
=== RUN   TestPssSend/ok_batch
=== RUN   TestPssSend/bad_request_-_batch_empty
=== RUN   TestPssSend/ok
=== RUN   TestPssSend/without_recipient
--- PASS: TestPssSend (0.09s)
    --- PASS: TestPssSend/err_-_bad_batch (0.00s)
    --- PASS: TestPssSend/ok_batch (0.00s)
    --- PASS: TestPssSend/bad_request_-_batch_empty (0.00s)
    --- PASS: TestPssSend/ok (0.04s)
    --- PASS: TestPssSend/without_recipient (0.04s)
=== RUN   TestPssPingPong
=== PAUSE TestPssPingPong
=== RUN   Test_pssPostHandler_invalidInputs
=== PAUSE Test_pssPostHandler_invalidInputs
=== RUN   TestReadiness
=== PAUSE TestReadiness
=== RUN   TestRedistributionStatus
=== PAUSE TestRedistributionStatus
=== RUN   TestSettlements
=== PAUSE TestSettlements
=== RUN   TestSettlementsError
=== PAUSE TestSettlementsError
=== RUN   TestSettlementsPeers
=== PAUSE TestSettlementsPeers
=== RUN   TestSettlementsPeersNoSettlements
=== PAUSE TestSettlementsPeersNoSettlements
=== RUN   Test_peerSettlementsHandler_invalidInputs
=== PAUSE Test_peerSettlementsHandler_invalidInputs
=== RUN   TestSettlementsPeersError
=== PAUSE TestSettlementsPeersError
=== RUN   TestSOC
=== RUN   TestSOC/cmpty_data
=== RUN   TestSOC/signature_invalid
=== RUN   TestSOC/ok
=== RUN   TestSOC/already_exists
=== RUN   TestSOC/postage
=== RUN   TestSOC/postage/err_-_bad_batch
=== RUN   TestSOC/postage/ok_batch
=== RUN   TestSOC/postage/err_-_batch_empty
--- PASS: TestSOC (0.01s)
    --- PASS: TestSOC/cmpty_data (0.00s)
    --- PASS: TestSOC/signature_invalid (0.00s)
    --- PASS: TestSOC/ok (0.00s)
    --- PASS: TestSOC/already_exists (0.00s)
    --- PASS: TestSOC/postage (0.00s)
        --- PASS: TestSOC/postage/err_-_bad_batch (0.00s)
        --- PASS: TestSOC/postage/ok_batch (0.00s)
        --- PASS: TestSOC/postage/err_-_batch_empty (0.00s)
=== RUN   TestDepositStake
=== PAUSE TestDepositStake
=== RUN   TestGetStake
=== PAUSE TestGetStake
=== RUN   Test_stakingDepositHandler_invalidInputs
=== PAUSE Test_stakingDepositHandler_invalidInputs
=== RUN   TestWithdrawAllStake
=== PAUSE TestWithdrawAllStake
=== RUN   TestStewardship
=== RUN   TestStewardship/re-upload
=== RUN   TestStewardship/is-retrievable
--- PASS: TestStewardship (0.00s)
    --- PASS: TestStewardship/re-upload (0.00s)
    --- PASS: TestStewardship/is-retrievable (0.00s)
=== RUN   Test_stewardshipHandlers_invalidInputs
=== PAUSE Test_stewardshipHandlers_invalidInputs
=== RUN   TestSubdomains
=== PAUSE TestSubdomains
=== RUN   TestDebugTags
=== PAUSE TestDebugTags
=== RUN   Test_getDebugTagHandler_invalidInputs
=== PAUSE Test_getDebugTagHandler_invalidInputs
=== RUN   TestTags
=== RUN   TestTags/list_tags_zero
=== RUN   TestTags/create_tag
=== RUN   TestTags/create_tag_with_invalid_id
=== RUN   TestTags/get_non-existent_tag
=== RUN   TestTags/create_tag_upload_chunk
=== RUN   TestTags/create_tag_upload_chunk_stream
=== RUN   TestTags/list_tags
=== RUN   TestTags/delete_non-existent_tag
=== RUN   TestTags/delete_tag
=== RUN   TestTags/done_split_non-existent_tag
=== RUN   TestTags/done_split
=== RUN   TestTags/file_tags
=== RUN   TestTags/dir_tags
=== RUN   TestTags/bytes_tags
--- PASS: TestTags (0.03s)
    --- PASS: TestTags/list_tags_zero (0.00s)
    --- PASS: TestTags/create_tag (0.00s)
    --- PASS: TestTags/create_tag_with_invalid_id (0.00s)
    --- PASS: TestTags/get_non-existent_tag (0.00s)
    --- PASS: TestTags/create_tag_upload_chunk (0.01s)
    --- PASS: TestTags/create_tag_upload_chunk_stream (0.00s)
    --- PASS: TestTags/list_tags (0.00s)
    --- PASS: TestTags/delete_non-existent_tag (0.00s)
    --- PASS: TestTags/delete_tag (0.00s)
    --- PASS: TestTags/done_split_non-existent_tag (0.00s)
    --- PASS: TestTags/done_split (0.00s)
    --- PASS: TestTags/file_tags (0.00s)
    --- PASS: TestTags/dir_tags (0.00s)
    --- PASS: TestTags/bytes_tags (0.00s)
=== RUN   Test_tagHandlers_invalidInputs
=== PAUSE Test_tagHandlers_invalidInputs
=== RUN   TestTopologyOK
=== PAUSE TestTopologyOK
=== RUN   TestTransactionStoredTransaction
=== PAUSE TestTransactionStoredTransaction
=== RUN   TestTransactionList
=== PAUSE TestTransactionList
=== RUN   TestTransactionListError
=== PAUSE TestTransactionListError
=== RUN   TestTransactionResend
=== PAUSE TestTransactionResend
=== RUN   TestMapStructure
=== PAUSE TestMapStructure
=== RUN   TestMapStructure_InputOutputSanityCheck
=== PAUSE TestMapStructure_InputOutputSanityCheck
=== RUN   TestWallet
=== PAUSE TestWallet
=== RUN   TestGetWelcomeMessage
=== PAUSE TestGetWelcomeMessage
=== RUN   TestSetWelcomeMessage
=== RUN   TestSetWelcomeMessage/OK
=== RUN   TestSetWelcomeMessage/OK_-_welcome_message_empty
=== RUN   TestSetWelcomeMessage/error_-_request_entity_too_large
--- PASS: TestSetWelcomeMessage (0.00s)
    --- PASS: TestSetWelcomeMessage/OK (0.00s)
    --- PASS: TestSetWelcomeMessage/OK_-_welcome_message_empty (0.00s)
    --- PASS: TestSetWelcomeMessage/error_-_request_entity_too_large (0.00s)
=== RUN   TestSetWelcomeMessageInternalServerError
=== PAUSE TestSetWelcomeMessageInternalServerError
=== CONT  TestAccountingInfo
=== CONT  TestDisconnect
=== CONT  Test_pssPostHandler_invalidInputs
=== CONT  TestChainState
=== RUN   TestChainState/ok
=== PAUSE TestChainState/ok
=== CONT  TestChequebookBalanceError
=== CONT  TestConnect
=== RUN   Test_pssPostHandler_invalidInputs/targets_-_odd_length_hex_string
=== PAUSE Test_pssPostHandler_invalidInputs/targets_-_odd_length_hex_string
=== RUN   Test_pssPostHandler_invalidInputs/targets_-_odd_length_hex_string#01
=== PAUSE Test_pssPostHandler_invalidInputs/targets_-_odd_length_hex_string#01
=== CONT  TestToFileSizeBucket
--- PASS: TestToFileSizeBucket (0.00s)
=== CONT  Test_loggerSetVerbosityHandler_invalidInputs
=== CONT  TestAddresses_error
=== CONT  TestAddresses
=== CONT  TestBeeNodeMode_String
=== RUN   TestDisconnect/ok
--- PASS: TestBeeNodeMode_String (0.00s)
=== CONT  Test_loggerGetHandler_invalidInputs
=== PAUSE TestDisconnect/ok
=== RUN   TestDisconnect/unknown
=== PAUSE TestDisconnect/unknown
=== RUN   TestDisconnect/error
=== PAUSE TestDisconnect/error
=== CONT  TestHealth
=== RUN   TestHealth/probe_not_set
=== PAUSE TestHealth/probe_not_set
=== RUN   TestHealth/health_probe_status_change
=== PAUSE TestHealth/health_probe_status_change
=== CONT  TestDirectUploadFeed
=== RUN   Test_loggerSetVerbosityHandler_invalidInputs/exp_-_illegal_base64
=== PAUSE Test_loggerSetVerbosityHandler_invalidInputs/exp_-_illegal_base64
=== RUN   Test_loggerSetVerbosityHandler_invalidInputs/exp_-_invalid_regex
=== PAUSE Test_loggerSetVerbosityHandler_invalidInputs/exp_-_invalid_regex
=== RUN   Test_loggerSetVerbosityHandler_invalidInputs/verbosity_-_invalid_value
=== PAUSE Test_loggerSetVerbosityHandler_invalidInputs/verbosity_-_invalid_value
=== CONT  TestFeed_Get
=== RUN   TestFeed_Get/with_at
=== PAUSE TestFeed_Get/with_at
=== RUN   TestFeed_Get/latest
=== PAUSE TestFeed_Get/latest
=== CONT  TestEmtpyDir
=== RUN   TestAddresses/ok
=== PAUSE TestAddresses/ok
=== RUN   Test_loggerGetHandler_invalidInputs/exp_-_illegal_base64
=== PAUSE Test_loggerGetHandler_invalidInputs/exp_-_illegal_base64
=== RUN   Test_loggerGetHandler_invalidInputs/exp_-_invalid_regex
=== PAUSE Test_loggerGetHandler_invalidInputs/exp_-_invalid_regex
=== CONT  TestDBIndices
=== RUN   TestDBIndices/success
=== PAUSE TestDBIndices/success
=== RUN   TestDBIndices/internal_error_returned
=== PAUSE TestDBIndices/internal_error_returned
=== RUN   TestDBIndices/not_implemented_error_returned
=== PAUSE TestDBIndices/not_implemented_error_returned
=== CONT  TestSubdomains
=== RUN   TestSubdomains/nested_files_with_extension
=== PAUSE TestSubdomains/nested_files_with_extension
=== RUN   TestSubdomains/explicit_index_and_error_filename
=== PAUSE TestSubdomains/explicit_index_and_error_filename
=== CONT  TestCorsStatus
=== RUN   TestCorsStatus/tags
=== PAUSE TestCorsStatus/tags
=== RUN   TestCorsStatus/bzz
=== PAUSE TestCorsStatus/bzz
=== RUN   TestCorsStatus/chunks
=== PAUSE TestCorsStatus/chunks
=== RUN   TestCorsStatus/chunks/0101011
=== PAUSE TestCorsStatus/chunks/0101011
=== RUN   TestCorsStatus/bytes
=== PAUSE TestCorsStatus/bytes
=== RUN   TestCorsStatus/bytes/0121012
=== PAUSE TestCorsStatus/bytes/0121012
=== CONT  TestSetWelcomeMessageInternalServerError
=== RUN   TestSetWelcomeMessageInternalServerError/internal_server_error_-_error_on_store
=== PAUSE TestSetWelcomeMessageInternalServerError/internal_server_error_-_error_on_store
=== CONT  TestCors
=== RUN   TestCors/tags
=== PAUSE TestCors/tags
=== RUN   TestCors/bzz
=== PAUSE TestCors/bzz
=== RUN   TestCors/bzz/0101011
=== PAUSE TestCors/bzz/0101011
=== RUN   TestCors/chunks
=== PAUSE TestCors/chunks
=== RUN   TestCors/chunks/123213
=== PAUSE TestCors/chunks/123213
=== RUN   TestCors/bytes
=== PAUSE TestCors/bytes
=== RUN   TestCors/bytes/0121012
=== PAUSE TestCors/bytes/0121012
=== CONT  TestGetWelcomeMessage
=== RUN   TestConnect/ok
=== PAUSE TestConnect/ok
=== RUN   TestConnect/error
=== PAUSE TestConnect/error
=== RUN   TestConnect/error_-_add_peer
=== PAUSE TestConnect/error_-_add_peer
=== CONT  TestCORSHeaders
=== RUN   TestCORSHeaders/none
=== PAUSE TestCORSHeaders/none
=== RUN   TestCORSHeaders/no_origin
=== PAUSE TestCORSHeaders/no_origin
=== RUN   TestCORSHeaders/single_explicit
=== PAUSE TestCORSHeaders/single_explicit
=== RUN   TestCORSHeaders/single_explicit_blocked
=== PAUSE TestCORSHeaders/single_explicit_blocked
=== RUN   TestCORSHeaders/multiple_explicit
=== PAUSE TestCORSHeaders/multiple_explicit
=== RUN   TestCORSHeaders/multiple_explicit_blocked
=== PAUSE TestCORSHeaders/multiple_explicit_blocked
=== RUN   TestCORSHeaders/wildcard
=== PAUSE TestCORSHeaders/wildcard
=== RUN   TestCORSHeaders/wildcard#01
=== PAUSE TestCORSHeaders/wildcard#01
=== RUN   TestCORSHeaders/with_origin_only
=== PAUSE TestCORSHeaders/with_origin_only
=== RUN   TestCORSHeaders/with_origin_only_not_nil
=== PAUSE TestCORSHeaders/with_origin_only_not_nil
=== CONT  TestDirectChunkUpload
--- PASS: TestEmtpyDir (0.01s)
--- PASS: TestAccountingInfo (0.01s)
=== CONT  TestInvalidChunkParams
=== CONT  TestMapStructure_InputOutputSanityCheck
=== RUN   TestMapStructure_InputOutputSanityCheck/input_is_nil
--- PASS: TestDirectUploadFeed (0.01s)
=== CONT  TestWallet
=== RUN   TestWallet/Okay
=== PAUSE TestWallet/Okay
=== RUN   TestWallet/500_-_erc20_error
=== PAUSE TestWallet/500_-_erc20_error
=== RUN   TestWallet/500_-_chain_backend_error
=== PAUSE TestWallet/500_-_chain_backend_error
=== CONT  Test_chunkHandlers_invalidInputs
=== PAUSE TestMapStructure_InputOutputSanityCheck/input_is_nil
=== RUN   TestMapStructure_InputOutputSanityCheck/input_is_not_a_map
--- PASS: TestChequebookBalanceError (0.01s)
=== PAUSE TestMapStructure_InputOutputSanityCheck/input_is_not_a_map
=== RUN   TestMapStructure_InputOutputSanityCheck/output_is_not_a_pointer
=== PAUSE TestMapStructure_InputOutputSanityCheck/output_is_not_a_pointer
=== RUN   TestMapStructure_InputOutputSanityCheck/output_is_nil
=== PAUSE TestMapStructure_InputOutputSanityCheck/output_is_nil
=== RUN   TestMapStructure_InputOutputSanityCheck/output_is_a_nil_pointer
=== PAUSE TestMapStructure_InputOutputSanityCheck/output_is_a_nil_pointer
--- PASS: TestDirectChunkUpload (0.01s)
--- PASS: TestGetWelcomeMessage (0.01s)
=== RUN   TestMapStructure_InputOutputSanityCheck/output_is_not_a_struct
=== PAUSE TestMapStructure_InputOutputSanityCheck/output_is_not_a_struct
=== RUN   TestInvalidChunkParams/batch_unusable
=== CONT  TestChequebookCashoutStatus
=== PAUSE TestInvalidChunkParams/batch_unusable
=== RUN   TestInvalidChunkParams/batch_exists
=== PAUSE TestInvalidChunkParams/batch_exists
=== RUN   TestInvalidChunkParams/batch_not_found
=== PAUSE TestInvalidChunkParams/batch_not_found
=== RUN   TestChequebookCashoutStatus/with_result
=== PAUSE TestChequebookCashoutStatus/with_result
=== RUN   TestChequebookCashoutStatus/without_result
=== PAUSE TestChequebookCashoutStatus/without_result
=== CONT  TestMapStructure
=== RUN   TestChequebookCashoutStatus/without_last
=== PAUSE TestChequebookCashoutStatus/without_last
=== CONT  TestTransactionResend
=== RUN   TestTransactionResend/ok
=== RUN   TestMapStructure/bool_zero_value
=== PAUSE TestTransactionResend/ok
=== CONT  TestTransactionListError
--- PASS: TestAddresses_error (0.01s)
=== PAUSE TestMapStructure/bool_zero_value
=== RUN   TestMapStructure/bool_false
=== PAUSE TestMapStructure/bool_false
=== RUN   TestMapStructure/bool_true
=== CONT  TestChequebookCashout_CustomGas
=== CONT  TestTransactionList
=== RUN   TestTransactionResend/unknown_transaction
=== PAUSE TestTransactionResend/unknown_transaction
=== RUN   TestTransactionResend/already_imported
=== PAUSE TestTransactionResend/already_imported
=== RUN   TestTransactionResend/other_error
=== PAUSE TestTransactionResend/other_error
=== RUN   TestTransactionListError/pending_transactions_error
=== PAUSE TestTransactionListError/pending_transactions_error
=== RUN   TestTransactionListError/pending_transactions_error#01
=== PAUSE TestTransactionListError/pending_transactions_error#01
=== CONT  TestTransactionStoredTransaction
=== RUN   TestTransactionStoredTransaction/found
=== PAUSE TestTransactionStoredTransaction/found
=== RUN   TestTransactionStoredTransaction/not_found
=== PAUSE TestTransactionStoredTransaction/not_found
=== RUN   TestTransactionStoredTransaction/other_errors
=== PAUSE TestTransactionStoredTransaction/other_errors
=== CONT  TestChequebookLastChequesPeer
=== RUN   Test_chunkHandlers_invalidInputs/GET_address_-_odd_hex_string
=== PAUSE Test_chunkHandlers_invalidInputs/GET_address_-_odd_hex_string
=== RUN   Test_chunkHandlers_invalidInputs/GET_address_-_invalid_hex_character
=== PAUSE Test_chunkHandlers_invalidInputs/GET_address_-_invalid_hex_character
=== RUN   Test_chunkHandlers_invalidInputs/DELETE_address_-_odd_hex_string
=== PAUSE Test_chunkHandlers_invalidInputs/DELETE_address_-_odd_hex_string
=== RUN   Test_chunkHandlers_invalidInputs/DELETE_address_-_invalid_hex_character
=== PAUSE Test_chunkHandlers_invalidInputs/DELETE_address_-_invalid_hex_character
=== CONT  TestChequebookLastCheques
=== CONT  TestChequebookCashout
=== PAUSE TestMapStructure/bool_true
=== RUN   TestMapStructure/bool_syntax_error
=== PAUSE TestMapStructure/bool_syntax_error
=== RUN   TestMapStructure/uint_zero_value
=== PAUSE TestMapStructure/uint_zero_value
=== RUN   TestMapStructure/uint_in_range_value
=== PAUSE TestMapStructure/uint_in_range_value
=== RUN   TestMapStructure/uint_max_value
=== PAUSE TestMapStructure/uint_max_value
=== RUN   TestMapStructure/uint_out_of_range_value
=== PAUSE TestMapStructure/uint_out_of_range_value
=== RUN   TestMapStructure/uint_syntax_error
=== PAUSE TestMapStructure/uint_syntax_error
=== RUN   TestMapStructure/uint8_zero_value
=== PAUSE TestMapStructure/uint8_zero_value
=== RUN   TestMapStructure/uint8_in_range_value
=== PAUSE TestMapStructure/uint8_in_range_value
=== RUN   TestMapStructure/uint8_max_value
=== PAUSE TestMapStructure/uint8_max_value
=== RUN   TestMapStructure/uint8_out_of_range_value
=== PAUSE TestMapStructure/uint8_out_of_range_value
=== RUN   TestMapStructure/uint8_syntax_error
=== PAUSE TestMapStructure/uint8_syntax_error
=== RUN   TestMapStructure/uint16_zero_value
=== PAUSE TestMapStructure/uint16_zero_value
=== RUN   TestMapStructure/uint16_in_range_value
=== PAUSE TestMapStructure/uint16_in_range_value
=== RUN   TestMapStructure/uint16_max_value
=== PAUSE TestMapStructure/uint16_max_value
=== RUN   TestMapStructure/uint16_out_of_range_value
=== PAUSE TestMapStructure/uint16_out_of_range_value
=== RUN   TestMapStructure/uint16_syntax_error
=== PAUSE TestMapStructure/uint16_syntax_error
=== RUN   TestMapStructure/uint32_zero_value
=== PAUSE TestMapStructure/uint32_zero_value
=== RUN   TestMapStructure/uint32_in_range_value
=== PAUSE TestMapStructure/uint32_in_range_value
=== RUN   TestMapStructure/uint32_max_value
=== PAUSE TestMapStructure/uint32_max_value
=== RUN   TestMapStructure/uint32_out_of_range_value
=== PAUSE TestMapStructure/uint32_out_of_range_value
=== RUN   TestMapStructure/uint32_syntax_error
=== PAUSE TestMapStructure/uint32_syntax_error
=== RUN   TestMapStructure/uint64_zero_value
=== PAUSE TestMapStructure/uint64_zero_value
=== RUN   TestMapStructure/uint64_in_range_value
=== PAUSE TestMapStructure/uint64_in_range_value
=== RUN   TestMapStructure/uint64_max_value
=== PAUSE TestMapStructure/uint64_max_value
--- PASS: TestChequebookCashout_CustomGas (0.00s)
=== CONT  Test_tagHandlers_invalidInputs
=== CONT  Test_chequebookLastPeerHandler_invalidInputs
=== RUN   TestMapStructure/uint64_out_of_range_value
=== CONT  TestChequebookDeposit
=== PAUSE TestMapStructure/uint64_out_of_range_value
=== RUN   TestChequebookDeposit/ok
=== PAUSE TestChequebookDeposit/ok
=== RUN   TestMapStructure/uint64_syntax_error
=== RUN   TestChequebookDeposit/custom_gas
=== PAUSE TestChequebookDeposit/custom_gas
=== PAUSE TestMapStructure/uint64_syntax_error
=== CONT  TestTopologyOK
=== RUN   TestMapStructure/int_zero_value
=== PAUSE TestMapStructure/int_zero_value
=== RUN   TestMapStructure/int_in_range_value
=== PAUSE TestMapStructure/int_in_range_value
=== RUN   TestMapStructure/int_min_value
=== PAUSE TestMapStructure/int_min_value
=== RUN   TestMapStructure/int_max_value
=== PAUSE TestMapStructure/int_max_value
=== RUN   TestMapStructure/int_min_out_of_range_value
=== PAUSE TestMapStructure/int_min_out_of_range_value
=== RUN   TestMapStructure/int_max_out_of_range_value
=== PAUSE TestMapStructure/int_max_out_of_range_value
=== RUN   TestMapStructure/int_syntax_error
=== PAUSE TestMapStructure/int_syntax_error
=== RUN   TestMapStructure/int8_zero_value
=== PAUSE TestMapStructure/int8_zero_value
=== RUN   TestMapStructure/int8_in_range_value
=== PAUSE TestMapStructure/int8_in_range_value
=== RUN   TestMapStructure/int8_min_value
=== PAUSE TestMapStructure/int8_min_value
=== RUN   TestMapStructure/int8_max_value
=== PAUSE TestMapStructure/int8_max_value
=== RUN   TestMapStructure/int8_min_out_of_range_value
=== PAUSE TestMapStructure/int8_min_out_of_range_value
=== RUN   TestMapStructure/int8_max_out_of_range_value
=== PAUSE TestMapStructure/int8_max_out_of_range_value
=== RUN   TestMapStructure/int8_syntax_error
=== PAUSE TestMapStructure/int8_syntax_error
=== RUN   TestMapStructure/int16_zero_value
=== PAUSE TestMapStructure/int16_zero_value
=== RUN   TestMapStructure/int16_in_range_value
=== PAUSE TestMapStructure/int16_in_range_value
=== RUN   TestMapStructure/int16_min_value
=== PAUSE TestMapStructure/int16_min_value
=== RUN   TestMapStructure/int16_max_value
=== PAUSE TestMapStructure/int16_max_value
=== RUN   TestMapStructure/int16_min_out_of_range_value
=== PAUSE TestMapStructure/int16_min_out_of_range_value
=== RUN   TestMapStructure/int16_max_out_of_range_value
=== PAUSE TestMapStructure/int16_max_out_of_range_value
=== RUN   TestMapStructure/int16_syntax_error
=== PAUSE TestMapStructure/int16_syntax_error
=== RUN   TestMapStructure/int32_zero_value
=== PAUSE TestMapStructure/int32_zero_value
=== RUN   TestMapStructure/int32_in_range_value
=== PAUSE TestMapStructure/int32_in_range_value
=== RUN   TestMapStructure/int32_min_value
=== PAUSE TestMapStructure/int32_min_value
=== RUN   TestMapStructure/int32_max_value
=== PAUSE TestMapStructure/int32_max_value
=== RUN   TestMapStructure/int32_min_out_of_range_value
=== PAUSE TestMapStructure/int32_min_out_of_range_value
=== RUN   TestMapStructure/int32_max_out_of_range_value
=== PAUSE TestMapStructure/int32_max_out_of_range_value
=== RUN   TestMapStructure/int32_syntax_error
=== PAUSE TestMapStructure/int32_syntax_error
=== RUN   TestMapStructure/int64_zero_value
=== PAUSE TestMapStructure/int64_zero_value
=== RUN   TestMapStructure/int64_in_range_value
=== PAUSE TestMapStructure/int64_in_range_value
=== RUN   TestMapStructure/int64_min_value
=== PAUSE TestMapStructure/int64_min_value
=== RUN   TestMapStructure/int64_max_value
=== PAUSE TestMapStructure/int64_max_value
=== RUN   TestMapStructure/int64_min_out_of_range_value
=== PAUSE TestMapStructure/int64_min_out_of_range_value
=== RUN   TestMapStructure/int64_max_out_of_range_value
=== PAUSE TestMapStructure/int64_max_out_of_range_value
=== RUN   TestMapStructure/int64_syntax_error
=== PAUSE TestMapStructure/int64_syntax_error
=== RUN   TestMapStructure/float32_zero_value
=== PAUSE TestMapStructure/float32_zero_value
=== RUN   TestMapStructure/float32_in_range_value
=== PAUSE TestMapStructure/float32_in_range_value
=== RUN   TestMapStructure/float32_min_value
=== PAUSE TestMapStructure/float32_min_value
=== RUN   TestMapStructure/float32_max_value
=== PAUSE TestMapStructure/float32_max_value
=== RUN   TestMapStructure/float32_max_out_of_range_value
=== PAUSE TestMapStructure/float32_max_out_of_range_value
=== RUN   TestMapStructure/float32_syntax_error
=== PAUSE TestMapStructure/float32_syntax_error
=== RUN   TestMapStructure/float64_zero_value
=== PAUSE TestMapStructure/float64_zero_value
=== RUN   TestMapStructure/float64_in_range_value
=== PAUSE TestMapStructure/float64_in_range_value
=== RUN   TestMapStructure/float64_min_value
=== PAUSE TestMapStructure/float64_min_value
=== RUN   TestMapStructure/float64_max_value
=== PAUSE TestMapStructure/float64_max_value
=== RUN   TestMapStructure/float64_max_out_of_range_value
=== PAUSE TestMapStructure/float64_max_out_of_range_value
=== RUN   TestMapStructure/float64_syntax_error
=== PAUSE TestMapStructure/float64_syntax_error
=== RUN   TestMapStructure/byte_slice_zero_value
=== PAUSE TestMapStructure/byte_slice_zero_value
=== RUN   TestMapStructure/byte_slice_single_byte
=== PAUSE TestMapStructure/byte_slice_single_byte
=== RUN   TestMapStructure/byte_slice_multiple_bytes
=== PAUSE TestMapStructure/byte_slice_multiple_bytes
=== RUN   TestMapStructure/byte_slice_invalid_byte
=== PAUSE TestMapStructure/byte_slice_invalid_byte
=== RUN   TestMapStructure/byte_slice_invalid_length
=== PAUSE TestMapStructure/byte_slice_invalid_length
=== RUN   TestMapStructure/string_zero_value
=== PAUSE TestMapStructure/string_zero_value
=== RUN   TestMapStructure/string_single_character
=== PAUSE TestMapStructure/string_single_character
=== CONT  TestChequebookWithdraw
=== RUN   TestMapStructure/string_multiple_characters
=== PAUSE TestMapStructure/string_multiple_characters
=== RUN   TestChequebookWithdraw/ok
=== RUN   TestMapStructure/string_with_multiple_values
=== PAUSE TestMapStructure/string_with_multiple_values
=== PAUSE TestChequebookWithdraw/ok
=== RUN   TestMapStructure/string_without_matching_field
=== RUN   TestChequebookWithdraw/custom_gas
=== PAUSE TestMapStructure/string_without_matching_field
=== PAUSE TestChequebookWithdraw/custom_gas
=== RUN   TestMapStructure/string_with_omitempty_field
=== CONT  TestDebugTags
=== PAUSE TestMapStructure/string_with_omitempty_field
=== RUN   TestMapStructure/bit.Int_value
=== PAUSE TestMapStructure/bit.Int_value
=== RUN   TestMapStructure/common.Hash_value
=== PAUSE TestMapStructure/common.Hash_value
=== RUN   TestMapStructure/swarm.Address_value
=== PAUSE TestMapStructure/swarm.Address_value
--- PASS: TestTransactionList (0.00s)
--- PASS: TestChequebookLastChequesPeer (0.00s)
=== CONT  TestChequebookAddress
--- PASS: TestChequebookCashout (0.00s)
=== CONT  TestChequebookAvailableBalanceError
--- PASS: TestChequebookLastCheques (0.00s)
=== CONT  TestConsumedPeers
=== RUN   Test_tagHandlers_invalidInputs/GET_id_-_invalid_value
=== PAUSE Test_tagHandlers_invalidInputs/GET_id_-_invalid_value
=== RUN   Test_tagHandlers_invalidInputs/DELETE_id_-_invalid_value
=== PAUSE Test_tagHandlers_invalidInputs/DELETE_id_-_invalid_value
=== RUN   Test_tagHandlers_invalidInputs/PATCH_id_-_invalid_value
=== PAUSE Test_tagHandlers_invalidInputs/PATCH_id_-_invalid_value
=== CONT  Test_postageGetStampHandler_invalidInputs
=== CONT  Test_getDebugTagHandler_invalidInputs
=== RUN   Test_chequebookLastPeerHandler_invalidInputs/peer_-_odd_hex_string
=== PAUSE Test_chequebookLastPeerHandler_invalidInputs/peer_-_odd_hex_string
=== RUN   Test_chequebookLastPeerHandler_invalidInputs/peer_-_invalid_hex_character
=== PAUSE Test_chequebookLastPeerHandler_invalidInputs/peer_-_invalid_hex_character
=== CONT  TestChequebookBalance
--- PASS: TestTopologyOK (0.00s)
=== CONT  TestPssPingPong
--- PASS: TestChequebookAvailableBalanceError (0.00s)
=== CONT  TestDirectUploadBzz
=== RUN   TestDebugTags/all
=== PAUSE TestDebugTags/all
--- PASS: TestChequebookAddress (0.00s)
=== CONT  TestPssWebsocketMultiHandler
=== RUN   Test_getDebugTagHandler_invalidInputs/id_-_invalid_value
=== PAUSE Test_getDebugTagHandler_invalidInputs/id_-_invalid_value
=== CONT  TestInvalidBzzParams
=== RUN   TestInvalidBzzParams/batch_unusable
=== PAUSE TestInvalidBzzParams/batch_unusable
=== RUN   TestInvalidBzzParams/batch_exists
=== PAUSE TestInvalidBzzParams/batch_exists
=== RUN   TestInvalidBzzParams/batch_not_found
=== PAUSE TestInvalidBzzParams/batch_not_found
=== RUN   TestInvalidBzzParams/upload,_invalid_tag
=== PAUSE TestInvalidBzzParams/upload,_invalid_tag
=== RUN   TestInvalidBzzParams/upload,_tag_not_found
=== PAUSE TestInvalidBzzParams/upload,_tag_not_found
=== RUN   TestInvalidBzzParams/address_not_found
=== PAUSE TestInvalidBzzParams/address_not_found
=== RUN   Test_postageGetStampHandler_invalidInputs/batch_id_-_odd_hex_string
=== PAUSE Test_postageGetStampHandler_invalidInputs/batch_id_-_odd_hex_string
=== RUN   Test_postageGetStampHandler_invalidInputs/batch_id_-_invalid_hex_character
=== PAUSE Test_postageGetStampHandler_invalidInputs/batch_id_-_invalid_hex_character
=== RUN   Test_postageGetStampHandler_invalidInputs/batch_id_-_invalid_length
=== PAUSE Test_postageGetStampHandler_invalidInputs/batch_id_-_invalid_length
=== CONT  Test_bzzDownloadHandler_invalidInputs
--- PASS: TestConsumedPeers (0.00s)
=== CONT  TestPssWebsocketSingleHandler
--- PASS: TestChequebookBalance (0.00s)
=== CONT  TestBzzFilesRangeRequests
=== RUN   TestBzzFilesRangeRequests/bytes
=== PAUSE TestBzzFilesRangeRequests/bytes
=== RUN   TestBzzFilesRangeRequests/file
=== PAUSE TestBzzFilesRangeRequests/file
=== RUN   TestBzzFilesRangeRequests/dir
=== PAUSE TestBzzFilesRangeRequests/dir
=== CONT  TestChainState/ok
=== CONT  TestPssWebsocketSingleHandlerDeregister
=== RUN   Test_bzzDownloadHandler_invalidInputs/address_-_odd_hex_string
=== PAUSE Test_bzzDownloadHandler_invalidInputs/address_-_odd_hex_string
=== RUN   Test_bzzDownloadHandler_invalidInputs/address_-_invalid_hex_character
=== PAUSE Test_bzzDownloadHandler_invalidInputs/address_-_invalid_hex_character
=== CONT  TestFeedIndirection
=== CONT  Test_postageDiluteHandler_invalidInputs
=== RUN   Test_postageDiluteHandler_invalidInputs/batch_id_-_odd_hex_string
--- PASS: TestDirectUploadBzz (0.01s)
=== CONT  TestDirectUploadBytes
--- PASS: TestPssWebsocketMultiHandler (0.01s)
=== RUN   Test_postageDiluteHandler_invalidInputs/batch_id_-_invalid_hex_character
=== CONT  Test_postageGetStampBucketsHandler_invalidInputs
=== RUN   Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_odd_hex_string
=== PAUSE Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_odd_hex_string
=== RUN   Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_invalid_hex_character
=== PAUSE Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_invalid_hex_character
=== RUN   Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_invalid_length
=== PAUSE Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_invalid_length
--- PASS: TestChainState (0.00s)
    --- PASS: TestChainState/ok (0.01s)
=== CONT  Test_postageTopUpHandler_invalidInputs
=== RUN   Test_postageTopUpHandler_invalidInputs/batch_id_-_odd_hex_string
=== CONT  Test_bytesGetHandler_invalidInputs
=== RUN   Test_bytesGetHandler_invalidInputs/address_-_odd_hex_string
=== PAUSE Test_bytesGetHandler_invalidInputs/address_-_odd_hex_string
=== RUN   Test_bytesGetHandler_invalidInputs/address_-_invalid_hex_character
=== PAUSE Test_bytesGetHandler_invalidInputs/address_-_invalid_hex_character
=== CONT  Test_postageGetStampsHandler_invalidInputs
=== RUN   Test_postageDiluteHandler_invalidInputs/batch_id_-_invalid_length
--- PASS: TestPssWebsocketSingleHandlerDeregister (0.01s)
=== CONT  Test_pingpongHandler_invalidInputs
=== RUN   Test_postageGetStampsHandler_invalidInputs/all_-_invalid_value
=== PAUSE Test_postageGetStampsHandler_invalidInputs/all_-_invalid_value
=== CONT  Test_postageCreateHandler_invalidInputs
=== RUN   Test_pingpongHandler_invalidInputs/address_-_odd_hex_string
=== PAUSE Test_pingpongHandler_invalidInputs/address_-_odd_hex_string
=== RUN   Test_pingpongHandler_invalidInputs/address_-_invalid_hex_character
=== PAUSE Test_pingpongHandler_invalidInputs/address_-_invalid_hex_character
=== CONT  TestPostageAccessHandler
=== RUN   TestPostageAccessHandler/create_batch_ok-create_batch_not_ok
=== PAUSE TestPostageAccessHandler/create_batch_ok-create_batch_not_ok
=== RUN   TestPostageAccessHandler/create_batch_ok-topup_batch_not_ok
=== PAUSE TestPostageAccessHandler/create_batch_ok-topup_batch_not_ok
=== RUN   TestPostageAccessHandler/create_batch_ok-dilute_batch_not_ok
=== PAUSE TestPostageAccessHandler/create_batch_ok-dilute_batch_not_ok
=== RUN   TestPostageAccessHandler/topup_batch_ok-create_batch_not_ok
=== PAUSE TestPostageAccessHandler/topup_batch_ok-create_batch_not_ok
=== RUN   TestPostageAccessHandler/topup_batch_ok-topup_batch_not_ok
=== PAUSE TestPostageAccessHandler/topup_batch_ok-topup_batch_not_ok
=== RUN   TestPostageAccessHandler/topup_batch_ok-dilute_batch_not_ok
=== PAUSE TestPostageAccessHandler/topup_batch_ok-dilute_batch_not_ok
=== RUN   TestPostageAccessHandler/dilute_batch_ok-create_batch_not_ok
=== PAUSE TestPostageAccessHandler/dilute_batch_ok-create_batch_not_ok
=== RUN   TestPostageAccessHandler/dilute_batch_ok-topup_batch_not_ok
=== PAUSE TestPostageAccessHandler/dilute_batch_ok-topup_batch_not_ok
=== RUN   TestPostageAccessHandler/dilute_batch_ok-dilute_batch_not_ok
=== PAUSE TestPostageAccessHandler/dilute_batch_ok-dilute_batch_not_ok
=== RUN   Test_postageCreateHandler_invalidInputs/amount_-_invalid_value
=== CONT  Test_bytesUploadHandler_invalidInputs
=== RUN   Test_postageTopUpHandler_invalidInputs/batch_id_-_invalid_hex_character
=== RUN   Test_bytesUploadHandler_invalidInputs/Content-Type_-_invalid
=== PAUSE Test_bytesUploadHandler_invalidInputs/Content-Type_-_invalid
=== CONT  TestPostageDiluteStamp
=== RUN   TestPostageDiluteStamp/ok
=== PAUSE TestPostageDiluteStamp/ok
=== RUN   TestPostageDiluteStamp/with-custom-gas
=== RUN   Test_postageDiluteHandler_invalidInputs/depth_-_invalid_syntax
=== PAUSE TestPostageDiluteStamp/with-custom-gas
=== RUN   TestPostageDiluteStamp/with-error
=== PAUSE TestPostageDiluteStamp/with-error
=== RUN   TestPostageDiluteStamp/with_depth_error
=== PAUSE TestPostageDiluteStamp/with_depth_error
=== RUN   TestPostageDiluteStamp/gas_limit_header
=== PAUSE TestPostageDiluteStamp/gas_limit_header
--- PASS: TestDirectUploadBytes (0.01s)
=== CONT  TestReserveState
=== RUN   TestReserveState/ok
=== PAUSE TestReserveState/ok
=== RUN   TestReserveState/empty
=== PAUSE TestReserveState/empty
=== CONT  TestPostageTopUpStamp
=== RUN   TestPostageTopUpStamp/ok
=== PAUSE TestPostageTopUpStamp/ok
=== RUN   TestPostageTopUpStamp/with-custom-gas
=== PAUSE TestPostageTopUpStamp/with-custom-gas
=== RUN   TestPostageTopUpStamp/with-error
=== CONT  TestPostageGetBuckets
=== PAUSE TestPostageTopUpStamp/with-error
=== RUN   TestPostageTopUpStamp/out-of-funds
=== PAUSE TestPostageTopUpStamp/out-of-funds
=== RUN   TestPostageTopUpStamp/gas_limit_header
=== PAUSE TestPostageTopUpStamp/gas_limit_header
=== CONT  Test_compensatedPeerBalanceHandler_invalidInputs
--- PASS: Test_postageDiluteHandler_invalidInputs (0.02s)
    --- PASS: Test_postageDiluteHandler_invalidInputs/batch_id_-_odd_hex_string (0.00s)
    --- PASS: Test_postageDiluteHandler_invalidInputs/batch_id_-_invalid_hex_character (0.01s)
    --- PASS: Test_postageDiluteHandler_invalidInputs/batch_id_-_invalid_length (0.01s)
    --- PASS: Test_postageDiluteHandler_invalidInputs/depth_-_invalid_syntax (0.00s)
=== CONT  TestPostageGetStamp
--- PASS: TestFeedIndirection (0.02s)
=== CONT  Test_peerBalanceHandler_invalidInputs
=== RUN   TestPostageGetBuckets/ok
=== PAUSE TestPostageGetBuckets/ok
=== RUN   TestPostageGetBuckets/batch_not_found
=== PAUSE TestPostageGetBuckets/batch_not_found
=== RUN   Test_compensatedPeerBalanceHandler_invalidInputs/peer_-_odd_hex_string
=== PAUSE Test_compensatedPeerBalanceHandler_invalidInputs/peer_-_odd_hex_string
=== RUN   Test_compensatedPeerBalanceHandler_invalidInputs/peer_-_invalid_hex_character
=== PAUSE Test_compensatedPeerBalanceHandler_invalidInputs/peer_-_invalid_hex_character
=== CONT  TestGetAllBatches
=== CONT  TestConsumedPeersNoBalance
=== RUN   Test_peerBalanceHandler_invalidInputs/peer_-_odd_hex_string
=== PAUSE Test_peerBalanceHandler_invalidInputs/peer_-_odd_hex_string
=== RUN   Test_peerBalanceHandler_invalidInputs/peer_-_invalid_hex_character
=== PAUSE Test_peerBalanceHandler_invalidInputs/peer_-_invalid_hex_character
=== CONT  TestConsumedPeersError
=== RUN   TestPostageGetStamp/ok
=== PAUSE TestPostageGetStamp/ok
=== RUN   TestGetAllBatches/all_stamps
=== PAUSE TestGetAllBatches/all_stamps
=== CONT  TestPostageGetStamps
=== RUN   TestPostageGetStamps/single_stamp
=== PAUSE TestPostageGetStamps/single_stamp
=== RUN   TestPostageGetStamps/expired_batch
=== PAUSE TestPostageGetStamps/expired_batch
=== RUN   TestPostageGetStamps/expired_Stamp
=== PAUSE TestPostageGetStamps/expired_Stamp
=== RUN   TestPostageGetStamps/single_expired_Stamp
=== CONT  TestPostageCreateStamp
=== PAUSE TestPostageGetStamps/single_expired_Stamp
=== RUN   TestPostageCreateStamp/ok
=== PAUSE TestPostageCreateStamp/ok
=== RUN   TestPostageCreateStamp/with-custom-gas
=== CONT  Test_peerConnectHandler_invalidInputs
=== PAUSE TestPostageCreateStamp/with-custom-gas
=== RUN   TestPostageCreateStamp/with-error
=== PAUSE TestPostageCreateStamp/with-error
=== RUN   TestPostageCreateStamp/out-of-funds
=== PAUSE TestPostageCreateStamp/out-of-funds
=== RUN   TestPostageCreateStamp/depth_less_than_bucket_depth
=== PAUSE TestPostageCreateStamp/depth_less_than_bucket_depth
=== RUN   TestPostageCreateStamp/immutable_header
=== PAUSE TestPostageCreateStamp/immutable_header
=== RUN   TestPostageCreateStamp/gas_limit_header
=== PAUSE TestPostageCreateStamp/gas_limit_header
=== RUN   TestPostageCreateStamp/syncing_in_progress
=== PAUSE TestPostageCreateStamp/syncing_in_progress
=== RUN   TestPostageCreateStamp/syncing_failed
=== PAUSE TestPostageCreateStamp/syncing_failed
=== CONT  TestPingpong
=== RUN   Test_postageTopUpHandler_invalidInputs/batch_id_-_invalid_length
=== RUN   TestPingpong/ok
=== PAUSE TestPingpong/ok
=== RUN   TestPingpong/peer_not_found
=== PAUSE TestPingpong/peer_not_found
=== RUN   TestPingpong/error
=== PAUSE TestPingpong/error
=== CONT  Test_peerSettlementsHandler_invalidInputs
=== RUN   Test_peerConnectHandler_invalidInputs/multi-address_-_invalid_value
=== PAUSE Test_peerConnectHandler_invalidInputs/multi-address_-_invalid_value
=== CONT  Test_pinHandlers_invalidInputs
=== RUN   Test_postageCreateHandler_invalidInputs/depth_-_invalid_value
=== RUN   Test_postageTopUpHandler_invalidInputs/amount_-_invalid_value
--- PASS: TestConsumedPeersError (0.01s)
=== CONT  Test_stewardshipHandlers_invalidInputs
=== RUN   Test_stewardshipHandlers_invalidInputs/GET_address_-_odd_hex_string
--- PASS: TestConsumedPeersNoBalance (0.01s)
=== CONT  Test_peerDisconnectHandler_invalidInputs
=== RUN   Test_peerSettlementsHandler_invalidInputs/peer_-_odd_hex_string
=== PAUSE Test_peerSettlementsHandler_invalidInputs/peer_-_odd_hex_string
=== RUN   Test_peerSettlementsHandler_invalidInputs/peer_-_invalid_hex_character
=== PAUSE Test_peerSettlementsHandler_invalidInputs/peer_-_invalid_hex_character
=== CONT  TestWithdrawAllStake
=== RUN   TestWithdrawAllStake/ok
=== PAUSE TestWithdrawAllStake/ok
=== RUN   TestWithdrawAllStake/with_invalid_stake_amount
=== PAUSE TestWithdrawAllStake/with_invalid_stake_amount
=== RUN   TestWithdrawAllStake/internal_error
=== PAUSE TestWithdrawAllStake/internal_error
=== RUN   TestWithdrawAllStake/gas_limit_header
=== PAUSE TestWithdrawAllStake/gas_limit_header
=== PAUSE Test_stewardshipHandlers_invalidInputs/GET_address_-_odd_hex_string
=== RUN   Test_stewardshipHandlers_invalidInputs/GET_address_-_invalid_hex_character
=== PAUSE Test_stewardshipHandlers_invalidInputs/GET_address_-_invalid_hex_character
=== RUN   Test_stewardshipHandlers_invalidInputs/PUT_address_-_odd_hex_string
=== PAUSE Test_stewardshipHandlers_invalidInputs/PUT_address_-_odd_hex_string
=== RUN   Test_stewardshipHandlers_invalidInputs/PUT_address_-_invalid_hex_character
=== PAUSE Test_stewardshipHandlers_invalidInputs/PUT_address_-_invalid_hex_character
=== CONT  Test_stakingDepositHandler_invalidInputs
=== RUN   Test_pinHandlers_invalidInputs/GET_reference_-_odd_hex_string
=== PAUSE Test_pinHandlers_invalidInputs/GET_reference_-_odd_hex_string
=== RUN   Test_pinHandlers_invalidInputs/GET_reference_-_invalid_hex_character
=== RUN   Test_peerDisconnectHandler_invalidInputs/address_-_odd_hex_string
=== PAUSE Test_peerDisconnectHandler_invalidInputs/address_-_odd_hex_string
=== RUN   Test_peerDisconnectHandler_invalidInputs/address_-_invalid_hex_character
=== PAUSE Test_peerDisconnectHandler_invalidInputs/address_-_invalid_hex_character
=== PAUSE Test_pinHandlers_invalidInputs/GET_reference_-_invalid_hex_character
=== RUN   Test_pinHandlers_invalidInputs/POST_reference_-_odd_hex_string
=== CONT  TestBlocklistedPeersErr
=== PAUSE Test_pinHandlers_invalidInputs/POST_reference_-_odd_hex_string
=== RUN   Test_pinHandlers_invalidInputs/POST_reference_-_invalid_hex_character
=== PAUSE Test_pinHandlers_invalidInputs/POST_reference_-_invalid_hex_character
=== RUN   Test_pinHandlers_invalidInputs/DELETE_reference_-_odd_hex_string
=== PAUSE Test_pinHandlers_invalidInputs/DELETE_reference_-_odd_hex_string
=== RUN   Test_pinHandlers_invalidInputs/DELETE_reference_-_invalid_hex_character
=== PAUSE Test_pinHandlers_invalidInputs/DELETE_reference_-_invalid_hex_character
--- PASS: Test_postageTopUpHandler_invalidInputs (0.02s)
    --- PASS: Test_postageTopUpHandler_invalidInputs/batch_id_-_odd_hex_string (0.01s)
    --- PASS: Test_postageTopUpHandler_invalidInputs/batch_id_-_invalid_hex_character (0.01s)
    --- PASS: Test_postageTopUpHandler_invalidInputs/batch_id_-_invalid_length (0.00s)
    --- PASS: Test_postageTopUpHandler_invalidInputs/amount_-_invalid_value (0.01s)
=== CONT  TestGetStake
=== RUN   TestGetStake/ok
=== PAUSE TestGetStake/ok
=== RUN   TestGetStake/with_error
=== PAUSE TestGetStake/with_error
=== CONT  TestDepositStake
=== RUN   TestDepositStake/ok
=== PAUSE TestDepositStake/ok
=== RUN   TestDepositStake/with_invalid_stake_amount
=== PAUSE TestDepositStake/with_invalid_stake_amount
=== RUN   TestDepositStake/out_of_funds
=== PAUSE TestDepositStake/out_of_funds
=== RUN   TestDepositStake/internal_error
=== PAUSE TestDepositStake/internal_error
=== RUN   TestDepositStake/gas_limit_header
=== PAUSE TestDepositStake/gas_limit_header
=== CONT  TestBalances
=== RUN   Test_stakingDepositHandler_invalidInputs/amount_-_invalid_value
--- PASS: TestBlocklistedPeersErr (0.00s)
=== CONT  TestBlocklistedPeers
=== PAUSE Test_stakingDepositHandler_invalidInputs/amount_-_invalid_value
=== CONT  TestSettlementsPeersError
=== CONT  TestConsumedError
--- PASS: Test_postageCreateHandler_invalidInputs (0.02s)
    --- PASS: Test_postageCreateHandler_invalidInputs/amount_-_invalid_value (0.01s)
    --- PASS: Test_postageCreateHandler_invalidInputs/depth_-_invalid_value (0.01s)
=== CONT  TestBalancesPeersNoBalance
=== CONT  TestConsumedBalances
--- PASS: TestBlocklistedPeers (0.00s)
=== CONT  TestBalancesPeersError
--- PASS: TestBalancesPeersNoBalance (0.00s)
=== CONT  TestBalancesPeers
--- PASS: TestSettlementsPeersError (0.00s)
=== CONT  TestBalancesError
--- PASS: TestBalancesPeersError (0.00s)
=== CONT  TestSettlementsError
--- PASS: TestConsumedError (0.00s)
--- PASS: TestBalances (0.01s)
--- PASS: TestConsumedBalances (0.01s)
=== CONT  TestSettlements
=== CONT  TestSettlementsPeersNoSettlements
=== RUN   TestSettlementsPeersNoSettlements/no_sent
=== PAUSE TestSettlementsPeersNoSettlements/no_sent
=== RUN   TestSettlementsPeersNoSettlements/no_received
=== PAUSE TestSettlementsPeersNoSettlements/no_received
=== CONT  TestRedistributionStatus
=== RUN   TestRedistributionStatus/success
=== PAUSE TestRedistributionStatus/success
=== RUN   TestRedistributionStatus/bad_request
=== PAUSE TestRedistributionStatus/bad_request
=== CONT  TestSettlementsPeers
--- PASS: TestSettlements (0.01s)
=== CONT  TestPostageDirectAndDeferred_FLAKY
=== RUN   TestPostageDirectAndDeferred_FLAKY/bytes_deferred
=== PAUSE TestPostageDirectAndDeferred_FLAKY/bytes_deferred
=== RUN   TestPostageDirectAndDeferred_FLAKY/bytes_direct_upload
=== PAUSE TestPostageDirectAndDeferred_FLAKY/bytes_direct_upload
=== RUN   TestPostageDirectAndDeferred_FLAKY/bzz_deferred
=== PAUSE TestPostageDirectAndDeferred_FLAKY/bzz_deferred
=== RUN   TestPostageDirectAndDeferred_FLAKY/bzz_direct_upload
=== PAUSE TestPostageDirectAndDeferred_FLAKY/bzz_direct_upload
=== RUN   TestPostageDirectAndDeferred_FLAKY/chunks_deferred
=== PAUSE TestPostageDirectAndDeferred_FLAKY/chunks_deferred
=== RUN   TestPostageDirectAndDeferred_FLAKY/chunks_direct_upload
=== PAUSE TestPostageDirectAndDeferred_FLAKY/chunks_direct_upload
--- PASS: TestBalancesError (0.02s)
=== CONT  TestOptions
=== RUN   TestOptions/tags_options_test
=== PAUSE TestOptions/tags_options_test
=== RUN   TestOptions/bzz_options_test
=== PAUSE TestOptions/bzz_options_test
=== RUN   TestOptions/chunks_options_test
=== PAUSE TestOptions/chunks_options_test
=== RUN   TestOptions/chunks/123213_options_test
=== PAUSE TestOptions/chunks/123213_options_test
=== RUN   TestOptions/bytes_options_test
=== PAUSE TestOptions/bytes_options_test
=== RUN   TestOptions/bytes/0121012_options_test
=== PAUSE TestOptions/bytes/0121012_options_test
=== CONT  TestReadiness
=== RUN   TestReadiness/probe_not_set
=== PAUSE TestReadiness/probe_not_set
=== RUN   TestReadiness/readiness_probe_status_change
=== PAUSE TestReadiness/readiness_probe_status_change
=== CONT  TestPeer
--- PASS: TestSettlementsError (0.02s)
=== CONT  TestParseName
=== CONT  TestPostageHeaderError
=== RUN   TestParseName/empty_name
=== PAUSE TestParseName/empty_name
=== CONT  TestCalculateNumberOfChunksEncrypted
--- PASS: TestBalancesPeers (0.02s)
--- PASS: TestCalculateNumberOfChunksEncrypted (0.00s)
=== CONT  TestCalculateNumberOfChunks
=== CONT  TestAccountingInfoError
--- PASS: TestCalculateNumberOfChunks (0.00s)
=== CONT  Test_pssPostHandler_invalidInputs/targets_-_odd_length_hex_string
=== RUN   TestPeer/ok
=== PAUSE TestPeer/ok
=== CONT  Test_pssPostHandler_invalidInputs/targets_-_odd_length_hex_string#01
=== CONT  TestDisconnect/ok
=== CONT  TestHealth/probe_not_set
=== RUN   TestPostageHeaderError/bytes:_empty_batch
=== PAUSE TestPostageHeaderError/bytes:_empty_batch
=== RUN   TestPostageHeaderError/bytes:_ok_batch
=== PAUSE TestPostageHeaderError/bytes:_ok_batch
=== RUN   TestPostageHeaderError/bytes:_bad_batch
=== PAUSE TestPostageHeaderError/bytes:_bad_batch
=== RUN   TestPostageHeaderError/bzz:_empty_batch
=== PAUSE TestPostageHeaderError/bzz:_empty_batch
=== RUN   TestPostageHeaderError/bzz:_ok_batch
=== PAUSE TestPostageHeaderError/bzz:_ok_batch
=== RUN   TestPostageHeaderError/bzz:_bad_batch
=== PAUSE TestPostageHeaderError/bzz:_bad_batch
=== RUN   TestPostageHeaderError/chunks:_empty_batch
=== PAUSE TestPostageHeaderError/chunks:_empty_batch
=== RUN   TestPostageHeaderError/chunks:_ok_batch
=== PAUSE TestPostageHeaderError/chunks:_ok_batch
=== RUN   TestPostageHeaderError/chunks:_bad_batch
=== PAUSE TestPostageHeaderError/chunks:_bad_batch
=== CONT  TestDisconnect/error
=== RUN   TestParseName/bzz_hash
=== PAUSE TestParseName/bzz_hash
--- PASS: TestSettlementsPeers (0.01s)
=== CONT  TestDisconnect/unknown
=== CONT  TestHealth/health_probe_status_change
=== RUN   TestParseName/no_resolver_connected_with_bzz_hash
=== PAUSE TestParseName/no_resolver_connected_with_bzz_hash
=== CONT  Test_loggerSetVerbosityHandler_invalidInputs/exp_-_illegal_base64
--- PASS: TestDisconnect (0.00s)
    --- PASS: TestDisconnect/ok (0.00s)
    --- PASS: TestDisconnect/unknown (0.00s)
    --- PASS: TestDisconnect/error (0.00s)
=== CONT  TestFeed_Get/with_at
--- PASS: TestAccountingInfoError (0.00s)
=== CONT  Test_loggerSetVerbosityHandler_invalidInputs/verbosity_-_invalid_value
=== CONT  Test_loggerSetVerbosityHandler_invalidInputs/exp_-_invalid_regex
=== RUN   TestParseName/no_resolver_connected_with_name
=== PAUSE TestParseName/no_resolver_connected_with_name
=== CONT  TestFeed_Get/latest
=== RUN   TestParseName/name_not_resolved
--- PASS: Test_pssPostHandler_invalidInputs (0.00s)
    --- PASS: Test_pssPostHandler_invalidInputs/targets_-_odd_length_hex_string (0.00s)
    --- PASS: Test_pssPostHandler_invalidInputs/targets_-_odd_length_hex_string#01 (0.00s)
=== PAUSE TestParseName/name_not_resolved
=== CONT  Test_loggerGetHandler_invalidInputs/exp_-_illegal_base64
=== CONT  TestDBIndices/success
=== RUN   TestParseName/name_resolved
=== PAUSE TestParseName/name_resolved
=== CONT  Test_loggerGetHandler_invalidInputs/exp_-_invalid_regex
=== CONT  TestDBIndices/not_implemented_error_returned
=== CONT  TestDBIndices/internal_error_returned
=== CONT  TestSubdomains/nested_files_with_extension
--- PASS: Test_loggerGetHandler_invalidInputs (0.00s)
    --- PASS: Test_loggerGetHandler_invalidInputs/exp_-_invalid_regex (0.00s)
    --- PASS: Test_loggerGetHandler_invalidInputs/exp_-_illegal_base64 (0.00s)
--- PASS: Test_loggerSetVerbosityHandler_invalidInputs (0.00s)
    --- PASS: Test_loggerSetVerbosityHandler_invalidInputs/exp_-_illegal_base64 (0.00s)
    --- PASS: Test_loggerSetVerbosityHandler_invalidInputs/verbosity_-_invalid_value (0.00s)
    --- PASS: Test_loggerSetVerbosityHandler_invalidInputs/exp_-_invalid_regex (0.00s)
--- PASS: TestFeed_Get (0.00s)
    --- PASS: TestFeed_Get/latest (0.00s)
    --- PASS: TestFeed_Get/with_at (0.00s)
=== CONT  TestCorsStatus/tags
=== CONT  TestSubdomains/explicit_index_and_error_filename
=== CONT  TestCorsStatus/chunks/0101011
=== CONT  TestCorsStatus/bytes
--- PASS: TestDBIndices (0.00s)
    --- PASS: TestDBIndices/internal_error_returned (0.00s)
    --- PASS: TestDBIndices/success (0.00s)
    --- PASS: TestDBIndices/not_implemented_error_returned (0.01s)
=== CONT  TestCorsStatus/bzz
--- PASS: TestHealth (0.00s)
    --- PASS: TestHealth/probe_not_set (0.00s)
    --- PASS: TestHealth/health_probe_status_change (0.01s)
=== CONT  TestCorsStatus/chunks
=== CONT  TestSetWelcomeMessageInternalServerError/internal_server_error_-_error_on_store
=== CONT  TestCors/tags
=== CONT  TestCors/bytes/0121012
=== CONT  TestCors/bytes
=== CONT  TestCorsStatus/bytes/0121012
--- PASS: TestSetWelcomeMessageInternalServerError (0.00s)
    --- PASS: TestSetWelcomeMessageInternalServerError/internal_server_error_-_error_on_store (0.00s)
=== CONT  TestCors/chunks/123213
=== CONT  TestCors/chunks
=== CONT  TestCors/bzz/0101011
=== CONT  TestCors/bzz
=== CONT  TestConnect/ok
=== CONT  TestCORSHeaders/none
=== CONT  TestConnect/error_-_add_peer
=== CONT  TestConnect/error
--- PASS: TestSubdomains (0.00s)
    --- PASS: TestSubdomains/nested_files_with_extension (0.01s)
    --- PASS: TestSubdomains/explicit_index_and_error_filename (0.01s)
=== CONT  TestCORSHeaders/with_origin_only_not_nil
--- PASS: TestCorsStatus (0.00s)
    --- PASS: TestCorsStatus/chunks/0101011 (0.01s)
    --- PASS: TestCorsStatus/tags (0.01s)
    --- PASS: TestCorsStatus/bytes (0.00s)
    --- PASS: TestCorsStatus/chunks (0.00s)
    --- PASS: TestCorsStatus/bzz (0.00s)
    --- PASS: TestCorsStatus/bytes/0121012 (0.00s)
=== CONT  TestCORSHeaders/with_origin_only
=== CONT  TestCORSHeaders/wildcard#01
--- PASS: TestPssWebsocketSingleHandler (0.08s)
--- PASS: TestCors (0.00s)
    --- PASS: TestCors/bytes (0.00s)
    --- PASS: TestCors/bytes/0121012 (0.00s)
    --- PASS: TestCors/tags (0.00s)
    --- PASS: TestCors/chunks (0.00s)
    --- PASS: TestCors/chunks/123213 (0.00s)
    --- PASS: TestCors/bzz (0.00s)
    --- PASS: TestCors/bzz/0101011 (0.00s)
=== CONT  TestCORSHeaders/multiple_explicit_blocked
=== CONT  TestCORSHeaders/wildcard
=== CONT  TestCORSHeaders/multiple_explicit
=== CONT  TestCORSHeaders/single_explicit_blocked
=== CONT  TestCORSHeaders/single_explicit
=== CONT  TestCORSHeaders/no_origin
=== CONT  TestWallet/Okay
=== CONT  TestWallet/500_-_chain_backend_error
=== CONT  TestWallet/500_-_erc20_error
=== CONT  TestMapStructure_InputOutputSanityCheck/input_is_nil
=== CONT  TestMapStructure_InputOutputSanityCheck/output_is_not_a_struct
=== CONT  TestMapStructure_InputOutputSanityCheck/output_is_a_nil_pointer
=== CONT  TestMapStructure_InputOutputSanityCheck/output_is_nil
=== CONT  TestMapStructure_InputOutputSanityCheck/output_is_not_a_pointer
=== CONT  TestMapStructure_InputOutputSanityCheck/input_is_not_a_map
--- PASS: TestMapStructure_InputOutputSanityCheck (0.00s)
    --- PASS: TestMapStructure_InputOutputSanityCheck/input_is_nil (0.00s)
    --- PASS: TestMapStructure_InputOutputSanityCheck/output_is_not_a_struct (0.00s)
    --- PASS: TestMapStructure_InputOutputSanityCheck/output_is_a_nil_pointer (0.00s)
    --- PASS: TestMapStructure_InputOutputSanityCheck/output_is_nil (0.00s)
    --- PASS: TestMapStructure_InputOutputSanityCheck/output_is_not_a_pointer (0.00s)
    --- PASS: TestMapStructure_InputOutputSanityCheck/input_is_not_a_map (0.00s)
=== CONT  TestInvalidChunkParams/batch_unusable
=== CONT  TestAddresses/ok
=== CONT  TestInvalidChunkParams/batch_not_found
=== CONT  TestInvalidChunkParams/batch_exists
=== CONT  TestChequebookCashoutStatus/with_result
--- PASS: TestCORSHeaders (0.00s)
    --- PASS: TestCORSHeaders/none (0.00s)
    --- PASS: TestCORSHeaders/with_origin_only_not_nil (0.00s)
    --- PASS: TestCORSHeaders/with_origin_only (0.00s)
    --- PASS: TestCORSHeaders/wildcard (0.00s)
    --- PASS: TestCORSHeaders/single_explicit (0.00s)
    --- PASS: TestCORSHeaders/wildcard#01 (0.01s)
    --- PASS: TestCORSHeaders/multiple_explicit (0.00s)
    --- PASS: TestCORSHeaders/multiple_explicit_blocked (0.00s)
    --- PASS: TestCORSHeaders/single_explicit_blocked (0.00s)
    --- PASS: TestCORSHeaders/no_origin (0.00s)
=== CONT  TestChequebookCashoutStatus/without_result
--- PASS: TestWallet (0.00s)
    --- PASS: TestWallet/Okay (0.00s)
    --- PASS: TestWallet/500_-_chain_backend_error (0.00s)
    --- PASS: TestWallet/500_-_erc20_error (0.00s)
=== CONT  TestTransactionResend/ok
--- PASS: TestAddresses (0.01s)
    --- PASS: TestAddresses/ok (0.00s)
=== CONT  TestChequebookCashoutStatus/without_last
=== CONT  TestTransactionListError/pending_transactions_error
--- PASS: TestConnect (0.01s)
    --- PASS: TestConnect/ok (0.00s)
    --- PASS: TestConnect/error (0.00s)
    --- PASS: TestConnect/error_-_add_peer (0.01s)
=== CONT  TestTransactionResend/unknown_transaction
=== CONT  TestTransactionResend/other_error
=== CONT  TestTransactionResend/already_imported
=== CONT  TestTransactionListError/pending_transactions_error#01
=== CONT  TestTransactionStoredTransaction/found
=== CONT  TestTransactionStoredTransaction/other_errors
=== CONT  TestTransactionStoredTransaction/not_found
--- PASS: TestTransactionResend (0.00s)
    --- PASS: TestTransactionResend/ok (0.00s)
    --- PASS: TestTransactionResend/unknown_transaction (0.00s)
    --- PASS: TestTransactionResend/other_error (0.00s)
    --- PASS: TestTransactionResend/already_imported (0.00s)
=== CONT  Test_chunkHandlers_invalidInputs/GET_address_-_odd_hex_string
--- PASS: TestTransactionListError (0.00s)
    --- PASS: TestTransactionListError/pending_transactions_error (0.00s)
    --- PASS: TestTransactionListError/pending_transactions_error#01 (0.00s)
=== CONT  Test_chunkHandlers_invalidInputs/DELETE_address_-_invalid_hex_character
=== CONT  Test_chunkHandlers_invalidInputs/DELETE_address_-_odd_hex_string
=== CONT  Test_chunkHandlers_invalidInputs/GET_address_-_invalid_hex_character
=== CONT  TestChequebookDeposit/ok
=== CONT  TestChequebookWithdraw/ok
=== CONT  TestChequebookDeposit/custom_gas
=== CONT  TestMapStructure/bool_zero_value
=== CONT  TestChequebookWithdraw/custom_gas
--- PASS: Test_chunkHandlers_invalidInputs (0.00s)
    --- PASS: Test_chunkHandlers_invalidInputs/GET_address_-_odd_hex_string (0.00s)
    --- PASS: Test_chunkHandlers_invalidInputs/DELETE_address_-_odd_hex_string (0.00s)
    --- PASS: Test_chunkHandlers_invalidInputs/DELETE_address_-_invalid_hex_character (0.00s)
    --- PASS: Test_chunkHandlers_invalidInputs/GET_address_-_invalid_hex_character (0.00s)
--- PASS: TestTransactionStoredTransaction (0.00s)
    --- PASS: TestTransactionStoredTransaction/found (0.00s)
    --- PASS: TestTransactionStoredTransaction/other_errors (0.00s)
    --- PASS: TestTransactionStoredTransaction/not_found (0.00s)
=== CONT  Test_tagHandlers_invalidInputs/GET_id_-_invalid_value
=== CONT  TestMapStructure/swarm.Address_value
=== CONT  TestMapStructure/common.Hash_value
=== CONT  TestMapStructure/bit.Int_value
=== CONT  TestMapStructure/string_with_omitempty_field
=== CONT  TestMapStructure/string_without_matching_field
=== CONT  TestMapStructure/string_single_character
=== CONT  TestMapStructure/string_zero_value
=== CONT  TestMapStructure/byte_slice_invalid_length
=== CONT  TestMapStructure/string_with_multiple_values
=== CONT  TestMapStructure/byte_slice_multiple_bytes
=== CONT  TestMapStructure/string_multiple_characters
=== CONT  TestMapStructure/byte_slice_single_byte
=== CONT  TestMapStructure/byte_slice_zero_value
=== CONT  TestMapStructure/float64_max_value
=== CONT  TestMapStructure/float64_min_value
=== CONT  TestMapStructure/byte_slice_invalid_byte
=== CONT  TestMapStructure/float64_syntax_error
=== CONT  TestMapStructure/float64_zero_value
=== CONT  Test_tagHandlers_invalidInputs/PATCH_id_-_invalid_value
--- PASS: TestInvalidChunkParams (0.00s)
    --- PASS: TestInvalidChunkParams/batch_unusable (0.00s)
    --- PASS: TestInvalidChunkParams/batch_not_found (0.00s)
    --- PASS: TestInvalidChunkParams/batch_exists (0.01s)
=== CONT  TestMapStructure/float64_max_out_of_range_value
--- PASS: TestChequebookWithdraw (0.00s)
    --- PASS: TestChequebookWithdraw/ok (0.00s)
    --- PASS: TestChequebookWithdraw/custom_gas (0.00s)
--- PASS: TestChequebookDeposit (0.00s)
    --- PASS: TestChequebookDeposit/ok (0.00s)
    --- PASS: TestChequebookDeposit/custom_gas (0.00s)
=== CONT  TestMapStructure/float32_max_out_of_range_value
=== CONT  TestMapStructure/float32_max_value
=== CONT  TestMapStructure/float32_min_value
=== CONT  TestMapStructure/float64_in_range_value
=== CONT  Test_tagHandlers_invalidInputs/DELETE_id_-_invalid_value
=== CONT  TestMapStructure/int_min_out_of_range_value
=== CONT  TestMapStructure/int_max_value
=== CONT  TestMapStructure/float32_zero_value
=== CONT  TestMapStructure/int_min_value
=== CONT  TestMapStructure/int_in_range_value
=== CONT  TestMapStructure/int64_syntax_error
=== CONT  TestMapStructure/int64_max_out_of_range_value
=== CONT  TestMapStructure/float32_in_range_value
=== CONT  TestMapStructure/int64_min_out_of_range_value
=== CONT  TestMapStructure/uint64_syntax_error
=== CONT  TestMapStructure/int64_max_value
=== CONT  TestMapStructure/uint64_out_of_range_value
=== CONT  TestMapStructure/float32_syntax_error
=== CONT  TestMapStructure/int64_in_range_value
=== CONT  TestMapStructure/uint64_max_value
=== CONT  TestMapStructure/int32_syntax_error
=== CONT  TestMapStructure/int64_zero_value
=== CONT  TestMapStructure/uint64_in_range_value
=== CONT  TestMapStructure/int32_max_out_of_range_value
=== CONT  TestMapStructure/uint64_zero_value
=== CONT  TestMapStructure/int64_min_value
=== CONT  TestMapStructure/uint32_syntax_error
=== CONT  TestMapStructure/uint32_out_of_range_value
=== CONT  TestMapStructure/int32_in_range_value
=== CONT  TestMapStructure/int32_min_out_of_range_value
=== CONT  TestMapStructure/int16_max_out_of_range_value
=== CONT  TestMapStructure/int32_zero_value
=== CONT  TestMapStructure/uint32_in_range_value
=== CONT  TestMapStructure/int16_max_value
=== CONT  TestMapStructure/uint32_zero_value
=== CONT  TestMapStructure/int16_min_value
=== CONT  TestMapStructure/uint16_syntax_error
=== CONT  TestMapStructure/int16_in_range_value
=== CONT  TestMapStructure/int16_zero_value
=== CONT  TestMapStructure/uint16_out_of_range_value
=== CONT  TestMapStructure/int16_min_out_of_range_value
=== CONT  TestMapStructure/int8_syntax_error
=== CONT  TestMapStructure/uint16_max_value
=== CONT  TestMapStructure/int8_max_out_of_range_value
=== CONT  TestMapStructure/uint16_in_range_value
=== CONT  TestMapStructure/uint16_zero_value
=== CONT  TestMapStructure/int8_max_value
=== CONT  TestMapStructure/uint8_syntax_error
=== CONT  TestMapStructure/int8_min_out_of_range_value
=== CONT  TestMapStructure/uint8_out_of_range_value
=== CONT  TestMapStructure/int32_min_value
=== CONT  TestMapStructure/int8_in_range_value
=== CONT  TestMapStructure/int8_zero_value
=== CONT  TestMapStructure/uint8_max_value
=== CONT  TestMapStructure/int_syntax_error
=== CONT  TestMapStructure/uint8_in_range_value
=== CONT  TestMapStructure/int_max_out_of_range_value
=== CONT  TestMapStructure/uint_syntax_error
=== CONT  TestMapStructure/uint_out_of_range_value
=== CONT  TestMapStructure/uint_max_value
=== CONT  TestMapStructure/bool_syntax_error
=== CONT  TestMapStructure/uint8_zero_value
=== CONT  TestMapStructure/uint32_max_value
=== CONT  TestMapStructure/bool_false
=== CONT  TestMapStructure/int_zero_value
=== CONT  TestMapStructure/bool_true
=== CONT  Test_chequebookLastPeerHandler_invalidInputs/peer_-_odd_hex_string
=== CONT  Test_chequebookLastPeerHandler_invalidInputs/peer_-_invalid_hex_character
=== CONT  TestMapStructure/int32_max_value
=== CONT  TestDebugTags/all
=== CONT  TestMapStructure/int8_min_value
=== CONT  Test_getDebugTagHandler_invalidInputs/id_-_invalid_value
--- PASS: Test_tagHandlers_invalidInputs (0.00s)
    --- PASS: Test_tagHandlers_invalidInputs/GET_id_-_invalid_value (0.00s)
    --- PASS: Test_tagHandlers_invalidInputs/PATCH_id_-_invalid_value (0.00s)
    --- PASS: Test_tagHandlers_invalidInputs/DELETE_id_-_invalid_value (0.00s)
=== CONT  TestMapStructure/int16_syntax_error
=== CONT  TestInvalidBzzParams/batch_unusable
=== CONT  TestMapStructure/uint_in_range_value
=== CONT  Test_postageGetStampHandler_invalidInputs/batch_id_-_odd_hex_string
=== CONT  TestMapStructure/uint_zero_value
=== CONT  TestBzzFilesRangeRequests/bytes
=== CONT  TestInvalidBzzParams/batch_not_found
--- PASS: TestChequebookCashoutStatus (0.00s)
    --- PASS: TestChequebookCashoutStatus/without_result (0.00s)
    --- PASS: TestChequebookCashoutStatus/without_last (0.00s)
    --- PASS: TestChequebookCashoutStatus/with_result (0.01s)
=== CONT  TestInvalidBzzParams/batch_exists
=== CONT  TestBzzFilesRangeRequests/dir
=== CONT  TestBzzFilesRangeRequests/file
=== CONT  Test_bzzDownloadHandler_invalidInputs/address_-_odd_hex_string
=== CONT  TestInvalidBzzParams/address_not_found
=== CONT  TestInvalidBzzParams/upload,_tag_not_found
=== CONT  TestInvalidBzzParams/upload,_invalid_tag
=== CONT  Test_postageGetStampHandler_invalidInputs/batch_id_-_invalid_length
--- PASS: Test_chequebookLastPeerHandler_invalidInputs (0.00s)
    --- PASS: Test_chequebookLastPeerHandler_invalidInputs/peer_-_invalid_hex_character (0.00s)
    --- PASS: Test_chequebookLastPeerHandler_invalidInputs/peer_-_odd_hex_string (0.00s)
--- PASS: TestMapStructure (0.00s)
    --- PASS: TestMapStructure/bool_zero_value (0.00s)
    --- PASS: TestMapStructure/swarm.Address_value (0.00s)
    --- PASS: TestMapStructure/common.Hash_value (0.00s)
    --- PASS: TestMapStructure/bit.Int_value (0.00s)
    --- PASS: TestMapStructure/string_with_omitempty_field (0.00s)
    --- PASS: TestMapStructure/string_without_matching_field (0.00s)
    --- PASS: TestMapStructure/string_single_character (0.00s)
    --- PASS: TestMapStructure/string_zero_value (0.00s)
    --- PASS: TestMapStructure/string_with_multiple_values (0.00s)
    --- PASS: TestMapStructure/byte_slice_multiple_bytes (0.00s)
    --- PASS: TestMapStructure/byte_slice_invalid_length (0.00s)
    --- PASS: TestMapStructure/string_multiple_characters (0.00s)
    --- PASS: TestMapStructure/byte_slice_single_byte (0.00s)
    --- PASS: TestMapStructure/byte_slice_zero_value (0.00s)
    --- PASS: TestMapStructure/float64_max_value (0.00s)
    --- PASS: TestMapStructure/float64_min_value (0.00s)
    --- PASS: TestMapStructure/byte_slice_invalid_byte (0.00s)
    --- PASS: TestMapStructure/float64_zero_value (0.00s)
    --- PASS: TestMapStructure/float64_syntax_error (0.00s)
    --- PASS: TestMapStructure/float64_max_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/float32_max_value (0.00s)
    --- PASS: TestMapStructure/float32_max_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/float32_min_value (0.00s)
    --- PASS: TestMapStructure/float64_in_range_value (0.00s)
    --- PASS: TestMapStructure/int_max_value (0.00s)
    --- PASS: TestMapStructure/int_min_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/float32_zero_value (0.00s)
    --- PASS: TestMapStructure/int_in_range_value (0.00s)
    --- PASS: TestMapStructure/int_min_value (0.00s)
    --- PASS: TestMapStructure/int64_syntax_error (0.00s)
    --- PASS: TestMapStructure/int64_max_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/float32_in_range_value (0.00s)
    --- PASS: TestMapStructure/int64_min_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/int64_max_value (0.00s)
    --- PASS: TestMapStructure/uint64_syntax_error (0.00s)
    --- PASS: TestMapStructure/uint64_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/float32_syntax_error (0.00s)
    --- PASS: TestMapStructure/int64_in_range_value (0.00s)
    --- PASS: TestMapStructure/uint64_max_value (0.00s)
    --- PASS: TestMapStructure/uint64_in_range_value (0.00s)
    --- PASS: TestMapStructure/int64_zero_value (0.00s)
    --- PASS: TestMapStructure/int32_syntax_error (0.00s)
    --- PASS: TestMapStructure/int32_max_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/uint64_zero_value (0.00s)
    --- PASS: TestMapStructure/int64_min_value (0.00s)
    --- PASS: TestMapStructure/uint32_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/uint32_syntax_error (0.00s)
    --- PASS: TestMapStructure/int32_in_range_value (0.00s)
    --- PASS: TestMapStructure/int32_min_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/int16_max_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/int32_zero_value (0.00s)
    --- PASS: TestMapStructure/uint32_in_range_value (0.00s)
    --- PASS: TestMapStructure/int16_max_value (0.00s)
    --- PASS: TestMapStructure/uint32_zero_value (0.00s)
    --- PASS: TestMapStructure/int16_min_value (0.00s)
    --- PASS: TestMapStructure/uint16_syntax_error (0.00s)
    --- PASS: TestMapStructure/int16_in_range_value (0.00s)
    --- PASS: TestMapStructure/int16_zero_value (0.00s)
    --- PASS: TestMapStructure/uint16_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/int8_syntax_error (0.00s)
    --- PASS: TestMapStructure/int16_min_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/uint16_max_value (0.00s)
    --- PASS: TestMapStructure/int8_max_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/uint16_in_range_value (0.00s)
    --- PASS: TestMapStructure/uint16_zero_value (0.00s)
    --- PASS: TestMapStructure/int8_max_value (0.00s)
    --- PASS: TestMapStructure/uint8_syntax_error (0.00s)
    --- PASS: TestMapStructure/int8_min_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/int32_min_value (0.00s)
    --- PASS: TestMapStructure/uint8_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/int8_in_range_value (0.00s)
    --- PASS: TestMapStructure/int8_zero_value (0.00s)
    --- PASS: TestMapStructure/uint8_max_value (0.00s)
    --- PASS: TestMapStructure/uint8_in_range_value (0.00s)
    --- PASS: TestMapStructure/int_syntax_error (0.00s)
    --- PASS: TestMapStructure/int_max_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/uint_syntax_error (0.00s)
    --- PASS: TestMapStructure/uint_out_of_range_value (0.00s)
    --- PASS: TestMapStructure/uint_max_value (0.00s)
    --- PASS: TestMapStructure/bool_syntax_error (0.00s)
    --- PASS: TestMapStructure/uint8_zero_value (0.00s)
    --- PASS: TestMapStructure/uint32_max_value (0.00s)
    --- PASS: TestMapStructure/bool_false (0.00s)
    --- PASS: TestMapStructure/int_zero_value (0.00s)
    --- PASS: TestMapStructure/bool_true (0.00s)
    --- PASS: TestMapStructure/int32_max_value (0.00s)
    --- PASS: TestMapStructure/int8_min_value (0.00s)
    --- PASS: TestMapStructure/int16_syntax_error (0.00s)
    --- PASS: TestMapStructure/uint_in_range_value (0.00s)
    --- PASS: TestMapStructure/uint_zero_value (0.00s)
--- PASS: Test_getDebugTagHandler_invalidInputs (0.00s)
    --- PASS: Test_getDebugTagHandler_invalidInputs/id_-_invalid_value (0.00s)
--- PASS: TestDebugTags (0.00s)
    --- PASS: TestDebugTags/all (0.00s)
=== CONT  Test_postageGetStampHandler_invalidInputs/batch_id_-_invalid_hex_character
=== RUN   TestBzzFilesRangeRequests/bytes/all
=== RUN   TestBzzFilesRangeRequests/bytes/all_without_end
=== CONT  Test_bzzDownloadHandler_invalidInputs/address_-_invalid_hex_character
=== RUN   TestBzzFilesRangeRequests/bytes/all_without_start
=== CONT  Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_odd_hex_string
--- PASS: Test_postageGetStampHandler_invalidInputs (0.00s)
    --- PASS: Test_postageGetStampHandler_invalidInputs/batch_id_-_odd_hex_string (0.00s)
    --- PASS: Test_postageGetStampHandler_invalidInputs/batch_id_-_invalid_length (0.00s)
    --- PASS: Test_postageGetStampHandler_invalidInputs/batch_id_-_invalid_hex_character (0.00s)
=== CONT  Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_invalid_length
--- PASS: Test_bzzDownloadHandler_invalidInputs (0.00s)
    --- PASS: Test_bzzDownloadHandler_invalidInputs/address_-_odd_hex_string (0.00s)
    --- PASS: Test_bzzDownloadHandler_invalidInputs/address_-_invalid_hex_character (0.00s)
=== RUN   TestBzzFilesRangeRequests/bytes/head
=== CONT  Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_invalid_hex_character
=== CONT  Test_bytesGetHandler_invalidInputs/address_-_odd_hex_string
=== RUN   TestBzzFilesRangeRequests/dir/all
=== RUN   TestBzzFilesRangeRequests/bytes/tail
=== RUN   TestBzzFilesRangeRequests/bytes/middle
=== RUN   TestBzzFilesRangeRequests/bytes/multiple
=== RUN   TestBzzFilesRangeRequests/dir/all_without_end
=== CONT  Test_bytesGetHandler_invalidInputs/address_-_invalid_hex_character
=== CONT  Test_postageGetStampsHandler_invalidInputs/all_-_invalid_value
--- PASS: Test_postageGetStampBucketsHandler_invalidInputs (0.00s)
    --- PASS: Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_odd_hex_string (0.00s)
    --- PASS: Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_invalid_length (0.00s)
    --- PASS: Test_postageGetStampBucketsHandler_invalidInputs/batch_id_-_invalid_hex_character (0.00s)
=== RUN   TestBzzFilesRangeRequests/file/all
=== RUN   TestBzzFilesRangeRequests/bytes/even_more_multiple_parts
=== RUN   TestBzzFilesRangeRequests/file/all_without_end
=== RUN   TestBzzFilesRangeRequests/dir/all_without_start
=== CONT  Test_pingpongHandler_invalidInputs/address_-_odd_hex_string
--- PASS: Test_postageGetStampsHandler_invalidInputs (0.00s)
    --- PASS: Test_postageGetStampsHandler_invalidInputs/all_-_invalid_value (0.00s)
=== CONT  TestPostageAccessHandler/create_batch_ok-create_batch_not_ok
=== RUN   TestBzzFilesRangeRequests/file/all_without_start
=== RUN   TestBzzFilesRangeRequests/dir/head
=== CONT  Test_pingpongHandler_invalidInputs/address_-_invalid_hex_character
=== RUN   TestBzzFilesRangeRequests/file/head
=== RUN   TestBzzFilesRangeRequests/dir/tail
=== CONT  Test_bytesUploadHandler_invalidInputs/Content-Type_-_invalid
--- PASS: Test_bytesGetHandler_invalidInputs (0.00s)
    --- PASS: Test_bytesGetHandler_invalidInputs/address_-_odd_hex_string (0.00s)
    --- PASS: Test_bytesGetHandler_invalidInputs/address_-_invalid_hex_character (0.00s)
=== CONT  TestPostageAccessHandler/dilute_batch_ok-dilute_batch_not_ok
=== CONT  TestPostageAccessHandler/dilute_batch_ok-topup_batch_not_ok
--- PASS: Test_pingpongHandler_invalidInputs (0.00s)
    --- PASS: Test_pingpongHandler_invalidInputs/address_-_odd_hex_string (0.00s)
    --- PASS: Test_pingpongHandler_invalidInputs/address_-_invalid_hex_character (0.00s)
=== RUN   TestBzzFilesRangeRequests/dir/middle
=== RUN   TestBzzFilesRangeRequests/file/tail
=== CONT  TestPostageAccessHandler/dilute_batch_ok-create_batch_not_ok
=== RUN   TestBzzFilesRangeRequests/dir/multiple
--- PASS: TestInvalidBzzParams (0.00s)
    --- PASS: TestInvalidBzzParams/batch_unusable (0.00s)
    --- PASS: TestInvalidBzzParams/batch_not_found (0.00s)
    --- PASS: TestInvalidBzzParams/batch_exists (0.00s)
    --- PASS: TestInvalidBzzParams/address_not_found (0.00s)
    --- PASS: TestInvalidBzzParams/upload,_tag_not_found (0.01s)
    --- PASS: TestInvalidBzzParams/upload,_invalid_tag (0.01s)
=== CONT  TestPostageAccessHandler/topup_batch_ok-dilute_batch_not_ok
=== RUN   TestBzzFilesRangeRequests/dir/even_more_multiple_parts
=== CONT  TestPostageAccessHandler/topup_batch_ok-topup_batch_not_ok
=== RUN   TestBzzFilesRangeRequests/file/middle
--- PASS: Test_bytesUploadHandler_invalidInputs (0.00s)
    --- PASS: Test_bytesUploadHandler_invalidInputs/Content-Type_-_invalid (0.00s)
=== RUN   TestBzzFilesRangeRequests/file/multiple
=== RUN   TestBzzFilesRangeRequests/file/even_more_multiple_parts
--- PASS: TestBzzFilesRangeRequests (0.00s)
    --- PASS: TestBzzFilesRangeRequests/bytes (0.01s)
        --- PASS: TestBzzFilesRangeRequests/bytes/all (0.00s)
        --- PASS: TestBzzFilesRangeRequests/bytes/all_without_end (0.00s)
        --- PASS: TestBzzFilesRangeRequests/bytes/all_without_start (0.00s)
        --- PASS: TestBzzFilesRangeRequests/bytes/head (0.00s)
        --- PASS: TestBzzFilesRangeRequests/bytes/tail (0.00s)
        --- PASS: TestBzzFilesRangeRequests/bytes/middle (0.00s)
        --- PASS: TestBzzFilesRangeRequests/bytes/multiple (0.00s)
        --- PASS: TestBzzFilesRangeRequests/bytes/even_more_multiple_parts (0.00s)
    --- PASS: TestBzzFilesRangeRequests/dir (0.01s)
        --- PASS: TestBzzFilesRangeRequests/dir/all (0.00s)
        --- PASS: TestBzzFilesRangeRequests/dir/all_without_end (0.00s)
        --- PASS: TestBzzFilesRangeRequests/dir/all_without_start (0.00s)
        --- PASS: TestBzzFilesRangeRequests/dir/head (0.00s)
        --- PASS: TestBzzFilesRangeRequests/dir/tail (0.00s)
        --- PASS: TestBzzFilesRangeRequests/dir/middle (0.00s)
        --- PASS: TestBzzFilesRangeRequests/dir/multiple (0.00s)
        --- PASS: TestBzzFilesRangeRequests/dir/even_more_multiple_parts (0.00s)
    --- PASS: TestBzzFilesRangeRequests/file (0.02s)
        --- PASS: TestBzzFilesRangeRequests/file/all (0.00s)
        --- PASS: TestBzzFilesRangeRequests/file/all_without_end (0.00s)
        --- PASS: TestBzzFilesRangeRequests/file/all_without_start (0.00s)
        --- PASS: TestBzzFilesRangeRequests/file/head (0.00s)
        --- PASS: TestBzzFilesRangeRequests/file/tail (0.00s)
        --- PASS: TestBzzFilesRangeRequests/file/middle (0.00s)
        --- PASS: TestBzzFilesRangeRequests/file/multiple (0.00s)
        --- PASS: TestBzzFilesRangeRequests/file/even_more_multiple_parts (0.00s)
=== CONT  TestPostageAccessHandler/topup_batch_ok-create_batch_not_ok
=== CONT  TestPostageAccessHandler/create_batch_ok-dilute_batch_not_ok
=== CONT  TestPostageAccessHandler/create_batch_ok-topup_batch_not_ok
=== CONT  TestReserveState/ok
=== CONT  TestReserveState/empty
=== CONT  TestPostageDiluteStamp/ok
=== CONT  TestPostageDiluteStamp/gas_limit_header
=== CONT  TestPostageDiluteStamp/with_depth_error
=== CONT  TestPostageDiluteStamp/with-error
--- PASS: TestReserveState (0.00s)
    --- PASS: TestReserveState/ok (0.00s)
    --- PASS: TestReserveState/empty (0.00s)
=== CONT  TestPostageDiluteStamp/with-custom-gas
=== CONT  TestPostageTopUpStamp/ok
=== CONT  TestPostageTopUpStamp/with-error
=== CONT  TestPostageTopUpStamp/gas_limit_header
=== CONT  TestPostageTopUpStamp/out-of-funds
--- PASS: TestPostageDiluteStamp (0.00s)
    --- PASS: TestPostageDiluteStamp/ok (0.00s)
    --- PASS: TestPostageDiluteStamp/gas_limit_header (0.00s)
    --- PASS: TestPostageDiluteStamp/with_depth_error (0.00s)
    --- PASS: TestPostageDiluteStamp/with-error (0.00s)
    --- PASS: TestPostageDiluteStamp/with-custom-gas (0.00s)
=== CONT  TestPostageTopUpStamp/with-custom-gas
=== CONT  TestPostageGetBuckets/ok
=== CONT  Test_compensatedPeerBalanceHandler_invalidInputs/peer_-_odd_hex_string
=== CONT  Test_compensatedPeerBalanceHandler_invalidInputs/peer_-_invalid_hex_character
=== CONT  Test_peerBalanceHandler_invalidInputs/peer_-_odd_hex_string
=== CONT  TestGetAllBatches/all_stamps
=== CONT  TestPostageGetStamp/ok
--- PASS: Test_compensatedPeerBalanceHandler_invalidInputs (0.00s)
    --- PASS: Test_compensatedPeerBalanceHandler_invalidInputs/peer_-_odd_hex_string (0.00s)
    --- PASS: Test_compensatedPeerBalanceHandler_invalidInputs/peer_-_invalid_hex_character (0.00s)
--- PASS: TestPostageTopUpStamp (0.00s)
    --- PASS: TestPostageTopUpStamp/ok (0.00s)
    --- PASS: TestPostageTopUpStamp/with-error (0.00s)
    --- PASS: TestPostageTopUpStamp/gas_limit_header (0.00s)
    --- PASS: TestPostageTopUpStamp/out-of-funds (0.00s)
    --- PASS: TestPostageTopUpStamp/with-custom-gas (0.00s)
=== CONT  TestPostageGetBuckets/batch_not_found
=== CONT  TestPostageGetStamps/single_stamp
=== CONT  TestPostageGetStamps/single_expired_Stamp
=== CONT  TestPostageCreateStamp/ok
--- PASS: TestGetAllBatches (0.00s)
    --- PASS: TestGetAllBatches/all_stamps (0.00s)
=== CONT  TestPostageGetStamps/expired_Stamp
--- PASS: TestPostageGetStamp (0.00s)
    --- PASS: TestPostageGetStamp/ok (0.00s)
=== CONT  TestPostageGetStamps/expired_batch
=== CONT  TestPingpong/ok
--- PASS: TestPostageGetBuckets (0.00s)
    --- PASS: TestPostageGetBuckets/ok (0.00s)
    --- PASS: TestPostageGetBuckets/batch_not_found (0.00s)
=== CONT  TestPostageCreateStamp/syncing_failed
=== CONT  Test_peerBalanceHandler_invalidInputs/peer_-_invalid_hex_character
=== CONT  TestPostageCreateStamp/syncing_in_progress
=== CONT  TestPostageCreateStamp/gas_limit_header
=== CONT  TestPostageCreateStamp/immutable_header
--- PASS: Test_peerBalanceHandler_invalidInputs (0.00s)
    --- PASS: Test_peerBalanceHandler_invalidInputs/peer_-_odd_hex_string (0.00s)
    --- PASS: Test_peerBalanceHandler_invalidInputs/peer_-_invalid_hex_character (0.00s)
=== CONT  TestPostageCreateStamp/depth_less_than_bucket_depth
=== CONT  TestPostageCreateStamp/out-of-funds
--- PASS: TestPostageGetStamps (0.00s)
    --- PASS: TestPostageGetStamps/single_expired_Stamp (0.00s)
    --- PASS: TestPostageGetStamps/single_stamp (0.00s)
    --- PASS: TestPostageGetStamps/expired_Stamp (0.00s)
    --- PASS: TestPostageGetStamps/expired_batch (0.00s)
=== CONT  TestPostageCreateStamp/with-error
=== CONT  TestPostageCreateStamp/with-custom-gas
=== CONT  Test_peerConnectHandler_invalidInputs/multi-address_-_invalid_value
=== CONT  TestPingpong/error
=== CONT  TestPingpong/peer_not_found
=== CONT  Test_peerSettlementsHandler_invalidInputs/peer_-_odd_hex_string
--- PASS: Test_peerConnectHandler_invalidInputs (0.00s)
    --- PASS: Test_peerConnectHandler_invalidInputs/multi-address_-_invalid_value (0.00s)
=== CONT  TestWithdrawAllStake/ok
=== CONT  Test_peerSettlementsHandler_invalidInputs/peer_-_invalid_hex_character
=== CONT  Test_stewardshipHandlers_invalidInputs/GET_address_-_odd_hex_string
--- PASS: TestPingpong (0.00s)
    --- PASS: TestPingpong/ok (0.00s)
    --- PASS: TestPingpong/error (0.00s)
    --- PASS: TestPingpong/peer_not_found (0.00s)
=== CONT  Test_stewardshipHandlers_invalidInputs/PUT_address_-_invalid_hex_character
--- PASS: TestPostageCreateStamp (0.00s)
    --- PASS: TestPostageCreateStamp/ok (0.00s)
    --- PASS: TestPostageCreateStamp/syncing_in_progress (0.00s)
    --- PASS: TestPostageCreateStamp/gas_limit_header (0.00s)
    --- PASS: TestPostageCreateStamp/syncing_failed (0.00s)
    --- PASS: TestPostageCreateStamp/immutable_header (0.00s)
    --- PASS: TestPostageCreateStamp/with-error (0.00s)
    --- PASS: TestPostageCreateStamp/with-custom-gas (0.00s)
    --- PASS: TestPostageCreateStamp/out-of-funds (0.00s)
    --- PASS: TestPostageCreateStamp/depth_less_than_bucket_depth (0.01s)
=== CONT  Test_stewardshipHandlers_invalidInputs/PUT_address_-_odd_hex_string
=== CONT  Test_stewardshipHandlers_invalidInputs/GET_address_-_invalid_hex_character
=== CONT  Test_peerDisconnectHandler_invalidInputs/address_-_odd_hex_string
--- PASS: Test_peerSettlementsHandler_invalidInputs (0.01s)
    --- PASS: Test_peerSettlementsHandler_invalidInputs/peer_-_odd_hex_string (0.00s)
    --- PASS: Test_peerSettlementsHandler_invalidInputs/peer_-_invalid_hex_character (0.00s)
=== CONT  TestWithdrawAllStake/gas_limit_header
=== CONT  TestWithdrawAllStake/internal_error
=== CONT  TestWithdrawAllStake/with_invalid_stake_amount
=== CONT  Test_pinHandlers_invalidInputs/GET_reference_-_odd_hex_string
=== CONT  Test_peerDisconnectHandler_invalidInputs/address_-_invalid_hex_character
--- PASS: Test_stewardshipHandlers_invalidInputs (0.00s)
    --- PASS: Test_stewardshipHandlers_invalidInputs/GET_address_-_odd_hex_string (0.00s)
    --- PASS: Test_stewardshipHandlers_invalidInputs/PUT_address_-_odd_hex_string (0.00s)
    --- PASS: Test_stewardshipHandlers_invalidInputs/PUT_address_-_invalid_hex_character (0.00s)
    --- PASS: Test_stewardshipHandlers_invalidInputs/GET_address_-_invalid_hex_character (0.00s)
=== CONT  TestDepositStake/ok
=== CONT  TestGetStake/ok
--- PASS: Test_peerDisconnectHandler_invalidInputs (0.00s)
    --- PASS: Test_peerDisconnectHandler_invalidInputs/address_-_odd_hex_string (0.00s)
    --- PASS: Test_peerDisconnectHandler_invalidInputs/address_-_invalid_hex_character (0.00s)
=== CONT  Test_pinHandlers_invalidInputs/DELETE_reference_-_invalid_hex_character
=== CONT  Test_pinHandlers_invalidInputs/DELETE_reference_-_odd_hex_string
--- PASS: TestWithdrawAllStake (0.00s)
    --- PASS: TestWithdrawAllStake/ok (0.00s)
    --- PASS: TestWithdrawAllStake/gas_limit_header (0.00s)
    --- PASS: TestWithdrawAllStake/with_invalid_stake_amount (0.00s)
    --- PASS: TestWithdrawAllStake/internal_error (0.00s)
=== CONT  Test_pinHandlers_invalidInputs/POST_reference_-_invalid_hex_character
=== CONT  Test_pinHandlers_invalidInputs/POST_reference_-_odd_hex_string
=== CONT  Test_pinHandlers_invalidInputs/GET_reference_-_invalid_hex_character
=== CONT  Test_stakingDepositHandler_invalidInputs/amount_-_invalid_value
=== CONT  TestGetStake/with_error
=== CONT  TestDepositStake/gas_limit_header
=== CONT  TestDepositStake/internal_error
--- PASS: Test_pinHandlers_invalidInputs (0.01s)
    --- PASS: Test_pinHandlers_invalidInputs/GET_reference_-_odd_hex_string (0.00s)
    --- PASS: Test_pinHandlers_invalidInputs/DELETE_reference_-_invalid_hex_character (0.00s)
    --- PASS: Test_pinHandlers_invalidInputs/DELETE_reference_-_odd_hex_string (0.00s)
    --- PASS: Test_pinHandlers_invalidInputs/POST_reference_-_invalid_hex_character (0.00s)
    --- PASS: Test_pinHandlers_invalidInputs/POST_reference_-_odd_hex_string (0.00s)
    --- PASS: Test_pinHandlers_invalidInputs/GET_reference_-_invalid_hex_character (0.00s)
=== CONT  TestDepositStake/out_of_funds
--- PASS: Test_stakingDepositHandler_invalidInputs (0.00s)
    --- PASS: Test_stakingDepositHandler_invalidInputs/amount_-_invalid_value (0.00s)
=== CONT  TestDepositStake/with_invalid_stake_amount
=== CONT  TestSettlementsPeersNoSettlements/no_sent
=== CONT  TestRedistributionStatus/success
=== CONT  TestPostageDirectAndDeferred_FLAKY/bytes_deferred
--- PASS: TestGetStake (0.00s)
    --- PASS: TestGetStake/ok (0.00s)
    --- PASS: TestGetStake/with_error (0.00s)
=== CONT  TestOptions/tags_options_test
=== CONT  TestReadiness/probe_not_set
--- PASS: TestDepositStake (0.00s)
    --- PASS: TestDepositStake/ok (0.00s)
    --- PASS: TestDepositStake/gas_limit_header (0.00s)
    --- PASS: TestDepositStake/internal_error (0.00s)
    --- PASS: TestDepositStake/out_of_funds (0.00s)
    --- PASS: TestDepositStake/with_invalid_stake_amount (0.00s)
=== CONT  TestPostageDirectAndDeferred_FLAKY/chunks_direct_upload
=== CONT  TestPostageDirectAndDeferred_FLAKY/chunks_deferred
=== CONT  TestPostageDirectAndDeferred_FLAKY/bzz_direct_upload
=== CONT  TestPostageDirectAndDeferred_FLAKY/bzz_deferred
=== CONT  TestPostageDirectAndDeferred_FLAKY/bytes_direct_upload
=== CONT  TestOptions/chunks_options_test
=== CONT  TestOptions/bytes/0121012_options_test
=== CONT  TestOptions/bytes_options_test
=== CONT  TestOptions/chunks/123213_options_test
=== CONT  TestReadiness/readiness_probe_status_change
=== CONT  TestOptions/bzz_options_test
=== CONT  TestSettlementsPeersNoSettlements/no_received
--- PASS: TestReadiness (0.00s)
    --- PASS: TestReadiness/probe_not_set (0.00s)
    --- PASS: TestReadiness/readiness_probe_status_change (0.00s)
=== CONT  TestPeer/ok
=== CONT  TestRedistributionStatus/bad_request
=== CONT  TestPostageHeaderError/bytes:_empty_batch
--- PASS: TestPeer (0.00s)
    --- PASS: TestPeer/ok (0.00s)
=== CONT  TestPostageHeaderError/chunks:_bad_batch
--- PASS: TestOptions (0.00s)
    --- PASS: TestOptions/tags_options_test (0.00s)
    --- PASS: TestOptions/chunks_options_test (0.00s)
    --- PASS: TestOptions/bytes/0121012_options_test (0.00s)
    --- PASS: TestOptions/bytes_options_test (0.00s)
    --- PASS: TestOptions/chunks/123213_options_test (0.00s)
    --- PASS: TestOptions/bzz_options_test (0.00s)
=== CONT  TestPostageHeaderError/chunks:_ok_batch
=== CONT  TestPostageHeaderError/chunks:_empty_batch
=== CONT  TestPostageHeaderError/bzz:_bad_batch
--- PASS: TestSettlementsPeersNoSettlements (0.00s)
    --- PASS: TestSettlementsPeersNoSettlements/no_sent (0.00s)
    --- PASS: TestSettlementsPeersNoSettlements/no_received (0.00s)
=== CONT  TestPostageHeaderError/bzz:_ok_batch
--- PASS: TestPostageDirectAndDeferred_FLAKY (0.00s)
    --- PASS: TestPostageDirectAndDeferred_FLAKY/chunks_direct_upload (0.00s)
    --- PASS: TestPostageDirectAndDeferred_FLAKY/bytes_deferred (0.00s)
    --- PASS: TestPostageDirectAndDeferred_FLAKY/chunks_deferred (0.00s)
    --- PASS: TestPostageDirectAndDeferred_FLAKY/bytes_direct_upload (0.00s)
    --- PASS: TestPostageDirectAndDeferred_FLAKY/bzz_direct_upload (0.00s)
    --- PASS: TestPostageDirectAndDeferred_FLAKY/bzz_deferred (0.00s)
=== CONT  TestPostageHeaderError/bzz:_empty_batch
=== CONT  TestPostageHeaderError/bytes:_bad_batch
=== CONT  TestPostageHeaderError/bytes:_ok_batch
=== CONT  TestParseName/empty_name
=== CONT  TestParseName/name_resolved
=== CONT  TestParseName/name_not_resolved
=== CONT  TestParseName/no_resolver_connected_with_name
=== CONT  TestParseName/no_resolver_connected_with_bzz_hash
=== CONT  TestParseName/bzz_hash
--- PASS: TestRedistributionStatus (0.00s)
    --- PASS: TestRedistributionStatus/success (0.00s)
    --- PASS: TestRedistributionStatus/bad_request (0.00s)
--- PASS: TestParseName (0.01s)
    --- PASS: TestParseName/empty_name (0.00s)
    --- PASS: TestParseName/name_resolved (0.00s)
    --- PASS: TestParseName/name_not_resolved (0.00s)
    --- PASS: TestParseName/no_resolver_connected_with_name (0.00s)
    --- PASS: TestParseName/no_resolver_connected_with_bzz_hash (0.00s)
    --- PASS: TestParseName/bzz_hash (0.00s)
--- PASS: TestPostageHeaderError (0.00s)
    --- PASS: TestPostageHeaderError/bytes:_empty_batch (0.00s)
    --- PASS: TestPostageHeaderError/chunks:_bad_batch (0.00s)
    --- PASS: TestPostageHeaderError/chunks:_empty_batch (0.00s)
    --- PASS: TestPostageHeaderError/bzz:_bad_batch (0.00s)
    --- PASS: TestPostageHeaderError/bzz:_empty_batch (0.00s)
    --- PASS: TestPostageHeaderError/bytes:_bad_batch (0.00s)
    --- PASS: TestPostageHeaderError/chunks:_ok_batch (0.00s)
    --- PASS: TestPostageHeaderError/bytes:_ok_batch (0.00s)
    --- PASS: TestPostageHeaderError/bzz:_ok_batch (0.00s)
--- PASS: TestPostageAccessHandler (0.00s)
    --- PASS: TestPostageAccessHandler/create_batch_ok-create_batch_not_ok (0.10s)
    --- PASS: TestPostageAccessHandler/dilute_batch_ok-create_batch_not_ok (0.10s)
    --- PASS: TestPostageAccessHandler/topup_batch_ok-dilute_batch_not_ok (0.10s)
    --- PASS: TestPostageAccessHandler/dilute_batch_ok-topup_batch_not_ok (0.10s)
    --- PASS: TestPostageAccessHandler/dilute_batch_ok-dilute_batch_not_ok (0.10s)
    --- PASS: TestPostageAccessHandler/topup_batch_ok-topup_batch_not_ok (0.10s)
    --- PASS: TestPostageAccessHandler/topup_batch_ok-create_batch_not_ok (0.10s)
    --- PASS: TestPostageAccessHandler/create_batch_ok-dilute_batch_not_ok (0.10s)
    --- PASS: TestPostageAccessHandler/create_batch_ok-topup_batch_not_ok (0.10s)
--- PASS: TestPssPingPong (0.54s)
PASS
ok  	github.com/ethersphere/bee/pkg/api	(cached)
=== RUN   TestAuthorize
=== PAUSE TestAuthorize
=== RUN   TestExpiry
=== PAUSE TestExpiry
=== RUN   TestEnforce
=== PAUSE TestEnforce
=== CONT  TestAuthorize
=== RUN   TestAuthorize/correct_credentials
=== PAUSE TestAuthorize/correct_credentials
=== RUN   TestAuthorize/wrong_password
=== PAUSE TestAuthorize/wrong_password
=== CONT  TestAuthorize/correct_credentials
=== CONT  TestExpiry
=== CONT  TestAuthorize/wrong_password
=== CONT  TestEnforce
=== RUN   TestEnforce/success
=== PAUSE TestEnforce/success
=== RUN   TestEnforce/success_with_query_param
=== PAUSE TestEnforce/success_with_query_param
=== RUN   TestEnforce/bad_role
=== PAUSE TestEnforce/bad_role
=== RUN   TestEnforce/bad_resource
=== PAUSE TestEnforce/bad_resource
=== RUN   TestEnforce/bad_action
=== PAUSE TestEnforce/bad_action
=== CONT  TestEnforce/success
=== CONT  TestEnforce/bad_action
=== CONT  TestEnforce/bad_resource
=== CONT  TestEnforce/bad_role
=== CONT  TestEnforce/success_with_query_param
--- PASS: TestEnforce (0.00s)
    --- PASS: TestEnforce/success (0.00s)
    --- PASS: TestEnforce/bad_action (0.00s)
    --- PASS: TestEnforce/bad_resource (0.00s)
    --- PASS: TestEnforce/bad_role (0.00s)
    --- PASS: TestEnforce/success_with_query_param (0.00s)
--- PASS: TestExpiry (0.02s)
--- PASS: TestAuthorize (0.00s)
    --- PASS: TestAuthorize/correct_credentials (0.20s)
    --- PASS: TestAuthorize/wrong_password (0.20s)
PASS
ok  	github.com/ethersphere/bee/pkg/auth	(cached)
=== RUN   TestMarshaling
=== PAUSE TestMarshaling
=== CONT  TestMarshaling
--- PASS: TestMarshaling (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/bigint	(cached)
=== RUN   TestBitvectorNew
=== PAUSE TestBitvectorNew
=== RUN   TestBitvectorGetSet
=== PAUSE TestBitvectorGetSet
=== RUN   TestBitvectorNewFromBytesGet
=== PAUSE TestBitvectorNewFromBytesGet
=== CONT  TestBitvectorNew
--- PASS: TestBitvectorNew (0.00s)
=== CONT  TestBitvectorNewFromBytesGet
--- PASS: TestBitvectorNewFromBytesGet (0.00s)
=== CONT  TestBitvectorGetSet
--- PASS: TestBitvectorGetSet (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/bitvector	(cached)
=== RUN   TestBlocksAfterFlagTimeout
=== PAUSE TestBlocksAfterFlagTimeout
=== RUN   TestUnflagBeforeBlock
=== PAUSE TestUnflagBeforeBlock
=== RUN   TestPruneBeforeBlock
=== PAUSE TestPruneBeforeBlock
=== CONT  TestBlocksAfterFlagTimeout
=== CONT  TestPruneBeforeBlock
=== CONT  TestUnflagBeforeBlock
--- PASS: TestPruneBeforeBlock (0.09s)
--- PASS: TestUnflagBeforeBlock (0.09s)
--- PASS: TestBlocksAfterFlagTimeout (0.09s)
PASS
ok  	github.com/ethersphere/bee/pkg/blocker	(cached)
=== RUN   TestHasherEmptyData
=== PAUSE TestHasherEmptyData
=== RUN   TestSyncHasherCorrectness
=== PAUSE TestSyncHasherCorrectness
=== RUN   TestHasherReuse
=== PAUSE TestHasherReuse
=== RUN   TestBMTConcurrentUse
=== PAUSE TestBMTConcurrentUse
=== RUN   TestBMTWriterBuffers
=== PAUSE TestBMTWriterBuffers
=== RUN   TestUseSyncAsOrdinaryHasher
=== PAUSE TestUseSyncAsOrdinaryHasher
=== RUN   TestProofCorrectness
=== PAUSE TestProofCorrectness
=== RUN   TestProof
=== PAUSE TestProof
=== CONT  TestHasherEmptyData
=== RUN   TestHasherEmptyData/1_segments
=== PAUSE TestHasherEmptyData/1_segments
=== RUN   TestHasherEmptyData/2_segments
=== PAUSE TestHasherEmptyData/2_segments
=== RUN   TestHasherEmptyData/3_segments
=== PAUSE TestHasherEmptyData/3_segments
=== RUN   TestHasherEmptyData/4_segments
=== PAUSE TestHasherEmptyData/4_segments
=== RUN   TestHasherEmptyData/5_segments
=== PAUSE TestHasherEmptyData/5_segments
=== RUN   TestHasherEmptyData/8_segments
=== PAUSE TestHasherEmptyData/8_segments
=== RUN   TestHasherEmptyData/9_segments
=== PAUSE TestHasherEmptyData/9_segments
=== RUN   TestHasherEmptyData/15_segments
=== PAUSE TestHasherEmptyData/15_segments
=== RUN   TestHasherEmptyData/16_segments
=== PAUSE TestHasherEmptyData/16_segments
=== RUN   TestHasherEmptyData/17_segments
=== PAUSE TestHasherEmptyData/17_segments
=== RUN   TestHasherEmptyData/32_segments
=== PAUSE TestHasherEmptyData/32_segments
=== RUN   TestHasherEmptyData/37_segments
=== CONT  TestUseSyncAsOrdinaryHasher
=== PAUSE TestHasherEmptyData/37_segments
=== RUN   TestHasherEmptyData/42_segments
=== PAUSE TestHasherEmptyData/42_segments
=== RUN   TestHasherEmptyData/53_segments
=== PAUSE TestHasherEmptyData/53_segments
=== RUN   TestHasherEmptyData/63_segments
=== PAUSE TestHasherEmptyData/63_segments
=== RUN   TestHasherEmptyData/64_segments
=== PAUSE TestHasherEmptyData/64_segments
=== RUN   TestHasherEmptyData/65_segments
=== PAUSE TestHasherEmptyData/65_segments
=== RUN   TestHasherEmptyData/111_segments
=== PAUSE TestHasherEmptyData/111_segments
=== RUN   TestHasherEmptyData/127_segments
=== PAUSE TestHasherEmptyData/127_segments
=== RUN   TestHasherEmptyData/128_segments
=== PAUSE TestHasherEmptyData/128_segments
=== CONT  TestHasherEmptyData/1_segments
=== CONT  TestBMTWriterBuffers
=== RUN   TestBMTWriterBuffers/1_segments
=== PAUSE TestBMTWriterBuffers/1_segments
=== RUN   TestBMTWriterBuffers/2_segments
=== PAUSE TestBMTWriterBuffers/2_segments
=== RUN   TestBMTWriterBuffers/3_segments
=== PAUSE TestBMTWriterBuffers/3_segments
=== RUN   TestBMTWriterBuffers/4_segments
=== PAUSE TestBMTWriterBuffers/4_segments
=== RUN   TestBMTWriterBuffers/5_segments
=== PAUSE TestBMTWriterBuffers/5_segments
=== RUN   TestBMTWriterBuffers/8_segments
=== PAUSE TestBMTWriterBuffers/8_segments
=== RUN   TestBMTWriterBuffers/9_segments
=== PAUSE TestBMTWriterBuffers/9_segments
=== RUN   TestBMTWriterBuffers/15_segments
=== PAUSE TestBMTWriterBuffers/15_segments
=== RUN   TestBMTWriterBuffers/16_segments
=== PAUSE TestBMTWriterBuffers/16_segments
=== RUN   TestBMTWriterBuffers/17_segments
=== PAUSE TestBMTWriterBuffers/17_segments
=== RUN   TestBMTWriterBuffers/32_segments
=== PAUSE TestBMTWriterBuffers/32_segments
=== RUN   TestBMTWriterBuffers/37_segments
=== PAUSE TestBMTWriterBuffers/37_segments
=== RUN   TestBMTWriterBuffers/42_segments
=== PAUSE TestBMTWriterBuffers/42_segments
=== RUN   TestBMTWriterBuffers/53_segments
=== PAUSE TestBMTWriterBuffers/53_segments
=== RUN   TestBMTWriterBuffers/63_segments
=== PAUSE TestBMTWriterBuffers/63_segments
=== CONT  TestProof
=== RUN   TestBMTWriterBuffers/64_segments
=== PAUSE TestBMTWriterBuffers/64_segments
=== RUN   TestBMTWriterBuffers/65_segments
=== PAUSE TestBMTWriterBuffers/65_segments
=== RUN   TestBMTWriterBuffers/111_segments
=== PAUSE TestBMTWriterBuffers/111_segments
=== RUN   TestBMTWriterBuffers/127_segments
=== PAUSE TestBMTWriterBuffers/127_segments
=== RUN   TestBMTWriterBuffers/128_segments
=== PAUSE TestBMTWriterBuffers/128_segments
=== CONT  TestBMTConcurrentUse
--- PASS: TestUseSyncAsOrdinaryHasher (0.00s)
=== CONT  TestHasherEmptyData/65_segments
=== CONT  TestHasherEmptyData/111_segments
=== CONT  TestHasherReuse
=== RUN   TestHasherReuse/poolsize_1
=== PAUSE TestHasherReuse/poolsize_1
=== RUN   TestHasherReuse/poolsize_16
=== PAUSE TestHasherReuse/poolsize_16
=== CONT  TestSyncHasherCorrectness
=== RUN   TestSyncHasherCorrectness/segments_1
=== PAUSE TestSyncHasherCorrectness/segments_1
=== RUN   TestSyncHasherCorrectness/segments_2
=== PAUSE TestSyncHasherCorrectness/segments_2
=== RUN   TestSyncHasherCorrectness/segments_3
=== PAUSE TestSyncHasherCorrectness/segments_3
=== RUN   TestSyncHasherCorrectness/segments_4
=== PAUSE TestSyncHasherCorrectness/segments_4
=== RUN   TestSyncHasherCorrectness/segments_5
=== PAUSE TestSyncHasherCorrectness/segments_5
=== RUN   TestSyncHasherCorrectness/segments_8
=== PAUSE TestSyncHasherCorrectness/segments_8
=== RUN   TestSyncHasherCorrectness/segments_9
=== PAUSE TestSyncHasherCorrectness/segments_9
=== CONT  TestHasherEmptyData/64_segments
=== CONT  TestHasherEmptyData/128_segments
=== CONT  TestHasherEmptyData/127_segments
=== RUN   TestSyncHasherCorrectness/segments_15
=== PAUSE TestSyncHasherCorrectness/segments_15
=== RUN   TestSyncHasherCorrectness/segments_16
=== PAUSE TestSyncHasherCorrectness/segments_16
=== RUN   TestSyncHasherCorrectness/segments_17
=== PAUSE TestSyncHasherCorrectness/segments_17
=== RUN   TestSyncHasherCorrectness/segments_32
=== PAUSE TestSyncHasherCorrectness/segments_32
=== RUN   TestSyncHasherCorrectness/segments_37
=== PAUSE TestSyncHasherCorrectness/segments_37
=== RUN   TestSyncHasherCorrectness/segments_42
=== PAUSE TestSyncHasherCorrectness/segments_42
=== RUN   TestSyncHasherCorrectness/segments_53
=== PAUSE TestSyncHasherCorrectness/segments_53
=== RUN   TestSyncHasherCorrectness/segments_63
=== PAUSE TestSyncHasherCorrectness/segments_63
=== RUN   TestSyncHasherCorrectness/segments_64
=== PAUSE TestSyncHasherCorrectness/segments_64
=== RUN   TestSyncHasherCorrectness/segments_65
=== PAUSE TestSyncHasherCorrectness/segments_65
=== RUN   TestSyncHasherCorrectness/segments_111
=== PAUSE TestSyncHasherCorrectness/segments_111
=== RUN   TestSyncHasherCorrectness/segments_127
=== PAUSE TestSyncHasherCorrectness/segments_127
=== RUN   TestSyncHasherCorrectness/segments_128
=== PAUSE TestSyncHasherCorrectness/segments_128
=== CONT  TestHasherEmptyData/53_segments
=== CONT  TestHasherEmptyData/42_segments
=== CONT  TestHasherEmptyData/37_segments
=== CONT  TestHasherEmptyData/32_segments
=== CONT  TestHasherEmptyData/17_segments
=== CONT  TestHasherEmptyData/16_segments
=== CONT  TestHasherEmptyData/15_segments
=== CONT  TestHasherEmptyData/9_segments
=== CONT  TestHasherEmptyData/8_segments
=== CONT  TestHasherEmptyData/5_segments
=== CONT  TestHasherEmptyData/4_segments
=== CONT  TestHasherEmptyData/3_segments
=== CONT  TestHasherEmptyData/2_segments
=== CONT  TestProofCorrectness
=== CONT  TestBMTWriterBuffers/1_segments
=== CONT  TestBMTWriterBuffers/128_segments
=== CONT  TestHasherEmptyData/63_segments
=== CONT  TestBMTWriterBuffers/127_segments
=== CONT  TestBMTWriterBuffers/111_segments
=== CONT  TestBMTWriterBuffers/65_segments
=== RUN   TestProofCorrectness/proof_for_left_most
=== PAUSE TestProofCorrectness/proof_for_left_most
=== RUN   TestProofCorrectness/proof_for_right_most
=== PAUSE TestProofCorrectness/proof_for_right_most
=== CONT  TestBMTWriterBuffers/53_segments
--- PASS: TestHasherEmptyData (0.00s)
    --- PASS: TestHasherEmptyData/1_segments (0.00s)
    --- PASS: TestHasherEmptyData/64_segments (0.01s)
    --- PASS: TestHasherEmptyData/53_segments (0.00s)
    --- PASS: TestHasherEmptyData/42_segments (0.00s)
    --- PASS: TestHasherEmptyData/37_segments (0.00s)
    --- PASS: TestHasherEmptyData/32_segments (0.00s)
    --- PASS: TestHasherEmptyData/111_segments (0.01s)
    --- PASS: TestHasherEmptyData/16_segments (0.00s)
    --- PASS: TestHasherEmptyData/15_segments (0.00s)
    --- PASS: TestHasherEmptyData/9_segments (0.00s)
    --- PASS: TestHasherEmptyData/8_segments (0.00s)
    --- PASS: TestHasherEmptyData/5_segments (0.00s)
    --- PASS: TestHasherEmptyData/4_segments (0.00s)
    --- PASS: TestHasherEmptyData/3_segments (0.00s)
    --- PASS: TestHasherEmptyData/2_segments (0.00s)
    --- PASS: TestHasherEmptyData/17_segments (0.00s)
    --- PASS: TestHasherEmptyData/65_segments (0.01s)
    --- PASS: TestHasherEmptyData/128_segments (0.01s)
    --- PASS: TestHasherEmptyData/63_segments (0.00s)
    --- PASS: TestHasherEmptyData/127_segments (0.02s)
=== CONT  TestBMTWriterBuffers/64_segments
--- PASS: TestBMTConcurrentUse (0.02s)
=== CONT  TestBMTWriterBuffers/63_segments
=== RUN   TestProofCorrectness/proof_for_middle
=== PAUSE TestProofCorrectness/proof_for_middle
=== RUN   TestProofCorrectness/root_hash_calculation
=== PAUSE TestProofCorrectness/root_hash_calculation
=== CONT  TestBMTWriterBuffers/42_segments
=== CONT  TestBMTWriterBuffers/37_segments
=== CONT  TestBMTWriterBuffers/32_segments
=== CONT  TestBMTWriterBuffers/17_segments
=== CONT  TestBMTWriterBuffers/16_segments
=== RUN   TestProof/segmentIndex_0
=== PAUSE TestProof/segmentIndex_0
=== RUN   TestProof/segmentIndex_1
=== PAUSE TestProof/segmentIndex_1
=== RUN   TestProof/segmentIndex_2
=== PAUSE TestProof/segmentIndex_2
=== RUN   TestProof/segmentIndex_3
=== PAUSE TestProof/segmentIndex_3
=== RUN   TestProof/segmentIndex_4
=== PAUSE TestProof/segmentIndex_4
=== RUN   TestProof/segmentIndex_5
=== PAUSE TestProof/segmentIndex_5
=== RUN   TestProof/segmentIndex_6
=== PAUSE TestProof/segmentIndex_6
=== RUN   TestProof/segmentIndex_7
=== PAUSE TestProof/segmentIndex_7
=== RUN   TestProof/segmentIndex_8
=== PAUSE TestProof/segmentIndex_8
=== RUN   TestProof/segmentIndex_9
=== PAUSE TestProof/segmentIndex_9
=== RUN   TestProof/segmentIndex_10
=== PAUSE TestProof/segmentIndex_10
=== RUN   TestProof/segmentIndex_11
=== PAUSE TestProof/segmentIndex_11
=== RUN   TestProof/segmentIndex_12
=== PAUSE TestProof/segmentIndex_12
=== RUN   TestProof/segmentIndex_13
=== PAUSE TestProof/segmentIndex_13
=== RUN   TestProof/segmentIndex_14
=== PAUSE TestProof/segmentIndex_14
=== RUN   TestProof/segmentIndex_15
=== PAUSE TestProof/segmentIndex_15
=== RUN   TestProof/segmentIndex_16
=== PAUSE TestProof/segmentIndex_16
=== RUN   TestProof/segmentIndex_17
=== PAUSE TestProof/segmentIndex_17
=== RUN   TestProof/segmentIndex_18
=== PAUSE TestProof/segmentIndex_18
=== RUN   TestProof/segmentIndex_19
=== PAUSE TestProof/segmentIndex_19
=== RUN   TestProof/segmentIndex_20
=== PAUSE TestProof/segmentIndex_20
=== RUN   TestProof/segmentIndex_21
=== PAUSE TestProof/segmentIndex_21
=== RUN   TestProof/segmentIndex_22
=== PAUSE TestProof/segmentIndex_22
=== RUN   TestProof/segmentIndex_23
=== PAUSE TestProof/segmentIndex_23
=== RUN   TestProof/segmentIndex_24
=== PAUSE TestProof/segmentIndex_24
=== RUN   TestProof/segmentIndex_25
=== PAUSE TestProof/segmentIndex_25
=== RUN   TestProof/segmentIndex_26
=== PAUSE TestProof/segmentIndex_26
=== RUN   TestProof/segmentIndex_27
=== PAUSE TestProof/segmentIndex_27
=== RUN   TestProof/segmentIndex_28
=== CONT  TestBMTWriterBuffers/15_segments
=== PAUSE TestProof/segmentIndex_28
=== RUN   TestProof/segmentIndex_29
=== PAUSE TestProof/segmentIndex_29
=== RUN   TestProof/segmentIndex_30
=== PAUSE TestProof/segmentIndex_30
=== RUN   TestProof/segmentIndex_31
=== PAUSE TestProof/segmentIndex_31
=== RUN   TestProof/segmentIndex_32
=== PAUSE TestProof/segmentIndex_32
=== RUN   TestProof/segmentIndex_33
=== PAUSE TestProof/segmentIndex_33
=== RUN   TestProof/segmentIndex_34
=== PAUSE TestProof/segmentIndex_34
=== RUN   TestProof/segmentIndex_35
=== PAUSE TestProof/segmentIndex_35
=== CONT  TestBMTWriterBuffers/9_segments
=== RUN   TestProof/segmentIndex_36
=== PAUSE TestProof/segmentIndex_36
=== RUN   TestProof/segmentIndex_37
=== PAUSE TestProof/segmentIndex_37
=== RUN   TestProof/segmentIndex_38
=== PAUSE TestProof/segmentIndex_38
=== RUN   TestProof/segmentIndex_39
=== PAUSE TestProof/segmentIndex_39
=== RUN   TestProof/segmentIndex_40
=== PAUSE TestProof/segmentIndex_40
=== RUN   TestProof/segmentIndex_41
=== PAUSE TestProof/segmentIndex_41
=== RUN   TestProof/segmentIndex_42
=== PAUSE TestProof/segmentIndex_42
=== RUN   TestProof/segmentIndex_43
=== PAUSE TestProof/segmentIndex_43
=== RUN   TestProof/segmentIndex_44
=== PAUSE TestProof/segmentIndex_44
=== RUN   TestProof/segmentIndex_45
=== PAUSE TestProof/segmentIndex_45
=== RUN   TestProof/segmentIndex_46
=== PAUSE TestProof/segmentIndex_46
=== RUN   TestProof/segmentIndex_47
=== PAUSE TestProof/segmentIndex_47
=== RUN   TestProof/segmentIndex_48
=== PAUSE TestProof/segmentIndex_48
=== RUN   TestProof/segmentIndex_49
=== PAUSE TestProof/segmentIndex_49
=== RUN   TestProof/segmentIndex_50
=== CONT  TestBMTWriterBuffers/8_segments
=== PAUSE TestProof/segmentIndex_50
=== RUN   TestProof/segmentIndex_51
=== PAUSE TestProof/segmentIndex_51
=== RUN   TestProof/segmentIndex_52
=== PAUSE TestProof/segmentIndex_52
=== RUN   TestProof/segmentIndex_53
=== PAUSE TestProof/segmentIndex_53
=== RUN   TestProof/segmentIndex_54
=== PAUSE TestProof/segmentIndex_54
=== RUN   TestProof/segmentIndex_55
=== PAUSE TestProof/segmentIndex_55
=== RUN   TestProof/segmentIndex_56
=== PAUSE TestProof/segmentIndex_56
=== RUN   TestProof/segmentIndex_57
=== PAUSE TestProof/segmentIndex_57
=== RUN   TestProof/segmentIndex_58
=== PAUSE TestProof/segmentIndex_58
=== RUN   TestProof/segmentIndex_59
=== PAUSE TestProof/segmentIndex_59
=== RUN   TestProof/segmentIndex_60
=== PAUSE TestProof/segmentIndex_60
=== RUN   TestProof/segmentIndex_61
=== PAUSE TestProof/segmentIndex_61
=== RUN   TestProof/segmentIndex_62
=== PAUSE TestProof/segmentIndex_62
=== RUN   TestProof/segmentIndex_63
=== PAUSE TestProof/segmentIndex_63
=== RUN   TestProof/segmentIndex_64
=== PAUSE TestProof/segmentIndex_64
=== RUN   TestProof/segmentIndex_65
=== PAUSE TestProof/segmentIndex_65
=== RUN   TestProof/segmentIndex_66
=== PAUSE TestProof/segmentIndex_66
=== RUN   TestProof/segmentIndex_67
=== PAUSE TestProof/segmentIndex_67
=== RUN   TestProof/segmentIndex_68
=== PAUSE TestProof/segmentIndex_68
=== RUN   TestProof/segmentIndex_69
=== PAUSE TestProof/segmentIndex_69
=== RUN   TestProof/segmentIndex_70
=== PAUSE TestProof/segmentIndex_70
=== RUN   TestProof/segmentIndex_71
=== PAUSE TestProof/segmentIndex_71
=== RUN   TestProof/segmentIndex_72
=== PAUSE TestProof/segmentIndex_72
=== RUN   TestProof/segmentIndex_73
=== PAUSE TestProof/segmentIndex_73
=== RUN   TestProof/segmentIndex_74
=== PAUSE TestProof/segmentIndex_74
=== RUN   TestProof/segmentIndex_75
=== PAUSE TestProof/segmentIndex_75
=== RUN   TestProof/segmentIndex_76
=== PAUSE TestProof/segmentIndex_76
=== RUN   TestProof/segmentIndex_77
=== PAUSE TestProof/segmentIndex_77
=== RUN   TestProof/segmentIndex_78
=== PAUSE TestProof/segmentIndex_78
=== RUN   TestProof/segmentIndex_79
=== PAUSE TestProof/segmentIndex_79
=== RUN   TestProof/segmentIndex_80
=== PAUSE TestProof/segmentIndex_80
=== RUN   TestProof/segmentIndex_81
=== PAUSE TestProof/segmentIndex_81
=== RUN   TestProof/segmentIndex_82
=== PAUSE TestProof/segmentIndex_82
=== RUN   TestProof/segmentIndex_83
=== PAUSE TestProof/segmentIndex_83
=== RUN   TestProof/segmentIndex_84
=== PAUSE TestProof/segmentIndex_84
=== RUN   TestProof/segmentIndex_85
=== PAUSE TestProof/segmentIndex_85
=== RUN   TestProof/segmentIndex_86
=== PAUSE TestProof/segmentIndex_86
=== RUN   TestProof/segmentIndex_87
=== PAUSE TestProof/segmentIndex_87
=== RUN   TestProof/segmentIndex_88
=== PAUSE TestProof/segmentIndex_88
=== CONT  TestBMTWriterBuffers/3_segments
=== RUN   TestProof/segmentIndex_89
=== PAUSE TestProof/segmentIndex_89
=== RUN   TestProof/segmentIndex_90
=== PAUSE TestProof/segmentIndex_90
=== RUN   TestProof/segmentIndex_91
=== PAUSE TestProof/segmentIndex_91
=== RUN   TestProof/segmentIndex_92
=== PAUSE TestProof/segmentIndex_92
=== RUN   TestProof/segmentIndex_93
=== PAUSE TestProof/segmentIndex_93
=== RUN   TestProof/segmentIndex_94
=== PAUSE TestProof/segmentIndex_94
=== RUN   TestProof/segmentIndex_95
=== PAUSE TestProof/segmentIndex_95
=== RUN   TestProof/segmentIndex_96
=== CONT  TestBMTWriterBuffers/4_segments
=== PAUSE TestProof/segmentIndex_96
=== RUN   TestProof/segmentIndex_97
=== PAUSE TestProof/segmentIndex_97
=== RUN   TestProof/segmentIndex_98
=== PAUSE TestProof/segmentIndex_98
=== RUN   TestProof/segmentIndex_99
=== PAUSE TestProof/segmentIndex_99
=== RUN   TestProof/segmentIndex_100
=== PAUSE TestProof/segmentIndex_100
=== RUN   TestProof/segmentIndex_101
=== PAUSE TestProof/segmentIndex_101
=== RUN   TestProof/segmentIndex_102
=== PAUSE TestProof/segmentIndex_102
=== RUN   TestProof/segmentIndex_103
=== PAUSE TestProof/segmentIndex_103
=== RUN   TestProof/segmentIndex_104
=== PAUSE TestProof/segmentIndex_104
=== RUN   TestProof/segmentIndex_105
=== PAUSE TestProof/segmentIndex_105
=== RUN   TestProof/segmentIndex_106
=== PAUSE TestProof/segmentIndex_106
=== RUN   TestProof/segmentIndex_107
=== PAUSE TestProof/segmentIndex_107
=== RUN   TestProof/segmentIndex_108
=== PAUSE TestProof/segmentIndex_108
=== RUN   TestProof/segmentIndex_109
=== PAUSE TestProof/segmentIndex_109
=== RUN   TestProof/segmentIndex_110
=== PAUSE TestProof/segmentIndex_110
=== RUN   TestProof/segmentIndex_111
=== PAUSE TestProof/segmentIndex_111
=== RUN   TestProof/segmentIndex_112
=== PAUSE TestProof/segmentIndex_112
=== RUN   TestProof/segmentIndex_113
=== PAUSE TestProof/segmentIndex_113
=== RUN   TestProof/segmentIndex_114
=== PAUSE TestProof/segmentIndex_114
=== RUN   TestProof/segmentIndex_115
=== PAUSE TestProof/segmentIndex_115
=== RUN   TestProof/segmentIndex_116
=== PAUSE TestProof/segmentIndex_116
=== RUN   TestProof/segmentIndex_117
=== PAUSE TestProof/segmentIndex_117
=== RUN   TestProof/segmentIndex_118
=== CONT  TestBMTWriterBuffers/2_segments
=== CONT  TestBMTWriterBuffers/5_segments
=== PAUSE TestProof/segmentIndex_118
=== RUN   TestProof/segmentIndex_119
=== CONT  TestHasherReuse/poolsize_1
=== PAUSE TestProof/segmentIndex_119
=== CONT  TestHasherReuse/poolsize_16
=== CONT  TestSyncHasherCorrectness/segments_1
=== RUN   TestProof/segmentIndex_120
=== PAUSE TestProof/segmentIndex_120
=== RUN   TestProof/segmentIndex_121
=== PAUSE TestProof/segmentIndex_121
=== RUN   TestProof/segmentIndex_122
=== PAUSE TestProof/segmentIndex_122
=== RUN   TestProof/segmentIndex_123
=== PAUSE TestProof/segmentIndex_123
=== RUN   TestProof/segmentIndex_124
=== PAUSE TestProof/segmentIndex_124
=== RUN   TestProof/segmentIndex_125
=== PAUSE TestProof/segmentIndex_125
=== RUN   TestProof/segmentIndex_126
=== PAUSE TestProof/segmentIndex_126
=== RUN   TestProof/segmentIndex_127
=== PAUSE TestProof/segmentIndex_127
=== CONT  TestSyncHasherCorrectness/segments_65
=== CONT  TestSyncHasherCorrectness/segments_111
=== CONT  TestSyncHasherCorrectness/segments_127
=== CONT  TestSyncHasherCorrectness/segments_64
--- PASS: TestBMTWriterBuffers (0.00s)
    --- PASS: TestBMTWriterBuffers/1_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/128_segments (0.02s)
    --- PASS: TestBMTWriterBuffers/63_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/42_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/53_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/32_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/16_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/127_segments (0.01s)
    --- PASS: TestBMTWriterBuffers/9_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/65_segments (0.02s)
    --- PASS: TestBMTWriterBuffers/64_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/17_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/3_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/4_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/2_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/15_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/8_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/37_segments (0.01s)
    --- PASS: TestBMTWriterBuffers/5_segments (0.00s)
    --- PASS: TestBMTWriterBuffers/111_segments (0.02s)
=== CONT  TestSyncHasherCorrectness/segments_53
=== CONT  TestSyncHasherCorrectness/segments_63
=== CONT  TestSyncHasherCorrectness/segments_42
--- PASS: TestHasherReuse (0.00s)
    --- PASS: TestHasherReuse/poolsize_1 (0.04s)
    --- PASS: TestHasherReuse/poolsize_16 (0.06s)
=== CONT  TestSyncHasherCorrectness/segments_37
=== CONT  TestSyncHasherCorrectness/segments_32
=== CONT  TestSyncHasherCorrectness/segments_17
=== CONT  TestSyncHasherCorrectness/segments_16
=== CONT  TestSyncHasherCorrectness/segments_15
=== CONT  TestSyncHasherCorrectness/segments_9
=== CONT  TestSyncHasherCorrectness/segments_8
=== CONT  TestSyncHasherCorrectness/segments_5
=== CONT  TestSyncHasherCorrectness/segments_4
=== CONT  TestSyncHasherCorrectness/segments_3
=== CONT  TestSyncHasherCorrectness/segments_2
=== CONT  TestSyncHasherCorrectness/segments_128
=== CONT  TestProofCorrectness/proof_for_left_most
=== CONT  TestProofCorrectness/root_hash_calculation
=== CONT  TestProofCorrectness/proof_for_middle
=== CONT  TestProofCorrectness/proof_for_right_most
--- PASS: TestProofCorrectness (0.02s)
    --- PASS: TestProofCorrectness/proof_for_left_most (0.00s)
    --- PASS: TestProofCorrectness/root_hash_calculation (0.00s)
    --- PASS: TestProofCorrectness/proof_for_middle (0.00s)
    --- PASS: TestProofCorrectness/proof_for_right_most (0.00s)
=== CONT  TestProof/segmentIndex_0
=== CONT  TestProof/segmentIndex_57
=== CONT  TestProof/segmentIndex_127
=== CONT  TestProof/segmentIndex_126
=== CONT  TestProof/segmentIndex_125
=== CONT  TestProof/segmentIndex_124
=== CONT  TestProof/segmentIndex_123
=== CONT  TestProof/segmentIndex_122
=== CONT  TestProof/segmentIndex_121
=== CONT  TestProof/segmentIndex_120
=== CONT  TestProof/segmentIndex_119
=== CONT  TestProof/segmentIndex_118
=== CONT  TestProof/segmentIndex_117
=== CONT  TestProof/segmentIndex_116
=== CONT  TestProof/segmentIndex_115
=== CONT  TestProof/segmentIndex_114
=== CONT  TestProof/segmentIndex_113
=== CONT  TestProof/segmentIndex_112
=== CONT  TestProof/segmentIndex_111
=== CONT  TestProof/segmentIndex_110
=== CONT  TestProof/segmentIndex_109
=== CONT  TestProof/segmentIndex_108
=== CONT  TestProof/segmentIndex_107
=== CONT  TestProof/segmentIndex_106
=== CONT  TestProof/segmentIndex_105
=== CONT  TestProof/segmentIndex_104
=== CONT  TestProof/segmentIndex_103
=== CONT  TestProof/segmentIndex_102
=== CONT  TestProof/segmentIndex_101
=== CONT  TestProof/segmentIndex_100
=== CONT  TestProof/segmentIndex_99
=== CONT  TestProof/segmentIndex_98
=== CONT  TestProof/segmentIndex_97
=== CONT  TestProof/segmentIndex_96
=== CONT  TestProof/segmentIndex_95
=== CONT  TestProof/segmentIndex_94
=== CONT  TestProof/segmentIndex_93
=== CONT  TestProof/segmentIndex_92
=== CONT  TestProof/segmentIndex_91
=== CONT  TestProof/segmentIndex_90
=== CONT  TestProof/segmentIndex_89
=== CONT  TestProof/segmentIndex_88
=== CONT  TestProof/segmentIndex_87
=== CONT  TestProof/segmentIndex_86
=== CONT  TestProof/segmentIndex_85
=== CONT  TestProof/segmentIndex_84
=== CONT  TestProof/segmentIndex_83
=== CONT  TestProof/segmentIndex_82
=== CONT  TestProof/segmentIndex_81
=== CONT  TestProof/segmentIndex_80
=== CONT  TestProof/segmentIndex_79
=== CONT  TestProof/segmentIndex_78
=== CONT  TestProof/segmentIndex_77
=== CONT  TestProof/segmentIndex_76
=== CONT  TestProof/segmentIndex_75
=== CONT  TestProof/segmentIndex_74
=== CONT  TestProof/segmentIndex_73
=== CONT  TestProof/segmentIndex_72
=== CONT  TestProof/segmentIndex_71
=== CONT  TestProof/segmentIndex_70
=== CONT  TestProof/segmentIndex_69
=== CONT  TestProof/segmentIndex_68
=== CONT  TestProof/segmentIndex_67
=== CONT  TestProof/segmentIndex_66
=== CONT  TestProof/segmentIndex_65
=== CONT  TestProof/segmentIndex_64
=== CONT  TestProof/segmentIndex_63
=== CONT  TestProof/segmentIndex_62
=== CONT  TestProof/segmentIndex_61
=== CONT  TestProof/segmentIndex_60
=== CONT  TestProof/segmentIndex_59
=== CONT  TestProof/segmentIndex_58
=== CONT  TestProof/segmentIndex_56
=== CONT  TestProof/segmentIndex_55
=== CONT  TestProof/segmentIndex_54
=== CONT  TestProof/segmentIndex_53
=== CONT  TestProof/segmentIndex_52
=== CONT  TestProof/segmentIndex_51
=== CONT  TestProof/segmentIndex_50
=== CONT  TestProof/segmentIndex_49
=== CONT  TestProof/segmentIndex_48
=== CONT  TestProof/segmentIndex_47
=== CONT  TestProof/segmentIndex_46
=== CONT  TestProof/segmentIndex_45
=== CONT  TestProof/segmentIndex_44
=== CONT  TestProof/segmentIndex_43
=== CONT  TestProof/segmentIndex_42
=== CONT  TestProof/segmentIndex_41
=== CONT  TestProof/segmentIndex_40
=== CONT  TestProof/segmentIndex_39
=== CONT  TestProof/segmentIndex_38
=== CONT  TestProof/segmentIndex_37
=== CONT  TestProof/segmentIndex_36
=== CONT  TestProof/segmentIndex_35
=== CONT  TestProof/segmentIndex_34
=== CONT  TestProof/segmentIndex_33
=== CONT  TestProof/segmentIndex_32
=== CONT  TestProof/segmentIndex_31
=== CONT  TestProof/segmentIndex_30
=== CONT  TestProof/segmentIndex_29
=== CONT  TestProof/segmentIndex_28
=== CONT  TestProof/segmentIndex_27
=== CONT  TestProof/segmentIndex_26
=== CONT  TestProof/segmentIndex_25
=== CONT  TestProof/segmentIndex_24
=== CONT  TestProof/segmentIndex_23
=== CONT  TestProof/segmentIndex_22
=== CONT  TestProof/segmentIndex_21
=== CONT  TestProof/segmentIndex_20
=== CONT  TestProof/segmentIndex_19
=== CONT  TestProof/segmentIndex_18
=== CONT  TestProof/segmentIndex_17
=== CONT  TestProof/segmentIndex_16
=== CONT  TestProof/segmentIndex_15
=== CONT  TestProof/segmentIndex_14
=== CONT  TestProof/segmentIndex_13
=== CONT  TestProof/segmentIndex_12
=== CONT  TestProof/segmentIndex_11
=== CONT  TestProof/segmentIndex_10
=== CONT  TestProof/segmentIndex_9
=== CONT  TestProof/segmentIndex_8
=== CONT  TestProof/segmentIndex_7
=== CONT  TestProof/segmentIndex_6
=== CONT  TestProof/segmentIndex_5
=== CONT  TestProof/segmentIndex_4
=== CONT  TestProof/segmentIndex_3
=== CONT  TestProof/segmentIndex_2
=== CONT  TestProof/segmentIndex_1
--- PASS: TestProof (0.04s)
    --- PASS: TestProof/segmentIndex_0 (0.00s)
    --- PASS: TestProof/segmentIndex_57 (0.00s)
    --- PASS: TestProof/segmentIndex_127 (0.00s)
    --- PASS: TestProof/segmentIndex_126 (0.00s)
    --- PASS: TestProof/segmentIndex_125 (0.00s)
    --- PASS: TestProof/segmentIndex_124 (0.00s)
    --- PASS: TestProof/segmentIndex_123 (0.00s)
    --- PASS: TestProof/segmentIndex_122 (0.00s)
    --- PASS: TestProof/segmentIndex_121 (0.00s)
    --- PASS: TestProof/segmentIndex_120 (0.00s)
    --- PASS: TestProof/segmentIndex_119 (0.00s)
    --- PASS: TestProof/segmentIndex_118 (0.00s)
    --- PASS: TestProof/segmentIndex_117 (0.00s)
    --- PASS: TestProof/segmentIndex_116 (0.00s)
    --- PASS: TestProof/segmentIndex_115 (0.00s)
    --- PASS: TestProof/segmentIndex_114 (0.00s)
    --- PASS: TestProof/segmentIndex_113 (0.00s)
    --- PASS: TestProof/segmentIndex_112 (0.00s)
    --- PASS: TestProof/segmentIndex_111 (0.00s)
    --- PASS: TestProof/segmentIndex_110 (0.00s)
    --- PASS: TestProof/segmentIndex_109 (0.00s)
    --- PASS: TestProof/segmentIndex_108 (0.00s)
    --- PASS: TestProof/segmentIndex_107 (0.00s)
    --- PASS: TestProof/segmentIndex_106 (0.00s)
    --- PASS: TestProof/segmentIndex_105 (0.00s)
    --- PASS: TestProof/segmentIndex_104 (0.00s)
    --- PASS: TestProof/segmentIndex_103 (0.00s)
    --- PASS: TestProof/segmentIndex_102 (0.00s)
    --- PASS: TestProof/segmentIndex_101 (0.00s)
    --- PASS: TestProof/segmentIndex_100 (0.00s)
    --- PASS: TestProof/segmentIndex_99 (0.00s)
    --- PASS: TestProof/segmentIndex_98 (0.00s)
    --- PASS: TestProof/segmentIndex_97 (0.00s)
    --- PASS: TestProof/segmentIndex_96 (0.00s)
    --- PASS: TestProof/segmentIndex_95 (0.00s)
    --- PASS: TestProof/segmentIndex_94 (0.00s)
    --- PASS: TestProof/segmentIndex_93 (0.00s)
    --- PASS: TestProof/segmentIndex_92 (0.00s)
    --- PASS: TestProof/segmentIndex_91 (0.00s)
    --- PASS: TestProof/segmentIndex_90 (0.00s)
    --- PASS: TestProof/segmentIndex_89 (0.00s)
    --- PASS: TestProof/segmentIndex_88 (0.00s)
    --- PASS: TestProof/segmentIndex_87 (0.00s)
    --- PASS: TestProof/segmentIndex_86 (0.00s)
    --- PASS: TestProof/segmentIndex_85 (0.00s)
    --- PASS: TestProof/segmentIndex_84 (0.00s)
    --- PASS: TestProof/segmentIndex_83 (0.00s)
    --- PASS: TestProof/segmentIndex_82 (0.00s)
    --- PASS: TestProof/segmentIndex_81 (0.00s)
    --- PASS: TestProof/segmentIndex_80 (0.00s)
    --- PASS: TestProof/segmentIndex_79 (0.00s)
    --- PASS: TestProof/segmentIndex_78 (0.00s)
    --- PASS: TestProof/segmentIndex_77 (0.00s)
    --- PASS: TestProof/segmentIndex_76 (0.00s)
    --- PASS: TestProof/segmentIndex_75 (0.00s)
    --- PASS: TestProof/segmentIndex_74 (0.00s)
    --- PASS: TestProof/segmentIndex_73 (0.00s)
    --- PASS: TestProof/segmentIndex_72 (0.00s)
    --- PASS: TestProof/segmentIndex_71 (0.00s)
    --- PASS: TestProof/segmentIndex_70 (0.00s)
    --- PASS: TestProof/segmentIndex_69 (0.00s)
    --- PASS: TestProof/segmentIndex_68 (0.00s)
    --- PASS: TestProof/segmentIndex_67 (0.00s)
    --- PASS: TestProof/segmentIndex_66 (0.00s)
    --- PASS: TestProof/segmentIndex_65 (0.00s)
    --- PASS: TestProof/segmentIndex_64 (0.00s)
    --- PASS: TestProof/segmentIndex_63 (0.00s)
    --- PASS: TestProof/segmentIndex_62 (0.00s)
    --- PASS: TestProof/segmentIndex_61 (0.00s)
    --- PASS: TestProof/segmentIndex_60 (0.00s)
    --- PASS: TestProof/segmentIndex_59 (0.00s)
    --- PASS: TestProof/segmentIndex_58 (0.00s)
    --- PASS: TestProof/segmentIndex_56 (0.00s)
    --- PASS: TestProof/segmentIndex_55 (0.00s)
    --- PASS: TestProof/segmentIndex_54 (0.00s)
    --- PASS: TestProof/segmentIndex_53 (0.00s)
    --- PASS: TestProof/segmentIndex_52 (0.00s)
    --- PASS: TestProof/segmentIndex_51 (0.00s)
    --- PASS: TestProof/segmentIndex_50 (0.00s)
    --- PASS: TestProof/segmentIndex_49 (0.00s)
    --- PASS: TestProof/segmentIndex_48 (0.00s)
    --- PASS: TestProof/segmentIndex_47 (0.00s)
    --- PASS: TestProof/segmentIndex_46 (0.00s)
    --- PASS: TestProof/segmentIndex_45 (0.00s)
    --- PASS: TestProof/segmentIndex_44 (0.00s)
    --- PASS: TestProof/segmentIndex_43 (0.00s)
    --- PASS: TestProof/segmentIndex_42 (0.00s)
    --- PASS: TestProof/segmentIndex_41 (0.00s)
    --- PASS: TestProof/segmentIndex_40 (0.00s)
    --- PASS: TestProof/segmentIndex_39 (0.00s)
    --- PASS: TestProof/segmentIndex_38 (0.00s)
    --- PASS: TestProof/segmentIndex_37 (0.00s)
    --- PASS: TestProof/segmentIndex_36 (0.00s)
    --- PASS: TestProof/segmentIndex_35 (0.00s)
    --- PASS: TestProof/segmentIndex_34 (0.00s)
    --- PASS: TestProof/segmentIndex_33 (0.00s)
    --- PASS: TestProof/segmentIndex_32 (0.00s)
    --- PASS: TestProof/segmentIndex_31 (0.00s)
    --- PASS: TestProof/segmentIndex_30 (0.00s)
    --- PASS: TestProof/segmentIndex_29 (0.00s)
    --- PASS: TestProof/segmentIndex_28 (0.00s)
    --- PASS: TestProof/segmentIndex_27 (0.00s)
    --- PASS: TestProof/segmentIndex_26 (0.00s)
    --- PASS: TestProof/segmentIndex_25 (0.00s)
    --- PASS: TestProof/segmentIndex_24 (0.00s)
    --- PASS: TestProof/segmentIndex_23 (0.00s)
    --- PASS: TestProof/segmentIndex_22 (0.00s)
    --- PASS: TestProof/segmentIndex_21 (0.00s)
    --- PASS: TestProof/segmentIndex_20 (0.00s)
    --- PASS: TestProof/segmentIndex_19 (0.00s)
    --- PASS: TestProof/segmentIndex_18 (0.00s)
    --- PASS: TestProof/segmentIndex_17 (0.00s)
    --- PASS: TestProof/segmentIndex_16 (0.00s)
    --- PASS: TestProof/segmentIndex_15 (0.00s)
    --- PASS: TestProof/segmentIndex_14 (0.00s)
    --- PASS: TestProof/segmentIndex_13 (0.00s)
    --- PASS: TestProof/segmentIndex_12 (0.00s)
    --- PASS: TestProof/segmentIndex_11 (0.00s)
    --- PASS: TestProof/segmentIndex_10 (0.00s)
    --- PASS: TestProof/segmentIndex_9 (0.00s)
    --- PASS: TestProof/segmentIndex_8 (0.00s)
    --- PASS: TestProof/segmentIndex_7 (0.00s)
    --- PASS: TestProof/segmentIndex_6 (0.00s)
    --- PASS: TestProof/segmentIndex_5 (0.00s)
    --- PASS: TestProof/segmentIndex_4 (0.00s)
    --- PASS: TestProof/segmentIndex_3 (0.00s)
    --- PASS: TestProof/segmentIndex_2 (0.00s)
    --- PASS: TestProof/segmentIndex_1 (0.00s)
--- PASS: TestSyncHasherCorrectness (0.01s)
    --- PASS: TestSyncHasherCorrectness/segments_1 (0.00s)
    --- PASS: TestSyncHasherCorrectness/segments_53 (0.12s)
    --- PASS: TestSyncHasherCorrectness/segments_42 (0.08s)
    --- PASS: TestSyncHasherCorrectness/segments_37 (0.07s)
    --- PASS: TestSyncHasherCorrectness/segments_16 (0.00s)
    --- PASS: TestSyncHasherCorrectness/segments_17 (0.02s)
    --- PASS: TestSyncHasherCorrectness/segments_15 (0.01s)
    --- PASS: TestSyncHasherCorrectness/segments_9 (0.00s)
    --- PASS: TestSyncHasherCorrectness/segments_63 (0.14s)
    --- PASS: TestSyncHasherCorrectness/segments_4 (0.00s)
    --- PASS: TestSyncHasherCorrectness/segments_5 (0.00s)
    --- PASS: TestSyncHasherCorrectness/segments_2 (0.00s)
    --- PASS: TestSyncHasherCorrectness/segments_3 (0.00s)
    --- PASS: TestSyncHasherCorrectness/segments_8 (0.00s)
    --- PASS: TestSyncHasherCorrectness/segments_32 (0.04s)
    --- PASS: TestSyncHasherCorrectness/segments_64 (0.16s)
    --- PASS: TestSyncHasherCorrectness/segments_65 (0.19s)
    --- PASS: TestSyncHasherCorrectness/segments_111 (0.32s)
    --- PASS: TestSyncHasherCorrectness/segments_127 (0.35s)
    --- PASS: TestSyncHasherCorrectness/segments_128 (0.28s)
PASS
ok  	github.com/ethersphere/bee/pkg/bmt	(cached)
=== RUN   TestRefHasher
=== PAUSE TestRefHasher
=== CONT  TestRefHasher
=== RUN   TestRefHasher/1_segments_1_bytes
=== PAUSE TestRefHasher/1_segments_1_bytes
=== RUN   TestRefHasher/1_segments_2_bytes
=== PAUSE TestRefHasher/1_segments_2_bytes
=== RUN   TestRefHasher/1_segments_3_bytes
=== PAUSE TestRefHasher/1_segments_3_bytes
=== RUN   TestRefHasher/1_segments_4_bytes
=== PAUSE TestRefHasher/1_segments_4_bytes
=== RUN   TestRefHasher/1_segments_5_bytes
=== PAUSE TestRefHasher/1_segments_5_bytes
=== RUN   TestRefHasher/1_segments_6_bytes
=== PAUSE TestRefHasher/1_segments_6_bytes
=== RUN   TestRefHasher/1_segments_7_bytes
=== PAUSE TestRefHasher/1_segments_7_bytes
=== RUN   TestRefHasher/1_segments_8_bytes
=== PAUSE TestRefHasher/1_segments_8_bytes
=== RUN   TestRefHasher/1_segments_9_bytes
=== PAUSE TestRefHasher/1_segments_9_bytes
=== RUN   TestRefHasher/1_segments_10_bytes
=== PAUSE TestRefHasher/1_segments_10_bytes
=== RUN   TestRefHasher/1_segments_11_bytes
=== PAUSE TestRefHasher/1_segments_11_bytes
=== RUN   TestRefHasher/1_segments_12_bytes
=== PAUSE TestRefHasher/1_segments_12_bytes
=== RUN   TestRefHasher/1_segments_13_bytes
=== PAUSE TestRefHasher/1_segments_13_bytes
=== RUN   TestRefHasher/1_segments_14_bytes
=== PAUSE TestRefHasher/1_segments_14_bytes
=== RUN   TestRefHasher/1_segments_15_bytes
=== PAUSE TestRefHasher/1_segments_15_bytes
=== RUN   TestRefHasher/1_segments_16_bytes
=== PAUSE TestRefHasher/1_segments_16_bytes
=== RUN   TestRefHasher/1_segments_17_bytes
=== PAUSE TestRefHasher/1_segments_17_bytes
=== RUN   TestRefHasher/1_segments_18_bytes
=== PAUSE TestRefHasher/1_segments_18_bytes
=== RUN   TestRefHasher/1_segments_19_bytes
=== PAUSE TestRefHasher/1_segments_19_bytes
=== RUN   TestRefHasher/1_segments_20_bytes
=== PAUSE TestRefHasher/1_segments_20_bytes
=== RUN   TestRefHasher/1_segments_21_bytes
=== PAUSE TestRefHasher/1_segments_21_bytes
=== RUN   TestRefHasher/1_segments_22_bytes
=== PAUSE TestRefHasher/1_segments_22_bytes
=== RUN   TestRefHasher/1_segments_23_bytes
=== PAUSE TestRefHasher/1_segments_23_bytes
=== RUN   TestRefHasher/1_segments_24_bytes
=== PAUSE TestRefHasher/1_segments_24_bytes
=== RUN   TestRefHasher/1_segments_25_bytes
=== PAUSE TestRefHasher/1_segments_25_bytes
=== RUN   TestRefHasher/1_segments_26_bytes
=== PAUSE TestRefHasher/1_segments_26_bytes
=== RUN   TestRefHasher/1_segments_27_bytes
=== PAUSE TestRefHasher/1_segments_27_bytes
=== RUN   TestRefHasher/1_segments_28_bytes
=== PAUSE TestRefHasher/1_segments_28_bytes
=== RUN   TestRefHasher/1_segments_29_bytes
=== PAUSE TestRefHasher/1_segments_29_bytes
=== RUN   TestRefHasher/1_segments_30_bytes
=== PAUSE TestRefHasher/1_segments_30_bytes
=== RUN   TestRefHasher/1_segments_31_bytes
=== PAUSE TestRefHasher/1_segments_31_bytes
=== RUN   TestRefHasher/1_segments_32_bytes
=== PAUSE TestRefHasher/1_segments_32_bytes
=== RUN   TestRefHasher/2_segments_1_bytes
=== PAUSE TestRefHasher/2_segments_1_bytes
=== RUN   TestRefHasher/2_segments_2_bytes
=== PAUSE TestRefHasher/2_segments_2_bytes
=== RUN   TestRefHasher/2_segments_3_bytes
=== PAUSE TestRefHasher/2_segments_3_bytes
=== RUN   TestRefHasher/2_segments_4_bytes
=== PAUSE TestRefHasher/2_segments_4_bytes
=== RUN   TestRefHasher/2_segments_5_bytes
=== PAUSE TestRefHasher/2_segments_5_bytes
=== RUN   TestRefHasher/2_segments_6_bytes
=== PAUSE TestRefHasher/2_segments_6_bytes
=== RUN   TestRefHasher/2_segments_7_bytes
=== PAUSE TestRefHasher/2_segments_7_bytes
=== RUN   TestRefHasher/2_segments_8_bytes
=== PAUSE TestRefHasher/2_segments_8_bytes
=== RUN   TestRefHasher/2_segments_9_bytes
=== PAUSE TestRefHasher/2_segments_9_bytes
=== RUN   TestRefHasher/2_segments_10_bytes
=== PAUSE TestRefHasher/2_segments_10_bytes
=== RUN   TestRefHasher/2_segments_11_bytes
=== PAUSE TestRefHasher/2_segments_11_bytes
=== RUN   TestRefHasher/2_segments_12_bytes
=== PAUSE TestRefHasher/2_segments_12_bytes
=== RUN   TestRefHasher/2_segments_13_bytes
=== PAUSE TestRefHasher/2_segments_13_bytes
=== RUN   TestRefHasher/2_segments_14_bytes
=== PAUSE TestRefHasher/2_segments_14_bytes
=== RUN   TestRefHasher/2_segments_15_bytes
=== PAUSE TestRefHasher/2_segments_15_bytes
=== RUN   TestRefHasher/2_segments_16_bytes
=== PAUSE TestRefHasher/2_segments_16_bytes
=== RUN   TestRefHasher/2_segments_17_bytes
=== PAUSE TestRefHasher/2_segments_17_bytes
=== RUN   TestRefHasher/2_segments_18_bytes
=== PAUSE TestRefHasher/2_segments_18_bytes
=== RUN   TestRefHasher/2_segments_19_bytes
=== PAUSE TestRefHasher/2_segments_19_bytes
=== RUN   TestRefHasher/2_segments_20_bytes
=== PAUSE TestRefHasher/2_segments_20_bytes
=== RUN   TestRefHasher/2_segments_21_bytes
=== PAUSE TestRefHasher/2_segments_21_bytes
=== RUN   TestRefHasher/2_segments_22_bytes
=== PAUSE TestRefHasher/2_segments_22_bytes
=== RUN   TestRefHasher/2_segments_23_bytes
=== PAUSE TestRefHasher/2_segments_23_bytes
=== RUN   TestRefHasher/2_segments_24_bytes
=== PAUSE TestRefHasher/2_segments_24_bytes
=== RUN   TestRefHasher/2_segments_25_bytes
=== PAUSE TestRefHasher/2_segments_25_bytes
=== RUN   TestRefHasher/2_segments_26_bytes
=== PAUSE TestRefHasher/2_segments_26_bytes
=== RUN   TestRefHasher/2_segments_27_bytes
=== PAUSE TestRefHasher/2_segments_27_bytes
=== RUN   TestRefHasher/2_segments_28_bytes
=== PAUSE TestRefHasher/2_segments_28_bytes
=== RUN   TestRefHasher/2_segments_29_bytes
=== PAUSE TestRefHasher/2_segments_29_bytes
=== RUN   TestRefHasher/2_segments_30_bytes
=== PAUSE TestRefHasher/2_segments_30_bytes
=== RUN   TestRefHasher/2_segments_31_bytes
=== PAUSE TestRefHasher/2_segments_31_bytes
=== RUN   TestRefHasher/2_segments_32_bytes
=== PAUSE TestRefHasher/2_segments_32_bytes
=== RUN   TestRefHasher/2_segments_33_bytes
=== PAUSE TestRefHasher/2_segments_33_bytes
=== RUN   TestRefHasher/2_segments_34_bytes
=== PAUSE TestRefHasher/2_segments_34_bytes
=== RUN   TestRefHasher/2_segments_35_bytes
=== PAUSE TestRefHasher/2_segments_35_bytes
=== RUN   TestRefHasher/2_segments_36_bytes
=== PAUSE TestRefHasher/2_segments_36_bytes
=== RUN   TestRefHasher/2_segments_37_bytes
=== PAUSE TestRefHasher/2_segments_37_bytes
=== RUN   TestRefHasher/2_segments_38_bytes
=== PAUSE TestRefHasher/2_segments_38_bytes
=== RUN   TestRefHasher/2_segments_39_bytes
=== PAUSE TestRefHasher/2_segments_39_bytes
=== RUN   TestRefHasher/2_segments_40_bytes
=== PAUSE TestRefHasher/2_segments_40_bytes
=== RUN   TestRefHasher/2_segments_41_bytes
=== PAUSE TestRefHasher/2_segments_41_bytes
=== RUN   TestRefHasher/2_segments_42_bytes
=== PAUSE TestRefHasher/2_segments_42_bytes
=== RUN   TestRefHasher/2_segments_43_bytes
=== PAUSE TestRefHasher/2_segments_43_bytes
=== RUN   TestRefHasher/2_segments_44_bytes
=== PAUSE TestRefHasher/2_segments_44_bytes
=== RUN   TestRefHasher/2_segments_45_bytes
=== PAUSE TestRefHasher/2_segments_45_bytes
=== RUN   TestRefHasher/2_segments_46_bytes
=== PAUSE TestRefHasher/2_segments_46_bytes
=== RUN   TestRefHasher/2_segments_47_bytes
=== PAUSE TestRefHasher/2_segments_47_bytes
=== RUN   TestRefHasher/2_segments_48_bytes
=== PAUSE TestRefHasher/2_segments_48_bytes
=== RUN   TestRefHasher/2_segments_49_bytes
=== PAUSE TestRefHasher/2_segments_49_bytes
=== RUN   TestRefHasher/2_segments_50_bytes
=== PAUSE TestRefHasher/2_segments_50_bytes
=== RUN   TestRefHasher/2_segments_51_bytes
=== PAUSE TestRefHasher/2_segments_51_bytes
=== RUN   TestRefHasher/2_segments_52_bytes
=== PAUSE TestRefHasher/2_segments_52_bytes
=== RUN   TestRefHasher/2_segments_53_bytes
=== PAUSE TestRefHasher/2_segments_53_bytes
=== RUN   TestRefHasher/2_segments_54_bytes
=== PAUSE TestRefHasher/2_segments_54_bytes
=== RUN   TestRefHasher/2_segments_55_bytes
=== PAUSE TestRefHasher/2_segments_55_bytes
=== RUN   TestRefHasher/2_segments_56_bytes
=== PAUSE TestRefHasher/2_segments_56_bytes
=== RUN   TestRefHasher/2_segments_57_bytes
=== PAUSE TestRefHasher/2_segments_57_bytes
=== RUN   TestRefHasher/2_segments_58_bytes
=== PAUSE TestRefHasher/2_segments_58_bytes
=== RUN   TestRefHasher/2_segments_59_bytes
=== PAUSE TestRefHasher/2_segments_59_bytes
=== RUN   TestRefHasher/2_segments_60_bytes
=== PAUSE TestRefHasher/2_segments_60_bytes
=== RUN   TestRefHasher/2_segments_61_bytes
=== PAUSE TestRefHasher/2_segments_61_bytes
=== RUN   TestRefHasher/2_segments_62_bytes
=== PAUSE TestRefHasher/2_segments_62_bytes
=== RUN   TestRefHasher/2_segments_63_bytes
=== PAUSE TestRefHasher/2_segments_63_bytes
=== RUN   TestRefHasher/2_segments_64_bytes
=== PAUSE TestRefHasher/2_segments_64_bytes
=== RUN   TestRefHasher/3_segments_1_bytes
=== PAUSE TestRefHasher/3_segments_1_bytes
=== RUN   TestRefHasher/3_segments_2_bytes
=== PAUSE TestRefHasher/3_segments_2_bytes
=== RUN   TestRefHasher/3_segments_3_bytes
=== PAUSE TestRefHasher/3_segments_3_bytes
=== RUN   TestRefHasher/3_segments_4_bytes
=== PAUSE TestRefHasher/3_segments_4_bytes
=== RUN   TestRefHasher/3_segments_5_bytes
=== PAUSE TestRefHasher/3_segments_5_bytes
=== RUN   TestRefHasher/3_segments_6_bytes
=== PAUSE TestRefHasher/3_segments_6_bytes
=== RUN   TestRefHasher/3_segments_7_bytes
=== PAUSE TestRefHasher/3_segments_7_bytes
=== RUN   TestRefHasher/3_segments_8_bytes
=== PAUSE TestRefHasher/3_segments_8_bytes
=== RUN   TestRefHasher/3_segments_9_bytes
=== PAUSE TestRefHasher/3_segments_9_bytes
=== RUN   TestRefHasher/3_segments_10_bytes
=== PAUSE TestRefHasher/3_segments_10_bytes
=== RUN   TestRefHasher/3_segments_11_bytes
=== PAUSE TestRefHasher/3_segments_11_bytes
=== RUN   TestRefHasher/3_segments_12_bytes
=== PAUSE TestRefHasher/3_segments_12_bytes
=== RUN   TestRefHasher/3_segments_13_bytes
=== PAUSE TestRefHasher/3_segments_13_bytes
=== RUN   TestRefHasher/3_segments_14_bytes
=== PAUSE TestRefHasher/3_segments_14_bytes
=== RUN   TestRefHasher/3_segments_15_bytes
=== PAUSE TestRefHasher/3_segments_15_bytes
=== RUN   TestRefHasher/3_segments_16_bytes
=== PAUSE TestRefHasher/3_segments_16_bytes
=== RUN   TestRefHasher/3_segments_17_bytes
=== PAUSE TestRefHasher/3_segments_17_bytes
=== RUN   TestRefHasher/3_segments_18_bytes
=== PAUSE TestRefHasher/3_segments_18_bytes
=== RUN   TestRefHasher/3_segments_19_bytes
=== PAUSE TestRefHasher/3_segments_19_bytes
=== RUN   TestRefHasher/3_segments_20_bytes
=== PAUSE TestRefHasher/3_segments_20_bytes
=== RUN   TestRefHasher/3_segments_21_bytes
=== PAUSE TestRefHasher/3_segments_21_bytes
=== RUN   TestRefHasher/3_segments_22_bytes
=== PAUSE TestRefHasher/3_segments_22_bytes
=== RUN   TestRefHasher/3_segments_23_bytes
=== PAUSE TestRefHasher/3_segments_23_bytes
=== RUN   TestRefHasher/3_segments_24_bytes
=== PAUSE TestRefHasher/3_segments_24_bytes
=== RUN   TestRefHasher/3_segments_25_bytes
=== PAUSE TestRefHasher/3_segments_25_bytes
=== RUN   TestRefHasher/3_segments_26_bytes
=== PAUSE TestRefHasher/3_segments_26_bytes
=== RUN   TestRefHasher/3_segments_27_bytes
=== PAUSE TestRefHasher/3_segments_27_bytes
=== RUN   TestRefHasher/3_segments_28_bytes
=== PAUSE TestRefHasher/3_segments_28_bytes
=== RUN   TestRefHasher/3_segments_29_bytes
=== PAUSE TestRefHasher/3_segments_29_bytes
=== RUN   TestRefHasher/3_segments_30_bytes
=== PAUSE TestRefHasher/3_segments_30_bytes
=== RUN   TestRefHasher/3_segments_31_bytes
=== PAUSE TestRefHasher/3_segments_31_bytes
=== RUN   TestRefHasher/3_segments_32_bytes
=== PAUSE TestRefHasher/3_segments_32_bytes
=== RUN   TestRefHasher/3_segments_33_bytes
=== PAUSE TestRefHasher/3_segments_33_bytes
=== RUN   TestRefHasher/3_segments_34_bytes
=== PAUSE TestRefHasher/3_segments_34_bytes
=== RUN   TestRefHasher/3_segments_35_bytes
=== PAUSE TestRefHasher/3_segments_35_bytes
=== RUN   TestRefHasher/3_segments_36_bytes
=== PAUSE TestRefHasher/3_segments_36_bytes
=== RUN   TestRefHasher/3_segments_37_bytes
=== PAUSE TestRefHasher/3_segments_37_bytes
=== RUN   TestRefHasher/3_segments_38_bytes
=== PAUSE TestRefHasher/3_segments_38_bytes
=== RUN   TestRefHasher/3_segments_39_bytes
=== PAUSE TestRefHasher/3_segments_39_bytes
=== RUN   TestRefHasher/3_segments_40_bytes
=== PAUSE TestRefHasher/3_segments_40_bytes
=== RUN   TestRefHasher/3_segments_41_bytes
=== PAUSE TestRefHasher/3_segments_41_bytes
=== RUN   TestRefHasher/3_segments_42_bytes
=== PAUSE TestRefHasher/3_segments_42_bytes
=== RUN   TestRefHasher/3_segments_43_bytes
=== PAUSE TestRefHasher/3_segments_43_bytes
=== RUN   TestRefHasher/3_segments_44_bytes
=== PAUSE TestRefHasher/3_segments_44_bytes
=== RUN   TestRefHasher/3_segments_45_bytes
=== PAUSE TestRefHasher/3_segments_45_bytes
=== RUN   TestRefHasher/3_segments_46_bytes
=== PAUSE TestRefHasher/3_segments_46_bytes
=== RUN   TestRefHasher/3_segments_47_bytes
=== PAUSE TestRefHasher/3_segments_47_bytes
=== RUN   TestRefHasher/3_segments_48_bytes
=== PAUSE TestRefHasher/3_segments_48_bytes
=== RUN   TestRefHasher/3_segments_49_bytes
=== PAUSE TestRefHasher/3_segments_49_bytes
=== RUN   TestRefHasher/3_segments_50_bytes
=== PAUSE TestRefHasher/3_segments_50_bytes
=== RUN   TestRefHasher/3_segments_51_bytes
=== PAUSE TestRefHasher/3_segments_51_bytes
=== RUN   TestRefHasher/3_segments_52_bytes
=== PAUSE TestRefHasher/3_segments_52_bytes
=== RUN   TestRefHasher/3_segments_53_bytes
=== PAUSE TestRefHasher/3_segments_53_bytes
=== RUN   TestRefHasher/3_segments_54_bytes
=== PAUSE TestRefHasher/3_segments_54_bytes
=== RUN   TestRefHasher/3_segments_55_bytes
=== PAUSE TestRefHasher/3_segments_55_bytes
=== RUN   TestRefHasher/3_segments_56_bytes
=== PAUSE TestRefHasher/3_segments_56_bytes
=== RUN   TestRefHasher/3_segments_57_bytes
=== PAUSE TestRefHasher/3_segments_57_bytes
=== RUN   TestRefHasher/3_segments_58_bytes
=== PAUSE TestRefHasher/3_segments_58_bytes
=== RUN   TestRefHasher/3_segments_59_bytes
=== PAUSE TestRefHasher/3_segments_59_bytes
=== RUN   TestRefHasher/3_segments_60_bytes
=== PAUSE TestRefHasher/3_segments_60_bytes
=== RUN   TestRefHasher/3_segments_61_bytes
=== PAUSE TestRefHasher/3_segments_61_bytes
=== RUN   TestRefHasher/3_segments_62_bytes
=== PAUSE TestRefHasher/3_segments_62_bytes
=== RUN   TestRefHasher/3_segments_63_bytes
=== PAUSE TestRefHasher/3_segments_63_bytes
=== RUN   TestRefHasher/3_segments_64_bytes
=== PAUSE TestRefHasher/3_segments_64_bytes
=== RUN   TestRefHasher/3_segments_65_bytes
=== PAUSE TestRefHasher/3_segments_65_bytes
=== RUN   TestRefHasher/3_segments_66_bytes
=== PAUSE TestRefHasher/3_segments_66_bytes
=== RUN   TestRefHasher/3_segments_67_bytes
=== PAUSE TestRefHasher/3_segments_67_bytes
=== RUN   TestRefHasher/3_segments_68_bytes
=== PAUSE TestRefHasher/3_segments_68_bytes
=== RUN   TestRefHasher/3_segments_69_bytes
=== PAUSE TestRefHasher/3_segments_69_bytes
=== RUN   TestRefHasher/3_segments_70_bytes
=== PAUSE TestRefHasher/3_segments_70_bytes
=== RUN   TestRefHasher/3_segments_71_bytes
=== PAUSE TestRefHasher/3_segments_71_bytes
=== RUN   TestRefHasher/3_segments_72_bytes
=== PAUSE TestRefHasher/3_segments_72_bytes
=== RUN   TestRefHasher/3_segments_73_bytes
=== PAUSE TestRefHasher/3_segments_73_bytes
=== RUN   TestRefHasher/3_segments_74_bytes
=== PAUSE TestRefHasher/3_segments_74_bytes
=== RUN   TestRefHasher/3_segments_75_bytes
=== PAUSE TestRefHasher/3_segments_75_bytes
=== RUN   TestRefHasher/3_segments_76_bytes
=== PAUSE TestRefHasher/3_segments_76_bytes
=== RUN   TestRefHasher/3_segments_77_bytes
=== PAUSE TestRefHasher/3_segments_77_bytes
=== RUN   TestRefHasher/3_segments_78_bytes
=== PAUSE TestRefHasher/3_segments_78_bytes
=== RUN   TestRefHasher/3_segments_79_bytes
=== PAUSE TestRefHasher/3_segments_79_bytes
=== RUN   TestRefHasher/3_segments_80_bytes
=== PAUSE TestRefHasher/3_segments_80_bytes
=== RUN   TestRefHasher/3_segments_81_bytes
=== PAUSE TestRefHasher/3_segments_81_bytes
=== RUN   TestRefHasher/3_segments_82_bytes
=== PAUSE TestRefHasher/3_segments_82_bytes
=== RUN   TestRefHasher/3_segments_83_bytes
=== PAUSE TestRefHasher/3_segments_83_bytes
=== RUN   TestRefHasher/3_segments_84_bytes
=== PAUSE TestRefHasher/3_segments_84_bytes
=== RUN   TestRefHasher/3_segments_85_bytes
=== PAUSE TestRefHasher/3_segments_85_bytes
=== RUN   TestRefHasher/3_segments_86_bytes
=== PAUSE TestRefHasher/3_segments_86_bytes
=== RUN   TestRefHasher/3_segments_87_bytes
=== PAUSE TestRefHasher/3_segments_87_bytes
=== RUN   TestRefHasher/3_segments_88_bytes
=== PAUSE TestRefHasher/3_segments_88_bytes
=== RUN   TestRefHasher/3_segments_89_bytes
=== PAUSE TestRefHasher/3_segments_89_bytes
=== RUN   TestRefHasher/3_segments_90_bytes
=== PAUSE TestRefHasher/3_segments_90_bytes
=== RUN   TestRefHasher/3_segments_91_bytes
=== PAUSE TestRefHasher/3_segments_91_bytes
=== RUN   TestRefHasher/3_segments_92_bytes
=== PAUSE TestRefHasher/3_segments_92_bytes
=== RUN   TestRefHasher/3_segments_93_bytes
=== PAUSE TestRefHasher/3_segments_93_bytes
=== RUN   TestRefHasher/3_segments_94_bytes
=== PAUSE TestRefHasher/3_segments_94_bytes
=== RUN   TestRefHasher/3_segments_95_bytes
=== PAUSE TestRefHasher/3_segments_95_bytes
=== RUN   TestRefHasher/3_segments_96_bytes
=== PAUSE TestRefHasher/3_segments_96_bytes
=== RUN   TestRefHasher/4_segments_1_bytes
=== PAUSE TestRefHasher/4_segments_1_bytes
=== RUN   TestRefHasher/4_segments_2_bytes
=== PAUSE TestRefHasher/4_segments_2_bytes
=== RUN   TestRefHasher/4_segments_3_bytes
=== PAUSE TestRefHasher/4_segments_3_bytes
=== RUN   TestRefHasher/4_segments_4_bytes
=== PAUSE TestRefHasher/4_segments_4_bytes
=== RUN   TestRefHasher/4_segments_5_bytes
=== PAUSE TestRefHasher/4_segments_5_bytes
=== RUN   TestRefHasher/4_segments_6_bytes
=== PAUSE TestRefHasher/4_segments_6_bytes
=== RUN   TestRefHasher/4_segments_7_bytes
=== PAUSE TestRefHasher/4_segments_7_bytes
=== RUN   TestRefHasher/4_segments_8_bytes
=== PAUSE TestRefHasher/4_segments_8_bytes
=== RUN   TestRefHasher/4_segments_9_bytes
=== PAUSE TestRefHasher/4_segments_9_bytes
=== RUN   TestRefHasher/4_segments_10_bytes
=== PAUSE TestRefHasher/4_segments_10_bytes
=== RUN   TestRefHasher/4_segments_11_bytes
=== PAUSE TestRefHasher/4_segments_11_bytes
=== RUN   TestRefHasher/4_segments_12_bytes
=== PAUSE TestRefHasher/4_segments_12_bytes
=== RUN   TestRefHasher/4_segments_13_bytes
=== PAUSE TestRefHasher/4_segments_13_bytes
=== RUN   TestRefHasher/4_segments_14_bytes
=== PAUSE TestRefHasher/4_segments_14_bytes
=== RUN   TestRefHasher/4_segments_15_bytes
=== PAUSE TestRefHasher/4_segments_15_bytes
=== RUN   TestRefHasher/4_segments_16_bytes
=== PAUSE TestRefHasher/4_segments_16_bytes
=== RUN   TestRefHasher/4_segments_17_bytes
=== PAUSE TestRefHasher/4_segments_17_bytes
=== RUN   TestRefHasher/4_segments_18_bytes
=== PAUSE TestRefHasher/4_segments_18_bytes
=== RUN   TestRefHasher/4_segments_19_bytes
=== PAUSE TestRefHasher/4_segments_19_bytes
=== RUN   TestRefHasher/4_segments_20_bytes
=== PAUSE TestRefHasher/4_segments_20_bytes
=== RUN   TestRefHasher/4_segments_21_bytes
=== PAUSE TestRefHasher/4_segments_21_bytes
=== RUN   TestRefHasher/4_segments_22_bytes
=== PAUSE TestRefHasher/4_segments_22_bytes
=== RUN   TestRefHasher/4_segments_23_bytes
=== PAUSE TestRefHasher/4_segments_23_bytes
=== RUN   TestRefHasher/4_segments_24_bytes
=== PAUSE TestRefHasher/4_segments_24_bytes
=== RUN   TestRefHasher/4_segments_25_bytes
=== PAUSE TestRefHasher/4_segments_25_bytes
=== RUN   TestRefHasher/4_segments_26_bytes
=== PAUSE TestRefHasher/4_segments_26_bytes
=== RUN   TestRefHasher/4_segments_27_bytes
=== PAUSE TestRefHasher/4_segments_27_bytes
=== RUN   TestRefHasher/4_segments_28_bytes
=== PAUSE TestRefHasher/4_segments_28_bytes
=== RUN   TestRefHasher/4_segments_29_bytes
=== PAUSE TestRefHasher/4_segments_29_bytes
=== RUN   TestRefHasher/4_segments_30_bytes
=== PAUSE TestRefHasher/4_segments_30_bytes
=== RUN   TestRefHasher/4_segments_31_bytes
=== PAUSE TestRefHasher/4_segments_31_bytes
=== RUN   TestRefHasher/4_segments_32_bytes
=== PAUSE TestRefHasher/4_segments_32_bytes
=== RUN   TestRefHasher/4_segments_33_bytes
=== PAUSE TestRefHasher/4_segments_33_bytes
=== RUN   TestRefHasher/4_segments_34_bytes
=== PAUSE TestRefHasher/4_segments_34_bytes
=== RUN   TestRefHasher/4_segments_35_bytes
=== PAUSE TestRefHasher/4_segments_35_bytes
=== RUN   TestRefHasher/4_segments_36_bytes
=== PAUSE TestRefHasher/4_segments_36_bytes
=== RUN   TestRefHasher/4_segments_37_bytes
=== PAUSE TestRefHasher/4_segments_37_bytes
=== RUN   TestRefHasher/4_segments_38_bytes
=== PAUSE TestRefHasher/4_segments_38_bytes
=== RUN   TestRefHasher/4_segments_39_bytes
=== PAUSE TestRefHasher/4_segments_39_bytes
=== RUN   TestRefHasher/4_segments_40_bytes
=== PAUSE TestRefHasher/4_segments_40_bytes
=== RUN   TestRefHasher/4_segments_41_bytes
=== PAUSE TestRefHasher/4_segments_41_bytes
=== RUN   TestRefHasher/4_segments_42_bytes
=== PAUSE TestRefHasher/4_segments_42_bytes
=== RUN   TestRefHasher/4_segments_43_bytes
=== PAUSE TestRefHasher/4_segments_43_bytes
=== RUN   TestRefHasher/4_segments_44_bytes
=== PAUSE TestRefHasher/4_segments_44_bytes
=== RUN   TestRefHasher/4_segments_45_bytes
=== PAUSE TestRefHasher/4_segments_45_bytes
=== RUN   TestRefHasher/4_segments_46_bytes
=== PAUSE TestRefHasher/4_segments_46_bytes
=== RUN   TestRefHasher/4_segments_47_bytes
=== PAUSE TestRefHasher/4_segments_47_bytes
=== RUN   TestRefHasher/4_segments_48_bytes
=== PAUSE TestRefHasher/4_segments_48_bytes
=== RUN   TestRefHasher/4_segments_49_bytes
=== PAUSE TestRefHasher/4_segments_49_bytes
=== RUN   TestRefHasher/4_segments_50_bytes
=== PAUSE TestRefHasher/4_segments_50_bytes
=== RUN   TestRefHasher/4_segments_51_bytes
=== PAUSE TestRefHasher/4_segments_51_bytes
=== RUN   TestRefHasher/4_segments_52_bytes
=== PAUSE TestRefHasher/4_segments_52_bytes
=== RUN   TestRefHasher/4_segments_53_bytes
=== PAUSE TestRefHasher/4_segments_53_bytes
=== RUN   TestRefHasher/4_segments_54_bytes
=== PAUSE TestRefHasher/4_segments_54_bytes
=== RUN   TestRefHasher/4_segments_55_bytes
=== PAUSE TestRefHasher/4_segments_55_bytes
=== RUN   TestRefHasher/4_segments_56_bytes
=== PAUSE TestRefHasher/4_segments_56_bytes
=== RUN   TestRefHasher/4_segments_57_bytes
=== PAUSE TestRefHasher/4_segments_57_bytes
=== RUN   TestRefHasher/4_segments_58_bytes
=== PAUSE TestRefHasher/4_segments_58_bytes
=== RUN   TestRefHasher/4_segments_59_bytes
=== PAUSE TestRefHasher/4_segments_59_bytes
=== RUN   TestRefHasher/4_segments_60_bytes
=== PAUSE TestRefHasher/4_segments_60_bytes
=== RUN   TestRefHasher/4_segments_61_bytes
=== PAUSE TestRefHasher/4_segments_61_bytes
=== RUN   TestRefHasher/4_segments_62_bytes
=== PAUSE TestRefHasher/4_segments_62_bytes
=== RUN   TestRefHasher/4_segments_63_bytes
=== PAUSE TestRefHasher/4_segments_63_bytes
=== RUN   TestRefHasher/4_segments_64_bytes
=== PAUSE TestRefHasher/4_segments_64_bytes
=== RUN   TestRefHasher/4_segments_65_bytes
=== PAUSE TestRefHasher/4_segments_65_bytes
=== RUN   TestRefHasher/4_segments_66_bytes
=== PAUSE TestRefHasher/4_segments_66_bytes
=== RUN   TestRefHasher/4_segments_67_bytes
=== PAUSE TestRefHasher/4_segments_67_bytes
=== RUN   TestRefHasher/4_segments_68_bytes
=== PAUSE TestRefHasher/4_segments_68_bytes
=== RUN   TestRefHasher/4_segments_69_bytes
=== PAUSE TestRefHasher/4_segments_69_bytes
=== RUN   TestRefHasher/4_segments_70_bytes
=== PAUSE TestRefHasher/4_segments_70_bytes
=== RUN   TestRefHasher/4_segments_71_bytes
=== PAUSE TestRefHasher/4_segments_71_bytes
=== RUN   TestRefHasher/4_segments_72_bytes
=== PAUSE TestRefHasher/4_segments_72_bytes
=== RUN   TestRefHasher/4_segments_73_bytes
=== PAUSE TestRefHasher/4_segments_73_bytes
=== RUN   TestRefHasher/4_segments_74_bytes
=== PAUSE TestRefHasher/4_segments_74_bytes
=== RUN   TestRefHasher/4_segments_75_bytes
=== PAUSE TestRefHasher/4_segments_75_bytes
=== RUN   TestRefHasher/4_segments_76_bytes
=== PAUSE TestRefHasher/4_segments_76_bytes
=== RUN   TestRefHasher/4_segments_77_bytes
=== PAUSE TestRefHasher/4_segments_77_bytes
=== RUN   TestRefHasher/4_segments_78_bytes
=== PAUSE TestRefHasher/4_segments_78_bytes
=== RUN   TestRefHasher/4_segments_79_bytes
=== PAUSE TestRefHasher/4_segments_79_bytes
=== RUN   TestRefHasher/4_segments_80_bytes
=== PAUSE TestRefHasher/4_segments_80_bytes
=== RUN   TestRefHasher/4_segments_81_bytes
=== PAUSE TestRefHasher/4_segments_81_bytes
=== RUN   TestRefHasher/4_segments_82_bytes
=== PAUSE TestRefHasher/4_segments_82_bytes
=== RUN   TestRefHasher/4_segments_83_bytes
=== PAUSE TestRefHasher/4_segments_83_bytes
=== RUN   TestRefHasher/4_segments_84_bytes
=== PAUSE TestRefHasher/4_segments_84_bytes
=== RUN   TestRefHasher/4_segments_85_bytes
=== PAUSE TestRefHasher/4_segments_85_bytes
=== RUN   TestRefHasher/4_segments_86_bytes
=== PAUSE TestRefHasher/4_segments_86_bytes
=== RUN   TestRefHasher/4_segments_87_bytes
=== PAUSE TestRefHasher/4_segments_87_bytes
=== RUN   TestRefHasher/4_segments_88_bytes
=== PAUSE TestRefHasher/4_segments_88_bytes
=== RUN   TestRefHasher/4_segments_89_bytes
=== PAUSE TestRefHasher/4_segments_89_bytes
=== RUN   TestRefHasher/4_segments_90_bytes
=== PAUSE TestRefHasher/4_segments_90_bytes
=== RUN   TestRefHasher/4_segments_91_bytes
=== PAUSE TestRefHasher/4_segments_91_bytes
=== RUN   TestRefHasher/4_segments_92_bytes
=== PAUSE TestRefHasher/4_segments_92_bytes
=== RUN   TestRefHasher/4_segments_93_bytes
=== PAUSE TestRefHasher/4_segments_93_bytes
=== RUN   TestRefHasher/4_segments_94_bytes
=== PAUSE TestRefHasher/4_segments_94_bytes
=== RUN   TestRefHasher/4_segments_95_bytes
=== PAUSE TestRefHasher/4_segments_95_bytes
=== RUN   TestRefHasher/4_segments_96_bytes
=== PAUSE TestRefHasher/4_segments_96_bytes
=== RUN   TestRefHasher/4_segments_97_bytes
=== PAUSE TestRefHasher/4_segments_97_bytes
=== RUN   TestRefHasher/4_segments_98_bytes
=== PAUSE TestRefHasher/4_segments_98_bytes
=== RUN   TestRefHasher/4_segments_99_bytes
=== PAUSE TestRefHasher/4_segments_99_bytes
=== RUN   TestRefHasher/4_segments_100_bytes
=== PAUSE TestRefHasher/4_segments_100_bytes
=== RUN   TestRefHasher/4_segments_101_bytes
=== PAUSE TestRefHasher/4_segments_101_bytes
=== RUN   TestRefHasher/4_segments_102_bytes
=== PAUSE TestRefHasher/4_segments_102_bytes
=== RUN   TestRefHasher/4_segments_103_bytes
=== PAUSE TestRefHasher/4_segments_103_bytes
=== RUN   TestRefHasher/4_segments_104_bytes
=== PAUSE TestRefHasher/4_segments_104_bytes
=== RUN   TestRefHasher/4_segments_105_bytes
=== PAUSE TestRefHasher/4_segments_105_bytes
=== RUN   TestRefHasher/4_segments_106_bytes
=== PAUSE TestRefHasher/4_segments_106_bytes
=== RUN   TestRefHasher/4_segments_107_bytes
=== PAUSE TestRefHasher/4_segments_107_bytes
=== RUN   TestRefHasher/4_segments_108_bytes
=== PAUSE TestRefHasher/4_segments_108_bytes
=== RUN   TestRefHasher/4_segments_109_bytes
=== PAUSE TestRefHasher/4_segments_109_bytes
=== RUN   TestRefHasher/4_segments_110_bytes
=== PAUSE TestRefHasher/4_segments_110_bytes
=== RUN   TestRefHasher/4_segments_111_bytes
=== PAUSE TestRefHasher/4_segments_111_bytes
=== RUN   TestRefHasher/4_segments_112_bytes
=== PAUSE TestRefHasher/4_segments_112_bytes
=== RUN   TestRefHasher/4_segments_113_bytes
=== PAUSE TestRefHasher/4_segments_113_bytes
=== RUN   TestRefHasher/4_segments_114_bytes
=== PAUSE TestRefHasher/4_segments_114_bytes
=== RUN   TestRefHasher/4_segments_115_bytes
=== PAUSE TestRefHasher/4_segments_115_bytes
=== RUN   TestRefHasher/4_segments_116_bytes
=== PAUSE TestRefHasher/4_segments_116_bytes
=== RUN   TestRefHasher/4_segments_117_bytes
=== PAUSE TestRefHasher/4_segments_117_bytes
=== RUN   TestRefHasher/4_segments_118_bytes
=== PAUSE TestRefHasher/4_segments_118_bytes
=== RUN   TestRefHasher/4_segments_119_bytes
=== PAUSE TestRefHasher/4_segments_119_bytes
=== RUN   TestRefHasher/4_segments_120_bytes
=== PAUSE TestRefHasher/4_segments_120_bytes
=== RUN   TestRefHasher/4_segments_121_bytes
=== PAUSE TestRefHasher/4_segments_121_bytes
=== RUN   TestRefHasher/4_segments_122_bytes
=== PAUSE TestRefHasher/4_segments_122_bytes
=== RUN   TestRefHasher/4_segments_123_bytes
=== PAUSE TestRefHasher/4_segments_123_bytes
=== RUN   TestRefHasher/4_segments_124_bytes
=== PAUSE TestRefHasher/4_segments_124_bytes
=== RUN   TestRefHasher/4_segments_125_bytes
=== PAUSE TestRefHasher/4_segments_125_bytes
=== RUN   TestRefHasher/4_segments_126_bytes
=== PAUSE TestRefHasher/4_segments_126_bytes
=== RUN   TestRefHasher/4_segments_127_bytes
=== PAUSE TestRefHasher/4_segments_127_bytes
=== RUN   TestRefHasher/4_segments_128_bytes
=== PAUSE TestRefHasher/4_segments_128_bytes
=== RUN   TestRefHasher/5_segments_1_bytes
=== PAUSE TestRefHasher/5_segments_1_bytes
=== RUN   TestRefHasher/5_segments_2_bytes
=== PAUSE TestRefHasher/5_segments_2_bytes
=== RUN   TestRefHasher/5_segments_3_bytes
=== PAUSE TestRefHasher/5_segments_3_bytes
=== RUN   TestRefHasher/5_segments_4_bytes
=== PAUSE TestRefHasher/5_segments_4_bytes
=== RUN   TestRefHasher/5_segments_5_bytes
=== PAUSE TestRefHasher/5_segments_5_bytes
=== RUN   TestRefHasher/5_segments_6_bytes
=== PAUSE TestRefHasher/5_segments_6_bytes
=== RUN   TestRefHasher/5_segments_7_bytes
=== PAUSE TestRefHasher/5_segments_7_bytes
=== RUN   TestRefHasher/5_segments_8_bytes
=== PAUSE TestRefHasher/5_segments_8_bytes
=== RUN   TestRefHasher/5_segments_9_bytes
=== PAUSE TestRefHasher/5_segments_9_bytes
=== RUN   TestRefHasher/5_segments_10_bytes
=== PAUSE TestRefHasher/5_segments_10_bytes
=== RUN   TestRefHasher/5_segments_11_bytes
=== PAUSE TestRefHasher/5_segments_11_bytes
=== RUN   TestRefHasher/5_segments_12_bytes
=== PAUSE TestRefHasher/5_segments_12_bytes
=== RUN   TestRefHasher/5_segments_13_bytes
=== PAUSE TestRefHasher/5_segments_13_bytes
=== RUN   TestRefHasher/5_segments_14_bytes
=== PAUSE TestRefHasher/5_segments_14_bytes
=== RUN   TestRefHasher/5_segments_15_bytes
=== PAUSE TestRefHasher/5_segments_15_bytes
=== RUN   TestRefHasher/5_segments_16_bytes
=== PAUSE TestRefHasher/5_segments_16_bytes
=== RUN   TestRefHasher/5_segments_17_bytes
=== PAUSE TestRefHasher/5_segments_17_bytes
=== RUN   TestRefHasher/5_segments_18_bytes
=== PAUSE TestRefHasher/5_segments_18_bytes
=== RUN   TestRefHasher/5_segments_19_bytes
=== PAUSE TestRefHasher/5_segments_19_bytes
=== RUN   TestRefHasher/5_segments_20_bytes
=== PAUSE TestRefHasher/5_segments_20_bytes
=== RUN   TestRefHasher/5_segments_21_bytes
=== PAUSE TestRefHasher/5_segments_21_bytes
=== RUN   TestRefHasher/5_segments_22_bytes
=== PAUSE TestRefHasher/5_segments_22_bytes
=== RUN   TestRefHasher/5_segments_23_bytes
=== PAUSE TestRefHasher/5_segments_23_bytes
=== RUN   TestRefHasher/5_segments_24_bytes
=== PAUSE TestRefHasher/5_segments_24_bytes
=== RUN   TestRefHasher/5_segments_25_bytes
=== PAUSE TestRefHasher/5_segments_25_bytes
=== RUN   TestRefHasher/5_segments_26_bytes
=== PAUSE TestRefHasher/5_segments_26_bytes
=== RUN   TestRefHasher/5_segments_27_bytes
=== PAUSE TestRefHasher/5_segments_27_bytes
=== RUN   TestRefHasher/5_segments_28_bytes
=== PAUSE TestRefHasher/5_segments_28_bytes
=== RUN   TestRefHasher/5_segments_29_bytes
=== PAUSE TestRefHasher/5_segments_29_bytes
=== RUN   TestRefHasher/5_segments_30_bytes
=== PAUSE TestRefHasher/5_segments_30_bytes
=== RUN   TestRefHasher/5_segments_31_bytes
=== PAUSE TestRefHasher/5_segments_31_bytes
=== RUN   TestRefHasher/5_segments_32_bytes
=== PAUSE TestRefHasher/5_segments_32_bytes
=== RUN   TestRefHasher/5_segments_33_bytes
=== PAUSE TestRefHasher/5_segments_33_bytes
=== RUN   TestRefHasher/5_segments_34_bytes
=== PAUSE TestRefHasher/5_segments_34_bytes
=== RUN   TestRefHasher/5_segments_35_bytes
=== PAUSE TestRefHasher/5_segments_35_bytes
=== RUN   TestRefHasher/5_segments_36_bytes
=== PAUSE TestRefHasher/5_segments_36_bytes
=== RUN   TestRefHasher/5_segments_37_bytes
=== PAUSE TestRefHasher/5_segments_37_bytes
=== RUN   TestRefHasher/5_segments_38_bytes
=== PAUSE TestRefHasher/5_segments_38_bytes
=== RUN   TestRefHasher/5_segments_39_bytes
=== PAUSE TestRefHasher/5_segments_39_bytes
=== RUN   TestRefHasher/5_segments_40_bytes
=== PAUSE TestRefHasher/5_segments_40_bytes
=== RUN   TestRefHasher/5_segments_41_bytes
=== PAUSE TestRefHasher/5_segments_41_bytes
=== RUN   TestRefHasher/5_segments_42_bytes
=== PAUSE TestRefHasher/5_segments_42_bytes
=== RUN   TestRefHasher/5_segments_43_bytes
=== PAUSE TestRefHasher/5_segments_43_bytes
=== RUN   TestRefHasher/5_segments_44_bytes
=== PAUSE TestRefHasher/5_segments_44_bytes
=== RUN   TestRefHasher/5_segments_45_bytes
=== PAUSE TestRefHasher/5_segments_45_bytes
=== RUN   TestRefHasher/5_segments_46_bytes
=== PAUSE TestRefHasher/5_segments_46_bytes
=== RUN   TestRefHasher/5_segments_47_bytes
=== PAUSE TestRefHasher/5_segments_47_bytes
=== RUN   TestRefHasher/5_segments_48_bytes
=== PAUSE TestRefHasher/5_segments_48_bytes
=== RUN   TestRefHasher/5_segments_49_bytes
=== PAUSE TestRefHasher/5_segments_49_bytes
=== RUN   TestRefHasher/5_segments_50_bytes
=== PAUSE TestRefHasher/5_segments_50_bytes
=== RUN   TestRefHasher/5_segments_51_bytes
=== PAUSE TestRefHasher/5_segments_51_bytes
=== RUN   TestRefHasher/5_segments_52_bytes
=== PAUSE TestRefHasher/5_segments_52_bytes
=== RUN   TestRefHasher/5_segments_53_bytes
=== PAUSE TestRefHasher/5_segments_53_bytes
=== RUN   TestRefHasher/5_segments_54_bytes
=== PAUSE TestRefHasher/5_segments_54_bytes
=== RUN   TestRefHasher/5_segments_55_bytes
=== PAUSE TestRefHasher/5_segments_55_bytes
=== RUN   TestRefHasher/5_segments_56_bytes
=== PAUSE TestRefHasher/5_segments_56_bytes
=== RUN   TestRefHasher/5_segments_57_bytes
=== PAUSE TestRefHasher/5_segments_57_bytes
=== RUN   TestRefHasher/5_segments_58_bytes
=== PAUSE TestRefHasher/5_segments_58_bytes
=== RUN   TestRefHasher/5_segments_59_bytes
=== PAUSE TestRefHasher/5_segments_59_bytes
=== RUN   TestRefHasher/5_segments_60_bytes
=== PAUSE TestRefHasher/5_segments_60_bytes
=== RUN   TestRefHasher/5_segments_61_bytes
=== PAUSE TestRefHasher/5_segments_61_bytes
=== RUN   TestRefHasher/5_segments_62_bytes
=== PAUSE TestRefHasher/5_segments_62_bytes
=== RUN   TestRefHasher/5_segments_63_bytes
=== PAUSE TestRefHasher/5_segments_63_bytes
=== RUN   TestRefHasher/5_segments_64_bytes
=== PAUSE TestRefHasher/5_segments_64_bytes
=== RUN   TestRefHasher/5_segments_65_bytes
=== PAUSE TestRefHasher/5_segments_65_bytes
=== RUN   TestRefHasher/5_segments_66_bytes
=== PAUSE TestRefHasher/5_segments_66_bytes
=== RUN   TestRefHasher/5_segments_67_bytes
=== PAUSE TestRefHasher/5_segments_67_bytes
=== RUN   TestRefHasher/5_segments_68_bytes
=== PAUSE TestRefHasher/5_segments_68_bytes
=== RUN   TestRefHasher/5_segments_69_bytes
=== PAUSE TestRefHasher/5_segments_69_bytes
=== RUN   TestRefHasher/5_segments_70_bytes
=== PAUSE TestRefHasher/5_segments_70_bytes
=== RUN   TestRefHasher/5_segments_71_bytes
=== PAUSE TestRefHasher/5_segments_71_bytes
=== RUN   TestRefHasher/5_segments_72_bytes
=== PAUSE TestRefHasher/5_segments_72_bytes
=== RUN   TestRefHasher/5_segments_73_bytes
=== PAUSE TestRefHasher/5_segments_73_bytes
=== RUN   TestRefHasher/5_segments_74_bytes
=== PAUSE TestRefHasher/5_segments_74_bytes
=== RUN   TestRefHasher/5_segments_75_bytes
=== PAUSE TestRefHasher/5_segments_75_bytes
=== RUN   TestRefHasher/5_segments_76_bytes
=== PAUSE TestRefHasher/5_segments_76_bytes
=== RUN   TestRefHasher/5_segments_77_bytes
=== PAUSE TestRefHasher/5_segments_77_bytes
=== RUN   TestRefHasher/5_segments_78_bytes
=== PAUSE TestRefHasher/5_segments_78_bytes
=== RUN   TestRefHasher/5_segments_79_bytes
=== PAUSE TestRefHasher/5_segments_79_bytes
=== RUN   TestRefHasher/5_segments_80_bytes
=== PAUSE TestRefHasher/5_segments_80_bytes
=== RUN   TestRefHasher/5_segments_81_bytes
=== PAUSE TestRefHasher/5_segments_81_bytes
=== RUN   TestRefHasher/5_segments_82_bytes
=== PAUSE TestRefHasher/5_segments_82_bytes
=== RUN   TestRefHasher/5_segments_83_bytes
=== PAUSE TestRefHasher/5_segments_83_bytes
=== RUN   TestRefHasher/5_segments_84_bytes
=== PAUSE TestRefHasher/5_segments_84_bytes
=== RUN   TestRefHasher/5_segments_85_bytes
=== PAUSE TestRefHasher/5_segments_85_bytes
=== RUN   TestRefHasher/5_segments_86_bytes
=== PAUSE TestRefHasher/5_segments_86_bytes
=== RUN   TestRefHasher/5_segments_87_bytes
=== PAUSE TestRefHasher/5_segments_87_bytes
=== RUN   TestRefHasher/5_segments_88_bytes
=== PAUSE TestRefHasher/5_segments_88_bytes
=== RUN   TestRefHasher/5_segments_89_bytes
=== PAUSE TestRefHasher/5_segments_89_bytes
=== RUN   TestRefHasher/5_segments_90_bytes
=== PAUSE TestRefHasher/5_segments_90_bytes
=== RUN   TestRefHasher/5_segments_91_bytes
=== PAUSE TestRefHasher/5_segments_91_bytes
=== RUN   TestRefHasher/5_segments_92_bytes
=== PAUSE TestRefHasher/5_segments_92_bytes
=== RUN   TestRefHasher/5_segments_93_bytes
=== PAUSE TestRefHasher/5_segments_93_bytes
=== RUN   TestRefHasher/5_segments_94_bytes
=== PAUSE TestRefHasher/5_segments_94_bytes
=== RUN   TestRefHasher/5_segments_95_bytes
=== PAUSE TestRefHasher/5_segments_95_bytes
=== RUN   TestRefHasher/5_segments_96_bytes
=== PAUSE TestRefHasher/5_segments_96_bytes
=== RUN   TestRefHasher/5_segments_97_bytes
=== PAUSE TestRefHasher/5_segments_97_bytes
=== RUN   TestRefHasher/5_segments_98_bytes
=== PAUSE TestRefHasher/5_segments_98_bytes
=== RUN   TestRefHasher/5_segments_99_bytes
=== PAUSE TestRefHasher/5_segments_99_bytes
=== RUN   TestRefHasher/5_segments_100_bytes
=== PAUSE TestRefHasher/5_segments_100_bytes
=== RUN   TestRefHasher/5_segments_101_bytes
=== PAUSE TestRefHasher/5_segments_101_bytes
=== RUN   TestRefHasher/5_segments_102_bytes
=== PAUSE TestRefHasher/5_segments_102_bytes
=== RUN   TestRefHasher/5_segments_103_bytes
=== PAUSE TestRefHasher/5_segments_103_bytes
=== RUN   TestRefHasher/5_segments_104_bytes
=== PAUSE TestRefHasher/5_segments_104_bytes
=== RUN   TestRefHasher/5_segments_105_bytes
=== PAUSE TestRefHasher/5_segments_105_bytes
=== RUN   TestRefHasher/5_segments_106_bytes
=== PAUSE TestRefHasher/5_segments_106_bytes
=== RUN   TestRefHasher/5_segments_107_bytes
=== PAUSE TestRefHasher/5_segments_107_bytes
=== RUN   TestRefHasher/5_segments_108_bytes
=== PAUSE TestRefHasher/5_segments_108_bytes
=== RUN   TestRefHasher/5_segments_109_bytes
=== PAUSE TestRefHasher/5_segments_109_bytes
=== RUN   TestRefHasher/5_segments_110_bytes
=== PAUSE TestRefHasher/5_segments_110_bytes
=== RUN   TestRefHasher/5_segments_111_bytes
=== PAUSE TestRefHasher/5_segments_111_bytes
=== RUN   TestRefHasher/5_segments_112_bytes
=== PAUSE TestRefHasher/5_segments_112_bytes
=== RUN   TestRefHasher/5_segments_113_bytes
=== PAUSE TestRefHasher/5_segments_113_bytes
=== RUN   TestRefHasher/5_segments_114_bytes
=== PAUSE TestRefHasher/5_segments_114_bytes
=== RUN   TestRefHasher/5_segments_115_bytes
=== PAUSE TestRefHasher/5_segments_115_bytes
=== RUN   TestRefHasher/5_segments_116_bytes
=== PAUSE TestRefHasher/5_segments_116_bytes
=== RUN   TestRefHasher/5_segments_117_bytes
=== PAUSE TestRefHasher/5_segments_117_bytes
=== RUN   TestRefHasher/5_segments_118_bytes
=== PAUSE TestRefHasher/5_segments_118_bytes
=== RUN   TestRefHasher/5_segments_119_bytes
=== PAUSE TestRefHasher/5_segments_119_bytes
=== RUN   TestRefHasher/5_segments_120_bytes
=== PAUSE TestRefHasher/5_segments_120_bytes
=== RUN   TestRefHasher/5_segments_121_bytes
=== PAUSE TestRefHasher/5_segments_121_bytes
=== RUN   TestRefHasher/5_segments_122_bytes
=== PAUSE TestRefHasher/5_segments_122_bytes
=== RUN   TestRefHasher/5_segments_123_bytes
=== PAUSE TestRefHasher/5_segments_123_bytes
=== RUN   TestRefHasher/5_segments_124_bytes
=== PAUSE TestRefHasher/5_segments_124_bytes
=== RUN   TestRefHasher/5_segments_125_bytes
=== PAUSE TestRefHasher/5_segments_125_bytes
=== RUN   TestRefHasher/5_segments_126_bytes
=== PAUSE TestRefHasher/5_segments_126_bytes
=== RUN   TestRefHasher/5_segments_127_bytes
=== PAUSE TestRefHasher/5_segments_127_bytes
=== RUN   TestRefHasher/5_segments_128_bytes
=== PAUSE TestRefHasher/5_segments_128_bytes
=== RUN   TestRefHasher/5_segments_129_bytes
=== PAUSE TestRefHasher/5_segments_129_bytes
=== RUN   TestRefHasher/5_segments_130_bytes
=== PAUSE TestRefHasher/5_segments_130_bytes
=== RUN   TestRefHasher/5_segments_131_bytes
=== PAUSE TestRefHasher/5_segments_131_bytes
=== RUN   TestRefHasher/5_segments_132_bytes
=== PAUSE TestRefHasher/5_segments_132_bytes
=== RUN   TestRefHasher/5_segments_133_bytes
=== PAUSE TestRefHasher/5_segments_133_bytes
=== RUN   TestRefHasher/5_segments_134_bytes
=== PAUSE TestRefHasher/5_segments_134_bytes
=== RUN   TestRefHasher/5_segments_135_bytes
=== PAUSE TestRefHasher/5_segments_135_bytes
=== RUN   TestRefHasher/5_segments_136_bytes
=== PAUSE TestRefHasher/5_segments_136_bytes
=== RUN   TestRefHasher/5_segments_137_bytes
=== PAUSE TestRefHasher/5_segments_137_bytes
=== RUN   TestRefHasher/5_segments_138_bytes
=== PAUSE TestRefHasher/5_segments_138_bytes
=== RUN   TestRefHasher/5_segments_139_bytes
=== PAUSE TestRefHasher/5_segments_139_bytes
=== RUN   TestRefHasher/5_segments_140_bytes
=== PAUSE TestRefHasher/5_segments_140_bytes
=== RUN   TestRefHasher/5_segments_141_bytes
=== PAUSE TestRefHasher/5_segments_141_bytes
=== RUN   TestRefHasher/5_segments_142_bytes
=== PAUSE TestRefHasher/5_segments_142_bytes
=== RUN   TestRefHasher/5_segments_143_bytes
=== PAUSE TestRefHasher/5_segments_143_bytes
=== RUN   TestRefHasher/5_segments_144_bytes
=== PAUSE TestRefHasher/5_segments_144_bytes
=== RUN   TestRefHasher/5_segments_145_bytes
=== PAUSE TestRefHasher/5_segments_145_bytes
=== RUN   TestRefHasher/5_segments_146_bytes
=== PAUSE TestRefHasher/5_segments_146_bytes
=== RUN   TestRefHasher/5_segments_147_bytes
=== PAUSE TestRefHasher/5_segments_147_bytes
=== RUN   TestRefHasher/5_segments_148_bytes
=== PAUSE TestRefHasher/5_segments_148_bytes
=== RUN   TestRefHasher/5_segments_149_bytes
=== PAUSE TestRefHasher/5_segments_149_bytes
=== RUN   TestRefHasher/5_segments_150_bytes
=== PAUSE TestRefHasher/5_segments_150_bytes
=== RUN   TestRefHasher/5_segments_151_bytes
=== PAUSE TestRefHasher/5_segments_151_bytes
=== RUN   TestRefHasher/5_segments_152_bytes
=== PAUSE TestRefHasher/5_segments_152_bytes
=== RUN   TestRefHasher/5_segments_153_bytes
=== PAUSE TestRefHasher/5_segments_153_bytes
=== RUN   TestRefHasher/5_segments_154_bytes
=== PAUSE TestRefHasher/5_segments_154_bytes
=== RUN   TestRefHasher/5_segments_155_bytes
=== PAUSE TestRefHasher/5_segments_155_bytes
=== RUN   TestRefHasher/5_segments_156_bytes
=== PAUSE TestRefHasher/5_segments_156_bytes
=== RUN   TestRefHasher/5_segments_157_bytes
=== PAUSE TestRefHasher/5_segments_157_bytes
=== RUN   TestRefHasher/5_segments_158_bytes
=== PAUSE TestRefHasher/5_segments_158_bytes
=== RUN   TestRefHasher/5_segments_159_bytes
=== PAUSE TestRefHasher/5_segments_159_bytes
=== RUN   TestRefHasher/5_segments_160_bytes
=== PAUSE TestRefHasher/5_segments_160_bytes
=== RUN   TestRefHasher/6_segments_1_bytes
=== PAUSE TestRefHasher/6_segments_1_bytes
=== RUN   TestRefHasher/6_segments_2_bytes
=== PAUSE TestRefHasher/6_segments_2_bytes
=== RUN   TestRefHasher/6_segments_3_bytes
=== PAUSE TestRefHasher/6_segments_3_bytes
=== RUN   TestRefHasher/6_segments_4_bytes
=== PAUSE TestRefHasher/6_segments_4_bytes
=== RUN   TestRefHasher/6_segments_5_bytes
=== PAUSE TestRefHasher/6_segments_5_bytes
=== RUN   TestRefHasher/6_segments_6_bytes
=== PAUSE TestRefHasher/6_segments_6_bytes
=== RUN   TestRefHasher/6_segments_7_bytes
=== PAUSE TestRefHasher/6_segments_7_bytes
=== RUN   TestRefHasher/6_segments_8_bytes
=== PAUSE TestRefHasher/6_segments_8_bytes
=== RUN   TestRefHasher/6_segments_9_bytes
=== PAUSE TestRefHasher/6_segments_9_bytes
=== RUN   TestRefHasher/6_segments_10_bytes
=== PAUSE TestRefHasher/6_segments_10_bytes
=== RUN   TestRefHasher/6_segments_11_bytes
=== PAUSE TestRefHasher/6_segments_11_bytes
=== RUN   TestRefHasher/6_segments_12_bytes
=== PAUSE TestRefHasher/6_segments_12_bytes
=== RUN   TestRefHasher/6_segments_13_bytes
=== PAUSE TestRefHasher/6_segments_13_bytes
=== RUN   TestRefHasher/6_segments_14_bytes
=== PAUSE TestRefHasher/6_segments_14_bytes
=== RUN   TestRefHasher/6_segments_15_bytes
=== PAUSE TestRefHasher/6_segments_15_bytes
=== RUN   TestRefHasher/6_segments_16_bytes
=== PAUSE TestRefHasher/6_segments_16_bytes
=== RUN   TestRefHasher/6_segments_17_bytes
=== PAUSE TestRefHasher/6_segments_17_bytes
=== RUN   TestRefHasher/6_segments_18_bytes
=== PAUSE TestRefHasher/6_segments_18_bytes
=== RUN   TestRefHasher/6_segments_19_bytes
=== PAUSE TestRefHasher/6_segments_19_bytes
=== RUN   TestRefHasher/6_segments_20_bytes
=== PAUSE TestRefHasher/6_segments_20_bytes
=== RUN   TestRefHasher/6_segments_21_bytes
=== PAUSE TestRefHasher/6_segments_21_bytes
=== RUN   TestRefHasher/6_segments_22_bytes
=== PAUSE TestRefHasher/6_segments_22_bytes
=== RUN   TestRefHasher/6_segments_23_bytes
=== PAUSE TestRefHasher/6_segments_23_bytes
=== RUN   TestRefHasher/6_segments_24_bytes
=== PAUSE TestRefHasher/6_segments_24_bytes
=== RUN   TestRefHasher/6_segments_25_bytes
=== PAUSE TestRefHasher/6_segments_25_bytes
=== RUN   TestRefHasher/6_segments_26_bytes
=== PAUSE TestRefHasher/6_segments_26_bytes
=== RUN   TestRefHasher/6_segments_27_bytes
=== PAUSE TestRefHasher/6_segments_27_bytes
=== RUN   TestRefHasher/6_segments_28_bytes
=== PAUSE TestRefHasher/6_segments_28_bytes
=== RUN   TestRefHasher/6_segments_29_bytes
=== PAUSE TestRefHasher/6_segments_29_bytes
=== RUN   TestRefHasher/6_segments_30_bytes
=== PAUSE TestRefHasher/6_segments_30_bytes
=== RUN   TestRefHasher/6_segments_31_bytes
=== PAUSE TestRefHasher/6_segments_31_bytes
=== RUN   TestRefHasher/6_segments_32_bytes
=== PAUSE TestRefHasher/6_segments_32_bytes
=== RUN   TestRefHasher/6_segments_33_bytes
=== PAUSE TestRefHasher/6_segments_33_bytes
=== RUN   TestRefHasher/6_segments_34_bytes
=== PAUSE TestRefHasher/6_segments_34_bytes
=== RUN   TestRefHasher/6_segments_35_bytes
=== PAUSE TestRefHasher/6_segments_35_bytes
=== RUN   TestRefHasher/6_segments_36_bytes
=== PAUSE TestRefHasher/6_segments_36_bytes
=== RUN   TestRefHasher/6_segments_37_bytes
=== PAUSE TestRefHasher/6_segments_37_bytes
=== RUN   TestRefHasher/6_segments_38_bytes
=== PAUSE TestRefHasher/6_segments_38_bytes
=== RUN   TestRefHasher/6_segments_39_bytes
=== PAUSE TestRefHasher/6_segments_39_bytes
=== RUN   TestRefHasher/6_segments_40_bytes
=== PAUSE TestRefHasher/6_segments_40_bytes
=== RUN   TestRefHasher/6_segments_41_bytes
=== PAUSE TestRefHasher/6_segments_41_bytes
=== RUN   TestRefHasher/6_segments_42_bytes
=== PAUSE TestRefHasher/6_segments_42_bytes
=== RUN   TestRefHasher/6_segments_43_bytes
=== PAUSE TestRefHasher/6_segments_43_bytes
=== RUN   TestRefHasher/6_segments_44_bytes
=== PAUSE TestRefHasher/6_segments_44_bytes
=== RUN   TestRefHasher/6_segments_45_bytes
=== PAUSE TestRefHasher/6_segments_45_bytes
=== RUN   TestRefHasher/6_segments_46_bytes
=== PAUSE TestRefHasher/6_segments_46_bytes
=== RUN   TestRefHasher/6_segments_47_bytes
=== PAUSE TestRefHasher/6_segments_47_bytes
=== RUN   TestRefHasher/6_segments_48_bytes
=== PAUSE TestRefHasher/6_segments_48_bytes
=== RUN   TestRefHasher/6_segments_49_bytes
=== PAUSE TestRefHasher/6_segments_49_bytes
=== RUN   TestRefHasher/6_segments_50_bytes
=== PAUSE TestRefHasher/6_segments_50_bytes
=== RUN   TestRefHasher/6_segments_51_bytes
=== PAUSE TestRefHasher/6_segments_51_bytes
=== RUN   TestRefHasher/6_segments_52_bytes
=== PAUSE TestRefHasher/6_segments_52_bytes
=== RUN   TestRefHasher/6_segments_53_bytes
=== PAUSE TestRefHasher/6_segments_53_bytes
=== RUN   TestRefHasher/6_segments_54_bytes
=== PAUSE TestRefHasher/6_segments_54_bytes
=== RUN   TestRefHasher/6_segments_55_bytes
=== PAUSE TestRefHasher/6_segments_55_bytes
=== RUN   TestRefHasher/6_segments_56_bytes
=== PAUSE TestRefHasher/6_segments_56_bytes
=== RUN   TestRefHasher/6_segments_57_bytes
=== PAUSE TestRefHasher/6_segments_57_bytes
=== RUN   TestRefHasher/6_segments_58_bytes
=== PAUSE TestRefHasher/6_segments_58_bytes
=== RUN   TestRefHasher/6_segments_59_bytes
=== PAUSE TestRefHasher/6_segments_59_bytes
=== RUN   TestRefHasher/6_segments_60_bytes
=== PAUSE TestRefHasher/6_segments_60_bytes
=== RUN   TestRefHasher/6_segments_61_bytes
=== PAUSE TestRefHasher/6_segments_61_bytes
=== RUN   TestRefHasher/6_segments_62_bytes
=== PAUSE TestRefHasher/6_segments_62_bytes
=== RUN   TestRefHasher/6_segments_63_bytes
=== PAUSE TestRefHasher/6_segments_63_bytes
=== RUN   TestRefHasher/6_segments_64_bytes
=== PAUSE TestRefHasher/6_segments_64_bytes
=== RUN   TestRefHasher/6_segments_65_bytes
=== PAUSE TestRefHasher/6_segments_65_bytes
=== RUN   TestRefHasher/6_segments_66_bytes
=== PAUSE TestRefHasher/6_segments_66_bytes
=== RUN   TestRefHasher/6_segments_67_bytes
=== PAUSE TestRefHasher/6_segments_67_bytes
=== RUN   TestRefHasher/6_segments_68_bytes
=== PAUSE TestRefHasher/6_segments_68_bytes
=== RUN   TestRefHasher/6_segments_69_bytes
=== PAUSE TestRefHasher/6_segments_69_bytes
=== RUN   TestRefHasher/6_segments_70_bytes
=== PAUSE TestRefHasher/6_segments_70_bytes
=== RUN   TestRefHasher/6_segments_71_bytes
=== PAUSE TestRefHasher/6_segments_71_bytes
=== RUN   TestRefHasher/6_segments_72_bytes
=== PAUSE TestRefHasher/6_segments_72_bytes
=== RUN   TestRefHasher/6_segments_73_bytes
=== PAUSE TestRefHasher/6_segments_73_bytes
=== RUN   TestRefHasher/6_segments_74_bytes
=== PAUSE TestRefHasher/6_segments_74_bytes
=== RUN   TestRefHasher/6_segments_75_bytes
=== PAUSE TestRefHasher/6_segments_75_bytes
=== RUN   TestRefHasher/6_segments_76_bytes
=== PAUSE TestRefHasher/6_segments_76_bytes
=== RUN   TestRefHasher/6_segments_77_bytes
=== PAUSE TestRefHasher/6_segments_77_bytes
=== RUN   TestRefHasher/6_segments_78_bytes
=== PAUSE TestRefHasher/6_segments_78_bytes
=== RUN   TestRefHasher/6_segments_79_bytes
=== PAUSE TestRefHasher/6_segments_79_bytes
=== RUN   TestRefHasher/6_segments_80_bytes
=== PAUSE TestRefHasher/6_segments_80_bytes
=== RUN   TestRefHasher/6_segments_81_bytes
=== PAUSE TestRefHasher/6_segments_81_bytes
=== RUN   TestRefHasher/6_segments_82_bytes
=== PAUSE TestRefHasher/6_segments_82_bytes
=== RUN   TestRefHasher/6_segments_83_bytes
=== PAUSE TestRefHasher/6_segments_83_bytes
=== RUN   TestRefHasher/6_segments_84_bytes
=== PAUSE TestRefHasher/6_segments_84_bytes
=== RUN   TestRefHasher/6_segments_85_bytes
=== PAUSE TestRefHasher/6_segments_85_bytes
=== RUN   TestRefHasher/6_segments_86_bytes
=== PAUSE TestRefHasher/6_segments_86_bytes
=== RUN   TestRefHasher/6_segments_87_bytes
=== PAUSE TestRefHasher/6_segments_87_bytes
=== RUN   TestRefHasher/6_segments_88_bytes
=== PAUSE TestRefHasher/6_segments_88_bytes
=== RUN   TestRefHasher/6_segments_89_bytes
=== PAUSE TestRefHasher/6_segments_89_bytes
=== RUN   TestRefHasher/6_segments_90_bytes
=== PAUSE TestRefHasher/6_segments_90_bytes
=== RUN   TestRefHasher/6_segments_91_bytes
=== PAUSE TestRefHasher/6_segments_91_bytes
=== RUN   TestRefHasher/6_segments_92_bytes
=== PAUSE TestRefHasher/6_segments_92_bytes
=== RUN   TestRefHasher/6_segments_93_bytes
=== PAUSE TestRefHasher/6_segments_93_bytes
=== RUN   TestRefHasher/6_segments_94_bytes
=== PAUSE TestRefHasher/6_segments_94_bytes
=== RUN   TestRefHasher/6_segments_95_bytes
=== PAUSE TestRefHasher/6_segments_95_bytes
=== RUN   TestRefHasher/6_segments_96_bytes
=== PAUSE TestRefHasher/6_segments_96_bytes
=== RUN   TestRefHasher/6_segments_97_bytes
=== PAUSE TestRefHasher/6_segments_97_bytes
=== RUN   TestRefHasher/6_segments_98_bytes
=== PAUSE TestRefHasher/6_segments_98_bytes
=== RUN   TestRefHasher/6_segments_99_bytes
=== PAUSE TestRefHasher/6_segments_99_bytes
=== RUN   TestRefHasher/6_segments_100_bytes
=== PAUSE TestRefHasher/6_segments_100_bytes
=== RUN   TestRefHasher/6_segments_101_bytes
=== PAUSE TestRefHasher/6_segments_101_bytes
=== RUN   TestRefHasher/6_segments_102_bytes
=== PAUSE TestRefHasher/6_segments_102_bytes
=== RUN   TestRefHasher/6_segments_103_bytes
=== PAUSE TestRefHasher/6_segments_103_bytes
=== RUN   TestRefHasher/6_segments_104_bytes
=== PAUSE TestRefHasher/6_segments_104_bytes
=== RUN   TestRefHasher/6_segments_105_bytes
=== PAUSE TestRefHasher/6_segments_105_bytes
=== RUN   TestRefHasher/6_segments_106_bytes
=== PAUSE TestRefHasher/6_segments_106_bytes
=== RUN   TestRefHasher/6_segments_107_bytes
=== PAUSE TestRefHasher/6_segments_107_bytes
=== RUN   TestRefHasher/6_segments_108_bytes
=== PAUSE TestRefHasher/6_segments_108_bytes
=== RUN   TestRefHasher/6_segments_109_bytes
=== PAUSE TestRefHasher/6_segments_109_bytes
=== RUN   TestRefHasher/6_segments_110_bytes
=== PAUSE TestRefHasher/6_segments_110_bytes
=== RUN   TestRefHasher/6_segments_111_bytes
=== PAUSE TestRefHasher/6_segments_111_bytes
=== RUN   TestRefHasher/6_segments_112_bytes
=== PAUSE TestRefHasher/6_segments_112_bytes
=== RUN   TestRefHasher/6_segments_113_bytes
=== PAUSE TestRefHasher/6_segments_113_bytes
=== RUN   TestRefHasher/6_segments_114_bytes
=== PAUSE TestRefHasher/6_segments_114_bytes
=== RUN   TestRefHasher/6_segments_115_bytes
=== PAUSE TestRefHasher/6_segments_115_bytes
=== RUN   TestRefHasher/6_segments_116_bytes
=== PAUSE TestRefHasher/6_segments_116_bytes
=== RUN   TestRefHasher/6_segments_117_bytes
=== PAUSE TestRefHasher/6_segments_117_bytes
=== RUN   TestRefHasher/6_segments_118_bytes
=== PAUSE TestRefHasher/6_segments_118_bytes
=== RUN   TestRefHasher/6_segments_119_bytes
=== PAUSE TestRefHasher/6_segments_119_bytes
=== RUN   TestRefHasher/6_segments_120_bytes
=== PAUSE TestRefHasher/6_segments_120_bytes
=== RUN   TestRefHasher/6_segments_121_bytes
=== PAUSE TestRefHasher/6_segments_121_bytes
=== RUN   TestRefHasher/6_segments_122_bytes
=== PAUSE TestRefHasher/6_segments_122_bytes
=== RUN   TestRefHasher/6_segments_123_bytes
=== PAUSE TestRefHasher/6_segments_123_bytes
=== RUN   TestRefHasher/6_segments_124_bytes
=== PAUSE TestRefHasher/6_segments_124_bytes
=== RUN   TestRefHasher/6_segments_125_bytes
=== PAUSE TestRefHasher/6_segments_125_bytes
=== RUN   TestRefHasher/6_segments_126_bytes
=== PAUSE TestRefHasher/6_segments_126_bytes
=== RUN   TestRefHasher/6_segments_127_bytes
=== PAUSE TestRefHasher/6_segments_127_bytes
=== RUN   TestRefHasher/6_segments_128_bytes
=== PAUSE TestRefHasher/6_segments_128_bytes
=== RUN   TestRefHasher/6_segments_129_bytes
=== PAUSE TestRefHasher/6_segments_129_bytes
=== RUN   TestRefHasher/6_segments_130_bytes
=== PAUSE TestRefHasher/6_segments_130_bytes
=== RUN   TestRefHasher/6_segments_131_bytes
=== PAUSE TestRefHasher/6_segments_131_bytes
=== RUN   TestRefHasher/6_segments_132_bytes
=== PAUSE TestRefHasher/6_segments_132_bytes
=== RUN   TestRefHasher/6_segments_133_bytes
=== PAUSE TestRefHasher/6_segments_133_bytes
=== RUN   TestRefHasher/6_segments_134_bytes
=== PAUSE TestRefHasher/6_segments_134_bytes
=== RUN   TestRefHasher/6_segments_135_bytes
=== PAUSE TestRefHasher/6_segments_135_bytes
=== RUN   TestRefHasher/6_segments_136_bytes
=== PAUSE TestRefHasher/6_segments_136_bytes
=== RUN   TestRefHasher/6_segments_137_bytes
=== PAUSE TestRefHasher/6_segments_137_bytes
=== RUN   TestRefHasher/6_segments_138_bytes
=== PAUSE TestRefHasher/6_segments_138_bytes
=== RUN   TestRefHasher/6_segments_139_bytes
=== PAUSE TestRefHasher/6_segments_139_bytes
=== RUN   TestRefHasher/6_segments_140_bytes
=== PAUSE TestRefHasher/6_segments_140_bytes
=== RUN   TestRefHasher/6_segments_141_bytes
=== PAUSE TestRefHasher/6_segments_141_bytes
=== RUN   TestRefHasher/6_segments_142_bytes
=== PAUSE TestRefHasher/6_segments_142_bytes
=== RUN   TestRefHasher/6_segments_143_bytes
=== PAUSE TestRefHasher/6_segments_143_bytes
=== RUN   TestRefHasher/6_segments_144_bytes
=== PAUSE TestRefHasher/6_segments_144_bytes
=== RUN   TestRefHasher/6_segments_145_bytes
=== PAUSE TestRefHasher/6_segments_145_bytes
=== RUN   TestRefHasher/6_segments_146_bytes
=== PAUSE TestRefHasher/6_segments_146_bytes
=== RUN   TestRefHasher/6_segments_147_bytes
=== PAUSE TestRefHasher/6_segments_147_bytes
=== RUN   TestRefHasher/6_segments_148_bytes
=== PAUSE TestRefHasher/6_segments_148_bytes
=== RUN   TestRefHasher/6_segments_149_bytes
=== PAUSE TestRefHasher/6_segments_149_bytes
=== RUN   TestRefHasher/6_segments_150_bytes
=== PAUSE TestRefHasher/6_segments_150_bytes
=== RUN   TestRefHasher/6_segments_151_bytes
=== PAUSE TestRefHasher/6_segments_151_bytes
=== RUN   TestRefHasher/6_segments_152_bytes
=== PAUSE TestRefHasher/6_segments_152_bytes
=== RUN   TestRefHasher/6_segments_153_bytes
=== PAUSE TestRefHasher/6_segments_153_bytes
=== RUN   TestRefHasher/6_segments_154_bytes
=== PAUSE TestRefHasher/6_segments_154_bytes
=== RUN   TestRefHasher/6_segments_155_bytes
=== PAUSE TestRefHasher/6_segments_155_bytes
=== RUN   TestRefHasher/6_segments_156_bytes
=== PAUSE TestRefHasher/6_segments_156_bytes
=== RUN   TestRefHasher/6_segments_157_bytes
=== PAUSE TestRefHasher/6_segments_157_bytes
=== RUN   TestRefHasher/6_segments_158_bytes
=== PAUSE TestRefHasher/6_segments_158_bytes
=== RUN   TestRefHasher/6_segments_159_bytes
=== PAUSE TestRefHasher/6_segments_159_bytes
=== RUN   TestRefHasher/6_segments_160_bytes
=== PAUSE TestRefHasher/6_segments_160_bytes
=== RUN   TestRefHasher/6_segments_161_bytes
=== PAUSE TestRefHasher/6_segments_161_bytes
=== RUN   TestRefHasher/6_segments_162_bytes
=== PAUSE TestRefHasher/6_segments_162_bytes
=== RUN   TestRefHasher/6_segments_163_bytes
=== PAUSE TestRefHasher/6_segments_163_bytes
=== RUN   TestRefHasher/6_segments_164_bytes
=== PAUSE TestRefHasher/6_segments_164_bytes
=== RUN   TestRefHasher/6_segments_165_bytes
=== PAUSE TestRefHasher/6_segments_165_bytes
=== RUN   TestRefHasher/6_segments_166_bytes
=== PAUSE TestRefHasher/6_segments_166_bytes
=== RUN   TestRefHasher/6_segments_167_bytes
=== PAUSE TestRefHasher/6_segments_167_bytes
=== RUN   TestRefHasher/6_segments_168_bytes
=== PAUSE TestRefHasher/6_segments_168_bytes
=== RUN   TestRefHasher/6_segments_169_bytes
=== PAUSE TestRefHasher/6_segments_169_bytes
=== RUN   TestRefHasher/6_segments_170_bytes
=== PAUSE TestRefHasher/6_segments_170_bytes
=== RUN   TestRefHasher/6_segments_171_bytes
=== PAUSE TestRefHasher/6_segments_171_bytes
=== RUN   TestRefHasher/6_segments_172_bytes
=== PAUSE TestRefHasher/6_segments_172_bytes
=== RUN   TestRefHasher/6_segments_173_bytes
=== PAUSE TestRefHasher/6_segments_173_bytes
=== RUN   TestRefHasher/6_segments_174_bytes
=== PAUSE TestRefHasher/6_segments_174_bytes
=== RUN   TestRefHasher/6_segments_175_bytes
=== PAUSE TestRefHasher/6_segments_175_bytes
=== RUN   TestRefHasher/6_segments_176_bytes
=== PAUSE TestRefHasher/6_segments_176_bytes
=== RUN   TestRefHasher/6_segments_177_bytes
=== PAUSE TestRefHasher/6_segments_177_bytes
=== RUN   TestRefHasher/6_segments_178_bytes
=== PAUSE TestRefHasher/6_segments_178_bytes
=== RUN   TestRefHasher/6_segments_179_bytes
=== PAUSE TestRefHasher/6_segments_179_bytes
=== RUN   TestRefHasher/6_segments_180_bytes
=== PAUSE TestRefHasher/6_segments_180_bytes
=== RUN   TestRefHasher/6_segments_181_bytes
=== PAUSE TestRefHasher/6_segments_181_bytes
=== RUN   TestRefHasher/6_segments_182_bytes
=== PAUSE TestRefHasher/6_segments_182_bytes
=== RUN   TestRefHasher/6_segments_183_bytes
=== PAUSE TestRefHasher/6_segments_183_bytes
=== RUN   TestRefHasher/6_segments_184_bytes
=== PAUSE TestRefHasher/6_segments_184_bytes
=== RUN   TestRefHasher/6_segments_185_bytes
=== PAUSE TestRefHasher/6_segments_185_bytes
=== RUN   TestRefHasher/6_segments_186_bytes
=== PAUSE TestRefHasher/6_segments_186_bytes
=== RUN   TestRefHasher/6_segments_187_bytes
=== PAUSE TestRefHasher/6_segments_187_bytes
=== RUN   TestRefHasher/6_segments_188_bytes
=== PAUSE TestRefHasher/6_segments_188_bytes
=== RUN   TestRefHasher/6_segments_189_bytes
=== PAUSE TestRefHasher/6_segments_189_bytes
=== RUN   TestRefHasher/6_segments_190_bytes
=== PAUSE TestRefHasher/6_segments_190_bytes
=== RUN   TestRefHasher/6_segments_191_bytes
=== PAUSE TestRefHasher/6_segments_191_bytes
=== RUN   TestRefHasher/6_segments_192_bytes
=== PAUSE TestRefHasher/6_segments_192_bytes
=== RUN   TestRefHasher/7_segments_1_bytes
=== PAUSE TestRefHasher/7_segments_1_bytes
=== RUN   TestRefHasher/7_segments_2_bytes
=== PAUSE TestRefHasher/7_segments_2_bytes
=== RUN   TestRefHasher/7_segments_3_bytes
=== PAUSE TestRefHasher/7_segments_3_bytes
=== RUN   TestRefHasher/7_segments_4_bytes
=== PAUSE TestRefHasher/7_segments_4_bytes
=== RUN   TestRefHasher/7_segments_5_bytes
=== PAUSE TestRefHasher/7_segments_5_bytes
=== RUN   TestRefHasher/7_segments_6_bytes
=== PAUSE TestRefHasher/7_segments_6_bytes
=== RUN   TestRefHasher/7_segments_7_bytes
=== PAUSE TestRefHasher/7_segments_7_bytes
=== RUN   TestRefHasher/7_segments_8_bytes
=== PAUSE TestRefHasher/7_segments_8_bytes
=== RUN   TestRefHasher/7_segments_9_bytes
=== PAUSE TestRefHasher/7_segments_9_bytes
=== RUN   TestRefHasher/7_segments_10_bytes
=== PAUSE TestRefHasher/7_segments_10_bytes
=== RUN   TestRefHasher/7_segments_11_bytes
=== PAUSE TestRefHasher/7_segments_11_bytes
=== RUN   TestRefHasher/7_segments_12_bytes
=== PAUSE TestRefHasher/7_segments_12_bytes
=== RUN   TestRefHasher/7_segments_13_bytes
=== PAUSE TestRefHasher/7_segments_13_bytes
=== RUN   TestRefHasher/7_segments_14_bytes
=== PAUSE TestRefHasher/7_segments_14_bytes
=== RUN   TestRefHasher/7_segments_15_bytes
=== PAUSE TestRefHasher/7_segments_15_bytes
=== RUN   TestRefHasher/7_segments_16_bytes
=== PAUSE TestRefHasher/7_segments_16_bytes
=== RUN   TestRefHasher/7_segments_17_bytes
=== PAUSE TestRefHasher/7_segments_17_bytes
=== RUN   TestRefHasher/7_segments_18_bytes
=== PAUSE TestRefHasher/7_segments_18_bytes
=== RUN   TestRefHasher/7_segments_19_bytes
=== PAUSE TestRefHasher/7_segments_19_bytes
=== RUN   TestRefHasher/7_segments_20_bytes
=== PAUSE TestRefHasher/7_segments_20_bytes
=== RUN   TestRefHasher/7_segments_21_bytes
=== PAUSE TestRefHasher/7_segments_21_bytes
=== RUN   TestRefHasher/7_segments_22_bytes
=== PAUSE TestRefHasher/7_segments_22_bytes
=== RUN   TestRefHasher/7_segments_23_bytes
=== PAUSE TestRefHasher/7_segments_23_bytes
=== RUN   TestRefHasher/7_segments_24_bytes
=== PAUSE TestRefHasher/7_segments_24_bytes
=== RUN   TestRefHasher/7_segments_25_bytes
=== PAUSE TestRefHasher/7_segments_25_bytes
=== RUN   TestRefHasher/7_segments_26_bytes
=== PAUSE TestRefHasher/7_segments_26_bytes
=== RUN   TestRefHasher/7_segments_27_bytes
=== PAUSE TestRefHasher/7_segments_27_bytes
=== RUN   TestRefHasher/7_segments_28_bytes
=== PAUSE TestRefHasher/7_segments_28_bytes
=== RUN   TestRefHasher/7_segments_29_bytes
=== PAUSE TestRefHasher/7_segments_29_bytes
=== RUN   TestRefHasher/7_segments_30_bytes
=== PAUSE TestRefHasher/7_segments_30_bytes
=== RUN   TestRefHasher/7_segments_31_bytes
=== PAUSE TestRefHasher/7_segments_31_bytes
=== RUN   TestRefHasher/7_segments_32_bytes
=== PAUSE TestRefHasher/7_segments_32_bytes
=== RUN   TestRefHasher/7_segments_33_bytes
=== PAUSE TestRefHasher/7_segments_33_bytes
=== RUN   TestRefHasher/7_segments_34_bytes
=== PAUSE TestRefHasher/7_segments_34_bytes
=== RUN   TestRefHasher/7_segments_35_bytes
=== PAUSE TestRefHasher/7_segments_35_bytes
=== RUN   TestRefHasher/7_segments_36_bytes
=== PAUSE TestRefHasher/7_segments_36_bytes
=== RUN   TestRefHasher/7_segments_37_bytes
=== PAUSE TestRefHasher/7_segments_37_bytes
=== RUN   TestRefHasher/7_segments_38_bytes
=== PAUSE TestRefHasher/7_segments_38_bytes
=== RUN   TestRefHasher/7_segments_39_bytes
=== PAUSE TestRefHasher/7_segments_39_bytes
=== RUN   TestRefHasher/7_segments_40_bytes
=== PAUSE TestRefHasher/7_segments_40_bytes
=== RUN   TestRefHasher/7_segments_41_bytes
=== PAUSE TestRefHasher/7_segments_41_bytes
=== RUN   TestRefHasher/7_segments_42_bytes
=== PAUSE TestRefHasher/7_segments_42_bytes
=== RUN   TestRefHasher/7_segments_43_bytes
=== PAUSE TestRefHasher/7_segments_43_bytes
=== RUN   TestRefHasher/7_segments_44_bytes
=== PAUSE TestRefHasher/7_segments_44_bytes
=== RUN   TestRefHasher/7_segments_45_bytes
=== PAUSE TestRefHasher/7_segments_45_bytes
=== RUN   TestRefHasher/7_segments_46_bytes
=== PAUSE TestRefHasher/7_segments_46_bytes
=== RUN   TestRefHasher/7_segments_47_bytes
=== PAUSE TestRefHasher/7_segments_47_bytes
=== RUN   TestRefHasher/7_segments_48_bytes
=== PAUSE TestRefHasher/7_segments_48_bytes
=== RUN   TestRefHasher/7_segments_49_bytes
=== PAUSE TestRefHasher/7_segments_49_bytes
=== RUN   TestRefHasher/7_segments_50_bytes
=== PAUSE TestRefHasher/7_segments_50_bytes
=== RUN   TestRefHasher/7_segments_51_bytes
=== PAUSE TestRefHasher/7_segments_51_bytes
=== RUN   TestRefHasher/7_segments_52_bytes
=== PAUSE TestRefHasher/7_segments_52_bytes
=== RUN   TestRefHasher/7_segments_53_bytes
=== PAUSE TestRefHasher/7_segments_53_bytes
=== RUN   TestRefHasher/7_segments_54_bytes
=== PAUSE TestRefHasher/7_segments_54_bytes
=== RUN   TestRefHasher/7_segments_55_bytes
=== PAUSE TestRefHasher/7_segments_55_bytes
=== RUN   TestRefHasher/7_segments_56_bytes
=== PAUSE TestRefHasher/7_segments_56_bytes
=== RUN   TestRefHasher/7_segments_57_bytes
=== PAUSE TestRefHasher/7_segments_57_bytes
=== RUN   TestRefHasher/7_segments_58_bytes
=== PAUSE TestRefHasher/7_segments_58_bytes
=== RUN   TestRefHasher/7_segments_59_bytes
=== PAUSE TestRefHasher/7_segments_59_bytes
=== RUN   TestRefHasher/7_segments_60_bytes
=== PAUSE TestRefHasher/7_segments_60_bytes
=== RUN   TestRefHasher/7_segments_61_bytes
=== PAUSE TestRefHasher/7_segments_61_bytes
=== RUN   TestRefHasher/7_segments_62_bytes
=== PAUSE TestRefHasher/7_segments_62_bytes
=== RUN   TestRefHasher/7_segments_63_bytes
=== PAUSE TestRefHasher/7_segments_63_bytes
=== RUN   TestRefHasher/7_segments_64_bytes
=== PAUSE TestRefHasher/7_segments_64_bytes
=== RUN   TestRefHasher/7_segments_65_bytes
=== PAUSE TestRefHasher/7_segments_65_bytes
=== RUN   TestRefHasher/7_segments_66_bytes
=== PAUSE TestRefHasher/7_segments_66_bytes
=== RUN   TestRefHasher/7_segments_67_bytes
=== PAUSE TestRefHasher/7_segments_67_bytes
=== RUN   TestRefHasher/7_segments_68_bytes
=== PAUSE TestRefHasher/7_segments_68_bytes
=== RUN   TestRefHasher/7_segments_69_bytes
=== PAUSE TestRefHasher/7_segments_69_bytes
=== RUN   TestRefHasher/7_segments_70_bytes
=== PAUSE TestRefHasher/7_segments_70_bytes
=== RUN   TestRefHasher/7_segments_71_bytes
=== PAUSE TestRefHasher/7_segments_71_bytes
=== RUN   TestRefHasher/7_segments_72_bytes
=== PAUSE TestRefHasher/7_segments_72_bytes
=== RUN   TestRefHasher/7_segments_73_bytes
=== PAUSE TestRefHasher/7_segments_73_bytes
=== RUN   TestRefHasher/7_segments_74_bytes
=== PAUSE TestRefHasher/7_segments_74_bytes
=== RUN   TestRefHasher/7_segments_75_bytes
=== PAUSE TestRefHasher/7_segments_75_bytes
=== RUN   TestRefHasher/7_segments_76_bytes
=== PAUSE TestRefHasher/7_segments_76_bytes
=== RUN   TestRefHasher/7_segments_77_bytes
=== PAUSE TestRefHasher/7_segments_77_bytes
=== RUN   TestRefHasher/7_segments_78_bytes
=== PAUSE TestRefHasher/7_segments_78_bytes
=== RUN   TestRefHasher/7_segments_79_bytes
=== PAUSE TestRefHasher/7_segments_79_bytes
=== RUN   TestRefHasher/7_segments_80_bytes
=== PAUSE TestRefHasher/7_segments_80_bytes
=== RUN   TestRefHasher/7_segments_81_bytes
=== PAUSE TestRefHasher/7_segments_81_bytes
=== RUN   TestRefHasher/7_segments_82_bytes
=== PAUSE TestRefHasher/7_segments_82_bytes
=== RUN   TestRefHasher/7_segments_83_bytes
=== PAUSE TestRefHasher/7_segments_83_bytes
=== RUN   TestRefHasher/7_segments_84_bytes
=== PAUSE TestRefHasher/7_segments_84_bytes
=== RUN   TestRefHasher/7_segments_85_bytes
=== PAUSE TestRefHasher/7_segments_85_bytes
=== RUN   TestRefHasher/7_segments_86_bytes
=== PAUSE TestRefHasher/7_segments_86_bytes
=== RUN   TestRefHasher/7_segments_87_bytes
=== PAUSE TestRefHasher/7_segments_87_bytes
=== RUN   TestRefHasher/7_segments_88_bytes
=== PAUSE TestRefHasher/7_segments_88_bytes
=== RUN   TestRefHasher/7_segments_89_bytes
=== PAUSE TestRefHasher/7_segments_89_bytes
=== RUN   TestRefHasher/7_segments_90_bytes
=== PAUSE TestRefHasher/7_segments_90_bytes
=== RUN   TestRefHasher/7_segments_91_bytes
=== PAUSE TestRefHasher/7_segments_91_bytes
=== RUN   TestRefHasher/7_segments_92_bytes
=== PAUSE TestRefHasher/7_segments_92_bytes
=== RUN   TestRefHasher/7_segments_93_bytes
=== PAUSE TestRefHasher/7_segments_93_bytes
=== RUN   TestRefHasher/7_segments_94_bytes
=== PAUSE TestRefHasher/7_segments_94_bytes
=== RUN   TestRefHasher/7_segments_95_bytes
=== PAUSE TestRefHasher/7_segments_95_bytes
=== RUN   TestRefHasher/7_segments_96_bytes
=== PAUSE TestRefHasher/7_segments_96_bytes
=== RUN   TestRefHasher/7_segments_97_bytes
=== PAUSE TestRefHasher/7_segments_97_bytes
=== RUN   TestRefHasher/7_segments_98_bytes
=== PAUSE TestRefHasher/7_segments_98_bytes
=== RUN   TestRefHasher/7_segments_99_bytes
=== PAUSE TestRefHasher/7_segments_99_bytes
=== RUN   TestRefHasher/7_segments_100_bytes
=== PAUSE TestRefHasher/7_segments_100_bytes
=== RUN   TestRefHasher/7_segments_101_bytes
=== PAUSE TestRefHasher/7_segments_101_bytes
=== RUN   TestRefHasher/7_segments_102_bytes
=== PAUSE TestRefHasher/7_segments_102_bytes
=== RUN   TestRefHasher/7_segments_103_bytes
=== PAUSE TestRefHasher/7_segments_103_bytes
=== RUN   TestRefHasher/7_segments_104_bytes
=== PAUSE TestRefHasher/7_segments_104_bytes
=== RUN   TestRefHasher/7_segments_105_bytes
=== PAUSE TestRefHasher/7_segments_105_bytes
=== RUN   TestRefHasher/7_segments_106_bytes
=== PAUSE TestRefHasher/7_segments_106_bytes
=== RUN   TestRefHasher/7_segments_107_bytes
=== PAUSE TestRefHasher/7_segments_107_bytes
=== RUN   TestRefHasher/7_segments_108_bytes
=== PAUSE TestRefHasher/7_segments_108_bytes
=== RUN   TestRefHasher/7_segments_109_bytes
=== PAUSE TestRefHasher/7_segments_109_bytes
=== RUN   TestRefHasher/7_segments_110_bytes
=== PAUSE TestRefHasher/7_segments_110_bytes
=== RUN   TestRefHasher/7_segments_111_bytes
=== PAUSE TestRefHasher/7_segments_111_bytes
=== RUN   TestRefHasher/7_segments_112_bytes
=== PAUSE TestRefHasher/7_segments_112_bytes
=== RUN   TestRefHasher/7_segments_113_bytes
=== PAUSE TestRefHasher/7_segments_113_bytes
=== RUN   TestRefHasher/7_segments_114_bytes
=== PAUSE TestRefHasher/7_segments_114_bytes
=== RUN   TestRefHasher/7_segments_115_bytes
=== PAUSE TestRefHasher/7_segments_115_bytes
=== RUN   TestRefHasher/7_segments_116_bytes
=== PAUSE TestRefHasher/7_segments_116_bytes
=== RUN   TestRefHasher/7_segments_117_bytes
=== PAUSE TestRefHasher/7_segments_117_bytes
=== RUN   TestRefHasher/7_segments_118_bytes
=== PAUSE TestRefHasher/7_segments_118_bytes
=== RUN   TestRefHasher/7_segments_119_bytes
=== PAUSE TestRefHasher/7_segments_119_bytes
=== RUN   TestRefHasher/7_segments_120_bytes
=== PAUSE TestRefHasher/7_segments_120_bytes
=== RUN   TestRefHasher/7_segments_121_bytes
=== PAUSE TestRefHasher/7_segments_121_bytes
=== RUN   TestRefHasher/7_segments_122_bytes
=== PAUSE TestRefHasher/7_segments_122_bytes
=== RUN   TestRefHasher/7_segments_123_bytes
=== PAUSE TestRefHasher/7_segments_123_bytes
=== RUN   TestRefHasher/7_segments_124_bytes
=== PAUSE TestRefHasher/7_segments_124_bytes
=== RUN   TestRefHasher/7_segments_125_bytes
=== PAUSE TestRefHasher/7_segments_125_bytes
=== RUN   TestRefHasher/7_segments_126_bytes
=== PAUSE TestRefHasher/7_segments_126_bytes
=== RUN   TestRefHasher/7_segments_127_bytes
=== PAUSE TestRefHasher/7_segments_127_bytes
=== RUN   TestRefHasher/7_segments_128_bytes
=== PAUSE TestRefHasher/7_segments_128_bytes
=== RUN   TestRefHasher/7_segments_129_bytes
=== PAUSE TestRefHasher/7_segments_129_bytes
=== RUN   TestRefHasher/7_segments_130_bytes
=== PAUSE TestRefHasher/7_segments_130_bytes
=== RUN   TestRefHasher/7_segments_131_bytes
=== PAUSE TestRefHasher/7_segments_131_bytes
=== RUN   TestRefHasher/7_segments_132_bytes
=== PAUSE TestRefHasher/7_segments_132_bytes
=== RUN   TestRefHasher/7_segments_133_bytes
=== PAUSE TestRefHasher/7_segments_133_bytes
=== RUN   TestRefHasher/7_segments_134_bytes
=== PAUSE TestRefHasher/7_segments_134_bytes
=== RUN   TestRefHasher/7_segments_135_bytes
=== PAUSE TestRefHasher/7_segments_135_bytes
=== RUN   TestRefHasher/7_segments_136_bytes
=== PAUSE TestRefHasher/7_segments_136_bytes
=== RUN   TestRefHasher/7_segments_137_bytes
=== PAUSE TestRefHasher/7_segments_137_bytes
=== RUN   TestRefHasher/7_segments_138_bytes
=== PAUSE TestRefHasher/7_segments_138_bytes
=== RUN   TestRefHasher/7_segments_139_bytes
=== PAUSE TestRefHasher/7_segments_139_bytes
=== RUN   TestRefHasher/7_segments_140_bytes
=== PAUSE TestRefHasher/7_segments_140_bytes
=== RUN   TestRefHasher/7_segments_141_bytes
=== PAUSE TestRefHasher/7_segments_141_bytes
=== RUN   TestRefHasher/7_segments_142_bytes
=== PAUSE TestRefHasher/7_segments_142_bytes
=== RUN   TestRefHasher/7_segments_143_bytes
=== PAUSE TestRefHasher/7_segments_143_bytes
=== RUN   TestRefHasher/7_segments_144_bytes
=== PAUSE TestRefHasher/7_segments_144_bytes
=== RUN   TestRefHasher/7_segments_145_bytes
=== PAUSE TestRefHasher/7_segments_145_bytes
=== RUN   TestRefHasher/7_segments_146_bytes
=== PAUSE TestRefHasher/7_segments_146_bytes
=== RUN   TestRefHasher/7_segments_147_bytes
=== PAUSE TestRefHasher/7_segments_147_bytes
=== RUN   TestRefHasher/7_segments_148_bytes
=== PAUSE TestRefHasher/7_segments_148_bytes
=== RUN   TestRefHasher/7_segments_149_bytes
=== PAUSE TestRefHasher/7_segments_149_bytes
=== RUN   TestRefHasher/7_segments_150_bytes
=== PAUSE TestRefHasher/7_segments_150_bytes
=== RUN   TestRefHasher/7_segments_151_bytes
=== PAUSE TestRefHasher/7_segments_151_bytes
=== RUN   TestRefHasher/7_segments_152_bytes
=== PAUSE TestRefHasher/7_segments_152_bytes
=== RUN   TestRefHasher/7_segments_153_bytes
=== PAUSE TestRefHasher/7_segments_153_bytes
=== RUN   TestRefHasher/7_segments_154_bytes
=== PAUSE TestRefHasher/7_segments_154_bytes
=== RUN   TestRefHasher/7_segments_155_bytes
=== PAUSE TestRefHasher/7_segments_155_bytes
=== RUN   TestRefHasher/7_segments_156_bytes
=== PAUSE TestRefHasher/7_segments_156_bytes
=== RUN   TestRefHasher/7_segments_157_bytes
=== PAUSE TestRefHasher/7_segments_157_bytes
=== RUN   TestRefHasher/7_segments_158_bytes
=== PAUSE TestRefHasher/7_segments_158_bytes
=== RUN   TestRefHasher/7_segments_159_bytes
=== PAUSE TestRefHasher/7_segments_159_bytes
=== RUN   TestRefHasher/7_segments_160_bytes
=== PAUSE TestRefHasher/7_segments_160_bytes
=== RUN   TestRefHasher/7_segments_161_bytes
=== PAUSE TestRefHasher/7_segments_161_bytes
=== RUN   TestRefHasher/7_segments_162_bytes
=== PAUSE TestRefHasher/7_segments_162_bytes
=== RUN   TestRefHasher/7_segments_163_bytes
=== PAUSE TestRefHasher/7_segments_163_bytes
=== RUN   TestRefHasher/7_segments_164_bytes
=== PAUSE TestRefHasher/7_segments_164_bytes
=== RUN   TestRefHasher/7_segments_165_bytes
=== PAUSE TestRefHasher/7_segments_165_bytes
=== RUN   TestRefHasher/7_segments_166_bytes
=== PAUSE TestRefHasher/7_segments_166_bytes
=== RUN   TestRefHasher/7_segments_167_bytes
=== PAUSE TestRefHasher/7_segments_167_bytes
=== RUN   TestRefHasher/7_segments_168_bytes
=== PAUSE TestRefHasher/7_segments_168_bytes
=== RUN   TestRefHasher/7_segments_169_bytes
=== PAUSE TestRefHasher/7_segments_169_bytes
=== RUN   TestRefHasher/7_segments_170_bytes
=== PAUSE TestRefHasher/7_segments_170_bytes
=== RUN   TestRefHasher/7_segments_171_bytes
=== PAUSE TestRefHasher/7_segments_171_bytes
=== RUN   TestRefHasher/7_segments_172_bytes
=== PAUSE TestRefHasher/7_segments_172_bytes
=== RUN   TestRefHasher/7_segments_173_bytes
=== PAUSE TestRefHasher/7_segments_173_bytes
=== RUN   TestRefHasher/7_segments_174_bytes
=== PAUSE TestRefHasher/7_segments_174_bytes
=== RUN   TestRefHasher/7_segments_175_bytes
=== PAUSE TestRefHasher/7_segments_175_bytes
=== RUN   TestRefHasher/7_segments_176_bytes
=== PAUSE TestRefHasher/7_segments_176_bytes
=== RUN   TestRefHasher/7_segments_177_bytes
=== PAUSE TestRefHasher/7_segments_177_bytes
=== RUN   TestRefHasher/7_segments_178_bytes
=== PAUSE TestRefHasher/7_segments_178_bytes
=== RUN   TestRefHasher/7_segments_179_bytes
=== PAUSE TestRefHasher/7_segments_179_bytes
=== RUN   TestRefHasher/7_segments_180_bytes
=== PAUSE TestRefHasher/7_segments_180_bytes
=== RUN   TestRefHasher/7_segments_181_bytes
=== PAUSE TestRefHasher/7_segments_181_bytes
=== RUN   TestRefHasher/7_segments_182_bytes
=== PAUSE TestRefHasher/7_segments_182_bytes
=== RUN   TestRefHasher/7_segments_183_bytes
=== PAUSE TestRefHasher/7_segments_183_bytes
=== RUN   TestRefHasher/7_segments_184_bytes
=== PAUSE TestRefHasher/7_segments_184_bytes
=== RUN   TestRefHasher/7_segments_185_bytes
=== PAUSE TestRefHasher/7_segments_185_bytes
=== RUN   TestRefHasher/7_segments_186_bytes
=== PAUSE TestRefHasher/7_segments_186_bytes
=== RUN   TestRefHasher/7_segments_187_bytes
=== PAUSE TestRefHasher/7_segments_187_bytes
=== RUN   TestRefHasher/7_segments_188_bytes
=== PAUSE TestRefHasher/7_segments_188_bytes
=== RUN   TestRefHasher/7_segments_189_bytes
=== PAUSE TestRefHasher/7_segments_189_bytes
=== RUN   TestRefHasher/7_segments_190_bytes
=== PAUSE TestRefHasher/7_segments_190_bytes
=== RUN   TestRefHasher/7_segments_191_bytes
=== PAUSE TestRefHasher/7_segments_191_bytes
=== RUN   TestRefHasher/7_segments_192_bytes
=== PAUSE TestRefHasher/7_segments_192_bytes
=== RUN   TestRefHasher/7_segments_193_bytes
=== PAUSE TestRefHasher/7_segments_193_bytes
=== RUN   TestRefHasher/7_segments_194_bytes
=== PAUSE TestRefHasher/7_segments_194_bytes
=== RUN   TestRefHasher/7_segments_195_bytes
=== PAUSE TestRefHasher/7_segments_195_bytes
=== RUN   TestRefHasher/7_segments_196_bytes
=== PAUSE TestRefHasher/7_segments_196_bytes
=== RUN   TestRefHasher/7_segments_197_bytes
=== PAUSE TestRefHasher/7_segments_197_bytes
=== RUN   TestRefHasher/7_segments_198_bytes
=== PAUSE TestRefHasher/7_segments_198_bytes
=== RUN   TestRefHasher/7_segments_199_bytes
=== PAUSE TestRefHasher/7_segments_199_bytes
=== RUN   TestRefHasher/7_segments_200_bytes
=== PAUSE TestRefHasher/7_segments_200_bytes
=== RUN   TestRefHasher/7_segments_201_bytes
=== PAUSE TestRefHasher/7_segments_201_bytes
=== RUN   TestRefHasher/7_segments_202_bytes
=== PAUSE TestRefHasher/7_segments_202_bytes
=== RUN   TestRefHasher/7_segments_203_bytes
=== PAUSE TestRefHasher/7_segments_203_bytes
=== RUN   TestRefHasher/7_segments_204_bytes
=== PAUSE TestRefHasher/7_segments_204_bytes
=== RUN   TestRefHasher/7_segments_205_bytes
=== PAUSE TestRefHasher/7_segments_205_bytes
=== RUN   TestRefHasher/7_segments_206_bytes
=== PAUSE TestRefHasher/7_segments_206_bytes
=== RUN   TestRefHasher/7_segments_207_bytes
=== PAUSE TestRefHasher/7_segments_207_bytes
=== RUN   TestRefHasher/7_segments_208_bytes
=== PAUSE TestRefHasher/7_segments_208_bytes
=== RUN   TestRefHasher/7_segments_209_bytes
=== PAUSE TestRefHasher/7_segments_209_bytes
=== RUN   TestRefHasher/7_segments_210_bytes
=== PAUSE TestRefHasher/7_segments_210_bytes
=== RUN   TestRefHasher/7_segments_211_bytes
=== PAUSE TestRefHasher/7_segments_211_bytes
=== RUN   TestRefHasher/7_segments_212_bytes
=== PAUSE TestRefHasher/7_segments_212_bytes
=== RUN   TestRefHasher/7_segments_213_bytes
=== PAUSE TestRefHasher/7_segments_213_bytes
=== RUN   TestRefHasher/7_segments_214_bytes
=== PAUSE TestRefHasher/7_segments_214_bytes
=== RUN   TestRefHasher/7_segments_215_bytes
=== PAUSE TestRefHasher/7_segments_215_bytes
=== RUN   TestRefHasher/7_segments_216_bytes
=== PAUSE TestRefHasher/7_segments_216_bytes
=== RUN   TestRefHasher/7_segments_217_bytes
=== PAUSE TestRefHasher/7_segments_217_bytes
=== RUN   TestRefHasher/7_segments_218_bytes
=== PAUSE TestRefHasher/7_segments_218_bytes
=== RUN   TestRefHasher/7_segments_219_bytes
=== PAUSE TestRefHasher/7_segments_219_bytes
=== RUN   TestRefHasher/7_segments_220_bytes
=== PAUSE TestRefHasher/7_segments_220_bytes
=== RUN   TestRefHasher/7_segments_221_bytes
=== PAUSE TestRefHasher/7_segments_221_bytes
=== RUN   TestRefHasher/7_segments_222_bytes
=== PAUSE TestRefHasher/7_segments_222_bytes
=== RUN   TestRefHasher/7_segments_223_bytes
=== PAUSE TestRefHasher/7_segments_223_bytes
=== RUN   TestRefHasher/7_segments_224_bytes
=== PAUSE TestRefHasher/7_segments_224_bytes
=== RUN   TestRefHasher/8_segments_1_bytes
=== PAUSE TestRefHasher/8_segments_1_bytes
=== RUN   TestRefHasher/8_segments_2_bytes
=== PAUSE TestRefHasher/8_segments_2_bytes
=== RUN   TestRefHasher/8_segments_3_bytes
=== PAUSE TestRefHasher/8_segments_3_bytes
=== RUN   TestRefHasher/8_segments_4_bytes
=== PAUSE TestRefHasher/8_segments_4_bytes
=== RUN   TestRefHasher/8_segments_5_bytes
=== PAUSE TestRefHasher/8_segments_5_bytes
=== RUN   TestRefHasher/8_segments_6_bytes
=== PAUSE TestRefHasher/8_segments_6_bytes
=== RUN   TestRefHasher/8_segments_7_bytes
=== PAUSE TestRefHasher/8_segments_7_bytes
=== RUN   TestRefHasher/8_segments_8_bytes
=== PAUSE TestRefHasher/8_segments_8_bytes
=== RUN   TestRefHasher/8_segments_9_bytes
=== PAUSE TestRefHasher/8_segments_9_bytes
=== RUN   TestRefHasher/8_segments_10_bytes
=== PAUSE TestRefHasher/8_segments_10_bytes
=== RUN   TestRefHasher/8_segments_11_bytes
=== PAUSE TestRefHasher/8_segments_11_bytes
=== RUN   TestRefHasher/8_segments_12_bytes
=== PAUSE TestRefHasher/8_segments_12_bytes
=== RUN   TestRefHasher/8_segments_13_bytes
=== PAUSE TestRefHasher/8_segments_13_bytes
=== RUN   TestRefHasher/8_segments_14_bytes
=== PAUSE TestRefHasher/8_segments_14_bytes
=== RUN   TestRefHasher/8_segments_15_bytes
=== PAUSE TestRefHasher/8_segments_15_bytes
=== RUN   TestRefHasher/8_segments_16_bytes
=== PAUSE TestRefHasher/8_segments_16_bytes
=== RUN   TestRefHasher/8_segments_17_bytes
=== PAUSE TestRefHasher/8_segments_17_bytes
=== RUN   TestRefHasher/8_segments_18_bytes
=== PAUSE TestRefHasher/8_segments_18_bytes
=== RUN   TestRefHasher/8_segments_19_bytes
=== PAUSE TestRefHasher/8_segments_19_bytes
=== RUN   TestRefHasher/8_segments_20_bytes
=== PAUSE TestRefHasher/8_segments_20_bytes
=== RUN   TestRefHasher/8_segments_21_bytes
=== PAUSE TestRefHasher/8_segments_21_bytes
=== RUN   TestRefHasher/8_segments_22_bytes
=== PAUSE TestRefHasher/8_segments_22_bytes
=== RUN   TestRefHasher/8_segments_23_bytes
=== PAUSE TestRefHasher/8_segments_23_bytes
=== RUN   TestRefHasher/8_segments_24_bytes
=== PAUSE TestRefHasher/8_segments_24_bytes
=== RUN   TestRefHasher/8_segments_25_bytes
=== PAUSE TestRefHasher/8_segments_25_bytes
=== RUN   TestRefHasher/8_segments_26_bytes
=== PAUSE TestRefHasher/8_segments_26_bytes
=== RUN   TestRefHasher/8_segments_27_bytes
=== PAUSE TestRefHasher/8_segments_27_bytes
=== RUN   TestRefHasher/8_segments_28_bytes
=== PAUSE TestRefHasher/8_segments_28_bytes
=== RUN   TestRefHasher/8_segments_29_bytes
=== PAUSE TestRefHasher/8_segments_29_bytes
=== RUN   TestRefHasher/8_segments_30_bytes
=== PAUSE TestRefHasher/8_segments_30_bytes
=== RUN   TestRefHasher/8_segments_31_bytes
=== PAUSE TestRefHasher/8_segments_31_bytes
=== RUN   TestRefHasher/8_segments_32_bytes
=== PAUSE TestRefHasher/8_segments_32_bytes
=== RUN   TestRefHasher/8_segments_33_bytes
=== PAUSE TestRefHasher/8_segments_33_bytes
=== RUN   TestRefHasher/8_segments_34_bytes
=== PAUSE TestRefHasher/8_segments_34_bytes
=== RUN   TestRefHasher/8_segments_35_bytes
=== PAUSE TestRefHasher/8_segments_35_bytes
=== RUN   TestRefHasher/8_segments_36_bytes
=== PAUSE TestRefHasher/8_segments_36_bytes
=== RUN   TestRefHasher/8_segments_37_bytes
=== PAUSE TestRefHasher/8_segments_37_bytes
=== RUN   TestRefHasher/8_segments_38_bytes
=== PAUSE TestRefHasher/8_segments_38_bytes
=== RUN   TestRefHasher/8_segments_39_bytes
=== PAUSE TestRefHasher/8_segments_39_bytes
=== RUN   TestRefHasher/8_segments_40_bytes
=== PAUSE TestRefHasher/8_segments_40_bytes
=== RUN   TestRefHasher/8_segments_41_bytes
=== PAUSE TestRefHasher/8_segments_41_bytes
=== RUN   TestRefHasher/8_segments_42_bytes
=== PAUSE TestRefHasher/8_segments_42_bytes
=== RUN   TestRefHasher/8_segments_43_bytes
=== PAUSE TestRefHasher/8_segments_43_bytes
=== RUN   TestRefHasher/8_segments_44_bytes
=== PAUSE TestRefHasher/8_segments_44_bytes
=== RUN   TestRefHasher/8_segments_45_bytes
=== PAUSE TestRefHasher/8_segments_45_bytes
=== RUN   TestRefHasher/8_segments_46_bytes
=== PAUSE TestRefHasher/8_segments_46_bytes
=== RUN   TestRefHasher/8_segments_47_bytes
=== PAUSE TestRefHasher/8_segments_47_bytes
=== RUN   TestRefHasher/8_segments_48_bytes
=== PAUSE TestRefHasher/8_segments_48_bytes
=== RUN   TestRefHasher/8_segments_49_bytes
=== PAUSE TestRefHasher/8_segments_49_bytes
=== RUN   TestRefHasher/8_segments_50_bytes
=== PAUSE TestRefHasher/8_segments_50_bytes
=== RUN   TestRefHasher/8_segments_51_bytes
=== PAUSE TestRefHasher/8_segments_51_bytes
=== RUN   TestRefHasher/8_segments_52_bytes
=== PAUSE TestRefHasher/8_segments_52_bytes
=== RUN   TestRefHasher/8_segments_53_bytes
=== PAUSE TestRefHasher/8_segments_53_bytes
=== RUN   TestRefHasher/8_segments_54_bytes
=== PAUSE TestRefHasher/8_segments_54_bytes
=== RUN   TestRefHasher/8_segments_55_bytes
=== PAUSE TestRefHasher/8_segments_55_bytes
=== RUN   TestRefHasher/8_segments_56_bytes
=== PAUSE TestRefHasher/8_segments_56_bytes
=== RUN   TestRefHasher/8_segments_57_bytes
=== PAUSE TestRefHasher/8_segments_57_bytes
=== RUN   TestRefHasher/8_segments_58_bytes
=== PAUSE TestRefHasher/8_segments_58_bytes
=== RUN   TestRefHasher/8_segments_59_bytes
=== PAUSE TestRefHasher/8_segments_59_bytes
=== RUN   TestRefHasher/8_segments_60_bytes
=== PAUSE TestRefHasher/8_segments_60_bytes
=== RUN   TestRefHasher/8_segments_61_bytes
=== PAUSE TestRefHasher/8_segments_61_bytes
=== RUN   TestRefHasher/8_segments_62_bytes
=== PAUSE TestRefHasher/8_segments_62_bytes
=== RUN   TestRefHasher/8_segments_63_bytes
=== PAUSE TestRefHasher/8_segments_63_bytes
=== RUN   TestRefHasher/8_segments_64_bytes
=== PAUSE TestRefHasher/8_segments_64_bytes
=== RUN   TestRefHasher/8_segments_65_bytes
=== PAUSE TestRefHasher/8_segments_65_bytes
=== RUN   TestRefHasher/8_segments_66_bytes
=== PAUSE TestRefHasher/8_segments_66_bytes
=== RUN   TestRefHasher/8_segments_67_bytes
=== PAUSE TestRefHasher/8_segments_67_bytes
=== RUN   TestRefHasher/8_segments_68_bytes
=== PAUSE TestRefHasher/8_segments_68_bytes
=== RUN   TestRefHasher/8_segments_69_bytes
=== PAUSE TestRefHasher/8_segments_69_bytes
=== RUN   TestRefHasher/8_segments_70_bytes
=== PAUSE TestRefHasher/8_segments_70_bytes
=== RUN   TestRefHasher/8_segments_71_bytes
=== PAUSE TestRefHasher/8_segments_71_bytes
=== RUN   TestRefHasher/8_segments_72_bytes
=== PAUSE TestRefHasher/8_segments_72_bytes
=== RUN   TestRefHasher/8_segments_73_bytes
=== PAUSE TestRefHasher/8_segments_73_bytes
=== RUN   TestRefHasher/8_segments_74_bytes
=== PAUSE TestRefHasher/8_segments_74_bytes
=== RUN   TestRefHasher/8_segments_75_bytes
=== PAUSE TestRefHasher/8_segments_75_bytes
=== RUN   TestRefHasher/8_segments_76_bytes
=== PAUSE TestRefHasher/8_segments_76_bytes
=== RUN   TestRefHasher/8_segments_77_bytes
=== PAUSE TestRefHasher/8_segments_77_bytes
=== RUN   TestRefHasher/8_segments_78_bytes
=== PAUSE TestRefHasher/8_segments_78_bytes
=== RUN   TestRefHasher/8_segments_79_bytes
=== PAUSE TestRefHasher/8_segments_79_bytes
=== RUN   TestRefHasher/8_segments_80_bytes
=== PAUSE TestRefHasher/8_segments_80_bytes
=== RUN   TestRefHasher/8_segments_81_bytes
=== PAUSE TestRefHasher/8_segments_81_bytes
=== RUN   TestRefHasher/8_segments_82_bytes
=== PAUSE TestRefHasher/8_segments_82_bytes
=== RUN   TestRefHasher/8_segments_83_bytes
=== PAUSE TestRefHasher/8_segments_83_bytes
=== RUN   TestRefHasher/8_segments_84_bytes
=== PAUSE TestRefHasher/8_segments_84_bytes
=== RUN   TestRefHasher/8_segments_85_bytes
=== PAUSE TestRefHasher/8_segments_85_bytes
=== RUN   TestRefHasher/8_segments_86_bytes
=== PAUSE TestRefHasher/8_segments_86_bytes
=== RUN   TestRefHasher/8_segments_87_bytes
=== PAUSE TestRefHasher/8_segments_87_bytes
=== RUN   TestRefHasher/8_segments_88_bytes
=== PAUSE TestRefHasher/8_segments_88_bytes
=== RUN   TestRefHasher/8_segments_89_bytes
=== PAUSE TestRefHasher/8_segments_89_bytes
=== RUN   TestRefHasher/8_segments_90_bytes
=== PAUSE TestRefHasher/8_segments_90_bytes
=== RUN   TestRefHasher/8_segments_91_bytes
=== PAUSE TestRefHasher/8_segments_91_bytes
=== RUN   TestRefHasher/8_segments_92_bytes
=== PAUSE TestRefHasher/8_segments_92_bytes
=== RUN   TestRefHasher/8_segments_93_bytes
=== PAUSE TestRefHasher/8_segments_93_bytes
=== RUN   TestRefHasher/8_segments_94_bytes
=== PAUSE TestRefHasher/8_segments_94_bytes
=== RUN   TestRefHasher/8_segments_95_bytes
=== PAUSE TestRefHasher/8_segments_95_bytes
=== RUN   TestRefHasher/8_segments_96_bytes
=== PAUSE TestRefHasher/8_segments_96_bytes
=== RUN   TestRefHasher/8_segments_97_bytes
=== PAUSE TestRefHasher/8_segments_97_bytes
=== RUN   TestRefHasher/8_segments_98_bytes
=== PAUSE TestRefHasher/8_segments_98_bytes
=== RUN   TestRefHasher/8_segments_99_bytes
=== PAUSE TestRefHasher/8_segments_99_bytes
=== RUN   TestRefHasher/8_segments_100_bytes
=== PAUSE TestRefHasher/8_segments_100_bytes
=== RUN   TestRefHasher/8_segments_101_bytes
=== PAUSE TestRefHasher/8_segments_101_bytes
=== RUN   TestRefHasher/8_segments_102_bytes
=== PAUSE TestRefHasher/8_segments_102_bytes
=== RUN   TestRefHasher/8_segments_103_bytes
=== PAUSE TestRefHasher/8_segments_103_bytes
=== RUN   TestRefHasher/8_segments_104_bytes
=== PAUSE TestRefHasher/8_segments_104_bytes
=== RUN   TestRefHasher/8_segments_105_bytes
=== PAUSE TestRefHasher/8_segments_105_bytes
=== RUN   TestRefHasher/8_segments_106_bytes
=== PAUSE TestRefHasher/8_segments_106_bytes
=== RUN   TestRefHasher/8_segments_107_bytes
=== PAUSE TestRefHasher/8_segments_107_bytes
=== RUN   TestRefHasher/8_segments_108_bytes
=== PAUSE TestRefHasher/8_segments_108_bytes
=== RUN   TestRefHasher/8_segments_109_bytes
=== PAUSE TestRefHasher/8_segments_109_bytes
=== RUN   TestRefHasher/8_segments_110_bytes
=== PAUSE TestRefHasher/8_segments_110_bytes
=== RUN   TestRefHasher/8_segments_111_bytes
=== PAUSE TestRefHasher/8_segments_111_bytes
=== RUN   TestRefHasher/8_segments_112_bytes
=== PAUSE TestRefHasher/8_segments_112_bytes
=== RUN   TestRefHasher/8_segments_113_bytes
=== PAUSE TestRefHasher/8_segments_113_bytes
=== RUN   TestRefHasher/8_segments_114_bytes
=== PAUSE TestRefHasher/8_segments_114_bytes
=== RUN   TestRefHasher/8_segments_115_bytes
=== PAUSE TestRefHasher/8_segments_115_bytes
=== RUN   TestRefHasher/8_segments_116_bytes
=== PAUSE TestRefHasher/8_segments_116_bytes
=== RUN   TestRefHasher/8_segments_117_bytes
=== PAUSE TestRefHasher/8_segments_117_bytes
=== RUN   TestRefHasher/8_segments_118_bytes
=== PAUSE TestRefHasher/8_segments_118_bytes
=== RUN   TestRefHasher/8_segments_119_bytes
=== PAUSE TestRefHasher/8_segments_119_bytes
=== RUN   TestRefHasher/8_segments_120_bytes
=== PAUSE TestRefHasher/8_segments_120_bytes
=== RUN   TestRefHasher/8_segments_121_bytes
=== PAUSE TestRefHasher/8_segments_121_bytes
=== RUN   TestRefHasher/8_segments_122_bytes
=== PAUSE TestRefHasher/8_segments_122_bytes
=== RUN   TestRefHasher/8_segments_123_bytes
=== PAUSE TestRefHasher/8_segments_123_bytes
=== RUN   TestRefHasher/8_segments_124_bytes
=== PAUSE TestRefHasher/8_segments_124_bytes
=== RUN   TestRefHasher/8_segments_125_bytes
=== PAUSE TestRefHasher/8_segments_125_bytes
=== RUN   TestRefHasher/8_segments_126_bytes
=== PAUSE TestRefHasher/8_segments_126_bytes
=== RUN   TestRefHasher/8_segments_127_bytes
=== PAUSE TestRefHasher/8_segments_127_bytes
=== RUN   TestRefHasher/8_segments_128_bytes
=== PAUSE TestRefHasher/8_segments_128_bytes
=== RUN   TestRefHasher/8_segments_129_bytes
=== PAUSE TestRefHasher/8_segments_129_bytes
=== RUN   TestRefHasher/8_segments_130_bytes
=== PAUSE TestRefHasher/8_segments_130_bytes
=== RUN   TestRefHasher/8_segments_131_bytes
=== PAUSE TestRefHasher/8_segments_131_bytes
=== RUN   TestRefHasher/8_segments_132_bytes
=== PAUSE TestRefHasher/8_segments_132_bytes
=== RUN   TestRefHasher/8_segments_133_bytes
=== PAUSE TestRefHasher/8_segments_133_bytes
=== RUN   TestRefHasher/8_segments_134_bytes
=== PAUSE TestRefHasher/8_segments_134_bytes
=== RUN   TestRefHasher/8_segments_135_bytes
=== PAUSE TestRefHasher/8_segments_135_bytes
=== RUN   TestRefHasher/8_segments_136_bytes
=== PAUSE TestRefHasher/8_segments_136_bytes
=== RUN   TestRefHasher/8_segments_137_bytes
=== PAUSE TestRefHasher/8_segments_137_bytes
=== RUN   TestRefHasher/8_segments_138_bytes
=== PAUSE TestRefHasher/8_segments_138_bytes
=== RUN   TestRefHasher/8_segments_139_bytes
=== PAUSE TestRefHasher/8_segments_139_bytes
=== RUN   TestRefHasher/8_segments_140_bytes
=== PAUSE TestRefHasher/8_segments_140_bytes
=== RUN   TestRefHasher/8_segments_141_bytes
=== PAUSE TestRefHasher/8_segments_141_bytes
=== RUN   TestRefHasher/8_segments_142_bytes
=== PAUSE TestRefHasher/8_segments_142_bytes
=== RUN   TestRefHasher/8_segments_143_bytes
=== PAUSE TestRefHasher/8_segments_143_bytes
=== RUN   TestRefHasher/8_segments_144_bytes
=== PAUSE TestRefHasher/8_segments_144_bytes
=== RUN   TestRefHasher/8_segments_145_bytes
=== PAUSE TestRefHasher/8_segments_145_bytes
=== RUN   TestRefHasher/8_segments_146_bytes
=== PAUSE TestRefHasher/8_segments_146_bytes
=== RUN   TestRefHasher/8_segments_147_bytes
=== PAUSE TestRefHasher/8_segments_147_bytes
=== RUN   TestRefHasher/8_segments_148_bytes
=== PAUSE TestRefHasher/8_segments_148_bytes
=== RUN   TestRefHasher/8_segments_149_bytes
=== PAUSE TestRefHasher/8_segments_149_bytes
=== RUN   TestRefHasher/8_segments_150_bytes
=== PAUSE TestRefHasher/8_segments_150_bytes
=== RUN   TestRefHasher/8_segments_151_bytes
=== PAUSE TestRefHasher/8_segments_151_bytes
=== RUN   TestRefHasher/8_segments_152_bytes
=== PAUSE TestRefHasher/8_segments_152_bytes
=== RUN   TestRefHasher/8_segments_153_bytes
=== PAUSE TestRefHasher/8_segments_153_bytes
=== RUN   TestRefHasher/8_segments_154_bytes
=== PAUSE TestRefHasher/8_segments_154_bytes
=== RUN   TestRefHasher/8_segments_155_bytes
=== PAUSE TestRefHasher/8_segments_155_bytes
=== RUN   TestRefHasher/8_segments_156_bytes
=== PAUSE TestRefHasher/8_segments_156_bytes
=== RUN   TestRefHasher/8_segments_157_bytes
=== PAUSE TestRefHasher/8_segments_157_bytes
=== RUN   TestRefHasher/8_segments_158_bytes
=== PAUSE TestRefHasher/8_segments_158_bytes
=== RUN   TestRefHasher/8_segments_159_bytes
=== PAUSE TestRefHasher/8_segments_159_bytes
=== RUN   TestRefHasher/8_segments_160_bytes
=== PAUSE TestRefHasher/8_segments_160_bytes
=== RUN   TestRefHasher/8_segments_161_bytes
=== PAUSE TestRefHasher/8_segments_161_bytes
=== RUN   TestRefHasher/8_segments_162_bytes
=== PAUSE TestRefHasher/8_segments_162_bytes
=== RUN   TestRefHasher/8_segments_163_bytes
=== PAUSE TestRefHasher/8_segments_163_bytes
=== RUN   TestRefHasher/8_segments_164_bytes
=== PAUSE TestRefHasher/8_segments_164_bytes
=== RUN   TestRefHasher/8_segments_165_bytes
=== PAUSE TestRefHasher/8_segments_165_bytes
=== RUN   TestRefHasher/8_segments_166_bytes
=== PAUSE TestRefHasher/8_segments_166_bytes
=== RUN   TestRefHasher/8_segments_167_bytes
=== PAUSE TestRefHasher/8_segments_167_bytes
=== RUN   TestRefHasher/8_segments_168_bytes
=== PAUSE TestRefHasher/8_segments_168_bytes
=== RUN   TestRefHasher/8_segments_169_bytes
=== PAUSE TestRefHasher/8_segments_169_bytes
=== RUN   TestRefHasher/8_segments_170_bytes
=== PAUSE TestRefHasher/8_segments_170_bytes
=== RUN   TestRefHasher/8_segments_171_bytes
=== PAUSE TestRefHasher/8_segments_171_bytes
=== RUN   TestRefHasher/8_segments_172_bytes
=== PAUSE TestRefHasher/8_segments_172_bytes
=== RUN   TestRefHasher/8_segments_173_bytes
=== PAUSE TestRefHasher/8_segments_173_bytes
=== RUN   TestRefHasher/8_segments_174_bytes
=== PAUSE TestRefHasher/8_segments_174_bytes
=== RUN   TestRefHasher/8_segments_175_bytes
=== PAUSE TestRefHasher/8_segments_175_bytes
=== RUN   TestRefHasher/8_segments_176_bytes
=== PAUSE TestRefHasher/8_segments_176_bytes
=== RUN   TestRefHasher/8_segments_177_bytes
=== PAUSE TestRefHasher/8_segments_177_bytes
=== RUN   TestRefHasher/8_segments_178_bytes
=== PAUSE TestRefHasher/8_segments_178_bytes
=== RUN   TestRefHasher/8_segments_179_bytes
=== PAUSE TestRefHasher/8_segments_179_bytes
=== RUN   TestRefHasher/8_segments_180_bytes
=== PAUSE TestRefHasher/8_segments_180_bytes
=== RUN   TestRefHasher/8_segments_181_bytes
=== PAUSE TestRefHasher/8_segments_181_bytes
=== RUN   TestRefHasher/8_segments_182_bytes
=== PAUSE TestRefHasher/8_segments_182_bytes
=== RUN   TestRefHasher/8_segments_183_bytes
=== PAUSE TestRefHasher/8_segments_183_bytes
=== RUN   TestRefHasher/8_segments_184_bytes
=== PAUSE TestRefHasher/8_segments_184_bytes
=== RUN   TestRefHasher/8_segments_185_bytes
=== PAUSE TestRefHasher/8_segments_185_bytes
=== RUN   TestRefHasher/8_segments_186_bytes
=== PAUSE TestRefHasher/8_segments_186_bytes
=== RUN   TestRefHasher/8_segments_187_bytes
=== PAUSE TestRefHasher/8_segments_187_bytes
=== RUN   TestRefHasher/8_segments_188_bytes
=== PAUSE TestRefHasher/8_segments_188_bytes
=== RUN   TestRefHasher/8_segments_189_bytes
=== PAUSE TestRefHasher/8_segments_189_bytes
=== RUN   TestRefHasher/8_segments_190_bytes
=== PAUSE TestRefHasher/8_segments_190_bytes
=== RUN   TestRefHasher/8_segments_191_bytes
=== PAUSE TestRefHasher/8_segments_191_bytes
=== RUN   TestRefHasher/8_segments_192_bytes
=== PAUSE TestRefHasher/8_segments_192_bytes
=== RUN   TestRefHasher/8_segments_193_bytes
=== PAUSE TestRefHasher/8_segments_193_bytes
=== RUN   TestRefHasher/8_segments_194_bytes
=== PAUSE TestRefHasher/8_segments_194_bytes
=== RUN   TestRefHasher/8_segments_195_bytes
=== PAUSE TestRefHasher/8_segments_195_bytes
=== RUN   TestRefHasher/8_segments_196_bytes
=== PAUSE TestRefHasher/8_segments_196_bytes
=== RUN   TestRefHasher/8_segments_197_bytes
=== PAUSE TestRefHasher/8_segments_197_bytes
=== RUN   TestRefHasher/8_segments_198_bytes
=== PAUSE TestRefHasher/8_segments_198_bytes
=== RUN   TestRefHasher/8_segments_199_bytes
=== PAUSE TestRefHasher/8_segments_199_bytes
=== RUN   TestRefHasher/8_segments_200_bytes
=== PAUSE TestRefHasher/8_segments_200_bytes
=== RUN   TestRefHasher/8_segments_201_bytes
=== PAUSE TestRefHasher/8_segments_201_bytes
=== RUN   TestRefHasher/8_segments_202_bytes
=== PAUSE TestRefHasher/8_segments_202_bytes
=== RUN   TestRefHasher/8_segments_203_bytes
=== PAUSE TestRefHasher/8_segments_203_bytes
=== RUN   TestRefHasher/8_segments_204_bytes
=== PAUSE TestRefHasher/8_segments_204_bytes
=== RUN   TestRefHasher/8_segments_205_bytes
=== PAUSE TestRefHasher/8_segments_205_bytes
=== RUN   TestRefHasher/8_segments_206_bytes
=== PAUSE TestRefHasher/8_segments_206_bytes
=== RUN   TestRefHasher/8_segments_207_bytes
=== PAUSE TestRefHasher/8_segments_207_bytes
=== RUN   TestRefHasher/8_segments_208_bytes
=== PAUSE TestRefHasher/8_segments_208_bytes
=== RUN   TestRefHasher/8_segments_209_bytes
=== PAUSE TestRefHasher/8_segments_209_bytes
=== RUN   TestRefHasher/8_segments_210_bytes
=== PAUSE TestRefHasher/8_segments_210_bytes
=== RUN   TestRefHasher/8_segments_211_bytes
=== PAUSE TestRefHasher/8_segments_211_bytes
=== RUN   TestRefHasher/8_segments_212_bytes
=== PAUSE TestRefHasher/8_segments_212_bytes
=== RUN   TestRefHasher/8_segments_213_bytes
=== PAUSE TestRefHasher/8_segments_213_bytes
=== RUN   TestRefHasher/8_segments_214_bytes
=== PAUSE TestRefHasher/8_segments_214_bytes
=== RUN   TestRefHasher/8_segments_215_bytes
=== PAUSE TestRefHasher/8_segments_215_bytes
=== RUN   TestRefHasher/8_segments_216_bytes
=== PAUSE TestRefHasher/8_segments_216_bytes
=== RUN   TestRefHasher/8_segments_217_bytes
=== PAUSE TestRefHasher/8_segments_217_bytes
=== RUN   TestRefHasher/8_segments_218_bytes
=== PAUSE TestRefHasher/8_segments_218_bytes
=== RUN   TestRefHasher/8_segments_219_bytes
=== PAUSE TestRefHasher/8_segments_219_bytes
=== RUN   TestRefHasher/8_segments_220_bytes
=== PAUSE TestRefHasher/8_segments_220_bytes
=== RUN   TestRefHasher/8_segments_221_bytes
=== PAUSE TestRefHasher/8_segments_221_bytes
=== RUN   TestRefHasher/8_segments_222_bytes
=== PAUSE TestRefHasher/8_segments_222_bytes
=== RUN   TestRefHasher/8_segments_223_bytes
=== PAUSE TestRefHasher/8_segments_223_bytes
=== RUN   TestRefHasher/8_segments_224_bytes
=== PAUSE TestRefHasher/8_segments_224_bytes
=== RUN   TestRefHasher/8_segments_225_bytes
=== PAUSE TestRefHasher/8_segments_225_bytes
=== RUN   TestRefHasher/8_segments_226_bytes
=== PAUSE TestRefHasher/8_segments_226_bytes
=== RUN   TestRefHasher/8_segments_227_bytes
=== PAUSE TestRefHasher/8_segments_227_bytes
=== RUN   TestRefHasher/8_segments_228_bytes
=== PAUSE TestRefHasher/8_segments_228_bytes
=== RUN   TestRefHasher/8_segments_229_bytes
=== PAUSE TestRefHasher/8_segments_229_bytes
=== RUN   TestRefHasher/8_segments_230_bytes
=== PAUSE TestRefHasher/8_segments_230_bytes
=== RUN   TestRefHasher/8_segments_231_bytes
=== PAUSE TestRefHasher/8_segments_231_bytes
=== RUN   TestRefHasher/8_segments_232_bytes
=== PAUSE TestRefHasher/8_segments_232_bytes
=== RUN   TestRefHasher/8_segments_233_bytes
=== PAUSE TestRefHasher/8_segments_233_bytes
=== RUN   TestRefHasher/8_segments_234_bytes
=== PAUSE TestRefHasher/8_segments_234_bytes
=== RUN   TestRefHasher/8_segments_235_bytes
=== PAUSE TestRefHasher/8_segments_235_bytes
=== RUN   TestRefHasher/8_segments_236_bytes
=== PAUSE TestRefHasher/8_segments_236_bytes
=== RUN   TestRefHasher/8_segments_237_bytes
=== PAUSE TestRefHasher/8_segments_237_bytes
=== RUN   TestRefHasher/8_segments_238_bytes
=== PAUSE TestRefHasher/8_segments_238_bytes
=== RUN   TestRefHasher/8_segments_239_bytes
=== PAUSE TestRefHasher/8_segments_239_bytes
=== RUN   TestRefHasher/8_segments_240_bytes
=== PAUSE TestRefHasher/8_segments_240_bytes
=== RUN   TestRefHasher/8_segments_241_bytes
=== PAUSE TestRefHasher/8_segments_241_bytes
=== RUN   TestRefHasher/8_segments_242_bytes
=== PAUSE TestRefHasher/8_segments_242_bytes
=== RUN   TestRefHasher/8_segments_243_bytes
=== PAUSE TestRefHasher/8_segments_243_bytes
=== RUN   TestRefHasher/8_segments_244_bytes
=== PAUSE TestRefHasher/8_segments_244_bytes
=== RUN   TestRefHasher/8_segments_245_bytes
=== PAUSE TestRefHasher/8_segments_245_bytes
=== RUN   TestRefHasher/8_segments_246_bytes
=== PAUSE TestRefHasher/8_segments_246_bytes
=== RUN   TestRefHasher/8_segments_247_bytes
=== PAUSE TestRefHasher/8_segments_247_bytes
=== RUN   TestRefHasher/8_segments_248_bytes
=== PAUSE TestRefHasher/8_segments_248_bytes
=== RUN   TestRefHasher/8_segments_249_bytes
=== PAUSE TestRefHasher/8_segments_249_bytes
=== RUN   TestRefHasher/8_segments_250_bytes
=== PAUSE TestRefHasher/8_segments_250_bytes
=== RUN   TestRefHasher/8_segments_251_bytes
=== PAUSE TestRefHasher/8_segments_251_bytes
=== RUN   TestRefHasher/8_segments_252_bytes
=== PAUSE TestRefHasher/8_segments_252_bytes
=== RUN   TestRefHasher/8_segments_253_bytes
=== PAUSE TestRefHasher/8_segments_253_bytes
=== RUN   TestRefHasher/8_segments_254_bytes
=== PAUSE TestRefHasher/8_segments_254_bytes
=== RUN   TestRefHasher/8_segments_255_bytes
=== PAUSE TestRefHasher/8_segments_255_bytes
=== RUN   TestRefHasher/8_segments_256_bytes
=== PAUSE TestRefHasher/8_segments_256_bytes
=== CONT  TestRefHasher/1_segments_1_bytes
=== CONT  TestRefHasher/4_segments_58_bytes
=== CONT  TestRefHasher/4_segments_57_bytes
=== CONT  TestRefHasher/4_segments_56_bytes
=== CONT  TestRefHasher/4_segments_55_bytes
=== CONT  TestRefHasher/4_segments_54_bytes
=== CONT  TestRefHasher/4_segments_53_bytes
=== CONT  TestRefHasher/4_segments_52_bytes
=== CONT  TestRefHasher/4_segments_51_bytes
=== CONT  TestRefHasher/4_segments_50_bytes
=== CONT  TestRefHasher/4_segments_49_bytes
=== CONT  TestRefHasher/4_segments_48_bytes
=== CONT  TestRefHasher/4_segments_47_bytes
=== CONT  TestRefHasher/4_segments_46_bytes
=== CONT  TestRefHasher/4_segments_45_bytes
=== CONT  TestRefHasher/4_segments_44_bytes
=== CONT  TestRefHasher/4_segments_43_bytes
=== CONT  TestRefHasher/4_segments_42_bytes
=== CONT  TestRefHasher/4_segments_41_bytes
=== CONT  TestRefHasher/4_segments_40_bytes
=== CONT  TestRefHasher/4_segments_39_bytes
=== CONT  TestRefHasher/4_segments_38_bytes
=== CONT  TestRefHasher/4_segments_37_bytes
=== CONT  TestRefHasher/4_segments_36_bytes
=== CONT  TestRefHasher/4_segments_35_bytes
=== CONT  TestRefHasher/4_segments_34_bytes
=== CONT  TestRefHasher/4_segments_33_bytes
=== CONT  TestRefHasher/4_segments_32_bytes
=== CONT  TestRefHasher/4_segments_31_bytes
=== CONT  TestRefHasher/4_segments_30_bytes
=== CONT  TestRefHasher/8_segments_256_bytes
=== CONT  TestRefHasher/8_segments_255_bytes
=== CONT  TestRefHasher/8_segments_254_bytes
=== CONT  TestRefHasher/8_segments_253_bytes
=== CONT  TestRefHasher/8_segments_252_bytes
=== CONT  TestRefHasher/8_segments_251_bytes
=== CONT  TestRefHasher/8_segments_143_bytes
=== CONT  TestRefHasher/8_segments_249_bytes
=== CONT  TestRefHasher/8_segments_248_bytes
=== CONT  TestRefHasher/8_segments_250_bytes
=== CONT  TestRefHasher/8_segments_240_bytes
=== CONT  TestRefHasher/8_segments_239_bytes
=== CONT  TestRefHasher/8_segments_238_bytes
=== CONT  TestRefHasher/8_segments_237_bytes
=== CONT  TestRefHasher/8_segments_236_bytes
=== CONT  TestRefHasher/8_segments_235_bytes
=== CONT  TestRefHasher/8_segments_234_bytes
=== CONT  TestRefHasher/8_segments_233_bytes
=== CONT  TestRefHasher/8_segments_232_bytes
=== CONT  TestRefHasher/8_segments_231_bytes
=== CONT  TestRefHasher/8_segments_230_bytes
=== CONT  TestRefHasher/8_segments_229_bytes
=== CONT  TestRefHasher/8_segments_228_bytes
=== CONT  TestRefHasher/8_segments_227_bytes
=== CONT  TestRefHasher/8_segments_226_bytes
=== CONT  TestRefHasher/8_segments_225_bytes
=== CONT  TestRefHasher/8_segments_224_bytes
=== CONT  TestRefHasher/8_segments_223_bytes
=== CONT  TestRefHasher/8_segments_222_bytes
=== CONT  TestRefHasher/8_segments_45_bytes
=== CONT  TestRefHasher/8_segments_221_bytes
=== CONT  TestRefHasher/8_segments_220_bytes
=== CONT  TestRefHasher/8_segments_219_bytes
=== CONT  TestRefHasher/8_segments_218_bytes
=== CONT  TestRefHasher/8_segments_217_bytes
=== CONT  TestRefHasher/8_segments_216_bytes
=== CONT  TestRefHasher/8_segments_215_bytes
=== CONT  TestRefHasher/8_segments_214_bytes
=== CONT  TestRefHasher/8_segments_213_bytes
=== CONT  TestRefHasher/8_segments_212_bytes
=== CONT  TestRefHasher/8_segments_211_bytes
=== CONT  TestRefHasher/8_segments_210_bytes
=== CONT  TestRefHasher/8_segments_209_bytes
=== CONT  TestRefHasher/8_segments_208_bytes
=== CONT  TestRefHasher/8_segments_207_bytes
=== CONT  TestRefHasher/8_segments_206_bytes
=== CONT  TestRefHasher/8_segments_142_bytes
=== CONT  TestRefHasher/8_segments_205_bytes
=== CONT  TestRefHasher/8_segments_204_bytes
=== CONT  TestRefHasher/8_segments_141_bytes
=== CONT  TestRefHasher/8_segments_203_bytes
=== CONT  TestRefHasher/8_segments_202_bytes
=== CONT  TestRefHasher/8_segments_140_bytes
=== CONT  TestRefHasher/8_segments_201_bytes
=== CONT  TestRefHasher/8_segments_200_bytes
=== CONT  TestRefHasher/8_segments_139_bytes
=== CONT  TestRefHasher/8_segments_44_bytes
=== CONT  TestRefHasher/8_segments_199_bytes
=== CONT  TestRefHasher/8_segments_198_bytes
=== CONT  TestRefHasher/8_segments_138_bytes
=== CONT  TestRefHasher/8_segments_43_bytes
=== CONT  TestRefHasher/8_segments_137_bytes
=== CONT  TestRefHasher/8_segments_42_bytes
=== CONT  TestRefHasher/7_segments_180_bytes
=== CONT  TestRefHasher/8_segments_136_bytes
=== CONT  TestRefHasher/8_segments_41_bytes
=== CONT  TestRefHasher/8_segments_197_bytes
=== CONT  TestRefHasher/8_segments_135_bytes
=== CONT  TestRefHasher/8_segments_40_bytes
=== CONT  TestRefHasher/8_segments_196_bytes
=== CONT  TestRefHasher/8_segments_39_bytes
=== CONT  TestRefHasher/8_segments_38_bytes
=== CONT  TestRefHasher/8_segments_37_bytes
=== CONT  TestRefHasher/8_segments_134_bytes
=== CONT  TestRefHasher/8_segments_36_bytes
=== CONT  TestRefHasher/8_segments_133_bytes
=== CONT  TestRefHasher/8_segments_35_bytes
=== CONT  TestRefHasher/8_segments_132_bytes
=== CONT  TestRefHasher/8_segments_34_bytes
=== CONT  TestRefHasher/8_segments_131_bytes
=== CONT  TestRefHasher/8_segments_33_bytes
=== CONT  TestRefHasher/8_segments_130_bytes
=== CONT  TestRefHasher/8_segments_32_bytes
=== CONT  TestRefHasher/8_segments_129_bytes
=== CONT  TestRefHasher/8_segments_31_bytes
=== CONT  TestRefHasher/7_segments_224_bytes
=== CONT  TestRefHasher/8_segments_30_bytes
=== CONT  TestRefHasher/8_segments_195_bytes
=== CONT  TestRefHasher/8_segments_128_bytes
=== CONT  TestRefHasher/8_segments_194_bytes
=== CONT  TestRefHasher/8_segments_127_bytes
=== CONT  TestRefHasher/8_segments_193_bytes
=== CONT  TestRefHasher/8_segments_126_bytes
=== CONT  TestRefHasher/8_segments_192_bytes
=== CONT  TestRefHasher/8_segments_125_bytes
=== CONT  TestRefHasher/8_segments_29_bytes
=== CONT  TestRefHasher/8_segments_124_bytes
=== CONT  TestRefHasher/8_segments_191_bytes
=== CONT  TestRefHasher/8_segments_123_bytes
=== CONT  TestRefHasher/8_segments_190_bytes
=== CONT  TestRefHasher/8_segments_28_bytes
=== CONT  TestRefHasher/8_segments_189_bytes
=== CONT  TestRefHasher/8_segments_122_bytes
=== CONT  TestRefHasher/8_segments_188_bytes
=== CONT  TestRefHasher/8_segments_27_bytes
=== CONT  TestRefHasher/8_segments_121_bytes
=== CONT  TestRefHasher/8_segments_26_bytes
=== CONT  TestRefHasher/8_segments_187_bytes
=== CONT  TestRefHasher/8_segments_25_bytes
=== CONT  TestRefHasher/8_segments_120_bytes
=== CONT  TestRefHasher/8_segments_186_bytes
=== CONT  TestRefHasher/8_segments_24_bytes
=== CONT  TestRefHasher/8_segments_119_bytes
=== CONT  TestRefHasher/8_segments_185_bytes
=== CONT  TestRefHasher/8_segments_23_bytes
=== CONT  TestRefHasher/8_segments_118_bytes
=== CONT  TestRefHasher/8_segments_184_bytes
=== CONT  TestRefHasher/8_segments_22_bytes
=== CONT  TestRefHasher/8_segments_183_bytes
=== CONT  TestRefHasher/8_segments_117_bytes
=== CONT  TestRefHasher/8_segments_21_bytes
=== CONT  TestRefHasher/8_segments_182_bytes
=== CONT  TestRefHasher/8_segments_116_bytes
=== CONT  TestRefHasher/8_segments_20_bytes
=== CONT  TestRefHasher/8_segments_181_bytes
=== CONT  TestRefHasher/8_segments_115_bytes
=== CONT  TestRefHasher/8_segments_180_bytes
=== CONT  TestRefHasher/8_segments_114_bytes
=== CONT  TestRefHasher/8_segments_19_bytes
=== CONT  TestRefHasher/8_segments_179_bytes
=== CONT  TestRefHasher/8_segments_113_bytes
=== CONT  TestRefHasher/8_segments_178_bytes
=== CONT  TestRefHasher/8_segments_177_bytes
=== CONT  TestRefHasher/8_segments_112_bytes
=== CONT  TestRefHasher/8_segments_18_bytes
=== CONT  TestRefHasher/8_segments_176_bytes
=== CONT  TestRefHasher/8_segments_111_bytes
=== CONT  TestRefHasher/8_segments_17_bytes
=== CONT  TestRefHasher/8_segments_175_bytes
=== CONT  TestRefHasher/8_segments_110_bytes
=== CONT  TestRefHasher/8_segments_174_bytes
=== CONT  TestRefHasher/8_segments_16_bytes
=== CONT  TestRefHasher/8_segments_109_bytes
=== CONT  TestRefHasher/8_segments_247_bytes
=== CONT  TestRefHasher/8_segments_246_bytes
=== CONT  TestRefHasher/8_segments_245_bytes
=== CONT  TestRefHasher/8_segments_244_bytes
=== CONT  TestRefHasher/8_segments_243_bytes
=== CONT  TestRefHasher/8_segments_242_bytes
=== CONT  TestRefHasher/8_segments_241_bytes
=== CONT  TestRefHasher/8_segments_15_bytes
=== CONT  TestRefHasher/8_segments_173_bytes
=== CONT  TestRefHasher/8_segments_108_bytes
=== CONT  TestRefHasher/8_segments_14_bytes
=== CONT  TestRefHasher/8_segments_172_bytes
=== CONT  TestRefHasher/8_segments_13_bytes
=== CONT  TestRefHasher/8_segments_107_bytes
=== CONT  TestRefHasher/8_segments_171_bytes
=== CONT  TestRefHasher/8_segments_12_bytes
=== CONT  TestRefHasher/8_segments_170_bytes
=== CONT  TestRefHasher/8_segments_106_bytes
=== CONT  TestRefHasher/8_segments_169_bytes
=== CONT  TestRefHasher/8_segments_11_bytes
=== CONT  TestRefHasher/8_segments_168_bytes
=== CONT  TestRefHasher/8_segments_105_bytes
=== CONT  TestRefHasher/8_segments_10_bytes
=== CONT  TestRefHasher/8_segments_167_bytes
=== CONT  TestRefHasher/8_segments_104_bytes
=== CONT  TestRefHasher/8_segments_166_bytes
=== CONT  TestRefHasher/8_segments_9_bytes
=== CONT  TestRefHasher/8_segments_165_bytes
=== CONT  TestRefHasher/8_segments_103_bytes
=== CONT  TestRefHasher/8_segments_8_bytes
=== CONT  TestRefHasher/8_segments_164_bytes
=== CONT  TestRefHasher/8_segments_102_bytes
=== CONT  TestRefHasher/8_segments_7_bytes
=== CONT  TestRefHasher/8_segments_163_bytes
=== CONT  TestRefHasher/8_segments_101_bytes
=== CONT  TestRefHasher/8_segments_162_bytes
=== CONT  TestRefHasher/8_segments_100_bytes
=== CONT  TestRefHasher/8_segments_161_bytes
=== CONT  TestRefHasher/8_segments_6_bytes
=== CONT  TestRefHasher/8_segments_99_bytes
=== CONT  TestRefHasher/8_segments_160_bytes
=== CONT  TestRefHasher/8_segments_98_bytes
=== CONT  TestRefHasher/8_segments_159_bytes
=== CONT  TestRefHasher/8_segments_5_bytes
=== CONT  TestRefHasher/8_segments_158_bytes
=== CONT  TestRefHasher/8_segments_4_bytes
=== CONT  TestRefHasher/8_segments_97_bytes
=== CONT  TestRefHasher/8_segments_157_bytes
=== CONT  TestRefHasher/8_segments_96_bytes
=== CONT  TestRefHasher/8_segments_3_bytes
=== CONT  TestRefHasher/8_segments_156_bytes
=== CONT  TestRefHasher/8_segments_95_bytes
=== CONT  TestRefHasher/8_segments_155_bytes
=== CONT  TestRefHasher/8_segments_2_bytes
=== CONT  TestRefHasher/8_segments_94_bytes
=== CONT  TestRefHasher/8_segments_93_bytes
=== CONT  TestRefHasher/8_segments_1_bytes
=== CONT  TestRefHasher/8_segments_154_bytes
=== CONT  TestRefHasher/8_segments_92_bytes
=== CONT  TestRefHasher/8_segments_153_bytes
=== CONT  TestRefHasher/7_segments_95_bytes
=== CONT  TestRefHasher/8_segments_149_bytes
=== CONT  TestRefHasher/7_segments_221_bytes
=== CONT  TestRefHasher/8_segments_91_bytes
=== CONT  TestRefHasher/8_segments_148_bytes
=== CONT  TestRefHasher/8_segments_89_bytes
=== CONT  TestRefHasher/7_segments_220_bytes
=== CONT  TestRefHasher/8_segments_152_bytes
=== CONT  TestRefHasher/8_segments_146_bytes
=== CONT  TestRefHasher/7_segments_219_bytes
=== CONT  TestRefHasher/8_segments_87_bytes
=== CONT  TestRefHasher/7_segments_223_bytes
=== CONT  TestRefHasher/8_segments_145_bytes
=== CONT  TestRefHasher/7_segments_218_bytes
=== CONT  TestRefHasher/8_segments_151_bytes
=== CONT  TestRefHasher/8_segments_144_bytes
=== CONT  TestRefHasher/8_segments_90_bytes
=== CONT  TestRefHasher/7_segments_217_bytes
=== CONT  TestRefHasher/8_segments_150_bytes
=== CONT  TestRefHasher/7_segments_222_bytes
=== CONT  TestRefHasher/8_segments_147_bytes
=== CONT  TestRefHasher/8_segments_85_bytes
=== CONT  TestRefHasher/7_segments_215_bytes
=== CONT  TestRefHasher/8_segments_84_bytes
=== CONT  TestRefHasher/7_segments_214_bytes
=== CONT  TestRefHasher/7_segments_213_bytes
=== CONT  TestRefHasher/8_segments_83_bytes
=== CONT  TestRefHasher/7_segments_212_bytes
=== CONT  TestRefHasher/8_segments_82_bytes
=== CONT  TestRefHasher/7_segments_181_bytes
=== CONT  TestRefHasher/8_segments_81_bytes
=== CONT  TestRefHasher/7_segments_211_bytes
=== CONT  TestRefHasher/8_segments_88_bytes
=== CONT  TestRefHasher/7_segments_210_bytes
=== CONT  TestRefHasher/8_segments_79_bytes
=== CONT  TestRefHasher/7_segments_209_bytes
=== CONT  TestRefHasher/8_segments_78_bytes
=== CONT  TestRefHasher/8_segments_77_bytes
=== CONT  TestRefHasher/7_segments_178_bytes
=== CONT  TestRefHasher/8_segments_76_bytes
=== CONT  TestRefHasher/7_segments_177_bytes
=== CONT  TestRefHasher/8_segments_75_bytes
=== CONT  TestRefHasher/7_segments_176_bytes
=== CONT  TestRefHasher/8_segments_74_bytes
=== CONT  TestRefHasher/7_segments_175_bytes
=== CONT  TestRefHasher/8_segments_73_bytes
=== CONT  TestRefHasher/7_segments_174_bytes
=== CONT  TestRefHasher/8_segments_72_bytes
=== CONT  TestRefHasher/8_segments_71_bytes
=== CONT  TestRefHasher/7_segments_173_bytes
=== CONT  TestRefHasher/8_segments_70_bytes
=== CONT  TestRefHasher/8_segments_69_bytes
=== CONT  TestRefHasher/8_segments_68_bytes
=== CONT  TestRefHasher/8_segments_67_bytes
=== CONT  TestRefHasher/8_segments_66_bytes
=== CONT  TestRefHasher/8_segments_65_bytes
=== CONT  TestRefHasher/8_segments_64_bytes
=== CONT  TestRefHasher/8_segments_63_bytes
=== CONT  TestRefHasher/8_segments_62_bytes
=== CONT  TestRefHasher/8_segments_61_bytes
=== CONT  TestRefHasher/8_segments_60_bytes
=== CONT  TestRefHasher/8_segments_59_bytes
=== CONT  TestRefHasher/8_segments_58_bytes
=== CONT  TestRefHasher/8_segments_57_bytes
=== CONT  TestRefHasher/8_segments_56_bytes
=== CONT  TestRefHasher/8_segments_55_bytes
=== CONT  TestRefHasher/8_segments_54_bytes
=== CONT  TestRefHasher/8_segments_53_bytes
=== CONT  TestRefHasher/8_segments_52_bytes
=== CONT  TestRefHasher/8_segments_51_bytes
=== CONT  TestRefHasher/8_segments_50_bytes
=== CONT  TestRefHasher/8_segments_49_bytes
=== CONT  TestRefHasher/8_segments_48_bytes
=== CONT  TestRefHasher/8_segments_47_bytes
=== CONT  TestRefHasher/8_segments_46_bytes
=== CONT  TestRefHasher/7_segments_36_bytes
=== CONT  TestRefHasher/7_segments_35_bytes
=== CONT  TestRefHasher/7_segments_34_bytes
=== CONT  TestRefHasher/7_segments_33_bytes
=== CONT  TestRefHasher/7_segments_32_bytes
=== CONT  TestRefHasher/7_segments_31_bytes
=== CONT  TestRefHasher/7_segments_30_bytes
=== CONT  TestRefHasher/7_segments_102_bytes
=== CONT  TestRefHasher/7_segments_29_bytes
=== CONT  TestRefHasher/7_segments_101_bytes
=== CONT  TestRefHasher/7_segments_28_bytes
=== CONT  TestRefHasher/7_segments_100_bytes
=== CONT  TestRefHasher/7_segments_99_bytes
=== CONT  TestRefHasher/7_segments_27_bytes
=== CONT  TestRefHasher/7_segments_98_bytes
=== CONT  TestRefHasher/7_segments_26_bytes
=== CONT  TestRefHasher/7_segments_97_bytes
=== CONT  TestRefHasher/7_segments_96_bytes
=== CONT  TestRefHasher/7_segments_25_bytes
=== CONT  TestRefHasher/6_segments_158_bytes
=== CONT  TestRefHasher/7_segments_24_bytes
=== CONT  TestRefHasher/7_segments_94_bytes
=== CONT  TestRefHasher/7_segments_93_bytes
=== CONT  TestRefHasher/7_segments_23_bytes
=== CONT  TestRefHasher/7_segments_92_bytes
=== CONT  TestRefHasher/7_segments_22_bytes
=== CONT  TestRefHasher/7_segments_91_bytes
=== CONT  TestRefHasher/7_segments_90_bytes
=== CONT  TestRefHasher/7_segments_21_bytes
=== CONT  TestRefHasher/7_segments_89_bytes
=== CONT  TestRefHasher/7_segments_20_bytes
=== CONT  TestRefHasher/7_segments_88_bytes
=== CONT  TestRefHasher/7_segments_19_bytes
=== CONT  TestRefHasher/7_segments_87_bytes
=== CONT  TestRefHasher/7_segments_86_bytes
=== CONT  TestRefHasher/7_segments_18_bytes
=== CONT  TestRefHasher/7_segments_85_bytes
=== CONT  TestRefHasher/7_segments_84_bytes
=== CONT  TestRefHasher/7_segments_17_bytes
=== CONT  TestRefHasher/7_segments_83_bytes
=== CONT  TestRefHasher/6_segments_169_bytes
=== CONT  TestRefHasher/7_segments_82_bytes
=== CONT  TestRefHasher/7_segments_16_bytes
=== CONT  TestRefHasher/7_segments_81_bytes
=== CONT  TestRefHasher/7_segments_80_bytes
=== CONT  TestRefHasher/7_segments_15_bytes
=== CONT  TestRefHasher/7_segments_79_bytes
=== CONT  TestRefHasher/7_segments_14_bytes
=== CONT  TestRefHasher/7_segments_78_bytes
=== CONT  TestRefHasher/7_segments_13_bytes
=== CONT  TestRefHasher/7_segments_77_bytes
=== CONT  TestRefHasher/7_segments_76_bytes
=== CONT  TestRefHasher/7_segments_12_bytes
=== CONT  TestRefHasher/7_segments_75_bytes
=== CONT  TestRefHasher/7_segments_11_bytes
=== CONT  TestRefHasher/7_segments_10_bytes
=== CONT  TestRefHasher/7_segments_74_bytes
=== CONT  TestRefHasher/7_segments_9_bytes
=== CONT  TestRefHasher/7_segments_73_bytes
=== CONT  TestRefHasher/7_segments_8_bytes
=== CONT  TestRefHasher/7_segments_72_bytes
=== CONT  TestRefHasher/7_segments_7_bytes
=== CONT  TestRefHasher/7_segments_71_bytes
=== CONT  TestRefHasher/7_segments_70_bytes
=== CONT  TestRefHasher/7_segments_6_bytes
=== CONT  TestRefHasher/7_segments_69_bytes
=== CONT  TestRefHasher/7_segments_5_bytes
=== CONT  TestRefHasher/7_segments_68_bytes
=== CONT  TestRefHasher/7_segments_4_bytes
=== CONT  TestRefHasher/7_segments_67_bytes
=== CONT  TestRefHasher/7_segments_3_bytes
=== CONT  TestRefHasher/7_segments_66_bytes
=== CONT  TestRefHasher/7_segments_65_bytes
=== CONT  TestRefHasher/7_segments_2_bytes
=== CONT  TestRefHasher/7_segments_64_bytes
=== CONT  TestRefHasher/7_segments_1_bytes
=== CONT  TestRefHasher/6_segments_170_bytes
=== CONT  TestRefHasher/7_segments_63_bytes
=== CONT  TestRefHasher/6_segments_192_bytes
=== CONT  TestRefHasher/7_segments_62_bytes
=== CONT  TestRefHasher/7_segments_208_bytes
=== CONT  TestRefHasher/7_segments_61_bytes
=== CONT  TestRefHasher/6_segments_191_bytes
=== CONT  TestRefHasher/7_segments_207_bytes
=== CONT  TestRefHasher/7_segments_60_bytes
=== CONT  TestRefHasher/7_segments_206_bytes
=== CONT  TestRefHasher/6_segments_190_bytes
=== CONT  TestRefHasher/7_segments_59_bytes
=== CONT  TestRefHasher/7_segments_205_bytes
=== CONT  TestRefHasher/7_segments_58_bytes
=== CONT  TestRefHasher/7_segments_204_bytes
=== CONT  TestRefHasher/7_segments_57_bytes
=== CONT  TestRefHasher/6_segments_189_bytes
=== CONT  TestRefHasher/7_segments_56_bytes
=== CONT  TestRefHasher/7_segments_203_bytes
=== CONT  TestRefHasher/6_segments_188_bytes
=== CONT  TestRefHasher/7_segments_202_bytes
=== CONT  TestRefHasher/6_segments_175_bytes
=== CONT  TestRefHasher/7_segments_55_bytes
=== CONT  TestRefHasher/7_segments_186_bytes
=== CONT  TestRefHasher/6_segments_187_bytes
=== CONT  TestRefHasher/7_segments_185_bytes
=== CONT  TestRefHasher/6_segments_174_bytes
=== CONT  TestRefHasher/7_segments_201_bytes
=== CONT  TestRefHasher/7_segments_184_bytes
=== CONT  TestRefHasher/6_segments_186_bytes
=== CONT  TestRefHasher/6_segments_173_bytes
=== CONT  TestRefHasher/7_segments_183_bytes
=== CONT  TestRefHasher/7_segments_200_bytes
=== CONT  TestRefHasher/6_segments_172_bytes
=== CONT  TestRefHasher/7_segments_182_bytes
=== CONT  TestRefHasher/7_segments_199_bytes
=== CONT  TestRefHasher/6_segments_171_bytes
=== CONT  TestRefHasher/6_segments_185_bytes
=== CONT  TestRefHasher/7_segments_198_bytes
=== CONT  TestRefHasher/7_segments_197_bytes
=== CONT  TestRefHasher/6_segments_184_bytes
=== CONT  TestRefHasher/7_segments_196_bytes
=== CONT  TestRefHasher/6_segments_183_bytes
=== CONT  TestRefHasher/7_segments_195_bytes
=== CONT  TestRefHasher/7_segments_194_bytes
=== CONT  TestRefHasher/6_segments_182_bytes
=== CONT  TestRefHasher/7_segments_193_bytes
=== CONT  TestRefHasher/6_segments_181_bytes
=== CONT  TestRefHasher/7_segments_192_bytes
=== CONT  TestRefHasher/7_segments_191_bytes
=== CONT  TestRefHasher/6_segments_180_bytes
=== CONT  TestRefHasher/6_segments_179_bytes
=== CONT  TestRefHasher/7_segments_190_bytes
=== CONT  TestRefHasher/6_segments_178_bytes
=== CONT  TestRefHasher/6_segments_177_bytes
=== CONT  TestRefHasher/7_segments_188_bytes
=== CONT  TestRefHasher/6_segments_168_bytes
=== CONT  TestRefHasher/6_segments_176_bytes
=== CONT  TestRefHasher/7_segments_187_bytes
=== CONT  TestRefHasher/6_segments_167_bytes
=== CONT  TestRefHasher/7_segments_189_bytes
=== CONT  TestRefHasher/6_segments_166_bytes
=== CONT  TestRefHasher/7_segments_54_bytes
=== CONT  TestRefHasher/6_segments_165_bytes
=== CONT  TestRefHasher/7_segments_53_bytes
=== CONT  TestRefHasher/6_segments_164_bytes
=== CONT  TestRefHasher/7_segments_52_bytes
=== CONT  TestRefHasher/6_segments_163_bytes
=== CONT  TestRefHasher/7_segments_51_bytes
=== CONT  TestRefHasher/6_segments_162_bytes
=== CONT  TestRefHasher/7_segments_50_bytes
=== CONT  TestRefHasher/6_segments_161_bytes
=== CONT  TestRefHasher/7_segments_49_bytes
=== CONT  TestRefHasher/7_segments_48_bytes
=== CONT  TestRefHasher/6_segments_160_bytes
=== CONT  TestRefHasher/7_segments_47_bytes
=== CONT  TestRefHasher/7_segments_46_bytes
=== CONT  TestRefHasher/6_segments_159_bytes
=== CONT  TestRefHasher/7_segments_45_bytes
=== CONT  TestRefHasher/7_segments_44_bytes
=== CONT  TestRefHasher/6_segments_29_bytes
=== CONT  TestRefHasher/7_segments_43_bytes
=== CONT  TestRefHasher/6_segments_157_bytes
=== CONT  TestRefHasher/6_segments_74_bytes
=== CONT  TestRefHasher/6_segments_156_bytes
=== CONT  TestRefHasher/7_segments_42_bytes
=== CONT  TestRefHasher/6_segments_155_bytes
=== CONT  TestRefHasher/7_segments_41_bytes
=== CONT  TestRefHasher/7_segments_40_bytes
=== CONT  TestRefHasher/6_segments_154_bytes
=== CONT  TestRefHasher/7_segments_39_bytes
=== CONT  TestRefHasher/6_segments_153_bytes
=== CONT  TestRefHasher/7_segments_38_bytes
=== CONT  TestRefHasher/6_segments_152_bytes
=== CONT  TestRefHasher/7_segments_37_bytes
=== CONT  TestRefHasher/6_segments_151_bytes
=== CONT  TestRefHasher/6_segments_150_bytes
=== CONT  TestRefHasher/6_segments_149_bytes
=== CONT  TestRefHasher/6_segments_148_bytes
=== CONT  TestRefHasher/6_segments_147_bytes
=== CONT  TestRefHasher/6_segments_73_bytes
=== CONT  TestRefHasher/4_segments_29_bytes
=== CONT  TestRefHasher/6_segments_72_bytes
=== CONT  TestRefHasher/6_segments_35_bytes
=== CONT  TestRefHasher/6_segments_71_bytes
=== CONT  TestRefHasher/4_segments_28_bytes
=== CONT  TestRefHasher/6_segments_146_bytes
=== CONT  TestRefHasher/6_segments_70_bytes
=== CONT  TestRefHasher/4_segments_27_bytes
=== CONT  TestRefHasher/6_segments_145_bytes
=== CONT  TestRefHasher/6_segments_69_bytes
=== CONT  TestRefHasher/4_segments_26_bytes
=== CONT  TestRefHasher/6_segments_144_bytes
=== CONT  TestRefHasher/7_segments_179_bytes
=== CONT  TestRefHasher/8_segments_80_bytes
=== CONT  TestRefHasher/6_segments_68_bytes
=== CONT  TestRefHasher/4_segments_25_bytes
=== CONT  TestRefHasher/8_segments_86_bytes
=== CONT  TestRefHasher/7_segments_216_bytes
=== CONT  TestRefHasher/6_segments_67_bytes
=== CONT  TestRefHasher/6_segments_143_bytes
=== CONT  TestRefHasher/6_segments_65_bytes
=== CONT  TestRefHasher/4_segments_24_bytes
=== CONT  TestRefHasher/4_segments_23_bytes
=== CONT  TestRefHasher/6_segments_141_bytes
=== CONT  TestRefHasher/6_segments_66_bytes
=== CONT  TestRefHasher/6_segments_142_bytes
=== CONT  TestRefHasher/6_segments_63_bytes
=== CONT  TestRefHasher/6_segments_64_bytes
=== CONT  TestRefHasher/6_segments_140_bytes
=== CONT  TestRefHasher/6_segments_139_bytes
=== CONT  TestRefHasher/4_segments_22_bytes
=== CONT  TestRefHasher/4_segments_21_bytes
=== CONT  TestRefHasher/6_segments_61_bytes
=== CONT  TestRefHasher/6_segments_118_bytes
=== CONT  TestRefHasher/4_segments_20_bytes
=== CONT  TestRefHasher/6_segments_117_bytes
=== CONT  TestRefHasher/6_segments_60_bytes
=== CONT  TestRefHasher/6_segments_138_bytes
=== CONT  TestRefHasher/6_segments_59_bytes
=== CONT  TestRefHasher/4_segments_19_bytes
=== CONT  TestRefHasher/6_segments_58_bytes
=== CONT  TestRefHasher/4_segments_18_bytes
=== CONT  TestRefHasher/6_segments_137_bytes
=== CONT  TestRefHasher/6_segments_62_bytes
=== CONT  TestRefHasher/4_segments_17_bytes
=== CONT  TestRefHasher/6_segments_116_bytes
=== CONT  TestRefHasher/6_segments_57_bytes
=== CONT  TestRefHasher/6_segments_114_bytes
=== CONT  TestRefHasher/6_segments_115_bytes
=== CONT  TestRefHasher/4_segments_16_bytes
=== CONT  TestRefHasher/6_segments_136_bytes
=== CONT  TestRefHasher/4_segments_15_bytes
=== CONT  TestRefHasher/6_segments_55_bytes
=== CONT  TestRefHasher/6_segments_56_bytes
=== CONT  TestRefHasher/6_segments_113_bytes
=== CONT  TestRefHasher/6_segments_54_bytes
=== CONT  TestRefHasher/4_segments_14_bytes
=== CONT  TestRefHasher/6_segments_112_bytes
=== CONT  TestRefHasher/6_segments_52_bytes
=== CONT  TestRefHasher/6_segments_135_bytes
=== CONT  TestRefHasher/4_segments_13_bytes
=== CONT  TestRefHasher/6_segments_53_bytes
=== CONT  TestRefHasher/6_segments_111_bytes
=== CONT  TestRefHasher/6_segments_51_bytes
=== CONT  TestRefHasher/4_segments_11_bytes
=== CONT  TestRefHasher/6_segments_110_bytes
=== CONT  TestRefHasher/4_segments_12_bytes
=== CONT  TestRefHasher/6_segments_109_bytes
=== CONT  TestRefHasher/4_segments_9_bytes
=== CONT  TestRefHasher/4_segments_10_bytes
=== CONT  TestRefHasher/6_segments_134_bytes
=== CONT  TestRefHasher/6_segments_50_bytes
=== CONT  TestRefHasher/6_segments_49_bytes
=== CONT  TestRefHasher/6_segments_48_bytes
=== CONT  TestRefHasher/6_segments_108_bytes
=== CONT  TestRefHasher/4_segments_8_bytes
=== CONT  TestRefHasher/6_segments_132_bytes
=== CONT  TestRefHasher/6_segments_107_bytes
=== CONT  TestRefHasher/6_segments_47_bytes
=== CONT  TestRefHasher/4_segments_7_bytes
=== CONT  TestRefHasher/6_segments_133_bytes
=== CONT  TestRefHasher/6_segments_46_bytes
=== CONT  TestRefHasher/6_segments_131_bytes
=== CONT  TestRefHasher/6_segments_106_bytes
=== CONT  TestRefHasher/6_segments_45_bytes
=== CONT  TestRefHasher/4_segments_6_bytes
=== CONT  TestRefHasher/6_segments_105_bytes
=== CONT  TestRefHasher/6_segments_44_bytes
=== CONT  TestRefHasher/6_segments_104_bytes
=== CONT  TestRefHasher/6_segments_130_bytes
=== CONT  TestRefHasher/4_segments_5_bytes
=== CONT  TestRefHasher/6_segments_129_bytes
=== CONT  TestRefHasher/6_segments_103_bytes
=== CONT  TestRefHasher/6_segments_128_bytes
=== CONT  TestRefHasher/6_segments_43_bytes
=== CONT  TestRefHasher/6_segments_42_bytes
=== CONT  TestRefHasher/6_segments_102_bytes
=== CONT  TestRefHasher/4_segments_3_bytes
=== CONT  TestRefHasher/6_segments_127_bytes
=== CONT  TestRefHasher/4_segments_4_bytes
=== CONT  TestRefHasher/6_segments_41_bytes
=== CONT  TestRefHasher/6_segments_33_bytes
=== CONT  TestRefHasher/6_segments_101_bytes
=== CONT  TestRefHasher/6_segments_40_bytes
=== CONT  TestRefHasher/6_segments_125_bytes
=== CONT  TestRefHasher/6_segments_124_bytes
=== CONT  TestRefHasher/6_segments_126_bytes
=== CONT  TestRefHasher/6_segments_39_bytes
=== CONT  TestRefHasher/6_segments_38_bytes
=== CONT  TestRefHasher/6_segments_123_bytes
=== CONT  TestRefHasher/6_segments_37_bytes
=== CONT  TestRefHasher/4_segments_2_bytes
=== CONT  TestRefHasher/6_segments_122_bytes
=== CONT  TestRefHasher/6_segments_121_bytes
=== CONT  TestRefHasher/6_segments_120_bytes
=== CONT  TestRefHasher/6_segments_36_bytes
=== CONT  TestRefHasher/6_segments_32_bytes
=== CONT  TestRefHasher/6_segments_119_bytes
=== CONT  TestRefHasher/7_segments_170_bytes
=== CONT  TestRefHasher/7_segments_169_bytes
=== CONT  TestRefHasher/7_segments_168_bytes
=== CONT  TestRefHasher/6_segments_100_bytes
=== CONT  TestRefHasher/7_segments_172_bytes
=== CONT  TestRefHasher/7_segments_167_bytes
=== CONT  TestRefHasher/7_segments_171_bytes
=== CONT  TestRefHasher/7_segments_165_bytes
=== CONT  TestRefHasher/6_segments_99_bytes
=== CONT  TestRefHasher/6_segments_97_bytes
=== CONT  TestRefHasher/6_segments_96_bytes
=== CONT  TestRefHasher/7_segments_164_bytes
=== CONT  TestRefHasher/6_segments_98_bytes
=== CONT  TestRefHasher/6_segments_95_bytes
=== CONT  TestRefHasher/6_segments_94_bytes
=== CONT  TestRefHasher/6_segments_93_bytes
=== CONT  TestRefHasher/7_segments_163_bytes
=== CONT  TestRefHasher/6_segments_92_bytes
=== CONT  TestRefHasher/6_segments_91_bytes
=== CONT  TestRefHasher/7_segments_160_bytes
=== CONT  TestRefHasher/6_segments_89_bytes
=== CONT  TestRefHasher/7_segments_161_bytes
=== CONT  TestRefHasher/7_segments_162_bytes
=== CONT  TestRefHasher/6_segments_31_bytes
=== CONT  TestRefHasher/7_segments_159_bytes
=== CONT  TestRefHasher/6_segments_88_bytes
=== CONT  TestRefHasher/6_segments_90_bytes
=== CONT  TestRefHasher/6_segments_30_bytes
=== CONT  TestRefHasher/6_segments_87_bytes
=== CONT  TestRefHasher/7_segments_158_bytes
=== CONT  TestRefHasher/5_segments_60_bytes
=== CONT  TestRefHasher/6_segments_86_bytes
=== CONT  TestRefHasher/6_segments_28_bytes
=== CONT  TestRefHasher/7_segments_156_bytes
=== CONT  TestRefHasher/6_segments_27_bytes
=== CONT  TestRefHasher/6_segments_26_bytes
=== CONT  TestRefHasher/7_segments_154_bytes
=== CONT  TestRefHasher/6_segments_24_bytes
=== CONT  TestRefHasher/7_segments_157_bytes
=== CONT  TestRefHasher/6_segments_23_bytes
=== CONT  TestRefHasher/6_segments_25_bytes
=== CONT  TestRefHasher/7_segments_152_bytes
=== CONT  TestRefHasher/7_segments_153_bytes
=== CONT  TestRefHasher/6_segments_22_bytes
=== CONT  TestRefHasher/7_segments_155_bytes
=== CONT  TestRefHasher/7_segments_151_bytes
=== CONT  TestRefHasher/5_segments_158_bytes
=== CONT  TestRefHasher/7_segments_150_bytes
=== CONT  TestRefHasher/5_segments_157_bytes
=== CONT  TestRefHasher/7_segments_149_bytes
=== CONT  TestRefHasher/6_segments_20_bytes
=== CONT  TestRefHasher/6_segments_21_bytes
=== CONT  TestRefHasher/7_segments_148_bytes
=== CONT  TestRefHasher/5_segments_156_bytes
=== CONT  TestRefHasher/6_segments_19_bytes
=== CONT  TestRefHasher/5_segments_155_bytes
=== CONT  TestRefHasher/7_segments_147_bytes
=== CONT  TestRefHasher/6_segments_18_bytes
=== CONT  TestRefHasher/5_segments_154_bytes
=== CONT  TestRefHasher/6_segments_17_bytes
=== CONT  TestRefHasher/7_segments_145_bytes
=== CONT  TestRefHasher/5_segments_153_bytes
=== CONT  TestRefHasher/6_segments_16_bytes
=== CONT  TestRefHasher/5_segments_129_bytes
=== CONT  TestRefHasher/7_segments_146_bytes
=== CONT  TestRefHasher/7_segments_144_bytes
=== CONT  TestRefHasher/5_segments_152_bytes
=== CONT  TestRefHasher/5_segments_128_bytes
=== CONT  TestRefHasher/7_segments_143_bytes
=== CONT  TestRefHasher/6_segments_85_bytes
=== CONT  TestRefHasher/7_segments_142_bytes
=== CONT  TestRefHasher/5_segments_151_bytes
=== CONT  TestRefHasher/6_segments_14_bytes
=== CONT  TestRefHasher/7_segments_141_bytes
=== CONT  TestRefHasher/6_segments_84_bytes
=== CONT  TestRefHasher/5_segments_150_bytes
=== CONT  TestRefHasher/6_segments_15_bytes
=== CONT  TestRefHasher/6_segments_13_bytes
=== CONT  TestRefHasher/7_segments_166_bytes
=== CONT  TestRefHasher/7_segments_140_bytes
=== CONT  TestRefHasher/5_segments_149_bytes
=== CONT  TestRefHasher/7_segments_139_bytes
=== CONT  TestRefHasher/6_segments_12_bytes
=== CONT  TestRefHasher/6_segments_82_bytes
=== CONT  TestRefHasher/7_segments_138_bytes
=== CONT  TestRefHasher/5_segments_148_bytes
=== CONT  TestRefHasher/6_segments_81_bytes
=== CONT  TestRefHasher/7_segments_137_bytes
=== CONT  TestRefHasher/5_segments_147_bytes
=== CONT  TestRefHasher/6_segments_80_bytes
=== CONT  TestRefHasher/7_segments_136_bytes
=== CONT  TestRefHasher/5_segments_146_bytes
=== CONT  TestRefHasher/6_segments_79_bytes
=== CONT  TestRefHasher/7_segments_135_bytes
=== CONT  TestRefHasher/5_segments_145_bytes
=== CONT  TestRefHasher/7_segments_134_bytes
=== CONT  TestRefHasher/5_segments_144_bytes
=== CONT  TestRefHasher/6_segments_78_bytes
=== CONT  TestRefHasher/7_segments_133_bytes
=== CONT  TestRefHasher/5_segments_143_bytes
=== CONT  TestRefHasher/6_segments_77_bytes
=== CONT  TestRefHasher/7_segments_132_bytes
=== CONT  TestRefHasher/5_segments_142_bytes
=== CONT  TestRefHasher/7_segments_131_bytes
=== CONT  TestRefHasher/6_segments_76_bytes
=== CONT  TestRefHasher/7_segments_130_bytes
=== CONT  TestRefHasher/5_segments_141_bytes
=== CONT  TestRefHasher/6_segments_75_bytes
=== CONT  TestRefHasher/7_segments_129_bytes
=== CONT  TestRefHasher/5_segments_140_bytes
=== CONT  TestRefHasher/7_segments_128_bytes
=== CONT  TestRefHasher/5_segments_139_bytes
=== CONT  TestRefHasher/5_segments_138_bytes
=== CONT  TestRefHasher/7_segments_126_bytes
=== CONT  TestRefHasher/5_segments_137_bytes
=== CONT  TestRefHasher/7_segments_125_bytes
=== CONT  TestRefHasher/5_segments_136_bytes
=== CONT  TestRefHasher/7_segments_124_bytes
=== CONT  TestRefHasher/5_segments_135_bytes
=== CONT  TestRefHasher/7_segments_122_bytes
=== CONT  TestRefHasher/7_segments_127_bytes
=== CONT  TestRefHasher/6_segments_10_bytes
=== CONT  TestRefHasher/7_segments_121_bytes
=== CONT  TestRefHasher/5_segments_127_bytes
=== CONT  TestRefHasher/6_segments_9_bytes
=== CONT  TestRefHasher/5_segments_133_bytes
=== CONT  TestRefHasher/7_segments_120_bytes
=== CONT  TestRefHasher/7_segments_123_bytes
=== CONT  TestRefHasher/6_segments_11_bytes
=== CONT  TestRefHasher/5_segments_134_bytes
=== CONT  TestRefHasher/5_segments_126_bytes
=== CONT  TestRefHasher/7_segments_119_bytes
=== CONT  TestRefHasher/6_segments_7_bytes
=== CONT  TestRefHasher/5_segments_132_bytes
=== CONT  TestRefHasher/6_segments_83_bytes
=== CONT  TestRefHasher/7_segments_118_bytes
=== CONT  TestRefHasher/6_segments_6_bytes
=== CONT  TestRefHasher/5_segments_125_bytes
=== CONT  TestRefHasher/7_segments_117_bytes
=== CONT  TestRefHasher/5_segments_131_bytes
=== CONT  TestRefHasher/6_segments_8_bytes
=== CONT  TestRefHasher/6_segments_5_bytes
=== CONT  TestRefHasher/5_segments_124_bytes
=== CONT  TestRefHasher/7_segments_116_bytes
=== CONT  TestRefHasher/5_segments_130_bytes
=== CONT  TestRefHasher/5_segments_123_bytes
=== CONT  TestRefHasher/6_segments_4_bytes
=== CONT  TestRefHasher/6_segments_3_bytes
=== CONT  TestRefHasher/7_segments_115_bytes
=== CONT  TestRefHasher/5_segments_122_bytes
=== CONT  TestRefHasher/6_segments_2_bytes
=== CONT  TestRefHasher/5_segments_121_bytes
=== CONT  TestRefHasher/7_segments_113_bytes
=== CONT  TestRefHasher/5_segments_120_bytes
=== CONT  TestRefHasher/7_segments_114_bytes
=== CONT  TestRefHasher/5_segments_160_bytes
=== CONT  TestRefHasher/6_segments_1_bytes
=== CONT  TestRefHasher/5_segments_119_bytes
=== CONT  TestRefHasher/5_segments_102_bytes
=== CONT  TestRefHasher/5_segments_118_bytes
=== CONT  TestRefHasher/4_segments_1_bytes
=== CONT  TestRefHasher/5_segments_101_bytes
=== CONT  TestRefHasher/5_segments_117_bytes
=== CONT  TestRefHasher/3_segments_96_bytes
=== CONT  TestRefHasher/5_segments_100_bytes
=== CONT  TestRefHasher/5_segments_116_bytes
=== CONT  TestRefHasher/3_segments_95_bytes
=== CONT  TestRefHasher/5_segments_80_bytes
=== CONT  TestRefHasher/5_segments_115_bytes
=== CONT  TestRefHasher/5_segments_159_bytes
=== CONT  TestRefHasher/5_segments_79_bytes
=== CONT  TestRefHasher/5_segments_114_bytes
=== CONT  TestRefHasher/3_segments_94_bytes
=== CONT  TestRefHasher/5_segments_99_bytes
=== CONT  TestRefHasher/5_segments_78_bytes
=== CONT  TestRefHasher/3_segments_93_bytes
=== CONT  TestRefHasher/5_segments_77_bytes
=== CONT  TestRefHasher/5_segments_113_bytes
=== CONT  TestRefHasher/3_segments_92_bytes
=== CONT  TestRefHasher/5_segments_98_bytes
=== CONT  TestRefHasher/5_segments_112_bytes
=== CONT  TestRefHasher/5_segments_76_bytes
=== CONT  TestRefHasher/5_segments_61_bytes
=== CONT  TestRefHasher/5_segments_111_bytes
=== CONT  TestRefHasher/3_segments_90_bytes
=== CONT  TestRefHasher/5_segments_97_bytes
=== CONT  TestRefHasher/5_segments_74_bytes
=== CONT  TestRefHasher/5_segments_110_bytes
=== CONT  TestRefHasher/5_segments_96_bytes
=== CONT  TestRefHasher/3_segments_89_bytes
=== CONT  TestRefHasher/5_segments_109_bytes
=== CONT  TestRefHasher/5_segments_95_bytes
=== CONT  TestRefHasher/5_segments_94_bytes
=== CONT  TestRefHasher/3_segments_91_bytes
=== CONT  TestRefHasher/5_segments_92_bytes
=== CONT  TestRefHasher/5_segments_91_bytes
=== CONT  TestRefHasher/5_segments_90_bytes
=== CONT  TestRefHasher/5_segments_89_bytes
=== CONT  TestRefHasher/5_segments_72_bytes
=== CONT  TestRefHasher/5_segments_88_bytes
=== CONT  TestRefHasher/5_segments_70_bytes
=== CONT  TestRefHasher/5_segments_93_bytes
=== CONT  TestRefHasher/5_segments_69_bytes
=== CONT  TestRefHasher/5_segments_73_bytes
=== CONT  TestRefHasher/5_segments_67_bytes
=== CONT  TestRefHasher/5_segments_71_bytes
=== CONT  TestRefHasher/5_segments_65_bytes
=== CONT  TestRefHasher/5_segments_64_bytes
=== CONT  TestRefHasher/5_segments_68_bytes
=== CONT  TestRefHasher/5_segments_66_bytes
=== CONT  TestRefHasher/5_segments_63_bytes
=== CONT  TestRefHasher/5_segments_62_bytes
=== CONT  TestRefHasher/5_segments_75_bytes
=== CONT  TestRefHasher/5_segments_59_bytes
=== CONT  TestRefHasher/5_segments_58_bytes
=== CONT  TestRefHasher/5_segments_57_bytes
=== CONT  TestRefHasher/5_segments_56_bytes
=== CONT  TestRefHasher/5_segments_55_bytes
=== CONT  TestRefHasher/5_segments_53_bytes
=== CONT  TestRefHasher/5_segments_52_bytes
=== CONT  TestRefHasher/5_segments_51_bytes
=== CONT  TestRefHasher/5_segments_50_bytes
=== CONT  TestRefHasher/5_segments_49_bytes
=== CONT  TestRefHasher/5_segments_48_bytes
=== CONT  TestRefHasher/5_segments_47_bytes
=== CONT  TestRefHasher/5_segments_46_bytes
=== CONT  TestRefHasher/5_segments_45_bytes
=== CONT  TestRefHasher/5_segments_44_bytes
=== CONT  TestRefHasher/5_segments_43_bytes
=== CONT  TestRefHasher/5_segments_42_bytes
=== CONT  TestRefHasher/5_segments_87_bytes
=== CONT  TestRefHasher/5_segments_41_bytes
=== CONT  TestRefHasher/5_segments_40_bytes
=== CONT  TestRefHasher/5_segments_86_bytes
=== CONT  TestRefHasher/5_segments_54_bytes
=== CONT  TestRefHasher/5_segments_38_bytes
=== CONT  TestRefHasher/5_segments_85_bytes
=== CONT  TestRefHasher/5_segments_36_bytes
=== CONT  TestRefHasher/5_segments_84_bytes
=== CONT  TestRefHasher/5_segments_83_bytes
=== CONT  TestRefHasher/5_segments_27_bytes
=== CONT  TestRefHasher/5_segments_82_bytes
=== CONT  TestRefHasher/5_segments_35_bytes
=== CONT  TestRefHasher/5_segments_34_bytes
=== CONT  TestRefHasher/5_segments_81_bytes
=== CONT  TestRefHasher/5_segments_108_bytes
=== CONT  TestRefHasher/5_segments_33_bytes
=== CONT  TestRefHasher/5_segments_107_bytes
=== CONT  TestRefHasher/5_segments_106_bytes
=== CONT  TestRefHasher/5_segments_105_bytes
=== CONT  TestRefHasher/5_segments_25_bytes
=== CONT  TestRefHasher/5_segments_39_bytes
=== CONT  TestRefHasher/5_segments_32_bytes
=== CONT  TestRefHasher/5_segments_104_bytes
=== CONT  TestRefHasher/5_segments_37_bytes
=== CONT  TestRefHasher/5_segments_24_bytes
=== CONT  TestRefHasher/5_segments_26_bytes
=== CONT  TestRefHasher/5_segments_103_bytes
=== CONT  TestRefHasher/5_segments_23_bytes
=== CONT  TestRefHasher/5_segments_30_bytes
=== CONT  TestRefHasher/5_segments_29_bytes
=== CONT  TestRefHasher/5_segments_21_bytes
=== CONT  TestRefHasher/5_segments_13_bytes
=== CONT  TestRefHasher/5_segments_20_bytes
=== CONT  TestRefHasher/5_segments_12_bytes
=== CONT  TestRefHasher/5_segments_31_bytes
=== CONT  TestRefHasher/5_segments_11_bytes
=== CONT  TestRefHasher/5_segments_2_bytes
=== CONT  TestRefHasher/5_segments_19_bytes
=== CONT  TestRefHasher/5_segments_10_bytes
=== CONT  TestRefHasher/5_segments_1_bytes
=== CONT  TestRefHasher/5_segments_18_bytes
=== CONT  TestRefHasher/5_segments_9_bytes
=== CONT  TestRefHasher/5_segments_8_bytes
=== CONT  TestRefHasher/5_segments_7_bytes
=== CONT  TestRefHasher/4_segments_128_bytes
=== CONT  TestRefHasher/5_segments_22_bytes
=== CONT  TestRefHasher/5_segments_16_bytes
=== CONT  TestRefHasher/4_segments_127_bytes
=== CONT  TestRefHasher/5_segments_28_bytes
=== CONT  TestRefHasher/5_segments_17_bytes
=== CONT  TestRefHasher/5_segments_6_bytes
=== CONT  TestRefHasher/4_segments_126_bytes
=== CONT  TestRefHasher/5_segments_15_bytes
=== CONT  TestRefHasher/4_segments_125_bytes
=== CONT  TestRefHasher/5_segments_5_bytes
=== CONT  TestRefHasher/4_segments_124_bytes
=== CONT  TestRefHasher/4_segments_123_bytes
=== CONT  TestRefHasher/5_segments_14_bytes
=== CONT  TestRefHasher/4_segments_122_bytes
=== CONT  TestRefHasher/5_segments_4_bytes
=== CONT  TestRefHasher/5_segments_3_bytes
=== CONT  TestRefHasher/4_segments_121_bytes
=== CONT  TestRefHasher/4_segments_120_bytes
=== CONT  TestRefHasher/4_segments_119_bytes
=== CONT  TestRefHasher/3_segments_88_bytes
=== CONT  TestRefHasher/4_segments_112_bytes
=== CONT  TestRefHasher/4_segments_118_bytes
=== CONT  TestRefHasher/4_segments_117_bytes
=== CONT  TestRefHasher/4_segments_110_bytes
=== CONT  TestRefHasher/3_segments_87_bytes
=== CONT  TestRefHasher/4_segments_109_bytes
=== CONT  TestRefHasher/4_segments_108_bytes
=== CONT  TestRefHasher/4_segments_107_bytes
=== CONT  TestRefHasher/3_segments_85_bytes
=== CONT  TestRefHasher/4_segments_106_bytes
=== CONT  TestRefHasher/3_segments_84_bytes
=== CONT  TestRefHasher/4_segments_113_bytes
=== CONT  TestRefHasher/3_segments_82_bytes
=== CONT  TestRefHasher/4_segments_104_bytes
=== CONT  TestRefHasher/3_segments_81_bytes
=== CONT  TestRefHasher/4_segments_103_bytes
=== CONT  TestRefHasher/4_segments_116_bytes
=== CONT  TestRefHasher/3_segments_80_bytes
=== CONT  TestRefHasher/3_segments_79_bytes
=== CONT  TestRefHasher/4_segments_102_bytes
=== CONT  TestRefHasher/4_segments_98_bytes
=== CONT  TestRefHasher/3_segments_78_bytes
=== CONT  TestRefHasher/4_segments_101_bytes
=== CONT  TestRefHasher/3_segments_77_bytes
=== CONT  TestRefHasher/4_segments_100_bytes
=== CONT  TestRefHasher/3_segments_76_bytes
=== CONT  TestRefHasher/4_segments_97_bytes
=== CONT  TestRefHasher/3_segments_75_bytes
=== CONT  TestRefHasher/4_segments_96_bytes
=== CONT  TestRefHasher/3_segments_74_bytes
=== CONT  TestRefHasher/4_segments_93_bytes
=== CONT  TestRefHasher/3_segments_73_bytes
=== CONT  TestRefHasher/4_segments_92_bytes
=== CONT  TestRefHasher/4_segments_95_bytes
=== CONT  TestRefHasher/3_segments_72_bytes
=== CONT  TestRefHasher/4_segments_91_bytes
=== CONT  TestRefHasher/4_segments_94_bytes
=== CONT  TestRefHasher/3_segments_71_bytes
=== CONT  TestRefHasher/4_segments_90_bytes
=== CONT  TestRefHasher/3_segments_70_bytes
=== CONT  TestRefHasher/3_segments_69_bytes
=== CONT  TestRefHasher/4_segments_89_bytes
=== CONT  TestRefHasher/4_segments_88_bytes
=== CONT  TestRefHasher/3_segments_68_bytes
=== CONT  TestRefHasher/4_segments_84_bytes
=== CONT  TestRefHasher/4_segments_87_bytes
=== CONT  TestRefHasher/3_segments_67_bytes
=== CONT  TestRefHasher/4_segments_83_bytes
=== CONT  TestRefHasher/4_segments_86_bytes
=== CONT  TestRefHasher/3_segments_66_bytes
=== CONT  TestRefHasher/4_segments_82_bytes
=== CONT  TestRefHasher/3_segments_65_bytes
=== CONT  TestRefHasher/4_segments_85_bytes
=== CONT  TestRefHasher/3_segments_64_bytes
=== CONT  TestRefHasher/4_segments_81_bytes
=== CONT  TestRefHasher/3_segments_63_bytes
=== CONT  TestRefHasher/4_segments_80_bytes
=== CONT  TestRefHasher/3_segments_62_bytes
=== CONT  TestRefHasher/4_segments_79_bytes
=== CONT  TestRefHasher/4_segments_77_bytes
=== CONT  TestRefHasher/3_segments_61_bytes
=== CONT  TestRefHasher/4_segments_114_bytes
=== CONT  TestRefHasher/3_segments_60_bytes
=== CONT  TestRefHasher/4_segments_99_bytes
=== CONT  TestRefHasher/4_segments_78_bytes
=== CONT  TestRefHasher/3_segments_59_bytes
=== CONT  TestRefHasher/4_segments_75_bytes
=== CONT  TestRefHasher/4_segments_74_bytes
=== CONT  TestRefHasher/3_segments_58_bytes
=== CONT  TestRefHasher/4_segments_71_bytes
=== CONT  TestRefHasher/4_segments_73_bytes
=== CONT  TestRefHasher/4_segments_70_bytes
=== CONT  TestRefHasher/4_segments_72_bytes
=== CONT  TestRefHasher/3_segments_57_bytes
=== CONT  TestRefHasher/4_segments_69_bytes
=== CONT  TestRefHasher/4_segments_67_bytes
=== CONT  TestRefHasher/3_segments_55_bytes
=== CONT  TestRefHasher/4_segments_68_bytes
=== CONT  TestRefHasher/3_segments_54_bytes
=== CONT  TestRefHasher/3_segments_83_bytes
=== CONT  TestRefHasher/3_segments_56_bytes
=== CONT  TestRefHasher/4_segments_66_bytes
=== CONT  TestRefHasher/3_segments_86_bytes
=== CONT  TestRefHasher/4_segments_111_bytes
=== CONT  TestRefHasher/4_segments_105_bytes
=== CONT  TestRefHasher/4_segments_115_bytes
=== CONT  TestRefHasher/4_segments_76_bytes
=== CONT  TestRefHasher/4_segments_63_bytes
=== CONT  TestRefHasher/3_segments_1_bytes
=== CONT  TestRefHasher/1_segments_31_bytes
=== CONT  TestRefHasher/3_segments_50_bytes
=== CONT  TestRefHasher/4_segments_65_bytes
=== CONT  TestRefHasher/4_segments_62_bytes
=== CONT  TestRefHasher/3_segments_48_bytes
=== CONT  TestRefHasher/2_segments_62_bytes
=== CONT  TestRefHasher/4_segments_61_bytes
=== CONT  TestRefHasher/2_segments_61_bytes
=== CONT  TestRefHasher/3_segments_45_bytes
=== CONT  TestRefHasher/2_segments_60_bytes
=== CONT  TestRefHasher/3_segments_47_bytes
=== CONT  TestRefHasher/3_segments_44_bytes
=== CONT  TestRefHasher/4_segments_60_bytes
=== CONT  TestRefHasher/2_segments_59_bytes
=== CONT  TestRefHasher/1_segments_29_bytes
=== CONT  TestRefHasher/3_segments_43_bytes
=== CONT  TestRefHasher/3_segments_42_bytes
=== CONT  TestRefHasher/2_segments_58_bytes
=== CONT  TestRefHasher/1_segments_28_bytes
=== CONT  TestRefHasher/3_segments_41_bytes
=== CONT  TestRefHasher/2_segments_57_bytes
=== CONT  TestRefHasher/3_segments_26_bytes
=== CONT  TestRefHasher/1_segments_27_bytes
=== CONT  TestRefHasher/2_segments_56_bytes
=== CONT  TestRefHasher/3_segments_52_bytes
=== CONT  TestRefHasher/3_segments_40_bytes
=== CONT  TestRefHasher/1_segments_26_bytes
=== CONT  TestRefHasher/2_segments_55_bytes
=== CONT  TestRefHasher/1_segments_25_bytes
=== CONT  TestRefHasher/3_segments_39_bytes
=== CONT  TestRefHasher/1_segments_32_bytes
=== CONT  TestRefHasher/3_segments_51_bytes
=== CONT  TestRefHasher/2_segments_64_bytes
=== CONT  TestRefHasher/3_segments_53_bytes
=== CONT  TestRefHasher/1_segments_30_bytes
=== CONT  TestRefHasher/4_segments_64_bytes
=== CONT  TestRefHasher/3_segments_38_bytes
=== CONT  TestRefHasher/2_segments_54_bytes
=== CONT  TestRefHasher/1_segments_24_bytes
=== CONT  TestRefHasher/2_segments_35_bytes
=== CONT  TestRefHasher/2_segments_34_bytes
=== CONT  TestRefHasher/2_segments_53_bytes
=== CONT  TestRefHasher/1_segments_23_bytes
=== CONT  TestRefHasher/3_segments_37_bytes
=== CONT  TestRefHasher/2_segments_33_bytes
=== CONT  TestRefHasher/2_segments_52_bytes
=== CONT  TestRefHasher/1_segments_22_bytes
=== CONT  TestRefHasher/3_segments_36_bytes
=== CONT  TestRefHasher/2_segments_32_bytes
=== CONT  TestRefHasher/2_segments_51_bytes
=== CONT  TestRefHasher/3_segments_35_bytes
=== CONT  TestRefHasher/2_segments_50_bytes
=== CONT  TestRefHasher/3_segments_34_bytes
=== CONT  TestRefHasher/2_segments_30_bytes
=== CONT  TestRefHasher/1_segments_21_bytes
=== CONT  TestRefHasher/2_segments_49_bytes
=== CONT  TestRefHasher/2_segments_29_bytes
=== CONT  TestRefHasher/3_segments_33_bytes
=== CONT  TestRefHasher/2_segments_48_bytes
=== CONT  TestRefHasher/1_segments_20_bytes
=== CONT  TestRefHasher/2_segments_28_bytes
=== CONT  TestRefHasher/2_segments_47_bytes
=== CONT  TestRefHasher/1_segments_19_bytes
=== CONT  TestRefHasher/3_segments_32_bytes
=== CONT  TestRefHasher/2_segments_46_bytes
=== CONT  TestRefHasher/2_segments_27_bytes
=== CONT  TestRefHasher/3_segments_31_bytes
=== CONT  TestRefHasher/1_segments_18_bytes
=== CONT  TestRefHasher/2_segments_45_bytes
=== CONT  TestRefHasher/2_segments_26_bytes
=== CONT  TestRefHasher/1_segments_17_bytes
=== CONT  TestRefHasher/3_segments_30_bytes
=== CONT  TestRefHasher/1_segments_16_bytes
=== CONT  TestRefHasher/2_segments_44_bytes
=== CONT  TestRefHasher/2_segments_25_bytes
=== CONT  TestRefHasher/1_segments_15_bytes
=== CONT  TestRefHasher/3_segments_29_bytes
=== CONT  TestRefHasher/3_segments_28_bytes
=== CONT  TestRefHasher/1_segments_14_bytes
=== CONT  TestRefHasher/2_segments_24_bytes
=== CONT  TestRefHasher/2_segments_23_bytes
=== CONT  TestRefHasher/3_segments_25_bytes
=== CONT  TestRefHasher/2_segments_40_bytes
=== CONT  TestRefHasher/2_segments_22_bytes
=== CONT  TestRefHasher/3_segments_24_bytes
=== CONT  TestRefHasher/2_segments_43_bytes
=== CONT  TestRefHasher/2_segments_21_bytes
=== CONT  TestRefHasher/3_segments_23_bytes
=== CONT  TestRefHasher/1_segments_13_bytes
=== CONT  TestRefHasher/3_segments_22_bytes
=== CONT  TestRefHasher/2_segments_20_bytes
=== CONT  TestRefHasher/1_segments_12_bytes
=== CONT  TestRefHasher/3_segments_21_bytes
=== CONT  TestRefHasher/2_segments_19_bytes
=== CONT  TestRefHasher/3_segments_19_bytes
=== CONT  TestRefHasher/1_segments_11_bytes
=== CONT  TestRefHasher/3_segments_20_bytes
=== CONT  TestRefHasher/3_segments_18_bytes
=== CONT  TestRefHasher/1_segments_10_bytes
=== CONT  TestRefHasher/2_segments_18_bytes
=== CONT  TestRefHasher/2_segments_42_bytes
=== CONT  TestRefHasher/1_segments_9_bytes
=== CONT  TestRefHasher/2_segments_17_bytes
=== CONT  TestRefHasher/2_segments_41_bytes
=== CONT  TestRefHasher/1_segments_8_bytes
=== CONT  TestRefHasher/3_segments_16_bytes
=== CONT  TestRefHasher/2_segments_38_bytes
=== CONT  TestRefHasher/2_segments_16_bytes
=== CONT  TestRefHasher/1_segments_7_bytes
=== CONT  TestRefHasher/2_segments_39_bytes
=== CONT  TestRefHasher/3_segments_15_bytes
=== CONT  TestRefHasher/1_segments_6_bytes
=== CONT  TestRefHasher/1_segments_4_bytes
=== CONT  TestRefHasher/2_segments_15_bytes
=== CONT  TestRefHasher/1_segments_5_bytes
=== CONT  TestRefHasher/1_segments_3_bytes
=== CONT  TestRefHasher/2_segments_14_bytes
=== CONT  TestRefHasher/7_segments_108_bytes
=== CONT  TestRefHasher/2_segments_13_bytes
=== CONT  TestRefHasher/3_segments_13_bytes
=== CONT  TestRefHasher/2_segments_37_bytes
=== CONT  TestRefHasher/7_segments_112_bytes
=== CONT  TestRefHasher/1_segments_2_bytes
=== CONT  TestRefHasher/3_segments_14_bytes
=== CONT  TestRefHasher/2_segments_36_bytes
=== CONT  TestRefHasher/2_segments_11_bytes
=== CONT  TestRefHasher/3_segments_11_bytes
=== CONT  TestRefHasher/2_segments_10_bytes
=== CONT  TestRefHasher/2_segments_9_bytes
=== CONT  TestRefHasher/7_segments_110_bytes
=== CONT  TestRefHasher/3_segments_10_bytes
=== CONT  TestRefHasher/7_segments_106_bytes
=== CONT  TestRefHasher/7_segments_105_bytes
=== CONT  TestRefHasher/2_segments_8_bytes
=== CONT  TestRefHasher/3_segments_9_bytes
=== CONT  TestRefHasher/7_segments_111_bytes
=== CONT  TestRefHasher/7_segments_103_bytes
=== CONT  TestRefHasher/3_segments_8_bytes
=== CONT  TestRefHasher/3_segments_7_bytes
=== CONT  TestRefHasher/2_segments_6_bytes
=== CONT  TestRefHasher/3_segments_6_bytes
=== CONT  TestRefHasher/2_segments_5_bytes
=== CONT  TestRefHasher/2_segments_4_bytes
=== CONT  TestRefHasher/3_segments_5_bytes
=== CONT  TestRefHasher/2_segments_3_bytes
=== CONT  TestRefHasher/7_segments_107_bytes
=== CONT  TestRefHasher/2_segments_2_bytes
=== CONT  TestRefHasher/3_segments_2_bytes
=== CONT  TestRefHasher/2_segments_1_bytes
=== CONT  TestRefHasher/3_segments_3_bytes
=== CONT  TestRefHasher/3_segments_4_bytes
=== CONT  TestRefHasher/3_segments_49_bytes
=== CONT  TestRefHasher/2_segments_31_bytes
=== CONT  TestRefHasher/3_segments_27_bytes
=== CONT  TestRefHasher/2_segments_63_bytes
=== CONT  TestRefHasher/3_segments_12_bytes
=== CONT  TestRefHasher/2_segments_7_bytes
=== CONT  TestRefHasher/3_segments_46_bytes
=== CONT  TestRefHasher/2_segments_12_bytes
=== CONT  TestRefHasher/4_segments_59_bytes
=== CONT  TestRefHasher/3_segments_17_bytes
=== CONT  TestRefHasher/7_segments_104_bytes
=== CONT  TestRefHasher/7_segments_109_bytes
=== CONT  TestRefHasher/6_segments_34_bytes
--- PASS: TestRefHasher (0.03s)
    --- PASS: TestRefHasher/1_segments_1_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_58_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_57_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_56_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_55_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_54_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_53_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_52_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_51_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_50_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_49_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_48_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_47_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_46_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_45_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_44_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_43_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_42_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_41_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_40_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_39_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_38_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_37_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_36_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_35_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_34_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_33_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_32_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_31_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_30_bytes (0.01s)
    --- PASS: TestRefHasher/8_segments_256_bytes (0.01s)
    --- PASS: TestRefHasher/8_segments_255_bytes (0.01s)
    --- PASS: TestRefHasher/8_segments_254_bytes (0.02s)
    --- PASS: TestRefHasher/8_segments_253_bytes (0.02s)
    --- PASS: TestRefHasher/8_segments_252_bytes (0.02s)
    --- PASS: TestRefHasher/8_segments_251_bytes (0.02s)
    --- PASS: TestRefHasher/8_segments_143_bytes (0.02s)
    --- PASS: TestRefHasher/8_segments_249_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_248_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_250_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_240_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_239_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_238_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_237_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_236_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_235_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_234_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_233_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_232_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_231_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_230_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_229_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_228_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_227_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_226_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_225_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_224_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_223_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_222_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_45_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_221_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_220_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_219_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_218_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_217_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_216_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_215_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_214_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_213_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_212_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_211_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_210_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_209_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_208_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_207_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_206_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_142_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_205_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_204_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_141_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_203_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_202_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_140_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_201_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_200_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_139_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_44_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_199_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_198_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_138_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_43_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_137_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_42_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_180_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_136_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_41_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_197_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_135_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_40_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_196_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_39_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_38_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_37_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_134_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_36_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_133_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_35_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_132_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_34_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_131_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_33_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_130_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_32_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_129_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_31_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_224_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_30_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_195_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_128_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_194_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_127_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_193_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_126_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_192_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_125_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_29_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_124_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_191_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_123_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_190_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_28_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_189_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_122_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_188_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_27_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_121_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_26_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_187_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_25_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_120_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_186_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_24_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_119_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_185_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_23_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_118_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_184_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_22_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_183_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_117_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_21_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_182_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_116_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_20_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_181_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_115_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_180_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_114_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_19_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_179_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_113_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_178_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_177_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_112_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_18_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_176_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_111_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_17_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_175_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_110_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_174_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_16_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_109_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_15_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_173_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_108_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_14_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_172_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_13_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_107_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_171_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_12_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_170_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_106_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_169_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_11_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_168_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_105_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_10_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_167_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_104_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_166_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_9_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_165_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_103_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_8_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_164_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_102_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_7_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_163_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_101_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_162_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_100_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_161_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_6_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_99_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_160_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_98_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_159_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_5_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_247_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_4_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_97_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_157_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_96_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_3_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_156_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_95_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_246_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_2_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_94_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_93_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_1_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_154_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_92_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_153_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_245_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_244_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_243_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_242_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_241_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_158_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_155_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_95_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_149_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_91_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_148_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_89_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_221_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_220_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_152_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_146_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_219_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_87_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_145_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_223_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_218_bytes (0.01s)
    --- PASS: TestRefHasher/8_segments_90_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_144_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_217_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_147_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_85_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_215_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_84_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_214_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_213_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_83_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_212_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_82_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_181_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_81_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_151_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_211_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_210_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_79_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_209_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_78_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_77_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_178_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_76_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_177_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_75_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_176_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_74_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_175_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_73_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_174_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_72_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_71_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_173_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_70_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_69_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_68_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_67_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_66_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_65_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_64_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_63_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_62_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_61_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_60_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_59_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_58_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_57_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_56_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_55_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_54_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_53_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_52_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_51_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_50_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_49_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_48_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_47_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_46_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_36_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_35_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_34_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_33_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_32_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_31_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_30_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_102_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_29_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_101_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_28_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_100_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_99_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_27_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_98_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_26_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_97_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_96_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_25_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_158_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_24_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_94_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_93_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_23_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_92_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_22_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_91_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_90_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_21_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_89_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_20_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_88_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_19_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_87_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_86_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_18_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_85_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_84_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_17_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_83_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_169_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_82_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_16_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_81_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_80_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_15_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_79_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_14_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_78_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_13_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_77_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_76_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_12_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_75_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_11_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_10_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_74_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_9_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_73_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_8_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_72_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_7_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_71_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_70_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_6_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_69_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_5_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_68_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_4_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_67_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_3_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_66_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_65_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_2_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_64_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_1_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_170_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_63_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_192_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_62_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_208_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_61_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_191_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_207_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_60_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_206_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_190_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_59_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_205_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_58_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_204_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_57_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_189_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_56_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_203_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_188_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_202_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_175_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_55_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_186_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_187_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_185_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_174_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_201_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_184_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_186_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_173_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_183_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_200_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_172_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_182_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_199_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_171_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_185_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_198_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_197_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_184_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_196_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_183_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_195_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_194_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_182_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_193_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_181_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_192_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_191_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_180_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_179_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_190_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_178_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_177_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_188_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_168_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_176_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_187_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_167_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_189_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_166_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_54_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_165_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_53_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_164_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_52_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_163_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_51_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_162_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_50_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_161_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_49_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_48_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_160_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_47_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_46_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_159_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_45_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_44_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_29_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_43_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_157_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_74_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_156_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_42_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_155_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_41_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_40_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_154_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_39_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_153_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_38_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_152_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_37_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_151_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_150_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_149_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_148_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_147_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_73_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_29_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_72_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_35_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_71_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_28_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_146_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_70_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_27_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_145_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_69_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_26_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_222_bytes (0.01s)
    --- PASS: TestRefHasher/8_segments_150_bytes (0.01s)
    --- PASS: TestRefHasher/6_segments_144_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_88_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_179_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_68_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_25_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_80_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_65_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_24_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_216_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_23_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_64_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_141_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_67_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_63_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_22_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_21_bytes (0.00s)
    --- PASS: TestRefHasher/8_segments_86_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_61_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_20_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_66_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_117_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_60_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_138_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_143_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_59_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_142_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_58_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_18_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_137_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_17_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_19_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_114_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_139_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_57_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_15_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_118_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_16_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_56_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_55_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_54_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_14_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_136_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_115_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_113_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_140_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_62_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_111_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_135_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_51_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_116_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_11_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_53_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_13_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_110_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_12_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_9_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_10_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_112_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_109_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_134_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_49_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_52_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_8_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_47_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_50_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_133_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_7_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_108_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_107_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_48_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_131_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_46_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_45_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_6_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_5_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_106_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_128_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_104_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_44_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_103_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_130_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_132_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_102_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_3_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_42_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_4_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_43_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_33_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_101_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_105_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_39_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_125_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_40_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_127_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_124_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_129_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_122_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_126_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_123_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_38_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_32_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_2_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_119_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_41_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_37_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_169_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_36_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_168_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_121_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_167_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_170_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_165_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_171_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_97_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_98_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_164_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_120_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_99_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_95_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_94_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_96_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_172_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_92_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_100_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_91_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_163_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_89_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_93_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_159_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_90_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_161_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_88_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_160_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_30_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_87_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_86_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_162_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_28_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_31_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_27_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_26_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_60_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_158_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_24_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_157_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_25_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_152_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_154_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_151_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_153_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_155_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_23_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_158_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_149_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_157_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_148_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_21_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_150_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_19_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_20_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_18_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_22_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_156_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_156_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_17_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_155_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_16_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_147_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_146_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_129_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_128_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_144_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_153_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_145_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_151_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_14_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_143_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_152_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_85_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_150_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_166_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_140_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_149_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_139_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_12_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_82_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_138_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_148_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_81_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_137_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_147_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_80_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_136_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_146_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_79_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_135_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_145_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_134_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_144_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_78_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_133_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_143_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_77_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_132_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_142_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_131_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_76_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_130_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_141_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_75_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_129_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_140_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_13_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_128_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_84_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_139_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_138_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_126_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_125_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_136_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_124_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_15_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_135_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_142_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_122_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_127_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_10_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_121_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_127_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_9_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_133_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_137_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_154_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_120_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_141_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_119_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_132_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_83_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_11_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_118_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_125_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_117_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_8_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_5_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_124_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_116_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_126_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_123_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_4_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_130_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_115_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_122_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_2_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_131_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_134_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_7_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_120_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_113_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_114_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_119_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_102_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_1_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_118_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_101_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_117_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_96_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_100_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_116_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_95_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_160_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_80_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_159_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_123_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_79_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_114_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_94_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_6_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_99_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_93_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_77_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_113_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_92_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_98_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_112_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_78_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_3_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_111_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_90_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_97_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_74_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_96_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_121_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_89_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_1_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_109_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_115_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_91_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_94_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_92_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_110_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_90_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_76_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_72_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_61_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_70_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_89_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_73_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_93_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_88_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_67_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_64_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_71_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_91_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_75_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_66_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_58_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_69_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_65_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_56_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_59_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_68_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_62_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_51_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_50_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_55_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_48_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_46_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_52_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_45_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_44_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_53_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_43_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_63_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_41_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_54_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_40_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_57_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_87_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_47_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_85_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_36_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_83_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_27_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_82_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_35_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_34_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_86_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_108_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_33_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_42_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_38_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_106_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_105_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_39_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_49_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_104_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_95_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_84_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_32_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_24_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_103_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_107_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_81_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_30_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_23_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_29_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_25_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_13_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_20_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_31_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_2_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_12_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_26_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_10_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_1_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_18_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_9_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_8_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_19_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_128_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_16_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_17_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_22_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_11_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_6_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_5_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_123_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_14_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_122_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_4_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_3_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_121_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_120_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_119_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_88_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_112_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_124_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_118_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_117_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_28_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_87_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_127_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_37_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_109_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_108_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_21_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_107_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_106_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_84_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_125_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_126_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_7_bytes (0.00s)
    --- PASS: TestRefHasher/5_segments_15_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_110_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_85_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_113_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_82_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_104_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_81_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_103_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_116_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_80_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_79_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_102_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_98_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_78_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_101_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_77_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_100_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_76_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_97_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_75_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_96_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_74_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_93_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_73_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_92_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_95_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_72_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_91_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_94_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_71_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_90_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_70_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_69_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_89_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_88_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_68_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_84_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_87_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_67_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_83_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_86_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_66_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_82_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_65_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_85_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_64_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_81_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_63_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_80_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_62_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_79_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_77_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_61_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_114_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_99_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_78_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_59_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_75_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_74_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_58_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_71_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_73_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_60_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_70_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_72_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_57_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_69_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_67_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_55_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_68_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_54_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_56_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_83_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_66_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_111_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_86_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_105_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_76_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_63_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_115_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_1_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_31_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_50_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_62_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_65_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_48_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_62_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_61_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_61_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_45_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_60_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_47_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_44_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_60_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_59_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_29_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_43_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_42_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_58_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_28_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_41_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_57_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_26_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_27_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_52_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_56_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_40_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_26_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_55_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_25_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_39_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_54_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_24_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_35_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_34_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_53_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_23_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_37_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_33_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_52_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_22_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_36_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_32_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_51_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_35_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_50_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_34_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_30_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_21_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_49_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_29_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_33_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_48_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_20_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_28_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_47_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_19_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_32_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_46_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_27_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_31_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_18_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_45_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_26_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_17_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_30_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_16_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_44_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_25_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_15_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_29_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_28_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_14_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_24_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_23_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_25_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_40_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_22_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_24_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_43_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_21_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_23_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_13_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_22_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_20_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_12_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_21_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_19_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_19_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_11_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_20_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_18_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_10_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_18_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_42_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_9_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_17_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_41_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_8_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_16_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_38_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_16_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_7_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_39_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_15_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_6_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_4_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_15_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_5_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_32_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_51_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_3_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_14_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_30_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_13_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_108_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_64_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_37_bytes (0.00s)
    --- PASS: TestRefHasher/1_segments_2_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_36_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_11_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_14_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_53_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_11_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_10_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_64_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_9_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_10_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_106_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_8_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_105_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_9_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_111_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_8_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_7_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_6_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_6_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_103_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_5_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_4_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_110_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_5_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_38_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_2_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_2_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_107_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_1_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_3_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_3_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_31_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_49_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_27_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_63_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_12_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_7_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_4_bytes (0.00s)
    --- PASS: TestRefHasher/2_segments_12_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_46_bytes (0.00s)
    --- PASS: TestRefHasher/4_segments_59_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_112_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_17_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_104_bytes (0.00s)
    --- PASS: TestRefHasher/7_segments_109_bytes (0.00s)
    --- PASS: TestRefHasher/6_segments_34_bytes (0.00s)
    --- PASS: TestRefHasher/3_segments_13_bytes (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/bmt/reference	(cached)
?   	github.com/ethersphere/bee/pkg/bmtpool	[no test files]
=== RUN   TestBzzAddress
=== PAUSE TestBzzAddress
=== RUN   Test_ContainsAddress
=== PAUSE Test_ContainsAddress
=== CONT  TestBzzAddress
=== CONT  Test_ContainsAddress
--- PASS: Test_ContainsAddress (0.00s)
--- PASS: TestBzzAddress (0.03s)
PASS
ok  	github.com/ethersphere/bee/pkg/bzz	(cached)
=== RUN   TestNewCAC
=== PAUSE TestNewCAC
=== RUN   TestNewWithDataSpan
=== PAUSE TestNewWithDataSpan
=== RUN   TestChunkInvariants
=== PAUSE TestChunkInvariants
=== RUN   TestValid
=== PAUSE TestValid
=== RUN   TestInvalid
=== PAUSE TestInvalid
=== CONT  TestNewCAC
--- PASS: TestNewCAC (0.00s)
=== CONT  TestInvalid
=== RUN   TestInvalid/wrong_address
=== PAUSE TestInvalid/wrong_address
=== RUN   TestInvalid/empty_address
=== PAUSE TestInvalid/empty_address
=== RUN   TestInvalid/zero_data
=== PAUSE TestInvalid/zero_data
=== RUN   TestInvalid/nil_data
=== PAUSE TestInvalid/nil_data
=== RUN   TestInvalid/small_data
=== PAUSE TestInvalid/small_data
=== RUN   TestInvalid/large_data
=== PAUSE TestInvalid/large_data
=== CONT  TestInvalid/wrong_address
=== CONT  TestInvalid/empty_address
=== CONT  TestInvalid/zero_data
=== CONT  TestChunkInvariants
=== RUN   TestChunkInvariants/new_cac-zero_data
=== PAUSE TestChunkInvariants/new_cac-zero_data
=== RUN   TestChunkInvariants/new_cac-nil
=== PAUSE TestChunkInvariants/new_cac-nil
=== RUN   TestChunkInvariants/new_cac-too_large_data_chunk
=== PAUSE TestChunkInvariants/new_cac-too_large_data_chunk
=== RUN   TestChunkInvariants/new_chunk_with_data_span-zero_data
=== PAUSE TestChunkInvariants/new_chunk_with_data_span-zero_data
=== RUN   TestChunkInvariants/new_chunk_with_data_span-nil
=== PAUSE TestChunkInvariants/new_chunk_with_data_span-nil
=== RUN   TestChunkInvariants/new_chunk_with_data_span-too_large_data_chunk
=== PAUSE TestChunkInvariants/new_chunk_with_data_span-too_large_data_chunk
=== CONT  TestChunkInvariants/new_cac-zero_data
=== CONT  TestNewWithDataSpan
--- PASS: TestNewWithDataSpan (0.00s)
=== CONT  TestInvalid/small_data
=== CONT  TestChunkInvariants/new_cac-too_large_data_chunk
=== CONT  TestValid
--- PASS: TestValid (0.00s)
=== CONT  TestInvalid/large_data
=== CONT  TestChunkInvariants/new_chunk_with_data_span-zero_data
=== CONT  TestChunkInvariants/new_chunk_with_data_span-too_large_data_chunk
=== CONT  TestChunkInvariants/new_chunk_with_data_span-nil
=== CONT  TestChunkInvariants/new_cac-nil
--- PASS: TestChunkInvariants (0.00s)
    --- PASS: TestChunkInvariants/new_cac-zero_data (0.00s)
    --- PASS: TestChunkInvariants/new_cac-too_large_data_chunk (0.00s)
    --- PASS: TestChunkInvariants/new_chunk_with_data_span-zero_data (0.00s)
    --- PASS: TestChunkInvariants/new_chunk_with_data_span-too_large_data_chunk (0.00s)
    --- PASS: TestChunkInvariants/new_chunk_with_data_span-nil (0.00s)
    --- PASS: TestChunkInvariants/new_cac-nil (0.00s)
=== CONT  TestInvalid/nil_data
--- PASS: TestInvalid (0.00s)
    --- PASS: TestInvalid/wrong_address (0.00s)
    --- PASS: TestInvalid/empty_address (0.00s)
    --- PASS: TestInvalid/zero_data (0.00s)
    --- PASS: TestInvalid/small_data (0.00s)
    --- PASS: TestInvalid/large_data (0.00s)
    --- PASS: TestInvalid/nil_data (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/cac	(cached)
=== RUN   TestProve
=== PAUSE TestProve
=== RUN   TestProveErr
=== PAUSE TestProveErr
=== CONT  TestProve
=== CONT  TestProveErr
--- PASS: TestProve (0.00s)
--- PASS: TestProveErr (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/chainsync	(cached)
?   	github.com/ethersphere/bee/pkg/chainsync/pb	[no test files]
?   	github.com/ethersphere/bee/pkg/config	[no test files]
=== RUN   TestChainsyncer
=== RUN   TestChainsyncer/prover_error
=== RUN   TestChainsyncer/blockhash_mismatch
=== RUN   TestChainsyncer/all_good
--- PASS: TestChainsyncer (4.90s)
    --- PASS: TestChainsyncer/prover_error (2.20s)
    --- PASS: TestChainsyncer/blockhash_mismatch (2.20s)
    --- PASS: TestChainsyncer/all_good (0.50s)
PASS
ok  	github.com/ethersphere/bee/pkg/chainsyncer	(cached)
=== RUN   TestGenerateSecp256k1Key
=== PAUSE TestGenerateSecp256k1Key
=== RUN   TestGenerateSecp256k1EDG
=== PAUSE TestGenerateSecp256k1EDG
=== RUN   TestNewAddress
=== PAUSE TestNewAddress
=== RUN   TestEncodeSecp256k1PrivateKey
=== PAUSE TestEncodeSecp256k1PrivateKey
=== RUN   TestEncodeSecp256k1EDG
=== PAUSE TestEncodeSecp256k1EDG
=== RUN   TestSecp256k1PrivateKeyFromBytes
=== PAUSE TestSecp256k1PrivateKeyFromBytes
=== RUN   TestGenerateSecp256r1Key
=== PAUSE TestGenerateSecp256r1Key
=== RUN   TestGenerateSecp256r1EDG
=== PAUSE TestGenerateSecp256r1EDG
=== RUN   TestEncodeSecp256r1PrivateKey
=== PAUSE TestEncodeSecp256r1PrivateKey
=== RUN   TestEncodeSecp256r1EDG
=== PAUSE TestEncodeSecp256r1EDG
=== RUN   TestNewEthereumAddress
=== PAUSE TestNewEthereumAddress
=== RUN   TestECDHCorrect
=== PAUSE TestECDHCorrect
=== RUN   TestSharedKey
=== PAUSE TestSharedKey
=== RUN   TestDefaultSigner
=== PAUSE TestDefaultSigner
=== RUN   TestDefaultSignerEthereumAddress
=== PAUSE TestDefaultSignerEthereumAddress
=== RUN   TestDefaultSignerSignTx
=== PAUSE TestDefaultSignerSignTx
=== RUN   TestDefaultSignerTypedData
=== PAUSE TestDefaultSignerTypedData
=== RUN   TestRecoverEIP712
=== PAUSE TestRecoverEIP712
=== RUN   TestDefaultSignerDeterministic
=== PAUSE TestDefaultSignerDeterministic
=== CONT  TestGenerateSecp256k1Key
=== CONT  TestNewEthereumAddress
=== CONT  TestDefaultSignerEthereumAddress
=== CONT  TestDefaultSigner
=== CONT  TestSharedKey
=== CONT  TestECDHCorrect
=== CONT  TestSecp256k1PrivateKeyFromBytes
=== CONT  TestEncodeSecp256r1EDG
--- PASS: TestEncodeSecp256r1EDG (0.00s)
=== CONT  TestGenerateSecp256r1EDG
--- PASS: TestGenerateSecp256r1EDG (0.00s)
=== CONT  TestEncodeSecp256r1PrivateKey
--- PASS: TestEncodeSecp256r1PrivateKey (0.00s)
=== CONT  TestRecoverEIP712
--- PASS: TestGenerateSecp256k1Key (0.03s)
--- PASS: TestNewEthereumAddress (0.03s)
=== CONT  TestDefaultSignerDeterministic
--- PASS: TestDefaultSignerEthereumAddress (0.03s)
=== CONT  TestDefaultSignerTypedData
--- PASS: TestSharedKey (0.03s)
=== CONT  TestEncodeSecp256k1PrivateKey
--- PASS: TestEncodeSecp256k1PrivateKey (0.00s)
=== CONT  TestEncodeSecp256k1EDG
=== CONT  TestGenerateSecp256r1Key
=== RUN   TestDefaultSigner/OK_-_sign_&_recover
--- PASS: TestGenerateSecp256r1Key (0.00s)
=== PAUSE TestDefaultSigner/OK_-_sign_&_recover
--- PASS: TestEncodeSecp256k1EDG (0.00s)
=== RUN   TestDefaultSigner/OK_-_recover_with_invalid_data
=== CONT  TestGenerateSecp256k1EDG
=== PAUSE TestDefaultSigner/OK_-_recover_with_invalid_data
=== RUN   TestDefaultSigner/OK_-_recover_with_short_signature
=== PAUSE TestDefaultSigner/OK_-_recover_with_short_signature
=== CONT  TestNewAddress
--- PASS: TestSecp256k1PrivateKeyFromBytes (0.03s)
=== CONT  TestDefaultSigner/OK_-_sign_&_recover
--- PASS: TestNewAddress (0.00s)
=== CONT  TestDefaultSignerSignTx
--- PASS: TestGenerateSecp256k1EDG (0.00s)
=== CONT  TestDefaultSigner/OK_-_recover_with_short_signature
=== CONT  TestDefaultSigner/OK_-_recover_with_invalid_data
--- PASS: TestECDHCorrect (0.03s)
--- PASS: TestRecoverEIP712 (0.03s)
--- PASS: TestDefaultSigner (0.03s)
    --- PASS: TestDefaultSigner/OK_-_sign_&_recover (0.00s)
    --- PASS: TestDefaultSigner/OK_-_recover_with_short_signature (0.00s)
    --- PASS: TestDefaultSigner/OK_-_recover_with_invalid_data (0.00s)
--- PASS: TestDefaultSignerSignTx (0.00s)
--- PASS: TestDefaultSignerDeterministic (0.00s)
--- PASS: TestDefaultSignerTypedData (0.02s)
PASS
ok  	github.com/ethersphere/bee/pkg/crypto	(cached)
?   	github.com/ethersphere/bee/pkg/crypto/eip712	[no test files]
?   	github.com/ethersphere/bee/pkg/crypto/mock	[no test files]
=== RUN   TestNewClefSigner
=== PAUSE TestNewClefSigner
=== RUN   TestNewClefSignerSpecificAccount
=== PAUSE TestNewClefSignerSpecificAccount
=== RUN   TestNewClefSignerAccountUnavailable
=== PAUSE TestNewClefSignerAccountUnavailable
=== RUN   TestClefNoAccounts
=== PAUSE TestClefNoAccounts
=== RUN   TestClefTypedData
=== PAUSE TestClefTypedData
=== CONT  TestNewClefSigner
=== CONT  TestClefNoAccounts
--- PASS: TestClefNoAccounts (0.00s)
=== CONT  TestNewClefSignerAccountUnavailable
--- PASS: TestNewClefSignerAccountUnavailable (0.00s)
=== CONT  TestNewClefSignerSpecificAccount
=== CONT  TestClefTypedData
--- PASS: TestNewClefSigner (0.02s)
--- PASS: TestNewClefSignerSpecificAccount (0.02s)
--- PASS: TestClefTypedData (0.02s)
PASS
ok  	github.com/ethersphere/bee/pkg/crypto/clef	(cached)
?   	github.com/ethersphere/bee/pkg/encryption/store	[no test files]
?   	github.com/ethersphere/bee/pkg/discovery/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/discovery	[no test files]
?   	github.com/ethersphere/bee/pkg/feeds	[no test files]
=== RUN   TestEncryptDataLongerThanPadding
=== PAUSE TestEncryptDataLongerThanPadding
=== RUN   TestEncryptDataZeroPadding
=== PAUSE TestEncryptDataZeroPadding
=== RUN   TestEncryptDataLengthEqualsPadding
=== PAUSE TestEncryptDataLengthEqualsPadding
=== RUN   TestEncryptDataLengthSmallerThanPadding
=== PAUSE TestEncryptDataLengthSmallerThanPadding
=== RUN   TestEncryptDataCounterNonZero
=== PAUSE TestEncryptDataCounterNonZero
=== RUN   TestDecryptDataLengthNotEqualsPadding
=== PAUSE TestDecryptDataLengthNotEqualsPadding
=== RUN   TestEncryptDecryptIsIdentity
=== PAUSE TestEncryptDecryptIsIdentity
=== RUN   TestEncryptSectioned
=== PAUSE TestEncryptSectioned
=== CONT  TestEncryptDataLongerThanPadding
=== CONT  TestEncryptDataCounterNonZero
--- PASS: TestEncryptDataLongerThanPadding (0.00s)
--- PASS: TestEncryptDataCounterNonZero (0.00s)
=== CONT  TestEncryptDataLengthSmallerThanPadding
=== CONT  TestEncryptSectioned
=== CONT  TestEncryptDataLengthEqualsPadding
=== CONT  TestEncryptDataZeroPadding
--- PASS: TestEncryptDataZeroPadding (0.00s)
=== CONT  TestEncryptDecryptIsIdentity
--- PASS: TestEncryptDataLengthEqualsPadding (0.00s)
=== CONT  TestDecryptDataLengthNotEqualsPadding
--- PASS: TestEncryptDataLengthSmallerThanPadding (0.00s)
--- PASS: TestDecryptDataLengthNotEqualsPadding (0.00s)
--- PASS: TestEncryptDecryptIsIdentity (0.00s)
--- PASS: TestEncryptSectioned (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/encryption	(cached)
=== RUN   TestElgamalCorrect
=== PAUSE TestElgamalCorrect
=== CONT  TestElgamalCorrect
--- PASS: TestElgamalCorrect (0.01s)
PASS
ok  	github.com/ethersphere/bee/pkg/encryption/elgamal	(cached)
=== RUN   TestEncryptor_Encrypt
=== PAUSE TestEncryptor_Encrypt
=== RUN   TestEncryptor_Decrypt
=== PAUSE TestEncryptor_Decrypt
=== RUN   TestEncryptor_Reset
=== PAUSE TestEncryptor_Reset
=== RUN   TestEncryptor_XOREncryption
=== PAUSE TestEncryptor_XOREncryption
=== CONT  TestEncryptor_Encrypt
=== CONT  TestEncryptor_Reset
=== RUN   TestEncryptor_Reset/empty
=== CONT  TestEncryptor_XOREncryption
=== RUN   TestEncryptor_Encrypt/empty
=== PAUSE TestEncryptor_Encrypt/empty
=== RUN   TestEncryptor_Encrypt/func_constant
=== PAUSE TestEncryptor_Encrypt/func_constant
=== RUN   TestEncryptor_Encrypt/func_identity
=== PAUSE TestEncryptor_Encrypt/func_identity
=== RUN   TestEncryptor_Encrypt/func_err
=== PAUSE TestEncryptor_Encrypt/func_err
=== RUN   TestEncryptor_Encrypt/xor
=== PAUSE TestEncryptor_Encrypt/xor
=== RUN   TestEncryptor_Encrypt/xor_error
=== PAUSE TestEncryptor_Encrypt/xor_error
=== CONT  TestEncryptor_Encrypt/empty
=== CONT  TestEncryptor_Encrypt/xor_error
=== PAUSE TestEncryptor_Reset/empty
=== RUN   TestEncryptor_Reset/func
=== PAUSE TestEncryptor_Reset/func
=== CONT  TestEncryptor_Reset/empty
=== CONT  TestEncryptor_Reset/func
--- PASS: TestEncryptor_XOREncryption (0.00s)
--- PASS: TestEncryptor_Reset (0.00s)
    --- PASS: TestEncryptor_Reset/empty (0.00s)
    --- PASS: TestEncryptor_Reset/func (0.00s)
=== CONT  TestEncryptor_Decrypt
=== RUN   TestEncryptor_Decrypt/empty
=== PAUSE TestEncryptor_Decrypt/empty
=== RUN   TestEncryptor_Decrypt/func_constant
=== PAUSE TestEncryptor_Decrypt/func_constant
=== RUN   TestEncryptor_Decrypt/func_identity
=== PAUSE TestEncryptor_Decrypt/func_identity
=== RUN   TestEncryptor_Decrypt/func_err
=== PAUSE TestEncryptor_Decrypt/func_err
=== RUN   TestEncryptor_Decrypt/xor
=== PAUSE TestEncryptor_Decrypt/xor
=== RUN   TestEncryptor_Decrypt/xor_error
=== PAUSE TestEncryptor_Decrypt/xor_error
=== CONT  TestEncryptor_Decrypt/empty
=== CONT  TestEncryptor_Encrypt/func_err
=== CONT  TestEncryptor_Decrypt/func_identity
=== CONT  TestEncryptor_Decrypt/func_constant
=== CONT  TestEncryptor_Decrypt/xor_error
=== CONT  TestEncryptor_Encrypt/xor
=== CONT  TestEncryptor_Decrypt/xor
=== CONT  TestEncryptor_Decrypt/func_err
--- PASS: TestEncryptor_Decrypt (0.00s)
    --- PASS: TestEncryptor_Decrypt/empty (0.00s)
    --- PASS: TestEncryptor_Decrypt/func_identity (0.00s)
    --- PASS: TestEncryptor_Decrypt/func_constant (0.00s)
    --- PASS: TestEncryptor_Decrypt/xor_error (0.00s)
    --- PASS: TestEncryptor_Decrypt/xor (0.00s)
    --- PASS: TestEncryptor_Decrypt/func_err (0.00s)
=== CONT  TestEncryptor_Encrypt/func_identity
=== CONT  TestEncryptor_Encrypt/func_constant
--- PASS: TestEncryptor_Encrypt (0.00s)
    --- PASS: TestEncryptor_Encrypt/empty (0.00s)
    --- PASS: TestEncryptor_Encrypt/xor_error (0.00s)
    --- PASS: TestEncryptor_Encrypt/func_err (0.00s)
    --- PASS: TestEncryptor_Encrypt/xor (0.00s)
    --- PASS: TestEncryptor_Encrypt/func_identity (0.00s)
    --- PASS: TestEncryptor_Encrypt/func_constant (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/encryption/mock	(cached)
=== RUN   TestFinder_FLAKY
=== PAUSE TestFinder_FLAKY
=== CONT  TestFinder_FLAKY
=== RUN   TestFinder_FLAKY/sync
=== PAUSE TestFinder_FLAKY/sync
=== RUN   TestFinder_FLAKY/async
=== PAUSE TestFinder_FLAKY/async
=== CONT  TestFinder_FLAKY/sync
=== RUN   TestFinder_FLAKY/sync/basic
=== PAUSE TestFinder_FLAKY/sync/basic
=== RUN   TestFinder_FLAKY/sync/fixed
=== PAUSE TestFinder_FLAKY/sync/fixed
=== RUN   TestFinder_FLAKY/sync/random
=== PAUSE TestFinder_FLAKY/sync/random
=== CONT  TestFinder_FLAKY/sync/basic
=== CONT  TestFinder_FLAKY/sync/random
=== RUN   TestFinder_FLAKY/sync/random/random_intervals_0
=== PAUSE TestFinder_FLAKY/sync/random/random_intervals_0
=== RUN   TestFinder_FLAKY/sync/random/random_intervals_1
=== PAUSE TestFinder_FLAKY/sync/random/random_intervals_1
=== RUN   TestFinder_FLAKY/sync/random/random_intervals_2
=== PAUSE TestFinder_FLAKY/sync/random/random_intervals_2
=== CONT  TestFinder_FLAKY/sync/random/random_intervals_0
=== CONT  TestFinder_FLAKY/sync/random/random_intervals_2
=== CONT  TestFinder_FLAKY/sync/random/random_intervals_1
=== CONT  TestFinder_FLAKY/sync/fixed
=== RUN   TestFinder_FLAKY/sync/fixed/custom_intervals_up_to_10
=== CONT  TestFinder_FLAKY/async
=== RUN   TestFinder_FLAKY/async/basic
=== PAUSE TestFinder_FLAKY/async/basic
=== RUN   TestFinder_FLAKY/async/fixed
=== PAUSE TestFinder_FLAKY/async/fixed
=== RUN   TestFinder_FLAKY/async/random
=== PAUSE TestFinder_FLAKY/async/random
=== CONT  TestFinder_FLAKY/async/basic
=== CONT  TestFinder_FLAKY/async/random
=== RUN   TestFinder_FLAKY/async/random/random_intervals_0
=== PAUSE TestFinder_FLAKY/async/random/random_intervals_0
=== RUN   TestFinder_FLAKY/async/random/random_intervals_1
=== PAUSE TestFinder_FLAKY/async/random/random_intervals_1
=== RUN   TestFinder_FLAKY/async/random/random_intervals_2
=== PAUSE TestFinder_FLAKY/async/random/random_intervals_2
=== CONT  TestFinder_FLAKY/async/random/random_intervals_0
=== CONT  TestFinder_FLAKY/async/fixed
=== RUN   TestFinder_FLAKY/async/fixed/custom_intervals_up_to_10
=== RUN   TestFinder_FLAKY/sync/basic/no_update
=== CONT  TestFinder_FLAKY/async/random/random_intervals_2
=== RUN   TestFinder_FLAKY/async/basic/no_update
=== CONT  TestFinder_FLAKY/async/random/random_intervals_1
=== RUN   TestFinder_FLAKY/async/basic/first_update
=== RUN   TestFinder_FLAKY/sync/basic/first_update
--- PASS: TestFinder_FLAKY (0.00s)
    --- PASS: TestFinder_FLAKY/async (0.00s)
        --- PASS: TestFinder_FLAKY/async/fixed (0.02s)
            --- PASS: TestFinder_FLAKY/async/fixed/custom_intervals_up_to_10 (0.02s)
        --- PASS: TestFinder_FLAKY/async/basic (0.08s)
            --- PASS: TestFinder_FLAKY/async/basic/no_update (0.03s)
            --- PASS: TestFinder_FLAKY/async/basic/first_update (0.03s)
        --- PASS: TestFinder_FLAKY/async/random (0.00s)
            --- PASS: TestFinder_FLAKY/async/random/random_intervals_2 (1.80s)
            --- PASS: TestFinder_FLAKY/async/random/random_intervals_0 (2.00s)
            --- PASS: TestFinder_FLAKY/async/random/random_intervals_1 (2.48s)
    --- PASS: TestFinder_FLAKY/sync (0.00s)
        --- PASS: TestFinder_FLAKY/sync/fixed (0.02s)
            --- PASS: TestFinder_FLAKY/sync/fixed/custom_intervals_up_to_10 (0.02s)
        --- PASS: TestFinder_FLAKY/sync/basic (0.12s)
            --- PASS: TestFinder_FLAKY/sync/basic/no_update (0.07s)
            --- PASS: TestFinder_FLAKY/sync/basic/first_update (0.04s)
        --- PASS: TestFinder_FLAKY/sync/random (0.00s)
            --- PASS: TestFinder_FLAKY/sync/random/random_intervals_2 (9.96s)
            --- PASS: TestFinder_FLAKY/sync/random/random_intervals_1 (10.19s)
            --- PASS: TestFinder_FLAKY/sync/random/random_intervals_0 (10.35s)
PASS
ok  	github.com/ethersphere/bee/pkg/feeds/epochs	(cached)
?   	github.com/ethersphere/bee/pkg/feeds/factory	[no test files]
?   	github.com/ethersphere/bee/pkg/feeds/testing	[no test files]
=== RUN   TestFinder
=== PAUSE TestFinder
=== CONT  TestFinder
=== RUN   TestFinder/sync
=== PAUSE TestFinder/sync
=== RUN   TestFinder/async
=== PAUSE TestFinder/async
=== CONT  TestFinder/sync
=== RUN   TestFinder/sync/basic
=== PAUSE TestFinder/sync/basic
=== RUN   TestFinder/sync/fixed
=== CONT  TestFinder/async
=== RUN   TestFinder/async/basic
=== PAUSE TestFinder/async/basic
=== RUN   TestFinder/async/fixed
=== PAUSE TestFinder/async/fixed
=== RUN   TestFinder/async/random
=== PAUSE TestFinder/async/random
=== CONT  TestFinder/async/basic
=== PAUSE TestFinder/sync/fixed
=== RUN   TestFinder/sync/random
=== PAUSE TestFinder/sync/random
=== CONT  TestFinder/sync/basic
=== CONT  TestFinder/sync/random
=== RUN   TestFinder/sync/random/random_intervals_0
=== PAUSE TestFinder/sync/random/random_intervals_0
=== CONT  TestFinder/async/random
=== CONT  TestFinder/sync/fixed
=== CONT  TestFinder/async/fixed
=== RUN   TestFinder/async/random/random_intervals_0
=== RUN   TestFinder/async/fixed/custom_intervals_up_to_10
=== PAUSE TestFinder/async/random/random_intervals_0
=== RUN   TestFinder/async/random/random_intervals_1
=== RUN   TestFinder/sync/random/random_intervals_1
=== PAUSE TestFinder/async/random/random_intervals_1
=== RUN   TestFinder/async/random/random_intervals_2
=== PAUSE TestFinder/sync/random/random_intervals_1
=== RUN   TestFinder/sync/random/random_intervals_2
=== RUN   TestFinder/sync/fixed/custom_intervals_up_to_10
=== PAUSE TestFinder/sync/random/random_intervals_2
=== CONT  TestFinder/sync/random/random_intervals_0
=== CONT  TestFinder/sync/random/random_intervals_2
=== CONT  TestFinder/sync/random/random_intervals_1
=== PAUSE TestFinder/async/random/random_intervals_2
=== CONT  TestFinder/async/random/random_intervals_0
=== RUN   TestFinder/async/basic/no_update
=== RUN   TestFinder/sync/basic/no_update
=== RUN   TestFinder/sync/basic/first_update
=== RUN   TestFinder/async/basic/first_update
=== CONT  TestFinder/async/random/random_intervals_2
=== CONT  TestFinder/async/random/random_intervals_1
=== RUN   TestFinder/async/fixed/custom_intervals_up_to_20
=== RUN   TestFinder/sync/fixed/custom_intervals_up_to_20
=== RUN   TestFinder/async/fixed/custom_intervals_up_to_30
=== RUN   TestFinder/sync/fixed/custom_intervals_up_to_30
--- PASS: TestFinder (0.00s)
    --- PASS: TestFinder/async (0.00s)
        --- PASS: TestFinder/async/basic (0.08s)
            --- PASS: TestFinder/async/basic/no_update (0.03s)
            --- PASS: TestFinder/async/basic/first_update (0.04s)
        --- PASS: TestFinder/async/fixed (0.86s)
            --- PASS: TestFinder/async/fixed/custom_intervals_up_to_10 (0.20s)
            --- PASS: TestFinder/async/fixed/custom_intervals_up_to_20 (0.51s)
            --- PASS: TestFinder/async/fixed/custom_intervals_up_to_30 (0.16s)
        --- PASS: TestFinder/async/random (0.00s)
            --- PASS: TestFinder/async/random/random_intervals_1 (2.80s)
            --- PASS: TestFinder/async/random/random_intervals_2 (2.86s)
            --- PASS: TestFinder/async/random/random_intervals_0 (3.14s)
    --- PASS: TestFinder/sync (0.00s)
        --- PASS: TestFinder/sync/basic (0.08s)
            --- PASS: TestFinder/sync/basic/no_update (0.03s)
            --- PASS: TestFinder/sync/basic/first_update (0.04s)
        --- PASS: TestFinder/sync/fixed (1.46s)
            --- PASS: TestFinder/sync/fixed/custom_intervals_up_to_10 (0.20s)
            --- PASS: TestFinder/sync/fixed/custom_intervals_up_to_20 (1.01s)
            --- PASS: TestFinder/sync/fixed/custom_intervals_up_to_30 (0.24s)
        --- PASS: TestFinder/sync/random (0.00s)
            --- PASS: TestFinder/sync/random/random_intervals_2 (10.43s)
            --- PASS: TestFinder/sync/random/random_intervals_0 (11.28s)
            --- PASS: TestFinder/sync/random/random_intervals_1 (11.82s)
PASS
ok  	github.com/ethersphere/bee/pkg/feeds/sequence	(cached)
=== RUN   TestChunkPipe
=== PAUSE TestChunkPipe
=== RUN   TestCopyBuffer
=== PAUSE TestCopyBuffer
=== RUN   TestSplitThenJoin
=== PAUSE TestSplitThenJoin
=== CONT  TestChunkPipe
=== CONT  TestSplitThenJoin
=== RUN   TestSplitThenJoin/0
=== PAUSE TestSplitThenJoin/0
=== RUN   TestSplitThenJoin/1
=== PAUSE TestSplitThenJoin/1
=== RUN   TestSplitThenJoin/2
=== PAUSE TestSplitThenJoin/2
=== RUN   TestSplitThenJoin/3
=== PAUSE TestSplitThenJoin/3
=== RUN   TestChunkPipe/0
=== PAUSE TestChunkPipe/0
=== RUN   TestChunkPipe/1
=== PAUSE TestChunkPipe/1
=== RUN   TestChunkPipe/2
=== CONT  TestCopyBuffer
=== PAUSE TestChunkPipe/2
=== RUN   TestChunkPipe/3
=== PAUSE TestChunkPipe/3
=== RUN   TestChunkPipe/4
=== PAUSE TestChunkPipe/4
=== RUN   TestChunkPipe/5
=== PAUSE TestChunkPipe/5
=== RUN   TestChunkPipe/6
=== PAUSE TestChunkPipe/6
=== RUN   TestChunkPipe/7
=== PAUSE TestChunkPipe/7
=== RUN   TestChunkPipe/8
=== PAUSE TestChunkPipe/8
=== CONT  TestChunkPipe/0
=== RUN   TestCopyBuffer/buf_64__/data_size_1
=== CONT  TestChunkPipe/5
=== PAUSE TestCopyBuffer/buf_64__/data_size_1
=== RUN   TestSplitThenJoin/4
=== PAUSE TestSplitThenJoin/4
=== RUN   TestCopyBuffer/buf_64__/data_size_64
=== RUN   TestSplitThenJoin/5
=== PAUSE TestCopyBuffer/buf_64__/data_size_64
=== RUN   TestCopyBuffer/buf_64__/data_size_1024
=== PAUSE TestCopyBuffer/buf_64__/data_size_1024
=== RUN   TestCopyBuffer/buf_64__/data_size_4095
=== PAUSE TestCopyBuffer/buf_64__/data_size_4095
=== RUN   TestCopyBuffer/buf_64__/data_size_4096
=== CONT  TestChunkPipe/8
=== PAUSE TestCopyBuffer/buf_64__/data_size_4096
=== RUN   TestCopyBuffer/buf_64__/data_size_4097
=== PAUSE TestCopyBuffer/buf_64__/data_size_4097
=== RUN   TestCopyBuffer/buf_64__/data_size_8192
=== PAUSE TestCopyBuffer/buf_64__/data_size_8192
=== RUN   TestCopyBuffer/buf_64__/data_size_8195
=== PAUSE TestCopyBuffer/buf_64__/data_size_8195
=== RUN   TestCopyBuffer/buf_64__/data_size_20480
=== PAUSE TestCopyBuffer/buf_64__/data_size_20480
=== RUN   TestCopyBuffer/buf_64__/data_size_20483
=== PAUSE TestCopyBuffer/buf_64__/data_size_20483
=== CONT  TestChunkPipe/1
=== CONT  TestChunkPipe/4
=== CONT  TestChunkPipe/3
=== CONT  TestChunkPipe/7
=== PAUSE TestSplitThenJoin/5
=== RUN   TestSplitThenJoin/6
=== PAUSE TestSplitThenJoin/6
=== RUN   TestSplitThenJoin/7
=== PAUSE TestSplitThenJoin/7
=== RUN   TestSplitThenJoin/8
=== CONT  TestChunkPipe/6
=== CONT  TestChunkPipe/2
--- PASS: TestChunkPipe (0.00s)
    --- PASS: TestChunkPipe/0 (0.00s)
    --- PASS: TestChunkPipe/5 (0.00s)
    --- PASS: TestChunkPipe/8 (0.00s)
    --- PASS: TestChunkPipe/1 (0.00s)
    --- PASS: TestChunkPipe/4 (0.00s)
    --- PASS: TestChunkPipe/3 (0.00s)
    --- PASS: TestChunkPipe/7 (0.00s)
    --- PASS: TestChunkPipe/6 (0.00s)
    --- PASS: TestChunkPipe/2 (0.00s)
=== RUN   TestCopyBuffer/buf_64__/data_size_69632
=== PAUSE TestCopyBuffer/buf_64__/data_size_69632
=== RUN   TestCopyBuffer/buf_64__/data_size_69635
=== PAUSE TestCopyBuffer/buf_64__/data_size_69635
=== RUN   TestCopyBuffer/buf_1024/data_size_1
=== PAUSE TestCopyBuffer/buf_1024/data_size_1
=== RUN   TestCopyBuffer/buf_1024/data_size_64
=== PAUSE TestCopyBuffer/buf_1024/data_size_64
=== RUN   TestCopyBuffer/buf_1024/data_size_1024
=== PAUSE TestCopyBuffer/buf_1024/data_size_1024
=== PAUSE TestSplitThenJoin/8
=== RUN   TestSplitThenJoin/9
=== PAUSE TestSplitThenJoin/9
=== RUN   TestSplitThenJoin/10
=== PAUSE TestSplitThenJoin/10
=== RUN   TestSplitThenJoin/11
=== PAUSE TestSplitThenJoin/11
=== RUN   TestSplitThenJoin/12
=== PAUSE TestSplitThenJoin/12
=== RUN   TestSplitThenJoin/13
=== PAUSE TestSplitThenJoin/13
=== RUN   TestSplitThenJoin/14
=== PAUSE TestSplitThenJoin/14
=== RUN   TestSplitThenJoin/15
=== PAUSE TestSplitThenJoin/15
=== RUN   TestSplitThenJoin/16
=== PAUSE TestSplitThenJoin/16
=== RUN   TestSplitThenJoin/17
=== PAUSE TestSplitThenJoin/17
=== RUN   TestSplitThenJoin/18
=== PAUSE TestSplitThenJoin/18
=== RUN   TestCopyBuffer/buf_1024/data_size_4095
=== CONT  TestSplitThenJoin/0
=== PAUSE TestCopyBuffer/buf_1024/data_size_4095
=== RUN   TestCopyBuffer/buf_1024/data_size_4096
=== PAUSE TestCopyBuffer/buf_1024/data_size_4096
=== CONT  TestSplitThenJoin/3
=== RUN   TestCopyBuffer/buf_1024/data_size_4097
=== PAUSE TestCopyBuffer/buf_1024/data_size_4097
=== RUN   TestCopyBuffer/buf_1024/data_size_8192
=== CONT  TestSplitThenJoin/9
=== PAUSE TestCopyBuffer/buf_1024/data_size_8192
=== RUN   TestCopyBuffer/buf_1024/data_size_8195
=== CONT  TestSplitThenJoin/10
=== PAUSE TestCopyBuffer/buf_1024/data_size_8195
=== RUN   TestCopyBuffer/buf_1024/data_size_20480
=== PAUSE TestCopyBuffer/buf_1024/data_size_20480
=== RUN   TestCopyBuffer/buf_1024/data_size_20483
=== PAUSE TestCopyBuffer/buf_1024/data_size_20483
=== RUN   TestCopyBuffer/buf_1024/data_size_69632
=== PAUSE TestCopyBuffer/buf_1024/data_size_69632
=== RUN   TestCopyBuffer/buf_1024/data_size_69635
=== PAUSE TestCopyBuffer/buf_1024/data_size_69635
=== RUN   TestCopyBuffer/buf_4096/data_size_1
=== PAUSE TestCopyBuffer/buf_4096/data_size_1
=== RUN   TestCopyBuffer/buf_4096/data_size_64
=== PAUSE TestCopyBuffer/buf_4096/data_size_64
=== RUN   TestCopyBuffer/buf_4096/data_size_1024
=== PAUSE TestCopyBuffer/buf_4096/data_size_1024
=== RUN   TestCopyBuffer/buf_4096/data_size_4095
=== PAUSE TestCopyBuffer/buf_4096/data_size_4095
=== RUN   TestCopyBuffer/buf_4096/data_size_4096
=== PAUSE TestCopyBuffer/buf_4096/data_size_4096
=== RUN   TestCopyBuffer/buf_4096/data_size_4097
=== PAUSE TestCopyBuffer/buf_4096/data_size_4097
=== CONT  TestSplitThenJoin/7
=== CONT  TestSplitThenJoin/8
=== CONT  TestSplitThenJoin/18
=== CONT  TestSplitThenJoin/4
=== CONT  TestSplitThenJoin/6
=== RUN   TestCopyBuffer/buf_4096/data_size_8192
=== PAUSE TestCopyBuffer/buf_4096/data_size_8192
=== RUN   TestCopyBuffer/buf_4096/data_size_8195
=== PAUSE TestCopyBuffer/buf_4096/data_size_8195
=== RUN   TestCopyBuffer/buf_4096/data_size_20480
=== PAUSE TestCopyBuffer/buf_4096/data_size_20480
=== RUN   TestCopyBuffer/buf_4096/data_size_20483
=== PAUSE TestCopyBuffer/buf_4096/data_size_20483
=== RUN   TestCopyBuffer/buf_4096/data_size_69632
=== PAUSE TestCopyBuffer/buf_4096/data_size_69632
=== RUN   TestCopyBuffer/buf_4096/data_size_69635
=== PAUSE TestCopyBuffer/buf_4096/data_size_69635
=== CONT  TestSplitThenJoin/5
=== CONT  TestSplitThenJoin/17
=== CONT  TestSplitThenJoin/16
=== CONT  TestSplitThenJoin/15
=== CONT  TestSplitThenJoin/14
=== CONT  TestSplitThenJoin/13
=== CONT  TestSplitThenJoin/12
=== CONT  TestSplitThenJoin/11
=== CONT  TestSplitThenJoin/2
=== CONT  TestSplitThenJoin/1
=== CONT  TestCopyBuffer/buf_64__/data_size_1
=== CONT  TestCopyBuffer/buf_4096/data_size_69635
=== CONT  TestCopyBuffer/buf_4096/data_size_69632
=== CONT  TestCopyBuffer/buf_4096/data_size_20483
=== CONT  TestCopyBuffer/buf_4096/data_size_20480
=== CONT  TestCopyBuffer/buf_4096/data_size_8195
=== CONT  TestCopyBuffer/buf_4096/data_size_8192
=== CONT  TestCopyBuffer/buf_4096/data_size_4097
=== CONT  TestCopyBuffer/buf_4096/data_size_4096
=== CONT  TestCopyBuffer/buf_4096/data_size_4095
=== CONT  TestCopyBuffer/buf_4096/data_size_1024
=== CONT  TestCopyBuffer/buf_4096/data_size_64
=== CONT  TestCopyBuffer/buf_4096/data_size_1
=== CONT  TestCopyBuffer/buf_1024/data_size_69635
=== CONT  TestCopyBuffer/buf_1024/data_size_69632
=== CONT  TestCopyBuffer/buf_1024/data_size_20483
=== CONT  TestCopyBuffer/buf_1024/data_size_20480
=== CONT  TestCopyBuffer/buf_1024/data_size_8195
=== CONT  TestCopyBuffer/buf_1024/data_size_8192
=== CONT  TestCopyBuffer/buf_1024/data_size_4097
=== CONT  TestCopyBuffer/buf_1024/data_size_4096
=== CONT  TestCopyBuffer/buf_1024/data_size_4095
=== CONT  TestCopyBuffer/buf_1024/data_size_1024
=== CONT  TestCopyBuffer/buf_1024/data_size_64
=== CONT  TestCopyBuffer/buf_1024/data_size_1
=== CONT  TestCopyBuffer/buf_64__/data_size_69635
=== CONT  TestCopyBuffer/buf_64__/data_size_69632
=== CONT  TestCopyBuffer/buf_64__/data_size_20483
=== CONT  TestCopyBuffer/buf_64__/data_size_20480
=== CONT  TestCopyBuffer/buf_64__/data_size_8195
=== CONT  TestCopyBuffer/buf_64__/data_size_8192
=== CONT  TestCopyBuffer/buf_64__/data_size_4097
=== CONT  TestCopyBuffer/buf_64__/data_size_4096
=== CONT  TestCopyBuffer/buf_64__/data_size_4095
=== CONT  TestCopyBuffer/buf_64__/data_size_1024
=== CONT  TestCopyBuffer/buf_64__/data_size_64
--- PASS: TestCopyBuffer (0.00s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_1 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_69635 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_69632 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_20483 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_20480 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_8192 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_8195 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_4096 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_4095 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_1024 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_64 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_1 (0.00s)
    --- PASS: TestCopyBuffer/buf_4096/data_size_4097 (0.00s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_69635 (0.01s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_20483 (0.00s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_20480 (0.00s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_8195 (0.00s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_8192 (0.00s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_4097 (0.00s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_4096 (0.00s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_4095 (0.00s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_1024 (0.00s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_64 (0.00s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_1 (0.00s)
    --- PASS: TestCopyBuffer/buf_1024/data_size_69632 (0.02s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_69635 (0.01s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_20483 (0.00s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_20480 (0.00s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_8195 (0.00s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_8192 (0.00s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_4097 (0.00s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_4096 (0.00s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_4095 (0.00s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_1024 (0.00s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_64 (0.00s)
    --- PASS: TestCopyBuffer/buf_64__/data_size_69632 (0.01s)
--- PASS: TestSplitThenJoin (0.00s)
    --- PASS: TestSplitThenJoin/0 (0.00s)
    --- PASS: TestSplitThenJoin/3 (0.00s)
    --- PASS: TestSplitThenJoin/10 (0.00s)
    --- PASS: TestSplitThenJoin/6 (0.00s)
    --- PASS: TestSplitThenJoin/5 (0.00s)
    --- PASS: TestSplitThenJoin/8 (0.01s)
    --- PASS: TestSplitThenJoin/7 (0.01s)
    --- PASS: TestSplitThenJoin/9 (0.01s)
    --- PASS: TestSplitThenJoin/4 (0.01s)
    --- PASS: TestSplitThenJoin/12 (0.00s)
    --- PASS: TestSplitThenJoin/2 (0.00s)
    --- PASS: TestSplitThenJoin/11 (0.00s)
    --- PASS: TestSplitThenJoin/1 (0.00s)
    --- PASS: TestSplitThenJoin/16 (0.06s)
    --- PASS: TestSplitThenJoin/17 (0.08s)
    --- PASS: TestSplitThenJoin/13 (0.07s)
    --- PASS: TestSplitThenJoin/15 (0.08s)
    --- PASS: TestSplitThenJoin/18 (0.08s)
    --- PASS: TestSplitThenJoin/14 (0.08s)
PASS
ok  	github.com/ethersphere/bee/pkg/file	(cached)
=== RUN   TestAddressesGetterIterateChunkAddresses
=== PAUSE TestAddressesGetterIterateChunkAddresses
=== CONT  TestAddressesGetterIterateChunkAddresses
--- PASS: TestAddressesGetterIterateChunkAddresses (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/file/addresses	(cached)
=== RUN   TestJoiner_ErrReferenceLength
=== PAUSE TestJoiner_ErrReferenceLength
=== RUN   TestJoinerSingleChunk
=== PAUSE TestJoinerSingleChunk
=== RUN   TestJoinerDecryptingStore_NormalChunk
=== PAUSE TestJoinerDecryptingStore_NormalChunk
=== RUN   TestJoinerWithReference
=== PAUSE TestJoinerWithReference
=== RUN   TestJoinerMalformed
=== PAUSE TestJoinerMalformed
=== RUN   TestEncryptDecrypt
=== PAUSE TestEncryptDecrypt
=== RUN   TestSeek
=== PAUSE TestSeek
=== RUN   TestPrefetch
=== PAUSE TestPrefetch
=== RUN   TestJoinerReadAt
=== PAUSE TestJoinerReadAt
=== RUN   TestJoinerOneLevel
=== PAUSE TestJoinerOneLevel
=== RUN   TestJoinerTwoLevelsAcrossChunk
=== PAUSE TestJoinerTwoLevelsAcrossChunk
=== RUN   TestJoinerIterateChunkAddresses
=== PAUSE TestJoinerIterateChunkAddresses
=== RUN   TestJoinerIterateChunkAddresses_Encrypted
=== PAUSE TestJoinerIterateChunkAddresses_Encrypted
=== CONT  TestJoiner_ErrReferenceLength
--- PASS: TestJoiner_ErrReferenceLength (0.00s)
=== CONT  TestJoinerTwoLevelsAcrossChunk
=== CONT  TestJoinerIterateChunkAddresses
=== CONT  TestJoinerOneLevel
=== CONT  TestJoinerReadAt
=== CONT  TestPrefetch
=== RUN   TestPrefetch/one_byte
=== PAUSE TestPrefetch/one_byte
=== RUN   TestPrefetch/one_byte#01
=== PAUSE TestPrefetch/one_byte#01
=== CONT  TestJoinerMalformed
=== CONT  TestSeek
=== RUN   TestSeek/one_byte
=== PAUSE TestSeek/one_byte
=== RUN   TestSeek/a_few_bytes
=== PAUSE TestSeek/a_few_bytes
=== RUN   TestSeek/a_few_bytes_more
=== PAUSE TestSeek/a_few_bytes_more
=== RUN   TestSeek/almost_a_chunk
=== PAUSE TestSeek/almost_a_chunk
=== RUN   TestSeek/one_chunk
=== PAUSE TestSeek/one_chunk
=== RUN   TestSeek/a_few_chunks
=== PAUSE TestSeek/a_few_chunks
=== RUN   TestSeek/a_few_chunks_and_a_change
=== PAUSE TestSeek/a_few_chunks_and_a_change
=== RUN   TestSeek/a_few_chunks_more
=== PAUSE TestSeek/a_few_chunks_more
=== CONT  TestJoinerIterateChunkAddresses_Encrypted
=== CONT  TestJoinerWithReference
--- PASS: TestJoinerIterateChunkAddresses (0.01s)
--- PASS: TestJoinerReadAt (0.00s)
=== CONT  TestEncryptDecrypt
=== RUN   TestEncryptDecrypt/Encrypt_10_bytes
=== PAUSE TestEncryptDecrypt/Encrypt_10_bytes
=== RUN   TestEncryptDecrypt/Encrypt_100_bytes
=== RUN   TestPrefetch/ten_bytes
--- PASS: TestJoinerWithReference (0.00s)
=== CONT  TestJoinerSingleChunk
--- PASS: TestJoinerSingleChunk (0.00s)
=== CONT  TestJoinerDecryptingStore_NormalChunk
--- PASS: TestJoinerDecryptingStore_NormalChunk (0.00s)
=== CONT  TestSeek/one_byte
--- PASS: TestJoinerMalformed (0.00s)
=== CONT  TestSeek/a_few_chunks_and_a_change
--- PASS: TestJoinerOneLevel (0.00s)
=== CONT  TestSeek/one_chunk
=== CONT  TestSeek/a_few_chunks_more
=== CONT  TestSeek/a_few_chunks
=== PAUSE TestEncryptDecrypt/Encrypt_100_bytes
=== RUN   TestEncryptDecrypt/Encrypt_1000_bytes
=== PAUSE TestEncryptDecrypt/Encrypt_1000_bytes
=== RUN   TestEncryptDecrypt/Encrypt_4095_bytes
=== PAUSE TestEncryptDecrypt/Encrypt_4095_bytes
=== RUN   TestEncryptDecrypt/Encrypt_4096_bytes
=== PAUSE TestEncryptDecrypt/Encrypt_4096_bytes
=== RUN   TestEncryptDecrypt/Encrypt_4097_bytes
=== PAUSE TestEncryptDecrypt/Encrypt_4097_bytes
=== RUN   TestEncryptDecrypt/Encrypt_1000000_bytes
=== PAUSE TestEncryptDecrypt/Encrypt_1000000_bytes
=== CONT  TestSeek/almost_a_chunk
=== PAUSE TestPrefetch/ten_bytes
=== RUN   TestPrefetch/thousand_bytes
=== PAUSE TestPrefetch/thousand_bytes
=== RUN   TestPrefetch/thousand_bytes#01
=== PAUSE TestPrefetch/thousand_bytes#01
=== RUN   TestPrefetch/thousand_bytes#02
=== PAUSE TestPrefetch/thousand_bytes#02
=== RUN   TestPrefetch/one_chunk
=== PAUSE TestPrefetch/one_chunk
=== RUN   TestPrefetch/one_chunk_minus_a_few
=== PAUSE TestPrefetch/one_chunk_minus_a_few
=== RUN   TestPrefetch/one_chunk_minus_a_few#01
=== PAUSE TestPrefetch/one_chunk_minus_a_few#01
=== RUN   TestPrefetch/one_byte_at_the_end
=== PAUSE TestPrefetch/one_byte_at_the_end
=== RUN   TestPrefetch/one_byte_at_the_end#01
=== PAUSE TestPrefetch/one_byte_at_the_end#01
=== RUN   TestPrefetch/one_byte_at_the_end#02
=== PAUSE TestPrefetch/one_byte_at_the_end#02
=== RUN   TestPrefetch/one_byte_at_the_end#03
=== PAUSE TestPrefetch/one_byte_at_the_end#03
=== RUN   TestPrefetch/10kb
=== PAUSE TestPrefetch/10kb
=== RUN   TestPrefetch/10kb#01
=== PAUSE TestPrefetch/10kb#01
=== RUN   TestPrefetch/100kb
=== PAUSE TestPrefetch/100kb
=== RUN   TestPrefetch/100kb#01
=== PAUSE TestPrefetch/100kb#01
=== RUN   TestPrefetch/10megs
=== PAUSE TestPrefetch/10megs
=== RUN   TestPrefetch/10megs#01
=== PAUSE TestPrefetch/10megs#01
=== RUN   TestPrefetch/10megs#02
=== PAUSE TestPrefetch/10megs#02
=== RUN   TestPrefetch/10megs#03
=== PAUSE TestPrefetch/10megs#03
=== CONT  TestEncryptDecrypt/Encrypt_10_bytes
=== CONT  TestSeek/a_few_bytes_more
=== CONT  TestSeek/a_few_bytes
=== CONT  TestEncryptDecrypt/Encrypt_1000000_bytes
=== CONT  TestEncryptDecrypt/Encrypt_4097_bytes
=== CONT  TestEncryptDecrypt/Encrypt_4096_bytes
=== CONT  TestEncryptDecrypt/Encrypt_4095_bytes
=== CONT  TestEncryptDecrypt/Encrypt_1000_bytes
--- PASS: TestJoinerIterateChunkAddresses_Encrypted (0.02s)
=== CONT  TestEncryptDecrypt/Encrypt_100_bytes
=== CONT  TestPrefetch/one_byte
=== CONT  TestPrefetch/one_byte_at_the_end#03
--- PASS: TestJoinerTwoLevelsAcrossChunk (0.03s)
=== CONT  TestPrefetch/one_byte_at_the_end#02
=== CONT  TestPrefetch/one_byte_at_the_end#01
=== CONT  TestPrefetch/one_byte_at_the_end
=== CONT  TestPrefetch/one_chunk_minus_a_few#01
=== CONT  TestPrefetch/one_chunk_minus_a_few
=== CONT  TestPrefetch/one_chunk
=== CONT  TestPrefetch/thousand_bytes#02
=== CONT  TestPrefetch/thousand_bytes#01
=== CONT  TestPrefetch/thousand_bytes
=== CONT  TestPrefetch/ten_bytes
=== CONT  TestPrefetch/one_byte#01
=== CONT  TestPrefetch/10megs#03
=== CONT  TestPrefetch/10megs#01
=== CONT  TestPrefetch/10megs#02
=== CONT  TestPrefetch/100kb#01
=== CONT  TestPrefetch/10megs
=== CONT  TestPrefetch/100kb
=== CONT  TestPrefetch/10kb#01
=== CONT  TestPrefetch/10kb
--- PASS: TestEncryptDecrypt (0.00s)
    --- PASS: TestEncryptDecrypt/Encrypt_10_bytes (0.00s)
    --- PASS: TestEncryptDecrypt/Encrypt_4096_bytes (0.00s)
    --- PASS: TestEncryptDecrypt/Encrypt_4095_bytes (0.00s)
    --- PASS: TestEncryptDecrypt/Encrypt_1000_bytes (0.02s)
    --- PASS: TestEncryptDecrypt/Encrypt_100_bytes (0.00s)
    --- PASS: TestEncryptDecrypt/Encrypt_4097_bytes (0.02s)
    --- PASS: TestEncryptDecrypt/Encrypt_1000000_bytes (0.47s)
--- PASS: TestPrefetch (0.01s)
    --- PASS: TestPrefetch/one_byte (0.00s)
    --- PASS: TestPrefetch/one_byte_at_the_end#02 (0.00s)
    --- PASS: TestPrefetch/one_byte_at_the_end#01 (0.00s)
    --- PASS: TestPrefetch/one_byte_at_the_end (0.00s)
    --- PASS: TestPrefetch/one_chunk_minus_a_few (0.00s)
    --- PASS: TestPrefetch/one_chunk_minus_a_few#01 (0.00s)
    --- PASS: TestPrefetch/thousand_bytes#02 (0.00s)
    --- PASS: TestPrefetch/one_chunk (0.00s)
    --- PASS: TestPrefetch/one_byte#01 (0.00s)
    --- PASS: TestPrefetch/thousand_bytes#01 (0.00s)
    --- PASS: TestPrefetch/thousand_bytes (0.00s)
    --- PASS: TestPrefetch/ten_bytes (0.00s)
    --- PASS: TestPrefetch/100kb#01 (0.01s)
    --- PASS: TestPrefetch/one_byte_at_the_end#03 (0.03s)
    --- PASS: TestPrefetch/10kb#01 (0.00s)
    --- PASS: TestPrefetch/10kb (0.00s)
    --- PASS: TestPrefetch/100kb (0.02s)
    --- PASS: TestPrefetch/10megs#03 (0.15s)
    --- PASS: TestPrefetch/10megs (0.51s)
    --- PASS: TestPrefetch/10megs#01 (0.52s)
    --- PASS: TestPrefetch/10megs#02 (0.52s)
--- PASS: TestSeek (0.00s)
    --- PASS: TestSeek/one_byte (0.00s)
    --- PASS: TestSeek/one_chunk (0.00s)
    --- PASS: TestSeek/almost_a_chunk (0.00s)
    --- PASS: TestSeek/a_few_bytes (0.00s)
    --- PASS: TestSeek/a_few_bytes_more (0.00s)
    --- PASS: TestSeek/a_few_chunks (0.02s)
    --- PASS: TestSeek/a_few_chunks_and_a_change (0.03s)
    --- PASS: TestSeek/a_few_chunks_more (1.23s)
PASS
ok  	github.com/ethersphere/bee/pkg/file/joiner	(cached)
=== RUN   TestLoadSave
=== PAUSE TestLoadSave
=== RUN   TestReadonlyLoadSave
=== PAUSE TestReadonlyLoadSave
=== CONT  TestLoadSave
--- PASS: TestLoadSave (0.00s)
=== CONT  TestReadonlyLoadSave
--- PASS: TestReadonlyLoadSave (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/file/loadsave	(cached)
?   	github.com/ethersphere/bee/pkg/file/pipeline/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/file/pipeline	[no test files]
=== RUN   TestBmtWriter
=== PAUSE TestBmtWriter
=== RUN   TestSum
=== PAUSE TestSum
=== CONT  TestBmtWriter
=== RUN   TestBmtWriter/empty_file
=== PAUSE TestBmtWriter/empty_file
=== RUN   TestBmtWriter/hello_world
=== PAUSE TestBmtWriter/hello_world
=== RUN   TestBmtWriter/no_data
=== PAUSE TestBmtWriter/no_data
=== CONT  TestBmtWriter/empty_file
=== CONT  TestSum
--- PASS: TestSum (0.00s)
=== CONT  TestBmtWriter/no_data
=== CONT  TestBmtWriter/hello_world
--- PASS: TestBmtWriter (0.00s)
    --- PASS: TestBmtWriter/empty_file (0.00s)
    --- PASS: TestBmtWriter/no_data (0.00s)
    --- PASS: TestBmtWriter/hello_world (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/file/pipeline/bmt	(cached)
=== RUN   TestPartialWrites
=== PAUSE TestPartialWrites
=== RUN   TestHelloWorld
=== PAUSE TestHelloWorld
=== RUN   TestEmpty
=== PAUSE TestEmpty
=== RUN   TestAllVectors
=== PAUSE TestAllVectors
=== CONT  TestPartialWrites
--- PASS: TestPartialWrites (0.00s)
=== CONT  TestAllVectors
=== CONT  TestEmpty
=== CONT  TestHelloWorld
--- PASS: TestHelloWorld (0.00s)
=== RUN   TestAllVectors/data_length_32,_vector_1
=== PAUSE TestAllVectors/data_length_32,_vector_1
=== RUN   TestAllVectors/data_length_33,_vector_2
=== PAUSE TestAllVectors/data_length_33,_vector_2
=== RUN   TestAllVectors/data_length_63,_vector_3
=== PAUSE TestAllVectors/data_length_63,_vector_3
=== RUN   TestAllVectors/data_length_64,_vector_4
=== PAUSE TestAllVectors/data_length_64,_vector_4
=== RUN   TestAllVectors/data_length_65,_vector_5
=== PAUSE TestAllVectors/data_length_65,_vector_5
=== RUN   TestAllVectors/data_length_4096,_vector_6
=== PAUSE TestAllVectors/data_length_4096,_vector_6
=== RUN   TestAllVectors/data_length_4127,_vector_7
=== PAUSE TestAllVectors/data_length_4127,_vector_7
=== RUN   TestAllVectors/data_length_4128,_vector_8
=== PAUSE TestAllVectors/data_length_4128,_vector_8
=== RUN   TestAllVectors/data_length_4159,_vector_9
=== PAUSE TestAllVectors/data_length_4159,_vector_9
=== RUN   TestAllVectors/data_length_4160,_vector_10
=== PAUSE TestAllVectors/data_length_4160,_vector_10
=== RUN   TestAllVectors/data_length_8192,_vector_11
=== PAUSE TestAllVectors/data_length_8192,_vector_11
=== RUN   TestAllVectors/data_length_8224,_vector_12
=== PAUSE TestAllVectors/data_length_8224,_vector_12
--- PASS: TestEmpty (0.00s)
=== RUN   TestAllVectors/data_length_524288,_vector_13
=== PAUSE TestAllVectors/data_length_524288,_vector_13
=== RUN   TestAllVectors/data_length_524319,_vector_14
=== PAUSE TestAllVectors/data_length_524319,_vector_14
=== RUN   TestAllVectors/data_length_524320,_vector_15
=== PAUSE TestAllVectors/data_length_524320,_vector_15
=== RUN   TestAllVectors/data_length_524352,_vector_16
=== PAUSE TestAllVectors/data_length_524352,_vector_16
=== RUN   TestAllVectors/data_length_528384,_vector_17
=== PAUSE TestAllVectors/data_length_528384,_vector_17
=== RUN   TestAllVectors/data_length_532480,_vector_18
=== PAUSE TestAllVectors/data_length_532480,_vector_18
=== RUN   TestAllVectors/data_length_67108864,_vector_19
=== PAUSE TestAllVectors/data_length_67108864,_vector_19
=== RUN   TestAllVectors/data_length_67108896,_vector_20
=== PAUSE TestAllVectors/data_length_67108896,_vector_20
=== CONT  TestAllVectors/data_length_32,_vector_1
=== CONT  TestAllVectors/data_length_528384,_vector_17
=== CONT  TestAllVectors/data_length_4159,_vector_9
=== CONT  TestAllVectors/data_length_524352,_vector_16
=== CONT  TestAllVectors/data_length_4160,_vector_10
=== CONT  TestAllVectors/data_length_524320,_vector_15
=== CONT  TestAllVectors/data_length_524319,_vector_14
=== CONT  TestAllVectors/data_length_524288,_vector_13
=== CONT  TestAllVectors/data_length_8224,_vector_12
=== CONT  TestAllVectors/data_length_8192,_vector_11
=== CONT  TestAllVectors/data_length_65,_vector_5
=== CONT  TestAllVectors/data_length_4128,_vector_8
=== CONT  TestAllVectors/data_length_4127,_vector_7
=== CONT  TestAllVectors/data_length_4096,_vector_6
=== CONT  TestAllVectors/data_length_63,_vector_3
=== CONT  TestAllVectors/data_length_64,_vector_4
=== CONT  TestAllVectors/data_length_67108896,_vector_20
=== CONT  TestAllVectors/data_length_33,_vector_2
=== CONT  TestAllVectors/data_length_532480,_vector_18
=== CONT  TestAllVectors/data_length_67108864,_vector_19
--- PASS: TestAllVectors (1.16s)
    --- PASS: TestAllVectors/data_length_32,_vector_1 (0.00s)
    --- PASS: TestAllVectors/data_length_4159,_vector_9 (0.00s)
    --- PASS: TestAllVectors/data_length_4160,_vector_10 (0.00s)
    --- PASS: TestAllVectors/data_length_65,_vector_5 (0.00s)
    --- PASS: TestAllVectors/data_length_4128,_vector_8 (0.00s)
    --- PASS: TestAllVectors/data_length_4127,_vector_7 (0.00s)
    --- PASS: TestAllVectors/data_length_4096,_vector_6 (0.00s)
    --- PASS: TestAllVectors/data_length_63,_vector_3 (0.00s)
    --- PASS: TestAllVectors/data_length_64,_vector_4 (0.00s)
    --- PASS: TestAllVectors/data_length_8224,_vector_12 (0.01s)
    --- PASS: TestAllVectors/data_length_33,_vector_2 (0.00s)
    --- PASS: TestAllVectors/data_length_8192,_vector_11 (0.01s)
    --- PASS: TestAllVectors/data_length_524320,_vector_15 (0.04s)
    --- PASS: TestAllVectors/data_length_524319,_vector_14 (0.04s)
    --- PASS: TestAllVectors/data_length_528384,_vector_17 (0.05s)
    --- PASS: TestAllVectors/data_length_532480,_vector_18 (0.03s)
    --- PASS: TestAllVectors/data_length_524352,_vector_16 (0.05s)
    --- PASS: TestAllVectors/data_length_524288,_vector_13 (0.06s)
    --- PASS: TestAllVectors/data_length_67108896,_vector_20 (1.87s)
    --- PASS: TestAllVectors/data_length_67108864,_vector_19 (1.92s)
PASS
ok  	github.com/ethersphere/bee/pkg/file/pipeline/builder	(cached)
=== RUN   TestEncryption
=== PAUSE TestEncryption
=== RUN   TestSum
=== PAUSE TestSum
=== CONT  TestEncryption
--- PASS: TestEncryption (0.00s)
=== CONT  TestSum
--- PASS: TestSum (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/file/pipeline/encryption	(cached)
=== RUN   TestFeeder
=== PAUSE TestFeeder
=== RUN   TestFeederFlush
=== PAUSE TestFeederFlush
=== CONT  TestFeeder
=== CONT  TestFeederFlush
=== RUN   TestFeeder/empty_write
=== PAUSE TestFeeder/empty_write
=== RUN   TestFeeder/less_than_chunk,_no_writes
=== PAUSE TestFeeder/less_than_chunk,_no_writes
=== RUN   TestFeeder/one_chunk,_one_write
=== PAUSE TestFeeder/one_chunk,_one_write
=== RUN   TestFeeder/two_chunks,_two_writes
=== PAUSE TestFeeder/two_chunks,_two_writes
=== RUN   TestFeeder/half_chunk,_then_full_one,_one_write
=== PAUSE TestFeeder/half_chunk,_then_full_one,_one_write
=== RUN   TestFeeder/half_chunk,_another_two_halves,_one_write
=== RUN   TestFeederFlush/empty_file
=== PAUSE TestFeeder/half_chunk,_another_two_halves,_one_write
=== RUN   TestFeeder/half_chunk,_another_two_halves,_another_full,_two_writes
=== PAUSE TestFeeder/half_chunk,_another_two_halves,_another_full,_two_writes
=== CONT  TestFeeder/empty_write
=== PAUSE TestFeederFlush/empty_file
=== RUN   TestFeederFlush/less_than_chunk,_one_write
=== PAUSE TestFeederFlush/less_than_chunk,_one_write
=== RUN   TestFeederFlush/one_chunk,_one_write
=== CONT  TestFeeder/two_chunks,_two_writes
=== PAUSE TestFeederFlush/one_chunk,_one_write
=== RUN   TestFeederFlush/two_chunks,_two_writes
=== PAUSE TestFeederFlush/two_chunks,_two_writes
=== RUN   TestFeederFlush/half_chunk,_then_full_one,_two_writes
=== PAUSE TestFeederFlush/half_chunk,_then_full_one,_two_writes
=== RUN   TestFeederFlush/half_chunk,_another_two_halves,_two_writes
=== PAUSE TestFeederFlush/half_chunk,_another_two_halves,_two_writes
=== RUN   TestFeederFlush/half_chunk,_another_two_halves,_another_full,_three_writes
=== CONT  TestFeeder/one_chunk,_one_write
=== PAUSE TestFeederFlush/half_chunk,_another_two_halves,_another_full,_three_writes
=== CONT  TestFeederFlush/empty_file
=== CONT  TestFeeder/less_than_chunk,_no_writes
=== CONT  TestFeeder/half_chunk,_then_full_one,_one_write
=== CONT  TestFeeder/half_chunk,_another_two_halves,_one_write
=== CONT  TestFeeder/half_chunk,_another_two_halves,_another_full,_two_writes
--- PASS: TestFeeder (0.00s)
    --- PASS: TestFeeder/empty_write (0.00s)
    --- PASS: TestFeeder/two_chunks,_two_writes (0.00s)
    --- PASS: TestFeeder/one_chunk,_one_write (0.00s)
    --- PASS: TestFeeder/less_than_chunk,_no_writes (0.00s)
    --- PASS: TestFeeder/half_chunk,_then_full_one,_one_write (0.00s)
    --- PASS: TestFeeder/half_chunk,_another_two_halves,_one_write (0.00s)
    --- PASS: TestFeeder/half_chunk,_another_two_halves,_another_full,_two_writes (0.00s)
=== CONT  TestFeederFlush/half_chunk,_another_two_halves,_another_full,_three_writes
=== CONT  TestFeederFlush/half_chunk,_another_two_halves,_two_writes
=== CONT  TestFeederFlush/half_chunk,_then_full_one,_two_writes
=== CONT  TestFeederFlush/two_chunks,_two_writes
=== CONT  TestFeederFlush/one_chunk,_one_write
=== CONT  TestFeederFlush/less_than_chunk,_one_write
--- PASS: TestFeederFlush (0.00s)
    --- PASS: TestFeederFlush/empty_file (0.00s)
    --- PASS: TestFeederFlush/half_chunk,_another_two_halves,_another_full,_three_writes (0.00s)
    --- PASS: TestFeederFlush/half_chunk,_another_two_halves,_two_writes (0.00s)
    --- PASS: TestFeederFlush/half_chunk,_then_full_one,_two_writes (0.00s)
    --- PASS: TestFeederFlush/two_chunks,_two_writes (0.00s)
    --- PASS: TestFeederFlush/one_chunk,_one_write (0.00s)
    --- PASS: TestFeederFlush/less_than_chunk,_one_write (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/file/pipeline/feeder	(cached)
=== RUN   TestLevels
=== PAUSE TestLevels
=== RUN   TestLevels_TrieFull
=== PAUSE TestLevels_TrieFull
=== RUN   TestRegression
=== PAUSE TestRegression
=== CONT  TestLevels
=== RUN   TestLevels/2_at_L1
=== PAUSE TestLevels/2_at_L1
=== RUN   TestLevels/1_at_L2,_1_at_L1
=== PAUSE TestLevels/1_at_L2,_1_at_L1
=== RUN   TestLevels/1_at_L3,_1_at_L2,_1_at_L1
=== PAUSE TestLevels/1_at_L3,_1_at_L2,_1_at_L1
=== RUN   TestLevels/1_at_L3,_2_at_L2,_1_at_L1
=== PAUSE TestLevels/1_at_L3,_2_at_L2,_1_at_L1
=== RUN   TestLevels/1_at_L5,_1_at_L1
=== PAUSE TestLevels/1_at_L5,_1_at_L1
=== RUN   TestLevels/1_at_L5,_1_at_L3
=== PAUSE TestLevels/1_at_L5,_1_at_L3
=== RUN   TestLevels/2_at_L5,_1_at_L1
=== PAUSE TestLevels/2_at_L5,_1_at_L1
=== RUN   TestLevels/3_at_L5,_2_at_L3,_1_at_L1
=== PAUSE TestLevels/3_at_L5,_2_at_L3,_1_at_L1
=== RUN   TestLevels/1_at_L7,_1_at_L1
=== PAUSE TestLevels/1_at_L7,_1_at_L1
=== RUN   TestLevels/1_at_L8
=== PAUSE TestLevels/1_at_L8
=== CONT  TestLevels/2_at_L1
=== CONT  TestRegression
=== CONT  TestLevels_TrieFull
=== CONT  TestLevels/1_at_L8
=== CONT  TestLevels/1_at_L7,_1_at_L1
=== CONT  TestLevels/3_at_L5,_2_at_L3,_1_at_L1
=== CONT  TestLevels/2_at_L5,_1_at_L1
=== CONT  TestLevels/1_at_L5,_1_at_L3
=== CONT  TestLevels/1_at_L5,_1_at_L1
=== CONT  TestLevels/1_at_L3,_2_at_L2,_1_at_L1
=== CONT  TestLevels/1_at_L3,_1_at_L2,_1_at_L1
=== CONT  TestLevels/1_at_L2,_1_at_L1
--- PASS: TestRegression (0.06s)
--- PASS: TestLevels (0.00s)
    --- PASS: TestLevels/2_at_L1 (0.00s)
    --- PASS: TestLevels/1_at_L5,_1_at_L1 (0.02s)
    --- PASS: TestLevels/1_at_L3,_2_at_L2,_1_at_L1 (0.00s)
    --- PASS: TestLevels/1_at_L3,_1_at_L2,_1_at_L1 (0.00s)
    --- PASS: TestLevels/1_at_L2,_1_at_L1 (0.00s)
    --- PASS: TestLevels/1_at_L5,_1_at_L3 (0.02s)
    --- PASS: TestLevels/2_at_L5,_1_at_L1 (0.04s)
    --- PASS: TestLevels/3_at_L5,_2_at_L3,_1_at_L1 (0.05s)
    --- PASS: TestLevels/1_at_L7,_1_at_L1 (0.06s)
    --- PASS: TestLevels/1_at_L8 (0.16s)
--- PASS: TestLevels_TrieFull (0.17s)
PASS
ok  	github.com/ethersphere/bee/pkg/file/pipeline/hashtrie	(cached)
=== RUN   TestStoreWriter
=== PAUSE TestStoreWriter
=== RUN   TestSum
=== PAUSE TestSum
=== CONT  TestStoreWriter
--- PASS: TestStoreWriter (0.00s)
=== CONT  TestSum
--- PASS: TestSum (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/file/pipeline/store	(cached)
=== RUN   TestSplitIncomplete
=== PAUSE TestSplitIncomplete
=== RUN   TestSplitSingleChunk
=== PAUSE TestSplitSingleChunk
=== RUN   TestSplitThreeLevels
=== PAUSE TestSplitThreeLevels
=== RUN   TestUnalignedSplit
=== PAUSE TestUnalignedSplit
=== CONT  TestSplitIncomplete
--- PASS: TestSplitIncomplete (0.00s)
=== CONT  TestSplitThreeLevels
=== CONT  TestSplitSingleChunk
=== CONT  TestUnalignedSplit
--- PASS: TestSplitSingleChunk (0.00s)
--- PASS: TestUnalignedSplit (0.00s)
--- PASS: TestSplitThreeLevels (0.02s)
PASS
ok  	github.com/ethersphere/bee/pkg/file/splitter	(cached)
=== RUN   TestSplitterJobPartialSingleChunk
=== PAUSE TestSplitterJobPartialSingleChunk
=== RUN   TestSplitterJobVector
=== PAUSE TestSplitterJobVector
=== CONT  TestSplitterJobPartialSingleChunk
=== CONT  TestSplitterJobVector
=== RUN   TestSplitterJobVector/0
--- PASS: TestSplitterJobPartialSingleChunk (0.00s)
=== PAUSE TestSplitterJobVector/0
=== RUN   TestSplitterJobVector/1
=== PAUSE TestSplitterJobVector/1
=== RUN   TestSplitterJobVector/2
=== PAUSE TestSplitterJobVector/2
=== RUN   TestSplitterJobVector/3
=== PAUSE TestSplitterJobVector/3
=== RUN   TestSplitterJobVector/4
=== PAUSE TestSplitterJobVector/4
=== RUN   TestSplitterJobVector/5
=== PAUSE TestSplitterJobVector/5
=== RUN   TestSplitterJobVector/6
=== PAUSE TestSplitterJobVector/6
=== RUN   TestSplitterJobVector/7
=== PAUSE TestSplitterJobVector/7
=== RUN   TestSplitterJobVector/8
=== PAUSE TestSplitterJobVector/8
=== RUN   TestSplitterJobVector/9
=== PAUSE TestSplitterJobVector/9
=== RUN   TestSplitterJobVector/10
=== PAUSE TestSplitterJobVector/10
=== RUN   TestSplitterJobVector/11
=== PAUSE TestSplitterJobVector/11
=== RUN   TestSplitterJobVector/12
=== PAUSE TestSplitterJobVector/12
=== RUN   TestSplitterJobVector/13
=== PAUSE TestSplitterJobVector/13
=== RUN   TestSplitterJobVector/14
=== PAUSE TestSplitterJobVector/14
=== RUN   TestSplitterJobVector/15
=== PAUSE TestSplitterJobVector/15
=== RUN   TestSplitterJobVector/16
=== PAUSE TestSplitterJobVector/16
=== RUN   TestSplitterJobVector/17
=== PAUSE TestSplitterJobVector/17
=== RUN   TestSplitterJobVector/18
=== PAUSE TestSplitterJobVector/18
=== CONT  TestSplitterJobVector/0
=== CONT  TestSplitterJobVector/10
=== CONT  TestSplitterJobVector/9
=== CONT  TestSplitterJobVector/18
=== CONT  TestSplitterJobVector/8
=== CONT  TestSplitterJobVector/7
=== CONT  TestSplitterJobVector/6
=== CONT  TestSplitterJobVector/5
=== CONT  TestSplitterJobVector/4
=== CONT  TestSplitterJobVector/3
=== CONT  TestSplitterJobVector/2
=== CONT  TestSplitterJobVector/1
=== CONT  TestSplitterJobVector/17
=== CONT  TestSplitterJobVector/15
=== CONT  TestSplitterJobVector/16
=== CONT  TestSplitterJobVector/14
=== CONT  TestSplitterJobVector/13
=== CONT  TestSplitterJobVector/12
=== CONT  TestSplitterJobVector/11
--- PASS: TestSplitterJobVector (0.00s)
    --- PASS: TestSplitterJobVector/0 (0.00s)
    --- PASS: TestSplitterJobVector/5 (0.00s)
    --- PASS: TestSplitterJobVector/7 (0.00s)
    --- PASS: TestSplitterJobVector/4 (0.00s)
    --- PASS: TestSplitterJobVector/3 (0.00s)
    --- PASS: TestSplitterJobVector/2 (0.00s)
    --- PASS: TestSplitterJobVector/1 (0.00s)
    --- PASS: TestSplitterJobVector/6 (0.00s)
    --- PASS: TestSplitterJobVector/9 (0.01s)
    --- PASS: TestSplitterJobVector/8 (0.03s)
    --- PASS: TestSplitterJobVector/10 (0.03s)
    --- PASS: TestSplitterJobVector/12 (0.00s)
    --- PASS: TestSplitterJobVector/11 (0.00s)
    --- PASS: TestSplitterJobVector/17 (0.05s)
    --- PASS: TestSplitterJobVector/13 (0.05s)
    --- PASS: TestSplitterJobVector/16 (0.06s)
    --- PASS: TestSplitterJobVector/18 (0.07s)
    --- PASS: TestSplitterJobVector/14 (0.04s)
    --- PASS: TestSplitterJobVector/15 (0.07s)
PASS
ok  	github.com/ethersphere/bee/pkg/file/splitter/internal	(cached)
?   	github.com/ethersphere/bee/pkg/file/testing	[no test files]
=== RUN   TestFallingEdge
=== PAUSE TestFallingEdge
=== RUN   TestFallingEdgeBuffer
=== PAUSE TestFallingEdgeBuffer
=== RUN   TestFallingEdgeWorstCase
=== PAUSE TestFallingEdgeWorstCase
=== CONT  TestFallingEdge
    falling_edge_test.go:16: github actions
--- SKIP: TestFallingEdge (0.00s)
=== CONT  TestFallingEdgeWorstCase
    falling_edge_test.go:86: github actions
--- SKIP: TestFallingEdgeWorstCase (0.00s)
=== CONT  TestFallingEdgeBuffer
    falling_edge_test.go:44: needs parameter tweaking on github actions
--- SKIP: TestFallingEdgeBuffer (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/flipflop	(cached)
=== RUN   TestHandlerRateLimit
=== PAUSE TestHandlerRateLimit
=== RUN   TestBroadcastPeers_FLAKY
=== PAUSE TestBroadcastPeers_FLAKY
=== CONT  TestHandlerRateLimit
=== CONT  TestBroadcastPeers_FLAKY
=== RUN   TestBroadcastPeers_FLAKY/OK_-_single_record
=== PAUSE TestBroadcastPeers_FLAKY/OK_-_single_record
=== RUN   TestBroadcastPeers_FLAKY/OK_-_single_batch_-_multiple_records
=== PAUSE TestBroadcastPeers_FLAKY/OK_-_single_batch_-_multiple_records
=== RUN   TestBroadcastPeers_FLAKY/OK_-_single_batch_-_max_number_of_records
=== PAUSE TestBroadcastPeers_FLAKY/OK_-_single_batch_-_max_number_of_records
=== RUN   TestBroadcastPeers_FLAKY/OK_-_multiple_batches
=== PAUSE TestBroadcastPeers_FLAKY/OK_-_multiple_batches
=== RUN   TestBroadcastPeers_FLAKY/OK_-_multiple_batches_-_max_number_of_records
=== PAUSE TestBroadcastPeers_FLAKY/OK_-_multiple_batches_-_max_number_of_records
=== RUN   TestBroadcastPeers_FLAKY/OK_-_single_batch_-_skip_ping_failures
=== PAUSE TestBroadcastPeers_FLAKY/OK_-_single_batch_-_skip_ping_failures
=== RUN   TestBroadcastPeers_FLAKY/Ok_-_don't_advertise_private_CIDRs
=== PAUSE TestBroadcastPeers_FLAKY/Ok_-_don't_advertise_private_CIDRs
=== CONT  TestBroadcastPeers_FLAKY/OK_-_single_record
=== CONT  TestBroadcastPeers_FLAKY/OK_-_multiple_batches
=== CONT  TestBroadcastPeers_FLAKY/OK_-_single_batch_-_multiple_records
=== CONT  TestBroadcastPeers_FLAKY/OK_-_single_batch_-_skip_ping_failures
=== CONT  TestBroadcastPeers_FLAKY/Ok_-_don't_advertise_private_CIDRs
=== CONT  TestBroadcastPeers_FLAKY/OK_-_single_batch_-_max_number_of_records
=== CONT  TestBroadcastPeers_FLAKY/OK_-_multiple_batches_-_max_number_of_records
--- PASS: TestHandlerRateLimit (0.09s)
--- PASS: TestBroadcastPeers_FLAKY (0.04s)
    --- PASS: TestBroadcastPeers_FLAKY/OK_-_single_record (0.04s)
    --- PASS: TestBroadcastPeers_FLAKY/Ok_-_don't_advertise_private_CIDRs (0.04s)
    --- PASS: TestBroadcastPeers_FLAKY/OK_-_single_batch_-_multiple_records (0.04s)
    --- PASS: TestBroadcastPeers_FLAKY/OK_-_single_batch_-_skip_ping_failures (0.04s)
    --- PASS: TestBroadcastPeers_FLAKY/OK_-_single_batch_-_max_number_of_records (0.06s)
    --- PASS: TestBroadcastPeers_FLAKY/OK_-_multiple_batches (0.06s)
    --- PASS: TestBroadcastPeers_FLAKY/OK_-_multiple_batches_-_max_number_of_records (0.06s)
PASS
ok  	github.com/ethersphere/bee/pkg/hive	(cached)
?   	github.com/ethersphere/bee/pkg/hive/pb	[no test files]
=== RUN   Test
=== PAUSE Test
=== RUN   TestMerge
=== PAUSE TestMerge
=== RUN   TestMaxUint64
=== PAUSE TestMaxUint64
=== RUN   TestEdgeBugUnmarshal
=== PAUSE TestEdgeBugUnmarshal
=== RUN   TestInmemoryStore
=== PAUSE TestInmemoryStore
=== RUN   TestDBStore
=== PAUSE TestDBStore
=== CONT  Test
--- PASS: Test (0.00s)
=== CONT  TestMaxUint64
--- PASS: TestMaxUint64 (0.00s)
=== CONT  TestMerge
--- PASS: TestMerge (0.00s)
=== CONT  TestInmemoryStore
--- PASS: TestInmemoryStore (0.00s)
=== CONT  TestEdgeBugUnmarshal
=== CONT  TestDBStore
--- PASS: TestEdgeBugUnmarshal (0.00s)
--- PASS: TestDBStore (0.02s)
PASS
ok  	github.com/ethersphere/bee/pkg/intervalstore	(cached)
?   	github.com/ethersphere/bee/pkg/keystore	[no test files]
=== RUN   TestMethodHandler
=== PAUSE TestMethodHandler
=== RUN   TestNotFoundHandler
=== PAUSE TestNotFoundHandler
=== RUN   TestNewMaxBodyBytesHandler
=== PAUSE TestNewMaxBodyBytesHandler
=== RUN   TestRespond_defaults
=== PAUSE TestRespond_defaults
=== RUN   TestRespond_statusResponse
=== PAUSE TestRespond_statusResponse
=== RUN   TestRespond_special
=== PAUSE TestRespond_special
=== RUN   TestRespond_custom
=== PAUSE TestRespond_custom
=== RUN   TestStandardHTTPResponds
=== PAUSE TestStandardHTTPResponds
=== RUN   TestPanicRespond
=== PAUSE TestPanicRespond
=== CONT  TestMethodHandler
=== RUN   TestMethodHandler/method_allowed
=== PAUSE TestMethodHandler/method_allowed
=== RUN   TestMethodHandler/method_not_allowed
=== PAUSE TestMethodHandler/method_not_allowed
=== CONT  TestMethodHandler/method_allowed
=== CONT  TestPanicRespond
--- PASS: TestPanicRespond (0.00s)
=== CONT  TestStandardHTTPResponds
--- PASS: TestStandardHTTPResponds (0.00s)
=== CONT  TestRespond_custom
--- PASS: TestRespond_custom (0.00s)
=== CONT  TestRespond_special
=== RUN   TestRespond_special/string_200
=== PAUSE TestRespond_special/string_200
=== RUN   TestRespond_special/string_404
=== PAUSE TestRespond_special/string_404
=== RUN   TestRespond_special/error_400
=== PAUSE TestRespond_special/error_400
=== RUN   TestRespond_special/error_500
=== PAUSE TestRespond_special/error_500
=== RUN   TestRespond_special/stringer_200
=== PAUSE TestRespond_special/stringer_200
=== RUN   TestRespond_special/stringer_403
=== PAUSE TestRespond_special/stringer_403
=== CONT  TestRespond_special/string_200
=== CONT  TestRespond_statusResponse
--- PASS: TestRespond_statusResponse (0.00s)
=== CONT  TestRespond_defaults
--- PASS: TestRespond_defaults (0.00s)
=== CONT  TestNewMaxBodyBytesHandler
=== RUN   TestNewMaxBodyBytesHandler/empty
=== PAUSE TestNewMaxBodyBytesHandler/empty
=== RUN   TestNewMaxBodyBytesHandler/within_limit_without_content_length_header
=== PAUSE TestNewMaxBodyBytesHandler/within_limit_without_content_length_header
=== RUN   TestNewMaxBodyBytesHandler/within_limit
=== PAUSE TestNewMaxBodyBytesHandler/within_limit
=== RUN   TestNewMaxBodyBytesHandler/over_limit
=== PAUSE TestNewMaxBodyBytesHandler/over_limit
=== RUN   TestNewMaxBodyBytesHandler/over_limit_without_content_length_header
=== PAUSE TestNewMaxBodyBytesHandler/over_limit_without_content_length_header
=== CONT  TestNewMaxBodyBytesHandler/empty
=== CONT  TestNotFoundHandler
--- PASS: TestNotFoundHandler (0.00s)
=== CONT  TestMethodHandler/method_not_allowed
--- PASS: TestMethodHandler (0.00s)
    --- PASS: TestMethodHandler/method_allowed (0.00s)
    --- PASS: TestMethodHandler/method_not_allowed (0.00s)
=== CONT  TestRespond_special/stringer_403
=== CONT  TestRespond_special/stringer_200
=== CONT  TestRespond_special/error_500
=== CONT  TestRespond_special/error_400
=== CONT  TestRespond_special/string_404
--- PASS: TestRespond_special (0.00s)
    --- PASS: TestRespond_special/string_200 (0.00s)
    --- PASS: TestRespond_special/stringer_403 (0.00s)
    --- PASS: TestRespond_special/stringer_200 (0.00s)
    --- PASS: TestRespond_special/error_500 (0.00s)
    --- PASS: TestRespond_special/error_400 (0.00s)
    --- PASS: TestRespond_special/string_404 (0.00s)
=== CONT  TestNewMaxBodyBytesHandler/over_limit_without_content_length_header
=== CONT  TestNewMaxBodyBytesHandler/over_limit
=== CONT  TestNewMaxBodyBytesHandler/within_limit
=== CONT  TestNewMaxBodyBytesHandler/within_limit_without_content_length_header
--- PASS: TestNewMaxBodyBytesHandler (0.00s)
    --- PASS: TestNewMaxBodyBytesHandler/empty (0.00s)
    --- PASS: TestNewMaxBodyBytesHandler/over_limit_without_content_length_header (0.00s)
    --- PASS: TestNewMaxBodyBytesHandler/over_limit (0.00s)
    --- PASS: TestNewMaxBodyBytesHandler/within_limit (0.00s)
    --- PASS: TestNewMaxBodyBytesHandler/within_limit_without_content_length_header (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/jsonhttp	(cached)
?   	github.com/ethersphere/bee/pkg/keystore/test	[no test files]
=== RUN   TestRequest_statusCode
=== PAUSE TestRequest_statusCode
=== RUN   TestRequest_method
=== PAUSE TestRequest_method
=== RUN   TestRequest_url
=== PAUSE TestRequest_url
=== RUN   TestRequest_responseHeader
=== PAUSE TestRequest_responseHeader
=== RUN   TestWithContext
=== PAUSE TestWithContext
=== RUN   TestWithRequestBody
=== PAUSE TestWithRequestBody
=== RUN   TestWithJSONRequestBody
=== PAUSE TestWithJSONRequestBody
=== RUN   TestWithMultipartRequest
=== PAUSE TestWithMultipartRequest
=== RUN   TestWithRequestHeader
=== PAUSE TestWithRequestHeader
=== RUN   TestWithExpectedResponse
=== PAUSE TestWithExpectedResponse
=== RUN   TestWithExpectedJSONResponse
=== PAUSE TestWithExpectedJSONResponse
=== RUN   TestWithUnmarhalJSONResponse
=== PAUSE TestWithUnmarhalJSONResponse
=== RUN   TestWithPutResponseBody
=== PAUSE TestWithPutResponseBody
=== RUN   TestWithNoResponseBody
=== PAUSE TestWithNoResponseBody
=== CONT  TestRequest_statusCode
=== CONT  TestWithNoResponseBody
=== CONT  TestWithPutResponseBody
=== CONT  TestWithUnmarhalJSONResponse
=== CONT  TestWithExpectedJSONResponse
=== CONT  TestWithExpectedResponse
=== CONT  TestWithRequestHeader
=== CONT  TestWithMultipartRequest
--- PASS: TestWithMultipartRequest (0.00s)
=== CONT  TestWithJSONRequestBody
--- PASS: TestWithPutResponseBody (0.00s)
=== CONT  TestWithRequestBody
--- PASS: TestWithRequestHeader (0.00s)
=== CONT  TestWithContext
--- PASS: TestWithExpectedResponse (0.00s)
=== CONT  TestRequest_responseHeader
--- PASS: TestWithContext (0.00s)
=== CONT  TestRequest_url
--- PASS: TestWithExpectedJSONResponse (0.00s)
--- PASS: TestWithUnmarhalJSONResponse (0.00s)
--- PASS: TestWithNoResponseBody (0.00s)
--- PASS: TestRequest_statusCode (0.00s)
=== CONT  TestRequest_method
--- PASS: TestWithRequestBody (0.00s)
--- PASS: TestWithJSONRequestBody (0.00s)
--- PASS: TestRequest_responseHeader (0.00s)
--- PASS: TestRequest_url (0.00s)
--- PASS: TestRequest_method (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest	(cached)
=== RUN   TestService
=== PAUSE TestService
=== CONT  TestService
--- PASS: TestService (0.45s)
PASS
ok  	github.com/ethersphere/bee/pkg/keystore/file	(cached)
=== RUN   TestService
=== PAUSE TestService
=== CONT  TestService
--- PASS: TestService (0.02s)
PASS
ok  	github.com/ethersphere/bee/pkg/keystore/mem	(cached)
=== RUN   TestRecovery
--- PASS: TestRecovery (0.01s)
=== RUN   TestExportImport
--- PASS: TestExportImport (0.04s)
=== RUN   TestDB_collectGarbageWorker
=== RUN   TestDB_collectGarbageWorker/pull_index_count
=== RUN   TestDB_collectGarbageWorker/postage_index_count
=== RUN   TestDB_collectGarbageWorker/gc_index_count
=== RUN   TestDB_collectGarbageWorker/gc_size
=== RUN   TestDB_collectGarbageWorker/get_the_first_synced_chunk
=== RUN   TestDB_collectGarbageWorker/only_first_inserted_chunks_should_be_removed
=== RUN   TestDB_collectGarbageWorker/get_most_recent_synced_chunk
--- PASS: TestDB_collectGarbageWorker (0.04s)
    --- PASS: TestDB_collectGarbageWorker/pull_index_count (0.00s)
    --- PASS: TestDB_collectGarbageWorker/postage_index_count (0.00s)
    --- PASS: TestDB_collectGarbageWorker/gc_index_count (0.00s)
    --- PASS: TestDB_collectGarbageWorker/gc_size (0.00s)
    --- PASS: TestDB_collectGarbageWorker/get_the_first_synced_chunk (0.00s)
    --- PASS: TestDB_collectGarbageWorker/only_first_inserted_chunks_should_be_removed (0.00s)
    --- PASS: TestDB_collectGarbageWorker/get_most_recent_synced_chunk (0.00s)
=== RUN   TestDB_collectGarbageWorker_multipleBatches
=== RUN   TestDB_collectGarbageWorker_multipleBatches/pull_index_count
=== RUN   TestDB_collectGarbageWorker_multipleBatches/postage_index_count
=== RUN   TestDB_collectGarbageWorker_multipleBatches/gc_index_count
=== RUN   TestDB_collectGarbageWorker_multipleBatches/gc_size
=== RUN   TestDB_collectGarbageWorker_multipleBatches/get_the_first_synced_chunk
=== RUN   TestDB_collectGarbageWorker_multipleBatches/only_first_inserted_chunks_should_be_removed
=== RUN   TestDB_collectGarbageWorker_multipleBatches/get_most_recent_synced_chunk
--- PASS: TestDB_collectGarbageWorker_multipleBatches (0.04s)
    --- PASS: TestDB_collectGarbageWorker_multipleBatches/pull_index_count (0.00s)
    --- PASS: TestDB_collectGarbageWorker_multipleBatches/postage_index_count (0.00s)
    --- PASS: TestDB_collectGarbageWorker_multipleBatches/gc_index_count (0.00s)
    --- PASS: TestDB_collectGarbageWorker_multipleBatches/gc_size (0.00s)
    --- PASS: TestDB_collectGarbageWorker_multipleBatches/get_the_first_synced_chunk (0.00s)
    --- PASS: TestDB_collectGarbageWorker_multipleBatches/only_first_inserted_chunks_should_be_removed (0.00s)
    --- PASS: TestDB_collectGarbageWorker_multipleBatches/get_most_recent_synced_chunk (0.00s)
=== RUN   TestPinGC
    gc_test.go:216: 90
=== RUN   TestPinGC/pin_Index_count
=== RUN   TestPinGC/pull_index_count
=== RUN   TestPinGC/postage_index_count
=== RUN   TestPinGC/postage_chunks_count
=== RUN   TestPinGC/gc_index_count
=== RUN   TestPinGC/gc_size
=== RUN   TestPinGC/pinned_chunk_not_in_gc_Index
=== RUN   TestPinGC/pinned_chunks_exists
=== RUN   TestPinGC/first_chunks_after_pinned_chunks_should_be_removed
--- PASS: TestPinGC (0.03s)
    --- PASS: TestPinGC/pin_Index_count (0.00s)
    --- PASS: TestPinGC/pull_index_count (0.00s)
    --- PASS: TestPinGC/postage_index_count (0.00s)
    --- PASS: TestPinGC/postage_chunks_count (0.00s)
    --- PASS: TestPinGC/gc_index_count (0.00s)
    --- PASS: TestPinGC/gc_size (0.00s)
    --- PASS: TestPinGC/pinned_chunk_not_in_gc_Index (0.00s)
    --- PASS: TestPinGC/pinned_chunks_exists (0.00s)
    --- PASS: TestPinGC/first_chunks_after_pinned_chunks_should_be_removed (0.00s)
=== RUN   TestGCAfterPin
=== RUN   TestGCAfterPin/pin_Index_count
=== RUN   TestGCAfterPin/gc_index_count
=== RUN   TestGCAfterPin/postage_index_count
--- PASS: TestGCAfterPin (0.01s)
    --- PASS: TestGCAfterPin/pin_Index_count (0.00s)
    --- PASS: TestGCAfterPin/gc_index_count (0.00s)
    --- PASS: TestGCAfterPin/postage_index_count (0.00s)
=== RUN   TestDB_collectGarbageWorker_withRequests
=== RUN   TestDB_collectGarbageWorker_withRequests/pull_index_count
=== RUN   TestDB_collectGarbageWorker_withRequests/postage_index_count
=== RUN   TestDB_collectGarbageWorker_withRequests/postage_chunks_count
=== RUN   TestDB_collectGarbageWorker_withRequests/gc_index_count
=== RUN   TestDB_collectGarbageWorker_withRequests/gc_size
=== RUN   TestDB_collectGarbageWorker_withRequests/get_requested_chunk
=== RUN   TestDB_collectGarbageWorker_withRequests/get_gc-ed_chunk
=== RUN   TestDB_collectGarbageWorker_withRequests/get_most_recent_synced_chunk
--- PASS: TestDB_collectGarbageWorker_withRequests (0.02s)
    --- PASS: TestDB_collectGarbageWorker_withRequests/pull_index_count (0.00s)
    --- PASS: TestDB_collectGarbageWorker_withRequests/postage_index_count (0.00s)
    --- PASS: TestDB_collectGarbageWorker_withRequests/postage_chunks_count (0.00s)
    --- PASS: TestDB_collectGarbageWorker_withRequests/gc_index_count (0.00s)
    --- PASS: TestDB_collectGarbageWorker_withRequests/gc_size (0.00s)
    --- PASS: TestDB_collectGarbageWorker_withRequests/get_requested_chunk (0.00s)
    --- PASS: TestDB_collectGarbageWorker_withRequests/get_gc-ed_chunk (0.00s)
    --- PASS: TestDB_collectGarbageWorker_withRequests/get_most_recent_synced_chunk (0.00s)
=== RUN   TestDB_gcSize
=== RUN   TestDB_gcSize/gc_index_size
--- PASS: TestDB_gcSize (0.14s)
    --- PASS: TestDB_gcSize/gc_index_size (0.00s)
=== RUN   TestSetTestHookCollectGarbage
--- PASS: TestSetTestHookCollectGarbage (0.00s)
=== RUN   TestPinAfterMultiGC
=== RUN   TestPinAfterMultiGC/pin_Index_count
--- PASS: TestPinAfterMultiGC (0.06s)
    --- PASS: TestPinAfterMultiGC/pin_Index_count (0.00s)
=== RUN   TestPinSyncAndAccessPutSetChunkMultipleTimes
--- PASS: TestPinSyncAndAccessPutSetChunkMultipleTimes (0.03s)
=== RUN   TestGC_NoEvictDirty
=== RUN   TestGC_NoEvictDirty/pull_index_count
=== RUN   TestGC_NoEvictDirty/postage_index_count
=== RUN   TestGC_NoEvictDirty/postage_chunks_count
=== RUN   TestGC_NoEvictDirty/gc_index_count
=== RUN   TestGC_NoEvictDirty/gc_size
=== RUN   TestGC_NoEvictDirty/get_the_first_two_chunks,_third_is_gone
=== RUN   TestGC_NoEvictDirty/only_later_inserted_chunks_should_be_removed
=== RUN   TestGC_NoEvictDirty/get_most_recent_synced_chunk
--- PASS: TestGC_NoEvictDirty (0.20s)
    --- PASS: TestGC_NoEvictDirty/pull_index_count (0.00s)
    --- PASS: TestGC_NoEvictDirty/postage_index_count (0.00s)
    --- PASS: TestGC_NoEvictDirty/postage_chunks_count (0.00s)
    --- PASS: TestGC_NoEvictDirty/gc_index_count (0.00s)
    --- PASS: TestGC_NoEvictDirty/gc_size (0.00s)
    --- PASS: TestGC_NoEvictDirty/get_the_first_two_chunks,_third_is_gone (0.00s)
    --- PASS: TestGC_NoEvictDirty/only_later_inserted_chunks_should_be_removed (0.00s)
    --- PASS: TestGC_NoEvictDirty/get_most_recent_synced_chunk (0.00s)
=== RUN   TestReserveEvictionWorker
=== RUN   TestReserveEvictionWorker/reserve_size
=== RUN   TestReserveEvictionWorker/pull_index_count
=== RUN   TestReserveEvictionWorker/postage_index_count
=== RUN   TestReserveEvictionWorker/postage_chunks_count
=== RUN   TestReserveEvictionWorker/postage_radius_count
=== RUN   TestReserveEvictionWorker/gc_index_count
=== RUN   TestReserveEvictionWorker/gc_size
=== RUN   TestReserveEvictionWorker/retrievalDataIndex_count
=== RUN   TestReserveEvictionWorker/all_chunks_should_be_accessible
=== RUN   TestReserveEvictionWorker/reserve_size#01
=== RUN   TestReserveEvictionWorker/reserve_size#02
=== RUN   TestReserveEvictionWorker/pull_index_count#01
=== RUN   TestReserveEvictionWorker/postage_index_count#01
=== RUN   TestReserveEvictionWorker/postage_chunks_count#01
=== RUN   TestReserveEvictionWorker/postage_radius_count#01
=== RUN   TestReserveEvictionWorker/retrievalDataIndex_count#01
=== RUN   TestReserveEvictionWorker/gc_index_count#01
=== RUN   TestReserveEvictionWorker/atleast_7/10_of_the_first_chunks_should_be_accessible
--- PASS: TestReserveEvictionWorker (0.01s)
    --- PASS: TestReserveEvictionWorker/reserve_size (0.00s)
    --- PASS: TestReserveEvictionWorker/pull_index_count (0.00s)
    --- PASS: TestReserveEvictionWorker/postage_index_count (0.00s)
    --- PASS: TestReserveEvictionWorker/postage_chunks_count (0.00s)
    --- PASS: TestReserveEvictionWorker/postage_radius_count (0.00s)
    --- PASS: TestReserveEvictionWorker/gc_index_count (0.00s)
    --- PASS: TestReserveEvictionWorker/gc_size (0.00s)
    --- PASS: TestReserveEvictionWorker/retrievalDataIndex_count (0.00s)
    --- PASS: TestReserveEvictionWorker/all_chunks_should_be_accessible (0.00s)
    --- PASS: TestReserveEvictionWorker/reserve_size#01 (0.00s)
    --- PASS: TestReserveEvictionWorker/reserve_size#02 (0.00s)
    --- PASS: TestReserveEvictionWorker/pull_index_count#01 (0.00s)
    --- PASS: TestReserveEvictionWorker/postage_index_count#01 (0.00s)
    --- PASS: TestReserveEvictionWorker/postage_chunks_count#01 (0.00s)
    --- PASS: TestReserveEvictionWorker/postage_radius_count#01 (0.00s)
    --- PASS: TestReserveEvictionWorker/retrievalDataIndex_count#01 (0.00s)
    --- PASS: TestReserveEvictionWorker/gc_index_count#01 (0.00s)
    --- PASS: TestReserveEvictionWorker/atleast_7/10_of_the_first_chunks_should_be_accessible (0.00s)
=== RUN   TestDB_pullIndex
--- PASS: TestDB_pullIndex (0.01s)
=== RUN   TestDB_gcIndex
=== RUN   TestDB_gcIndex/request_unsynced
=== RUN   TestDB_gcIndex/sync_one_chunk
=== RUN   TestDB_gcIndex/sync_all_chunks
=== RUN   TestDB_gcIndex/request_one_chunk
=== RUN   TestDB_gcIndex/random_chunk_request
=== RUN   TestDB_gcIndex/remove_one_chunk
--- PASS: TestDB_gcIndex (0.01s)
    --- PASS: TestDB_gcIndex/request_unsynced (0.00s)
    --- PASS: TestDB_gcIndex/sync_one_chunk (0.00s)
    --- PASS: TestDB_gcIndex/sync_all_chunks (0.00s)
    --- PASS: TestDB_gcIndex/request_one_chunk (0.00s)
    --- PASS: TestDB_gcIndex/random_chunk_request (0.00s)
    --- PASS: TestDB_gcIndex/remove_one_chunk (0.00s)
=== RUN   TestCacheCapacity
--- PASS: TestCacheCapacity (0.00s)
=== RUN   TestDB
--- PASS: TestDB (0.00s)
=== RUN   TestDB_updateGCSem
--- PASS: TestDB_updateGCSem (2.00s)
=== RUN   TestParallelPutAndGet
--- PASS: TestParallelPutAndGet (0.26s)
=== RUN   TestGenerateTestRandomChunk
--- PASS: TestGenerateTestRandomChunk (0.00s)
=== RUN   TestSetNow
--- PASS: TestSetNow (0.00s)
=== RUN   TestDBDebugIndexes
--- PASS: TestDBDebugIndexes (0.00s)
=== RUN   TestOneMigration
--- PASS: TestOneMigration (0.19s)
=== RUN   TestManyMigrations
--- PASS: TestManyMigrations (0.21s)
=== RUN   TestMigrationErrorFrom
--- PASS: TestMigrationErrorFrom (0.20s)
=== RUN   TestMigrationErrorTo
--- PASS: TestMigrationErrorTo (0.19s)
=== RUN   TestModeGetMulti
=== RUN   TestModeGetMulti/Request
=== RUN   TestModeGetMulti/Sync
=== RUN   TestModeGetMulti/Lookup
--- PASS: TestModeGetMulti (0.01s)
    --- PASS: TestModeGetMulti/Request (0.00s)
    --- PASS: TestModeGetMulti/Sync (0.00s)
    --- PASS: TestModeGetMulti/Lookup (0.00s)
=== RUN   TestModeGetRequest
=== RUN   TestModeGetRequest/get_unsynced
=== RUN   TestModeGetRequest/get_unsynced/retrieve_indexes
=== RUN   TestModeGetRequest/get_unsynced/gc_index_count
=== RUN   TestModeGetRequest/get_unsynced/gc_size
=== RUN   TestModeGetRequest/first_get
=== RUN   TestModeGetRequest/first_get/retrieve_indexes
=== RUN   TestModeGetRequest/first_get/gc_index
=== RUN   TestModeGetRequest/first_get/access_count
=== RUN   TestModeGetRequest/first_get/gc_index_count
=== RUN   TestModeGetRequest/first_get/gc_size
=== RUN   TestModeGetRequest/second_get
=== RUN   TestModeGetRequest/second_get/retrieve_indexes
=== RUN   TestModeGetRequest/second_get/gc_index
=== RUN   TestModeGetRequest/second_get/access_count
=== RUN   TestModeGetRequest/second_get/gc_index_count
=== RUN   TestModeGetRequest/second_get/gc_size
=== RUN   TestModeGetRequest/multi
=== RUN   TestModeGetRequest/multi/retrieve_indexes
=== RUN   TestModeGetRequest/multi/gc_index
=== RUN   TestModeGetRequest/multi/gc_index_count
=== RUN   TestModeGetRequest/multi/gc_size
--- PASS: TestModeGetRequest (0.00s)
    --- PASS: TestModeGetRequest/get_unsynced (0.00s)
        --- PASS: TestModeGetRequest/get_unsynced/retrieve_indexes (0.00s)
        --- PASS: TestModeGetRequest/get_unsynced/gc_index_count (0.00s)
        --- PASS: TestModeGetRequest/get_unsynced/gc_size (0.00s)
    --- PASS: TestModeGetRequest/first_get (0.00s)
        --- PASS: TestModeGetRequest/first_get/retrieve_indexes (0.00s)
        --- PASS: TestModeGetRequest/first_get/gc_index (0.00s)
        --- PASS: TestModeGetRequest/first_get/access_count (0.00s)
        --- PASS: TestModeGetRequest/first_get/gc_index_count (0.00s)
        --- PASS: TestModeGetRequest/first_get/gc_size (0.00s)
    --- PASS: TestModeGetRequest/second_get (0.00s)
        --- PASS: TestModeGetRequest/second_get/retrieve_indexes (0.00s)
        --- PASS: TestModeGetRequest/second_get/gc_index (0.00s)
        --- PASS: TestModeGetRequest/second_get/access_count (0.00s)
        --- PASS: TestModeGetRequest/second_get/gc_index_count (0.00s)
        --- PASS: TestModeGetRequest/second_get/gc_size (0.00s)
    --- PASS: TestModeGetRequest/multi (0.00s)
        --- PASS: TestModeGetRequest/multi/retrieve_indexes (0.00s)
        --- PASS: TestModeGetRequest/multi/gc_index (0.00s)
        --- PASS: TestModeGetRequest/multi/gc_index_count (0.00s)
        --- PASS: TestModeGetRequest/multi/gc_size (0.00s)
=== RUN   TestModeGetSync
=== RUN   TestModeGetSync/retrieve_indexes
=== RUN   TestModeGetSync/gc_index_count
=== RUN   TestModeGetSync/gc_size
=== RUN   TestModeGetSync/multi
--- PASS: TestModeGetSync (0.00s)
    --- PASS: TestModeGetSync/retrieve_indexes (0.00s)
    --- PASS: TestModeGetSync/gc_index_count (0.00s)
    --- PASS: TestModeGetSync/gc_size (0.00s)
    --- PASS: TestModeGetSync/multi (0.00s)
=== RUN   TestSetTestHookUpdateGC
--- PASS: TestSetTestHookUpdateGC (0.00s)
=== RUN   TestHas
--- PASS: TestHas (0.00s)
=== RUN   TestHasMulti
=== RUN   TestHasMulti/one
=== RUN   TestHasMulti/two
=== RUN   TestHasMulti/eight
=== RUN   TestHasMulti/hundred
=== RUN   TestHasMulti/thousand
--- PASS: TestHasMulti (0.05s)
    --- PASS: TestHasMulti/one (0.00s)
    --- PASS: TestHasMulti/two (0.00s)
    --- PASS: TestHasMulti/eight (0.00s)
    --- PASS: TestHasMulti/hundred (0.00s)
    --- PASS: TestHasMulti/thousand (0.04s)
=== RUN   TestModePutRequest
=== RUN   TestModePutRequest/one
=== RUN   TestModePutRequest/one/first_put
=== RUN   TestModePutRequest/one/second_put
=== RUN   TestModePutRequest/two
=== RUN   TestModePutRequest/two/first_put
=== RUN   TestModePutRequest/two/second_put
=== RUN   TestModePutRequest/eight
=== RUN   TestModePutRequest/eight/first_put
=== RUN   TestModePutRequest/eight/second_put
=== RUN   TestModePutRequest/hundred
=== RUN   TestModePutRequest/hundred/first_put
=== RUN   TestModePutRequest/hundred/second_put
=== RUN   TestModePutRequest/thousand
=== RUN   TestModePutRequest/thousand/first_put
=== RUN   TestModePutRequest/thousand/second_put
--- PASS: TestModePutRequest (0.11s)
    --- PASS: TestModePutRequest/one (0.00s)
        --- PASS: TestModePutRequest/one/first_put (0.00s)
        --- PASS: TestModePutRequest/one/second_put (0.00s)
    --- PASS: TestModePutRequest/two (0.00s)
        --- PASS: TestModePutRequest/two/first_put (0.00s)
        --- PASS: TestModePutRequest/two/second_put (0.00s)
    --- PASS: TestModePutRequest/eight (0.00s)
        --- PASS: TestModePutRequest/eight/first_put (0.00s)
        --- PASS: TestModePutRequest/eight/second_put (0.00s)
    --- PASS: TestModePutRequest/hundred (0.01s)
        --- PASS: TestModePutRequest/hundred/first_put (0.01s)
        --- PASS: TestModePutRequest/hundred/second_put (0.00s)
    --- PASS: TestModePutRequest/thousand (0.08s)
        --- PASS: TestModePutRequest/thousand/first_put (0.05s)
        --- PASS: TestModePutRequest/thousand/second_put (0.02s)
=== RUN   TestModePutRequestPin
=== RUN   TestModePutRequestPin/one
=== RUN   TestModePutRequestPin/two
=== RUN   TestModePutRequestPin/eight
=== RUN   TestModePutRequestPin/hundred
=== RUN   TestModePutRequestPin/thousand
--- PASS: TestModePutRequestPin (0.09s)
    --- PASS: TestModePutRequestPin/one (0.00s)
    --- PASS: TestModePutRequestPin/two (0.00s)
    --- PASS: TestModePutRequestPin/eight (0.00s)
    --- PASS: TestModePutRequestPin/hundred (0.01s)
    --- PASS: TestModePutRequestPin/thousand (0.07s)
=== RUN   TestModePutRequestCache
=== RUN   TestModePutRequestCache/one
=== RUN   TestModePutRequestCache/two
=== RUN   TestModePutRequestCache/eight
=== RUN   TestModePutRequestCache/hundred
=== RUN   TestModePutRequestCache/thousand
--- PASS: TestModePutRequestCache (0.10s)
    --- PASS: TestModePutRequestCache/one (0.00s)
    --- PASS: TestModePutRequestCache/two (0.01s)
    --- PASS: TestModePutRequestCache/eight (0.00s)
    --- PASS: TestModePutRequestCache/hundred (0.01s)
    --- PASS: TestModePutRequestCache/thousand (0.08s)
=== RUN   TestModePutSync
=== RUN   TestModePutSync/one
=== RUN   TestModePutSync/two
=== RUN   TestModePutSync/eight
=== RUN   TestModePutSync/hundred
=== RUN   TestModePutSync/thousand
--- PASS: TestModePutSync (0.09s)
    --- PASS: TestModePutSync/one (0.00s)
    --- PASS: TestModePutSync/two (0.00s)
    --- PASS: TestModePutSync/eight (0.00s)
    --- PASS: TestModePutSync/hundred (0.01s)
    --- PASS: TestModePutSync/thousand (0.08s)
=== RUN   TestModePutUpload
=== RUN   TestModePutUpload/one
=== RUN   TestModePutUpload/two
=== RUN   TestModePutUpload/eight
=== RUN   TestModePutUpload/hundred
=== RUN   TestModePutUpload/thousand
--- PASS: TestModePutUpload (0.08s)
    --- PASS: TestModePutUpload/one (0.00s)
    --- PASS: TestModePutUpload/two (0.00s)
    --- PASS: TestModePutUpload/eight (0.00s)
    --- PASS: TestModePutUpload/hundred (0.00s)
    --- PASS: TestModePutUpload/thousand (0.07s)
=== RUN   TestModePutSyncUpload_SameIndex
    mode_put_test.go:290: po 0 0 po 1 1
--- PASS: TestModePutSyncUpload_SameIndex (0.00s)
=== RUN   TestModePutUploadPin
=== RUN   TestModePutUploadPin/one
=== RUN   TestModePutUploadPin/two
=== RUN   TestModePutUploadPin/eight
=== RUN   TestModePutUploadPin/hundred
=== RUN   TestModePutUploadPin/thousand
--- PASS: TestModePutUploadPin (0.09s)
    --- PASS: TestModePutUploadPin/one (0.00s)
    --- PASS: TestModePutUploadPin/two (0.00s)
    --- PASS: TestModePutUploadPin/eight (0.00s)
    --- PASS: TestModePutUploadPin/hundred (0.01s)
    --- PASS: TestModePutUploadPin/thousand (0.08s)
=== RUN   TestModePutUpload_parallel
=== RUN   TestModePutUpload_parallel/one
=== RUN   TestModePutUpload_parallel/two
=== RUN   TestModePutUpload_parallel/eight
--- PASS: TestModePutUpload_parallel (0.22s)
    --- PASS: TestModePutUpload_parallel/one (0.02s)
    --- PASS: TestModePutUpload_parallel/two (0.04s)
    --- PASS: TestModePutUpload_parallel/eight (0.16s)
=== RUN   TestModePut_sameChunk
=== RUN   TestModePut_sameChunk/one
=== RUN   TestModePut_sameChunk/one/ModePutRequest
=== RUN   TestModePut_sameChunk/one/ModePutRequestPin
=== RUN   TestModePut_sameChunk/one/ModePutRequestCache
=== RUN   TestModePut_sameChunk/one/ModePutUpload
=== RUN   TestModePut_sameChunk/one/ModePutSync
=== RUN   TestModePut_sameChunk/two
=== RUN   TestModePut_sameChunk/two/ModePutRequest
=== RUN   TestModePut_sameChunk/two/ModePutRequestPin
=== RUN   TestModePut_sameChunk/two/ModePutRequestCache
=== RUN   TestModePut_sameChunk/two/ModePutUpload
=== RUN   TestModePut_sameChunk/two/ModePutSync
=== RUN   TestModePut_sameChunk/eight
=== RUN   TestModePut_sameChunk/eight/ModePutRequest
=== RUN   TestModePut_sameChunk/eight/ModePutRequestPin
=== RUN   TestModePut_sameChunk/eight/ModePutRequestCache
=== RUN   TestModePut_sameChunk/eight/ModePutUpload
=== RUN   TestModePut_sameChunk/eight/ModePutSync
=== RUN   TestModePut_sameChunk/hundred
=== RUN   TestModePut_sameChunk/hundred/ModePutRequest
=== RUN   TestModePut_sameChunk/hundred/ModePutRequestPin
=== RUN   TestModePut_sameChunk/hundred/ModePutRequestCache
=== RUN   TestModePut_sameChunk/hundred/ModePutUpload
=== RUN   TestModePut_sameChunk/hundred/ModePutSync
=== RUN   TestModePut_sameChunk/thousand
=== RUN   TestModePut_sameChunk/thousand/ModePutRequest
=== RUN   TestModePut_sameChunk/thousand/ModePutRequestPin
=== RUN   TestModePut_sameChunk/thousand/ModePutRequestCache
=== RUN   TestModePut_sameChunk/thousand/ModePutUpload
=== RUN   TestModePut_sameChunk/thousand/ModePutSync
--- PASS: TestModePut_sameChunk (0.52s)
    --- PASS: TestModePut_sameChunk/one (0.01s)
        --- PASS: TestModePut_sameChunk/one/ModePutRequest (0.00s)
        --- PASS: TestModePut_sameChunk/one/ModePutRequestPin (0.00s)
        --- PASS: TestModePut_sameChunk/one/ModePutRequestCache (0.00s)
        --- PASS: TestModePut_sameChunk/one/ModePutUpload (0.00s)
        --- PASS: TestModePut_sameChunk/one/ModePutSync (0.00s)
    --- PASS: TestModePut_sameChunk/two (0.01s)
        --- PASS: TestModePut_sameChunk/two/ModePutRequest (0.00s)
        --- PASS: TestModePut_sameChunk/two/ModePutRequestPin (0.00s)
        --- PASS: TestModePut_sameChunk/two/ModePutRequestCache (0.00s)
        --- PASS: TestModePut_sameChunk/two/ModePutUpload (0.00s)
        --- PASS: TestModePut_sameChunk/two/ModePutSync (0.00s)
    --- PASS: TestModePut_sameChunk/eight (0.01s)
        --- PASS: TestModePut_sameChunk/eight/ModePutRequest (0.00s)
        --- PASS: TestModePut_sameChunk/eight/ModePutRequestPin (0.00s)
        --- PASS: TestModePut_sameChunk/eight/ModePutRequestCache (0.00s)
        --- PASS: TestModePut_sameChunk/eight/ModePutUpload (0.00s)
        --- PASS: TestModePut_sameChunk/eight/ModePutSync (0.00s)
    --- PASS: TestModePut_sameChunk/hundred (0.04s)
        --- PASS: TestModePut_sameChunk/hundred/ModePutRequest (0.01s)
        --- PASS: TestModePut_sameChunk/hundred/ModePutRequestPin (0.01s)
        --- PASS: TestModePut_sameChunk/hundred/ModePutRequestCache (0.01s)
        --- PASS: TestModePut_sameChunk/hundred/ModePutUpload (0.01s)
        --- PASS: TestModePut_sameChunk/hundred/ModePutSync (0.01s)
    --- PASS: TestModePut_sameChunk/thousand (0.45s)
        --- PASS: TestModePut_sameChunk/thousand/ModePutRequest (0.08s)
        --- PASS: TestModePut_sameChunk/thousand/ModePutRequestPin (0.07s)
        --- PASS: TestModePut_sameChunk/thousand/ModePutRequestCache (0.10s)
        --- PASS: TestModePut_sameChunk/thousand/ModePutUpload (0.08s)
        --- PASS: TestModePut_sameChunk/thousand/ModePutSync (0.09s)
=== RUN   TestModePut_SameStamp
=== RUN   TestModePut_SameStamp/RequestRequest
=== RUN   TestModePut_SameStamp/RequestRequest#01
=== RUN   TestModePut_SameStamp/RequestRequest#02
=== RUN   TestModePut_SameStamp/RequestRequestPin
=== RUN   TestModePut_SameStamp/RequestRequestPin#01
=== RUN   TestModePut_SameStamp/RequestRequestPin#02
=== RUN   TestModePut_SameStamp/RequestSync
=== RUN   TestModePut_SameStamp/RequestSync#01
=== RUN   TestModePut_SameStamp/RequestSync#02
=== RUN   TestModePut_SameStamp/RequestUpload
=== RUN   TestModePut_SameStamp/RequestUpload#01
=== RUN   TestModePut_SameStamp/RequestUpload#02
=== RUN   TestModePut_SameStamp/RequestUploadPin
=== RUN   TestModePut_SameStamp/RequestUploadPin#01
=== RUN   TestModePut_SameStamp/RequestUploadPin#02
=== RUN   TestModePut_SameStamp/RequestRequestCache
=== RUN   TestModePut_SameStamp/RequestRequestCache#01
=== RUN   TestModePut_SameStamp/RequestRequestCache#02
=== RUN   TestModePut_SameStamp/RequestPinRequest
=== RUN   TestModePut_SameStamp/RequestPinRequest#01
=== RUN   TestModePut_SameStamp/RequestPinRequest#02
=== RUN   TestModePut_SameStamp/RequestPinRequestPin
=== RUN   TestModePut_SameStamp/RequestPinRequestPin#01
=== RUN   TestModePut_SameStamp/RequestPinRequestPin#02
=== RUN   TestModePut_SameStamp/RequestPinSync
=== RUN   TestModePut_SameStamp/RequestPinSync#01
=== RUN   TestModePut_SameStamp/RequestPinSync#02
=== RUN   TestModePut_SameStamp/RequestPinUpload
=== RUN   TestModePut_SameStamp/RequestPinUpload#01
=== RUN   TestModePut_SameStamp/RequestPinUpload#02
=== RUN   TestModePut_SameStamp/RequestPinUploadPin
=== RUN   TestModePut_SameStamp/RequestPinUploadPin#01
=== RUN   TestModePut_SameStamp/RequestPinUploadPin#02
=== RUN   TestModePut_SameStamp/RequestPinRequestCache
=== RUN   TestModePut_SameStamp/RequestPinRequestCache#01
=== RUN   TestModePut_SameStamp/RequestPinRequestCache#02
=== RUN   TestModePut_SameStamp/SyncRequest
=== RUN   TestModePut_SameStamp/SyncRequest#01
=== RUN   TestModePut_SameStamp/SyncRequest#02
=== RUN   TestModePut_SameStamp/SyncRequestPin
=== RUN   TestModePut_SameStamp/SyncRequestPin#01
=== RUN   TestModePut_SameStamp/SyncRequestPin#02
=== RUN   TestModePut_SameStamp/SyncSync
=== RUN   TestModePut_SameStamp/SyncSync#01
=== RUN   TestModePut_SameStamp/SyncSync#02
=== RUN   TestModePut_SameStamp/SyncUpload
=== RUN   TestModePut_SameStamp/SyncUpload#01
=== RUN   TestModePut_SameStamp/SyncUpload#02
=== RUN   TestModePut_SameStamp/SyncUploadPin
=== RUN   TestModePut_SameStamp/SyncUploadPin#01
=== RUN   TestModePut_SameStamp/SyncUploadPin#02
=== RUN   TestModePut_SameStamp/SyncRequestCache
=== RUN   TestModePut_SameStamp/SyncRequestCache#01
=== RUN   TestModePut_SameStamp/SyncRequestCache#02
=== RUN   TestModePut_SameStamp/UploadRequest
=== RUN   TestModePut_SameStamp/UploadRequest#01
=== RUN   TestModePut_SameStamp/UploadRequest#02
=== RUN   TestModePut_SameStamp/UploadRequestPin
=== RUN   TestModePut_SameStamp/UploadRequestPin#01
=== RUN   TestModePut_SameStamp/UploadRequestPin#02
=== RUN   TestModePut_SameStamp/UploadSync
=== RUN   TestModePut_SameStamp/UploadSync#01
=== RUN   TestModePut_SameStamp/UploadSync#02
=== RUN   TestModePut_SameStamp/UploadUpload
=== RUN   TestModePut_SameStamp/UploadUpload#01
=== RUN   TestModePut_SameStamp/UploadUpload#02
=== RUN   TestModePut_SameStamp/UploadUploadPin
=== RUN   TestModePut_SameStamp/UploadUploadPin#01
=== RUN   TestModePut_SameStamp/UploadUploadPin#02
=== RUN   TestModePut_SameStamp/UploadRequestCache
=== RUN   TestModePut_SameStamp/UploadRequestCache#01
=== RUN   TestModePut_SameStamp/UploadRequestCache#02
=== RUN   TestModePut_SameStamp/UploadPinRequest
=== RUN   TestModePut_SameStamp/UploadPinRequest#01
=== RUN   TestModePut_SameStamp/UploadPinRequest#02
=== RUN   TestModePut_SameStamp/UploadPinRequestPin
=== RUN   TestModePut_SameStamp/UploadPinRequestPin#01
=== RUN   TestModePut_SameStamp/UploadPinRequestPin#02
=== RUN   TestModePut_SameStamp/UploadPinSync
=== RUN   TestModePut_SameStamp/UploadPinSync#01
=== RUN   TestModePut_SameStamp/UploadPinSync#02
=== RUN   TestModePut_SameStamp/UploadPinUpload
=== RUN   TestModePut_SameStamp/UploadPinUpload#01
=== RUN   TestModePut_SameStamp/UploadPinUpload#02
=== RUN   TestModePut_SameStamp/UploadPinUploadPin
=== RUN   TestModePut_SameStamp/UploadPinUploadPin#01
=== RUN   TestModePut_SameStamp/UploadPinUploadPin#02
=== RUN   TestModePut_SameStamp/UploadPinRequestCache
=== RUN   TestModePut_SameStamp/UploadPinRequestCache#01
=== RUN   TestModePut_SameStamp/UploadPinRequestCache#02
=== RUN   TestModePut_SameStamp/RequestCacheRequest
=== RUN   TestModePut_SameStamp/RequestCacheRequest#01
=== RUN   TestModePut_SameStamp/RequestCacheRequest#02
=== RUN   TestModePut_SameStamp/RequestCacheRequestPin
=== RUN   TestModePut_SameStamp/RequestCacheRequestPin#01
=== RUN   TestModePut_SameStamp/RequestCacheRequestPin#02
=== RUN   TestModePut_SameStamp/RequestCacheSync
=== RUN   TestModePut_SameStamp/RequestCacheSync#01
=== RUN   TestModePut_SameStamp/RequestCacheSync#02
=== RUN   TestModePut_SameStamp/RequestCacheUpload
=== RUN   TestModePut_SameStamp/RequestCacheUpload#01
=== RUN   TestModePut_SameStamp/RequestCacheUpload#02
=== RUN   TestModePut_SameStamp/RequestCacheUploadPin
=== RUN   TestModePut_SameStamp/RequestCacheUploadPin#01
=== RUN   TestModePut_SameStamp/RequestCacheUploadPin#02
=== RUN   TestModePut_SameStamp/RequestCacheRequestCache
=== RUN   TestModePut_SameStamp/RequestCacheRequestCache#01
=== RUN   TestModePut_SameStamp/RequestCacheRequestCache#02
--- PASS: TestModePut_SameStamp (0.21s)
    --- PASS: TestModePut_SameStamp/RequestRequest (0.00s)
    --- PASS: TestModePut_SameStamp/RequestRequest#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestRequest#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestRequestPin (0.00s)
    --- PASS: TestModePut_SameStamp/RequestRequestPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestRequestPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestSync (0.00s)
    --- PASS: TestModePut_SameStamp/RequestSync#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestSync#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestUpload (0.00s)
    --- PASS: TestModePut_SameStamp/RequestUpload#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestUpload#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestUploadPin (0.00s)
    --- PASS: TestModePut_SameStamp/RequestUploadPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestUploadPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestRequestCache (0.00s)
    --- PASS: TestModePut_SameStamp/RequestRequestCache#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestRequestCache#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinRequest (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinRequest#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinRequest#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinRequestPin (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinRequestPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinRequestPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinSync (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinSync#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinSync#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinUpload (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinUpload#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinUpload#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinUploadPin (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinUploadPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinUploadPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinRequestCache (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinRequestCache#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestPinRequestCache#02 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncRequest (0.01s)
    --- PASS: TestModePut_SameStamp/SyncRequest#01 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncRequest#02 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncRequestPin (0.00s)
    --- PASS: TestModePut_SameStamp/SyncRequestPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncRequestPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncSync (0.00s)
    --- PASS: TestModePut_SameStamp/SyncSync#01 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncSync#02 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncUpload (0.00s)
    --- PASS: TestModePut_SameStamp/SyncUpload#01 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncUpload#02 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncUploadPin (0.00s)
    --- PASS: TestModePut_SameStamp/SyncUploadPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncUploadPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncRequestCache (0.00s)
    --- PASS: TestModePut_SameStamp/SyncRequestCache#01 (0.00s)
    --- PASS: TestModePut_SameStamp/SyncRequestCache#02 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadRequest (0.00s)
    --- PASS: TestModePut_SameStamp/UploadRequest#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadRequest#02 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadRequestPin (0.00s)
    --- PASS: TestModePut_SameStamp/UploadRequestPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadRequestPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadSync (0.00s)
    --- PASS: TestModePut_SameStamp/UploadSync#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadSync#02 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadUpload (0.00s)
    --- PASS: TestModePut_SameStamp/UploadUpload#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadUpload#02 (0.01s)
    --- PASS: TestModePut_SameStamp/UploadUploadPin (0.00s)
    --- PASS: TestModePut_SameStamp/UploadUploadPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadUploadPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadRequestCache (0.01s)
    --- PASS: TestModePut_SameStamp/UploadRequestCache#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadRequestCache#02 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinRequest (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinRequest#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinRequest#02 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinRequestPin (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinRequestPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinRequestPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinSync (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinSync#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinSync#02 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinUpload (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinUpload#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinUpload#02 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinUploadPin (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinUploadPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinUploadPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinRequestCache (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinRequestCache#01 (0.00s)
    --- PASS: TestModePut_SameStamp/UploadPinRequestCache#02 (0.01s)
    --- PASS: TestModePut_SameStamp/RequestCacheRequest (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheRequest#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheRequest#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheRequestPin (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheRequestPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheRequestPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheSync (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheSync#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheSync#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheUpload (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheUpload#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheUpload#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheUploadPin (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheUploadPin#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheUploadPin#02 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheRequestCache (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheRequestCache#01 (0.00s)
    --- PASS: TestModePut_SameStamp/RequestCacheRequestCache#02 (0.00s)
=== RUN   TestModePut_ImmutableStamp
=== RUN   TestModePut_ImmutableStamp/Request_Request_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Request_Request_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Request_Request_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Request_RequestPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Request_RequestPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Request_RequestPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Request_Sync_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Request_Sync_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Request_Sync_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Request_Upload_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Request_Upload_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Request_Upload_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Request_UploadPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Request_UploadPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Request_UploadPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Request_RequestCache_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Request_RequestCache_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Request_RequestCache_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestPin_Request_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestPin_Request_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestPin_Request_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestPin_RequestPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestPin_RequestPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestPin_RequestPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestPin_Sync_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestPin_Sync_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestPin_Sync_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestPin_Upload_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestPin_Upload_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestPin_Upload_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestPin_UploadPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestPin_UploadPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestPin_UploadPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestPin_RequestCache_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestPin_RequestCache_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestPin_RequestCache_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Sync_Request_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Sync_Request_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Sync_Request_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Sync_RequestPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Sync_RequestPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Sync_RequestPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Sync_Sync_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Sync_Sync_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Sync_Sync_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Sync_Upload_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Sync_Upload_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Sync_Upload_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Sync_UploadPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Sync_UploadPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Sync_UploadPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Sync_RequestCache_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Sync_RequestCache_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Sync_RequestCache_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Upload_Request_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Upload_Request_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Upload_Request_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Upload_RequestPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Upload_RequestPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Upload_RequestPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Upload_Sync_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Upload_Sync_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Upload_Sync_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Upload_Upload_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Upload_Upload_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Upload_Upload_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Upload_UploadPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Upload_UploadPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Upload_UploadPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/Upload_RequestCache_same_timestamps
=== RUN   TestModePut_ImmutableStamp/Upload_RequestCache_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/Upload_RequestCache_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/UploadPin_Request_same_timestamps
=== RUN   TestModePut_ImmutableStamp/UploadPin_Request_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/UploadPin_Request_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/UploadPin_RequestPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/UploadPin_RequestPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/UploadPin_RequestPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/UploadPin_Sync_same_timestamps
=== RUN   TestModePut_ImmutableStamp/UploadPin_Sync_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/UploadPin_Sync_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/UploadPin_Upload_same_timestamps
=== RUN   TestModePut_ImmutableStamp/UploadPin_Upload_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/UploadPin_Upload_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/UploadPin_UploadPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/UploadPin_UploadPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/UploadPin_UploadPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/UploadPin_RequestCache_same_timestamps
=== RUN   TestModePut_ImmutableStamp/UploadPin_RequestCache_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/UploadPin_RequestCache_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestCache_Request_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestCache_Request_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestCache_Request_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestCache_RequestPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestCache_RequestPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestCache_RequestPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestCache_Sync_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestCache_Sync_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestCache_Sync_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestCache_Upload_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestCache_Upload_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestCache_Upload_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestCache_UploadPin_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestCache_UploadPin_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestCache_UploadPin_higher_timestamp_first
=== RUN   TestModePut_ImmutableStamp/RequestCache_RequestCache_same_timestamps
=== RUN   TestModePut_ImmutableStamp/RequestCache_RequestCache_higher_timestamp
=== RUN   TestModePut_ImmutableStamp/RequestCache_RequestCache_higher_timestamp_first
--- PASS: TestModePut_ImmutableStamp (0.21s)
    --- PASS: TestModePut_ImmutableStamp/Request_Request_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_Request_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_Request_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_RequestPin_same_timestamps (0.01s)
    --- PASS: TestModePut_ImmutableStamp/Request_RequestPin_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_RequestPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_Sync_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_Sync_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_Sync_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_Upload_same_timestamps (0.01s)
    --- PASS: TestModePut_ImmutableStamp/Request_Upload_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_Upload_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_UploadPin_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_UploadPin_higher_timestamp (0.01s)
    --- PASS: TestModePut_ImmutableStamp/Request_UploadPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_RequestCache_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_RequestCache_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Request_RequestCache_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_Request_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_Request_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_Request_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_RequestPin_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_RequestPin_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_RequestPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_Sync_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_Sync_higher_timestamp (0.01s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_Sync_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_Upload_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_Upload_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_Upload_higher_timestamp_first (0.01s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_UploadPin_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_UploadPin_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_UploadPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_RequestCache_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_RequestCache_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestPin_RequestCache_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_Request_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_Request_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_Request_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_RequestPin_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_RequestPin_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_RequestPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_Sync_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_Sync_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_Sync_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_Upload_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_Upload_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_Upload_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_UploadPin_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_UploadPin_higher_timestamp (0.01s)
    --- PASS: TestModePut_ImmutableStamp/Sync_UploadPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_RequestCache_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_RequestCache_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Sync_RequestCache_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_Request_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_Request_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_Request_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_RequestPin_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_RequestPin_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_RequestPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_Sync_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_Sync_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_Sync_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_Upload_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_Upload_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_Upload_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_UploadPin_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_UploadPin_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_UploadPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_RequestCache_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_RequestCache_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/Upload_RequestCache_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_Request_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_Request_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_Request_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_RequestPin_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_RequestPin_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_RequestPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_Sync_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_Sync_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_Sync_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_Upload_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_Upload_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_Upload_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_UploadPin_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_UploadPin_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_UploadPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_RequestCache_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_RequestCache_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/UploadPin_RequestCache_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_Request_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_Request_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_Request_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_RequestPin_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_RequestPin_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_RequestPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_Sync_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_Sync_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_Sync_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_Upload_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_Upload_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_Upload_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_UploadPin_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_UploadPin_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_UploadPin_higher_timestamp_first (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_RequestCache_same_timestamps (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_RequestCache_higher_timestamp (0.00s)
    --- PASS: TestModePut_ImmutableStamp/RequestCache_RequestCache_higher_timestamp_first (0.00s)
=== RUN   TestPutDuplicateChunks
=== RUN   TestPutDuplicateChunks/Upload
=== RUN   TestPutDuplicateChunks/Request
=== RUN   TestPutDuplicateChunks/Sync
--- PASS: TestPutDuplicateChunks (0.00s)
    --- PASS: TestPutDuplicateChunks/Upload (0.00s)
    --- PASS: TestPutDuplicateChunks/Request (0.00s)
    --- PASS: TestPutDuplicateChunks/Sync (0.00s)
=== RUN   TestReleaseLocations
--- PASS: TestReleaseLocations (0.00s)
=== RUN   TestModeSetRemove
=== RUN   TestModeSetRemove/one
=== RUN   TestModeSetRemove/one/retrieve_indexes
=== RUN   TestModeSetRemove/one/retrieve_indexes/retrieve_data_index_count
=== RUN   TestModeSetRemove/one/retrieve_indexes/retrieve_access_index_count
=== RUN   TestModeSetRemove/one/pull_index_count
=== RUN   TestModeSetRemove/one/gc_index_count
=== RUN   TestModeSetRemove/one/gc_size
=== RUN   TestModeSetRemove/two
=== RUN   TestModeSetRemove/two/retrieve_indexes
=== RUN   TestModeSetRemove/two/retrieve_indexes/retrieve_data_index_count
=== RUN   TestModeSetRemove/two/retrieve_indexes/retrieve_access_index_count
=== RUN   TestModeSetRemove/two/pull_index_count
=== RUN   TestModeSetRemove/two/gc_index_count
=== RUN   TestModeSetRemove/two/gc_size
=== RUN   TestModeSetRemove/eight
=== RUN   TestModeSetRemove/eight/retrieve_indexes
=== RUN   TestModeSetRemove/eight/retrieve_indexes/retrieve_data_index_count
=== RUN   TestModeSetRemove/eight/retrieve_indexes/retrieve_access_index_count
=== RUN   TestModeSetRemove/eight/pull_index_count
=== RUN   TestModeSetRemove/eight/gc_index_count
=== RUN   TestModeSetRemove/eight/gc_size
=== RUN   TestModeSetRemove/hundred
=== RUN   TestModeSetRemove/hundred/retrieve_indexes
=== RUN   TestModeSetRemove/hundred/retrieve_indexes/retrieve_data_index_count
=== RUN   TestModeSetRemove/hundred/retrieve_indexes/retrieve_access_index_count
=== RUN   TestModeSetRemove/hundred/pull_index_count
=== RUN   TestModeSetRemove/hundred/gc_index_count
=== RUN   TestModeSetRemove/hundred/gc_size
=== RUN   TestModeSetRemove/thousand
=== RUN   TestModeSetRemove/thousand/retrieve_indexes
=== RUN   TestModeSetRemove/thousand/retrieve_indexes/retrieve_data_index_count
=== RUN   TestModeSetRemove/thousand/retrieve_indexes/retrieve_access_index_count
=== RUN   TestModeSetRemove/thousand/pull_index_count
=== RUN   TestModeSetRemove/thousand/gc_index_count
=== RUN   TestModeSetRemove/thousand/gc_size
--- PASS: TestModeSetRemove (0.07s)
    --- PASS: TestModeSetRemove/one (0.00s)
        --- PASS: TestModeSetRemove/one/retrieve_indexes (0.00s)
            --- PASS: TestModeSetRemove/one/retrieve_indexes/retrieve_data_index_count (0.00s)
            --- PASS: TestModeSetRemove/one/retrieve_indexes/retrieve_access_index_count (0.00s)
        --- PASS: TestModeSetRemove/one/pull_index_count (0.00s)
        --- PASS: TestModeSetRemove/one/gc_index_count (0.00s)
        --- PASS: TestModeSetRemove/one/gc_size (0.00s)
    --- PASS: TestModeSetRemove/two (0.00s)
        --- PASS: TestModeSetRemove/two/retrieve_indexes (0.00s)
            --- PASS: TestModeSetRemove/two/retrieve_indexes/retrieve_data_index_count (0.00s)
            --- PASS: TestModeSetRemove/two/retrieve_indexes/retrieve_access_index_count (0.00s)
        --- PASS: TestModeSetRemove/two/pull_index_count (0.00s)
        --- PASS: TestModeSetRemove/two/gc_index_count (0.00s)
        --- PASS: TestModeSetRemove/two/gc_size (0.00s)
    --- PASS: TestModeSetRemove/eight (0.00s)
        --- PASS: TestModeSetRemove/eight/retrieve_indexes (0.00s)
            --- PASS: TestModeSetRemove/eight/retrieve_indexes/retrieve_data_index_count (0.00s)
            --- PASS: TestModeSetRemove/eight/retrieve_indexes/retrieve_access_index_count (0.00s)
        --- PASS: TestModeSetRemove/eight/pull_index_count (0.00s)
        --- PASS: TestModeSetRemove/eight/gc_index_count (0.00s)
        --- PASS: TestModeSetRemove/eight/gc_size (0.00s)
    --- PASS: TestModeSetRemove/hundred (0.01s)
        --- PASS: TestModeSetRemove/hundred/retrieve_indexes (0.00s)
            --- PASS: TestModeSetRemove/hundred/retrieve_indexes/retrieve_data_index_count (0.00s)
            --- PASS: TestModeSetRemove/hundred/retrieve_indexes/retrieve_access_index_count (0.00s)
        --- PASS: TestModeSetRemove/hundred/pull_index_count (0.00s)
        --- PASS: TestModeSetRemove/hundred/gc_index_count (0.00s)
        --- PASS: TestModeSetRemove/hundred/gc_size (0.00s)
    --- PASS: TestModeSetRemove/thousand (0.06s)
        --- PASS: TestModeSetRemove/thousand/retrieve_indexes (0.00s)
            --- PASS: TestModeSetRemove/thousand/retrieve_indexes/retrieve_data_index_count (0.00s)
            --- PASS: TestModeSetRemove/thousand/retrieve_indexes/retrieve_access_index_count (0.00s)
        --- PASS: TestModeSetRemove/thousand/pull_index_count (0.00s)
        --- PASS: TestModeSetRemove/thousand/gc_index_count (0.00s)
        --- PASS: TestModeSetRemove/thousand/gc_size (0.00s)
=== RUN   TestModeSetRemove_WithSync
=== RUN   TestModeSetRemove_WithSync/one
=== RUN   TestModeSetRemove_WithSync/one/retrieve_indexes
=== RUN   TestModeSetRemove_WithSync/one/retrieve_indexes/retrieve_data_index_count
=== RUN   TestModeSetRemove_WithSync/one/retrieve_indexes/retrieve_access_index_count
=== RUN   TestModeSetRemove_WithSync/one/postage_chunks_index_count
=== RUN   TestModeSetRemove_WithSync/one/postage_index_index_count
=== RUN   TestModeSetRemove_WithSync/one/pull_index_count
=== RUN   TestModeSetRemove_WithSync/one/gc_index_count
=== RUN   TestModeSetRemove_WithSync/one/gc_size
=== RUN   TestModeSetRemove_WithSync/two
=== RUN   TestModeSetRemove_WithSync/two/retrieve_indexes
=== RUN   TestModeSetRemove_WithSync/two/retrieve_indexes/retrieve_data_index_count
=== RUN   TestModeSetRemove_WithSync/two/retrieve_indexes/retrieve_access_index_count
=== RUN   TestModeSetRemove_WithSync/two/postage_chunks_index_count
=== RUN   TestModeSetRemove_WithSync/two/postage_index_index_count
=== RUN   TestModeSetRemove_WithSync/two/pull_index_count
=== RUN   TestModeSetRemove_WithSync/two/gc_index_count
=== RUN   TestModeSetRemove_WithSync/two/gc_size
=== RUN   TestModeSetRemove_WithSync/eight
=== RUN   TestModeSetRemove_WithSync/eight/retrieve_indexes
=== RUN   TestModeSetRemove_WithSync/eight/retrieve_indexes/retrieve_data_index_count
=== RUN   TestModeSetRemove_WithSync/eight/retrieve_indexes/retrieve_access_index_count
=== RUN   TestModeSetRemove_WithSync/eight/postage_chunks_index_count
=== RUN   TestModeSetRemove_WithSync/eight/postage_index_index_count
=== RUN   TestModeSetRemove_WithSync/eight/pull_index_count
=== RUN   TestModeSetRemove_WithSync/eight/gc_index_count
=== RUN   TestModeSetRemove_WithSync/eight/gc_size
=== RUN   TestModeSetRemove_WithSync/hundred
=== RUN   TestModeSetRemove_WithSync/hundred/retrieve_indexes
=== RUN   TestModeSetRemove_WithSync/hundred/retrieve_indexes/retrieve_data_index_count
=== RUN   TestModeSetRemove_WithSync/hundred/retrieve_indexes/retrieve_access_index_count
=== RUN   TestModeSetRemove_WithSync/hundred/postage_chunks_index_count
=== RUN   TestModeSetRemove_WithSync/hundred/postage_index_index_count
=== RUN   TestModeSetRemove_WithSync/hundred/pull_index_count
=== RUN   TestModeSetRemove_WithSync/hundred/gc_index_count
=== RUN   TestModeSetRemove_WithSync/hundred/gc_size
=== RUN   TestModeSetRemove_WithSync/thousand
=== RUN   TestModeSetRemove_WithSync/thousand/retrieve_indexes
=== RUN   TestModeSetRemove_WithSync/thousand/retrieve_indexes/retrieve_data_index_count
=== RUN   TestModeSetRemove_WithSync/thousand/retrieve_indexes/retrieve_access_index_count
=== RUN   TestModeSetRemove_WithSync/thousand/postage_chunks_index_count
=== RUN   TestModeSetRemove_WithSync/thousand/postage_index_index_count
=== RUN   TestModeSetRemove_WithSync/thousand/pull_index_count
=== RUN   TestModeSetRemove_WithSync/thousand/gc_index_count
=== RUN   TestModeSetRemove_WithSync/thousand/gc_size
--- PASS: TestModeSetRemove_WithSync (0.10s)
    --- PASS: TestModeSetRemove_WithSync/one (0.00s)
        --- PASS: TestModeSetRemove_WithSync/one/retrieve_indexes (0.00s)
            --- PASS: TestModeSetRemove_WithSync/one/retrieve_indexes/retrieve_data_index_count (0.00s)
            --- PASS: TestModeSetRemove_WithSync/one/retrieve_indexes/retrieve_access_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/one/postage_chunks_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/one/postage_index_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/one/pull_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/one/gc_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/one/gc_size (0.00s)
    --- PASS: TestModeSetRemove_WithSync/two (0.00s)
        --- PASS: TestModeSetRemove_WithSync/two/retrieve_indexes (0.00s)
            --- PASS: TestModeSetRemove_WithSync/two/retrieve_indexes/retrieve_data_index_count (0.00s)
            --- PASS: TestModeSetRemove_WithSync/two/retrieve_indexes/retrieve_access_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/two/postage_chunks_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/two/postage_index_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/two/pull_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/two/gc_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/two/gc_size (0.00s)
    --- PASS: TestModeSetRemove_WithSync/eight (0.00s)
        --- PASS: TestModeSetRemove_WithSync/eight/retrieve_indexes (0.00s)
            --- PASS: TestModeSetRemove_WithSync/eight/retrieve_indexes/retrieve_data_index_count (0.00s)
            --- PASS: TestModeSetRemove_WithSync/eight/retrieve_indexes/retrieve_access_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/eight/postage_chunks_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/eight/postage_index_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/eight/pull_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/eight/gc_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/eight/gc_size (0.00s)
    --- PASS: TestModeSetRemove_WithSync/hundred (0.01s)
        --- PASS: TestModeSetRemove_WithSync/hundred/retrieve_indexes (0.00s)
            --- PASS: TestModeSetRemove_WithSync/hundred/retrieve_indexes/retrieve_data_index_count (0.00s)
            --- PASS: TestModeSetRemove_WithSync/hundred/retrieve_indexes/retrieve_access_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/hundred/postage_chunks_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/hundred/postage_index_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/hundred/pull_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/hundred/gc_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/hundred/gc_size (0.00s)
    --- PASS: TestModeSetRemove_WithSync/thousand (0.09s)
        --- PASS: TestModeSetRemove_WithSync/thousand/retrieve_indexes (0.00s)
            --- PASS: TestModeSetRemove_WithSync/thousand/retrieve_indexes/retrieve_data_index_count (0.00s)
            --- PASS: TestModeSetRemove_WithSync/thousand/retrieve_indexes/retrieve_access_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/thousand/postage_chunks_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/thousand/postage_index_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/thousand/pull_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/thousand/gc_index_count (0.00s)
        --- PASS: TestModeSetRemove_WithSync/thousand/gc_size (0.00s)
=== RUN   TestPinCounter
=== RUN   TestPinCounter/+1_after_first_pin
=== RUN   TestPinCounter/2_after_second_pin
=== RUN   TestPinCounter/1_after_first_unpin
=== RUN   TestPinCounter/not_found_after_second_unpin
--- PASS: TestPinCounter (0.00s)
    --- PASS: TestPinCounter/+1_after_first_pin (0.00s)
    --- PASS: TestPinCounter/2_after_second_pin (0.00s)
    --- PASS: TestPinCounter/1_after_first_unpin (0.00s)
    --- PASS: TestPinCounter/not_found_after_second_unpin (0.00s)
=== RUN   TestPinIndexes
=== RUN   TestPinIndexes/putUpload
=== RUN   TestPinIndexes/putUpload/retrieval_data_Index_count
=== RUN   TestPinIndexes/putUpload/retrieval_access_Index_count
=== RUN   TestPinIndexes/putUpload/push_Index_count
=== RUN   TestPinIndexes/putUpload/pull_Index_count
=== RUN   TestPinIndexes/putUpload/pin_Index_count
=== RUN   TestPinIndexes/putUpload/gc_index_count
=== RUN   TestPinIndexes/putUpload/gc_size
=== RUN   TestPinIndexes/setSync
=== RUN   TestPinIndexes/setSync/retrieval_data_Index_count
=== RUN   TestPinIndexes/setSync/retrieval_access_Index_count
=== RUN   TestPinIndexes/setSync/push_Index_count
=== RUN   TestPinIndexes/setSync/pull_Index_count
=== RUN   TestPinIndexes/setSync/pin_Index_count
=== RUN   TestPinIndexes/setSync/gc_index_count
=== RUN   TestPinIndexes/setSync/gc_size
=== RUN   TestPinIndexes/setPin
=== RUN   TestPinIndexes/setPin/retrieval_data_Index_count
=== RUN   TestPinIndexes/setPin/retrieval_access_Index_count
=== RUN   TestPinIndexes/setPin/push_Index_count
=== RUN   TestPinIndexes/setPin/pull_Index_count
=== RUN   TestPinIndexes/setPin/pin_Index_count
=== RUN   TestPinIndexes/setPin/gc_index_count
=== RUN   TestPinIndexes/setPin/gc_size
=== RUN   TestPinIndexes/setPin_2
=== RUN   TestPinIndexes/setPin_2/retrieval_data_Index_count
=== RUN   TestPinIndexes/setPin_2/retrieval_access_Index_count
=== RUN   TestPinIndexes/setPin_2/push_Index_count
=== RUN   TestPinIndexes/setPin_2/pull_Index_count
=== RUN   TestPinIndexes/setPin_2/pin_Index_count
=== RUN   TestPinIndexes/setPin_2/gc_index_count
=== RUN   TestPinIndexes/setPin_2/gc_size
=== RUN   TestPinIndexes/setUnPin
=== RUN   TestPinIndexes/setUnPin/retrieval_data_Index_count
=== RUN   TestPinIndexes/setUnPin/retrieval_access_Index_count
=== RUN   TestPinIndexes/setUnPin/push_Index_count
=== RUN   TestPinIndexes/setUnPin/pull_Index_count
=== RUN   TestPinIndexes/setUnPin/pin_Index_count
=== RUN   TestPinIndexes/setUnPin/gc_index_count
=== RUN   TestPinIndexes/setUnPin/gc_size
=== RUN   TestPinIndexes/setUnPin_2
=== RUN   TestPinIndexes/setUnPin_2/retrieval_data_Index_count
=== RUN   TestPinIndexes/setUnPin_2/retrieval_access_Index_count
=== RUN   TestPinIndexes/setUnPin_2/push_Index_count
=== RUN   TestPinIndexes/setUnPin_2/pull_Index_count
=== RUN   TestPinIndexes/setUnPin_2/pin_Index_count
=== RUN   TestPinIndexes/setUnPin_2/gc_index_count
=== RUN   TestPinIndexes/setUnPin_2/gc_size
--- PASS: TestPinIndexes (0.00s)
    --- PASS: TestPinIndexes/putUpload (0.00s)
        --- PASS: TestPinIndexes/putUpload/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexes/putUpload/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexes/putUpload/push_Index_count (0.00s)
        --- PASS: TestPinIndexes/putUpload/pull_Index_count (0.00s)
        --- PASS: TestPinIndexes/putUpload/pin_Index_count (0.00s)
        --- PASS: TestPinIndexes/putUpload/gc_index_count (0.00s)
        --- PASS: TestPinIndexes/putUpload/gc_size (0.00s)
    --- PASS: TestPinIndexes/setSync (0.00s)
        --- PASS: TestPinIndexes/setSync/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexes/setSync/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexes/setSync/push_Index_count (0.00s)
        --- PASS: TestPinIndexes/setSync/pull_Index_count (0.00s)
        --- PASS: TestPinIndexes/setSync/pin_Index_count (0.00s)
        --- PASS: TestPinIndexes/setSync/gc_index_count (0.00s)
        --- PASS: TestPinIndexes/setSync/gc_size (0.00s)
    --- PASS: TestPinIndexes/setPin (0.00s)
        --- PASS: TestPinIndexes/setPin/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexes/setPin/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexes/setPin/push_Index_count (0.00s)
        --- PASS: TestPinIndexes/setPin/pull_Index_count (0.00s)
        --- PASS: TestPinIndexes/setPin/pin_Index_count (0.00s)
        --- PASS: TestPinIndexes/setPin/gc_index_count (0.00s)
        --- PASS: TestPinIndexes/setPin/gc_size (0.00s)
    --- PASS: TestPinIndexes/setPin_2 (0.00s)
        --- PASS: TestPinIndexes/setPin_2/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexes/setPin_2/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexes/setPin_2/push_Index_count (0.00s)
        --- PASS: TestPinIndexes/setPin_2/pull_Index_count (0.00s)
        --- PASS: TestPinIndexes/setPin_2/pin_Index_count (0.00s)
        --- PASS: TestPinIndexes/setPin_2/gc_index_count (0.00s)
        --- PASS: TestPinIndexes/setPin_2/gc_size (0.00s)
    --- PASS: TestPinIndexes/setUnPin (0.00s)
        --- PASS: TestPinIndexes/setUnPin/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin/push_Index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin/pull_Index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin/pin_Index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin/gc_index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin/gc_size (0.00s)
    --- PASS: TestPinIndexes/setUnPin_2 (0.00s)
        --- PASS: TestPinIndexes/setUnPin_2/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin_2/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin_2/push_Index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin_2/pull_Index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin_2/pin_Index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin_2/gc_index_count (0.00s)
        --- PASS: TestPinIndexes/setUnPin_2/gc_size (0.00s)
=== RUN   TestPinIndexesSync
=== RUN   TestPinIndexesSync/putUpload
=== RUN   TestPinIndexesSync/putUpload/retrieval_data_Index_count
=== RUN   TestPinIndexesSync/putUpload/retrieval_access_Index_count
=== RUN   TestPinIndexesSync/putUpload/push_Index_count
=== RUN   TestPinIndexesSync/putUpload/pull_Index_count
=== RUN   TestPinIndexesSync/putUpload/pin_Index_count
=== RUN   TestPinIndexesSync/putUpload/gc_index_count
=== RUN   TestPinIndexesSync/putUpload/gc_size
=== RUN   TestPinIndexesSync/setPin
=== RUN   TestPinIndexesSync/setPin/retrieval_data_Index_count
=== RUN   TestPinIndexesSync/setPin/retrieval_access_Index_count
=== RUN   TestPinIndexesSync/setPin/push_Index_count
=== RUN   TestPinIndexesSync/setPin/pull_Index_count
=== RUN   TestPinIndexesSync/setPin/pin_Index_count
=== RUN   TestPinIndexesSync/setPin/gc_index_count
=== RUN   TestPinIndexesSync/setPin/gc_size
=== RUN   TestPinIndexesSync/setPin_2
=== RUN   TestPinIndexesSync/setPin_2/retrieval_data_Index_count
=== RUN   TestPinIndexesSync/setPin_2/retrieval_access_Index_count
=== RUN   TestPinIndexesSync/setPin_2/push_Index_count
=== RUN   TestPinIndexesSync/setPin_2/pull_Index_count
=== RUN   TestPinIndexesSync/setPin_2/pin_Index_count
=== RUN   TestPinIndexesSync/setPin_2/gc_index_count
=== RUN   TestPinIndexesSync/setPin_2/gc_size
=== RUN   TestPinIndexesSync/setUnPin
=== RUN   TestPinIndexesSync/setUnPin/retrieval_data_Index_count
=== RUN   TestPinIndexesSync/setUnPin/retrieval_access_Index_count
=== RUN   TestPinIndexesSync/setUnPin/push_Index_count
=== RUN   TestPinIndexesSync/setUnPin/pull_Index_count
=== RUN   TestPinIndexesSync/setUnPin/pin_Index_count
=== RUN   TestPinIndexesSync/setUnPin/gc_index_count
=== RUN   TestPinIndexesSync/setUnPin/gc_size
=== RUN   TestPinIndexesSync/setUnPin_2
=== RUN   TestPinIndexesSync/setUnPin_2/retrieval_data_Index_count
=== RUN   TestPinIndexesSync/setUnPin_2/retrieval_access_Index_count
=== RUN   TestPinIndexesSync/setUnPin_2/push_Index_count
=== RUN   TestPinIndexesSync/setUnPin_2/pull_Index_count
=== RUN   TestPinIndexesSync/setUnPin_2/pin_Index_count
=== RUN   TestPinIndexesSync/setUnPin_2/gc_index_count
=== RUN   TestPinIndexesSync/setUnPin_2/gc_size
=== RUN   TestPinIndexesSync/setPin_3
=== RUN   TestPinIndexesSync/setPin_3/retrieval_data_Index_count
=== RUN   TestPinIndexesSync/setPin_3/retrieval_access_Index_count
=== RUN   TestPinIndexesSync/setPin_3/push_Index_count
=== RUN   TestPinIndexesSync/setPin_3/pull_Index_count
=== RUN   TestPinIndexesSync/setPin_3/pin_Index_count
=== RUN   TestPinIndexesSync/setPin_3/gc_index_count
=== RUN   TestPinIndexesSync/setPin_3/gc_size
=== RUN   TestPinIndexesSync/setSync
=== RUN   TestPinIndexesSync/setSync/retrieval_data_Index_count
=== RUN   TestPinIndexesSync/setSync/retrieval_access_Index_count
=== RUN   TestPinIndexesSync/setSync/push_Index_count
=== RUN   TestPinIndexesSync/setSync/pull_Index_count
=== RUN   TestPinIndexesSync/setSync/pin_Index_count
=== RUN   TestPinIndexesSync/setSync/gc_index_count
=== RUN   TestPinIndexesSync/setSync/gc_size
=== RUN   TestPinIndexesSync/setUnPin#01
=== RUN   TestPinIndexesSync/setUnPin#01/retrieval_data_Index_count
=== RUN   TestPinIndexesSync/setUnPin#01/retrieval_access_Index_count
=== RUN   TestPinIndexesSync/setUnPin#01/push_Index_count
=== RUN   TestPinIndexesSync/setUnPin#01/pull_Index_count
=== RUN   TestPinIndexesSync/setUnPin#01/pin_Index_count
=== RUN   TestPinIndexesSync/setUnPin#01/gc_index_count
=== RUN   TestPinIndexesSync/setUnPin#01/gc_size
--- PASS: TestPinIndexesSync (0.00s)
    --- PASS: TestPinIndexesSync/putUpload (0.00s)
        --- PASS: TestPinIndexesSync/putUpload/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/putUpload/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/putUpload/push_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/putUpload/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/putUpload/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/putUpload/gc_index_count (0.00s)
        --- PASS: TestPinIndexesSync/putUpload/gc_size (0.00s)
    --- PASS: TestPinIndexesSync/setPin (0.00s)
        --- PASS: TestPinIndexesSync/setPin/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin/push_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin/gc_index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin/gc_size (0.00s)
    --- PASS: TestPinIndexesSync/setPin_2 (0.00s)
        --- PASS: TestPinIndexesSync/setPin_2/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_2/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_2/push_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_2/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_2/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_2/gc_index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_2/gc_size (0.00s)
    --- PASS: TestPinIndexesSync/setUnPin (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin/push_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin/gc_index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin/gc_size (0.00s)
    --- PASS: TestPinIndexesSync/setUnPin_2 (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin_2/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin_2/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin_2/push_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin_2/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin_2/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin_2/gc_index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin_2/gc_size (0.00s)
    --- PASS: TestPinIndexesSync/setPin_3 (0.00s)
        --- PASS: TestPinIndexesSync/setPin_3/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_3/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_3/push_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_3/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_3/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_3/gc_index_count (0.00s)
        --- PASS: TestPinIndexesSync/setPin_3/gc_size (0.00s)
    --- PASS: TestPinIndexesSync/setSync (0.00s)
        --- PASS: TestPinIndexesSync/setSync/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setSync/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setSync/push_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setSync/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setSync/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setSync/gc_index_count (0.00s)
        --- PASS: TestPinIndexesSync/setSync/gc_size (0.00s)
    --- PASS: TestPinIndexesSync/setUnPin#01 (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin#01/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin#01/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin#01/push_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin#01/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin#01/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin#01/gc_index_count (0.00s)
        --- PASS: TestPinIndexesSync/setUnPin#01/gc_size (0.00s)
=== RUN   TestPinIndexesPutSync
=== RUN   TestPinIndexesPutSync/putSync
=== RUN   TestPinIndexesPutSync/putSync/retrieval_data_Index_count
=== RUN   TestPinIndexesPutSync/putSync/retrieval_access_Index_count
=== RUN   TestPinIndexesPutSync/putSync/push_Index_count
=== RUN   TestPinIndexesPutSync/putSync/pull_Index_count
=== RUN   TestPinIndexesPutSync/putSync/pin_Index_count
=== RUN   TestPinIndexesPutSync/putSync/gc_index_count
=== RUN   TestPinIndexesPutSync/putSync/gc_size
=== RUN   TestPinIndexesPutSync/putSync#01
=== RUN   TestPinIndexesPutSync/putSync#01/retrieval_data_Index_count
=== RUN   TestPinIndexesPutSync/putSync#01/retrieval_access_Index_count
=== RUN   TestPinIndexesPutSync/putSync#01/push_Index_count
=== RUN   TestPinIndexesPutSync/putSync#01/pull_Index_count
=== RUN   TestPinIndexesPutSync/putSync#01/pin_Index_count
=== RUN   TestPinIndexesPutSync/putSync#01/gc_index_count
=== RUN   TestPinIndexesPutSync/putSync#01/gc_size
=== RUN   TestPinIndexesPutSync/setPin_2
=== RUN   TestPinIndexesPutSync/setPin_2/retrieval_data_Index_count
=== RUN   TestPinIndexesPutSync/setPin_2/retrieval_access_Index_count
=== RUN   TestPinIndexesPutSync/setPin_2/push_Index_count
=== RUN   TestPinIndexesPutSync/setPin_2/pull_Index_count
=== RUN   TestPinIndexesPutSync/setPin_2/pin_Index_count
=== RUN   TestPinIndexesPutSync/setPin_2/gc_index_count
=== RUN   TestPinIndexesPutSync/setPin_2/gc_size
=== RUN   TestPinIndexesPutSync/setUnPin
=== RUN   TestPinIndexesPutSync/setUnPin/retrieval_data_Index_count
=== RUN   TestPinIndexesPutSync/setUnPin/retrieval_access_Index_count
=== RUN   TestPinIndexesPutSync/setUnPin/push_Index_count
=== RUN   TestPinIndexesPutSync/setUnPin/pull_Index_count
=== RUN   TestPinIndexesPutSync/setUnPin/pin_Index_count
=== RUN   TestPinIndexesPutSync/setUnPin/gc_index_count
=== RUN   TestPinIndexesPutSync/setUnPin/gc_size
=== RUN   TestPinIndexesPutSync/setUnPin_2
=== RUN   TestPinIndexesPutSync/setUnPin_2/retrieval_data_Index_count
=== RUN   TestPinIndexesPutSync/setUnPin_2/retrieval_access_Index_count
=== RUN   TestPinIndexesPutSync/setUnPin_2/push_Index_count
=== RUN   TestPinIndexesPutSync/setUnPin_2/pull_Index_count
=== RUN   TestPinIndexesPutSync/setUnPin_2/pin_Index_count
=== RUN   TestPinIndexesPutSync/setUnPin_2/gc_index_count
=== RUN   TestPinIndexesPutSync/setUnPin_2/gc_size
--- PASS: TestPinIndexesPutSync (0.00s)
    --- PASS: TestPinIndexesPutSync/putSync (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync/gc_size (0.00s)
    --- PASS: TestPinIndexesPutSync/putSync#01 (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync#01/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync#01/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync#01/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync#01/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync#01/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync#01/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/putSync#01/gc_size (0.00s)
    --- PASS: TestPinIndexesPutSync/setPin_2 (0.00s)
        --- PASS: TestPinIndexesPutSync/setPin_2/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setPin_2/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setPin_2/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setPin_2/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setPin_2/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setPin_2/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setPin_2/gc_size (0.00s)
    --- PASS: TestPinIndexesPutSync/setUnPin (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin/gc_size (0.00s)
    --- PASS: TestPinIndexesPutSync/setUnPin_2 (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin_2/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin_2/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin_2/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin_2/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin_2/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin_2/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutSync/setUnPin_2/gc_size (0.00s)
=== RUN   TestPinIndexesPutSyncOutOfDepth
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync/retrieval_data_Index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync/retrieval_access_Index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync/push_Index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync/pull_Index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync/pin_Index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync/gc_index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync/gc_size
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync#01
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync#01/retrieval_data_Index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync#01/retrieval_access_Index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync#01/push_Index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync#01/pull_Index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync#01/pin_Index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync#01/gc_index_count
=== RUN   TestPinIndexesPutSyncOutOfDepth/putSync#01/gc_size
--- PASS: TestPinIndexesPutSyncOutOfDepth (0.00s)
    --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync/gc_size (0.00s)
    --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync#01 (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync#01/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync#01/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync#01/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync#01/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync#01/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync#01/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutSyncOutOfDepth/putSync#01/gc_size (0.00s)
=== RUN   TestPinIndexesPutRequest
=== RUN   TestPinIndexesPutRequest/putRequest
=== RUN   TestPinIndexesPutRequest/putRequest/retrieval_data_Index_count
=== RUN   TestPinIndexesPutRequest/putRequest/retrieval_access_Index_count
=== RUN   TestPinIndexesPutRequest/putRequest/push_Index_count
=== RUN   TestPinIndexesPutRequest/putRequest/pull_Index_count
=== RUN   TestPinIndexesPutRequest/putRequest/pin_Index_count
=== RUN   TestPinIndexesPutRequest/putRequest/gc_index_count
=== RUN   TestPinIndexesPutRequest/putRequest/gc_size
=== RUN   TestPinIndexesPutRequest/putRequest#01
=== RUN   TestPinIndexesPutRequest/putRequest#01/retrieval_data_Index_count
=== RUN   TestPinIndexesPutRequest/putRequest#01/retrieval_access_Index_count
=== RUN   TestPinIndexesPutRequest/putRequest#01/push_Index_count
=== RUN   TestPinIndexesPutRequest/putRequest#01/pull_Index_count
=== RUN   TestPinIndexesPutRequest/putRequest#01/pin_Index_count
=== RUN   TestPinIndexesPutRequest/putRequest#01/gc_index_count
=== RUN   TestPinIndexesPutRequest/putRequest#01/gc_size
=== RUN   TestPinIndexesPutRequest/setPin_2
=== RUN   TestPinIndexesPutRequest/setPin_2/retrieval_data_Index_count
=== RUN   TestPinIndexesPutRequest/setPin_2/retrieval_access_Index_count
=== RUN   TestPinIndexesPutRequest/setPin_2/push_Index_count
=== RUN   TestPinIndexesPutRequest/setPin_2/pull_Index_count
=== RUN   TestPinIndexesPutRequest/setPin_2/pin_Index_count
=== RUN   TestPinIndexesPutRequest/setPin_2/gc_index_count
=== RUN   TestPinIndexesPutRequest/setPin_2/gc_size
=== RUN   TestPinIndexesPutRequest/setUnPin
=== RUN   TestPinIndexesPutRequest/setUnPin/retrieval_data_Index_count
=== RUN   TestPinIndexesPutRequest/setUnPin/retrieval_access_Index_count
=== RUN   TestPinIndexesPutRequest/setUnPin/push_Index_count
=== RUN   TestPinIndexesPutRequest/setUnPin/pull_Index_count
=== RUN   TestPinIndexesPutRequest/setUnPin/pin_Index_count
=== RUN   TestPinIndexesPutRequest/setUnPin/gc_index_count
=== RUN   TestPinIndexesPutRequest/setUnPin/gc_size
=== RUN   TestPinIndexesPutRequest/setUnPin_2
=== RUN   TestPinIndexesPutRequest/setUnPin_2/retrieval_data_Index_count
=== RUN   TestPinIndexesPutRequest/setUnPin_2/retrieval_access_Index_count
=== RUN   TestPinIndexesPutRequest/setUnPin_2/push_Index_count
=== RUN   TestPinIndexesPutRequest/setUnPin_2/pull_Index_count
=== RUN   TestPinIndexesPutRequest/setUnPin_2/pin_Index_count
=== RUN   TestPinIndexesPutRequest/setUnPin_2/gc_index_count
=== RUN   TestPinIndexesPutRequest/setUnPin_2/gc_size
--- PASS: TestPinIndexesPutRequest (0.00s)
    --- PASS: TestPinIndexesPutRequest/putRequest (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest/gc_size (0.00s)
    --- PASS: TestPinIndexesPutRequest/putRequest#01 (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest#01/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest#01/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest#01/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest#01/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest#01/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest#01/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/putRequest#01/gc_size (0.00s)
    --- PASS: TestPinIndexesPutRequest/setPin_2 (0.00s)
        --- PASS: TestPinIndexesPutRequest/setPin_2/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setPin_2/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setPin_2/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setPin_2/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setPin_2/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setPin_2/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setPin_2/gc_size (0.00s)
    --- PASS: TestPinIndexesPutRequest/setUnPin (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin/gc_size (0.00s)
    --- PASS: TestPinIndexesPutRequest/setUnPin_2 (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin_2/retrieval_data_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin_2/retrieval_access_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin_2/push_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin_2/pull_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin_2/pin_Index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin_2/gc_index_count (0.00s)
        --- PASS: TestPinIndexesPutRequest/setUnPin_2/gc_size (0.00s)
=== RUN   TestDB_ReserveGC_AllOutOfRadius
=== RUN   TestDB_ReserveGC_AllOutOfRadius/pull_index_count
=== RUN   TestDB_ReserveGC_AllOutOfRadius/postage_chunks_index_count
=== RUN   TestDB_ReserveGC_AllOutOfRadius/postage_radius_index_count
=== RUN   TestDB_ReserveGC_AllOutOfRadius/gc_index_count
=== RUN   TestDB_ReserveGC_AllOutOfRadius/gc_size
=== RUN   TestDB_ReserveGC_AllOutOfRadius/reserve_size
=== RUN   TestDB_ReserveGC_AllOutOfRadius/get_the_first_synced_chunk
=== RUN   TestDB_ReserveGC_AllOutOfRadius/only_first_inserted_chunks_should_be_removed
=== RUN   TestDB_ReserveGC_AllOutOfRadius/get_most_recent_synced_chunk
--- PASS: TestDB_ReserveGC_AllOutOfRadius (0.01s)
    --- PASS: TestDB_ReserveGC_AllOutOfRadius/pull_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_AllOutOfRadius/postage_chunks_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_AllOutOfRadius/postage_radius_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_AllOutOfRadius/gc_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_AllOutOfRadius/gc_size (0.00s)
    --- PASS: TestDB_ReserveGC_AllOutOfRadius/reserve_size (0.00s)
    --- PASS: TestDB_ReserveGC_AllOutOfRadius/get_the_first_synced_chunk (0.00s)
    --- PASS: TestDB_ReserveGC_AllOutOfRadius/only_first_inserted_chunks_should_be_removed (0.00s)
    --- PASS: TestDB_ReserveGC_AllOutOfRadius/get_most_recent_synced_chunk (0.00s)
=== RUN   TestDB_ReserveGC_AllWithinRadius
=== RUN   TestDB_ReserveGC_AllWithinRadius/pull_index_count
=== RUN   TestDB_ReserveGC_AllWithinRadius/postage_chunks_index_count
=== RUN   TestDB_ReserveGC_AllWithinRadius/postage_radius_index_count
=== RUN   TestDB_ReserveGC_AllWithinRadius/gc_index_count
=== RUN   TestDB_ReserveGC_AllWithinRadius/gc_size
=== RUN   TestDB_ReserveGC_AllWithinRadius/reserve_size
=== RUN   TestDB_ReserveGC_AllWithinRadius/all_chunks_should_be_accessible
--- PASS: TestDB_ReserveGC_AllWithinRadius (1.01s)
    --- PASS: TestDB_ReserveGC_AllWithinRadius/pull_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_AllWithinRadius/postage_chunks_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_AllWithinRadius/postage_radius_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_AllWithinRadius/gc_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_AllWithinRadius/gc_size (0.00s)
    --- PASS: TestDB_ReserveGC_AllWithinRadius/reserve_size (0.00s)
    --- PASS: TestDB_ReserveGC_AllWithinRadius/all_chunks_should_be_accessible (0.00s)
=== RUN   TestDB_ReserveGC_Unreserve
=== RUN   TestDB_ReserveGC_Unreserve/reserve_size
=== RUN   TestDB_ReserveGC_Unreserve/reserve_size#01
=== RUN   TestDB_ReserveGC_Unreserve/pull_index_count
=== RUN   TestDB_ReserveGC_Unreserve/postage_chunks_index_count
=== RUN   TestDB_ReserveGC_Unreserve/postage_radius_index_count
=== RUN   TestDB_ReserveGC_Unreserve/gc_index_count
=== RUN   TestDB_ReserveGC_Unreserve/gc_size
=== RUN   TestDB_ReserveGC_Unreserve/reserve_size#02
=== RUN   TestDB_ReserveGC_Unreserve/first_ten_unreserved_chunks_should_not_be_accessible
=== RUN   TestDB_ReserveGC_Unreserve/the_rest_should_be_accessible
--- PASS: TestDB_ReserveGC_Unreserve (0.02s)
    --- PASS: TestDB_ReserveGC_Unreserve/reserve_size (0.00s)
    --- PASS: TestDB_ReserveGC_Unreserve/reserve_size#01 (0.00s)
    --- PASS: TestDB_ReserveGC_Unreserve/pull_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_Unreserve/postage_chunks_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_Unreserve/postage_radius_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_Unreserve/gc_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_Unreserve/gc_size (0.00s)
    --- PASS: TestDB_ReserveGC_Unreserve/reserve_size#02 (0.00s)
    --- PASS: TestDB_ReserveGC_Unreserve/first_ten_unreserved_chunks_should_not_be_accessible (0.00s)
    --- PASS: TestDB_ReserveGC_Unreserve/the_rest_should_be_accessible (0.00s)
=== RUN   TestDB_ReserveGC_EvictMaxPO
=== RUN   TestDB_ReserveGC_EvictMaxPO/reserve_size
=== RUN   TestDB_ReserveGC_EvictMaxPO/postage_radius_index_count
=== RUN   TestDB_ReserveGC_EvictMaxPO/reserve_size#01
=== RUN   TestDB_ReserveGC_EvictMaxPO/pull_index_count
=== RUN   TestDB_ReserveGC_EvictMaxPO/postage_chunks_index_count
=== RUN   TestDB_ReserveGC_EvictMaxPO/postage_radius_index_count#01
=== RUN   TestDB_ReserveGC_EvictMaxPO/gc_index_count
=== RUN   TestDB_ReserveGC_EvictMaxPO/gc_size
=== RUN   TestDB_ReserveGC_EvictMaxPO/reserve_size#02
=== RUN   TestDB_ReserveGC_EvictMaxPO/first_ten_unreserved_chunks_should_not_be_accessible
=== RUN   TestDB_ReserveGC_EvictMaxPO/the_rest_should_be_accessible
--- PASS: TestDB_ReserveGC_EvictMaxPO (0.04s)
    --- PASS: TestDB_ReserveGC_EvictMaxPO/reserve_size (0.00s)
    --- PASS: TestDB_ReserveGC_EvictMaxPO/postage_radius_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_EvictMaxPO/reserve_size#01 (0.00s)
    --- PASS: TestDB_ReserveGC_EvictMaxPO/pull_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_EvictMaxPO/postage_chunks_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_EvictMaxPO/postage_radius_index_count#01 (0.00s)
    --- PASS: TestDB_ReserveGC_EvictMaxPO/gc_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_EvictMaxPO/gc_size (0.00s)
    --- PASS: TestDB_ReserveGC_EvictMaxPO/reserve_size#02 (0.00s)
    --- PASS: TestDB_ReserveGC_EvictMaxPO/first_ten_unreserved_chunks_should_not_be_accessible (0.00s)
    --- PASS: TestDB_ReserveGC_EvictMaxPO/the_rest_should_be_accessible (0.00s)
=== RUN   TestReserveSize
=== RUN   TestReserveSize/variadic_put_sync
=== RUN   TestReserveSize/variadic_put_sync/reserve_size
=== RUN   TestReserveSize/variadic_put_upload_then_set_sync
=== RUN   TestReserveSize/variadic_put_upload_then_set_sync/reserve_size
=== RUN   TestReserveSize/variadic_put_upload_then_set_sync/reserve_size#01
=== RUN   TestReserveSize/sequencial_put_sync
=== RUN   TestReserveSize/sequencial_put_sync/reserve_size
=== RUN   TestReserveSize/sequencial_put_request
=== RUN   TestReserveSize/sequencial_put_request/reserve_size
--- PASS: TestReserveSize (0.02s)
    --- PASS: TestReserveSize/variadic_put_sync (0.00s)
        --- PASS: TestReserveSize/variadic_put_sync/reserve_size (0.00s)
    --- PASS: TestReserveSize/variadic_put_upload_then_set_sync (0.00s)
        --- PASS: TestReserveSize/variadic_put_upload_then_set_sync/reserve_size (0.00s)
        --- PASS: TestReserveSize/variadic_put_upload_then_set_sync/reserve_size#01 (0.00s)
    --- PASS: TestReserveSize/sequencial_put_sync (0.01s)
        --- PASS: TestReserveSize/sequencial_put_sync/reserve_size (0.00s)
    --- PASS: TestReserveSize/sequencial_put_request (0.00s)
        --- PASS: TestReserveSize/sequencial_put_request/reserve_size (0.00s)
=== RUN   TestComputeReserveSize
=== RUN   TestComputeReserveSize/reserve_size
--- PASS: TestComputeReserveSize (0.01s)
    --- PASS: TestComputeReserveSize/reserve_size (0.00s)
=== RUN   TestDB_ReserveGC_BatchedUnreserve
=== RUN   TestDB_ReserveGC_BatchedUnreserve/reserve_size
=== RUN   TestDB_ReserveGC_BatchedUnreserve/reserve_size#01
=== RUN   TestDB_ReserveGC_BatchedUnreserve/pull_index_count
=== RUN   TestDB_ReserveGC_BatchedUnreserve/postage_chunks_index_count
=== RUN   TestDB_ReserveGC_BatchedUnreserve/postage_radius_index_count
=== RUN   TestDB_ReserveGC_BatchedUnreserve/gc_index_count
=== RUN   TestDB_ReserveGC_BatchedUnreserve/gc_size
--- PASS: TestDB_ReserveGC_BatchedUnreserve (0.01s)
    --- PASS: TestDB_ReserveGC_BatchedUnreserve/reserve_size (0.00s)
    --- PASS: TestDB_ReserveGC_BatchedUnreserve/reserve_size#01 (0.00s)
    --- PASS: TestDB_ReserveGC_BatchedUnreserve/pull_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_BatchedUnreserve/postage_chunks_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_BatchedUnreserve/postage_radius_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_BatchedUnreserve/gc_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_BatchedUnreserve/gc_size (0.00s)
=== RUN   TestDB_ReserveGC_EvictBatch
=== RUN   TestDB_ReserveGC_EvictBatch/reserve_size
=== RUN   TestDB_ReserveGC_EvictBatch/reserve_size#01
=== RUN   TestDB_ReserveGC_EvictBatch/postage_chunks_index_count
=== RUN   TestDB_ReserveGC_EvictBatch/postage_radius_index_count
=== RUN   TestDB_ReserveGC_EvictBatch/gc_index_count
=== RUN   TestDB_ReserveGC_EvictBatch/gc_size
--- PASS: TestDB_ReserveGC_EvictBatch (0.01s)
    --- PASS: TestDB_ReserveGC_EvictBatch/reserve_size (0.00s)
    --- PASS: TestDB_ReserveGC_EvictBatch/reserve_size#01 (0.00s)
    --- PASS: TestDB_ReserveGC_EvictBatch/postage_chunks_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_EvictBatch/postage_radius_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_EvictBatch/gc_index_count (0.00s)
    --- PASS: TestDB_ReserveGC_EvictBatch/gc_size (0.00s)
=== RUN   TestReserveSampler
=== RUN   TestReserveSampler/reserve_size
=== RUN   TestReserveSampler/reserve_sample_1
=== RUN   TestReserveSampler/reserve_size#01
=== RUN   TestReserveSampler/reserve_sample_2
--- PASS: TestReserveSampler (0.02s)
    --- PASS: TestReserveSampler/reserve_size (0.00s)
    --- PASS: TestReserveSampler/reserve_sample_1 (0.00s)
    --- PASS: TestReserveSampler/reserve_size#01 (0.00s)
    --- PASS: TestReserveSampler/reserve_sample_2 (0.00s)
=== RUN   TestReserveSamplerStop_FLAKY
--- PASS: TestReserveSamplerStop_FLAKY (0.02s)
=== RUN   TestDB_SubscribePull_first
--- PASS: TestDB_SubscribePull_first (0.29s)
=== RUN   TestDB_SubscribePull
--- PASS: TestDB_SubscribePull (0.36s)
=== RUN   TestDB_SubscribePull_multiple
--- PASS: TestDB_SubscribePull_multiple (0.36s)
=== RUN   TestDB_SubscribePull_since
--- PASS: TestDB_SubscribePull_since (0.16s)
=== RUN   TestDB_SubscribePull_until
--- PASS: TestDB_SubscribePull_until (0.16s)
=== RUN   TestDB_SubscribePull_sinceAndUntil
--- PASS: TestDB_SubscribePull_sinceAndUntil (0.19s)
=== RUN   TestDB_SubscribePull_rangeOnRemovedChunks
--- PASS: TestDB_SubscribePull_rangeOnRemovedChunks (1.54s)
=== RUN   TestDB_LastPullSubscriptionBinID
--- PASS: TestDB_LastPullSubscriptionBinID (0.04s)
=== RUN   TestAddressInBin
--- PASS: TestAddressInBin (0.00s)
=== RUN   TestDB_SubscribePush
--- PASS: TestDB_SubscribePush (0.21s)
=== RUN   TestDB_SubscribePush_multiple
--- PASS: TestDB_SubscribePush_multiple (0.21s)
=== RUN   TestDB_SubscribePush_iterator_restart
--- PASS: TestDB_SubscribePush_iterator_restart (3.01s)
PASS
ok  	github.com/ethersphere/bee/pkg/localstore	(cached)
?   	github.com/ethersphere/bee/pkg/localstorev2/internal	[no test files]
?   	github.com/ethersphere/bee/pkg/log/httpaccess	[no test files]
?   	github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/pb	[no test files]
?   	github.com/ethersphere/bee/pkg/p2p/libp2p/internal/headers/pb	[no test files]
?   	github.com/ethersphere/bee/pkg/p2p/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/p2p/protobuf/internal/pb	[no test files]
?   	github.com/ethersphere/bee/pkg/pingpong/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/pingpong/pb	[no test files]
?   	github.com/ethersphere/bee/pkg/pinning/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/postage/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/postage/postagecontract/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/pricer	[no test files]
?   	github.com/ethersphere/bee/pkg/postage/testing	[no test files]
?   	github.com/ethersphere/bee/pkg/pricer/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/pricing/pb	[no test files]
?   	github.com/ethersphere/bee/pkg/pullsync/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/pullsync/pb	[no test files]
?   	github.com/ethersphere/bee/pkg/pullsync/pullstorage/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/pusher/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/pushsync/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/pushsync/pb	[no test files]
?   	github.com/ethersphere/bee/pkg/resolver	[no test files]
?   	github.com/ethersphere/bee/pkg/resolver/client	[no test files]
?   	github.com/ethersphere/bee/pkg/resolver/client/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/resolver/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/retrieval/pb	[no test files]
?   	github.com/ethersphere/bee/pkg/settlement	[no test files]
?   	github.com/ethersphere/bee/pkg/sctx	[no test files]
?   	github.com/ethersphere/bee/pkg/settlement/pseudosettle/pb	[no test files]
?   	github.com/ethersphere/bee/pkg/settlement/swap/chequebook/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/settlement/swap/chequestore/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/settlement/swap/erc20/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/settlement/swap/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/settlement/swap/priceoracle/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/settlement/swap/swapprotocol/pb	[no test files]
?   	github.com/ethersphere/bee/pkg/soc/testing	[no test files]
?   	github.com/ethersphere/bee/pkg/statestore	[no test files]
?   	github.com/ethersphere/bee/pkg/statestore/test	[no test files]
?   	github.com/ethersphere/bee/pkg/steward/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/storage	[no test files]
?   	github.com/ethersphere/bee/pkg/storage/testing	[no test files]
?   	github.com/ethersphere/bee/pkg/storageincentives/staking/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/tags/testing	[no test files]
?   	github.com/ethersphere/bee/pkg/topology	[no test files]
?   	github.com/ethersphere/bee/pkg/topology/kademlia/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/topology/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/transaction/backendmock	[no test files]
?   	github.com/ethersphere/bee/pkg/transaction/backendsimulation	[no test files]
?   	github.com/ethersphere/bee/pkg/transaction/monitormock	[no test files]
?   	github.com/ethersphere/bee/pkg/transaction/mock	[no test files]
?   	github.com/ethersphere/bee/pkg/transaction/wrapped	[no test files]
?   	github.com/ethersphere/bee/pkg/util	[no test files]
=== RUN   TestCacheStore
=== PAUSE TestCacheStore
=== RUN   TestNetStore
=== PAUSE TestNetStore
=== RUN   TestPinStore
=== PAUSE TestPinStore
=== RUN   TestIndexCollision
=== PAUSE TestIndexCollision
=== RUN   TestEvictBatch
=== PAUSE TestEvictBatch
=== RUN   TestUnreserveCap
=== PAUSE TestUnreserveCap
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
=== RUN   TestReserveSampler/mem/reserve_size
=== RUN   TestReserveSampler/mem/reserve_sample_1
=== RUN   TestReserveSampler/disk/reserve_size
=== RUN   TestReserveSampler/disk/reserve_sample_1
=== RUN   TestReserveSampler/mem/reserve_size#01
=== RUN   TestReserveSampler/mem/reserve_sample_2
=== RUN   TestReserveSampler/disk/reserve_size#01
=== RUN   TestReserveSampler/disk/reserve_sample_2
--- PASS: TestReserveSampler (0.00s)
    --- PASS: TestReserveSampler/mem (3.54s)
        --- PASS: TestReserveSampler/mem/reserve_size (0.00s)
        --- PASS: TestReserveSampler/mem/reserve_sample_1 (0.00s)
        --- PASS: TestReserveSampler/mem/reserve_size#01 (0.00s)
        --- PASS: TestReserveSampler/mem/reserve_sample_2 (0.00s)
    --- PASS: TestReserveSampler/disk (3.54s)
        --- PASS: TestReserveSampler/disk/reserve_size (0.00s)
        --- PASS: TestReserveSampler/disk/reserve_sample_1 (0.00s)
        --- PASS: TestReserveSampler/disk/reserve_size#01 (0.00s)
        --- PASS: TestReserveSampler/disk/reserve_sample_2 (0.00s)
=== RUN   TestNew
=== PAUSE TestNew
=== RUN   TestUploadStore
=== PAUSE TestUploadStore
=== CONT  TestCacheStore
=== RUN   TestCacheStore/inmem
=== CONT  TestNew
=== PAUSE TestCacheStore/inmem
=== CONT  TestEvictBatch
=== RUN   TestCacheStore/disk
=== PAUSE TestCacheStore/disk
=== RUN   TestNew/inmem_with_options
=== PAUSE TestNew/inmem_with_options
=== RUN   TestNew/disk_default_options
=== PAUSE TestNew/disk_default_options
=== RUN   TestNew/disk_with_options
=== PAUSE TestNew/disk_with_options
=== CONT  TestSubscribeBinTrigger
=== RUN   TestNew/migration_on_latest_version
=== PAUSE TestNew/migration_on_latest_version
=== RUN   TestSubscribeBinTrigger/disk
=== CONT  TestNetStore
=== PAUSE TestSubscribeBinTrigger/disk
=== RUN   TestSubscribeBinTrigger/mem
=== PAUSE TestSubscribeBinTrigger/mem
=== CONT  TestRadiusManager
=== RUN   TestRadiusManager/old_nodes_starts_at_previous_radius
=== PAUSE TestRadiusManager/old_nodes_starts_at_previous_radius
=== RUN   TestRadiusManager/radius_decrease_due_to_under_utilization
=== PAUSE TestRadiusManager/radius_decrease_due_to_under_utilization
=== RUN   TestRadiusManager/radius_doesnt_change_due_to_non-zero_pull_rate
=== PAUSE TestRadiusManager/radius_doesnt_change_due_to_non-zero_pull_rate
=== CONT  TestPinStore
=== CONT  TestCacheStore/inmem
=== RUN   TestNetStore/inmem
=== PAUSE TestNetStore/inmem
=== RUN   TestNetStore/disk
=== PAUSE TestNetStore/disk
=== RUN   TestPinStore/inmem
=== PAUSE TestPinStore/inmem
=== CONT  TestCacheStore/disk
=== CONT  TestSubscribeBin
=== RUN   TestSubscribeBin/disk
=== PAUSE TestSubscribeBin/disk
=== RUN   TestSubscribeBin/mem
=== PAUSE TestSubscribeBin/mem
=== RUN   TestPinStore/disk
=== PAUSE TestPinStore/disk
=== CONT  TestIndexCollision
=== RUN   TestIndexCollision/disk
=== PAUSE TestIndexCollision/disk
=== RUN   TestIndexCollision/mem
=== CONT  TestUnreserveCap
=== RUN   TestUnreserveCap/disk
=== PAUSE TestUnreserveCap/disk
=== RUN   TestUnreserveCap/mem
=== PAUSE TestUnreserveCap/mem
=== CONT  TestNew/inmem_with_options
=== CONT  TestNew/disk_with_options
=== CONT  TestUploadStore
=== RUN   TestUploadStore/inmem
=== PAUSE TestUploadStore/inmem
=== RUN   TestUploadStore/disk
=== PAUSE TestUploadStore/disk
=== CONT  TestNew/migration_on_latest_version
=== RUN   TestNew/migration_on_latest_version/inmem
=== PAUSE TestNew/migration_on_latest_version/inmem
=== RUN   TestNew/migration_on_latest_version/disk
=== PAUSE TestNew/migration_on_latest_version/disk
=== CONT  TestNew/disk_default_options
=== PAUSE TestIndexCollision/mem
=== CONT  TestRadiusManager/old_nodes_starts_at_previous_radius
=== RUN   TestCacheStore/inmem/cache_chunks
=== CONT  TestSubscribeBinTrigger/disk
=== RUN   TestCacheStore/inmem/cache_chunks/commit
=== CONT  TestSubscribeBinTrigger/mem
=== RUN   TestCacheStore/inmem/cache_chunks/rollback
=== RUN   TestCacheStore/inmem/lookup
=== RUN   TestCacheStore/inmem/lookup/commit
=== RUN   TestCacheStore/inmem/lookup/rollback
=== RUN   TestCacheStore/inmem/cache_chunks_beyond_capacity
=== CONT  TestRadiusManager/radius_decrease_due_to_under_utilization
=== RUN   TestCacheStore/disk/cache_chunks
=== RUN   TestCacheStore/disk/cache_chunks/commit
=== CONT  TestRadiusManager/radius_doesnt_change_due_to_non-zero_pull_rate
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
=== CONT  TestSubscribeBin/disk
=== CONT  TestPinStore/inmem
=== RUN   TestPinStore/inmem/pin_10_chunks
=== RUN   TestPinStore/inmem/pin_20_chunks_rollback
=== RUN   TestPinStore/inmem/pin_30_chunks
=== RUN   TestPinStore/inmem/has_f7b6efec160030e4a4effb912f23a3cbf69099061d06393b7efb6c5ebf1bc54e
=== RUN   TestPinStore/inmem/has_f8ba269eaca11c9e1966e96cd58f7d829c2a0ce0cd9409302fca56e5dcc51243
=== RUN   TestPinStore/inmem/has_c7ed60fa223c52905dd79c612de5049b324b1275143a914f29f557657d14f62f
=== RUN   TestPinStore/inmem/pins
=== RUN   TestPinStore/inmem/delete_pin
=== RUN   TestPinStore/inmem/delete_pin/commit
=== RUN   TestPinStore/inmem/delete_pin/rollback
=== CONT  TestSubscribeBin/mem
=== RUN   TestCacheStore/disk/cache_chunks/rollback
=== RUN   TestCacheStore/disk/lookup
=== RUN   TestCacheStore/disk/lookup/commit
=== RUN   TestCacheStore/disk/lookup/rollback
=== RUN   TestCacheStore/disk/cache_chunks_beyond_capacity
--- PASS: TestCacheStore (0.00s)
    --- PASS: TestCacheStore/inmem (0.00s)
        --- PASS: TestCacheStore/inmem/cache_chunks (0.00s)
            --- PASS: TestCacheStore/inmem/cache_chunks/commit (0.00s)
            --- PASS: TestCacheStore/inmem/cache_chunks/rollback (0.00s)
        --- PASS: TestCacheStore/inmem/lookup (0.00s)
            --- PASS: TestCacheStore/inmem/lookup/commit (0.00s)
            --- PASS: TestCacheStore/inmem/lookup/rollback (0.00s)
        --- PASS: TestCacheStore/inmem/cache_chunks_beyond_capacity (0.00s)
    --- PASS: TestCacheStore/disk (0.35s)
        --- PASS: TestCacheStore/disk/cache_chunks (0.09s)
            --- PASS: TestCacheStore/disk/cache_chunks/commit (0.08s)
            --- PASS: TestCacheStore/disk/cache_chunks/rollback (0.01s)
        --- PASS: TestCacheStore/disk/lookup (0.11s)
            --- PASS: TestCacheStore/disk/lookup/commit (0.06s)
            --- PASS: TestCacheStore/disk/lookup/rollback (0.05s)
        --- PASS: TestCacheStore/disk/cache_chunks_beyond_capacity (0.12s)
=== CONT  TestPinStore/disk
=== RUN   TestPinStore/disk/pin_10_chunks
=== RUN   TestPinStore/disk/pin_20_chunks_rollback
=== RUN   TestPinStore/disk/pin_30_chunks
--- PASS: TestEvictBatch (0.51s)
=== CONT  TestUnreserveCap/disk
=== RUN   TestPinStore/disk/has_cce7f1d8d185601ec476ffaaa8d21df114c4cddf47873ef498452172d8ead3df
=== RUN   TestPinStore/disk/has_8b874a492459cc5eef84a6805848843eaa8ce59a1e3004124354e9e95aeb6381
=== RUN   TestPinStore/disk/has_b48a8137abdbf895325dd7af29743a8d6c39ec4f25925690e5e372d51b1df094
=== RUN   TestPinStore/disk/pins
=== RUN   TestPinStore/disk/delete_pin
=== RUN   TestPinStore/disk/delete_pin/commit
=== CONT  TestUnreserveCap/mem
=== CONT  TestUploadStore/inmem
=== RUN   TestUploadStore/inmem/new_session
=== PAUSE TestUploadStore/inmem/new_session
=== RUN   TestUploadStore/inmem/no_tag
=== PAUSE TestUploadStore/inmem/no_tag
--- PASS: TestSubscribeBinTrigger (0.00s)
    --- PASS: TestSubscribeBinTrigger/mem (0.68s)
    --- PASS: TestSubscribeBinTrigger/disk (0.68s)
=== RUN   TestUploadStore/inmem/upload_10_chunks
=== PAUSE TestUploadStore/inmem/upload_10_chunks
=== RUN   TestUploadStore/inmem/upload_20_chunks_rollback
=== PAUSE TestUploadStore/inmem/upload_20_chunks_rollback
=== RUN   TestUploadStore/inmem/upload_30_chunks
=== PAUSE TestUploadStore/inmem/upload_30_chunks
=== RUN   TestUploadStore/inmem/upload_10_chunks_with_pin
=== PAUSE TestUploadStore/inmem/upload_10_chunks_with_pin
=== RUN   TestUploadStore/inmem/upload_20_chunks_with_pin_rollback
=== PAUSE TestUploadStore/inmem/upload_20_chunks_with_pin_rollback
=== RUN   TestUploadStore/inmem/upload_30_chunks_with_pin
=== PAUSE TestUploadStore/inmem/upload_30_chunks_with_pin
=== RUN   TestUploadStore/inmem/get_session_info
=== PAUSE TestUploadStore/inmem/get_session_info
=== CONT  TestNew/migration_on_latest_version/inmem
=== CONT  TestIndexCollision/disk
=== CONT  TestUploadStore/disk
=== RUN   TestUploadStore/disk/new_session
=== PAUSE TestUploadStore/disk/new_session
=== RUN   TestUploadStore/disk/no_tag
=== PAUSE TestUploadStore/disk/no_tag
=== RUN   TestUploadStore/disk/upload_10_chunks
=== PAUSE TestUploadStore/disk/upload_10_chunks
=== RUN   TestUploadStore/disk/upload_20_chunks_rollback
=== PAUSE TestUploadStore/disk/upload_20_chunks_rollback
=== RUN   TestUploadStore/disk/upload_30_chunks
=== PAUSE TestUploadStore/disk/upload_30_chunks
=== RUN   TestUploadStore/disk/upload_10_chunks_with_pin
=== PAUSE TestUploadStore/disk/upload_10_chunks_with_pin
=== RUN   TestUploadStore/disk/upload_20_chunks_with_pin_rollback
=== PAUSE TestUploadStore/disk/upload_20_chunks_with_pin_rollback
=== RUN   TestUploadStore/disk/upload_30_chunks_with_pin
=== PAUSE TestUploadStore/disk/upload_30_chunks_with_pin
=== RUN   TestUploadStore/disk/get_session_info
=== PAUSE TestUploadStore/disk/get_session_info
=== CONT  TestIndexCollision/mem
=== RUN   TestPinStore/disk/delete_pin/rollback
=== CONT  TestNew/migration_on_latest_version/disk
--- PASS: TestIndexCollision (0.00s)
    --- PASS: TestIndexCollision/disk (0.05s)
    --- PASS: TestIndexCollision/mem (0.04s)
--- PASS: TestPinStore (0.00s)
    --- PASS: TestPinStore/inmem (0.00s)
        --- PASS: TestPinStore/inmem/pin_10_chunks (0.00s)
        --- PASS: TestPinStore/inmem/pin_20_chunks_rollback (0.00s)
        --- PASS: TestPinStore/inmem/pin_30_chunks (0.00s)
        --- PASS: TestPinStore/inmem/has_f7b6efec160030e4a4effb912f23a3cbf69099061d06393b7efb6c5ebf1bc54e (0.00s)
        --- PASS: TestPinStore/inmem/has_f8ba269eaca11c9e1966e96cd58f7d829c2a0ce0cd9409302fca56e5dcc51243 (0.00s)
        --- PASS: TestPinStore/inmem/has_c7ed60fa223c52905dd79c612de5049b324b1275143a914f29f557657d14f62f (0.00s)
        --- PASS: TestPinStore/inmem/pins (0.00s)
        --- PASS: TestPinStore/inmem/delete_pin (0.00s)
            --- PASS: TestPinStore/inmem/delete_pin/commit (0.00s)
            --- PASS: TestPinStore/inmem/delete_pin/rollback (0.00s)
    --- PASS: TestPinStore/disk (0.43s)
        --- PASS: TestPinStore/disk/pin_10_chunks (0.04s)
        --- PASS: TestPinStore/disk/pin_20_chunks_rollback (0.09s)
        --- PASS: TestPinStore/disk/pin_30_chunks (0.12s)
        --- PASS: TestPinStore/disk/has_cce7f1d8d185601ec476ffaaa8d21df114c4cddf47873ef498452172d8ead3df (0.00s)
        --- PASS: TestPinStore/disk/has_8b874a492459cc5eef84a6805848843eaa8ce59a1e3004124354e9e95aeb6381 (0.00s)
        --- PASS: TestPinStore/disk/has_b48a8137abdbf895325dd7af29743a8d6c39ec4f25925690e5e372d51b1df094 (0.00s)
        --- PASS: TestPinStore/disk/pins (0.00s)
        --- PASS: TestPinStore/disk/delete_pin (0.15s)
            --- PASS: TestPinStore/disk/delete_pin/commit (0.14s)
            --- PASS: TestPinStore/disk/delete_pin/rollback (0.02s)
=== CONT  TestNetStore/inmem/direct_upload
=== RUN   TestNetStore/inmem/direct_upload/commit
=== PAUSE TestNetStore/inmem/direct_upload/commit
=== RUN   TestNetStore/inmem/direct_upload/pusher_error
=== PAUSE TestNetStore/inmem/direct_upload/pusher_error
=== RUN   TestNetStore/inmem/direct_upload/context_cancellation
=== PAUSE TestNetStore/inmem/direct_upload/context_cancellation
=== CONT  TestNetStore/inmem/download
=== RUN   TestNetStore/inmem/download/with_cache
=== PAUSE TestNetStore/inmem/download/with_cache
=== RUN   TestNetStore/inmem/download/no_cache
=== PAUSE TestNetStore/inmem/download/no_cache
=== CONT  TestNetStore/disk/direct_upload
=== RUN   TestNetStore/disk/direct_upload/commit
=== PAUSE TestNetStore/disk/direct_upload/commit
=== RUN   TestNetStore/disk/direct_upload/pusher_error
=== PAUSE TestNetStore/disk/direct_upload/pusher_error
=== RUN   TestNetStore/disk/direct_upload/context_cancellation
=== PAUSE TestNetStore/disk/direct_upload/context_cancellation
=== CONT  TestNetStore/disk/download
=== RUN   TestNetStore/disk/download/with_cache
=== PAUSE TestNetStore/disk/download/with_cache
=== RUN   TestNetStore/disk/download/no_cache
=== PAUSE TestNetStore/disk/download/no_cache
=== CONT  TestUploadStore/inmem/new_session
=== CONT  TestUploadStore/inmem/upload_10_chunks_with_pin
=== CONT  TestUploadStore/inmem/get_session_info
=== RUN   TestUploadStore/inmem/get_session_info/done
=== RUN   TestUploadStore/inmem/get_session_info/cleanup
=== CONT  TestUploadStore/inmem/upload_30_chunks_with_pin
=== CONT  TestUploadStore/inmem/upload_20_chunks_with_pin_rollback
=== CONT  TestUploadStore/inmem/upload_20_chunks_rollback
=== CONT  TestUploadStore/inmem/upload_30_chunks
=== CONT  TestUploadStore/inmem/upload_10_chunks
=== CONT  TestUploadStore/inmem/no_tag
=== CONT  TestUploadStore/disk/new_session
--- PASS: TestNew (0.00s)
    --- PASS: TestNew/inmem_with_options (0.00s)
    --- PASS: TestNew/disk_with_options (0.03s)
    --- PASS: TestNew/disk_default_options (0.03s)
    --- PASS: TestNew/migration_on_latest_version (0.00s)
        --- PASS: TestNew/migration_on_latest_version/inmem (0.00s)
        --- PASS: TestNew/migration_on_latest_version/disk (0.03s)
=== CONT  TestUploadStore/disk/upload_10_chunks_with_pin
=== CONT  TestUploadStore/disk/get_session_info
=== RUN   TestUploadStore/disk/get_session_info/done
=== RUN   TestUploadStore/disk/get_session_info/cleanup
=== CONT  TestUploadStore/disk/upload_30_chunks_with_pin
=== CONT  TestUploadStore/disk/upload_20_chunks_with_pin_rollback
=== CONT  TestUploadStore/disk/upload_20_chunks_rollback
--- PASS: TestRadiusManager (0.00s)
    --- PASS: TestRadiusManager/old_nodes_starts_at_previous_radius (0.09s)
    --- PASS: TestRadiusManager/radius_decrease_due_to_under_utilization (0.98s)
    --- PASS: TestRadiusManager/radius_doesnt_change_due_to_non-zero_pull_rate (1.05s)
=== CONT  TestUploadStore/disk/upload_30_chunks
=== CONT  TestUploadStore/disk/no_tag
=== CONT  TestUploadStore/disk/upload_10_chunks
=== CONT  TestNetStore/inmem/direct_upload/context_cancellation
=== CONT  TestNetStore/inmem/direct_upload/commit
=== CONT  TestNetStore/inmem/direct_upload/pusher_error
=== CONT  TestNetStore/inmem/download/with_cache
=== CONT  TestNetStore/inmem/download/no_cache
=== CONT  TestNetStore/disk/direct_upload/commit
=== CONT  TestNetStore/disk/direct_upload/context_cancellation
=== CONT  TestNetStore/disk/direct_upload/pusher_error
=== CONT  TestNetStore/disk/download/with_cache
=== RUN   TestSubscribeBin/disk/subscribe_full_range
=== PAUSE TestSubscribeBin/disk/subscribe_full_range
=== RUN   TestSubscribeBin/disk/subscribe_unsub
=== PAUSE TestSubscribeBin/disk/subscribe_unsub
=== RUN   TestSubscribeBin/disk/subscribe_sub_range
=== PAUSE TestSubscribeBin/disk/subscribe_sub_range
=== RUN   TestSubscribeBin/disk/subscribe_beyond_range
=== PAUSE TestSubscribeBin/disk/subscribe_beyond_range
=== CONT  TestNetStore/disk/download/no_cache
=== CONT  TestSubscribeBin/disk/subscribe_full_range
=== CONT  TestSubscribeBin/disk/subscribe_sub_range
=== CONT  TestSubscribeBin/disk/subscribe_beyond_range
=== CONT  TestSubscribeBin/disk/subscribe_unsub
=== RUN   TestSubscribeBin/mem/subscribe_full_range
=== PAUSE TestSubscribeBin/mem/subscribe_full_range
=== RUN   TestSubscribeBin/mem/subscribe_unsub
=== PAUSE TestSubscribeBin/mem/subscribe_unsub
=== RUN   TestSubscribeBin/mem/subscribe_sub_range
=== PAUSE TestSubscribeBin/mem/subscribe_sub_range
=== RUN   TestSubscribeBin/mem/subscribe_beyond_range
=== PAUSE TestSubscribeBin/mem/subscribe_beyond_range
=== CONT  TestSubscribeBin/mem/subscribe_full_range
=== CONT  TestSubscribeBin/mem/subscribe_sub_range
=== CONT  TestSubscribeBin/mem/subscribe_beyond_range
=== CONT  TestSubscribeBin/mem/subscribe_unsub
--- PASS: TestUploadStore (0.00s)
    --- PASS: TestUploadStore/inmem (0.00s)
        --- PASS: TestUploadStore/inmem/new_session (0.00s)
        --- PASS: TestUploadStore/inmem/upload_10_chunks_with_pin (0.00s)
        --- PASS: TestUploadStore/inmem/get_session_info (0.00s)
            --- PASS: TestUploadStore/inmem/get_session_info/done (0.00s)
            --- PASS: TestUploadStore/inmem/get_session_info/cleanup (0.00s)
        --- PASS: TestUploadStore/inmem/upload_30_chunks_with_pin (0.00s)
        --- PASS: TestUploadStore/inmem/upload_20_chunks_with_pin_rollback (0.00s)
        --- PASS: TestUploadStore/inmem/upload_20_chunks_rollback (0.00s)
        --- PASS: TestUploadStore/inmem/upload_30_chunks (0.00s)
        --- PASS: TestUploadStore/inmem/upload_10_chunks (0.00s)
        --- PASS: TestUploadStore/inmem/no_tag (0.00s)
    --- PASS: TestUploadStore/disk (0.08s)
        --- PASS: TestUploadStore/disk/new_session (0.03s)
        --- PASS: TestUploadStore/disk/upload_10_chunks_with_pin (0.17s)
        --- PASS: TestUploadStore/disk/get_session_info (0.22s)
            --- PASS: TestUploadStore/disk/get_session_info/done (0.10s)
            --- PASS: TestUploadStore/disk/get_session_info/cleanup (0.10s)
        --- PASS: TestUploadStore/disk/upload_20_chunks_rollback (0.21s)
        --- PASS: TestUploadStore/disk/no_tag (0.02s)
        --- PASS: TestUploadStore/disk/upload_20_chunks_with_pin_rollback (0.29s)
        --- PASS: TestUploadStore/disk/upload_30_chunks (0.29s)
        --- PASS: TestUploadStore/disk/upload_10_chunks (0.11s)
        --- PASS: TestUploadStore/disk/upload_30_chunks_with_pin (0.42s)
--- PASS: TestNetStore (0.00s)
    --- PASS: TestNetStore/inmem (0.00s)
        --- PASS: TestNetStore/inmem/direct_upload (0.00s)
            --- PASS: TestNetStore/inmem/direct_upload/context_cancellation (0.00s)
            --- PASS: TestNetStore/inmem/direct_upload/commit (0.00s)
            --- PASS: TestNetStore/inmem/direct_upload/pusher_error (0.00s)
        --- PASS: TestNetStore/inmem/download (0.00s)
            --- PASS: TestNetStore/inmem/download/with_cache (0.00s)
            --- PASS: TestNetStore/inmem/download/no_cache (0.00s)
    --- PASS: TestNetStore/disk (0.04s)
        --- PASS: TestNetStore/disk/direct_upload (0.00s)
            --- PASS: TestNetStore/disk/direct_upload/commit (0.02s)
            --- PASS: TestNetStore/disk/direct_upload/context_cancellation (0.02s)
            --- PASS: TestNetStore/disk/direct_upload/pusher_error (0.02s)
        --- PASS: TestNetStore/disk/download (0.00s)
            --- PASS: TestNetStore/disk/download/no_cache (0.09s)
            --- PASS: TestNetStore/disk/download/with_cache (0.13s)
--- PASS: TestUnreserveCap (0.00s)
    --- PASS: TestUnreserveCap/disk (0.89s)
    --- PASS: TestUnreserveCap/mem (0.86s)
--- PASS: TestSubscribeBin (0.00s)
    --- PASS: TestSubscribeBin/disk (1.35s)
        --- PASS: TestSubscribeBin/disk/subscribe_full_range (0.00s)
        --- PASS: TestSubscribeBin/disk/subscribe_sub_range (0.00s)
        --- PASS: TestSubscribeBin/disk/subscribe_unsub (0.00s)
        --- PASS: TestSubscribeBin/disk/subscribe_beyond_range (0.50s)
    --- PASS: TestSubscribeBin/mem (1.33s)
        --- PASS: TestSubscribeBin/mem/subscribe_full_range (0.00s)
        --- PASS: TestSubscribeBin/mem/subscribe_sub_range (0.00s)
        --- PASS: TestSubscribeBin/mem/subscribe_unsub (0.00s)
        --- PASS: TestSubscribeBin/mem/subscribe_beyond_range (0.50s)
PASS
ok  	github.com/ethersphere/bee/pkg/localstorev2	5.488s
=== RUN   TestCacheStateItem
=== PAUSE TestCacheStateItem
=== RUN   TestCacheEntryItem
=== PAUSE TestCacheEntryItem
=== RUN   TestCache
=== PAUSE TestCache
=== CONT  TestCacheStateItem
=== RUN   TestCacheStateItem/zero_values_marshal/unmarshal
=== PAUSE TestCacheStateItem/zero_values_marshal/unmarshal
=== RUN   TestCacheStateItem/zero_values_clone
=== PAUSE TestCacheStateItem/zero_values_clone
=== RUN   TestCacheStateItem/zero_and_non-zero_1_marshal/unmarshal
=== PAUSE TestCacheStateItem/zero_and_non-zero_1_marshal/unmarshal
=== RUN   TestCacheStateItem/zero_and_non-zero_1_clone
=== PAUSE TestCacheStateItem/zero_and_non-zero_1_clone
=== RUN   TestCacheStateItem/zero_and_non-zero_2_marshal/unmarshal
=== PAUSE TestCacheStateItem/zero_and_non-zero_2_marshal/unmarshal
=== CONT  TestCache
=== RUN   TestCache/fresh_new_cache
=== PAUSE TestCache/fresh_new_cache
=== RUN   TestCache/putter
=== PAUSE TestCache/putter
=== RUN   TestCacheStateItem/zero_and_non-zero_2_clone
=== RUN   TestCache/getter
=== PAUSE TestCache/getter
=== RUN   TestCache/handle_error
=== PAUSE TestCache/handle_error
=== CONT  TestCache/fresh_new_cache
=== CONT  TestCache/handle_error
=== PAUSE TestCacheStateItem/zero_and_non-zero_2_clone
=== RUN   TestCacheStateItem/max_values_marshal/unmarshal
=== PAUSE TestCacheStateItem/max_values_marshal/unmarshal
=== RUN   TestCacheStateItem/max_values_clone
=== CONT  TestCache/getter
=== PAUSE TestCacheStateItem/max_values_clone
=== RUN   TestCacheStateItem/invalid_size_marshal/unmarshal
=== PAUSE TestCacheStateItem/invalid_size_marshal/unmarshal
=== RUN   TestCacheStateItem/invalid_size_clone
=== PAUSE TestCacheStateItem/invalid_size_clone
=== CONT  TestCache/putter
=== CONT  TestCacheEntryItem
=== RUN   TestCacheEntryItem/zero_address_marshal/unmarshal
=== PAUSE TestCacheEntryItem/zero_address_marshal/unmarshal
=== RUN   TestCacheEntryItem/zero_address_clone
=== PAUSE TestCacheEntryItem/zero_address_clone
=== RUN   TestCacheEntryItem/zero_values_marshal/unmarshal
=== PAUSE TestCacheEntryItem/zero_values_marshal/unmarshal
=== RUN   TestCacheEntryItem/zero_values_clone
=== PAUSE TestCacheEntryItem/zero_values_clone
=== RUN   TestCacheEntryItem/max_values_marshal/unmarshal
=== PAUSE TestCacheEntryItem/max_values_marshal/unmarshal
=== RUN   TestCacheEntryItem/max_values_clone
=== PAUSE TestCacheEntryItem/max_values_clone
=== RUN   TestCacheEntryItem/invalid_size_marshal/unmarshal
=== PAUSE TestCacheEntryItem/invalid_size_marshal/unmarshal
=== RUN   TestCacheEntryItem/invalid_size_clone
=== PAUSE TestCacheEntryItem/invalid_size_clone
=== CONT  TestCacheStateItem/zero_values_marshal/unmarshal
=== RUN   TestCache/handle_error/put_error_handling
=== CONT  TestCacheStateItem/invalid_size_clone
=== CONT  TestCacheStateItem/zero_and_non-zero_2_clone
=== CONT  TestCacheStateItem/zero_and_non-zero_1_clone
=== CONT  TestCacheStateItem/zero_and_non-zero_1_marshal/unmarshal
=== CONT  TestCacheStateItem/zero_values_clone
=== CONT  TestCacheEntryItem/zero_address_marshal/unmarshal
=== CONT  TestCacheStateItem/max_values_marshal/unmarshal
=== CONT  TestCacheEntryItem/zero_values_clone
=== CONT  TestCacheEntryItem/zero_values_marshal/unmarshal
=== CONT  TestCacheEntryItem/zero_address_clone
=== CONT  TestCacheStateItem/invalid_size_marshal/unmarshal
=== CONT  TestCacheEntryItem/invalid_size_clone
=== CONT  TestCacheEntryItem/max_values_clone
=== CONT  TestCacheEntryItem/max_values_marshal/unmarshal
=== RUN   TestCache/getter/add_and_get_last
=== CONT  TestCacheEntryItem/invalid_size_marshal/unmarshal
--- PASS: TestCacheEntryItem (0.00s)
    --- PASS: TestCacheEntryItem/zero_address_marshal/unmarshal (0.00s)
    --- PASS: TestCacheEntryItem/zero_values_clone (0.00s)
    --- PASS: TestCacheEntryItem/zero_values_marshal/unmarshal (0.00s)
    --- PASS: TestCacheEntryItem/zero_address_clone (0.00s)
    --- PASS: TestCacheEntryItem/invalid_size_clone (0.00s)
    --- PASS: TestCacheEntryItem/max_values_clone (0.00s)
    --- PASS: TestCacheEntryItem/max_values_marshal/unmarshal (0.00s)
    --- PASS: TestCacheEntryItem/invalid_size_marshal/unmarshal (0.00s)
=== CONT  TestCacheStateItem/max_values_clone
=== CONT  TestCacheStateItem/zero_and_non-zero_2_marshal/unmarshal
--- PASS: TestCacheStateItem (0.00s)
    --- PASS: TestCacheStateItem/zero_values_marshal/unmarshal (0.00s)
    --- PASS: TestCacheStateItem/invalid_size_clone (0.00s)
    --- PASS: TestCacheStateItem/zero_and_non-zero_2_clone (0.00s)
    --- PASS: TestCacheStateItem/zero_and_non-zero_1_clone (0.00s)
    --- PASS: TestCacheStateItem/zero_and_non-zero_1_marshal/unmarshal (0.00s)
    --- PASS: TestCacheStateItem/zero_values_clone (0.00s)
    --- PASS: TestCacheStateItem/zero_and_non-zero_2_marshal/unmarshal (0.00s)
    --- PASS: TestCacheStateItem/max_values_marshal/unmarshal (0.00s)
    --- PASS: TestCacheStateItem/invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestCacheStateItem/max_values_clone (0.00s)
=== RUN   TestCache/putter/add_till_full
=== RUN   TestCache/getter/get_reverse_order
=== RUN   TestCache/putter/new_cache_retains_state
=== RUN   TestCache/handle_error/get_error_handling
=== RUN   TestCache/getter/not_in_chunkstore_returns_error
=== RUN   TestCache/putter/add_over_capacity
=== RUN   TestCache/putter/new_with_lower_capacity
=== RUN   TestCache/getter/not_in_cache_doesnt_affect_state
--- PASS: TestCache (0.00s)
    --- PASS: TestCache/fresh_new_cache (0.00s)
    --- PASS: TestCache/handle_error (0.01s)
        --- PASS: TestCache/handle_error/put_error_handling (0.00s)
        --- PASS: TestCache/handle_error/get_error_handling (0.00s)
    --- PASS: TestCache/putter (0.01s)
        --- PASS: TestCache/putter/add_till_full (0.00s)
        --- PASS: TestCache/putter/new_cache_retains_state (0.00s)
        --- PASS: TestCache/putter/add_over_capacity (0.00s)
        --- PASS: TestCache/putter/new_with_lower_capacity (0.00s)
    --- PASS: TestCache/getter (0.01s)
        --- PASS: TestCache/getter/add_and_get_last (0.00s)
        --- PASS: TestCache/getter/get_reverse_order (0.00s)
        --- PASS: TestCache/getter/not_in_chunkstore_returns_error (0.00s)
        --- PASS: TestCache/getter/not_in_cache_doesnt_affect_state (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/localstorev2/internal/cache	(cached)
=== RUN   TestChunkStampItem
=== PAUSE TestChunkStampItem
=== RUN   TestStoreLoadDelete
=== PAUSE TestStoreLoadDelete
=== CONT  TestChunkStampItem
=== CONT  TestStoreLoadDelete
=== RUN   TestStoreLoadDelete/namespace_0
=== RUN   TestStoreLoadDelete/namespace_0/store_new_chunk_stamp
=== RUN   TestChunkStampItem/zero_namespace_marshal/unmarshal
=== PAUSE TestChunkStampItem/zero_namespace_marshal/unmarshal
=== RUN   TestChunkStampItem/zero_namespace_clone
=== RUN   TestStoreLoadDelete/namespace_0/load_stored_chunk_stamp
=== PAUSE TestChunkStampItem/zero_namespace_clone
=== RUN   TestChunkStampItem/zero_address_marshal/unmarshal
=== PAUSE TestChunkStampItem/zero_address_marshal/unmarshal
=== RUN   TestChunkStampItem/zero_address_clone
=== PAUSE TestChunkStampItem/zero_address_clone
=== RUN   TestChunkStampItem/nil_stamp_marshal/unmarshal
=== PAUSE TestChunkStampItem/nil_stamp_marshal/unmarshal
=== RUN   TestChunkStampItem/nil_stamp_clone
=== PAUSE TestChunkStampItem/nil_stamp_clone
=== RUN   TestChunkStampItem/zero_stamp_marshal/unmarshal
=== PAUSE TestChunkStampItem/zero_stamp_marshal/unmarshal
=== RUN   TestChunkStampItem/zero_stamp_clone
=== RUN   TestStoreLoadDelete/namespace_0/load_stored_chunk_stamp_with_batch_id
=== PAUSE TestChunkStampItem/zero_stamp_clone
=== RUN   TestChunkStampItem/min_values_marshal/unmarshal
=== PAUSE TestChunkStampItem/min_values_marshal/unmarshal
=== RUN   TestChunkStampItem/min_values_clone
=== PAUSE TestChunkStampItem/min_values_clone
=== RUN   TestChunkStampItem/valid_values_marshal/unmarshal
=== PAUSE TestChunkStampItem/valid_values_marshal/unmarshal
=== RUN   TestChunkStampItem/valid_values_clone
=== PAUSE TestChunkStampItem/valid_values_clone
=== RUN   TestChunkStampItem/nil_address_on_unmarshal_marshal/unmarshal
=== PAUSE TestChunkStampItem/nil_address_on_unmarshal_marshal/unmarshal
=== RUN   TestChunkStampItem/nil_address_on_unmarshal_clone
=== PAUSE TestChunkStampItem/nil_address_on_unmarshal_clone
=== RUN   TestChunkStampItem/invalid_size_marshal/unmarshal
=== PAUSE TestChunkStampItem/invalid_size_marshal/unmarshal
=== RUN   TestChunkStampItem/invalid_size_clone
=== PAUSE TestChunkStampItem/invalid_size_clone
=== CONT  TestChunkStampItem/zero_namespace_marshal/unmarshal
=== CONT  TestChunkStampItem/invalid_size_clone
=== CONT  TestChunkStampItem/invalid_size_marshal/unmarshal
=== CONT  TestChunkStampItem/nil_address_on_unmarshal_clone
=== CONT  TestChunkStampItem/zero_stamp_clone
=== CONT  TestChunkStampItem/valid_values_clone
=== RUN   TestStoreLoadDelete/namespace_0/delete_stored_stamp
=== CONT  TestChunkStampItem/valid_values_marshal/unmarshal
=== RUN   TestStoreLoadDelete/namespace_0/delete_all_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_1
=== RUN   TestStoreLoadDelete/namespace_1/store_new_chunk_stamp
=== CONT  TestChunkStampItem/min_values_clone
=== CONT  TestChunkStampItem/zero_address_clone
=== CONT  TestChunkStampItem/min_values_marshal/unmarshal
=== RUN   TestStoreLoadDelete/namespace_1/load_stored_chunk_stamp
=== CONT  TestChunkStampItem/zero_stamp_marshal/unmarshal
=== CONT  TestChunkStampItem/zero_address_marshal/unmarshal
=== CONT  TestChunkStampItem/nil_stamp_clone
=== CONT  TestChunkStampItem/zero_namespace_clone
=== CONT  TestChunkStampItem/nil_address_on_unmarshal_marshal/unmarshal
=== CONT  TestChunkStampItem/nil_stamp_marshal/unmarshal
=== RUN   TestStoreLoadDelete/namespace_1/load_stored_chunk_stamp_with_batch_id
--- PASS: TestChunkStampItem (0.00s)
    --- PASS: TestChunkStampItem/zero_namespace_marshal/unmarshal (0.00s)
    --- PASS: TestChunkStampItem/invalid_size_clone (0.00s)
    --- PASS: TestChunkStampItem/invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestChunkStampItem/zero_stamp_clone (0.00s)
    --- PASS: TestChunkStampItem/nil_address_on_unmarshal_clone (0.00s)
    --- PASS: TestChunkStampItem/valid_values_clone (0.00s)
    --- PASS: TestChunkStampItem/zero_address_clone (0.00s)
    --- PASS: TestChunkStampItem/valid_values_marshal/unmarshal (0.00s)
    --- PASS: TestChunkStampItem/min_values_clone (0.00s)
    --- PASS: TestChunkStampItem/zero_stamp_marshal/unmarshal (0.00s)
    --- PASS: TestChunkStampItem/zero_address_marshal/unmarshal (0.00s)
    --- PASS: TestChunkStampItem/nil_stamp_clone (0.00s)
    --- PASS: TestChunkStampItem/zero_namespace_clone (0.00s)
    --- PASS: TestChunkStampItem/nil_address_on_unmarshal_marshal/unmarshal (0.00s)
    --- PASS: TestChunkStampItem/nil_stamp_marshal/unmarshal (0.00s)
    --- PASS: TestChunkStampItem/min_values_marshal/unmarshal (0.00s)
=== RUN   TestStoreLoadDelete/namespace_1/delete_stored_stamp
=== RUN   TestStoreLoadDelete/namespace_1/delete_all_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_2
=== RUN   TestStoreLoadDelete/namespace_2/store_new_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_2/load_stored_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_2/load_stored_chunk_stamp_with_batch_id
=== RUN   TestStoreLoadDelete/namespace_2/delete_stored_stamp
=== RUN   TestStoreLoadDelete/namespace_2/delete_all_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_3
=== RUN   TestStoreLoadDelete/namespace_3/store_new_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_3/load_stored_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_3/load_stored_chunk_stamp_with_batch_id
=== RUN   TestStoreLoadDelete/namespace_3/delete_stored_stamp
=== RUN   TestStoreLoadDelete/namespace_3/delete_all_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_4
=== RUN   TestStoreLoadDelete/namespace_4/store_new_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_4/load_stored_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_4/load_stored_chunk_stamp_with_batch_id
=== RUN   TestStoreLoadDelete/namespace_4/delete_stored_stamp
=== RUN   TestStoreLoadDelete/namespace_4/delete_all_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_5
=== RUN   TestStoreLoadDelete/namespace_5/store_new_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_5/load_stored_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_5/load_stored_chunk_stamp_with_batch_id
=== RUN   TestStoreLoadDelete/namespace_5/delete_stored_stamp
=== RUN   TestStoreLoadDelete/namespace_5/delete_all_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_6
=== RUN   TestStoreLoadDelete/namespace_6/store_new_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_6/load_stored_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_6/load_stored_chunk_stamp_with_batch_id
=== RUN   TestStoreLoadDelete/namespace_6/delete_stored_stamp
=== RUN   TestStoreLoadDelete/namespace_6/delete_all_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_7
=== RUN   TestStoreLoadDelete/namespace_7/store_new_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_7/load_stored_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_7/load_stored_chunk_stamp_with_batch_id
=== RUN   TestStoreLoadDelete/namespace_7/delete_stored_stamp
=== RUN   TestStoreLoadDelete/namespace_7/delete_all_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_8
=== RUN   TestStoreLoadDelete/namespace_8/store_new_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_8/load_stored_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_8/load_stored_chunk_stamp_with_batch_id
=== RUN   TestStoreLoadDelete/namespace_8/delete_stored_stamp
=== RUN   TestStoreLoadDelete/namespace_8/delete_all_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_9
=== RUN   TestStoreLoadDelete/namespace_9/store_new_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_9/load_stored_chunk_stamp
=== RUN   TestStoreLoadDelete/namespace_9/load_stored_chunk_stamp_with_batch_id
=== RUN   TestStoreLoadDelete/namespace_9/delete_stored_stamp
=== RUN   TestStoreLoadDelete/namespace_9/delete_all_stored_stamp_index
--- PASS: TestStoreLoadDelete (0.01s)
    --- PASS: TestStoreLoadDelete/namespace_0 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_0/store_new_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_0/load_stored_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_0/load_stored_chunk_stamp_with_batch_id (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_0/delete_stored_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_0/delete_all_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_1 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_1/store_new_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_1/load_stored_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_1/load_stored_chunk_stamp_with_batch_id (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_1/delete_stored_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_1/delete_all_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_2 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_2/store_new_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_2/load_stored_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_2/load_stored_chunk_stamp_with_batch_id (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_2/delete_stored_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_2/delete_all_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_3 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_3/store_new_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_3/load_stored_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_3/load_stored_chunk_stamp_with_batch_id (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_3/delete_stored_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_3/delete_all_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_4 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_4/store_new_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_4/load_stored_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_4/load_stored_chunk_stamp_with_batch_id (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_4/delete_stored_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_4/delete_all_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_5 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_5/store_new_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_5/load_stored_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_5/load_stored_chunk_stamp_with_batch_id (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_5/delete_stored_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_5/delete_all_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_6 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_6/store_new_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_6/load_stored_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_6/load_stored_chunk_stamp_with_batch_id (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_6/delete_stored_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_6/delete_all_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_7 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_7/store_new_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_7/load_stored_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_7/load_stored_chunk_stamp_with_batch_id (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_7/delete_stored_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_7/delete_all_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_8 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_8/store_new_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_8/load_stored_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_8/load_stored_chunk_stamp_with_batch_id (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_8/delete_stored_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_8/delete_all_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_9 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_9/store_new_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_9/load_stored_chunk_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_9/load_stored_chunk_stamp_with_batch_id (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_9/delete_stored_stamp (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_9/delete_all_stored_stamp_index (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstamp	(cached)
=== RUN   TestRetrievalIndexItem
=== PAUSE TestRetrievalIndexItem
=== RUN   TestChunkStore
=== PAUSE TestChunkStore
=== RUN   TestIterateLocations
=== PAUSE TestIterateLocations
=== RUN   TestIterateLocations_Stop
=== PAUSE TestIterateLocations_Stop
=== RUN   TestTxChunkStore
=== PAUSE TestTxChunkStore
=== RUN   TestMultipleStampsRefCnt
=== PAUSE TestMultipleStampsRefCnt
=== CONT  TestRetrievalIndexItem
=== RUN   TestRetrievalIndexItem/zero_values_marshal/unmarshal
=== PAUSE TestRetrievalIndexItem/zero_values_marshal/unmarshal
=== RUN   TestRetrievalIndexItem/zero_values_clone
=== PAUSE TestRetrievalIndexItem/zero_values_clone
=== RUN   TestRetrievalIndexItem/zero_address_marshal/unmarshal
=== PAUSE TestRetrievalIndexItem/zero_address_marshal/unmarshal
=== RUN   TestRetrievalIndexItem/zero_address_clone
=== PAUSE TestRetrievalIndexItem/zero_address_clone
=== RUN   TestRetrievalIndexItem/min_values_marshal/unmarshal
=== PAUSE TestRetrievalIndexItem/min_values_marshal/unmarshal
=== RUN   TestRetrievalIndexItem/min_values_clone
=== PAUSE TestRetrievalIndexItem/min_values_clone
=== CONT  TestIterateLocations_Stop
=== CONT  TestMultipleStampsRefCnt
=== CONT  TestTxChunkStore
=== RUN   TestTxChunkStore/commit_empty
=== CONT  TestIterateLocations
=== CONT  TestChunkStore
=== RUN   TestMultipleStampsRefCnt/put_with_multiple_stamps
=== RUN   TestMultipleStampsRefCnt/rollback_delete_operations
=== RUN   TestMultipleStampsRefCnt/rollback_delete_operations/less_than_refCnt
=== RUN   TestMultipleStampsRefCnt/rollback_delete_operations/till_refCnt
--- PASS: TestMultipleStampsRefCnt (0.00s)
    --- PASS: TestMultipleStampsRefCnt/put_with_multiple_stamps (0.00s)
    --- PASS: TestMultipleStampsRefCnt/rollback_delete_operations (0.00s)
        --- PASS: TestMultipleStampsRefCnt/rollback_delete_operations/less_than_refCnt (0.00s)
        --- PASS: TestMultipleStampsRefCnt/rollback_delete_operations/till_refCnt (0.00s)
=== RUN   TestRetrievalIndexItem/max_values_marshal/unmarshal
=== PAUSE TestRetrievalIndexItem/max_values_marshal/unmarshal
=== RUN   TestRetrievalIndexItem/max_values_clone
=== PAUSE TestRetrievalIndexItem/max_values_clone
=== RUN   TestRetrievalIndexItem/invalid_size_marshal/unmarshal
=== PAUSE TestRetrievalIndexItem/invalid_size_marshal/unmarshal
=== RUN   TestRetrievalIndexItem/invalid_size_clone
=== PAUSE TestRetrievalIndexItem/invalid_size_clone
=== CONT  TestRetrievalIndexItem/zero_values_marshal/unmarshal
=== CONT  TestRetrievalIndexItem/max_values_marshal/unmarshal
=== CONT  TestRetrievalIndexItem/min_values_clone
=== CONT  TestRetrievalIndexItem/min_values_marshal/unmarshal
=== CONT  TestRetrievalIndexItem/zero_address_clone
=== RUN   TestTxChunkStore/commit
=== CONT  TestRetrievalIndexItem/zero_address_marshal/unmarshal
=== CONT  TestRetrievalIndexItem/zero_values_clone
=== CONT  TestRetrievalIndexItem/max_values_clone
=== CONT  TestRetrievalIndexItem/invalid_size_clone
=== CONT  TestRetrievalIndexItem/invalid_size_marshal/unmarshal
--- PASS: TestRetrievalIndexItem (0.00s)
    --- PASS: TestRetrievalIndexItem/zero_values_marshal/unmarshal (0.00s)
    --- PASS: TestRetrievalIndexItem/max_values_marshal/unmarshal (0.00s)
    --- PASS: TestRetrievalIndexItem/zero_address_clone (0.00s)
    --- PASS: TestRetrievalIndexItem/zero_address_marshal/unmarshal (0.00s)
    --- PASS: TestRetrievalIndexItem/zero_values_clone (0.00s)
    --- PASS: TestRetrievalIndexItem/max_values_clone (0.00s)
    --- PASS: TestRetrievalIndexItem/invalid_size_clone (0.00s)
    --- PASS: TestRetrievalIndexItem/invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestRetrievalIndexItem/min_values_marshal/unmarshal (0.00s)
    --- PASS: TestRetrievalIndexItem/min_values_clone (0.00s)
=== RUN   TestTxChunkStore/commit/add_new_chunks
=== RUN   TestTxChunkStore/commit/delete_existing_chunks
=== RUN   TestTxChunkStore/rollback_empty
=== RUN   TestChunkStore/put_chunks
=== RUN   TestTxChunkStore/rollback_added_chunks
--- PASS: TestIterateLocations_Stop (0.00s)
=== RUN   TestChunkStore/put_existing_chunks
--- PASS: TestIterateLocations (0.00s)
=== RUN   TestChunkStore/get_chunks
=== RUN   TestChunkStore/has_chunks
=== RUN   TestChunkStore/iterate_chunks
=== RUN   TestChunkStore/delete_unique_chunks
=== RUN   TestChunkStore/check_deleted_chunks
=== RUN   TestChunkStore/iterate_chunks_after_delete
=== RUN   TestChunkStore/delete_duplicate_chunks
=== RUN   TestChunkStore/check_chunks_still_exists
=== RUN   TestChunkStore/delete_duplicate_chunks_again
=== RUN   TestChunkStore/check_all_are_deleted
=== RUN   TestChunkStore/close_store
--- PASS: TestChunkStore (0.01s)
    --- PASS: TestChunkStore/put_chunks (0.00s)
    --- PASS: TestChunkStore/put_existing_chunks (0.00s)
    --- PASS: TestChunkStore/get_chunks (0.00s)
    --- PASS: TestChunkStore/has_chunks (0.00s)
    --- PASS: TestChunkStore/iterate_chunks (0.00s)
    --- PASS: TestChunkStore/delete_unique_chunks (0.00s)
    --- PASS: TestChunkStore/check_deleted_chunks (0.00s)
    --- PASS: TestChunkStore/iterate_chunks_after_delete (0.00s)
    --- PASS: TestChunkStore/delete_duplicate_chunks (0.00s)
    --- PASS: TestChunkStore/check_chunks_still_exists (0.00s)
    --- PASS: TestChunkStore/delete_duplicate_chunks_again (0.00s)
    --- PASS: TestChunkStore/check_all_are_deleted (0.00s)
    --- PASS: TestChunkStore/close_store (0.00s)
=== RUN   TestTxChunkStore/rollback_removed_chunks
--- PASS: TestTxChunkStore (0.01s)
    --- PASS: TestTxChunkStore/commit_empty (0.00s)
    --- PASS: TestTxChunkStore/commit (0.00s)
        --- PASS: TestTxChunkStore/commit/add_new_chunks (0.00s)
        --- PASS: TestTxChunkStore/commit/delete_existing_chunks (0.00s)
    --- PASS: TestTxChunkStore/rollback_empty (0.00s)
    --- PASS: TestTxChunkStore/rollback_added_chunks (0.00s)
    --- PASS: TestTxChunkStore/rollback_removed_chunks (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstore	(cached)
=== RUN   TestSubscriber
=== PAUSE TestSubscriber
=== CONT  TestSubscriber
--- PASS: TestSubscriber (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/localstorev2/internal/events	(cached)
=== RUN   TestPinStore
=== RUN   TestPinStore/create_new_collections
=== RUN   TestPinStore/create_new_collections/create_collection_0
=== RUN   TestPinStore/create_new_collections/create_collection_1
=== RUN   TestPinStore/create_new_collections/create_collection_2
=== RUN   TestPinStore/verify_all_collection_data
=== RUN   TestPinStore/verify_all_collection_data/verify_collection_0
=== RUN   TestPinStore/verify_all_collection_data/verify_collection_1
=== RUN   TestPinStore/verify_all_collection_data/verify_collection_2
=== RUN   TestPinStore/verify_root_pins
=== RUN   TestPinStore/has_pin
=== RUN   TestPinStore/verify_internal_state
=== RUN   TestPinStore/delete_collection
=== RUN   TestPinStore/error_after_close
=== RUN   TestPinStore/zero_address_close
--- PASS: TestPinStore (0.01s)
    --- PASS: TestPinStore/create_new_collections (0.00s)
        --- PASS: TestPinStore/create_new_collections/create_collection_0 (0.00s)
        --- PASS: TestPinStore/create_new_collections/create_collection_1 (0.00s)
        --- PASS: TestPinStore/create_new_collections/create_collection_2 (0.00s)
    --- PASS: TestPinStore/verify_all_collection_data (0.00s)
        --- PASS: TestPinStore/verify_all_collection_data/verify_collection_0 (0.00s)
        --- PASS: TestPinStore/verify_all_collection_data/verify_collection_1 (0.00s)
        --- PASS: TestPinStore/verify_all_collection_data/verify_collection_2 (0.00s)
    --- PASS: TestPinStore/verify_root_pins (0.00s)
    --- PASS: TestPinStore/has_pin (0.00s)
    --- PASS: TestPinStore/verify_internal_state (0.00s)
    --- PASS: TestPinStore/delete_collection (0.00s)
    --- PASS: TestPinStore/error_after_close (0.00s)
    --- PASS: TestPinStore/zero_address_close (0.00s)
=== RUN   TestPinCollectionItem
=== PAUSE TestPinCollectionItem
=== RUN   TestPinChunkItem
=== PAUSE TestPinChunkItem
=== CONT  TestPinCollectionItem
=== RUN   TestPinCollectionItem/zero_values_marshal/unmarshal
=== PAUSE TestPinCollectionItem/zero_values_marshal/unmarshal
=== RUN   TestPinCollectionItem/zero_values_clone
=== PAUSE TestPinCollectionItem/zero_values_clone
=== RUN   TestPinCollectionItem/zero_address_marshal/unmarshal
=== PAUSE TestPinCollectionItem/zero_address_marshal/unmarshal
=== RUN   TestPinCollectionItem/zero_address_clone
=== PAUSE TestPinCollectionItem/zero_address_clone
=== RUN   TestPinCollectionItem/zero_UUID_marshal/unmarshal
=== PAUSE TestPinCollectionItem/zero_UUID_marshal/unmarshal
=== RUN   TestPinCollectionItem/zero_UUID_clone
=== PAUSE TestPinCollectionItem/zero_UUID_clone
=== RUN   TestPinCollectionItem/valid_values_marshal/unmarshal
=== PAUSE TestPinCollectionItem/valid_values_marshal/unmarshal
=== RUN   TestPinCollectionItem/valid_values_clone
=== PAUSE TestPinCollectionItem/valid_values_clone
=== RUN   TestPinCollectionItem/max_values_marshal/unmarshal
=== PAUSE TestPinCollectionItem/max_values_marshal/unmarshal
=== RUN   TestPinCollectionItem/max_values_clone
=== PAUSE TestPinCollectionItem/max_values_clone
=== RUN   TestPinCollectionItem/invalid_size_marshal/unmarshal
=== PAUSE TestPinCollectionItem/invalid_size_marshal/unmarshal
=== RUN   TestPinCollectionItem/invalid_size_clone
=== PAUSE TestPinCollectionItem/invalid_size_clone
=== CONT  TestPinCollectionItem/zero_values_marshal/unmarshal
=== CONT  TestPinChunkItem
--- PASS: TestPinChunkItem (0.00s)
=== CONT  TestPinCollectionItem/invalid_size_clone
=== CONT  TestPinCollectionItem/invalid_size_marshal/unmarshal
=== CONT  TestPinCollectionItem/max_values_clone
=== CONT  TestPinCollectionItem/max_values_marshal/unmarshal
=== CONT  TestPinCollectionItem/valid_values_clone
=== CONT  TestPinCollectionItem/valid_values_marshal/unmarshal
=== CONT  TestPinCollectionItem/zero_UUID_clone
=== CONT  TestPinCollectionItem/zero_UUID_marshal/unmarshal
=== CONT  TestPinCollectionItem/zero_address_clone
=== CONT  TestPinCollectionItem/zero_address_marshal/unmarshal
=== CONT  TestPinCollectionItem/zero_values_clone
--- PASS: TestPinCollectionItem (0.00s)
    --- PASS: TestPinCollectionItem/zero_values_marshal/unmarshal (0.00s)
    --- PASS: TestPinCollectionItem/invalid_size_clone (0.00s)
    --- PASS: TestPinCollectionItem/invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestPinCollectionItem/max_values_clone (0.00s)
    --- PASS: TestPinCollectionItem/max_values_marshal/unmarshal (0.00s)
    --- PASS: TestPinCollectionItem/valid_values_clone (0.00s)
    --- PASS: TestPinCollectionItem/valid_values_marshal/unmarshal (0.00s)
    --- PASS: TestPinCollectionItem/zero_UUID_clone (0.00s)
    --- PASS: TestPinCollectionItem/zero_UUID_marshal/unmarshal (0.00s)
    --- PASS: TestPinCollectionItem/zero_address_clone (0.00s)
    --- PASS: TestPinCollectionItem/zero_address_marshal/unmarshal (0.00s)
    --- PASS: TestPinCollectionItem/zero_values_clone (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/localstorev2/internal/pinning	(cached)
=== RUN   TestReserveItems
=== PAUSE TestReserveItems
=== CONT  TestReserveItems
=== RUN   TestReserveItems/BatchRadiusItem_marshal/unmarshal
=== PAUSE TestReserveItems/BatchRadiusItem_marshal/unmarshal
=== RUN   TestReserveItems/BatchRadiusItem_clone
=== PAUSE TestReserveItems/BatchRadiusItem_clone
=== RUN   TestReserveItems/ChunkBinItem_marshal/unmarshal
=== PAUSE TestReserveItems/ChunkBinItem_marshal/unmarshal
=== RUN   TestReserveItems/ChunkBinItem_clone
=== PAUSE TestReserveItems/ChunkBinItem_clone
=== RUN   TestReserveItems/BinItem_marshal/unmarshal
=== PAUSE TestReserveItems/BinItem_marshal/unmarshal
=== RUN   TestReserveItems/BinItem_clone
=== PAUSE TestReserveItems/BinItem_clone
=== RUN   TestReserveItems/RadiusItem_marshal/unmarshal
=== PAUSE TestReserveItems/RadiusItem_marshal/unmarshal
=== RUN   TestReserveItems/RadiusItem_clone
=== PAUSE TestReserveItems/RadiusItem_clone
=== RUN   TestReserveItems/BatchRadiusItem_zero_address_marshal/unmarshal
=== PAUSE TestReserveItems/BatchRadiusItem_zero_address_marshal/unmarshal
=== RUN   TestReserveItems/BatchRadiusItem_zero_address_clone
=== PAUSE TestReserveItems/BatchRadiusItem_zero_address_clone
=== RUN   TestReserveItems/ChunkBinItem_zero_address_marshal/unmarshal
=== PAUSE TestReserveItems/ChunkBinItem_zero_address_marshal/unmarshal
=== RUN   TestReserveItems/ChunkBinItem_zero_address_clone
=== PAUSE TestReserveItems/ChunkBinItem_zero_address_clone
=== RUN   TestReserveItems/BatchRadiusItem_invalid_size_marshal/unmarshal
=== PAUSE TestReserveItems/BatchRadiusItem_invalid_size_marshal/unmarshal
=== RUN   TestReserveItems/BatchRadiusItem_invalid_size_clone
=== PAUSE TestReserveItems/BatchRadiusItem_invalid_size_clone
=== RUN   TestReserveItems/ChunkBinItem_invalid_size_marshal/unmarshal
=== PAUSE TestReserveItems/ChunkBinItem_invalid_size_marshal/unmarshal
=== RUN   TestReserveItems/ChunkBinItem_invalid_size_clone
=== PAUSE TestReserveItems/ChunkBinItem_invalid_size_clone
=== RUN   TestReserveItems/BinItem_invalid_size_marshal/unmarshal
=== PAUSE TestReserveItems/BinItem_invalid_size_marshal/unmarshal
=== RUN   TestReserveItems/BinItem_invalid_size_clone
=== PAUSE TestReserveItems/BinItem_invalid_size_clone
=== RUN   TestReserveItems/RadiusItem_invalid_size_marshal/unmarshal
=== PAUSE TestReserveItems/RadiusItem_invalid_size_marshal/unmarshal
=== RUN   TestReserveItems/RadiusItem_invalid_size_clone
=== PAUSE TestReserveItems/RadiusItem_invalid_size_clone
=== CONT  TestReserveItems/BatchRadiusItem_clone
=== CONT  TestReserveItems/ChunkBinItem_zero_address_marshal/unmarshal
=== CONT  TestReserveItems/ChunkBinItem_invalid_size_marshal/unmarshal
=== CONT  TestReserveItems/BinItem_invalid_size_clone
=== CONT  TestReserveItems/BinItem_invalid_size_marshal/unmarshal
=== CONT  TestReserveItems/BatchRadiusItem_zero_address_clone
=== CONT  TestReserveItems/BatchRadiusItem_zero_address_marshal/unmarshal
=== CONT  TestReserveItems/RadiusItem_marshal/unmarshal
=== CONT  TestReserveItems/BinItem_clone
=== CONT  TestReserveItems/BinItem_marshal/unmarshal
=== CONT  TestReserveItems/ChunkBinItem_marshal/unmarshal
=== CONT  TestReserveItems/ChunkBinItem_invalid_size_clone
=== CONT  TestReserveItems/BatchRadiusItem_marshal/unmarshal
=== CONT  TestReserveItems/BatchRadiusItem_invalid_size_marshal/unmarshal
=== CONT  TestReserveItems/RadiusItem_clone
=== CONT  TestReserveItems/ChunkBinItem_zero_address_clone
=== CONT  TestReserveItems/RadiusItem_invalid_size_marshal/unmarshal
=== CONT  TestReserveItems/RadiusItem_invalid_size_clone
=== CONT  TestReserveItems/BatchRadiusItem_invalid_size_clone
=== CONT  TestReserveItems/ChunkBinItem_clone
--- PASS: TestReserveItems (0.00s)
    --- PASS: TestReserveItems/ChunkBinItem_zero_address_marshal/unmarshal (0.00s)
    --- PASS: TestReserveItems/ChunkBinItem_invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestReserveItems/BatchRadiusItem_clone (0.00s)
    --- PASS: TestReserveItems/BinItem_invalid_size_clone (0.00s)
    --- PASS: TestReserveItems/BinItem_invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestReserveItems/BatchRadiusItem_zero_address_clone (0.00s)
    --- PASS: TestReserveItems/BatchRadiusItem_zero_address_marshal/unmarshal (0.00s)
    --- PASS: TestReserveItems/RadiusItem_marshal/unmarshal (0.00s)
    --- PASS: TestReserveItems/BinItem_clone (0.00s)
    --- PASS: TestReserveItems/BinItem_marshal/unmarshal (0.00s)
    --- PASS: TestReserveItems/ChunkBinItem_marshal/unmarshal (0.00s)
    --- PASS: TestReserveItems/BatchRadiusItem_invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestReserveItems/RadiusItem_clone (0.00s)
    --- PASS: TestReserveItems/ChunkBinItem_zero_address_clone (0.00s)
    --- PASS: TestReserveItems/BatchRadiusItem_marshal/unmarshal (0.00s)
    --- PASS: TestReserveItems/RadiusItem_invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestReserveItems/RadiusItem_invalid_size_clone (0.00s)
    --- PASS: TestReserveItems/BatchRadiusItem_invalid_size_clone (0.00s)
    --- PASS: TestReserveItems/ChunkBinItem_invalid_size_clone (0.00s)
    --- PASS: TestReserveItems/ChunkBinItem_clone (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/localstorev2/internal/reserve	(cached)
=== RUN   TestStampIndexItem
=== PAUSE TestStampIndexItem
=== RUN   TestStoreLoadDelete
=== PAUSE TestStoreLoadDelete
=== RUN   TestLoadOrStore
=== PAUSE TestLoadOrStore
=== CONT  TestStampIndexItem
=== CONT  TestLoadOrStore
=== RUN   TestStampIndexItem/zero_namespace_marshal/unmarshal
=== PAUSE TestStampIndexItem/zero_namespace_marshal/unmarshal
=== RUN   TestStampIndexItem/zero_namespace_clone
=== PAUSE TestStampIndexItem/zero_namespace_clone
=== RUN   TestStampIndexItem/zero_batchID_marshal/unmarshal
=== PAUSE TestStampIndexItem/zero_batchID_marshal/unmarshal
=== RUN   TestStampIndexItem/zero_batchID_clone
=== PAUSE TestStampIndexItem/zero_batchID_clone
=== RUN   TestStampIndexItem/zero_batchIndex_marshal/unmarshal
=== PAUSE TestStampIndexItem/zero_batchIndex_marshal/unmarshal
=== RUN   TestStampIndexItem/zero_batchIndex_clone
=== PAUSE TestStampIndexItem/zero_batchIndex_clone
=== RUN   TestStampIndexItem/valid_values_marshal/unmarshal
=== PAUSE TestStampIndexItem/valid_values_marshal/unmarshal
=== CONT  TestStoreLoadDelete
=== RUN   TestLoadOrStore/namespace_0
=== RUN   TestStampIndexItem/valid_values_clone
=== PAUSE TestStampIndexItem/valid_values_clone
=== RUN   TestStampIndexItem/max_values_marshal/unmarshal
=== PAUSE TestStampIndexItem/max_values_marshal/unmarshal
=== RUN   TestStampIndexItem/max_values_clone
=== PAUSE TestStampIndexItem/max_values_clone
=== RUN   TestStampIndexItem/invalid_size_marshal/unmarshal
=== PAUSE TestStampIndexItem/invalid_size_marshal/unmarshal
=== RUN   TestStampIndexItem/invalid_size_clone
=== PAUSE TestStampIndexItem/invalid_size_clone
=== CONT  TestStampIndexItem/zero_namespace_marshal/unmarshal
=== RUN   TestLoadOrStore/namespace_1
=== RUN   TestStoreLoadDelete/namespace_0
=== RUN   TestStoreLoadDelete/namespace_0/store_new_stamp_index
=== CONT  TestStampIndexItem/valid_values_marshal/unmarshal
=== RUN   TestStoreLoadDelete/namespace_0/load_stored_stamp_index
=== CONT  TestStampIndexItem/zero_batchIndex_clone
=== RUN   TestLoadOrStore/namespace_2
=== CONT  TestStampIndexItem/zero_namespace_clone
=== CONT  TestStampIndexItem/max_values_marshal/unmarshal
=== CONT  TestStampIndexItem/zero_batchID_clone
=== CONT  TestStampIndexItem/zero_batchID_marshal/unmarshal
=== CONT  TestStampIndexItem/valid_values_clone
=== CONT  TestStampIndexItem/max_values_clone
=== CONT  TestStampIndexItem/invalid_size_clone
=== CONT  TestStampIndexItem/invalid_size_marshal/unmarshal
=== CONT  TestStampIndexItem/zero_batchIndex_marshal/unmarshal
=== RUN   TestLoadOrStore/namespace_3
=== RUN   TestStoreLoadDelete/namespace_0/delete_stored_stamp_index
--- PASS: TestStampIndexItem (0.00s)
    --- PASS: TestStampIndexItem/zero_namespace_marshal/unmarshal (0.00s)
    --- PASS: TestStampIndexItem/valid_values_marshal/unmarshal (0.00s)
    --- PASS: TestStampIndexItem/zero_batchID_clone (0.00s)
    --- PASS: TestStampIndexItem/zero_batchIndex_clone (0.00s)
    --- PASS: TestStampIndexItem/zero_namespace_clone (0.00s)
    --- PASS: TestStampIndexItem/zero_batchID_marshal/unmarshal (0.00s)
    --- PASS: TestStampIndexItem/max_values_marshal/unmarshal (0.00s)
    --- PASS: TestStampIndexItem/valid_values_clone (0.00s)
    --- PASS: TestStampIndexItem/max_values_clone (0.00s)
    --- PASS: TestStampIndexItem/invalid_size_clone (0.00s)
    --- PASS: TestStampIndexItem/invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestStampIndexItem/zero_batchIndex_marshal/unmarshal (0.00s)
=== RUN   TestStoreLoadDelete/namespace_1
=== RUN   TestStoreLoadDelete/namespace_1/store_new_stamp_index
=== RUN   TestStoreLoadDelete/namespace_1/load_stored_stamp_index
=== RUN   TestLoadOrStore/namespace_4
=== RUN   TestStoreLoadDelete/namespace_1/delete_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_2
=== RUN   TestStoreLoadDelete/namespace_2/store_new_stamp_index
=== RUN   TestLoadOrStore/namespace_5
=== RUN   TestStoreLoadDelete/namespace_2/load_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_2/delete_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_3
=== RUN   TestLoadOrStore/namespace_6
=== RUN   TestStoreLoadDelete/namespace_3/store_new_stamp_index
=== RUN   TestLoadOrStore/namespace_7
=== RUN   TestStoreLoadDelete/namespace_3/load_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_3/delete_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_4
=== RUN   TestStoreLoadDelete/namespace_4/store_new_stamp_index
=== RUN   TestLoadOrStore/namespace_8
=== RUN   TestStoreLoadDelete/namespace_4/load_stored_stamp_index
=== RUN   TestLoadOrStore/namespace_9
=== RUN   TestStoreLoadDelete/namespace_4/delete_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_5
=== RUN   TestStoreLoadDelete/namespace_5/store_new_stamp_index
--- PASS: TestLoadOrStore (0.00s)
    --- PASS: TestLoadOrStore/namespace_0 (0.00s)
    --- PASS: TestLoadOrStore/namespace_1 (0.00s)
    --- PASS: TestLoadOrStore/namespace_2 (0.00s)
    --- PASS: TestLoadOrStore/namespace_3 (0.00s)
    --- PASS: TestLoadOrStore/namespace_4 (0.00s)
    --- PASS: TestLoadOrStore/namespace_5 (0.00s)
    --- PASS: TestLoadOrStore/namespace_6 (0.00s)
    --- PASS: TestLoadOrStore/namespace_7 (0.00s)
    --- PASS: TestLoadOrStore/namespace_8 (0.00s)
    --- PASS: TestLoadOrStore/namespace_9 (0.00s)
=== RUN   TestStoreLoadDelete/namespace_5/load_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_5/delete_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_6
=== RUN   TestStoreLoadDelete/namespace_6/store_new_stamp_index
=== RUN   TestStoreLoadDelete/namespace_6/load_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_6/delete_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_7
=== RUN   TestStoreLoadDelete/namespace_7/store_new_stamp_index
=== RUN   TestStoreLoadDelete/namespace_7/load_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_7/delete_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_8
=== RUN   TestStoreLoadDelete/namespace_8/store_new_stamp_index
=== RUN   TestStoreLoadDelete/namespace_8/load_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_8/delete_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_9
=== RUN   TestStoreLoadDelete/namespace_9/store_new_stamp_index
=== RUN   TestStoreLoadDelete/namespace_9/load_stored_stamp_index
=== RUN   TestStoreLoadDelete/namespace_9/delete_stored_stamp_index
--- PASS: TestStoreLoadDelete (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_0 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_0/store_new_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_0/load_stored_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_0/delete_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_1 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_1/store_new_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_1/load_stored_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_1/delete_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_2 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_2/store_new_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_2/load_stored_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_2/delete_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_3 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_3/store_new_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_3/load_stored_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_3/delete_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_4 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_4/store_new_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_4/load_stored_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_4/delete_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_5 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_5/store_new_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_5/load_stored_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_5/delete_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_6 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_6/store_new_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_6/load_stored_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_6/delete_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_7 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_7/store_new_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_7/load_stored_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_7/delete_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_8 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_8/store_new_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_8/load_stored_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_8/delete_stored_stamp_index (0.00s)
    --- PASS: TestStoreLoadDelete/namespace_9 (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_9/store_new_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_9/load_stored_stamp_index (0.00s)
        --- PASS: TestStoreLoadDelete/namespace_9/delete_stored_stamp_index (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/localstorev2/internal/stampindex	(cached)
=== RUN   TestPushItem
=== PAUSE TestPushItem
=== RUN   TestTagItem
=== PAUSE TestTagItem
=== RUN   TestUploadItem
=== PAUSE TestUploadItem
=== RUN   TestItemNextTagID
=== PAUSE TestItemNextTagID
=== RUN   TestChunkPutter
=== PAUSE TestChunkPutter
=== RUN   TestChunkReporter
=== PAUSE TestChunkReporter
=== RUN   TestStampIndexHandling
=== PAUSE TestStampIndexHandling
=== RUN   TestNextTagID
=== PAUSE TestNextTagID
=== RUN   TestIterate
=== PAUSE TestIterate
=== CONT  TestPushItem
=== RUN   TestPushItem/zero_values_marshal/unmarshal
=== CONT  TestChunkReporter
=== CONT  TestStampIndexHandling
=== RUN   TestStampIndexHandling/put_chunk_with_immutable_batch
=== PAUSE TestPushItem/zero_values_marshal/unmarshal
=== RUN   TestPushItem/zero_values_clone
=== PAUSE TestPushItem/zero_values_clone
=== RUN   TestPushItem/zero_address_marshal/unmarshal
=== PAUSE TestPushItem/zero_address_marshal/unmarshal
=== RUN   TestPushItem/zero_address_clone
=== PAUSE TestPushItem/zero_address_clone
=== RUN   TestPushItem/nil_stamp_marshal/unmarshal
=== PAUSE TestPushItem/nil_stamp_marshal/unmarshal
=== RUN   TestPushItem/nil_stamp_clone
=== PAUSE TestPushItem/nil_stamp_clone
=== RUN   TestPushItem/min_values_marshal/unmarshal
=== PAUSE TestPushItem/min_values_marshal/unmarshal
=== RUN   TestPushItem/min_values_clone
=== PAUSE TestPushItem/min_values_clone
=== RUN   TestPushItem/max_values_marshal/unmarshal
=== PAUSE TestPushItem/max_values_marshal/unmarshal
=== RUN   TestPushItem/max_values_clone
=== PAUSE TestPushItem/max_values_clone
=== RUN   TestPushItem/random_values_marshal/unmarshal
=== PAUSE TestPushItem/random_values_marshal/unmarshal
=== RUN   TestPushItem/random_values_clone
=== PAUSE TestPushItem/random_values_clone
=== RUN   TestPushItem/invalid_size_marshal/unmarshal
=== PAUSE TestPushItem/invalid_size_marshal/unmarshal
=== RUN   TestPushItem/invalid_size_clone
=== PAUSE TestPushItem/invalid_size_clone
=== RUN   TestChunkReporter/chunk_9193c850ab9d90c5f01985bb880e0364512fab9e42256bdfdf056fd612126808
=== CONT  TestUploadItem
=== RUN   TestUploadItem/zero_values_marshal/unmarshal
=== PAUSE TestUploadItem/zero_values_marshal/unmarshal
=== RUN   TestUploadItem/zero_values_clone
=== PAUSE TestUploadItem/zero_values_clone
=== CONT  TestTagItem
=== RUN   TestUploadItem/zero_address_marshal/unmarshal
=== RUN   TestTagItem/zero_values_marshal/unmarshal
=== PAUSE TestUploadItem/zero_address_marshal/unmarshal
=== PAUSE TestTagItem/zero_values_marshal/unmarshal
=== RUN   TestChunkReporter/chunk_9193c850ab9d90c5f01985bb880e0364512fab9e42256bdfdf056fd612126808/mark_sent
=== RUN   TestChunkReporter/chunk_9193c850ab9d90c5f01985bb880e0364512fab9e42256bdfdf056fd612126808/mark_stored
=== RUN   TestChunkReporter/chunk_9193c850ab9d90c5f01985bb880e0364512fab9e42256bdfdf056fd612126808/mark_synced
=== RUN   TestChunkReporter/chunk_9193c850ab9d90c5f01985bb880e0364512fab9e42256bdfdf056fd612126808/verify_internal_state
=== RUN   TestChunkReporter/chunk_2447e20eb2b7e9c130e2c85b9fc6327da23cb04d6b4abdec1c0260a6c9c85de1
=== RUN   TestChunkReporter/chunk_2447e20eb2b7e9c130e2c85b9fc6327da23cb04d6b4abdec1c0260a6c9c85de1/mark_sent
=== RUN   TestChunkReporter/chunk_2447e20eb2b7e9c130e2c85b9fc6327da23cb04d6b4abdec1c0260a6c9c85de1/mark_stored
=== RUN   TestChunkReporter/chunk_2447e20eb2b7e9c130e2c85b9fc6327da23cb04d6b4abdec1c0260a6c9c85de1/mark_synced
=== RUN   TestChunkReporter/chunk_2447e20eb2b7e9c130e2c85b9fc6327da23cb04d6b4abdec1c0260a6c9c85de1/verify_internal_state
=== RUN   TestStampIndexHandling/put_existing_index_with_older_batch_timestamp
=== RUN   TestUploadItem/zero_address_clone
=== PAUSE TestUploadItem/zero_address_clone
=== RUN   TestUploadItem/nil_stamp_marshal/unmarshal
=== PAUSE TestUploadItem/nil_stamp_marshal/unmarshal
=== RUN   TestUploadItem/nil_stamp_clone
=== PAUSE TestUploadItem/nil_stamp_clone
=== RUN   TestUploadItem/min_values_marshal/unmarshal
=== PAUSE TestUploadItem/min_values_marshal/unmarshal
=== RUN   TestUploadItem/min_values_clone
=== PAUSE TestUploadItem/min_values_clone
=== RUN   TestUploadItem/max_values_marshal/unmarshal
=== PAUSE TestUploadItem/max_values_marshal/unmarshal
=== RUN   TestUploadItem/max_values_clone
=== PAUSE TestUploadItem/max_values_clone
=== RUN   TestUploadItem/random_values_marshal/unmarshal
=== PAUSE TestUploadItem/random_values_marshal/unmarshal
=== RUN   TestUploadItem/random_values_clone
=== PAUSE TestUploadItem/random_values_clone
=== RUN   TestUploadItem/invalid_size_marshal/unmarshal
=== PAUSE TestUploadItem/invalid_size_marshal/unmarshal
=== RUN   TestUploadItem/invalid_size_clone
=== PAUSE TestUploadItem/invalid_size_clone
=== CONT  TestNextTagID
--- PASS: TestNextTagID (0.00s)
=== CONT  TestPushItem/invalid_size_clone
=== CONT  TestPushItem/invalid_size_marshal/unmarshal
=== CONT  TestPushItem/random_values_clone
=== RUN   TestChunkReporter/chunk_aa09d9f512e7643830f38d850a474da60ec35140ce29be75901f8a5d4f618523
=== RUN   TestChunkReporter/chunk_aa09d9f512e7643830f38d850a474da60ec35140ce29be75901f8a5d4f618523/mark_sent
=== CONT  TestPushItem/random_values_marshal/unmarshal
=== RUN   TestChunkReporter/chunk_aa09d9f512e7643830f38d850a474da60ec35140ce29be75901f8a5d4f618523/mark_stored
=== RUN   TestChunkReporter/chunk_aa09d9f512e7643830f38d850a474da60ec35140ce29be75901f8a5d4f618523/mark_synced
=== RUN   TestChunkReporter/chunk_aa09d9f512e7643830f38d850a474da60ec35140ce29be75901f8a5d4f618523/verify_internal_state
=== CONT  TestPushItem/max_values_clone
=== CONT  TestPushItem/zero_values_marshal/unmarshal
=== CONT  TestIterate
=== RUN   TestIterate/on_empty_storage_does_not_call_the_callback_fn
=== RUN   TestIterate/iterates_chunks
=== CONT  TestPushItem/max_values_marshal/unmarshal
=== CONT  TestItemNextTagID
=== CONT  TestChunkPutter
=== RUN   TestTagItem/zero_values_clone
=== CONT  TestPushItem/min_values_clone
=== CONT  TestPushItem/min_values_marshal/unmarshal
=== RUN   TestChunkReporter/chunk_203fa6fee5229e6cdb9396f6d2a9115bdcfeb7822d677572e4e00a5e5e655e70
=== RUN   TestStampIndexHandling/put_existing_chunk_with_newer_batch_timestamp
=== RUN   TestChunkReporter/chunk_203fa6fee5229e6cdb9396f6d2a9115bdcfeb7822d677572e4e00a5e5e655e70/mark_sent
=== RUN   TestChunkReporter/chunk_203fa6fee5229e6cdb9396f6d2a9115bdcfeb7822d677572e4e00a5e5e655e70/mark_stored
=== RUN   TestChunkReporter/chunk_203fa6fee5229e6cdb9396f6d2a9115bdcfeb7822d677572e4e00a5e5e655e70/mark_synced
=== RUN   TestItemNextTagID/zero_values_marshal/unmarshal
=== PAUSE TestItemNextTagID/zero_values_marshal/unmarshal
=== RUN   TestItemNextTagID/zero_values_clone
=== PAUSE TestItemNextTagID/zero_values_clone
=== RUN   TestItemNextTagID/max_value_marshal/unmarshal
=== PAUSE TestItemNextTagID/max_value_marshal/unmarshal
=== RUN   TestItemNextTagID/max_value_clone
=== RUN   TestChunkReporter/chunk_203fa6fee5229e6cdb9396f6d2a9115bdcfeb7822d677572e4e00a5e5e655e70/verify_internal_state
=== PAUSE TestItemNextTagID/max_value_clone
=== RUN   TestItemNextTagID/invalid_size_marshal/unmarshal
=== PAUSE TestItemNextTagID/invalid_size_marshal/unmarshal
=== RUN   TestItemNextTagID/invalid_size_clone
=== PAUSE TestItemNextTagID/invalid_size_clone
=== CONT  TestPushItem/nil_stamp_clone
=== CONT  TestPushItem/zero_address_marshal/unmarshal
=== CONT  TestPushItem/zero_values_clone
=== CONT  TestPushItem/zero_address_clone
=== RUN   TestChunkReporter/chunk_dcc73ac3732c923d9e162c7ee4364c33b4a786b2eb84b4a1bc8be003514cc9df
=== CONT  TestUploadItem/zero_values_marshal/unmarshal
=== CONT  TestUploadItem/invalid_size_clone
=== CONT  TestUploadItem/invalid_size_marshal/unmarshal
=== CONT  TestUploadItem/random_values_clone
=== RUN   TestChunkReporter/chunk_dcc73ac3732c923d9e162c7ee4364c33b4a786b2eb84b4a1bc8be003514cc9df/mark_sent
=== RUN   TestChunkReporter/chunk_dcc73ac3732c923d9e162c7ee4364c33b4a786b2eb84b4a1bc8be003514cc9df/mark_stored
=== RUN   TestChunkReporter/chunk_dcc73ac3732c923d9e162c7ee4364c33b4a786b2eb84b4a1bc8be003514cc9df/mark_synced
=== RUN   TestChunkReporter/chunk_dcc73ac3732c923d9e162c7ee4364c33b4a786b2eb84b4a1bc8be003514cc9df/verify_internal_state
=== CONT  TestUploadItem/random_values_marshal/unmarshal
=== CONT  TestUploadItem/max_values_clone
=== RUN   TestChunkReporter/chunk_26ed320d1a5813ea83b50bea254674454a3294a8a0fb4ebbb5ae756a9efbf6b7
=== RUN   TestChunkReporter/chunk_26ed320d1a5813ea83b50bea254674454a3294a8a0fb4ebbb5ae756a9efbf6b7/mark_sent
=== RUN   TestChunkReporter/chunk_26ed320d1a5813ea83b50bea254674454a3294a8a0fb4ebbb5ae756a9efbf6b7/mark_stored
=== CONT  TestUploadItem/max_values_marshal/unmarshal
=== RUN   TestChunkPutter/chunk_6291e4f9927c031f688c7e65ce0aec6aafc90155ce2e154c945abc023113de7d
=== RUN   TestChunkReporter/chunk_26ed320d1a5813ea83b50bea254674454a3294a8a0fb4ebbb5ae756a9efbf6b7/mark_synced
--- PASS: TestStampIndexHandling (0.00s)
    --- PASS: TestStampIndexHandling/put_chunk_with_immutable_batch (0.00s)
    --- PASS: TestStampIndexHandling/put_existing_index_with_older_batch_timestamp (0.00s)
    --- PASS: TestStampIndexHandling/put_existing_chunk_with_newer_batch_timestamp (0.00s)
=== CONT  TestUploadItem/min_values_marshal/unmarshal
=== RUN   TestChunkPutter/chunk_6291e4f9927c031f688c7e65ce0aec6aafc90155ce2e154c945abc023113de7d/put_new_chunk
=== RUN   TestChunkReporter/chunk_26ed320d1a5813ea83b50bea254674454a3294a8a0fb4ebbb5ae756a9efbf6b7/verify_internal_state
=== CONT  TestUploadItem/nil_stamp_clone
=== RUN   TestChunkPutter/chunk_6291e4f9927c031f688c7e65ce0aec6aafc90155ce2e154c945abc023113de7d/put_existing_chunk
=== CONT  TestUploadItem/nil_stamp_marshal/unmarshal
=== CONT  TestUploadItem/zero_address_clone
=== RUN   TestChunkPutter/chunk_6291e4f9927c031f688c7e65ce0aec6aafc90155ce2e154c945abc023113de7d/verify_internal_state
=== CONT  TestUploadItem/zero_address_marshal/unmarshal
=== CONT  TestUploadItem/zero_values_clone
=== RUN   TestChunkReporter/chunk_a633a3fd532fcf032aaefbcbe903794dd0cb59d69ab9eea71db96d820c13dbbb
=== CONT  TestItemNextTagID/zero_values_marshal/unmarshal
=== CONT  TestItemNextTagID/invalid_size_clone
=== RUN   TestChunkReporter/chunk_a633a3fd532fcf032aaefbcbe903794dd0cb59d69ab9eea71db96d820c13dbbb/mark_sent
=== CONT  TestItemNextTagID/invalid_size_marshal/unmarshal
=== PAUSE TestTagItem/zero_values_clone
=== CONT  TestPushItem/nil_stamp_marshal/unmarshal
=== RUN   TestTagItem/max_values_marshal/unmarshal
=== PAUSE TestTagItem/max_values_marshal/unmarshal
=== CONT  TestItemNextTagID/max_value_clone
=== CONT  TestItemNextTagID/zero_values_clone
=== RUN   TestTagItem/max_values_clone
=== PAUSE TestTagItem/max_values_clone
=== RUN   TestTagItem/random_values_marshal/unmarshal
=== PAUSE TestTagItem/random_values_marshal/unmarshal
=== RUN   TestTagItem/random_values_clone
--- PASS: TestPushItem (0.00s)
    --- PASS: TestPushItem/invalid_size_clone (0.00s)
    --- PASS: TestPushItem/invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestPushItem/random_values_clone (0.00s)
    --- PASS: TestPushItem/random_values_marshal/unmarshal (0.00s)
    --- PASS: TestPushItem/zero_values_marshal/unmarshal (0.00s)
    --- PASS: TestPushItem/min_values_clone (0.00s)
    --- PASS: TestPushItem/max_values_clone (0.00s)
    --- PASS: TestPushItem/min_values_marshal/unmarshal (0.00s)
    --- PASS: TestPushItem/max_values_marshal/unmarshal (0.00s)
    --- PASS: TestPushItem/nil_stamp_clone (0.00s)
    --- PASS: TestPushItem/zero_address_marshal/unmarshal (0.00s)
    --- PASS: TestPushItem/zero_values_clone (0.00s)
    --- PASS: TestPushItem/zero_address_clone (0.00s)
    --- PASS: TestPushItem/nil_stamp_marshal/unmarshal (0.00s)
=== RUN   TestChunkReporter/chunk_a633a3fd532fcf032aaefbcbe903794dd0cb59d69ab9eea71db96d820c13dbbb/mark_stored
=== RUN   TestChunkReporter/chunk_a633a3fd532fcf032aaefbcbe903794dd0cb59d69ab9eea71db96d820c13dbbb/mark_synced
=== RUN   TestChunkReporter/chunk_a633a3fd532fcf032aaefbcbe903794dd0cb59d69ab9eea71db96d820c13dbbb/verify_internal_state
=== RUN   TestChunkReporter/chunk_4b1f9feaaf63451c41d4a9afeba2e016e32403f055843aeea492913263e77d4a
=== CONT  TestUploadItem/min_values_clone
--- PASS: TestIterate (0.00s)
    --- PASS: TestIterate/on_empty_storage_does_not_call_the_callback_fn (0.00s)
    --- PASS: TestIterate/iterates_chunks (0.00s)
=== RUN   TestChunkReporter/chunk_4b1f9feaaf63451c41d4a9afeba2e016e32403f055843aeea492913263e77d4a/mark_sent
=== RUN   TestChunkReporter/chunk_4b1f9feaaf63451c41d4a9afeba2e016e32403f055843aeea492913263e77d4a/mark_stored
=== RUN   TestChunkReporter/chunk_4b1f9feaaf63451c41d4a9afeba2e016e32403f055843aeea492913263e77d4a/mark_synced
=== RUN   TestChunkReporter/chunk_4b1f9feaaf63451c41d4a9afeba2e016e32403f055843aeea492913263e77d4a/verify_internal_state
=== RUN   TestChunkPutter/chunk_29ab0bee85f823e022369176afef0258812c1eeb2191fae17175e86cc1b74146
=== RUN   TestChunkPutter/chunk_29ab0bee85f823e022369176afef0258812c1eeb2191fae17175e86cc1b74146/put_new_chunk
=== CONT  TestItemNextTagID/max_value_marshal/unmarshal
--- PASS: TestItemNextTagID (0.00s)
    --- PASS: TestItemNextTagID/zero_values_marshal/unmarshal (0.00s)
    --- PASS: TestItemNextTagID/invalid_size_clone (0.00s)
    --- PASS: TestItemNextTagID/invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestItemNextTagID/max_value_clone (0.00s)
    --- PASS: TestItemNextTagID/zero_values_clone (0.00s)
    --- PASS: TestItemNextTagID/max_value_marshal/unmarshal (0.00s)
=== PAUSE TestTagItem/random_values_clone
=== RUN   TestChunkPutter/chunk_29ab0bee85f823e022369176afef0258812c1eeb2191fae17175e86cc1b74146/put_existing_chunk
=== RUN   TestTagItem/invalid_size_marshal/unmarshal
=== RUN   TestChunkReporter/chunk_fb2c7b420b86feb5fda73a0cd6eca0d2dd323b455fd1670501b3ab3ee6cb3a34
=== PAUSE TestTagItem/invalid_size_marshal/unmarshal
=== RUN   TestChunkPutter/chunk_29ab0bee85f823e022369176afef0258812c1eeb2191fae17175e86cc1b74146/verify_internal_state
=== RUN   TestTagItem/invalid_size_clone
=== PAUSE TestTagItem/invalid_size_clone
=== CONT  TestTagItem/zero_values_marshal/unmarshal
=== RUN   TestChunkReporter/chunk_fb2c7b420b86feb5fda73a0cd6eca0d2dd323b455fd1670501b3ab3ee6cb3a34/mark_sent
=== RUN   TestChunkReporter/chunk_fb2c7b420b86feb5fda73a0cd6eca0d2dd323b455fd1670501b3ab3ee6cb3a34/mark_stored
=== RUN   TestChunkReporter/chunk_fb2c7b420b86feb5fda73a0cd6eca0d2dd323b455fd1670501b3ab3ee6cb3a34/mark_synced
=== CONT  TestTagItem/invalid_size_clone
=== RUN   TestChunkReporter/chunk_fb2c7b420b86feb5fda73a0cd6eca0d2dd323b455fd1670501b3ab3ee6cb3a34/verify_internal_state
=== CONT  TestTagItem/invalid_size_marshal/unmarshal
=== CONT  TestTagItem/random_values_clone
=== CONT  TestTagItem/random_values_marshal/unmarshal
=== CONT  TestTagItem/max_values_clone
=== RUN   TestChunkPutter/chunk_f9679b07d588c9c6fdd8b321d56098dc92abb0abb50f181010ba0b46f40cf66d
=== RUN   TestChunkPutter/chunk_f9679b07d588c9c6fdd8b321d56098dc92abb0abb50f181010ba0b46f40cf66d/put_new_chunk
=== RUN   TestChunkReporter/chunk_404ce9ffc48243b15c5f793db2260ffb34ee41ad550fab91d71ceb504c8e907a
=== CONT  TestTagItem/max_values_marshal/unmarshal
--- PASS: TestUploadItem (0.00s)
    --- PASS: TestUploadItem/invalid_size_clone (0.00s)
    --- PASS: TestUploadItem/invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestUploadItem/random_values_clone (0.00s)
    --- PASS: TestUploadItem/random_values_marshal/unmarshal (0.00s)
    --- PASS: TestUploadItem/max_values_clone (0.00s)
    --- PASS: TestUploadItem/zero_values_marshal/unmarshal (0.00s)
    --- PASS: TestUploadItem/max_values_marshal/unmarshal (0.00s)
    --- PASS: TestUploadItem/nil_stamp_clone (0.00s)
    --- PASS: TestUploadItem/nil_stamp_marshal/unmarshal (0.00s)
    --- PASS: TestUploadItem/zero_address_clone (0.00s)
    --- PASS: TestUploadItem/zero_address_marshal/unmarshal (0.00s)
    --- PASS: TestUploadItem/zero_values_clone (0.00s)
    --- PASS: TestUploadItem/min_values_clone (0.00s)
    --- PASS: TestUploadItem/min_values_marshal/unmarshal (0.00s)
=== RUN   TestChunkPutter/chunk_f9679b07d588c9c6fdd8b321d56098dc92abb0abb50f181010ba0b46f40cf66d/put_existing_chunk
=== CONT  TestTagItem/zero_values_clone
=== RUN   TestChunkPutter/chunk_f9679b07d588c9c6fdd8b321d56098dc92abb0abb50f181010ba0b46f40cf66d/verify_internal_state
--- PASS: TestTagItem (0.00s)
    --- PASS: TestTagItem/zero_values_marshal/unmarshal (0.00s)
    --- PASS: TestTagItem/invalid_size_clone (0.00s)
    --- PASS: TestTagItem/invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestTagItem/random_values_clone (0.00s)
    --- PASS: TestTagItem/random_values_marshal/unmarshal (0.00s)
    --- PASS: TestTagItem/max_values_clone (0.00s)
    --- PASS: TestTagItem/max_values_marshal/unmarshal (0.00s)
    --- PASS: TestTagItem/zero_values_clone (0.00s)
=== RUN   TestChunkReporter/chunk_404ce9ffc48243b15c5f793db2260ffb34ee41ad550fab91d71ceb504c8e907a/mark_sent
=== RUN   TestChunkReporter/chunk_404ce9ffc48243b15c5f793db2260ffb34ee41ad550fab91d71ceb504c8e907a/mark_stored
=== RUN   TestChunkReporter/chunk_404ce9ffc48243b15c5f793db2260ffb34ee41ad550fab91d71ceb504c8e907a/mark_synced
=== RUN   TestChunkReporter/chunk_404ce9ffc48243b15c5f793db2260ffb34ee41ad550fab91d71ceb504c8e907a/verify_internal_state
=== RUN   TestChunkReporter/close_with_reference
=== RUN   TestChunkPutter/chunk_de2df8b7213e782fdea156b15b1f321f5f58483a7f955b98b61fb70858f84bb1
=== RUN   TestChunkPutter/chunk_de2df8b7213e782fdea156b15b1f321f5f58483a7f955b98b61fb70858f84bb1/put_new_chunk
--- PASS: TestChunkReporter (0.01s)
    --- PASS: TestChunkReporter/chunk_9193c850ab9d90c5f01985bb880e0364512fab9e42256bdfdf056fd612126808 (0.00s)
        --- PASS: TestChunkReporter/chunk_9193c850ab9d90c5f01985bb880e0364512fab9e42256bdfdf056fd612126808/mark_sent (0.00s)
        --- PASS: TestChunkReporter/chunk_9193c850ab9d90c5f01985bb880e0364512fab9e42256bdfdf056fd612126808/mark_stored (0.00s)
        --- PASS: TestChunkReporter/chunk_9193c850ab9d90c5f01985bb880e0364512fab9e42256bdfdf056fd612126808/mark_synced (0.00s)
        --- PASS: TestChunkReporter/chunk_9193c850ab9d90c5f01985bb880e0364512fab9e42256bdfdf056fd612126808/verify_internal_state (0.00s)
    --- PASS: TestChunkReporter/chunk_2447e20eb2b7e9c130e2c85b9fc6327da23cb04d6b4abdec1c0260a6c9c85de1 (0.00s)
        --- PASS: TestChunkReporter/chunk_2447e20eb2b7e9c130e2c85b9fc6327da23cb04d6b4abdec1c0260a6c9c85de1/mark_sent (0.00s)
        --- PASS: TestChunkReporter/chunk_2447e20eb2b7e9c130e2c85b9fc6327da23cb04d6b4abdec1c0260a6c9c85de1/mark_stored (0.00s)
        --- PASS: TestChunkReporter/chunk_2447e20eb2b7e9c130e2c85b9fc6327da23cb04d6b4abdec1c0260a6c9c85de1/mark_synced (0.00s)
        --- PASS: TestChunkReporter/chunk_2447e20eb2b7e9c130e2c85b9fc6327da23cb04d6b4abdec1c0260a6c9c85de1/verify_internal_state (0.00s)
    --- PASS: TestChunkReporter/chunk_aa09d9f512e7643830f38d850a474da60ec35140ce29be75901f8a5d4f618523 (0.00s)
        --- PASS: TestChunkReporter/chunk_aa09d9f512e7643830f38d850a474da60ec35140ce29be75901f8a5d4f618523/mark_sent (0.00s)
        --- PASS: TestChunkReporter/chunk_aa09d9f512e7643830f38d850a474da60ec35140ce29be75901f8a5d4f618523/mark_stored (0.00s)
        --- PASS: TestChunkReporter/chunk_aa09d9f512e7643830f38d850a474da60ec35140ce29be75901f8a5d4f618523/mark_synced (0.00s)
        --- PASS: TestChunkReporter/chunk_aa09d9f512e7643830f38d850a474da60ec35140ce29be75901f8a5d4f618523/verify_internal_state (0.00s)
    --- PASS: TestChunkReporter/chunk_203fa6fee5229e6cdb9396f6d2a9115bdcfeb7822d677572e4e00a5e5e655e70 (0.00s)
        --- PASS: TestChunkReporter/chunk_203fa6fee5229e6cdb9396f6d2a9115bdcfeb7822d677572e4e00a5e5e655e70/mark_sent (0.00s)
        --- PASS: TestChunkReporter/chunk_203fa6fee5229e6cdb9396f6d2a9115bdcfeb7822d677572e4e00a5e5e655e70/mark_stored (0.00s)
        --- PASS: TestChunkReporter/chunk_203fa6fee5229e6cdb9396f6d2a9115bdcfeb7822d677572e4e00a5e5e655e70/mark_synced (0.00s)
        --- PASS: TestChunkReporter/chunk_203fa6fee5229e6cdb9396f6d2a9115bdcfeb7822d677572e4e00a5e5e655e70/verify_internal_state (0.00s)
    --- PASS: TestChunkReporter/chunk_dcc73ac3732c923d9e162c7ee4364c33b4a786b2eb84b4a1bc8be003514cc9df (0.00s)
        --- PASS: TestChunkReporter/chunk_dcc73ac3732c923d9e162c7ee4364c33b4a786b2eb84b4a1bc8be003514cc9df/mark_sent (0.00s)
        --- PASS: TestChunkReporter/chunk_dcc73ac3732c923d9e162c7ee4364c33b4a786b2eb84b4a1bc8be003514cc9df/mark_stored (0.00s)
        --- PASS: TestChunkReporter/chunk_dcc73ac3732c923d9e162c7ee4364c33b4a786b2eb84b4a1bc8be003514cc9df/mark_synced (0.00s)
        --- PASS: TestChunkReporter/chunk_dcc73ac3732c923d9e162c7ee4364c33b4a786b2eb84b4a1bc8be003514cc9df/verify_internal_state (0.00s)
    --- PASS: TestChunkReporter/chunk_26ed320d1a5813ea83b50bea254674454a3294a8a0fb4ebbb5ae756a9efbf6b7 (0.00s)
        --- PASS: TestChunkReporter/chunk_26ed320d1a5813ea83b50bea254674454a3294a8a0fb4ebbb5ae756a9efbf6b7/mark_sent (0.00s)
        --- PASS: TestChunkReporter/chunk_26ed320d1a5813ea83b50bea254674454a3294a8a0fb4ebbb5ae756a9efbf6b7/mark_stored (0.00s)
        --- PASS: TestChunkReporter/chunk_26ed320d1a5813ea83b50bea254674454a3294a8a0fb4ebbb5ae756a9efbf6b7/mark_synced (0.00s)
        --- PASS: TestChunkReporter/chunk_26ed320d1a5813ea83b50bea254674454a3294a8a0fb4ebbb5ae756a9efbf6b7/verify_internal_state (0.00s)
    --- PASS: TestChunkReporter/chunk_a633a3fd532fcf032aaefbcbe903794dd0cb59d69ab9eea71db96d820c13dbbb (0.00s)
        --- PASS: TestChunkReporter/chunk_a633a3fd532fcf032aaefbcbe903794dd0cb59d69ab9eea71db96d820c13dbbb/mark_sent (0.00s)
        --- PASS: TestChunkReporter/chunk_a633a3fd532fcf032aaefbcbe903794dd0cb59d69ab9eea71db96d820c13dbbb/mark_stored (0.00s)
        --- PASS: TestChunkReporter/chunk_a633a3fd532fcf032aaefbcbe903794dd0cb59d69ab9eea71db96d820c13dbbb/mark_synced (0.00s)
        --- PASS: TestChunkReporter/chunk_a633a3fd532fcf032aaefbcbe903794dd0cb59d69ab9eea71db96d820c13dbbb/verify_internal_state (0.00s)
    --- PASS: TestChunkReporter/chunk_4b1f9feaaf63451c41d4a9afeba2e016e32403f055843aeea492913263e77d4a (0.00s)
        --- PASS: TestChunkReporter/chunk_4b1f9feaaf63451c41d4a9afeba2e016e32403f055843aeea492913263e77d4a/mark_sent (0.00s)
        --- PASS: TestChunkReporter/chunk_4b1f9feaaf63451c41d4a9afeba2e016e32403f055843aeea492913263e77d4a/mark_stored (0.00s)
        --- PASS: TestChunkReporter/chunk_4b1f9feaaf63451c41d4a9afeba2e016e32403f055843aeea492913263e77d4a/mark_synced (0.00s)
        --- PASS: TestChunkReporter/chunk_4b1f9feaaf63451c41d4a9afeba2e016e32403f055843aeea492913263e77d4a/verify_internal_state (0.00s)
    --- PASS: TestChunkReporter/chunk_fb2c7b420b86feb5fda73a0cd6eca0d2dd323b455fd1670501b3ab3ee6cb3a34 (0.00s)
        --- PASS: TestChunkReporter/chunk_fb2c7b420b86feb5fda73a0cd6eca0d2dd323b455fd1670501b3ab3ee6cb3a34/mark_sent (0.00s)
        --- PASS: TestChunkReporter/chunk_fb2c7b420b86feb5fda73a0cd6eca0d2dd323b455fd1670501b3ab3ee6cb3a34/mark_stored (0.00s)
        --- PASS: TestChunkReporter/chunk_fb2c7b420b86feb5fda73a0cd6eca0d2dd323b455fd1670501b3ab3ee6cb3a34/mark_synced (0.00s)
        --- PASS: TestChunkReporter/chunk_fb2c7b420b86feb5fda73a0cd6eca0d2dd323b455fd1670501b3ab3ee6cb3a34/verify_internal_state (0.00s)
    --- PASS: TestChunkReporter/chunk_404ce9ffc48243b15c5f793db2260ffb34ee41ad550fab91d71ceb504c8e907a (0.00s)
        --- PASS: TestChunkReporter/chunk_404ce9ffc48243b15c5f793db2260ffb34ee41ad550fab91d71ceb504c8e907a/mark_sent (0.00s)
        --- PASS: TestChunkReporter/chunk_404ce9ffc48243b15c5f793db2260ffb34ee41ad550fab91d71ceb504c8e907a/mark_stored (0.00s)
        --- PASS: TestChunkReporter/chunk_404ce9ffc48243b15c5f793db2260ffb34ee41ad550fab91d71ceb504c8e907a/mark_synced (0.00s)
        --- PASS: TestChunkReporter/chunk_404ce9ffc48243b15c5f793db2260ffb34ee41ad550fab91d71ceb504c8e907a/verify_internal_state (0.00s)
    --- PASS: TestChunkReporter/close_with_reference (0.00s)
=== RUN   TestChunkPutter/chunk_de2df8b7213e782fdea156b15b1f321f5f58483a7f955b98b61fb70858f84bb1/put_existing_chunk
=== RUN   TestChunkPutter/chunk_de2df8b7213e782fdea156b15b1f321f5f58483a7f955b98b61fb70858f84bb1/verify_internal_state
=== RUN   TestChunkPutter/chunk_138e8ff43450b931e7bfbd8695ecce848b6364b9159eb7d69aa7b75577de4389
=== RUN   TestChunkPutter/chunk_138e8ff43450b931e7bfbd8695ecce848b6364b9159eb7d69aa7b75577de4389/put_new_chunk
=== RUN   TestChunkPutter/chunk_138e8ff43450b931e7bfbd8695ecce848b6364b9159eb7d69aa7b75577de4389/put_existing_chunk
=== RUN   TestChunkPutter/chunk_138e8ff43450b931e7bfbd8695ecce848b6364b9159eb7d69aa7b75577de4389/verify_internal_state
=== RUN   TestChunkPutter/chunk_b29a02e3c3aa0080a9d30a59b8b923ebd0bab0af8a2eb57c5d376922b77fa1d4
=== RUN   TestChunkPutter/chunk_b29a02e3c3aa0080a9d30a59b8b923ebd0bab0af8a2eb57c5d376922b77fa1d4/put_new_chunk
=== RUN   TestChunkPutter/chunk_b29a02e3c3aa0080a9d30a59b8b923ebd0bab0af8a2eb57c5d376922b77fa1d4/put_existing_chunk
=== RUN   TestChunkPutter/chunk_b29a02e3c3aa0080a9d30a59b8b923ebd0bab0af8a2eb57c5d376922b77fa1d4/verify_internal_state
=== RUN   TestChunkPutter/chunk_6164c7383c53b8ea9146c55ace422c3b010bc6581ef531eb7ead3b9a58486520
=== RUN   TestChunkPutter/chunk_6164c7383c53b8ea9146c55ace422c3b010bc6581ef531eb7ead3b9a58486520/put_new_chunk
=== RUN   TestChunkPutter/chunk_6164c7383c53b8ea9146c55ace422c3b010bc6581ef531eb7ead3b9a58486520/put_existing_chunk
=== RUN   TestChunkPutter/chunk_6164c7383c53b8ea9146c55ace422c3b010bc6581ef531eb7ead3b9a58486520/verify_internal_state
=== RUN   TestChunkPutter/chunk_2d01b6527cda17c43cc578914eace0e396e15e504bfea9dc40d47f7e066c2b6d
=== RUN   TestChunkPutter/chunk_2d01b6527cda17c43cc578914eace0e396e15e504bfea9dc40d47f7e066c2b6d/put_new_chunk
=== RUN   TestChunkPutter/chunk_2d01b6527cda17c43cc578914eace0e396e15e504bfea9dc40d47f7e066c2b6d/put_existing_chunk
=== RUN   TestChunkPutter/chunk_2d01b6527cda17c43cc578914eace0e396e15e504bfea9dc40d47f7e066c2b6d/verify_internal_state
=== RUN   TestChunkPutter/chunk_318697a0a5bc1e500990506e55147caa0ea15fa37299d3990cfca3fb455fde9b
=== RUN   TestChunkPutter/chunk_318697a0a5bc1e500990506e55147caa0ea15fa37299d3990cfca3fb455fde9b/put_new_chunk
=== RUN   TestChunkPutter/chunk_318697a0a5bc1e500990506e55147caa0ea15fa37299d3990cfca3fb455fde9b/put_existing_chunk
=== RUN   TestChunkPutter/chunk_318697a0a5bc1e500990506e55147caa0ea15fa37299d3990cfca3fb455fde9b/verify_internal_state
=== RUN   TestChunkPutter/chunk_f2c97565d0550cae68a8e65b31fc22ca653a84405ecaf3252c99b55089ade31e
=== RUN   TestChunkPutter/chunk_f2c97565d0550cae68a8e65b31fc22ca653a84405ecaf3252c99b55089ade31e/put_new_chunk
=== RUN   TestChunkPutter/chunk_f2c97565d0550cae68a8e65b31fc22ca653a84405ecaf3252c99b55089ade31e/put_existing_chunk
=== RUN   TestChunkPutter/chunk_f2c97565d0550cae68a8e65b31fc22ca653a84405ecaf3252c99b55089ade31e/verify_internal_state
=== RUN   TestChunkPutter/close_with_reference
=== RUN   TestChunkPutter/error_after_close
--- PASS: TestChunkPutter (0.00s)
    --- PASS: TestChunkPutter/chunk_6291e4f9927c031f688c7e65ce0aec6aafc90155ce2e154c945abc023113de7d (0.00s)
        --- PASS: TestChunkPutter/chunk_6291e4f9927c031f688c7e65ce0aec6aafc90155ce2e154c945abc023113de7d/put_new_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_6291e4f9927c031f688c7e65ce0aec6aafc90155ce2e154c945abc023113de7d/put_existing_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_6291e4f9927c031f688c7e65ce0aec6aafc90155ce2e154c945abc023113de7d/verify_internal_state (0.00s)
    --- PASS: TestChunkPutter/chunk_29ab0bee85f823e022369176afef0258812c1eeb2191fae17175e86cc1b74146 (0.00s)
        --- PASS: TestChunkPutter/chunk_29ab0bee85f823e022369176afef0258812c1eeb2191fae17175e86cc1b74146/put_new_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_29ab0bee85f823e022369176afef0258812c1eeb2191fae17175e86cc1b74146/put_existing_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_29ab0bee85f823e022369176afef0258812c1eeb2191fae17175e86cc1b74146/verify_internal_state (0.00s)
    --- PASS: TestChunkPutter/chunk_f9679b07d588c9c6fdd8b321d56098dc92abb0abb50f181010ba0b46f40cf66d (0.00s)
        --- PASS: TestChunkPutter/chunk_f9679b07d588c9c6fdd8b321d56098dc92abb0abb50f181010ba0b46f40cf66d/put_new_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_f9679b07d588c9c6fdd8b321d56098dc92abb0abb50f181010ba0b46f40cf66d/put_existing_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_f9679b07d588c9c6fdd8b321d56098dc92abb0abb50f181010ba0b46f40cf66d/verify_internal_state (0.00s)
    --- PASS: TestChunkPutter/chunk_de2df8b7213e782fdea156b15b1f321f5f58483a7f955b98b61fb70858f84bb1 (0.00s)
        --- PASS: TestChunkPutter/chunk_de2df8b7213e782fdea156b15b1f321f5f58483a7f955b98b61fb70858f84bb1/put_new_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_de2df8b7213e782fdea156b15b1f321f5f58483a7f955b98b61fb70858f84bb1/put_existing_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_de2df8b7213e782fdea156b15b1f321f5f58483a7f955b98b61fb70858f84bb1/verify_internal_state (0.00s)
    --- PASS: TestChunkPutter/chunk_138e8ff43450b931e7bfbd8695ecce848b6364b9159eb7d69aa7b75577de4389 (0.00s)
        --- PASS: TestChunkPutter/chunk_138e8ff43450b931e7bfbd8695ecce848b6364b9159eb7d69aa7b75577de4389/put_new_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_138e8ff43450b931e7bfbd8695ecce848b6364b9159eb7d69aa7b75577de4389/put_existing_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_138e8ff43450b931e7bfbd8695ecce848b6364b9159eb7d69aa7b75577de4389/verify_internal_state (0.00s)
    --- PASS: TestChunkPutter/chunk_b29a02e3c3aa0080a9d30a59b8b923ebd0bab0af8a2eb57c5d376922b77fa1d4 (0.00s)
        --- PASS: TestChunkPutter/chunk_b29a02e3c3aa0080a9d30a59b8b923ebd0bab0af8a2eb57c5d376922b77fa1d4/put_new_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_b29a02e3c3aa0080a9d30a59b8b923ebd0bab0af8a2eb57c5d376922b77fa1d4/put_existing_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_b29a02e3c3aa0080a9d30a59b8b923ebd0bab0af8a2eb57c5d376922b77fa1d4/verify_internal_state (0.00s)
    --- PASS: TestChunkPutter/chunk_6164c7383c53b8ea9146c55ace422c3b010bc6581ef531eb7ead3b9a58486520 (0.00s)
        --- PASS: TestChunkPutter/chunk_6164c7383c53b8ea9146c55ace422c3b010bc6581ef531eb7ead3b9a58486520/put_new_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_6164c7383c53b8ea9146c55ace422c3b010bc6581ef531eb7ead3b9a58486520/put_existing_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_6164c7383c53b8ea9146c55ace422c3b010bc6581ef531eb7ead3b9a58486520/verify_internal_state (0.00s)
    --- PASS: TestChunkPutter/chunk_2d01b6527cda17c43cc578914eace0e396e15e504bfea9dc40d47f7e066c2b6d (0.00s)
        --- PASS: TestChunkPutter/chunk_2d01b6527cda17c43cc578914eace0e396e15e504bfea9dc40d47f7e066c2b6d/put_new_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_2d01b6527cda17c43cc578914eace0e396e15e504bfea9dc40d47f7e066c2b6d/put_existing_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_2d01b6527cda17c43cc578914eace0e396e15e504bfea9dc40d47f7e066c2b6d/verify_internal_state (0.00s)
    --- PASS: TestChunkPutter/chunk_318697a0a5bc1e500990506e55147caa0ea15fa37299d3990cfca3fb455fde9b (0.00s)
        --- PASS: TestChunkPutter/chunk_318697a0a5bc1e500990506e55147caa0ea15fa37299d3990cfca3fb455fde9b/put_new_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_318697a0a5bc1e500990506e55147caa0ea15fa37299d3990cfca3fb455fde9b/put_existing_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_318697a0a5bc1e500990506e55147caa0ea15fa37299d3990cfca3fb455fde9b/verify_internal_state (0.00s)
    --- PASS: TestChunkPutter/chunk_f2c97565d0550cae68a8e65b31fc22ca653a84405ecaf3252c99b55089ade31e (0.00s)
        --- PASS: TestChunkPutter/chunk_f2c97565d0550cae68a8e65b31fc22ca653a84405ecaf3252c99b55089ade31e/put_new_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_f2c97565d0550cae68a8e65b31fc22ca653a84405ecaf3252c99b55089ade31e/put_existing_chunk (0.00s)
        --- PASS: TestChunkPutter/chunk_f2c97565d0550cae68a8e65b31fc22ca653a84405ecaf3252c99b55089ade31e/verify_internal_state (0.00s)
    --- PASS: TestChunkPutter/close_with_reference (0.00s)
    --- PASS: TestChunkPutter/error_after_close (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/localstorev2/internal/upload	(cached)
=== RUN   TestAllSteps
=== PAUSE TestAllSteps
=== RUN   Test_Step_01
=== PAUSE Test_Step_01
=== CONT  TestAllSteps
=== RUN   TestAllSteps/version_numbers
=== PAUSE TestAllSteps/version_numbers
=== RUN   TestAllSteps/zero_store_migration
=== PAUSE TestAllSteps/zero_store_migration
=== CONT  TestAllSteps/version_numbers
=== CONT  Test_Step_01
--- PASS: Test_Step_01 (0.00s)
=== CONT  TestAllSteps/zero_store_migration
--- PASS: TestAllSteps (0.00s)
    --- PASS: TestAllSteps/version_numbers (0.00s)
    --- PASS: TestAllSteps/zero_store_migration (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/localstorev2/migration	(cached)
=== RUN   TestPretty
=== RUN   TestPretty/#00
=== RUN   TestPretty/#01
=== RUN   TestPretty/#02
=== RUN   TestPretty/#03
=== RUN   TestPretty/#04
=== RUN   TestPretty/#05
=== RUN   TestPretty/#06
=== RUN   TestPretty/#07
=== RUN   TestPretty/#08
=== RUN   TestPretty/#09
=== RUN   TestPretty/#10
=== RUN   TestPretty/#11
=== RUN   TestPretty/#12
=== RUN   TestPretty/#13
=== RUN   TestPretty/#14
=== RUN   TestPretty/#15
=== RUN   TestPretty/#16
=== RUN   TestPretty/#17
=== RUN   TestPretty/#18
=== RUN   TestPretty/#19
=== RUN   TestPretty/#20
=== RUN   TestPretty/#21
=== RUN   TestPretty/#22
=== RUN   TestPretty/#23
=== RUN   TestPretty/#24
=== RUN   TestPretty/#25
=== RUN   TestPretty/#26
=== RUN   TestPretty/#27
=== RUN   TestPretty/#28
=== RUN   TestPretty/#29
=== RUN   TestPretty/#30
=== RUN   TestPretty/#31
=== RUN   TestPretty/#32
=== RUN   TestPretty/#33
=== RUN   TestPretty/#34
=== RUN   TestPretty/#35
=== RUN   TestPretty/#36
=== RUN   TestPretty/#37
=== RUN   TestPretty/#38
=== RUN   TestPretty/#39
=== RUN   TestPretty/#40
=== RUN   TestPretty/#41
=== RUN   TestPretty/#42
=== RUN   TestPretty/#43
=== RUN   TestPretty/#44
=== RUN   TestPretty/#45
=== RUN   TestPretty/#46
=== RUN   TestPretty/#47
=== RUN   TestPretty/#48
=== RUN   TestPretty/#49
=== RUN   TestPretty/#50
=== RUN   TestPretty/#51
=== RUN   TestPretty/#52
=== RUN   TestPretty/#53
=== RUN   TestPretty/#54
=== RUN   TestPretty/#55
=== RUN   TestPretty/#56
=== RUN   TestPretty/#57
=== RUN   TestPretty/#58
=== RUN   TestPretty/#59
=== RUN   TestPretty/#60
=== RUN   TestPretty/#61
=== RUN   TestPretty/#62
=== RUN   TestPretty/#63
=== RUN   TestPretty/#64
=== RUN   TestPretty/#65
=== RUN   TestPretty/#66
=== RUN   TestPretty/#67
=== RUN   TestPretty/#68
=== RUN   TestPretty/#69
=== RUN   TestPretty/#70
=== RUN   TestPretty/#71
=== RUN   TestPretty/#72
=== RUN   TestPretty/#73
=== RUN   TestPretty/#74
=== RUN   TestPretty/#75
=== RUN   TestPretty/#76
=== RUN   TestPretty/#77
=== RUN   TestPretty/#78
=== RUN   TestPretty/#79
=== RUN   TestPretty/#80
=== RUN   TestPretty/#81
=== RUN   TestPretty/#82
=== RUN   TestPretty/#83
=== RUN   TestPretty/#84
=== RUN   TestPretty/#85
=== RUN   TestPretty/#86
=== RUN   TestPretty/#87
=== RUN   TestPretty/#88
=== RUN   TestPretty/#89
=== RUN   TestPretty/#90
=== RUN   TestPretty/#91
=== RUN   TestPretty/#92
=== RUN   TestPretty/#93
=== RUN   TestPretty/#94
=== RUN   TestPretty/#95
=== RUN   TestPretty/#96
=== RUN   TestPretty/#97
=== RUN   TestPretty/#98
=== RUN   TestPretty/#99
=== RUN   TestPretty/#100
=== RUN   TestPretty/#101
=== RUN   TestPretty/#102
=== RUN   TestPretty/#103
=== RUN   TestPretty/#104
=== RUN   TestPretty/#105
--- PASS: TestPretty (0.00s)
    --- PASS: TestPretty/#00 (0.00s)
    --- PASS: TestPretty/#01 (0.00s)
    --- PASS: TestPretty/#02 (0.00s)
    --- PASS: TestPretty/#03 (0.00s)
    --- PASS: TestPretty/#04 (0.00s)
    --- PASS: TestPretty/#05 (0.00s)
    --- PASS: TestPretty/#06 (0.00s)
    --- PASS: TestPretty/#07 (0.00s)
    --- PASS: TestPretty/#08 (0.00s)
    --- PASS: TestPretty/#09 (0.00s)
    --- PASS: TestPretty/#10 (0.00s)
    --- PASS: TestPretty/#11 (0.00s)
    --- PASS: TestPretty/#12 (0.00s)
    --- PASS: TestPretty/#13 (0.00s)
    --- PASS: TestPretty/#14 (0.00s)
    --- PASS: TestPretty/#15 (0.00s)
    --- PASS: TestPretty/#16 (0.00s)
    --- PASS: TestPretty/#17 (0.00s)
    --- PASS: TestPretty/#18 (0.00s)
    --- PASS: TestPretty/#19 (0.00s)
    --- PASS: TestPretty/#20 (0.00s)
    --- PASS: TestPretty/#21 (0.00s)
    --- PASS: TestPretty/#22 (0.00s)
    --- PASS: TestPretty/#23 (0.00s)
    --- PASS: TestPretty/#24 (0.00s)
    --- PASS: TestPretty/#25 (0.00s)
    --- PASS: TestPretty/#26 (0.00s)
    --- PASS: TestPretty/#27 (0.00s)
    --- PASS: TestPretty/#28 (0.00s)
    --- PASS: TestPretty/#29 (0.00s)
    --- PASS: TestPretty/#30 (0.00s)
    --- PASS: TestPretty/#31 (0.00s)
    --- PASS: TestPretty/#32 (0.00s)
    --- PASS: TestPretty/#33 (0.00s)
    --- PASS: TestPretty/#34 (0.00s)
    --- PASS: TestPretty/#35 (0.00s)
    --- PASS: TestPretty/#36 (0.00s)
    --- PASS: TestPretty/#37 (0.00s)
    --- PASS: TestPretty/#38 (0.00s)
    --- PASS: TestPretty/#39 (0.00s)
    --- PASS: TestPretty/#40 (0.00s)
    --- PASS: TestPretty/#41 (0.00s)
    --- PASS: TestPretty/#42 (0.00s)
    --- PASS: TestPretty/#43 (0.00s)
    --- PASS: TestPretty/#44 (0.00s)
    --- PASS: TestPretty/#45 (0.00s)
    --- PASS: TestPretty/#46 (0.00s)
    --- PASS: TestPretty/#47 (0.00s)
    --- PASS: TestPretty/#48 (0.00s)
    --- PASS: TestPretty/#49 (0.00s)
    --- PASS: TestPretty/#50 (0.00s)
    --- PASS: TestPretty/#51 (0.00s)
    --- PASS: TestPretty/#52 (0.00s)
    --- PASS: TestPretty/#53 (0.00s)
    --- PASS: TestPretty/#54 (0.00s)
    --- PASS: TestPretty/#55 (0.00s)
    --- PASS: TestPretty/#56 (0.00s)
    --- PASS: TestPretty/#57 (0.00s)
    --- PASS: TestPretty/#58 (0.00s)
    --- PASS: TestPretty/#59 (0.00s)
    --- PASS: TestPretty/#60 (0.00s)
    --- PASS: TestPretty/#61 (0.00s)
    --- PASS: TestPretty/#62 (0.00s)
    --- PASS: TestPretty/#63 (0.00s)
    --- PASS: TestPretty/#64 (0.00s)
    --- PASS: TestPretty/#65 (0.00s)
    --- PASS: TestPretty/#66 (0.00s)
    --- PASS: TestPretty/#67 (0.00s)
    --- PASS: TestPretty/#68 (0.00s)
    --- PASS: TestPretty/#69 (0.00s)
    --- PASS: TestPretty/#70 (0.00s)
    --- PASS: TestPretty/#71 (0.00s)
    --- PASS: TestPretty/#72 (0.00s)
    --- PASS: TestPretty/#73 (0.00s)
    --- PASS: TestPretty/#74 (0.00s)
    --- PASS: TestPretty/#75 (0.00s)
    --- PASS: TestPretty/#76 (0.00s)
    --- PASS: TestPretty/#77 (0.00s)
    --- PASS: TestPretty/#78 (0.00s)
    --- PASS: TestPretty/#79 (0.00s)
    --- PASS: TestPretty/#80 (0.00s)
    --- PASS: TestPretty/#81 (0.00s)
    --- PASS: TestPretty/#82 (0.00s)
    --- PASS: TestPretty/#83 (0.00s)
    --- PASS: TestPretty/#84 (0.00s)
    --- PASS: TestPretty/#85 (0.00s)
    --- PASS: TestPretty/#86 (0.00s)
    --- PASS: TestPretty/#87 (0.00s)
    --- PASS: TestPretty/#88 (0.00s)
    --- PASS: TestPretty/#89 (0.00s)
    --- PASS: TestPretty/#90 (0.00s)
    --- PASS: TestPretty/#91 (0.00s)
    --- PASS: TestPretty/#92 (0.00s)
    --- PASS: TestPretty/#93 (0.00s)
    --- PASS: TestPretty/#94 (0.00s)
    --- PASS: TestPretty/#95 (0.00s)
    --- PASS: TestPretty/#96 (0.00s)
    --- PASS: TestPretty/#97 (0.00s)
    --- PASS: TestPretty/#98 (0.00s)
    --- PASS: TestPretty/#99 (0.00s)
    --- PASS: TestPretty/#100 (0.00s)
    --- PASS: TestPretty/#101 (0.00s)
    --- PASS: TestPretty/#102 (0.00s)
    --- PASS: TestPretty/#103 (0.00s)
    --- PASS: TestPretty/#104 (0.00s)
    --- PASS: TestPretty/#105 (0.00s)
=== RUN   TestRender
=== RUN   TestRender/nil
=== RUN   TestRender/nil/KV
=== RUN   TestRender/nil/JSON
=== RUN   TestRender/empty
=== RUN   TestRender/empty/KV
=== RUN   TestRender/empty/JSON
=== RUN   TestRender/primitives
=== RUN   TestRender/primitives/KV
=== RUN   TestRender/primitives/JSON
=== RUN   TestRender/pseudo_structs
=== RUN   TestRender/pseudo_structs/KV
=== RUN   TestRender/pseudo_structs/JSON
=== RUN   TestRender/escapes
=== RUN   TestRender/escapes/KV
=== RUN   TestRender/escapes/JSON
=== RUN   TestRender/missing_value
=== RUN   TestRender/missing_value/KV
=== RUN   TestRender/missing_value/JSON
=== RUN   TestRender/non-string_key_int
=== RUN   TestRender/non-string_key_int/KV
=== RUN   TestRender/non-string_key_int/JSON
=== RUN   TestRender/non-string_key_struct
=== RUN   TestRender/non-string_key_struct/KV
=== RUN   TestRender/non-string_key_struct/JSON
--- PASS: TestRender (0.00s)
    --- PASS: TestRender/nil (0.00s)
        --- PASS: TestRender/nil/KV (0.00s)
        --- PASS: TestRender/nil/JSON (0.00s)
    --- PASS: TestRender/empty (0.00s)
        --- PASS: TestRender/empty/KV (0.00s)
        --- PASS: TestRender/empty/JSON (0.00s)
    --- PASS: TestRender/primitives (0.00s)
        --- PASS: TestRender/primitives/KV (0.00s)
        --- PASS: TestRender/primitives/JSON (0.00s)
    --- PASS: TestRender/pseudo_structs (0.00s)
        --- PASS: TestRender/pseudo_structs/KV (0.00s)
        --- PASS: TestRender/pseudo_structs/JSON (0.00s)
    --- PASS: TestRender/escapes (0.00s)
        --- PASS: TestRender/escapes/KV (0.00s)
        --- PASS: TestRender/escapes/JSON (0.00s)
    --- PASS: TestRender/missing_value (0.00s)
        --- PASS: TestRender/missing_value/KV (0.00s)
        --- PASS: TestRender/missing_value/JSON (0.00s)
    --- PASS: TestRender/non-string_key_int (0.00s)
        --- PASS: TestRender/non-string_key_int/KV (0.00s)
        --- PASS: TestRender/non-string_key_int/JSON (0.00s)
    --- PASS: TestRender/non-string_key_struct (0.00s)
        --- PASS: TestRender/non-string_key_struct/KV (0.00s)
        --- PASS: TestRender/non-string_key_struct/JSON (0.00s)
=== RUN   TestSanitize
=== RUN   TestSanitize/empty
=== RUN   TestSanitize/already_sane
=== RUN   TestSanitize/missing_value
=== RUN   TestSanitize/non-string_key_int
=== RUN   TestSanitize/non-string_key_struct
--- PASS: TestSanitize (0.00s)
    --- PASS: TestSanitize/empty (0.00s)
    --- PASS: TestSanitize/already_sane (0.00s)
    --- PASS: TestSanitize/missing_value (0.00s)
    --- PASS: TestSanitize/non-string_key_int (0.00s)
    --- PASS: TestSanitize/non-string_key_struct (0.00s)
=== RUN   TestLoggerOptionsLevelHooks
=== RUN   TestLoggerOptionsLevelHooks/verbosity=none
=== RUN   TestLoggerOptionsLevelHooks/verbosity=debug
=== RUN   TestLoggerOptionsLevelHooks/verbosity=info
=== RUN   TestLoggerOptionsLevelHooks/verbosity=warning
=== RUN   TestLoggerOptionsLevelHooks/verbosity=error
=== RUN   TestLoggerOptionsLevelHooks/verbosity=all
--- PASS: TestLoggerOptionsLevelHooks (0.00s)
    --- PASS: TestLoggerOptionsLevelHooks/verbosity=none (0.00s)
    --- PASS: TestLoggerOptionsLevelHooks/verbosity=debug (0.00s)
    --- PASS: TestLoggerOptionsLevelHooks/verbosity=info (0.00s)
    --- PASS: TestLoggerOptionsLevelHooks/verbosity=warning (0.00s)
    --- PASS: TestLoggerOptionsLevelHooks/verbosity=error (0.00s)
    --- PASS: TestLoggerOptionsLevelHooks/verbosity=all (0.00s)
=== RUN   TestLoggerOptionsTimestampFormat
--- PASS: TestLoggerOptionsTimestampFormat (0.00s)
=== RUN   TestLogger
=== RUN   TestLogger/just_msg
=== RUN   TestLogger/primitives
=== RUN   TestLogger/just_msg#01
=== RUN   TestLogger/primitives#01
=== RUN   TestLogger/just_msg#02
=== RUN   TestLogger/primitives#02
=== RUN   TestLogger/just_msg#03
=== RUN   TestLogger/primitives#03
--- PASS: TestLogger (0.00s)
    --- PASS: TestLogger/just_msg (0.00s)
    --- PASS: TestLogger/primitives (0.00s)
    --- PASS: TestLogger/just_msg#01 (0.00s)
    --- PASS: TestLogger/primitives#01 (0.00s)
    --- PASS: TestLogger/just_msg#02 (0.00s)
    --- PASS: TestLogger/primitives#02 (0.00s)
    --- PASS: TestLogger/just_msg#03 (0.00s)
    --- PASS: TestLogger/primitives#03 (0.00s)
=== RUN   TestLoggerWithCaller
=== RUN   TestLoggerWithCaller/caller=CategoryAll
=== RUN   TestLoggerWithCaller/caller=CategoryAll,_logCallerFunc=true
=== RUN   TestLoggerWithCaller/caller=CategoryDebug
=== RUN   TestLoggerWithCaller/caller=CategoryInfo
=== RUN   TestLoggerWithCaller/caller=CategoryWarning
=== RUN   TestLoggerWithCaller/caller=CategoryError
=== RUN   TestLoggerWithCaller/caller=CategoryNone
--- PASS: TestLoggerWithCaller (0.00s)
    --- PASS: TestLoggerWithCaller/caller=CategoryAll (0.00s)
    --- PASS: TestLoggerWithCaller/caller=CategoryAll,_logCallerFunc=true (0.00s)
    --- PASS: TestLoggerWithCaller/caller=CategoryDebug (0.00s)
    --- PASS: TestLoggerWithCaller/caller=CategoryInfo (0.00s)
    --- PASS: TestLoggerWithCaller/caller=CategoryWarning (0.00s)
    --- PASS: TestLoggerWithCaller/caller=CategoryError (0.00s)
    --- PASS: TestLoggerWithCaller/caller=CategoryNone (0.00s)
=== RUN   TestLoggerWithName
=== RUN   TestLoggerWithName/one
=== RUN   TestLoggerWithName/two
=== RUN   TestLoggerWithName/one#01
=== RUN   TestLoggerWithName/two#01
=== RUN   TestLoggerWithName/one#02
=== RUN   TestLoggerWithName/two#02
=== RUN   TestLoggerWithName/one#03
=== RUN   TestLoggerWithName/two#03
--- PASS: TestLoggerWithName (0.00s)
    --- PASS: TestLoggerWithName/one (0.00s)
    --- PASS: TestLoggerWithName/two (0.00s)
    --- PASS: TestLoggerWithName/one#01 (0.00s)
    --- PASS: TestLoggerWithName/two#01 (0.00s)
    --- PASS: TestLoggerWithName/one#02 (0.00s)
    --- PASS: TestLoggerWithName/two#02 (0.00s)
    --- PASS: TestLoggerWithName/one#03 (0.00s)
    --- PASS: TestLoggerWithName/two#03 (0.00s)
=== RUN   TestLoggerWithValues
=== RUN   TestLoggerWithValues/zero
=== RUN   TestLoggerWithValues/one
=== RUN   TestLoggerWithValues/two
=== RUN   TestLoggerWithValues/dangling
=== RUN   TestLoggerWithValues/zero#01
=== RUN   TestLoggerWithValues/one#01
=== RUN   TestLoggerWithValues/two#01
=== RUN   TestLoggerWithValues/dangling#01
=== RUN   TestLoggerWithValues/zero#02
=== RUN   TestLoggerWithValues/one#02
=== RUN   TestLoggerWithValues/two#02
=== RUN   TestLoggerWithValues/dangling#02
=== RUN   TestLoggerWithValues/zero#03
=== RUN   TestLoggerWithValues/one#03
=== RUN   TestLoggerWithValues/two#03
=== RUN   TestLoggerWithValues/dangling#03
--- PASS: TestLoggerWithValues (0.00s)
    --- PASS: TestLoggerWithValues/zero (0.00s)
    --- PASS: TestLoggerWithValues/one (0.00s)
    --- PASS: TestLoggerWithValues/two (0.00s)
    --- PASS: TestLoggerWithValues/dangling (0.00s)
    --- PASS: TestLoggerWithValues/zero#01 (0.00s)
    --- PASS: TestLoggerWithValues/one#01 (0.00s)
    --- PASS: TestLoggerWithValues/two#01 (0.00s)
    --- PASS: TestLoggerWithValues/dangling#01 (0.00s)
    --- PASS: TestLoggerWithValues/zero#02 (0.00s)
    --- PASS: TestLoggerWithValues/one#02 (0.00s)
    --- PASS: TestLoggerWithValues/two#02 (0.00s)
    --- PASS: TestLoggerWithValues/dangling#02 (0.00s)
    --- PASS: TestLoggerWithValues/zero#03 (0.00s)
    --- PASS: TestLoggerWithValues/one#03 (0.00s)
    --- PASS: TestLoggerWithValues/two#03 (0.00s)
    --- PASS: TestLoggerWithValues/dangling#03 (0.00s)
=== RUN   TestLoggerWithCallDepth
=== RUN   TestLoggerWithCallDepth/verbosity=debug,_callerDepth=1
=== RUN   TestLoggerWithCallDepth/verbosity=info,_callerDepth=1
=== RUN   TestLoggerWithCallDepth/verbosity=warning,_callerDepth=1
=== RUN   TestLoggerWithCallDepth/verbosity=error,_callerDepth=1
--- PASS: TestLoggerWithCallDepth (0.00s)
    --- PASS: TestLoggerWithCallDepth/verbosity=debug,_callerDepth=1 (0.00s)
    --- PASS: TestLoggerWithCallDepth/verbosity=info,_callerDepth=1 (0.00s)
    --- PASS: TestLoggerWithCallDepth/verbosity=warning,_callerDepth=1 (0.00s)
    --- PASS: TestLoggerWithCallDepth/verbosity=error,_callerDepth=1 (0.00s)
=== RUN   TestLoggerVerbosity
=== RUN   TestLoggerVerbosity/verbosity=all
=== RUN   TestLoggerVerbosity/verbosity=debug
=== RUN   TestLoggerVerbosity/verbosity=info
=== RUN   TestLoggerVerbosity/verbosity=warning
=== RUN   TestLoggerVerbosity/verbosity=error
=== RUN   TestLoggerVerbosity/verbosity=none
--- PASS: TestLoggerVerbosity (0.00s)
    --- PASS: TestLoggerVerbosity/verbosity=all (0.00s)
    --- PASS: TestLoggerVerbosity/verbosity=debug (0.00s)
    --- PASS: TestLoggerVerbosity/verbosity=info (0.00s)
    --- PASS: TestLoggerVerbosity/verbosity=warning (0.00s)
    --- PASS: TestLoggerVerbosity/verbosity=error (0.00s)
    --- PASS: TestLoggerVerbosity/verbosity=none (0.00s)
=== RUN   TestModifyDefaults
--- PASS: TestModifyDefaults (0.00s)
=== RUN   TestNewLogger
--- PASS: TestNewLogger (0.00s)
=== RUN   TestSetVerbosity
=== RUN   TestSetVerbosity/to=none/by_logger
=== RUN   TestSetVerbosity/to=error/by_logger
=== RUN   TestSetVerbosity/to=warning/by_logger
=== RUN   TestSetVerbosity/to=info/by_logger
=== RUN   TestSetVerbosity/to=debug/by_logger
=== RUN   TestSetVerbosity/to=all/by_logger
=== RUN   TestSetVerbosity/to=none/by_exp
=== RUN   TestSetVerbosity/to=error/by_exp
=== RUN   TestSetVerbosity/to=warning/by_exp
=== RUN   TestSetVerbosity/to=info/by_exp
=== RUN   TestSetVerbosity/to=debug/by_exp
=== RUN   TestSetVerbosity/to=all/by_exp
--- PASS: TestSetVerbosity (0.00s)
    --- PASS: TestSetVerbosity/to=none/by_logger (0.00s)
    --- PASS: TestSetVerbosity/to=error/by_logger (0.00s)
    --- PASS: TestSetVerbosity/to=warning/by_logger (0.00s)
    --- PASS: TestSetVerbosity/to=info/by_logger (0.00s)
    --- PASS: TestSetVerbosity/to=debug/by_logger (0.00s)
    --- PASS: TestSetVerbosity/to=all/by_logger (0.00s)
    --- PASS: TestSetVerbosity/to=none/by_exp (0.00s)
    --- PASS: TestSetVerbosity/to=error/by_exp (0.00s)
    --- PASS: TestSetVerbosity/to=warning/by_exp (0.00s)
    --- PASS: TestSetVerbosity/to=info/by_exp (0.00s)
    --- PASS: TestSetVerbosity/to=debug/by_exp (0.00s)
    --- PASS: TestSetVerbosity/to=all/by_exp (0.00s)
=== RUN   TestRegistryRange
--- PASS: TestRegistryRange (0.00s)
=== RUN   Example
--- PASS: Example (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/log	(cached)
testing: warning: no tests to run
PASS
ok  	github.com/ethersphere/bee/pkg/manifest	(cached) [no tests to run]
=== RUN   TestVersion01
=== PAUSE TestVersion01
=== RUN   TestVersion02
=== PAUSE TestVersion02
=== RUN   TestUnmarshal01
=== PAUSE TestUnmarshal01
=== RUN   TestUnmarshal02
=== PAUSE TestUnmarshal02
=== RUN   TestMarshal
--- PASS: TestMarshal (0.00s)
=== RUN   Test_UnmarshalBinary
=== PAUSE Test_UnmarshalBinary
=== RUN   TestNilPath
=== PAUSE TestNilPath
=== RUN   TestAddAndLookup
=== PAUSE TestAddAndLookup
=== RUN   TestAddAndLookupNode
=== PAUSE TestAddAndLookupNode
=== RUN   TestRemove
=== PAUSE TestRemove
=== RUN   TestHasPrefix
=== PAUSE TestHasPrefix
=== RUN   TestPersistIdempotence
=== PAUSE TestPersistIdempotence
=== RUN   TestWalkNode
=== PAUSE TestWalkNode
=== RUN   TestWalk
=== PAUSE TestWalk
=== CONT  TestVersion01
--- PASS: TestVersion01 (0.00s)
=== CONT  TestAddAndLookupNode
=== RUN   TestAddAndLookupNode/a
=== PAUSE TestAddAndLookupNode/a
=== RUN   TestAddAndLookupNode/a/with_load_save
=== PAUSE TestAddAndLookupNode/a/with_load_save
=== RUN   TestAddAndLookupNode/simple
=== PAUSE TestAddAndLookupNode/simple
=== CONT  TestWalk
=== RUN   TestWalk/simple
=== PAUSE TestWalk/simple
=== RUN   TestWalk/simple/with_load_save
=== PAUSE TestWalk/simple/with_load_save
=== CONT  TestWalkNode
=== RUN   TestWalkNode/simple
=== PAUSE TestWalkNode/simple
=== CONT  TestPersistIdempotence
=== RUN   TestWalkNode/simple/with_load_save
=== PAUSE TestWalkNode/simple/with_load_save
=== CONT  TestAddAndLookup
--- PASS: TestAddAndLookup (0.00s)
=== CONT  TestNilPath
--- PASS: TestNilPath (0.00s)
=== CONT  Test_UnmarshalBinary
=== RUN   Test_UnmarshalBinary/nil_input_bytes
=== PAUSE Test_UnmarshalBinary/nil_input_bytes
=== CONT  TestHasPrefix
=== RUN   Test_UnmarshalBinary/not_enough_bytes_for_header
=== RUN   TestHasPrefix/simple
=== CONT  TestRemove
=== PAUSE TestHasPrefix/simple
=== RUN   TestHasPrefix/nested-single
=== PAUSE TestHasPrefix/nested-single
=== PAUSE Test_UnmarshalBinary/not_enough_bytes_for_header
=== RUN   Test_UnmarshalBinary/invalid_header_-_empty_bytes
=== PAUSE Test_UnmarshalBinary/invalid_header_-_empty_bytes
=== RUN   Test_UnmarshalBinary/invalid_manifest_size_89
=== PAUSE Test_UnmarshalBinary/invalid_manifest_size_89
=== RUN   Test_UnmarshalBinary/valid_manifest
=== CONT  TestVersion02
=== PAUSE Test_UnmarshalBinary/valid_manifest
--- PASS: TestVersion02 (0.00s)
=== RUN   TestRemove/simple
=== CONT  TestUnmarshal02
=== PAUSE TestRemove/simple
=== RUN   TestRemove/nested-prefix-is-not-collapsed
=== PAUSE TestRemove/nested-prefix-is-not-collapsed
=== RUN   TestAddAndLookupNode/simple/with_load_save
=== PAUSE TestAddAndLookupNode/simple/with_load_save
=== RUN   TestAddAndLookupNode/nested-value-node-is-recognized
=== PAUSE TestAddAndLookupNode/nested-value-node-is-recognized
=== RUN   TestAddAndLookupNode/nested-value-node-is-recognized/with_load_save
=== PAUSE TestAddAndLookupNode/nested-value-node-is-recognized/with_load_save
=== RUN   TestAddAndLookupNode/nested-prefix-is-not-collapsed
=== PAUSE TestAddAndLookupNode/nested-prefix-is-not-collapsed
=== RUN   TestAddAndLookupNode/nested-prefix-is-not-collapsed/with_load_save
--- PASS: TestUnmarshal02 (0.00s)
=== PAUSE TestAddAndLookupNode/nested-prefix-is-not-collapsed/with_load_save
=== RUN   TestAddAndLookupNode/conflicting-path
=== PAUSE TestAddAndLookupNode/conflicting-path
=== RUN   TestAddAndLookupNode/conflicting-path/with_load_save
=== PAUSE TestAddAndLookupNode/conflicting-path/with_load_save
=== RUN   TestAddAndLookupNode/spa-website
=== PAUSE TestAddAndLookupNode/spa-website
=== RUN   TestAddAndLookupNode/spa-website/with_load_save
=== PAUSE TestAddAndLookupNode/spa-website/with_load_save
=== RUN   Test_UnmarshalBinary/invalid_manifest_size_95
=== PAUSE Test_UnmarshalBinary/invalid_manifest_size_95
=== RUN   Test_UnmarshalBinary/invalid_manifest_size_96
=== PAUSE Test_UnmarshalBinary/invalid_manifest_size_96
=== CONT  TestHasPrefix/simple
=== CONT  TestWalkNode/simple/with_load_save
=== CONT  TestHasPrefix/nested-single
--- PASS: TestHasPrefix (0.00s)
    --- PASS: TestHasPrefix/simple (0.00s)
    --- PASS: TestHasPrefix/nested-single (0.00s)
=== CONT  TestRemove/simple
--- PASS: TestPersistIdempotence (0.00s)
=== CONT  TestAddAndLookupNode/a
=== CONT  TestRemove/nested-prefix-is-not-collapsed
--- PASS: TestRemove (0.00s)
    --- PASS: TestRemove/simple (0.00s)
    --- PASS: TestRemove/nested-prefix-is-not-collapsed (0.00s)
=== CONT  Test_UnmarshalBinary/nil_input_bytes
=== CONT  TestAddAndLookupNode/spa-website
=== CONT  TestAddAndLookupNode/conflicting-path
=== CONT  TestAddAndLookupNode/nested-prefix-is-not-collapsed/with_load_save
=== CONT  TestAddAndLookupNode/nested-prefix-is-not-collapsed
=== CONT  TestWalk/simple
=== CONT  TestAddAndLookupNode/nested-value-node-is-recognized/with_load_save
=== CONT  TestAddAndLookupNode/conflicting-path/with_load_save
=== CONT  TestAddAndLookupNode/simple/with_load_save
=== CONT  TestAddAndLookupNode/spa-website/with_load_save
=== CONT  TestWalkNode/simple
--- PASS: TestWalkNode (0.00s)
    --- PASS: TestWalkNode/simple/with_load_save (0.00s)
    --- PASS: TestWalkNode/simple (0.00s)
=== CONT  TestAddAndLookupNode/nested-value-node-is-recognized
=== CONT  Test_UnmarshalBinary/invalid_manifest_size_96
=== CONT  Test_UnmarshalBinary/invalid_manifest_size_95
=== CONT  Test_UnmarshalBinary/valid_manifest
=== CONT  Test_UnmarshalBinary/invalid_manifest_size_89
=== CONT  Test_UnmarshalBinary/invalid_header_-_empty_bytes
=== CONT  Test_UnmarshalBinary/not_enough_bytes_for_header
=== CONT  TestAddAndLookupNode/simple
--- PASS: Test_UnmarshalBinary (0.00s)
    --- PASS: Test_UnmarshalBinary/nil_input_bytes (0.00s)
    --- PASS: Test_UnmarshalBinary/invalid_manifest_size_96 (0.00s)
    --- PASS: Test_UnmarshalBinary/invalid_manifest_size_95 (0.00s)
    --- PASS: Test_UnmarshalBinary/invalid_manifest_size_89 (0.00s)
    --- PASS: Test_UnmarshalBinary/invalid_header_-_empty_bytes (0.00s)
    --- PASS: Test_UnmarshalBinary/valid_manifest (0.00s)
    --- PASS: Test_UnmarshalBinary/not_enough_bytes_for_header (0.00s)
=== CONT  TestUnmarshal01
--- PASS: TestUnmarshal01 (0.00s)
=== CONT  TestAddAndLookupNode/a/with_load_save
=== CONT  TestWalk/simple/with_load_save
--- PASS: TestWalk (0.00s)
    --- PASS: TestWalk/simple (0.00s)
    --- PASS: TestWalk/simple/with_load_save (0.00s)
--- PASS: TestAddAndLookupNode (0.00s)
    --- PASS: TestAddAndLookupNode/a (0.00s)
    --- PASS: TestAddAndLookupNode/spa-website (0.00s)
    --- PASS: TestAddAndLookupNode/conflicting-path (0.00s)
    --- PASS: TestAddAndLookupNode/nested-prefix-is-not-collapsed (0.00s)
    --- PASS: TestAddAndLookupNode/nested-value-node-is-recognized/with_load_save (0.00s)
    --- PASS: TestAddAndLookupNode/conflicting-path/with_load_save (0.00s)
    --- PASS: TestAddAndLookupNode/nested-value-node-is-recognized (0.00s)
    --- PASS: TestAddAndLookupNode/simple/with_load_save (0.00s)
    --- PASS: TestAddAndLookupNode/simple (0.00s)
    --- PASS: TestAddAndLookupNode/nested-prefix-is-not-collapsed/with_load_save (0.00s)
    --- PASS: TestAddAndLookupNode/spa-website/with_load_save (0.00s)
    --- PASS: TestAddAndLookupNode/a/with_load_save (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/manifest/mantaray	(cached)
=== RUN   TestNilPath
=== PAUSE TestNilPath
=== RUN   TestEntries
=== PAUSE TestEntries
=== RUN   TestMarshal
=== PAUSE TestMarshal
=== RUN   TestHasPrefix
=== PAUSE TestHasPrefix
=== RUN   TestWalkEntry
=== PAUSE TestWalkEntry
=== CONT  TestNilPath
--- PASS: TestNilPath (0.00s)
=== CONT  TestWalkEntry
=== CONT  TestMarshal
=== RUN   TestMarshal/empty-manifest
=== PAUSE TestMarshal/empty-manifest
=== RUN   TestMarshal/one-entry
=== PAUSE TestMarshal/one-entry
=== RUN   TestMarshal/two-entries
=== PAUSE TestMarshal/two-entries
=== RUN   TestMarshal/nested-entries
=== PAUSE TestMarshal/nested-entries
=== CONT  TestMarshal/empty-manifest
=== RUN   TestWalkEntry/empty-manifest
=== PAUSE TestWalkEntry/empty-manifest
=== RUN   TestWalkEntry/one-entry
=== PAUSE TestWalkEntry/one-entry
=== RUN   TestWalkEntry/two-entries
=== PAUSE TestWalkEntry/two-entries
=== RUN   TestWalkEntry/nested-entries
=== PAUSE TestWalkEntry/nested-entries
=== CONT  TestWalkEntry/empty-manifest
=== CONT  TestMarshal/nested-entries
=== CONT  TestMarshal/two-entries
=== CONT  TestMarshal/one-entry
--- PASS: TestMarshal (0.00s)
    --- PASS: TestMarshal/empty-manifest (0.00s)
    --- PASS: TestMarshal/nested-entries (0.00s)
    --- PASS: TestMarshal/two-entries (0.00s)
    --- PASS: TestMarshal/one-entry (0.00s)
=== CONT  TestEntries
=== RUN   TestEntries/empty-manifest
=== PAUSE TestEntries/empty-manifest
=== RUN   TestEntries/one-entry
=== PAUSE TestEntries/one-entry
=== RUN   TestEntries/two-entries
=== PAUSE TestEntries/two-entries
=== RUN   TestEntries/nested-entries
=== PAUSE TestEntries/nested-entries
=== CONT  TestEntries/empty-manifest
=== CONT  TestWalkEntry/nested-entries
=== CONT  TestWalkEntry/two-entries
=== CONT  TestWalkEntry/one-entry
--- PASS: TestWalkEntry (0.00s)
    --- PASS: TestWalkEntry/empty-manifest (0.00s)
    --- PASS: TestWalkEntry/nested-entries (0.00s)
    --- PASS: TestWalkEntry/two-entries (0.00s)
    --- PASS: TestWalkEntry/one-entry (0.00s)
=== CONT  TestHasPrefix
=== RUN   TestHasPrefix/simple
=== PAUSE TestHasPrefix/simple
=== RUN   TestHasPrefix/nested-single
=== PAUSE TestHasPrefix/nested-single
=== CONT  TestHasPrefix/simple
=== CONT  TestEntries/nested-entries
=== CONT  TestEntries/two-entries
=== CONT  TestEntries/one-entry
--- PASS: TestEntries (0.00s)
    --- PASS: TestEntries/empty-manifest (0.00s)
    --- PASS: TestEntries/nested-entries (0.00s)
    --- PASS: TestEntries/two-entries (0.00s)
    --- PASS: TestEntries/one-entry (0.00s)
=== CONT  TestHasPrefix/nested-single
--- PASS: TestHasPrefix (0.00s)
    --- PASS: TestHasPrefix/simple (0.00s)
    --- PASS: TestHasPrefix/nested-single (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/manifest/simple	(cached)
=== RUN   TestPrometheusCollectorsFromFields
=== PAUSE TestPrometheusCollectorsFromFields
=== CONT  TestPrometheusCollectorsFromFields
--- PASS: TestPrometheusCollectorsFromFields (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/metrics	(cached)
=== RUN   TestNetstoreRetrieval
=== PAUSE TestNetstoreRetrieval
=== RUN   TestNetstoreNoRetrieval
=== PAUSE TestNetstoreNoRetrieval
=== RUN   TestInvalidChunkNetstoreRetrieval
=== PAUSE TestInvalidChunkNetstoreRetrieval
=== RUN   TestInvalidPostageStamp
=== PAUSE TestInvalidPostageStamp
=== CONT  TestNetstoreRetrieval
=== CONT  TestInvalidChunkNetstoreRetrieval
=== CONT  TestNetstoreNoRetrieval
=== CONT  TestInvalidPostageStamp
--- PASS: TestNetstoreNoRetrieval (0.00s)
--- PASS: TestInvalidChunkNetstoreRetrieval (0.02s)
--- PASS: TestNetstoreRetrieval (0.02s)
--- PASS: TestInvalidPostageStamp (0.02s)
PASS
ok  	github.com/ethersphere/bee/pkg/netstore	(cached)
testing: warning: no tests to run
PASS
ok  	github.com/ethersphere/bee/pkg/node	(cached) [no tests to run]
=== RUN   TestNewSwarmStreamName
=== PAUSE TestNewSwarmStreamName
=== RUN   TestReachabilityStatus_String
=== PAUSE TestReachabilityStatus_String
=== RUN   TestBlocklistError
=== PAUSE TestBlocklistError
=== RUN   TestDisconnectError
=== PAUSE TestDisconnectError
=== CONT  TestNewSwarmStreamName
--- PASS: TestNewSwarmStreamName (0.00s)
=== CONT  TestReachabilityStatus_String
--- PASS: TestReachabilityStatus_String (0.00s)
=== CONT  TestBlocklistError
--- PASS: TestBlocklistError (0.00s)
=== CONT  TestDisconnectError
--- PASS: TestDisconnectError (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/p2p	(cached)
=== RUN   TestAddresses
=== PAUSE TestAddresses
=== RUN   TestConnectDisconnect
=== PAUSE TestConnectDisconnect
=== RUN   TestConnectToLightPeer
=== PAUSE TestConnectToLightPeer
=== RUN   TestLightPeerLimit
=== PAUSE TestLightPeerLimit
=== RUN   TestStreamsMaxIncomingLimit
=== PAUSE TestStreamsMaxIncomingLimit
=== RUN   TestDoubleConnect
=== PAUSE TestDoubleConnect
=== RUN   TestDoubleDisconnect
=== PAUSE TestDoubleDisconnect
=== RUN   TestMultipleConnectDisconnect
=== PAUSE TestMultipleConnectDisconnect
=== RUN   TestConnectDisconnectOnAllAddresses
=== PAUSE TestConnectDisconnectOnAllAddresses
=== RUN   TestDoubleConnectOnAllAddresses
=== PAUSE TestDoubleConnectOnAllAddresses
=== RUN   TestDifferentNetworkIDs
=== PAUSE TestDifferentNetworkIDs
=== RUN   TestConnectWithEnabledWSTransports
=== PAUSE TestConnectWithEnabledWSTransports
=== RUN   TestConnectRepeatHandshake
=== PAUSE TestConnectRepeatHandshake
=== RUN   TestBlocklisting
=== PAUSE TestBlocklisting
=== RUN   TestTopologyNotifier
=== PAUSE TestTopologyNotifier
=== RUN   TestTopologyAnnounce
=== PAUSE TestTopologyAnnounce
=== RUN   TestTopologyOverSaturated
=== PAUSE TestTopologyOverSaturated
=== RUN   TestWithDisconnectStreams
=== PAUSE TestWithDisconnectStreams
=== RUN   TestWithBlocklistStreams
=== PAUSE TestWithBlocklistStreams
=== RUN   TestUserAgentLogging
=== PAUSE TestUserAgentLogging
=== RUN   TestReachabilityUpdate
=== PAUSE TestReachabilityUpdate
=== RUN   TestHeaders
=== PAUSE TestHeaders
=== RUN   TestHeaders_empty
=== PAUSE TestHeaders_empty
=== RUN   TestHeadler
=== PAUSE TestHeadler
=== RUN   TestNewStream
=== PAUSE TestNewStream
=== RUN   TestNewStream_OnlyFull
=== PAUSE TestNewStream_OnlyFull
=== RUN   TestNewStream_Mixed
=== PAUSE TestNewStream_Mixed
=== RUN   TestNewStreamMulti
=== PAUSE TestNewStreamMulti
=== RUN   TestNewStream_errNotSupported
=== PAUSE TestNewStream_errNotSupported
=== RUN   TestNewStream_semanticVersioning
=== PAUSE TestNewStream_semanticVersioning
=== RUN   TestDisconnectError
=== PAUSE TestDisconnectError
=== RUN   TestConnectDisconnectEvents
=== PAUSE TestConnectDisconnectEvents
=== RUN   TestPing
=== PAUSE TestPing
=== RUN   TestStaticAddressResolver
=== PAUSE TestStaticAddressResolver
=== RUN   TestTracing
=== PAUSE TestTracing
=== RUN   TestDynamicWelcomeMessage
=== PAUSE TestDynamicWelcomeMessage
=== CONT  TestAddresses
=== CONT  TestWithBlocklistStreams
    connections_test.go:956: this test always fails
--- SKIP: TestWithBlocklistStreams (0.00s)
=== CONT  TestDynamicWelcomeMessage
=== RUN   TestDynamicWelcomeMessage/Get_current_message_-_OK
=== PAUSE TestDynamicWelcomeMessage/Get_current_message_-_OK
=== RUN   TestDynamicWelcomeMessage/Set_new_message
=== RUN   TestDynamicWelcomeMessage/Set_new_message/OK
=== PAUSE TestDynamicWelcomeMessage/Set_new_message/OK
=== RUN   TestDynamicWelcomeMessage/Set_new_message/error_-_message_too_long
=== PAUSE TestDynamicWelcomeMessage/Set_new_message/error_-_message_too_long
=== CONT  TestDynamicWelcomeMessage/Set_new_message/OK
=== CONT  TestConnectDisconnectEvents
=== CONT  TestHeadler
=== CONT  TestNewStream_Mixed
=== CONT  TestNewStream_OnlyFull
=== CONT  TestNewStream
=== CONT  TestHeaders_empty
=== CONT  TestStaticAddressResolver
=== RUN   TestStaticAddressResolver/replace_port
=== PAUSE TestStaticAddressResolver/replace_port
=== RUN   TestStaticAddressResolver/replace_ip_v4
=== PAUSE TestStaticAddressResolver/replace_ip_v4
=== RUN   TestStaticAddressResolver/replace_ip_v6
=== PAUSE TestStaticAddressResolver/replace_ip_v6
=== RUN   TestStaticAddressResolver/replace_ip_v4_with_ip_v6
=== PAUSE TestStaticAddressResolver/replace_ip_v4_with_ip_v6
=== RUN   TestStaticAddressResolver/replace_ip_v6_with_ip_v4
=== PAUSE TestStaticAddressResolver/replace_ip_v6_with_ip_v4
=== RUN   TestStaticAddressResolver/replace_ip_and_port
=== PAUSE TestStaticAddressResolver/replace_ip_and_port
=== RUN   TestStaticAddressResolver/replace_ip_v4_and_port_with_ip_v6
=== PAUSE TestStaticAddressResolver/replace_ip_v4_and_port_with_ip_v6
=== RUN   TestStaticAddressResolver/replace_ip_v6_and_port_with_ip_v4
=== PAUSE TestStaticAddressResolver/replace_ip_v6_and_port_with_ip_v4
=== RUN   TestStaticAddressResolver/replace_ip_v6_and_port_with_dns_v4
=== PAUSE TestStaticAddressResolver/replace_ip_v6_and_port_with_dns_v4
=== RUN   TestStaticAddressResolver/replace_ip_v4_and_port_with_dns
=== PAUSE TestStaticAddressResolver/replace_ip_v4_and_port_with_dns
=== CONT  TestTracing
--- PASS: TestAddresses (0.05s)
=== CONT  TestReachabilityUpdate
2023/02/27 17:15:51 failed to sufficiently increase receive buffer size (was: 208 kiB, wanted: 2048 kiB, got: 416 kiB). See https://github.com/quic-go/quic-go/wiki/UDP-Receive-Buffer-Size for details.
--- PASS: TestReachabilityUpdate (0.00s)
=== CONT  TestHeaders
--- PASS: TestHeadler (0.06s)
=== CONT  TestNewStream_semanticVersioning
--- PASS: TestNewStream_OnlyFull (0.07s)
=== CONT  TestDisconnectError
--- PASS: TestHeaders_empty (0.07s)
=== CONT  TestPing
--- PASS: TestNewStream (0.07s)
=== CONT  TestUserAgentLogging
--- PASS: TestTracing (0.03s)
=== CONT  TestDynamicWelcomeMessage/Set_new_message/error_-_message_too_long
--- PASS: TestPing (0.01s)
=== CONT  TestNewStream_errNotSupported
--- PASS: TestHeaders (0.02s)
=== CONT  TestDoubleConnectOnAllAddresses
=== CONT  TestWithDisconnectStreams
--- PASS: TestNewStream_Mixed (0.09s)
=== CONT  TestTopologyOverSaturated
--- PASS: TestNewStream_semanticVersioning (0.04s)
=== CONT  TestTopologyAnnounce
--- PASS: TestNewStream_errNotSupported (0.02s)
=== CONT  TestTopologyNotifier
--- PASS: TestDisconnectError (0.04s)
=== CONT  TestDoubleConnect
--- PASS: TestUserAgentLogging (0.05s)
=== CONT  TestBlocklisting
--- PASS: TestWithDisconnectStreams (0.05s)
=== CONT  TestConnectRepeatHandshake
--- PASS: TestDoubleConnect (0.05s)
=== CONT  TestConnectDisconnectOnAllAddresses
--- PASS: TestTopologyOverSaturated (0.08s)
=== CONT  TestConnectWithEnabledWSTransports
2023-02-27T17:15:51.990+0300	ERROR	swarm2	swarm/swarm.go:223	error when shutting down listener: close tcp6 [::]:34815: use of closed network connection
2023-02-27T17:15:51.990+0300	ERROR	swarm2	swarm/swarm.go:223	error when shutting down listener: close tcp4 0.0.0.0:37789: use of closed network connection
2023-02-27T17:15:51.990+0300	ERROR	swarm2	swarm/swarm.go:223	error when shutting down listener: close tcp4 0.0.0.0:36955: use of closed network connection
2023-02-27T17:15:51.990+0300	ERROR	swarm2	swarm/swarm.go:223	error when shutting down listener: close tcp6 [::]:35995: use of closed network connection
--- PASS: TestConnectWithEnabledWSTransports (0.03s)
=== CONT  TestMultipleConnectDisconnect
--- PASS: TestConnectDisconnectEvents (0.22s)
=== CONT  TestDifferentNetworkIDs
--- PASS: TestConnectRepeatHandshake (0.08s)
=== CONT  TestDoubleDisconnect
--- PASS: TestBlocklisting (0.10s)
=== CONT  TestLightPeerLimit
--- PASS: TestDifferentNetworkIDs (0.02s)
=== CONT  TestStreamsMaxIncomingLimit
--- PASS: TestDoubleConnectOnAllAddresses (0.17s)
=== CONT  TestConnectToLightPeer
--- PASS: TestConnectToLightPeer (0.05s)
=== CONT  TestConnectDisconnect
--- PASS: TestMultipleConnectDisconnect (0.11s)
=== CONT  TestNewStreamMulti
--- PASS: TestTopologyNotifier (0.21s)
=== CONT  TestStaticAddressResolver/replace_port
=== CONT  TestStaticAddressResolver/replace_ip_and_port
=== CONT  TestStaticAddressResolver/replace_ip_v6_and_port_with_ip_v4
=== CONT  TestStaticAddressResolver/replace_ip_v4_and_port_with_ip_v6
--- PASS: TestDoubleDisconnect (0.09s)
--- PASS: TestConnectDisconnectOnAllAddresses (0.16s)
=== CONT  TestStaticAddressResolver/replace_ip_v4_and_port_with_dns
=== CONT  TestStaticAddressResolver/replace_ip_v4_with_ip_v6
=== CONT  TestStaticAddressResolver/replace_ip_v6_with_ip_v4
=== CONT  TestStaticAddressResolver/replace_ip_v6_and_port_with_dns_v4
=== CONT  TestStaticAddressResolver/replace_ip_v4
=== CONT  TestStaticAddressResolver/replace_ip_v6
--- PASS: TestStaticAddressResolver (0.00s)
    --- PASS: TestStaticAddressResolver/replace_port (0.00s)
    --- PASS: TestStaticAddressResolver/replace_ip_and_port (0.00s)
    --- PASS: TestStaticAddressResolver/replace_ip_v6_and_port_with_ip_v4 (0.00s)
    --- PASS: TestStaticAddressResolver/replace_ip_v4_and_port_with_ip_v6 (0.00s)
    --- PASS: TestStaticAddressResolver/replace_ip_v4_and_port_with_dns (0.00s)
    --- PASS: TestStaticAddressResolver/replace_ip_v4_with_ip_v6 (0.00s)
    --- PASS: TestStaticAddressResolver/replace_ip_v6_with_ip_v4 (0.00s)
    --- PASS: TestStaticAddressResolver/replace_ip_v6_and_port_with_dns_v4 (0.00s)
    --- PASS: TestStaticAddressResolver/replace_ip_v4 (0.00s)
    --- PASS: TestStaticAddressResolver/replace_ip_v6 (0.00s)
=== CONT  TestDynamicWelcomeMessage/Get_current_message_-_OK
--- PASS: TestDynamicWelcomeMessage (0.31s)
    --- PASS: TestDynamicWelcomeMessage/Set_new_message (0.00s)
        --- PASS: TestDynamicWelcomeMessage/Set_new_message/OK (0.04s)
        --- PASS: TestDynamicWelcomeMessage/Set_new_message/error_-_message_too_long (0.01s)
    --- PASS: TestDynamicWelcomeMessage/Get_current_message_-_OK (0.00s)
--- PASS: TestLightPeerLimit (0.09s)
--- PASS: TestNewStreamMulti (0.02s)
--- PASS: TestConnectDisconnect (0.05s)
--- PASS: TestTopologyAnnounce (0.59s)
--- PASS: TestStreamsMaxIncomingLimit (1.85s)
PASS
ok  	github.com/ethersphere/bee/pkg/p2p/libp2p	(cached)
=== RUN   TestExist
=== PAUSE TestExist
=== RUN   TestPeers
=== PAUSE TestPeers
=== CONT  TestExist
--- PASS: TestExist (0.00s)
=== CONT  TestPeers
--- PASS: TestPeers (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/p2p/libp2p/internal/blocklist	(cached)
=== RUN   TestExecute
=== PAUSE TestExecute
=== RUN   TestClosedUntil
=== PAUSE TestClosedUntil
=== CONT  TestExecute
=== CONT  TestClosedUntil
=== RUN   TestExecute/f()_returns_nil
=== PAUSE TestExecute/f()_returns_nil
--- PASS: TestClosedUntil (0.00s)
=== RUN   TestExecute/f()_returns_error
=== PAUSE TestExecute/f()_returns_error
=== RUN   TestExecute/Break_error
=== PAUSE TestExecute/Break_error
=== RUN   TestExecute/Break_error_-_mix_iterations
=== PAUSE TestExecute/Break_error_-_mix_iterations
=== RUN   TestExecute/Expiration_-_return_f()_error
=== PAUSE TestExecute/Expiration_-_return_f()_error
=== RUN   TestExecute/Backoff_-_close,_reopen,_close,_don't_open
=== PAUSE TestExecute/Backoff_-_close,_reopen,_close,_don't_open
=== CONT  TestExecute/f()_returns_nil
=== CONT  TestExecute/Backoff_-_close,_reopen,_close,_don't_open
=== CONT  TestExecute/Expiration_-_return_f()_error
=== CONT  TestExecute/Break_error_-_mix_iterations
=== CONT  TestExecute/Break_error
=== CONT  TestExecute/f()_returns_error
--- PASS: TestExecute (0.00s)
    --- PASS: TestExecute/f()_returns_nil (0.00s)
    --- PASS: TestExecute/Backoff_-_close,_reopen,_close,_don't_open (0.00s)
    --- PASS: TestExecute/Expiration_-_return_f()_error (0.00s)
    --- PASS: TestExecute/Break_error_-_mix_iterations (0.00s)
    --- PASS: TestExecute/Break_error (0.00s)
    --- PASS: TestExecute/f()_returns_error (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/p2p/libp2p/internal/breaker	(cached)
=== RUN   TestHandshake
=== RUN   TestHandshake/Handshake_-_OK
=== RUN   TestHandshake/Handshake_-_picker_error
=== RUN   TestHandshake/Handshake_-_welcome_message_too_long
=== RUN   TestHandshake/Handshake_-_dynamic_welcome_message_too_long
=== RUN   TestHandshake/Handshake_-_set_welcome_message
=== RUN   TestHandshake/Handshake_-_Syn_write_error
=== RUN   TestHandshake/Handshake_-_Syn_read_error
=== RUN   TestHandshake/Handshake_-_ack_write_error
=== RUN   TestHandshake/Handshake_-_networkID_mismatch
=== RUN   TestHandshake/Handshake_-_invalid_ack
=== RUN   TestHandshake/Handshake_-_error_advertisable_address
=== RUN   TestHandshake/Handle_-_OK
=== RUN   TestHandshake/Handle_-_read_error_
=== RUN   TestHandshake/Handle_-_write_error_
=== RUN   TestHandshake/Handle_-_ack_read_error_
=== RUN   TestHandshake/Handle_-_networkID_mismatch_
=== RUN   TestHandshake/Handle_-_invalid_ack
=== RUN   TestHandshake/Handle_-_advertisable_error
--- PASS: TestHandshake (0.02s)
    --- PASS: TestHandshake/Handshake_-_OK (0.00s)
    --- PASS: TestHandshake/Handshake_-_picker_error (0.00s)
    --- PASS: TestHandshake/Handshake_-_welcome_message_too_long (0.00s)
    --- PASS: TestHandshake/Handshake_-_dynamic_welcome_message_too_long (0.00s)
    --- PASS: TestHandshake/Handshake_-_set_welcome_message (0.00s)
    --- PASS: TestHandshake/Handshake_-_Syn_write_error (0.00s)
    --- PASS: TestHandshake/Handshake_-_Syn_read_error (0.00s)
    --- PASS: TestHandshake/Handshake_-_ack_write_error (0.00s)
    --- PASS: TestHandshake/Handshake_-_networkID_mismatch (0.00s)
    --- PASS: TestHandshake/Handshake_-_invalid_ack (0.00s)
    --- PASS: TestHandshake/Handshake_-_error_advertisable_address (0.00s)
    --- PASS: TestHandshake/Handle_-_OK (0.00s)
    --- PASS: TestHandshake/Handle_-_read_error_ (0.00s)
    --- PASS: TestHandshake/Handle_-_write_error_ (0.00s)
    --- PASS: TestHandshake/Handle_-_ack_read_error_ (0.00s)
    --- PASS: TestHandshake/Handle_-_networkID_mismatch_ (0.00s)
    --- PASS: TestHandshake/Handle_-_invalid_ack (0.00s)
    --- PASS: TestHandshake/Handle_-_advertisable_error (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake	(cached)
=== RUN   TestPingSuccess
=== PAUSE TestPingSuccess
=== RUN   TestDisconnected
=== PAUSE TestDisconnected
=== CONT  TestPingSuccess
=== RUN   TestPingSuccess/ping_success
=== PAUSE TestPingSuccess/ping_success
=== RUN   TestPingSuccess/ping_failure
=== PAUSE TestPingSuccess/ping_failure
=== CONT  TestPingSuccess/ping_success
=== CONT  TestDisconnected
=== CONT  TestPingSuccess/ping_failure
--- PASS: TestPingSuccess (0.00s)
    --- PASS: TestPingSuccess/ping_success (0.00s)
    --- PASS: TestPingSuccess/ping_failure (0.00s)
--- PASS: TestDisconnected (0.05s)
PASS
ok  	github.com/ethersphere/bee/pkg/p2p/libp2p/internal/reacher	(cached)
=== RUN   TestReader_ReadMsg
=== PAUSE TestReader_ReadMsg
=== RUN   TestReader_timeout
=== PAUSE TestReader_timeout
=== RUN   TestWriter
=== PAUSE TestWriter
=== RUN   TestWriter_timeout
=== RUN   TestWriter_timeout/NewWriter
=== RUN   TestWriter_timeout/NewWriter/WithContext
=== PAUSE TestWriter_timeout/NewWriter/WithContext
=== CONT  TestWriter_timeout/NewWriter/WithContext
=== RUN   TestWriter_timeout/NewWriterAndReader
=== RUN   TestWriter_timeout/NewWriterAndReader/WithContext
=== PAUSE TestWriter_timeout/NewWriterAndReader/WithContext
=== CONT  TestWriter_timeout/NewWriterAndReader/WithContext
--- PASS: TestWriter_timeout (1.02s)
    --- PASS: TestWriter_timeout/NewWriter (0.00s)
        --- PASS: TestWriter_timeout/NewWriter/WithContext (0.51s)
    --- PASS: TestWriter_timeout/NewWriterAndReader (0.00s)
        --- PASS: TestWriter_timeout/NewWriterAndReader/WithContext (0.51s)
=== RUN   TestReadMessages
=== PAUSE TestReadMessages
=== CONT  TestReader_ReadMsg
=== RUN   TestReader_ReadMsg/NewReader
=== PAUSE TestReader_ReadMsg/NewReader
=== RUN   TestReader_ReadMsg/NewWriterAndReader
=== PAUSE TestReader_ReadMsg/NewWriterAndReader
=== CONT  TestReader_ReadMsg/NewReader
=== CONT  TestReadMessages
--- PASS: TestReadMessages (0.00s)
=== CONT  TestWriter
=== RUN   TestWriter/NewWriter
=== PAUSE TestWriter/NewWriter
=== RUN   TestWriter/NewWriterAndReader
=== PAUSE TestWriter/NewWriterAndReader
=== CONT  TestWriter/NewWriter
=== CONT  TestReader_timeout
=== RUN   TestReader_timeout/NewReader
=== PAUSE TestReader_timeout/NewReader
=== RUN   TestReader_timeout/NewWriterAndReader
=== PAUSE TestReader_timeout/NewWriterAndReader
=== CONT  TestReader_timeout/NewReader
=== RUN   TestReader_timeout/NewReader/WithContext
=== PAUSE TestReader_timeout/NewReader/WithContext
=== CONT  TestReader_timeout/NewReader/WithContext
=== CONT  TestReader_ReadMsg/NewWriterAndReader
--- PASS: TestReader_ReadMsg (0.00s)
    --- PASS: TestReader_ReadMsg/NewReader (0.00s)
    --- PASS: TestReader_ReadMsg/NewWriterAndReader (0.00s)
=== CONT  TestWriter/NewWriterAndReader
--- PASS: TestWriter (0.00s)
    --- PASS: TestWriter/NewWriter (0.00s)
    --- PASS: TestWriter/NewWriterAndReader (0.00s)
=== CONT  TestReader_timeout/NewWriterAndReader
=== RUN   TestReader_timeout/NewWriterAndReader/WithContext
=== PAUSE TestReader_timeout/NewWriterAndReader/WithContext
=== CONT  TestReader_timeout/NewWriterAndReader/WithContext
--- PASS: TestReader_timeout (0.00s)
    --- PASS: TestReader_timeout/NewReader (0.00s)
        --- PASS: TestReader_timeout/NewReader/WithContext (0.51s)
    --- PASS: TestReader_timeout/NewWriterAndReader (0.00s)
        --- PASS: TestReader_timeout/NewWriterAndReader/WithContext (0.51s)
PASS
ok  	github.com/ethersphere/bee/pkg/p2p/protobuf	(cached)
=== RUN   TestRecorder
=== PAUSE TestRecorder
=== RUN   TestRecorder_errStreamNotSupported
=== PAUSE TestRecorder_errStreamNotSupported
=== RUN   TestRecorder_fullcloseWithRemoteClose
=== PAUSE TestRecorder_fullcloseWithRemoteClose
=== RUN   TestRecorder_fullcloseWithoutRemoteClose
--- PASS: TestRecorder_fullcloseWithoutRemoteClose (0.00s)
=== RUN   TestRecorder_multipleParallelFullCloseAndClose
=== PAUSE TestRecorder_multipleParallelFullCloseAndClose
=== RUN   TestRecorder_closeAfterPartialWrite
=== PAUSE TestRecorder_closeAfterPartialWrite
=== RUN   TestRecorder_resetAfterPartialWrite
=== PAUSE TestRecorder_resetAfterPartialWrite
=== RUN   TestRecorder_withMiddlewares
=== PAUSE TestRecorder_withMiddlewares
=== RUN   TestRecorder_recordErr
=== PAUSE TestRecorder_recordErr
=== RUN   TestRecorder_withPeerProtocols
=== PAUSE TestRecorder_withPeerProtocols
=== RUN   TestRecorder_withStreamError
=== PAUSE TestRecorder_withStreamError
=== RUN   TestRecorder_ping
=== PAUSE TestRecorder_ping
=== CONT  TestRecorder
--- PASS: TestRecorder (0.00s)
=== CONT  TestRecorder_ping
--- PASS: TestRecorder_ping (0.00s)
=== CONT  TestRecorder_withStreamError
--- PASS: TestRecorder_withStreamError (0.00s)
=== CONT  TestRecorder_withPeerProtocols
--- PASS: TestRecorder_withPeerProtocols (0.00s)
=== CONT  TestRecorder_recordErr
--- PASS: TestRecorder_recordErr (0.00s)
=== CONT  TestRecorder_withMiddlewares
--- PASS: TestRecorder_withMiddlewares (0.00s)
=== CONT  TestRecorder_resetAfterPartialWrite
--- PASS: TestRecorder_resetAfterPartialWrite (0.00s)
=== CONT  TestRecorder_closeAfterPartialWrite
--- PASS: TestRecorder_closeAfterPartialWrite (0.00s)
=== CONT  TestRecorder_multipleParallelFullCloseAndClose
--- PASS: TestRecorder_multipleParallelFullCloseAndClose (0.00s)
=== CONT  TestRecorder_fullcloseWithRemoteClose
--- PASS: TestRecorder_fullcloseWithRemoteClose (0.00s)
=== CONT  TestRecorder_errStreamNotSupported
--- PASS: TestRecorder_errStreamNotSupported (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/p2p/streamtest	(cached)
=== RUN   TestPing
=== PAUSE TestPing
=== CONT  TestPing
--- PASS: TestPing (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/pingpong	(cached)
=== RUN   TestPinningService
=== RUN   TestPinningService/create_and_list
=== RUN   TestPinningService/create_idempotent_and_list
=== RUN   TestPinningService/delete_and_has
=== RUN   TestPinningService/delete_idempotent_and_has
--- PASS: TestPinningService (0.00s)
    --- PASS: TestPinningService/create_and_list (0.00s)
    --- PASS: TestPinningService/create_idempotent_and_list (0.00s)
    --- PASS: TestPinningService/delete_and_has (0.00s)
    --- PASS: TestPinningService/delete_idempotent_and_has (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/pinning	(cached)
=== RUN   TestBatchMarshalling
--- PASS: TestBatchMarshalling (0.00s)
=== RUN   TestSaveLoad
--- PASS: TestSaveLoad (0.00s)
=== RUN   TestGetStampIssuer
=== RUN   TestGetStampIssuer/found
=== RUN   TestGetStampIssuer/not_found
=== RUN   TestGetStampIssuer/not_usable
=== RUN   TestGetStampIssuer/recovered
=== RUN   TestGetStampIssuer/topup
=== RUN   TestGetStampIssuer/dilute
--- PASS: TestGetStampIssuer (0.00s)
    --- PASS: TestGetStampIssuer/found (0.00s)
    --- PASS: TestGetStampIssuer/not_found (0.00s)
    --- PASS: TestGetStampIssuer/not_usable (0.00s)
    --- PASS: TestGetStampIssuer/recovered (0.00s)
    --- PASS: TestGetStampIssuer/topup (0.00s)
    --- PASS: TestGetStampIssuer/dilute (0.00s)
=== RUN   TestStampMarshalling
--- PASS: TestStampMarshalling (0.00s)
=== RUN   TestStampIndexMarshalling
--- PASS: TestStampIndexMarshalling (0.00s)
=== RUN   TestValidStamp
--- PASS: TestValidStamp (0.02s)
=== RUN   TestStamperStamping
=== RUN   TestStamperStamping/valid_stamp
=== RUN   TestStamperStamping/bucket_mismatch
=== RUN   TestStamperStamping/invalid_index
=== RUN   TestStamperStamping/bucket_full
=== RUN   TestStamperStamping/owner_mismatch
--- PASS: TestStamperStamping (0.02s)
    --- PASS: TestStamperStamping/valid_stamp (0.00s)
    --- PASS: TestStamperStamping/bucket_mismatch (0.00s)
    --- PASS: TestStamperStamping/invalid_index (0.01s)
    --- PASS: TestStamperStamping/bucket_full (0.01s)
    --- PASS: TestStamperStamping/owner_mismatch (0.00s)
=== RUN   TestStampIssuerMarshalling
--- PASS: TestStampIssuerMarshalling (0.00s)
=== RUN   TestStampItem
=== PAUSE TestStampItem
=== CONT  TestStampItem
=== RUN   TestStampItem/zero_batchID_marshal/unmarshal
=== PAUSE TestStampItem/zero_batchID_marshal/unmarshal
=== RUN   TestStampItem/zero_batchID_clone
=== PAUSE TestStampItem/zero_batchID_clone
=== RUN   TestStampItem/zero_chunkAddress_marshal/unmarshal
=== PAUSE TestStampItem/zero_chunkAddress_marshal/unmarshal
=== RUN   TestStampItem/zero_chunkAddress_clone
=== PAUSE TestStampItem/zero_chunkAddress_clone
=== RUN   TestStampItem/valid_values_marshal/unmarshal
=== PAUSE TestStampItem/valid_values_marshal/unmarshal
=== RUN   TestStampItem/valid_values_clone
=== PAUSE TestStampItem/valid_values_clone
=== RUN   TestStampItem/max_values_marshal/unmarshal
=== PAUSE TestStampItem/max_values_marshal/unmarshal
=== RUN   TestStampItem/max_values_clone
=== PAUSE TestStampItem/max_values_clone
=== RUN   TestStampItem/invalid_size_marshal/unmarshal
=== PAUSE TestStampItem/invalid_size_marshal/unmarshal
=== RUN   TestStampItem/invalid_size_clone
=== PAUSE TestStampItem/invalid_size_clone
=== CONT  TestStampItem/zero_batchID_marshal/unmarshal
=== CONT  TestStampItem/invalid_size_clone
=== CONT  TestStampItem/zero_chunkAddress_clone
=== CONT  TestStampItem/valid_values_marshal/unmarshal
=== CONT  TestStampItem/zero_chunkAddress_marshal/unmarshal
=== CONT  TestStampItem/zero_batchID_clone
=== CONT  TestStampItem/max_values_clone
=== CONT  TestStampItem/invalid_size_marshal/unmarshal
=== CONT  TestStampItem/max_values_marshal/unmarshal
=== CONT  TestStampItem/valid_values_clone
--- PASS: TestStampItem (0.00s)
    --- PASS: TestStampItem/zero_batchID_marshal/unmarshal (0.00s)
    --- PASS: TestStampItem/invalid_size_clone (0.00s)
    --- PASS: TestStampItem/zero_chunkAddress_marshal/unmarshal (0.00s)
    --- PASS: TestStampItem/valid_values_marshal/unmarshal (0.00s)
    --- PASS: TestStampItem/zero_chunkAddress_clone (0.00s)
    --- PASS: TestStampItem/max_values_clone (0.00s)
    --- PASS: TestStampItem/zero_batchID_clone (0.00s)
    --- PASS: TestStampItem/invalid_size_marshal/unmarshal (0.00s)
    --- PASS: TestStampItem/max_values_marshal/unmarshal (0.00s)
    --- PASS: TestStampItem/valid_values_clone (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/postage	(cached)
=== RUN   TestBatchServiceCreate
=== RUN   TestBatchServiceCreate/expect_put_create_put_error
=== RUN   TestBatchServiceCreate/passes
=== RUN   TestBatchServiceCreate/passes_without_recovery
=== RUN   TestBatchServiceCreate/batch_with_near-zero_val_rejected
--- PASS: TestBatchServiceCreate (0.00s)
    --- PASS: TestBatchServiceCreate/expect_put_create_put_error (0.00s)
    --- PASS: TestBatchServiceCreate/passes (0.00s)
    --- PASS: TestBatchServiceCreate/passes_without_recovery (0.00s)
    --- PASS: TestBatchServiceCreate/batch_with_near-zero_val_rejected (0.00s)
=== RUN   TestBatchServiceTopUp
=== RUN   TestBatchServiceTopUp/expect_get_error
=== RUN   TestBatchServiceTopUp/expect_update_error
=== RUN   TestBatchServiceTopUp/passes
=== RUN   TestBatchServiceTopUp/passes_without_BatchEventListener_update
--- PASS: TestBatchServiceTopUp (0.00s)
    --- PASS: TestBatchServiceTopUp/expect_get_error (0.00s)
    --- PASS: TestBatchServiceTopUp/expect_update_error (0.00s)
    --- PASS: TestBatchServiceTopUp/passes (0.00s)
    --- PASS: TestBatchServiceTopUp/passes_without_BatchEventListener_update (0.00s)
=== RUN   TestBatchServiceUpdateDepth
=== RUN   TestBatchServiceUpdateDepth/expect_get_error
=== RUN   TestBatchServiceUpdateDepth/expect_put_error
=== RUN   TestBatchServiceUpdateDepth/passes
=== RUN   TestBatchServiceUpdateDepth/passes_without_BatchEventListener_update
--- PASS: TestBatchServiceUpdateDepth (0.00s)
    --- PASS: TestBatchServiceUpdateDepth/expect_get_error (0.00s)
    --- PASS: TestBatchServiceUpdateDepth/expect_put_error (0.00s)
    --- PASS: TestBatchServiceUpdateDepth/passes (0.00s)
    --- PASS: TestBatchServiceUpdateDepth/passes_without_BatchEventListener_update (0.00s)
=== RUN   TestBatchServiceUpdatePrice
=== RUN   TestBatchServiceUpdatePrice/expect_put_error
=== RUN   TestBatchServiceUpdatePrice/passes
--- PASS: TestBatchServiceUpdatePrice (0.00s)
    --- PASS: TestBatchServiceUpdatePrice/expect_put_error (0.00s)
    --- PASS: TestBatchServiceUpdatePrice/passes (0.00s)
=== RUN   TestBatchServiceUpdateBlockNumber
--- PASS: TestBatchServiceUpdateBlockNumber (0.00s)
=== RUN   TestTransactionOk
--- PASS: TestTransactionOk (0.00s)
=== RUN   TestTransactionError
--- PASS: TestTransactionError (0.00s)
=== RUN   TestChecksum
--- PASS: TestChecksum (0.00s)
=== RUN   TestChecksumResync
--- PASS: TestChecksumResync (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/postage/batchservice	(cached)
=== RUN   TestBatchSave
--- PASS: TestBatchSave (0.03s)
=== RUN   TestBatchUpdate
--- PASS: TestBatchUpdate (0.02s)
=== RUN   TestPutChainState
--- PASS: TestPutChainState (0.01s)
=== RUN   TestUnreserveAndLowerStorageRadius
--- PASS: TestUnreserveAndLowerStorageRadius (0.01s)
=== RUN   TestBatchExpiry
--- PASS: TestBatchExpiry (0.01s)
=== RUN   TestUnexpiredBatch
--- PASS: TestUnexpiredBatch (0.01s)
=== RUN   TestBatchStore_Get
--- PASS: TestBatchStore_Get (0.00s)
=== RUN   TestBatchStore_Iterate
--- PASS: TestBatchStore_Iterate (0.00s)
=== RUN   TestBatchStore_IterateStopsEarly
--- PASS: TestBatchStore_IterateStopsEarly (0.00s)
=== RUN   TestBatchStore_SaveAndUpdate
--- PASS: TestBatchStore_SaveAndUpdate (0.00s)
=== RUN   TestBatchStore_GetChainState
--- PASS: TestBatchStore_GetChainState (0.00s)
=== RUN   TestBatchStore_PutChainState
--- PASS: TestBatchStore_PutChainState (0.00s)
=== RUN   TestBatchStore_SetStorageRadius
--- PASS: TestBatchStore_SetStorageRadius (0.00s)
=== RUN   TestBatchStore_IsWithinRadius
=== PAUSE TestBatchStore_IsWithinRadius
=== RUN   TestBatchStore_Reset
--- PASS: TestBatchStore_Reset (0.01s)
=== CONT  TestBatchStore_IsWithinRadius
--- PASS: TestBatchStore_IsWithinRadius (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/postage/batchstore	(cached)
=== RUN   TestBatchStore
--- PASS: TestBatchStore (0.00s)
=== RUN   TestBatchStorePutChainState
--- PASS: TestBatchStorePutChainState (0.00s)
=== RUN   TestBatchStoreWithBatch
--- PASS: TestBatchStoreWithBatch (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/postage/batchstore/mock	(cached)
=== RUN   TestListener
=== RUN   TestListener/create_event
=== RUN   TestListener/topup_event
=== RUN   TestListener/depthIncrease_event
=== RUN   TestListener/priceUpdate_event
=== RUN   TestListener/multiple_events
=== RUN   TestListener/do_not_shutdown_on_error_event
=== RUN   TestListener/shutdown_on_stalling
=== RUN   TestListener/shutdown_on_processing_error
--- PASS: TestListener (0.05s)
    --- PASS: TestListener/create_event (0.00s)
    --- PASS: TestListener/topup_event (0.00s)
    --- PASS: TestListener/depthIncrease_event (0.00s)
    --- PASS: TestListener/priceUpdate_event (0.00s)
    --- PASS: TestListener/multiple_events (0.00s)
    --- PASS: TestListener/do_not_shutdown_on_error_event (0.00s)
    --- PASS: TestListener/shutdown_on_stalling (0.05s)
    --- PASS: TestListener/shutdown_on_processing_error (0.00s)
=== RUN   TestListenerBatchState
--- PASS: TestListenerBatchState (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/postage/listener	(cached)
=== RUN   TestCreateBatch
=== RUN   TestCreateBatch/ok
=== RUN   TestCreateBatch/invalid_depth
=== RUN   TestCreateBatch/insufficient_funds
--- PASS: TestCreateBatch (0.00s)
    --- PASS: TestCreateBatch/ok (0.00s)
    --- PASS: TestCreateBatch/invalid_depth (0.00s)
    --- PASS: TestCreateBatch/insufficient_funds (0.00s)
=== RUN   TestTopUpBatch
=== RUN   TestTopUpBatch/ok
=== RUN   TestTopUpBatch/batch_doesnt_exist
=== RUN   TestTopUpBatch/insufficient_funds
--- PASS: TestTopUpBatch (0.00s)
    --- PASS: TestTopUpBatch/ok (0.00s)
    --- PASS: TestTopUpBatch/batch_doesnt_exist (0.00s)
    --- PASS: TestTopUpBatch/insufficient_funds (0.00s)
=== RUN   TestDiluteBatch
=== RUN   TestDiluteBatch/ok
=== RUN   TestDiluteBatch/batch_doesnt_exist
=== RUN   TestDiluteBatch/invalid_depth
--- PASS: TestDiluteBatch (0.00s)
    --- PASS: TestDiluteBatch/ok (0.00s)
    --- PASS: TestDiluteBatch/batch_doesnt_exist (0.00s)
    --- PASS: TestDiluteBatch/invalid_depth (0.00s)
=== RUN   TestBatchExpirer
=== PAUSE TestBatchExpirer
=== RUN   TestLookupERC20Address
--- PASS: TestLookupERC20Address (0.00s)
=== CONT  TestBatchExpirer
=== RUN   TestBatchExpirer/ok
=== PAUSE TestBatchExpirer/ok
=== RUN   TestBatchExpirer/wrong_call_data_for_expired_batches_exist
=== PAUSE TestBatchExpirer/wrong_call_data_for_expired_batches_exist
=== RUN   TestBatchExpirer/wrong_call_data_for_expired_limited_batches
=== PAUSE TestBatchExpirer/wrong_call_data_for_expired_limited_batches
=== RUN   TestBatchExpirer/correct_and_incorrect_call_data
=== PAUSE TestBatchExpirer/correct_and_incorrect_call_data
=== RUN   TestBatchExpirer/wrong_result_for_expire_limited_batches
=== PAUSE TestBatchExpirer/wrong_result_for_expire_limited_batches
=== RUN   TestBatchExpirer/wrong_result_for_expired_batches_exist
=== PAUSE TestBatchExpirer/wrong_result_for_expired_batches_exist
=== RUN   TestBatchExpirer/unpack_err_for_expired_batches_exist
=== PAUSE TestBatchExpirer/unpack_err_for_expired_batches_exist
=== RUN   TestBatchExpirer/tx_err_for_expire_limited_batches
=== PAUSE TestBatchExpirer/tx_err_for_expire_limited_batches
=== CONT  TestBatchExpirer/ok
=== CONT  TestBatchExpirer/wrong_result_for_expire_limited_batches
=== CONT  TestBatchExpirer/wrong_call_data_for_expired_batches_exist
=== CONT  TestBatchExpirer/correct_and_incorrect_call_data
=== CONT  TestBatchExpirer/unpack_err_for_expired_batches_exist
=== CONT  TestBatchExpirer/tx_err_for_expire_limited_batches
=== CONT  TestBatchExpirer/wrong_result_for_expired_batches_exist
=== CONT  TestBatchExpirer/wrong_call_data_for_expired_limited_batches
--- PASS: TestBatchExpirer (0.00s)
    --- PASS: TestBatchExpirer/ok (0.00s)
    --- PASS: TestBatchExpirer/wrong_result_for_expire_limited_batches (0.00s)
    --- PASS: TestBatchExpirer/wrong_call_data_for_expired_batches_exist (0.00s)
    --- PASS: TestBatchExpirer/correct_and_incorrect_call_data (0.00s)
    --- PASS: TestBatchExpirer/unpack_err_for_expired_batches_exist (0.00s)
    --- PASS: TestBatchExpirer/tx_err_for_expire_limited_batches (0.00s)
    --- PASS: TestBatchExpirer/wrong_result_for_expired_batches_exist (0.00s)
    --- PASS: TestBatchExpirer/wrong_call_data_for_expired_limited_batches (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/postage/postagecontract	(cached)
=== RUN   TestMakePricingHeaders
=== PAUSE TestMakePricingHeaders
=== RUN   TestMakePricingResponseHeaders
=== PAUSE TestMakePricingResponseHeaders
=== RUN   TestParsePricingHeaders
=== PAUSE TestParsePricingHeaders
=== RUN   TestParsePricingResponseHeaders
=== PAUSE TestParsePricingResponseHeaders
=== RUN   TestParseIndexHeader
=== PAUSE TestParseIndexHeader
=== RUN   TestParseTargetHeader
=== PAUSE TestParseTargetHeader
=== RUN   TestParsePriceHeader
=== PAUSE TestParsePriceHeader
=== RUN   TestReadMalformedHeaders
=== PAUSE TestReadMalformedHeaders
=== CONT  TestMakePricingHeaders
=== CONT  TestParseIndexHeader
=== CONT  TestParseTargetHeader
--- PASS: TestMakePricingHeaders (0.00s)
--- PASS: TestParseTargetHeader (0.00s)
=== CONT  TestParsePricingResponseHeaders
=== CONT  TestParsePricingHeaders
--- PASS: TestParsePricingHeaders (0.00s)
--- PASS: TestParsePricingResponseHeaders (0.00s)
--- PASS: TestParseIndexHeader (0.00s)
=== CONT  TestMakePricingResponseHeaders
--- PASS: TestMakePricingResponseHeaders (0.00s)
=== CONT  TestReadMalformedHeaders
--- PASS: TestReadMalformedHeaders (0.00s)
=== CONT  TestParsePriceHeader
--- PASS: TestParsePriceHeader (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/pricer/headerutils	(cached)
=== RUN   TestAnnouncePaymentThreshold
=== PAUSE TestAnnouncePaymentThreshold
=== RUN   TestAnnouncePaymentWithInsufficientThreshold
=== PAUSE TestAnnouncePaymentWithInsufficientThreshold
=== RUN   TestInitialPaymentThreshold
=== PAUSE TestInitialPaymentThreshold
=== RUN   TestInitialPaymentThresholdLightNode
=== PAUSE TestInitialPaymentThresholdLightNode
=== CONT  TestAnnouncePaymentThreshold
=== CONT  TestInitialPaymentThreshold
=== CONT  TestAnnouncePaymentWithInsufficientThreshold
=== CONT  TestInitialPaymentThresholdLightNode
--- PASS: TestAnnouncePaymentWithInsufficientThreshold (0.00s)
--- PASS: TestAnnouncePaymentThreshold (0.00s)
--- PASS: TestInitialPaymentThresholdLightNode (0.00s)
--- PASS: TestInitialPaymentThreshold (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/pricing	(cached)
=== RUN   TestSend
=== PAUSE TestSend
=== RUN   TestDeliver
=== PAUSE TestDeliver
=== RUN   TestRegister
=== PAUSE TestRegister
=== RUN   TestWrap
=== PAUSE TestWrap
=== RUN   TestUnwrap
=== PAUSE TestUnwrap
=== RUN   TestUnwrapTopicEncrypted
=== PAUSE TestUnwrapTopicEncrypted
=== CONT  TestSend
=== CONT  TestWrap
=== CONT  TestRegister
=== CONT  TestDeliver
=== CONT  TestUnwrapTopicEncrypted
=== CONT  TestUnwrap
--- PASS: TestUnwrapTopicEncrypted (0.02s)
--- PASS: TestDeliver (0.02s)
--- PASS: TestWrap (0.02s)
--- PASS: TestSend (0.03s)
--- PASS: TestRegister (0.04s)
--- PASS: TestUnwrap (0.04s)
PASS
ok  	github.com/ethersphere/bee/pkg/pss	(cached)
=== RUN   TestOneSync
=== PAUSE TestOneSync
=== RUN   TestSyncOutsideDepth
=== PAUSE TestSyncOutsideDepth
=== RUN   TestSyncFlow_PeerWithinDepth_Live
=== PAUSE TestSyncFlow_PeerWithinDepth_Live
=== RUN   TestSyncFlow_PeerWithinDepth_Historical
=== PAUSE TestSyncFlow_PeerWithinDepth_Historical
=== RUN   TestSyncFlow_PeerWithinDepth_Live2
=== PAUSE TestSyncFlow_PeerWithinDepth_Live2
=== RUN   TestPeerDisconnected
=== PAUSE TestPeerDisconnected
=== RUN   TestBinReset
=== PAUSE TestBinReset
=== RUN   TestDepthChange
=== PAUSE TestDepthChange
=== RUN   TestContinueSyncing
=== PAUSE TestContinueSyncing
=== RUN   TestPeerGone
=== PAUSE TestPeerGone
=== CONT  TestOneSync
=== CONT  TestPeerGone
=== CONT  TestContinueSyncing
=== CONT  TestDepthChange
    puller_test.go:393: this test always panics
--- SKIP: TestDepthChange (0.00s)
=== CONT  TestBinReset
=== CONT  TestPeerDisconnected
=== CONT  TestSyncFlow_PeerWithinDepth_Live2
=== CONT  TestSyncFlow_PeerWithinDepth_Historical
=== RUN   TestSyncFlow_PeerWithinDepth_Historical/1,1_-_1_call
=== PAUSE TestSyncFlow_PeerWithinDepth_Historical/1,1_-_1_call
=== RUN   TestSyncFlow_PeerWithinDepth_Historical/1,10_-_1_call
=== PAUSE TestSyncFlow_PeerWithinDepth_Historical/1,10_-_1_call
=== RUN   TestSyncFlow_PeerWithinDepth_Historical/1,50_-_1_call
=== PAUSE TestSyncFlow_PeerWithinDepth_Historical/1,50_-_1_call
=== RUN   TestSyncFlow_PeerWithinDepth_Historical/1,51_-_2_calls
=== PAUSE TestSyncFlow_PeerWithinDepth_Historical/1,51_-_2_calls
=== RUN   TestSyncFlow_PeerWithinDepth_Historical/1,100_-_2_calls
=== PAUSE TestSyncFlow_PeerWithinDepth_Historical/1,100_-_2_calls
=== RUN   TestSyncFlow_PeerWithinDepth_Historical/1,200_-_4_calls
=== PAUSE TestSyncFlow_PeerWithinDepth_Historical/1,200_-_4_calls
=== CONT  TestSyncFlow_PeerWithinDepth_Historical/1,1_-_1_call
=== CONT  TestSyncFlow_PeerWithinDepth_Live
=== RUN   TestSyncFlow_PeerWithinDepth_Live/cursor_0,_1_chunk_on_live
=== RUN   TestSyncFlow_PeerWithinDepth_Live2/cursor_0,_1_chunk_on_live
=== PAUSE TestSyncFlow_PeerWithinDepth_Live/cursor_0,_1_chunk_on_live
=== RUN   TestSyncFlow_PeerWithinDepth_Live/cursor_0_-_calls_1-1,_2-5,_6-10
=== PAUSE TestSyncFlow_PeerWithinDepth_Live/cursor_0_-_calls_1-1,_2-5,_6-10
=== CONT  TestSyncOutsideDepth
=== PAUSE TestSyncFlow_PeerWithinDepth_Live2/cursor_0,_1_chunk_on_live
=== CONT  TestSyncFlow_PeerWithinDepth_Historical/1,200_-_4_calls
--- PASS: TestOneSync (0.14s)
=== CONT  TestSyncFlow_PeerWithinDepth_Historical/1,100_-_2_calls
=== CONT  TestSyncFlow_PeerWithinDepth_Historical/1,51_-_2_calls
--- PASS: TestPeerDisconnected (0.14s)
=== CONT  TestSyncFlow_PeerWithinDepth_Historical/1,50_-_1_call
--- PASS: TestSyncOutsideDepth (0.18s)
=== CONT  TestSyncFlow_PeerWithinDepth_Historical/1,10_-_1_call
=== CONT  TestSyncFlow_PeerWithinDepth_Live/cursor_0,_1_chunk_on_live
--- PASS: TestBinReset (0.24s)
=== CONT  TestSyncFlow_PeerWithinDepth_Live2/cursor_0,_1_chunk_on_live
--- PASS: TestPeerGone (0.30s)
=== CONT  TestSyncFlow_PeerWithinDepth_Live/cursor_0_-_calls_1-1,_2-5,_6-10
--- PASS: TestSyncFlow_PeerWithinDepth_Historical (0.00s)
    --- PASS: TestSyncFlow_PeerWithinDepth_Historical/1,1_-_1_call (0.18s)
    --- PASS: TestSyncFlow_PeerWithinDepth_Historical/1,200_-_4_calls (0.18s)
    --- PASS: TestSyncFlow_PeerWithinDepth_Historical/1,100_-_2_calls (0.18s)
    --- PASS: TestSyncFlow_PeerWithinDepth_Historical/1,51_-_2_calls (0.18s)
    --- PASS: TestSyncFlow_PeerWithinDepth_Historical/1,50_-_1_call (0.18s)
    --- PASS: TestSyncFlow_PeerWithinDepth_Historical/1,10_-_1_call (0.18s)
--- PASS: TestSyncFlow_PeerWithinDepth_Live (0.01s)
    --- PASS: TestSyncFlow_PeerWithinDepth_Live/cursor_0,_1_chunk_on_live (0.18s)
    --- PASS: TestSyncFlow_PeerWithinDepth_Live/cursor_0_-_calls_1-1,_2-5,_6-10 (0.19s)
--- PASS: TestSyncFlow_PeerWithinDepth_Live2 (0.01s)
    --- PASS: TestSyncFlow_PeerWithinDepth_Live2/cursor_0,_1_chunk_on_live (0.26s)
--- PASS: TestContinueSyncing (1.10s)
PASS
ok  	github.com/ethersphere/bee/pkg/puller	(cached)
=== RUN   TestIncoming_WantEmptyInterval
=== PAUSE TestIncoming_WantEmptyInterval
=== RUN   TestIncoming_WantNone
=== PAUSE TestIncoming_WantNone
=== RUN   TestIncoming_ContextTimeout
=== PAUSE TestIncoming_ContextTimeout
=== RUN   TestIncoming_WantOne
=== PAUSE TestIncoming_WantOne
=== RUN   TestIncoming_WantAll
=== PAUSE TestIncoming_WantAll
=== RUN   TestIncoming_UnsolicitedChunk
=== PAUSE TestIncoming_UnsolicitedChunk
=== RUN   TestGetCursors
=== PAUSE TestGetCursors
=== RUN   TestGetCursorsError
=== PAUSE TestGetCursorsError
=== CONT  TestIncoming_WantEmptyInterval
=== CONT  TestGetCursorsError
=== CONT  TestGetCursors
=== CONT  TestIncoming_UnsolicitedChunk
=== CONT  TestIncoming_WantAll
=== CONT  TestIncoming_WantOne
=== CONT  TestIncoming_ContextTimeout
--- PASS: TestIncoming_ContextTimeout (0.00s)
=== CONT  TestIncoming_WantNone
--- PASS: TestGetCursorsError (0.00s)
--- PASS: TestGetCursors (0.00s)
--- PASS: TestIncoming_WantNone (0.00s)
--- PASS: TestIncoming_WantOne (0.00s)
--- PASS: TestIncoming_WantEmptyInterval (0.01s)
--- PASS: TestIncoming_WantAll (0.01s)
--- PASS: TestIncoming_UnsolicitedChunk (0.01s)
PASS
ok  	github.com/ethersphere/bee/pkg/pullsync	(cached)
ok  	github.com/ethersphere/bee/pkg/pullsync/pullstorage	0.016s
=== RUN   TestSendChunkToSyncWithTag
=== PAUSE TestSendChunkToSyncWithTag
=== RUN   TestSendChunkToPushSyncWithoutTag
=== PAUSE TestSendChunkToPushSyncWithoutTag
=== RUN   TestSendChunkToPushSyncViaApiChannel
=== PAUSE TestSendChunkToPushSyncViaApiChannel
=== RUN   TestSendChunkToPushSyncDirect
=== PAUSE TestSendChunkToPushSyncDirect
=== RUN   TestSendChunkAndReceiveInvalidReceipt
=== PAUSE TestSendChunkAndReceiveInvalidReceipt
=== RUN   TestSendChunkAndTimeoutinReceivingReceipt
=== PAUSE TestSendChunkAndTimeoutinReceivingReceipt
=== RUN   TestPusherRetryShallow
=== PAUSE TestPusherRetryShallow
=== RUN   TestChunkWithInvalidStampSkipped
=== PAUSE TestChunkWithInvalidStampSkipped
=== CONT  TestSendChunkToSyncWithTag
=== CONT  TestChunkWithInvalidStampSkipped
=== CONT  TestPusherRetryShallow
=== CONT  TestSendChunkToPushSyncViaApiChannel
=== CONT  TestSendChunkAndTimeoutinReceivingReceipt
=== CONT  TestSendChunkAndReceiveInvalidReceipt
=== CONT  TestSendChunkToPushSyncWithoutTag
=== CONT  TestSendChunkToPushSyncDirect
--- PASS: TestSendChunkToSyncWithTag (0.04s)
--- PASS: TestChunkWithInvalidStampSkipped (0.04s)
--- PASS: TestSendChunkToPushSyncViaApiChannel (0.04s)
--- PASS: TestSendChunkToPushSyncWithoutTag (0.04s)
--- PASS: TestSendChunkToPushSyncDirect (1.01s)
--- PASS: TestPusherRetryShallow (2.04s)
--- PASS: TestSendChunkAndReceiveInvalidReceipt (3.01s)
--- PASS: TestSendChunkAndTimeoutinReceivingReceipt (5.03s)
PASS
ok  	github.com/ethersphere/bee/pkg/pusher	(cached)
=== RUN   TestPushClosest
=== PAUSE TestPushClosest
=== RUN   TestReplicateBeforeReceipt
=== PAUSE TestReplicateBeforeReceipt
=== RUN   TestPushChunkToClosest
=== PAUSE TestPushChunkToClosest
=== RUN   TestPushChunkToNextClosest
=== PAUSE TestPushChunkToNextClosest
=== RUN   TestPushChunkToClosestErrorAttemptRetry
=== PAUSE TestPushChunkToClosestErrorAttemptRetry
=== RUN   TestHandler
=== PAUSE TestHandler
=== RUN   TestSignsReceipt
=== PAUSE TestSignsReceipt
=== RUN   TestPeerSkipList
=== PAUSE TestPeerSkipList
=== RUN   TestPushChunkToClosestSkipError
=== PAUSE TestPushChunkToClosestSkipError
=== CONT  TestPushClosest
=== CONT  TestHandler
=== CONT  TestPushChunkToClosestErrorAttemptRetry
=== CONT  TestPushChunkToClosestSkipError
=== CONT  TestSignsReceipt
=== CONT  TestPushChunkToClosest
=== CONT  TestPushChunkToNextClosest
=== CONT  TestPeerSkipList
--- PASS: TestSignsReceipt (0.00s)
=== CONT  TestReplicateBeforeReceipt
--- PASS: TestPeerSkipList (0.01s)
--- PASS: TestPushClosest (0.04s)
--- PASS: TestPushChunkToClosestSkipError (0.05s)
--- PASS: TestPushChunkToClosest (0.04s)
--- PASS: TestPushChunkToNextClosest (0.04s)
--- PASS: TestPushChunkToClosestErrorAttemptRetry (0.05s)
--- PASS: TestHandler (0.09s)
--- PASS: TestReplicateBeforeReceipt (0.18s)
PASS
ok  	github.com/ethersphere/bee/pkg/pushsync	(cached)
=== RUN   TestRateFirstBucket
=== PAUSE TestRateFirstBucket
=== RUN   TestIgnoreOldBuckets
=== PAUSE TestIgnoreOldBuckets
=== RUN   TestRate
=== PAUSE TestRate
=== CONT  TestRateFirstBucket
--- PASS: TestRateFirstBucket (0.00s)
=== CONT  TestIgnoreOldBuckets
--- PASS: TestIgnoreOldBuckets (0.00s)
=== CONT  TestRate
--- PASS: TestRate (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/rate	(cached)
=== RUN   TestRateLimit
=== PAUSE TestRateLimit
=== CONT  TestRateLimit
--- PASS: TestRateLimit (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/ratelimit	(cached)
=== RUN   TestCIDResolver
=== PAUSE TestCIDResolver
=== CONT  TestCIDResolver
=== RUN   TestCIDResolver/resolve_manifest_CID
=== PAUSE TestCIDResolver/resolve_manifest_CID
=== RUN   TestCIDResolver/resolve_feed_CID
=== PAUSE TestCIDResolver/resolve_feed_CID
=== RUN   TestCIDResolver/fail_other_codecs
=== PAUSE TestCIDResolver/fail_other_codecs
=== RUN   TestCIDResolver/fail_on_invalid_CID
=== PAUSE TestCIDResolver/fail_on_invalid_CID
=== CONT  TestCIDResolver/resolve_manifest_CID
=== CONT  TestCIDResolver/fail_other_codecs
=== CONT  TestCIDResolver/resolve_feed_CID
=== CONT  TestCIDResolver/fail_on_invalid_CID
--- PASS: TestCIDResolver (0.00s)
    --- PASS: TestCIDResolver/resolve_manifest_CID (0.00s)
    --- PASS: TestCIDResolver/resolve_feed_CID (0.00s)
    --- PASS: TestCIDResolver/fail_other_codecs (0.00s)
    --- PASS: TestCIDResolver/fail_on_invalid_CID (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/resolver/cidv1	(cached)
=== RUN   TestNewENSClient
=== PAUSE TestNewENSClient
=== RUN   TestClose
=== PAUSE TestClose
=== RUN   TestResolve
=== PAUSE TestResolve
=== CONT  TestNewENSClient
=== CONT  TestResolve
=== RUN   TestResolve/nil_resolve_function
=== PAUSE TestResolve/nil_resolve_function
=== CONT  TestClose
=== RUN   TestResolve/resolve_function_internal_error
=== PAUSE TestResolve/resolve_function_internal_error
=== RUN   TestNewENSClient/nil_dial_function
=== PAUSE TestNewENSClient/nil_dial_function
=== RUN   TestNewENSClient/error_in_dial_function
=== PAUSE TestNewENSClient/error_in_dial_function
=== RUN   TestNewENSClient/regular_endpoint
=== PAUSE TestNewENSClient/regular_endpoint
=== CONT  TestNewENSClient/nil_dial_function
=== CONT  TestNewENSClient/regular_endpoint
=== RUN   TestClose/connected
=== PAUSE TestClose/connected
=== RUN   TestClose/not_connected
=== PAUSE TestClose/not_connected
=== CONT  TestClose/connected
=== RUN   TestResolve/resolver_returns_empty_string
=== CONT  TestClose/not_connected
=== PAUSE TestResolve/resolver_returns_empty_string
--- PASS: TestClose (0.00s)
    --- PASS: TestClose/not_connected (0.00s)
    --- PASS: TestClose/connected (0.00s)
=== CONT  TestNewENSClient/error_in_dial_function
=== RUN   TestResolve/resolve_does_not_prefix_address_with_/swarm
=== PAUSE TestResolve/resolve_does_not_prefix_address_with_/swarm
=== RUN   TestResolve/resolve_returns_prefixed_address
=== PAUSE TestResolve/resolve_returns_prefixed_address
=== RUN   TestResolve/expect_properly_set_contract_address
=== PAUSE TestResolve/expect_properly_set_contract_address
=== CONT  TestResolve/nil_resolve_function
=== CONT  TestResolve/resolve_does_not_prefix_address_with_/swarm
=== CONT  TestResolve/resolve_returns_prefixed_address
--- PASS: TestNewENSClient (0.00s)
    --- PASS: TestNewENSClient/regular_endpoint (0.00s)
    --- PASS: TestNewENSClient/nil_dial_function (0.00s)
    --- PASS: TestNewENSClient/error_in_dial_function (0.00s)
=== CONT  TestResolve/expect_properly_set_contract_address
=== CONT  TestResolve/resolver_returns_empty_string
=== CONT  TestResolve/resolve_function_internal_error
--- PASS: TestResolve (0.00s)
    --- PASS: TestResolve/nil_resolve_function (0.00s)
    --- PASS: TestResolve/resolve_does_not_prefix_address_with_/swarm (0.00s)
    --- PASS: TestResolve/resolve_returns_prefixed_address (0.00s)
    --- PASS: TestResolve/expect_properly_set_contract_address (0.00s)
    --- PASS: TestResolve/resolver_returns_empty_string (0.00s)
    --- PASS: TestResolve/resolve_function_internal_error (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/resolver/client/ens	(cached)
=== RUN   TestParseConnectionStrings
=== PAUSE TestParseConnectionStrings
=== RUN   TestMultiresolverOpts
=== PAUSE TestMultiresolverOpts
=== RUN   TestPushResolver
=== PAUSE TestPushResolver
=== RUN   TestResolve
=== PAUSE TestResolve
=== CONT  TestParseConnectionStrings
=== RUN   TestParseConnectionStrings/tld_too_long
=== CONT  TestPushResolver
=== PAUSE TestParseConnectionStrings/tld_too_long
=== RUN   TestParseConnectionStrings/single_endpoint_default_tld
=== PAUSE TestParseConnectionStrings/single_endpoint_default_tld
=== RUN   TestParseConnectionStrings/single_endpoint_explicit_tld
=== PAUSE TestParseConnectionStrings/single_endpoint_explicit_tld
=== CONT  TestResolve
=== RUN   TestResolve/#00
=== PAUSE TestResolve/#00
=== RUN   TestResolve/hello
=== PAUSE TestResolve/hello
=== RUN   TestParseConnectionStrings/single_endpoint_with_address_default_tld
=== PAUSE TestParseConnectionStrings/single_endpoint_with_address_default_tld
=== RUN   TestParseConnectionStrings/single_endpoint_with_address_explicit_tld
=== PAUSE TestParseConnectionStrings/single_endpoint_with_address_explicit_tld
=== RUN   TestParseConnectionStrings/mixed
=== PAUSE TestParseConnectionStrings/mixed
=== RUN   TestParseConnectionStrings/mixed_with_error
=== CONT  TestMultiresolverOpts
=== PAUSE TestParseConnectionStrings/mixed_with_error
=== CONT  TestParseConnectionStrings/tld_too_long
=== CONT  TestParseConnectionStrings/mixed_with_error
=== CONT  TestParseConnectionStrings/mixed
=== CONT  TestParseConnectionStrings/single_endpoint_explicit_tld
=== RUN   TestPushResolver/empty_string,_default
--- PASS: TestMultiresolverOpts (0.00s)
=== PAUSE TestPushResolver/empty_string,_default
=== RUN   TestPushResolver/regular_tld,_named_chain
=== PAUSE TestPushResolver/regular_tld,_named_chain
=== RUN   TestResolve/example.tld
=== RUN   TestPushResolver/pop_empty_chain
=== PAUSE TestPushResolver/pop_empty_chain
=== CONT  TestPushResolver/empty_string,_default
=== CONT  TestParseConnectionStrings/single_endpoint_with_address_default_tld
=== PAUSE TestResolve/example.tld
=== RUN   TestResolve/.tld
=== PAUSE TestResolve/.tld
=== RUN   TestResolve/get.good
=== PAUSE TestResolve/get.good
=== RUN   TestResolve/this.empty
=== PAUSE TestResolve/this.empty
=== RUN   TestResolve/this.dies
=== PAUSE TestResolve/this.dies
=== CONT  TestPushResolver/regular_tld,_named_chain
=== RUN   TestResolve/iam.unregistered
=== PAUSE TestResolve/iam.unregistered
=== RUN   TestResolve/close_all
=== CONT  TestResolve/#00
=== CONT  TestParseConnectionStrings/single_endpoint_with_address_explicit_tld
=== CONT  TestPushResolver/pop_empty_chain
=== CONT  TestParseConnectionStrings/single_endpoint_default_tld
=== CONT  TestResolve/get.good
=== CONT  TestResolve/iam.unregistered
=== CONT  TestResolve/this.dies
=== CONT  TestResolve/this.empty
=== CONT  TestResolve/example.tld
=== CONT  TestResolve/.tld
--- PASS: TestPushResolver (0.00s)
    --- PASS: TestPushResolver/empty_string,_default (0.00s)
    --- PASS: TestPushResolver/regular_tld,_named_chain (0.00s)
    --- PASS: TestPushResolver/pop_empty_chain (0.00s)
--- PASS: TestParseConnectionStrings (0.00s)
    --- PASS: TestParseConnectionStrings/tld_too_long (0.00s)
    --- PASS: TestParseConnectionStrings/mixed_with_error (0.00s)
    --- PASS: TestParseConnectionStrings/single_endpoint_explicit_tld (0.00s)
    --- PASS: TestParseConnectionStrings/mixed (0.00s)
    --- PASS: TestParseConnectionStrings/single_endpoint_with_address_default_tld (0.00s)
    --- PASS: TestParseConnectionStrings/single_endpoint_with_address_explicit_tld (0.00s)
    --- PASS: TestParseConnectionStrings/single_endpoint_default_tld (0.00s)
=== CONT  TestResolve/hello
--- PASS: TestResolve (0.00s)
    --- PASS: TestResolve/close_all (0.00s)
    --- PASS: TestResolve/#00 (0.00s)
    --- PASS: TestResolve/get.good (0.00s)
    --- PASS: TestResolve/iam.unregistered (0.00s)
    --- PASS: TestResolve/this.dies (0.00s)
    --- PASS: TestResolve/this.empty (0.00s)
    --- PASS: TestResolve/example.tld (0.00s)
    --- PASS: TestResolve/.tld (0.00s)
    --- PASS: TestResolve/hello (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/resolver/multiresolver	(cached)
=== RUN   TestDelivery
=== PAUSE TestDelivery
=== RUN   TestWaitForInflight
=== PAUSE TestWaitForInflight
=== RUN   TestRetrieveChunk
=== PAUSE TestRetrieveChunk
=== RUN   TestRetrievePreemptiveRetry
=== PAUSE TestRetrievePreemptiveRetry
=== RUN   TestClosestPeer
=== PAUSE TestClosestPeer
=== CONT  TestDelivery
=== CONT  TestClosestPeer
=== CONT  TestRetrieveChunk
=== RUN   TestRetrieveChunk/downstream
=== PAUSE TestRetrieveChunk/downstream
=== CONT  TestWaitForInflight
=== RUN   TestClosestPeer/closest
=== PAUSE TestClosestPeer/closest
=== RUN   TestClosestPeer/second_closest
=== PAUSE TestClosestPeer/second_closest
=== RUN   TestClosestPeer/closest_is_further_than_base_addr
=== PAUSE TestClosestPeer/closest_is_further_than_base_addr
=== CONT  TestClosestPeer/closest
=== CONT  TestClosestPeer/closest_is_further_than_base_addr
=== CONT  TestClosestPeer/second_closest
--- PASS: TestClosestPeer (0.00s)
    --- PASS: TestClosestPeer/closest (0.00s)
    --- PASS: TestClosestPeer/closest_is_further_than_base_addr (0.00s)
    --- PASS: TestClosestPeer/second_closest (0.00s)
=== RUN   TestRetrieveChunk/forward
=== PAUSE TestRetrieveChunk/forward
=== CONT  TestRetrieveChunk/downstream
=== CONT  TestRetrieveChunk/forward
--- PASS: TestDelivery (0.00s)
=== CONT  TestRetrievePreemptiveRetry
=== RUN   TestRetrievePreemptiveRetry/peer_not_reachable
=== PAUSE TestRetrievePreemptiveRetry/peer_not_reachable
=== RUN   TestRetrievePreemptiveRetry/peer_does_not_have_chunk
=== PAUSE TestRetrievePreemptiveRetry/peer_does_not_have_chunk
=== RUN   TestRetrievePreemptiveRetry/one_peer_is_slower
=== PAUSE TestRetrievePreemptiveRetry/one_peer_is_slower
=== RUN   TestRetrievePreemptiveRetry/peer_forwards_request
=== PAUSE TestRetrievePreemptiveRetry/peer_forwards_request
=== CONT  TestRetrievePreemptiveRetry/peer_not_reachable
=== CONT  TestRetrievePreemptiveRetry/peer_forwards_request
=== CONT  TestRetrievePreemptiveRetry/one_peer_is_slower
=== CONT  TestRetrievePreemptiveRetry/peer_does_not_have_chunk
--- PASS: TestRetrieveChunk (0.00s)
    --- PASS: TestRetrieveChunk/downstream (0.00s)
    --- PASS: TestRetrieveChunk/forward (0.02s)
--- PASS: TestWaitForInflight (2.00s)
--- PASS: TestRetrievePreemptiveRetry (0.00s)
    --- PASS: TestRetrievePreemptiveRetry/peer_does_not_have_chunk (0.00s)
    --- PASS: TestRetrievePreemptiveRetry/peer_forwards_request (0.02s)
    --- PASS: TestRetrievePreemptiveRetry/peer_not_reachable (1.00s)
    --- PASS: TestRetrievePreemptiveRetry/one_peer_is_slower (3.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/retrieval	(cached)
=== RUN   TestPayment
=== PAUSE TestPayment
=== RUN   TestTimeLimitedPayment
=== PAUSE TestTimeLimitedPayment
=== RUN   TestTimeLimitedPaymentLight
=== PAUSE TestTimeLimitedPaymentLight
=== CONT  TestPayment
=== CONT  TestTimeLimitedPaymentLight
=== CONT  TestTimeLimitedPayment
--- PASS: TestPayment (0.00s)
--- PASS: TestTimeLimitedPayment (1.00s)
--- PASS: TestTimeLimitedPaymentLight (1.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/settlement/pseudosettle	(cached)
=== RUN   TestReceiveCheque
=== PAUSE TestReceiveCheque
=== RUN   TestReceiveChequeReject
=== PAUSE TestReceiveChequeReject
=== RUN   TestReceiveChequeWrongChequebook
=== PAUSE TestReceiveChequeWrongChequebook
=== RUN   TestPay
=== PAUSE TestPay
=== RUN   TestPayIssueError
=== PAUSE TestPayIssueError
=== RUN   TestPayUnknownBeneficiary
=== PAUSE TestPayUnknownBeneficiary
=== RUN   TestHandshake
=== PAUSE TestHandshake
=== RUN   TestHandshakeNewPeer
=== PAUSE TestHandshakeNewPeer
=== RUN   TestMigratePeer
=== PAUSE TestMigratePeer
=== RUN   TestCashout
=== PAUSE TestCashout
=== RUN   TestCashoutStatus
=== PAUSE TestCashoutStatus
=== RUN   TestStateStoreKeys
=== PAUSE TestStateStoreKeys
=== CONT  TestReceiveCheque
--- PASS: TestReceiveCheque (0.00s)
=== CONT  TestStateStoreKeys
--- PASS: TestStateStoreKeys (0.00s)
=== CONT  TestCashoutStatus
--- PASS: TestCashoutStatus (0.00s)
=== CONT  TestCashout
--- PASS: TestCashout (0.00s)
=== CONT  TestMigratePeer
--- PASS: TestMigratePeer (0.00s)
=== CONT  TestHandshakeNewPeer
--- PASS: TestHandshakeNewPeer (0.00s)
=== CONT  TestHandshake
--- PASS: TestHandshake (0.00s)
=== CONT  TestPayUnknownBeneficiary
--- PASS: TestPayUnknownBeneficiary (0.00s)
=== CONT  TestPayIssueError
--- PASS: TestPayIssueError (0.00s)
=== CONT  TestPay
--- PASS: TestPay (0.00s)
=== CONT  TestReceiveChequeWrongChequebook
--- PASS: TestReceiveChequeWrongChequebook (0.00s)
=== CONT  TestReceiveChequeReject
--- PASS: TestReceiveChequeReject (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/settlement/swap	(cached)
=== RUN   TestCashout
=== PAUSE TestCashout
=== RUN   TestCashoutBounced
=== PAUSE TestCashoutBounced
=== RUN   TestCashoutStatusReverted
=== PAUSE TestCashoutStatusReverted
=== RUN   TestCashoutStatusPending
=== PAUSE TestCashoutStatusPending
=== RUN   TestSignCheque
=== PAUSE TestSignCheque
=== RUN   TestSignChequeIntegration
=== PAUSE TestSignChequeIntegration
=== RUN   TestChequebookAddress
=== PAUSE TestChequebookAddress
=== RUN   TestChequebookBalance
=== PAUSE TestChequebookBalance
=== RUN   TestChequebookDeposit
=== PAUSE TestChequebookDeposit
=== RUN   TestChequebookWaitForDeposit
=== PAUSE TestChequebookWaitForDeposit
=== RUN   TestChequebookWaitForDepositReverted
=== PAUSE TestChequebookWaitForDepositReverted
=== RUN   TestChequebookIssue
=== PAUSE TestChequebookIssue
=== RUN   TestChequebookIssueErrorSend
=== PAUSE TestChequebookIssueErrorSend
=== RUN   TestChequebookIssueOutOfFunds
=== PAUSE TestChequebookIssueOutOfFunds
=== RUN   TestChequebookWithdraw
=== PAUSE TestChequebookWithdraw
=== RUN   TestChequebookWithdrawInsufficientFunds
=== PAUSE TestChequebookWithdrawInsufficientFunds
=== RUN   TestStateStoreKeys
=== PAUSE TestStateStoreKeys
=== RUN   TestReceiveCheque
=== PAUSE TestReceiveCheque
=== RUN   TestReceiveChequeInvalidBeneficiary
=== PAUSE TestReceiveChequeInvalidBeneficiary
=== RUN   TestReceiveChequeInvalidAmount
=== PAUSE TestReceiveChequeInvalidAmount
=== RUN   TestReceiveChequeInvalidChequebook
=== PAUSE TestReceiveChequeInvalidChequebook
=== RUN   TestReceiveChequeInvalidSignature
=== PAUSE TestReceiveChequeInvalidSignature
=== RUN   TestReceiveChequeInsufficientBalance
=== PAUSE TestReceiveChequeInsufficientBalance
=== RUN   TestReceiveChequeSufficientBalancePaidOut
=== PAUSE TestReceiveChequeSufficientBalancePaidOut
=== RUN   TestReceiveChequeNotEnoughValue
=== PAUSE TestReceiveChequeNotEnoughValue
=== RUN   TestReceiveChequeNotEnoughValueAfterDeduction
=== PAUSE TestReceiveChequeNotEnoughValueAfterDeduction
=== RUN   TestFactoryERC20Address
=== PAUSE TestFactoryERC20Address
=== RUN   TestFactoryVerifySelf
=== PAUSE TestFactoryVerifySelf
=== RUN   TestFactoryVerifyChequebook
=== PAUSE TestFactoryVerifyChequebook
=== RUN   TestFactoryDeploy
=== PAUSE TestFactoryDeploy
=== RUN   TestFactoryDeployReverted
=== PAUSE TestFactoryDeployReverted
=== CONT  TestCashout
=== CONT  TestStateStoreKeys
=== CONT  TestChequebookDeposit
--- PASS: TestChequebookDeposit (0.00s)
--- PASS: TestStateStoreKeys (0.00s)
=== CONT  TestFactoryDeployReverted
--- PASS: TestFactoryDeployReverted (0.00s)
=== CONT  TestReceiveChequeSufficientBalancePaidOut
--- PASS: TestCashout (0.00s)
=== CONT  TestReceiveChequeInvalidSignature
=== CONT  TestReceiveChequeInvalidChequebook
=== CONT  TestReceiveChequeInvalidAmount
=== CONT  TestFactoryVerifyChequebook
=== RUN   TestFactoryVerifyChequebook/valid
=== PAUSE TestFactoryVerifyChequebook/valid
=== RUN   TestFactoryVerifyChequebook/valid_legacy
=== PAUSE TestFactoryVerifyChequebook/valid_legacy
=== RUN   TestFactoryVerifyChequebook/invalid
=== CONT  TestFactoryVerifySelf
=== PAUSE TestFactoryVerifyChequebook/invalid
=== RUN   TestFactoryVerifySelf/valid
=== PAUSE TestFactoryVerifySelf/valid
=== RUN   TestFactoryVerifySelf/invalid_deploy_factory
=== PAUSE TestFactoryVerifySelf/invalid_deploy_factory
=== RUN   TestFactoryVerifySelf/invalid_legacy_factories
=== PAUSE TestFactoryVerifySelf/invalid_legacy_factories
=== CONT  TestReceiveCheque
=== CONT  TestCashoutStatusReverted
=== CONT  TestCashoutStatusPending
=== CONT  TestChequebookIssueErrorSend
=== CONT  TestChequebookWithdrawInsufficientFunds
=== CONT  TestFactoryDeploy
=== CONT  TestChequebookWithdraw
=== CONT  TestChequebookIssue
=== CONT  TestChequebookIssueOutOfFunds
=== CONT  TestChequebookAddress
=== CONT  TestChequebookBalance
=== CONT  TestSignChequeIntegration
=== CONT  TestCashoutBounced
=== CONT  TestChequebookWaitForDeposit
=== CONT  TestSignCheque
=== CONT  TestReceiveChequeInvalidBeneficiary
=== CONT  TestFactoryVerifyChequebook/valid
=== CONT  TestFactoryVerifyChequebook/valid_legacy
=== CONT  TestFactoryVerifySelf/valid
=== CONT  TestFactoryVerifyChequebook/invalid
=== CONT  TestFactoryVerifySelf/invalid_legacy_factories
=== CONT  TestFactoryVerifySelf/invalid_deploy_factory
=== CONT  TestFactoryERC20Address
=== CONT  TestReceiveChequeNotEnoughValue
=== CONT  TestChequebookWaitForDepositReverted
=== CONT  TestReceiveChequeInsufficientBalance
--- PASS: TestReceiveChequeSufficientBalancePaidOut (0.00s)
--- PASS: TestReceiveChequeInvalidSignature (0.00s)
--- PASS: TestReceiveChequeInvalidChequebook (0.00s)
--- PASS: TestReceiveChequeInvalidAmount (0.00s)
--- PASS: TestReceiveCheque (0.00s)
--- PASS: TestCashoutStatusReverted (0.00s)
--- PASS: TestChequebookIssueErrorSend (0.00s)
--- PASS: TestCashoutStatusPending (0.00s)
--- PASS: TestChequebookWithdrawInsufficientFunds (0.00s)
--- PASS: TestFactoryDeploy (0.00s)
--- PASS: TestChequebookWithdraw (0.00s)
--- PASS: TestChequebookIssue (0.00s)
--- PASS: TestChequebookAddress (0.00s)
--- PASS: TestChequebookBalance (0.00s)
--- PASS: TestChequebookIssueOutOfFunds (0.00s)
--- PASS: TestCashoutBounced (0.00s)
=== CONT  TestReceiveChequeNotEnoughValueAfterDeduction
--- PASS: TestReceiveChequeNotEnoughValueAfterDeduction (0.00s)
--- PASS: TestChequebookWaitForDeposit (0.00s)
--- PASS: TestReceiveChequeInvalidBeneficiary (0.00s)
--- PASS: TestSignCheque (0.00s)
--- PASS: TestFactoryVerifyChequebook (0.00s)
    --- PASS: TestFactoryVerifyChequebook/valid (0.00s)
    --- PASS: TestFactoryVerifyChequebook/valid_legacy (0.00s)
    --- PASS: TestFactoryVerifyChequebook/invalid (0.00s)
--- PASS: TestFactoryVerifySelf (0.00s)
    --- PASS: TestFactoryVerifySelf/valid (0.00s)
    --- PASS: TestFactoryVerifySelf/invalid_deploy_factory (0.00s)
    --- PASS: TestFactoryVerifySelf/invalid_legacy_factories (0.00s)
--- PASS: TestFactoryERC20Address (0.00s)
--- PASS: TestReceiveChequeNotEnoughValue (0.00s)
--- PASS: TestChequebookWaitForDepositReverted (0.00s)
--- PASS: TestReceiveChequeInsufficientBalance (0.00s)
--- PASS: TestSignChequeIntegration (0.02s)
PASS
ok  	github.com/ethersphere/bee/pkg/settlement/swap/chequebook	(cached)
=== RUN   TestBalanceOf
=== PAUSE TestBalanceOf
=== RUN   TestTransfer
=== PAUSE TestTransfer
=== CONT  TestBalanceOf
--- PASS: TestBalanceOf (0.00s)
=== CONT  TestTransfer
--- PASS: TestTransfer (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/settlement/swap/erc20	(cached)
=== RUN   TestParseSettlementResponseHeaders
=== PAUSE TestParseSettlementResponseHeaders
=== RUN   TestMakeSettlementHeaders
=== PAUSE TestMakeSettlementHeaders
=== RUN   TestParseExchangeHeader
=== PAUSE TestParseExchangeHeader
=== RUN   TestParseDeductionHeader
=== PAUSE TestParseDeductionHeader
=== CONT  TestParseSettlementResponseHeaders
--- PASS: TestParseSettlementResponseHeaders (0.00s)
=== CONT  TestParseDeductionHeader
--- PASS: TestParseDeductionHeader (0.00s)
=== CONT  TestParseExchangeHeader
--- PASS: TestParseExchangeHeader (0.00s)
=== CONT  TestMakeSettlementHeaders
--- PASS: TestMakeSettlementHeaders (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/settlement/swap/headers	(cached)
=== RUN   TestExchangeGetPrice
=== PAUSE TestExchangeGetPrice
=== CONT  TestExchangeGetPrice
--- PASS: TestExchangeGetPrice (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/settlement/swap/priceoracle	(cached)
=== RUN   TestEmitCheques
=== PAUSE TestEmitCheques
=== RUN   TestCantEmitChequeRateMismatch
=== PAUSE TestCantEmitChequeRateMismatch
=== RUN   TestCantEmitChequeDeductionMismatch
=== PAUSE TestCantEmitChequeDeductionMismatch
=== RUN   TestCantEmitChequeIneligibleDeduction
=== PAUSE TestCantEmitChequeIneligibleDeduction
=== CONT  TestEmitCheques
=== CONT  TestCantEmitChequeIneligibleDeduction
--- PASS: TestCantEmitChequeIneligibleDeduction (0.00s)
=== CONT  TestCantEmitChequeDeductionMismatch
--- PASS: TestCantEmitChequeDeductionMismatch (0.00s)
=== CONT  TestCantEmitChequeRateMismatch
--- PASS: TestCantEmitChequeRateMismatch (0.00s)
--- PASS: TestEmitCheques (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/settlement/swap/swapprotocol	(cached)
=== RUN   TestShard
=== PAUSE TestShard
=== RUN   TestMissingShard
=== PAUSE TestMissingShard
=== RUN   TestRecovery
=== RUN   TestRecovery/recover_based_on_preserved_map
=== RUN   TestRecovery/check_integrity_of_recovered_sharky
=== RUN   TestRecovery/check_integrity_of_recovered_sharky/preserved_are_found
=== RUN   TestRecovery/check_integrity_of_recovered_sharky/correct_number_of_free_slots
=== RUN   TestRecovery/check_integrity_of_recovered_sharky/added_locs_are_still_preserved
=== RUN   TestRecovery/check_integrity_of_recovered_sharky/all_other_slots_also_overwritten
--- PASS: TestRecovery (0.04s)
    --- PASS: TestRecovery/recover_based_on_preserved_map (0.02s)
    --- PASS: TestRecovery/check_integrity_of_recovered_sharky (0.01s)
        --- PASS: TestRecovery/check_integrity_of_recovered_sharky/preserved_are_found (0.00s)
        --- PASS: TestRecovery/check_integrity_of_recovered_sharky/correct_number_of_free_slots (0.00s)
        --- PASS: TestRecovery/check_integrity_of_recovered_sharky/added_locs_are_still_preserved (0.00s)
        --- PASS: TestRecovery/check_integrity_of_recovered_sharky/all_other_slots_also_overwritten (0.00s)
=== RUN   TestLocationSerialization
=== PAUSE TestLocationSerialization
=== RUN   TestSingleRetrieval
=== PAUSE TestSingleRetrieval
=== RUN   TestPersistence
=== PAUSE TestPersistence
=== RUN   TestConcurrency
=== PAUSE TestConcurrency
=== CONT  TestShard
=== CONT  TestSingleRetrieval
=== CONT  TestLocationSerialization
=== RUN   TestLocationSerialization/1_100_4096
=== PAUSE TestLocationSerialization/1_100_4096
=== CONT  TestMissingShard
=== RUN   TestLocationSerialization/0_0_0
=== PAUSE TestLocationSerialization/0_0_0
=== RUN   TestLocationSerialization/255_4294967295_65535
=== CONT  TestConcurrency
=== RUN   TestConcurrency/workers:3,shards:2,size:2
=== PAUSE TestConcurrency/workers:3,shards:2,size:2
=== RUN   TestSingleRetrieval/write_and_read
=== RUN   TestConcurrency/workers:2,shards:64,size:2
=== PAUSE TestConcurrency/workers:2,shards:64,size:2
=== RUN   TestConcurrency/workers:32,shards:8,size:32
=== PAUSE TestConcurrency/workers:32,shards:8,size:32
=== PAUSE TestSingleRetrieval/write_and_read
=== CONT  TestSingleRetrieval/write_and_read
=== RUN   TestSingleRetrieval/write_and_read/short_data
=== CONT  TestPersistence
=== PAUSE TestLocationSerialization/255_4294967295_65535
=== CONT  TestLocationSerialization/1_100_4096
=== RUN   TestConcurrency/workers:64,shards:32,size:64
=== PAUSE TestConcurrency/workers:64,shards:32,size:64
=== CONT  TestLocationSerialization/255_4294967295_65535
=== CONT  TestConcurrency/workers:3,shards:2,size:2
--- PASS: TestMissingShard (0.00s)
=== RUN   TestSingleRetrieval/write_and_read/exact_size_data
=== CONT  TestLocationSerialization/0_0_0
=== CONT  TestConcurrency/workers:32,shards:8,size:32
=== CONT  TestConcurrency/workers:2,shards:64,size:2
=== CONT  TestConcurrency/workers:64,shards:32,size:64
--- PASS: TestLocationSerialization (0.00s)
    --- PASS: TestLocationSerialization/1_100_4096 (0.00s)
    --- PASS: TestLocationSerialization/255_4294967295_65535 (0.00s)
    --- PASS: TestLocationSerialization/0_0_0 (0.00s)
=== RUN   TestSingleRetrieval/write_and_read/exact_size_data_2
=== RUN   TestSingleRetrieval/write_and_read/long_data
=== RUN   TestSingleRetrieval/write_and_read/exact_size_data_3
--- PASS: TestShard (0.00s)
--- PASS: TestSingleRetrieval (0.01s)
    --- PASS: TestSingleRetrieval/write_and_read (0.00s)
        --- PASS: TestSingleRetrieval/write_and_read/short_data (0.00s)
        --- PASS: TestSingleRetrieval/write_and_read/exact_size_data (0.00s)
        --- PASS: TestSingleRetrieval/write_and_read/exact_size_data_2 (0.00s)
        --- PASS: TestSingleRetrieval/write_and_read/long_data (0.00s)
        --- PASS: TestSingleRetrieval/write_and_read/exact_size_data_3 (0.00s)
=== NAME  TestPersistence
    sharky_test.go:141: got full in 9 sessions
--- PASS: TestPersistence (0.06s)
--- PASS: TestConcurrency (0.00s)
    --- PASS: TestConcurrency/workers:3,shards:2,size:2 (0.01s)
    --- PASS: TestConcurrency/workers:32,shards:8,size:32 (0.07s)
    --- PASS: TestConcurrency/workers:2,shards:64,size:2 (0.13s)
    --- PASS: TestConcurrency/workers:64,shards:32,size:64 (1.14s)
PASS
ok  	github.com/ethersphere/bee/pkg/sharky	(cached)
=== RUN   TestNewDB
=== PAUSE TestNewDB
=== RUN   TestDB_persistence
=== PAUSE TestDB_persistence
=== RUN   TestStringField
=== PAUSE TestStringField
=== RUN   TestStructField
=== RUN   TestStructField/get_empty
=== RUN   TestStructField/put
=== RUN   TestStructField/put/overwrite
=== RUN   TestStructField/put_in_batch
=== RUN   TestStructField/put_in_batch/overwrite
--- PASS: TestStructField (0.00s)
    --- PASS: TestStructField/get_empty (0.00s)
    --- PASS: TestStructField/put (0.00s)
        --- PASS: TestStructField/put/overwrite (0.00s)
    --- PASS: TestStructField/put_in_batch (0.00s)
        --- PASS: TestStructField/put_in_batch/overwrite (0.00s)
=== RUN   TestUint64Field
=== PAUSE TestUint64Field
=== RUN   TestUint64Field_Inc
=== PAUSE TestUint64Field_Inc
=== RUN   TestUint64Field_IncInBatch
=== PAUSE TestUint64Field_IncInBatch
=== RUN   TestUint64Field_Dec
=== PAUSE TestUint64Field_Dec
=== RUN   TestUint64Field_DecInBatch
=== PAUSE TestUint64Field_DecInBatch
=== RUN   TestIndex
=== RUN   TestIndex/put
=== RUN   TestIndex/put/overwrite
=== RUN   TestIndex/put_in_batch
=== RUN   TestIndex/put_in_batch/overwrite
=== RUN   TestIndex/put_in_batch_twice
=== RUN   TestIndex/has
=== RUN   TestIndex/delete
=== RUN   TestIndex/delete_in_batch
=== RUN   TestIndex/fill
=== RUN   TestIndex/fill/not_found
--- PASS: TestIndex (0.00s)
    --- PASS: TestIndex/put (0.00s)
        --- PASS: TestIndex/put/overwrite (0.00s)
    --- PASS: TestIndex/put_in_batch (0.00s)
        --- PASS: TestIndex/put_in_batch/overwrite (0.00s)
    --- PASS: TestIndex/put_in_batch_twice (0.00s)
    --- PASS: TestIndex/has (0.00s)
    --- PASS: TestIndex/delete (0.00s)
    --- PASS: TestIndex/delete_in_batch (0.00s)
    --- PASS: TestIndex/fill (0.00s)
        --- PASS: TestIndex/fill/not_found (0.00s)
=== RUN   TestIndex_Iterate
=== RUN   TestIndex_Iterate/all
=== RUN   TestIndex_Iterate/start_from
=== RUN   TestIndex_Iterate/skip_start_from
=== RUN   TestIndex_Iterate/stop
=== RUN   TestIndex_Iterate/no_overflow
--- PASS: TestIndex_Iterate (0.00s)
    --- PASS: TestIndex_Iterate/all (0.00s)
    --- PASS: TestIndex_Iterate/start_from (0.00s)
    --- PASS: TestIndex_Iterate/skip_start_from (0.00s)
    --- PASS: TestIndex_Iterate/stop (0.00s)
    --- PASS: TestIndex_Iterate/no_overflow (0.00s)
=== RUN   TestIndex_IterateReverse
=== RUN   TestIndex_IterateReverse/all
=== RUN   TestIndex_IterateReverse/start_from
=== RUN   TestIndex_IterateReverse/skip_start_from
=== RUN   TestIndex_IterateReverse/stop
=== RUN   TestIndex_IterateReverse/no_overflow
--- PASS: TestIndex_IterateReverse (0.00s)
    --- PASS: TestIndex_IterateReverse/all (0.00s)
    --- PASS: TestIndex_IterateReverse/start_from (0.00s)
    --- PASS: TestIndex_IterateReverse/skip_start_from (0.00s)
    --- PASS: TestIndex_IterateReverse/stop (0.00s)
    --- PASS: TestIndex_IterateReverse/no_overflow (0.00s)
=== RUN   TestIndex_Iterate_withPrefix
=== RUN   TestIndex_Iterate_withPrefix/with_prefix
=== RUN   TestIndex_Iterate_withPrefix/with_prefix_and_start_from
=== RUN   TestIndex_Iterate_withPrefix/with_prefix_and_skip_start_from
=== RUN   TestIndex_Iterate_withPrefix/stop
=== RUN   TestIndex_Iterate_withPrefix/no_overflow
--- PASS: TestIndex_Iterate_withPrefix (0.00s)
    --- PASS: TestIndex_Iterate_withPrefix/with_prefix (0.00s)
    --- PASS: TestIndex_Iterate_withPrefix/with_prefix_and_start_from (0.00s)
    --- PASS: TestIndex_Iterate_withPrefix/with_prefix_and_skip_start_from (0.00s)
    --- PASS: TestIndex_Iterate_withPrefix/stop (0.00s)
    --- PASS: TestIndex_Iterate_withPrefix/no_overflow (0.00s)
=== RUN   TestIndex_IterateReverse_withPrefix
=== RUN   TestIndex_IterateReverse_withPrefix/with_prefix
=== RUN   TestIndex_IterateReverse_withPrefix/with_prefix_and_start_from
=== RUN   TestIndex_IterateReverse_withPrefix/with_prefix_and_skip_start_from
=== RUN   TestIndex_IterateReverse_withPrefix/stop
=== RUN   TestIndex_IterateReverse_withPrefix/no_overflow
--- PASS: TestIndex_IterateReverse_withPrefix (0.00s)
    --- PASS: TestIndex_IterateReverse_withPrefix/with_prefix (0.00s)
    --- PASS: TestIndex_IterateReverse_withPrefix/with_prefix_and_start_from (0.00s)
    --- PASS: TestIndex_IterateReverse_withPrefix/with_prefix_and_skip_start_from (0.00s)
    --- PASS: TestIndex_IterateReverse_withPrefix/stop (0.00s)
    --- PASS: TestIndex_IterateReverse_withPrefix/no_overflow (0.00s)
=== RUN   TestIndex_count
=== RUN   TestIndex_count/Count
=== RUN   TestIndex_count/CountFrom
=== RUN   TestIndex_count/add_item
=== RUN   TestIndex_count/add_item/Count
=== RUN   TestIndex_count/add_item/CountFrom
=== RUN   TestIndex_count/delete_items
=== RUN   TestIndex_count/delete_items/Count
=== RUN   TestIndex_count/delete_items/CountFrom
--- PASS: TestIndex_count (0.00s)
    --- PASS: TestIndex_count/Count (0.00s)
    --- PASS: TestIndex_count/CountFrom (0.00s)
    --- PASS: TestIndex_count/add_item (0.00s)
        --- PASS: TestIndex_count/add_item/Count (0.00s)
        --- PASS: TestIndex_count/add_item/CountFrom (0.00s)
    --- PASS: TestIndex_count/delete_items (0.00s)
        --- PASS: TestIndex_count/delete_items/Count (0.00s)
        --- PASS: TestIndex_count/delete_items/CountFrom (0.00s)
=== RUN   TestIndex_firstAndLast
=== PAUSE TestIndex_firstAndLast
=== RUN   TestIncByteSlice
=== PAUSE TestIncByteSlice
=== RUN   TestIndex_HasMulti
=== PAUSE TestIndex_HasMulti
=== RUN   TestDB_schemaFieldKey
=== PAUSE TestDB_schemaFieldKey
=== RUN   TestDB_schemaIndexPrefix
=== PAUSE TestDB_schemaIndexPrefix
=== RUN   TestDB_RenameIndex
=== PAUSE TestDB_RenameIndex
=== RUN   TestUint64Vector
=== PAUSE TestUint64Vector
=== RUN   TestUint64Vector_Inc
=== PAUSE TestUint64Vector_Inc
=== RUN   TestUint64Vector_IncInBatch
=== PAUSE TestUint64Vector_IncInBatch
=== RUN   TestUint64Vector_Dec
=== PAUSE TestUint64Vector_Dec
=== RUN   TestUint64Vector_DecInBatch
=== PAUSE TestUint64Vector_DecInBatch
=== CONT  TestNewDB
=== CONT  TestIndex_HasMulti
=== CONT  TestIncByteSlice
--- PASS: TestIncByteSlice (0.00s)
=== CONT  TestUint64Field
=== CONT  TestIndex_firstAndLast
=== CONT  TestUint64Field_DecInBatch
=== CONT  TestUint64Field_Dec
=== CONT  TestUint64Field_IncInBatch
=== CONT  TestUint64Field_Inc
=== RUN   TestUint64Field/get_empty
=== PAUSE TestUint64Field/get_empty
=== RUN   TestUint64Field/put
=== PAUSE TestUint64Field/put
=== RUN   TestUint64Field/put_in_batch
=== PAUSE TestUint64Field/put_in_batch
=== CONT  TestStringField
=== RUN   TestStringField/get_empty
=== PAUSE TestStringField/get_empty
=== RUN   TestStringField/put
=== PAUSE TestStringField/put
--- PASS: TestNewDB (0.00s)
=== CONT  TestDB_persistence
=== RUN   TestStringField/put_in_batch
=== PAUSE TestStringField/put_in_batch
=== CONT  TestUint64Vector_DecInBatch
--- PASS: TestIndex_HasMulti (0.00s)
=== CONT  TestUint64Vector_Dec
--- PASS: TestUint64Field_Inc (0.01s)
=== CONT  TestUint64Vector_IncInBatch
--- PASS: TestUint64Field_IncInBatch (0.01s)
=== CONT  TestUint64Vector_Inc
--- PASS: TestIndex_firstAndLast (0.01s)
=== CONT  TestUint64Vector
=== RUN   TestUint64Vector/get_empty
=== PAUSE TestUint64Vector/get_empty
=== RUN   TestUint64Vector/put
=== PAUSE TestUint64Vector/put
=== RUN   TestUint64Vector/put_in_batch
=== PAUSE TestUint64Vector/put_in_batch
=== CONT  TestDB_RenameIndex
=== RUN   TestDB_RenameIndex/empty_names
=== PAUSE TestDB_RenameIndex/empty_names
=== RUN   TestDB_RenameIndex/same_names
=== PAUSE TestDB_RenameIndex/same_names
=== RUN   TestDB_RenameIndex/unknown_name
=== PAUSE TestDB_RenameIndex/unknown_name
=== RUN   TestDB_RenameIndex/valid_names
=== PAUSE TestDB_RenameIndex/valid_names
=== CONT  TestDB_schemaIndexPrefix
=== RUN   TestDB_schemaIndexPrefix/same_name
=== PAUSE TestDB_schemaIndexPrefix/same_name
=== RUN   TestDB_schemaIndexPrefix/different_names
=== PAUSE TestDB_schemaIndexPrefix/different_names
=== CONT  TestDB_schemaFieldKey
=== RUN   TestDB_schemaFieldKey/empty_name_or_type
=== PAUSE TestDB_schemaFieldKey/empty_name_or_type
=== RUN   TestDB_schemaFieldKey/same_field
=== PAUSE TestDB_schemaFieldKey/same_field
=== RUN   TestDB_schemaFieldKey/different_fields
=== PAUSE TestDB_schemaFieldKey/different_fields
=== RUN   TestDB_schemaFieldKey/same_field_name_different_types
=== PAUSE TestDB_schemaFieldKey/same_field_name_different_types
=== CONT  TestUint64Field/get_empty
--- PASS: TestUint64Vector_IncInBatch (0.00s)
=== CONT  TestUint64Field/put_in_batch
--- PASS: TestUint64Vector_DecInBatch (0.01s)
=== CONT  TestUint64Field/put
=== CONT  TestStringField/put_in_batch
=== CONT  TestStringField/put
=== CONT  TestUint64Vector/get_empty
--- PASS: TestUint64Field_DecInBatch (0.01s)
--- PASS: TestUint64Vector_Dec (0.01s)
--- PASS: TestUint64Field_Dec (0.01s)
--- PASS: TestUint64Vector_Inc (0.00s)
=== CONT  TestStringField/get_empty
=== CONT  TestDB_RenameIndex/empty_names
=== RUN   TestUint64Field/put_in_batch/overwrite
=== CONT  TestUint64Vector/put_in_batch
=== RUN   TestStringField/put_in_batch/overwrite
=== PAUSE TestStringField/put_in_batch/overwrite
=== CONT  TestUint64Vector/put
=== RUN   TestStringField/put/overwrite
=== PAUSE TestStringField/put/overwrite
=== CONT  TestDB_schemaIndexPrefix/same_name
=== CONT  TestDB_RenameIndex/valid_names
=== CONT  TestDB_RenameIndex/unknown_name
=== CONT  TestDB_RenameIndex/same_names
=== RUN   TestUint64Vector/put/overwrite
=== CONT  TestDB_schemaIndexPrefix/different_names
=== CONT  TestDB_schemaFieldKey/empty_name_or_type
=== CONT  TestDB_schemaFieldKey/different_fields
=== RUN   TestUint64Vector/put/overwrite#01
=== RUN   TestUint64Vector/put/overwrite#02
=== RUN   TestUint64Vector/put/overwrite#03
=== RUN   TestUint64Vector/put/overwrite#04
=== CONT  TestDB_schemaFieldKey/same_field_name_different_types
--- PASS: TestDB_schemaIndexPrefix (0.00s)
    --- PASS: TestDB_schemaIndexPrefix/same_name (0.00s)
    --- PASS: TestDB_schemaIndexPrefix/different_names (0.00s)
=== CONT  TestDB_schemaFieldKey/same_field
=== CONT  TestStringField/put_in_batch/overwrite
=== CONT  TestStringField/put/overwrite
--- PASS: TestStringField (0.00s)
    --- PASS: TestStringField/get_empty (0.00s)
    --- PASS: TestStringField/put_in_batch (0.00s)
        --- PASS: TestStringField/put_in_batch/overwrite (0.00s)
    --- PASS: TestStringField/put (0.00s)
        --- PASS: TestStringField/put/overwrite (0.00s)
=== RUN   TestUint64Vector/put_in_batch/overwrite
=== RUN   TestUint64Field/put/overwrite
=== RUN   TestUint64Vector/put_in_batch/overwrite#01
=== RUN   TestUint64Vector/put_in_batch/overwrite#02
=== RUN   TestUint64Vector/put_in_batch/overwrite#03
=== RUN   TestUint64Vector/put_in_batch/overwrite#04
--- PASS: TestDB_RenameIndex (0.00s)
    --- PASS: TestDB_RenameIndex/empty_names (0.00s)
    --- PASS: TestDB_RenameIndex/valid_names (0.00s)
    --- PASS: TestDB_RenameIndex/unknown_name (0.00s)
    --- PASS: TestDB_RenameIndex/same_names (0.01s)
=== RUN   TestUint64Vector/put_in_batch/overwrite#05
--- PASS: TestUint64Field (0.00s)
    --- PASS: TestUint64Field/get_empty (0.00s)
    --- PASS: TestUint64Field/put_in_batch (0.00s)
        --- PASS: TestUint64Field/put_in_batch/overwrite (0.00s)
    --- PASS: TestUint64Field/put (0.01s)
        --- PASS: TestUint64Field/put/overwrite (0.00s)
--- PASS: TestUint64Vector (0.00s)
    --- PASS: TestUint64Vector/get_empty (0.00s)
    --- PASS: TestUint64Vector/put (0.00s)
        --- PASS: TestUint64Vector/put/overwrite (0.00s)
        --- PASS: TestUint64Vector/put/overwrite#01 (0.00s)
        --- PASS: TestUint64Vector/put/overwrite#02 (0.00s)
        --- PASS: TestUint64Vector/put/overwrite#03 (0.00s)
        --- PASS: TestUint64Vector/put/overwrite#04 (0.00s)
    --- PASS: TestUint64Vector/put_in_batch (0.01s)
        --- PASS: TestUint64Vector/put_in_batch/overwrite (0.00s)
        --- PASS: TestUint64Vector/put_in_batch/overwrite#01 (0.00s)
        --- PASS: TestUint64Vector/put_in_batch/overwrite#02 (0.00s)
        --- PASS: TestUint64Vector/put_in_batch/overwrite#03 (0.00s)
        --- PASS: TestUint64Vector/put_in_batch/overwrite#04 (0.00s)
        --- PASS: TestUint64Vector/put_in_batch/overwrite#05 (0.00s)
--- PASS: TestDB_schemaFieldKey (0.00s)
    --- PASS: TestDB_schemaFieldKey/different_fields (0.00s)
    --- PASS: TestDB_schemaFieldKey/same_field_name_different_types (0.00s)
    --- PASS: TestDB_schemaFieldKey/empty_name_or_type (0.00s)
    --- PASS: TestDB_schemaFieldKey/same_field (0.00s)
--- PASS: TestDB_persistence (0.03s)
=== RUN   Example_store
--- PASS: Example_store (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/shed	(cached)
=== RUN   TestAddOverdraft
=== PAUSE TestAddOverdraft
=== CONT  TestAddOverdraft
--- PASS: TestAddOverdraft (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/skippeers	(cached)
=== RUN   TestNew
=== PAUSE TestNew
=== RUN   TestNewSigned
=== PAUSE TestNewSigned
=== RUN   TestChunk
=== PAUSE TestChunk
=== RUN   TestChunkErrorWithoutOwner
=== PAUSE TestChunkErrorWithoutOwner
=== RUN   TestSign
=== PAUSE TestSign
=== RUN   TestFromChunk
=== PAUSE TestFromChunk
=== RUN   TestCreateAddress
=== PAUSE TestCreateAddress
=== RUN   TestRecoverAddress
=== PAUSE TestRecoverAddress
=== RUN   TestValid
=== PAUSE TestValid
=== RUN   TestInvalid
=== PAUSE TestInvalid
=== CONT  TestNew
=== CONT  TestFromChunk
--- PASS: TestNew (0.00s)
=== CONT  TestChunk
--- PASS: TestChunk (0.00s)
=== CONT  TestNewSigned
--- PASS: TestNewSigned (0.00s)
=== CONT  TestSign
=== CONT  TestValid
=== CONT  TestInvalid
=== RUN   TestInvalid/wrong_soc_address
=== PAUSE TestInvalid/wrong_soc_address
=== RUN   TestInvalid/invalid_data
=== PAUSE TestInvalid/invalid_data
=== RUN   TestInvalid/invalid_id
=== PAUSE TestInvalid/invalid_id
=== RUN   TestInvalid/invalid_signature
=== PAUSE TestInvalid/invalid_signature
=== RUN   TestInvalid/nil_data
=== PAUSE TestInvalid/nil_data
=== RUN   TestInvalid/small_data
=== PAUSE TestInvalid/small_data
=== RUN   TestInvalid/large_data
=== PAUSE TestInvalid/large_data
=== CONT  TestInvalid/wrong_soc_address
=== CONT  TestChunkErrorWithoutOwner
--- PASS: TestChunkErrorWithoutOwner (0.00s)
=== CONT  TestInvalid/large_data
=== CONT  TestInvalid/small_data
=== CONT  TestInvalid/nil_data
=== CONT  TestInvalid/invalid_signature
=== CONT  TestRecoverAddress
=== CONT  TestCreateAddress
--- PASS: TestCreateAddress (0.00s)
=== CONT  TestInvalid/invalid_id
--- PASS: TestValid (0.02s)
--- PASS: TestFromChunk (0.02s)
=== CONT  TestInvalid/invalid_data
--- PASS: TestSign (0.02s)
--- PASS: TestRecoverAddress (0.02s)
--- PASS: TestInvalid (0.00s)
    --- PASS: TestInvalid/small_data (0.00s)
    --- PASS: TestInvalid/nil_data (0.00s)
    --- PASS: TestInvalid/wrong_soc_address (0.02s)
    --- PASS: TestInvalid/invalid_signature (0.02s)
    --- PASS: TestInvalid/invalid_data (0.00s)
    --- PASS: TestInvalid/large_data (0.02s)
    --- PASS: TestInvalid/invalid_id (0.02s)
PASS
ok  	github.com/ethersphere/bee/pkg/soc	(cached)
=== RUN   TestWait
=== PAUSE TestWait
=== CONT  TestWait
=== RUN   TestWait/timed_out
=== PAUSE TestWait/timed_out
=== RUN   TestWait/condition_satisfied
=== PAUSE TestWait/condition_satisfied
=== CONT  TestWait/timed_out
=== CONT  TestWait/condition_satisfied
--- PASS: TestWait (0.00s)
    --- PASS: TestWait/timed_out (0.02s)
    --- PASS: TestWait/condition_satisfied (0.10s)
PASS
ok  	github.com/ethersphere/bee/pkg/spinlock	(cached)
=== RUN   TestOneMigration
--- PASS: TestOneMigration (0.03s)
=== RUN   TestManyMigrations
--- PASS: TestManyMigrations (0.02s)
=== RUN   TestMigrationErrorFrom
--- PASS: TestMigrationErrorFrom (0.02s)
=== RUN   TestMigrationErrorTo
--- PASS: TestMigrationErrorTo (0.02s)
=== RUN   TestMigrationSwap
--- PASS: TestMigrationSwap (0.01s)
=== RUN   TestPersistentStateStore
=== RUN   TestPersistentStateStore/test_put_get
=== RUN   TestPersistentStateStore/test_delete
=== RUN   TestPersistentStateStore/test_iterator
--- PASS: TestPersistentStateStore (0.05s)
    --- PASS: TestPersistentStateStore/test_put_get (0.01s)
    --- PASS: TestPersistentStateStore/test_delete (0.01s)
    --- PASS: TestPersistentStateStore/test_iterator (0.01s)
=== RUN   TestGetSchemaName
--- PASS: TestGetSchemaName (0.01s)
PASS
ok  	github.com/ethersphere/bee/pkg/statestore/leveldb	(cached)
=== RUN   TestMockStateStore
=== RUN   TestMockStateStore/test_put_get
=== RUN   TestMockStateStore/test_delete
=== RUN   TestMockStateStore/test_iterator
--- PASS: TestMockStateStore (0.00s)
    --- PASS: TestMockStateStore/test_put_get (0.00s)
    --- PASS: TestMockStateStore/test_delete (0.00s)
    --- PASS: TestMockStateStore/test_iterator (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/statestore/mock	(cached)
=== RUN   TestSteward
=== PAUSE TestSteward
=== RUN   TestSteward_ErrWantSelf
=== PAUSE TestSteward_ErrWantSelf
=== CONT  TestSteward
=== CONT  TestSteward_ErrWantSelf
--- PASS: TestSteward_ErrWantSelf (0.02s)
--- PASS: TestSteward (0.27s)
PASS
ok  	github.com/ethersphere/bee/pkg/steward	(cached)
=== RUN   TestStorePutGet
=== PAUSE TestStorePutGet
=== CONT  TestStorePutGet
--- PASS: TestStorePutGet (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/storage/inmemstore	(cached)
=== RUN   TestMockStorer
=== PAUSE TestMockStorer
=== CONT  TestMockStorer
--- PASS: TestMockStorer (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/storage/mock	(cached)
=== RUN   TestState
=== PAUSE TestState
=== RUN   TestReward
=== PAUSE TestReward
=== RUN   TestFee
=== PAUSE TestFee
=== RUN   TestAgent
=== PAUSE TestAgent
=== RUN   TestClose
=== PAUSE TestClose
=== RUN   TestPhaseCancel
=== PAUSE TestPhaseCancel
=== CONT  TestState
=== CONT  TestAgent
=== RUN   TestAgent/3_blocks_per_phase,_same_block_number_returns_twice
=== PAUSE TestAgent/3_blocks_per_phase,_same_block_number_returns_twice
=== CONT  TestFee
=== RUN   TestAgent/3_blocks_per_phase,_block_number_returns_every_block
=== PAUSE TestAgent/3_blocks_per_phase,_block_number_returns_every_block
=== RUN   TestAgent/no_expected_calls_-_block_number_returns_late_after_each_phase
=== PAUSE TestAgent/no_expected_calls_-_block_number_returns_late_after_each_phase
=== RUN   TestAgent/4_blocks_per_phase,_block_number_returns_every_other_block
=== PAUSE TestAgent/4_blocks_per_phase,_block_number_returns_every_other_block
--- PASS: TestState (0.00s)
=== CONT  TestAgent/3_blocks_per_phase,_same_block_number_returns_twice
=== CONT  TestReward
--- PASS: TestFee (0.00s)
=== CONT  TestAgent/4_blocks_per_phase,_block_number_returns_every_other_block
--- PASS: TestReward (0.00s)
=== CONT  TestPhaseCancel
=== CONT  TestAgent/no_expected_calls_-_block_number_returns_late_after_each_phase
=== CONT  TestAgent/3_blocks_per_phase,_block_number_returns_every_block
--- PASS: TestPhaseCancel (0.00s)
=== CONT  TestClose
--- PASS: TestClose (0.00s)
--- PASS: TestAgent (0.00s)
    --- PASS: TestAgent/no_expected_calls_-_block_number_returns_late_after_each_phase (0.13s)
    --- PASS: TestAgent/4_blocks_per_phase,_block_number_returns_every_other_block (0.59s)
    --- PASS: TestAgent/3_blocks_per_phase,_same_block_number_returns_twice (0.96s)
    --- PASS: TestAgent/3_blocks_per_phase,_block_number_returns_every_block (0.96s)
PASS
ok  	github.com/ethersphere/bee/pkg/storageincentives	(cached)
=== RUN   TestRedistribution
=== PAUSE TestRedistribution
=== CONT  TestRedistribution
=== RUN   TestRedistribution/IsPlaying_-_true
=== PAUSE TestRedistribution/IsPlaying_-_true
=== RUN   TestRedistribution/IsPlaying_-_false
=== PAUSE TestRedistribution/IsPlaying_-_false
=== RUN   TestRedistribution/IsWinner_-_false
=== PAUSE TestRedistribution/IsWinner_-_false
=== RUN   TestRedistribution/IsWinner_-_true
=== PAUSE TestRedistribution/IsWinner_-_true
=== RUN   TestRedistribution/Claim
=== PAUSE TestRedistribution/Claim
=== RUN   TestRedistribution/Claim_with_tx_reverted
=== PAUSE TestRedistribution/Claim_with_tx_reverted
=== RUN   TestRedistribution/Commit
=== PAUSE TestRedistribution/Commit
=== RUN   TestRedistribution/Reveal
=== PAUSE TestRedistribution/Reveal
=== RUN   TestRedistribution/Reserve_Salt
=== PAUSE TestRedistribution/Reserve_Salt
=== RUN   TestRedistribution/send_tx_failed
=== PAUSE TestRedistribution/send_tx_failed
=== RUN   TestRedistribution/invalid_call_data
=== PAUSE TestRedistribution/invalid_call_data
=== CONT  TestRedistribution/IsPlaying_-_true
=== CONT  TestRedistribution/invalid_call_data
=== CONT  TestRedistribution/send_tx_failed
=== CONT  TestRedistribution/Reserve_Salt
=== CONT  TestRedistribution/Reveal
=== CONT  TestRedistribution/Commit
=== CONT  TestRedistribution/Claim_with_tx_reverted
=== CONT  TestRedistribution/Claim
=== CONT  TestRedistribution/IsWinner_-_true
=== CONT  TestRedistribution/IsWinner_-_false
=== CONT  TestRedistribution/IsPlaying_-_false
--- PASS: TestRedistribution (0.00s)
    --- PASS: TestRedistribution/IsPlaying_-_true (0.00s)
    --- PASS: TestRedistribution/invalid_call_data (0.00s)
    --- PASS: TestRedistribution/send_tx_failed (0.00s)
    --- PASS: TestRedistribution/Reserve_Salt (0.00s)
    --- PASS: TestRedistribution/Reveal (0.00s)
    --- PASS: TestRedistribution/Commit (0.00s)
    --- PASS: TestRedistribution/Claim_with_tx_reverted (0.00s)
    --- PASS: TestRedistribution/Claim (0.00s)
    --- PASS: TestRedistribution/IsWinner_-_true (0.00s)
    --- PASS: TestRedistribution/IsWinner_-_false (0.00s)
    --- PASS: TestRedistribution/IsPlaying_-_false (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/storageincentives/redistribution	(cached)
=== RUN   TestDepositStake
=== PAUSE TestDepositStake
=== RUN   TestGetStake
=== PAUSE TestGetStake
=== RUN   TestWithdrawStake
=== PAUSE TestWithdrawStake
=== CONT  TestDepositStake
=== RUN   TestDepositStake/ok
=== CONT  TestWithdrawStake
=== RUN   TestWithdrawStake/ok
=== CONT  TestGetStake
=== PAUSE TestWithdrawStake/ok
=== RUN   TestWithdrawStake/is_paused
=== PAUSE TestWithdrawStake/is_paused
=== RUN   TestWithdrawStake/has_no_stake
=== PAUSE TestWithdrawStake/has_no_stake
=== RUN   TestWithdrawStake/invalid_call_data
=== PAUSE TestWithdrawStake/invalid_call_data
=== RUN   TestWithdrawStake/send_tx_failed
=== PAUSE TestWithdrawStake/send_tx_failed
=== RUN   TestWithdrawStake/tx_reverted
=== PAUSE TestWithdrawStake/tx_reverted
=== RUN   TestWithdrawStake/is_paused_with_err
=== PAUSE TestWithdrawStake/is_paused_with_err
=== RUN   TestWithdrawStake/get_stake_with_err
=== PAUSE TestWithdrawStake/get_stake_with_err
=== CONT  TestWithdrawStake/ok
=== RUN   TestGetStake/ok
=== PAUSE TestGetStake/ok
=== RUN   TestGetStake/error_with_unpacking
=== PAUSE TestGetStake/error_with_unpacking
=== RUN   TestGetStake/with_invalid_call_data
=== PAUSE TestGetStake/with_invalid_call_data
=== RUN   TestGetStake/transaction_error
=== PAUSE TestGetStake/transaction_error
=== CONT  TestGetStake/ok
=== CONT  TestWithdrawStake/get_stake_with_err
=== CONT  TestWithdrawStake/is_paused
=== CONT  TestWithdrawStake/has_no_stake
=== CONT  TestGetStake/with_invalid_call_data
=== CONT  TestGetStake/transaction_error
=== CONT  TestGetStake/error_with_unpacking
=== CONT  TestWithdrawStake/is_paused_with_err
=== CONT  TestWithdrawStake/send_tx_failed
=== CONT  TestWithdrawStake/invalid_call_data
=== CONT  TestWithdrawStake/tx_reverted
=== PAUSE TestDepositStake/ok
=== RUN   TestDepositStake/ok_with_addon_stake
=== PAUSE TestDepositStake/ok_with_addon_stake
=== RUN   TestDepositStake/insufficient_stake_amount
--- PASS: TestGetStake (0.00s)
    --- PASS: TestGetStake/ok (0.00s)
    --- PASS: TestGetStake/with_invalid_call_data (0.00s)
    --- PASS: TestGetStake/transaction_error (0.00s)
    --- PASS: TestGetStake/error_with_unpacking (0.00s)
=== PAUSE TestDepositStake/insufficient_stake_amount
=== RUN   TestDepositStake/insufficient_funds
=== PAUSE TestDepositStake/insufficient_funds
=== RUN   TestDepositStake/insufficient_stake_amount#01
=== PAUSE TestDepositStake/insufficient_stake_amount#01
=== RUN   TestDepositStake/send_tx_failed
=== PAUSE TestDepositStake/send_tx_failed
=== RUN   TestDepositStake/invalid_call_data
=== PAUSE TestDepositStake/invalid_call_data
--- PASS: TestWithdrawStake (0.00s)
    --- PASS: TestWithdrawStake/ok (0.00s)
    --- PASS: TestWithdrawStake/get_stake_with_err (0.00s)
    --- PASS: TestWithdrawStake/is_paused (0.00s)
    --- PASS: TestWithdrawStake/has_no_stake (0.00s)
    --- PASS: TestWithdrawStake/is_paused_with_err (0.00s)
    --- PASS: TestWithdrawStake/send_tx_failed (0.00s)
    --- PASS: TestWithdrawStake/invalid_call_data (0.00s)
    --- PASS: TestWithdrawStake/tx_reverted (0.00s)
=== RUN   TestDepositStake/transaction_reverted
=== PAUSE TestDepositStake/transaction_reverted
=== RUN   TestDepositStake/transaction_error
=== PAUSE TestDepositStake/transaction_error
=== RUN   TestDepositStake/transaction_error_in_call
=== PAUSE TestDepositStake/transaction_error_in_call
=== CONT  TestDepositStake/ok
=== CONT  TestDepositStake/transaction_error_in_call
=== CONT  TestDepositStake/transaction_error
=== CONT  TestDepositStake/transaction_reverted
=== CONT  TestDepositStake/invalid_call_data
=== CONT  TestDepositStake/send_tx_failed
=== CONT  TestDepositStake/insufficient_stake_amount#01
=== CONT  TestDepositStake/insufficient_funds
=== CONT  TestDepositStake/insufficient_stake_amount
=== CONT  TestDepositStake/ok_with_addon_stake
--- PASS: TestDepositStake (0.00s)
    --- PASS: TestDepositStake/ok (0.00s)
    --- PASS: TestDepositStake/transaction_error_in_call (0.00s)
    --- PASS: TestDepositStake/transaction_error (0.00s)
    --- PASS: TestDepositStake/transaction_reverted (0.00s)
    --- PASS: TestDepositStake/invalid_call_data (0.00s)
    --- PASS: TestDepositStake/send_tx_failed (0.00s)
    --- PASS: TestDepositStake/insufficient_stake_amount#01 (0.00s)
    --- PASS: TestDepositStake/insufficient_funds (0.00s)
    --- PASS: TestDepositStake/insufficient_stake_amount (0.00s)
    --- PASS: TestDepositStake/ok_with_addon_stake (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/storageincentives/staking	(cached)
=== RUN   TestTxState
=== PAUSE TestTxState
=== CONT  TestTxState
=== RUN   TestTxState/lifecycle-normal
=== PAUSE TestTxState/lifecycle-normal
=== RUN   TestTxState/lifecycle-done-by-parent-ctx
=== PAUSE TestTxState/lifecycle-done-by-parent-ctx
=== CONT  TestTxState/lifecycle-normal
=== CONT  TestTxState/lifecycle-done-by-parent-ctx
--- PASS: TestTxState (0.00s)
    --- PASS: TestTxState/lifecycle-done-by-parent-ctx (0.10s)
    --- PASS: TestTxState/lifecycle-normal (0.10s)
PASS
ok  	github.com/ethersphere/bee/pkg/storagev2	(cached)
=== RUN   TestChunkStore
=== PAUSE TestChunkStore
=== RUN   TestTxChunkStore
=== PAUSE TestTxChunkStore
=== CONT  TestChunkStore
=== CONT  TestTxChunkStore
=== RUN   TestTxChunkStore/commit_empty
=== RUN   TestTxChunkStore/commit
=== RUN   TestTxChunkStore/commit/add_new_chunks
=== RUN   TestTxChunkStore/commit/delete_existing_chunks
=== RUN   TestChunkStore/put_chunks
=== RUN   TestChunkStore/put_existing_chunks
=== RUN   TestChunkStore/get_chunks
=== RUN   TestChunkStore/get_non-existing_chunk
=== RUN   TestTxChunkStore/rollback_empty
=== RUN   TestChunkStore/has_chunks
=== RUN   TestTxChunkStore/rollback_added_chunks
=== RUN   TestChunkStore/iterate_chunks
=== RUN   TestChunkStore/delete_chunks
=== RUN   TestChunkStore/check_deleted_chunks
=== RUN   TestChunkStore/iterate_chunks_after_delete
=== RUN   TestChunkStore/close_store
--- PASS: TestChunkStore (0.00s)
    --- PASS: TestChunkStore/put_chunks (0.00s)
    --- PASS: TestChunkStore/put_existing_chunks (0.00s)
    --- PASS: TestChunkStore/get_chunks (0.00s)
    --- PASS: TestChunkStore/get_non-existing_chunk (0.00s)
    --- PASS: TestChunkStore/has_chunks (0.00s)
    --- PASS: TestChunkStore/iterate_chunks (0.00s)
    --- PASS: TestChunkStore/delete_chunks (0.00s)
    --- PASS: TestChunkStore/check_deleted_chunks (0.00s)
    --- PASS: TestChunkStore/iterate_chunks_after_delete (0.00s)
    --- PASS: TestChunkStore/close_store (0.00s)
=== RUN   TestTxChunkStore/rollback_removed_chunks
--- PASS: TestTxChunkStore (0.01s)
    --- PASS: TestTxChunkStore/commit_empty (0.00s)
    --- PASS: TestTxChunkStore/commit (0.00s)
        --- PASS: TestTxChunkStore/commit/add_new_chunks (0.00s)
        --- PASS: TestTxChunkStore/commit/delete_existing_chunks (0.00s)
    --- PASS: TestTxChunkStore/rollback_empty (0.00s)
    --- PASS: TestTxChunkStore/rollback_added_chunks (0.00s)
    --- PASS: TestTxChunkStore/rollback_removed_chunks (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore	(cached)
=== RUN   TestStore
=== PAUSE TestStore
=== RUN   TestBatchedStore
=== PAUSE TestBatchedStore
=== RUN   TestTxStore
=== PAUSE TestTxStore
=== CONT  TestStore
=== CONT  TestTxStore
=== RUN   TestStore/create_new_entries
=== RUN   TestTxStore/commit_empty
=== RUN   TestTxStore/commit
=== RUN   TestTxStore/commit/add_new_objects
=== RUN   TestStore/has_entries
=== RUN   TestStore/get_entries
=== RUN   TestTxStore/commit/delete_existing_objects
=== RUN   TestTxStore/rollback_empty
=== RUN   TestStore/get_size
=== RUN   TestTxStore/rollback_added_objects
=== CONT  TestBatchedStore
=== RUN   TestBatchedStore/duplicates_are_rejected
=== RUN   TestTxStore/rollback_updated_objects
=== RUN   TestBatchedStore/only_last_ops_are_of_interest
=== RUN   TestBatchedStore/batch_not_reusable_after_commit
=== RUN   TestBatchedStore/batch_not_usable_with_expired_context
--- PASS: TestBatchedStore (0.00s)
    --- PASS: TestBatchedStore/duplicates_are_rejected (0.00s)
    --- PASS: TestBatchedStore/only_last_ops_are_of_interest (0.00s)
    --- PASS: TestBatchedStore/batch_not_reusable_after_commit (0.00s)
    --- PASS: TestBatchedStore/batch_not_usable_with_expired_context (0.00s)
=== RUN   TestTxStore/rollback_removed_objects
=== RUN   TestStore/count
--- PASS: TestTxStore (0.00s)
    --- PASS: TestTxStore/commit_empty (0.00s)
    --- PASS: TestTxStore/commit (0.00s)
        --- PASS: TestTxStore/commit/add_new_objects (0.00s)
        --- PASS: TestTxStore/commit/delete_existing_objects (0.00s)
    --- PASS: TestTxStore/rollback_empty (0.00s)
    --- PASS: TestTxStore/rollback_added_objects (0.00s)
    --- PASS: TestTxStore/rollback_updated_objects (0.00s)
    --- PASS: TestTxStore/rollback_removed_objects (0.00s)
=== RUN   TestStore/count/obj1
=== RUN   TestStore/count/obj2
=== RUN   TestStore/iterate_start_prefix
=== RUN   TestStore/iterate_start_prefix/obj1
=== RUN   TestStore/iterate_subset_prefix
=== RUN   TestStore/iterate_subset_prefix/obj1
=== RUN   TestStore/iterate_prefix
=== RUN   TestStore/iterate_prefix/obj1
=== RUN   TestStore/iterate_prefix/obj2_decending
=== RUN   TestStore/iterate_skip_first
=== RUN   TestStore/iterate_skip_first/obj1
=== RUN   TestStore/iterate_skip_first/obj2_decending
=== RUN   TestStore/iterate_ascending
=== RUN   TestStore/iterate_ascending/obj1
=== RUN   TestStore/iterate_descending
=== RUN   TestStore/iterate_descending/obj1
=== RUN   TestStore/iterate_descending/obj2
=== RUN   TestStore/iterate_property
=== RUN   TestStore/iterate_property/key_only
=== RUN   TestStore/iterate_property/size_only
=== RUN   TestStore/iterate_filters
=== RUN   TestStore/delete
=== RUN   TestStore/count_after_delete
=== RUN   TestStore/count_after_delete/obj1
=== RUN   TestStore/count_after_delete/obj2
=== RUN   TestStore/iterate_after_delete
=== RUN   TestStore/iterate_after_delete/obj1
=== RUN   TestStore/iterate_after_delete/obj2
=== RUN   TestStore/error_during_iteration
=== RUN   TestStore/close
--- PASS: TestStore (0.00s)
    --- PASS: TestStore/create_new_entries (0.00s)
    --- PASS: TestStore/has_entries (0.00s)
    --- PASS: TestStore/get_entries (0.00s)
    --- PASS: TestStore/get_size (0.00s)
    --- PASS: TestStore/count (0.00s)
        --- PASS: TestStore/count/obj1 (0.00s)
        --- PASS: TestStore/count/obj2 (0.00s)
    --- PASS: TestStore/iterate_start_prefix (0.00s)
        --- PASS: TestStore/iterate_start_prefix/obj1 (0.00s)
    --- PASS: TestStore/iterate_subset_prefix (0.00s)
        --- PASS: TestStore/iterate_subset_prefix/obj1 (0.00s)
    --- PASS: TestStore/iterate_prefix (0.00s)
        --- PASS: TestStore/iterate_prefix/obj1 (0.00s)
        --- PASS: TestStore/iterate_prefix/obj2_decending (0.00s)
    --- PASS: TestStore/iterate_skip_first (0.00s)
        --- PASS: TestStore/iterate_skip_first/obj1 (0.00s)
        --- PASS: TestStore/iterate_skip_first/obj2_decending (0.00s)
    --- PASS: TestStore/iterate_ascending (0.00s)
        --- PASS: TestStore/iterate_ascending/obj1 (0.00s)
    --- PASS: TestStore/iterate_descending (0.00s)
        --- PASS: TestStore/iterate_descending/obj1 (0.00s)
        --- PASS: TestStore/iterate_descending/obj2 (0.00s)
    --- PASS: TestStore/iterate_property (0.00s)
        --- PASS: TestStore/iterate_property/key_only (0.00s)
        --- PASS: TestStore/iterate_property/size_only (0.00s)
    --- PASS: TestStore/iterate_filters (0.00s)
    --- PASS: TestStore/delete (0.00s)
    --- PASS: TestStore/count_after_delete (0.00s)
        --- PASS: TestStore/count_after_delete/obj1 (0.00s)
        --- PASS: TestStore/count_after_delete/obj2 (0.00s)
    --- PASS: TestStore/iterate_after_delete (0.00s)
        --- PASS: TestStore/iterate_after_delete/obj1 (0.00s)
        --- PASS: TestStore/iterate_after_delete/obj2 (0.00s)
    --- PASS: TestStore/error_during_iteration (0.00s)
    --- PASS: TestStore/close (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/storagev2/inmemstore	(cached)
=== RUN   TestStore
=== PAUSE TestStore
=== RUN   TestBatchedStore
=== PAUSE TestBatchedStore
=== RUN   TestTxStore
=== PAUSE TestTxStore
=== CONT  TestStore
=== CONT  TestTxStore
=== CONT  TestBatchedStore
=== RUN   TestBatchedStore/duplicates_are_rejected
=== RUN   TestBatchedStore/only_last_ops_are_of_interest
=== RUN   TestBatchedStore/batch_not_reusable_after_commit
=== RUN   TestBatchedStore/batch_not_usable_with_expired_context
--- PASS: TestBatchedStore (0.02s)
    --- PASS: TestBatchedStore/duplicates_are_rejected (0.00s)
    --- PASS: TestBatchedStore/only_last_ops_are_of_interest (0.00s)
    --- PASS: TestBatchedStore/batch_not_reusable_after_commit (0.00s)
    --- PASS: TestBatchedStore/batch_not_usable_with_expired_context (0.00s)
=== RUN   TestTxStore/commit_empty
=== RUN   TestStore/create_new_entries
=== RUN   TestTxStore/commit
=== RUN   TestTxStore/commit/add_new_objects
=== RUN   TestTxStore/commit/delete_existing_objects
=== RUN   TestTxStore/rollback_empty
=== RUN   TestTxStore/rollback_added_objects
=== RUN   TestStore/has_entries
=== RUN   TestStore/get_entries
=== RUN   TestStore/get_size
=== RUN   TestStore/count
=== RUN   TestStore/count/obj1
=== RUN   TestStore/count/obj2
=== RUN   TestStore/iterate_start_prefix
=== RUN   TestStore/iterate_start_prefix/obj1
=== RUN   TestStore/iterate_subset_prefix
=== RUN   TestStore/iterate_subset_prefix/obj1
=== RUN   TestStore/iterate_prefix
=== RUN   TestStore/iterate_prefix/obj1
=== RUN   TestStore/iterate_prefix/obj2_decending
=== RUN   TestTxStore/rollback_updated_objects
=== RUN   TestStore/iterate_skip_first
=== RUN   TestStore/iterate_skip_first/obj1
=== RUN   TestStore/iterate_skip_first/obj2_decending
=== RUN   TestStore/iterate_ascending
=== RUN   TestStore/iterate_ascending/obj1
=== RUN   TestStore/iterate_descending
=== RUN   TestStore/iterate_descending/obj1
=== RUN   TestStore/iterate_descending/obj2
=== RUN   TestStore/iterate_property
=== RUN   TestStore/iterate_property/key_only
=== RUN   TestStore/iterate_property/size_only
=== RUN   TestStore/iterate_filters
=== RUN   TestStore/delete
=== RUN   TestStore/count_after_delete
=== RUN   TestStore/count_after_delete/obj1
=== RUN   TestStore/count_after_delete/obj2
=== RUN   TestStore/iterate_after_delete
=== RUN   TestStore/iterate_after_delete/obj1
=== RUN   TestStore/iterate_after_delete/obj2
=== RUN   TestStore/error_during_iteration
=== RUN   TestStore/close
--- PASS: TestStore (0.08s)
    --- PASS: TestStore/create_new_entries (0.04s)
    --- PASS: TestStore/has_entries (0.00s)
    --- PASS: TestStore/get_entries (0.00s)
    --- PASS: TestStore/get_size (0.00s)
    --- PASS: TestStore/count (0.00s)
        --- PASS: TestStore/count/obj1 (0.00s)
        --- PASS: TestStore/count/obj2 (0.00s)
    --- PASS: TestStore/iterate_start_prefix (0.00s)
        --- PASS: TestStore/iterate_start_prefix/obj1 (0.00s)
    --- PASS: TestStore/iterate_subset_prefix (0.00s)
        --- PASS: TestStore/iterate_subset_prefix/obj1 (0.00s)
    --- PASS: TestStore/iterate_prefix (0.00s)
        --- PASS: TestStore/iterate_prefix/obj1 (0.00s)
        --- PASS: TestStore/iterate_prefix/obj2_decending (0.00s)
    --- PASS: TestStore/iterate_skip_first (0.00s)
        --- PASS: TestStore/iterate_skip_first/obj1 (0.00s)
        --- PASS: TestStore/iterate_skip_first/obj2_decending (0.00s)
    --- PASS: TestStore/iterate_ascending (0.00s)
        --- PASS: TestStore/iterate_ascending/obj1 (0.00s)
    --- PASS: TestStore/iterate_descending (0.00s)
        --- PASS: TestStore/iterate_descending/obj1 (0.00s)
        --- PASS: TestStore/iterate_descending/obj2 (0.00s)
    --- PASS: TestStore/iterate_property (0.00s)
        --- PASS: TestStore/iterate_property/key_only (0.00s)
        --- PASS: TestStore/iterate_property/size_only (0.00s)
    --- PASS: TestStore/iterate_filters (0.00s)
    --- PASS: TestStore/delete (0.01s)
    --- PASS: TestStore/count_after_delete (0.00s)
        --- PASS: TestStore/count_after_delete/obj1 (0.00s)
        --- PASS: TestStore/count_after_delete/obj2 (0.00s)
    --- PASS: TestStore/iterate_after_delete (0.00s)
        --- PASS: TestStore/iterate_after_delete/obj1 (0.00s)
        --- PASS: TestStore/iterate_after_delete/obj2 (0.00s)
    --- PASS: TestStore/error_during_iteration (0.00s)
    --- PASS: TestStore/close (0.00s)
=== RUN   TestTxStore/rollback_removed_objects
--- PASS: TestTxStore (0.10s)
    --- PASS: TestTxStore/commit_empty (0.00s)
    --- PASS: TestTxStore/commit (0.01s)
        --- PASS: TestTxStore/commit/add_new_objects (0.01s)
        --- PASS: TestTxStore/commit/delete_existing_objects (0.01s)
    --- PASS: TestTxStore/rollback_empty (0.00s)
    --- PASS: TestTxStore/rollback_added_objects (0.02s)
    --- PASS: TestTxStore/rollback_updated_objects (0.01s)
    --- PASS: TestTxStore/rollback_removed_objects (0.01s)
PASS
ok  	github.com/ethersphere/bee/pkg/storagev2/leveldbstore	(cached)
=== RUN   TestNewStepOnIndex
=== PAUSE TestNewStepOnIndex
=== RUN   TestStepIndex_BatchSize
=== PAUSE TestStepIndex_BatchSize
=== RUN   TestOptions
=== PAUSE TestOptions
=== RUN   TestLatestVersion
=== PAUSE TestLatestVersion
=== RUN   TestGetSetVersion
=== PAUSE TestGetSetVersion
=== RUN   TestValidateVersions
=== PAUSE TestValidateVersions
=== RUN   TestMigrate
=== PAUSE TestMigrate
=== RUN   TestTagIDAddressItem_MarshalAndUnmarshal
=== PAUSE TestTagIDAddressItem_MarshalAndUnmarshal
=== RUN   TestNewStepsChain
=== PAUSE TestNewStepsChain
=== CONT  TestNewStepOnIndex
=== CONT  TestValidateVersions
=== RUN   TestValidateVersions/empty
=== PAUSE TestValidateVersions/empty
=== RUN   TestValidateVersions/missing_version_3
=== PAUSE TestValidateVersions/missing_version_3
=== RUN   TestValidateVersions/not_missing
=== PAUSE TestValidateVersions/not_missing
=== RUN   TestValidateVersions/desc_order_versions
=== PAUSE TestValidateVersions/desc_order_versions
=== RUN   TestValidateVersions/desc_order_version_missing
=== PAUSE TestValidateVersions/desc_order_version_missing
=== CONT  TestTagIDAddressItem_MarshalAndUnmarshal
=== RUN   TestNewStepOnIndex/noop_step
=== PAUSE TestNewStepOnIndex/noop_step
=== RUN   TestNewStepOnIndex/delete_items
=== PAUSE TestNewStepOnIndex/delete_items
=== RUN   TestNewStepOnIndex/update_items
=== PAUSE TestNewStepOnIndex/update_items
=== RUN   TestNewStepOnIndex/delete_and_update_items
=== PAUSE TestNewStepOnIndex/delete_and_update_items
=== RUN   TestNewStepOnIndex/update_with_ID_change
=== PAUSE TestNewStepOnIndex/update_with_ID_change
=== CONT  TestGetSetVersion
=== RUN   TestGetSetVersion/Version
=== PAUSE TestGetSetVersion/Version
=== RUN   TestGetSetVersion/SetVersion
=== PAUSE TestGetSetVersion/SetVersion
=== CONT  TestValidateVersions/not_missing
=== CONT  TestValidateVersions/missing_version_3
=== CONT  TestValidateVersions/desc_order_version_missing
=== CONT  TestValidateVersions/desc_order_versions
=== CONT  TestNewStepOnIndex/noop_step
=== CONT  TestValidateVersions/empty
--- PASS: TestValidateVersions (0.00s)
    --- PASS: TestValidateVersions/not_missing (0.00s)
    --- PASS: TestValidateVersions/missing_version_3 (0.00s)
    --- PASS: TestValidateVersions/desc_order_version_missing (0.00s)
    --- PASS: TestValidateVersions/desc_order_versions (0.00s)
    --- PASS: TestValidateVersions/empty (0.00s)
=== CONT  TestOptions
=== RUN   TestOptions/new_options
=== PAUSE TestOptions/new_options
=== RUN   TestOptions/delete_option_apply
=== PAUSE TestOptions/delete_option_apply
=== RUN   TestOptions/update_option_apply
=== PAUSE TestOptions/update_option_apply
=== RUN   TestOptions/opPerBatch_option_apply
=== PAUSE TestOptions/opPerBatch_option_apply
=== CONT  TestNewStepOnIndex/update_items
=== CONT  TestNewStepOnIndex/delete_items
=== CONT  TestMigrate
=== CONT  TestNewStepOnIndex/update_with_ID_change
=== RUN   TestMigrate/migration:_0_to_3
=== PAUSE TestMigrate/migration:_0_to_3
=== RUN   TestMigrate/migration:_5_to_8
=== PAUSE TestMigrate/migration:_5_to_8
=== RUN   TestMigrate/migration:_5_to_8_with_steps_error
=== PAUSE TestMigrate/migration:_5_to_8_with_steps_error
=== CONT  TestGetSetVersion/Version
=== CONT  TestGetSetVersion/SetVersion
=== RUN   TestTagIDAddressItem_MarshalAndUnmarshal/zero_values
=== PAUSE TestTagIDAddressItem_MarshalAndUnmarshal/zero_values
=== CONT  TestOptions/new_options
=== CONT  TestOptions/delete_option_apply
=== CONT  TestNewStepOnIndex/delete_and_update_items
=== CONT  TestMigrate/migration:_0_to_3
=== CONT  TestMigrate/migration:_5_to_8
=== CONT  TestMigrate/migration:_5_to_8_with_steps_error
=== CONT  TestStepIndex_BatchSize
=== RUN   TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_1
=== PAUSE TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_1
=== CONT  TestNewStepsChain
--- PASS: TestGetSetVersion (0.00s)
    --- PASS: TestGetSetVersion/Version (0.00s)
    --- PASS: TestGetSetVersion/SetVersion (0.00s)
=== CONT  TestOptions/update_option_apply
=== CONT  TestLatestVersion
--- PASS: TestNewStepOnIndex (0.00s)
    --- PASS: TestNewStepOnIndex/noop_step (0.00s)
    --- PASS: TestNewStepOnIndex/delete_items (0.00s)
    --- PASS: TestNewStepOnIndex/update_items (0.00s)
    --- PASS: TestNewStepOnIndex/update_with_ID_change (0.00s)
    --- PASS: TestNewStepOnIndex/delete_and_update_items (0.00s)
=== CONT  TestOptions/opPerBatch_option_apply
--- PASS: TestMigrate (0.00s)
    --- PASS: TestMigrate/migration:_0_to_3 (0.00s)
    --- PASS: TestMigrate/migration:_5_to_8 (0.00s)
    --- PASS: TestMigrate/migration:_5_to_8_with_steps_error (0.00s)
--- PASS: TestLatestVersion (0.00s)
--- PASS: TestOptions (0.00s)
    --- PASS: TestOptions/new_options (0.00s)
    --- PASS: TestOptions/delete_option_apply (0.00s)
    --- PASS: TestOptions/update_option_apply (0.00s)
    --- PASS: TestOptions/opPerBatch_option_apply (0.00s)
=== RUN   TestTagIDAddressItem_MarshalAndUnmarshal/max_value
=== PAUSE TestTagIDAddressItem_MarshalAndUnmarshal/max_value
=== RUN   TestTagIDAddressItem_MarshalAndUnmarshal/invalid_size
=== PAUSE TestTagIDAddressItem_MarshalAndUnmarshal/invalid_size
=== RUN   TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_2
=== RUN   TestTagIDAddressItem_MarshalAndUnmarshal/random_value
=== PAUSE TestTagIDAddressItem_MarshalAndUnmarshal/random_value
=== CONT  TestTagIDAddressItem_MarshalAndUnmarshal/zero_values
--- PASS: TestNewStepsChain (0.00s)
=== CONT  TestTagIDAddressItem_MarshalAndUnmarshal/random_value
=== PAUSE TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_2
=== RUN   TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_4
=== PAUSE TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_4
=== CONT  TestTagIDAddressItem_MarshalAndUnmarshal/invalid_size
=== CONT  TestTagIDAddressItem_MarshalAndUnmarshal/max_value
--- PASS: TestTagIDAddressItem_MarshalAndUnmarshal (0.00s)
    --- PASS: TestTagIDAddressItem_MarshalAndUnmarshal/zero_values (0.00s)
    --- PASS: TestTagIDAddressItem_MarshalAndUnmarshal/random_value (0.00s)
    --- PASS: TestTagIDAddressItem_MarshalAndUnmarshal/invalid_size (0.00s)
    --- PASS: TestTagIDAddressItem_MarshalAndUnmarshal/max_value (0.00s)
=== RUN   TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_8
=== PAUSE TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_8
=== RUN   TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_16
=== PAUSE TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_16
=== RUN   TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_32
=== PAUSE TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_32
=== RUN   TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_64
=== PAUSE TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_64
=== RUN   TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_128
=== PAUSE TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_128
=== RUN   TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_256
=== PAUSE TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_256
=== CONT  TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_1
=== CONT  TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_64
=== CONT  TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_8
=== CONT  TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_4
=== CONT  TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_16
=== CONT  TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_256
=== CONT  TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_2
=== CONT  TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_32
=== CONT  TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_128
--- PASS: TestStepIndex_BatchSize (0.00s)
    --- PASS: TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_64 (0.00s)
    --- PASS: TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_8 (0.00s)
    --- PASS: TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_4 (0.00s)
    --- PASS: TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_16 (0.00s)
    --- PASS: TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_256 (0.00s)
    --- PASS: TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_32 (0.00s)
    --- PASS: TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_128 (0.00s)
    --- PASS: TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_2 (0.00s)
    --- PASS: TestStepIndex_BatchSize/callback_called_once_per_item_with_batch_size:_1 (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/storagev2/migration	(cached)
=== RUN   TestCompressibleBytes
=== PAUSE TestCompressibleBytes
=== RUN   TestRandomValueGenerator
=== PAUSE TestRandomValueGenerator
=== RUN   TestFullRandomEntryGenerator
=== PAUSE TestFullRandomEntryGenerator
=== RUN   TestSequentialEntryGenerator
=== PAUSE TestSequentialEntryGenerator
=== RUN   TestReverseGenerator
=== PAUSE TestReverseGenerator
=== RUN   TestStartAtEntryGenerator
=== PAUSE TestStartAtEntryGenerator
=== RUN   TestRoundKeyGenerator
=== PAUSE TestRoundKeyGenerator
=== CONT  TestCompressibleBytes
--- PASS: TestCompressibleBytes (0.00s)
=== CONT  TestReverseGenerator
=== RUN   TestReverseGenerator/generated_values_are_consecutive_descending
=== CONT  TestRoundKeyGenerator
--- PASS: TestReverseGenerator (0.00s)
    --- PASS: TestReverseGenerator/generated_values_are_consecutive_descending (0.00s)
=== CONT  TestStartAtEntryGenerator
=== RUN   TestStartAtEntryGenerator/generated_values_are_consecutive_ascending
=== RUN   TestRoundKeyGenerator/repeating_values_are_generated
=== CONT  TestFullRandomEntryGenerator
=== CONT  TestSequentialEntryGenerator
=== CONT  TestRandomValueGenerator
--- PASS: TestRoundKeyGenerator (0.01s)
    --- PASS: TestRoundKeyGenerator/repeating_values_are_generated (0.01s)
=== RUN   TestFullRandomEntryGenerator/startAt_is_respected
=== RUN   TestRandomValueGenerator/generates_random_values
=== RUN   TestSequentialEntryGenerator/generated_values_are_consecutive_ascending
--- PASS: TestStartAtEntryGenerator (0.02s)
    --- PASS: TestStartAtEntryGenerator/generated_values_are_consecutive_ascending (0.02s)
=== RUN   TestRandomValueGenerator/respects_value_size
--- PASS: TestFullRandomEntryGenerator (0.02s)
    --- PASS: TestFullRandomEntryGenerator/startAt_is_respected (0.01s)
--- PASS: TestSequentialEntryGenerator (0.02s)
    --- PASS: TestSequentialEntryGenerator/generated_values_are_consecutive_ascending (0.02s)
--- PASS: TestRandomValueGenerator (0.03s)
    --- PASS: TestRandomValueGenerator/generates_random_values (0.01s)
    --- PASS: TestRandomValueGenerator/respects_value_size (0.01s)
PASS
ok  	github.com/ethersphere/bee/pkg/storagev2/storagetest	(cached)
=== RUN   TestProximity
=== PAUSE TestProximity
=== RUN   TestDistance
=== PAUSE TestDistance
=== RUN   TestDistanceCmp
=== PAUSE TestDistanceCmp
=== RUN   TestAddress
=== PAUSE TestAddress
=== RUN   TestAddress_jsonMarshalling
=== PAUSE TestAddress_jsonMarshalling
=== RUN   TestAddress_MemberOf
=== PAUSE TestAddress_MemberOf
=== RUN   TestAddress_Clone
=== PAUSE TestAddress_Clone
=== RUN   TestCloser
=== PAUSE TestCloser
=== RUN   Test_ContainsAddress
=== PAUSE Test_ContainsAddress
=== RUN   Test_IndexOfAddress
=== PAUSE Test_IndexOfAddress
=== RUN   Test_RemoveAddress
=== PAUSE Test_RemoveAddress
=== RUN   Test_IndexOfChunkWithAddress
=== PAUSE Test_IndexOfChunkWithAddress
=== RUN   Test_ContainsChunkWithData
=== PAUSE Test_ContainsChunkWithData
=== RUN   Test_FindStampWithBatchID
=== PAUSE Test_FindStampWithBatchID
=== CONT  TestProximity
=== CONT  TestCloser
=== CONT  TestAddress_MemberOf
=== CONT  Test_IndexOfAddress
--- PASS: TestProximity (0.00s)
=== CONT  TestAddress
--- PASS: TestCloser (0.00s)
=== RUN   TestAddress/blank
=== CONT  Test_RemoveAddress
=== PAUSE TestAddress/blank
=== RUN   TestAddress/odd
=== CONT  Test_ContainsAddress
=== PAUSE TestAddress/odd
=== CONT  TestAddress_jsonMarshalling
--- PASS: TestAddress_MemberOf (0.00s)
--- PASS: Test_IndexOfAddress (0.00s)
--- PASS: Test_RemoveAddress (0.00s)
--- PASS: Test_ContainsAddress (0.00s)
=== CONT  Test_IndexOfChunkWithAddress
--- PASS: TestAddress_jsonMarshalling (0.00s)
=== RUN   TestAddress/zero
=== CONT  TestDistance
=== PAUSE TestAddress/zero
=== RUN   TestAddress/one
=== PAUSE TestAddress/one
=== RUN   TestAddress/arbitrary
=== PAUSE TestAddress/arbitrary
=== CONT  TestAddress/blank
=== CONT  TestAddress_Clone
=== CONT  TestDistanceCmp
--- PASS: Test_IndexOfChunkWithAddress (0.00s)
--- PASS: TestDistance (0.00s)
=== CONT  TestAddress/arbitrary
=== CONT  TestAddress/one
=== CONT  TestAddress/zero
=== CONT  Test_FindStampWithBatchID
--- PASS: TestAddress_Clone (0.00s)
--- PASS: TestDistanceCmp (0.00s)
=== CONT  Test_ContainsChunkWithData
--- PASS: Test_FindStampWithBatchID (0.00s)
--- PASS: Test_ContainsChunkWithData (0.00s)
=== CONT  TestAddress/odd
--- PASS: TestAddress (0.00s)
    --- PASS: TestAddress/blank (0.00s)
    --- PASS: TestAddress/arbitrary (0.00s)
    --- PASS: TestAddress/one (0.00s)
    --- PASS: TestAddress/zero (0.00s)
    --- PASS: TestAddress/odd (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/swarm	(cached)
=== RUN   TestRandomAddressAt
=== PAUSE TestRandomAddressAt
=== CONT  TestRandomAddressAt
--- PASS: TestRandomAddressAt (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/swarm/test	(cached)
=== RUN   TestTagSingleIncrements
=== PAUSE TestTagSingleIncrements
=== RUN   TestTagStatus
=== PAUSE TestTagStatus
=== RUN   TestTagETA
=== PAUSE TestTagETA
=== RUN   TestTagConcurrentIncrements
=== PAUSE TestTagConcurrentIncrements
=== RUN   TestTagsMultipleConcurrentIncrementsSyncMap
=== PAUSE TestTagsMultipleConcurrentIncrementsSyncMap
=== RUN   TestMarshallingWithAddr
=== PAUSE TestMarshallingWithAddr
=== RUN   TestMarshallingNoAddr
=== PAUSE TestMarshallingNoAddr
=== RUN   TestAll
=== PAUSE TestAll
=== RUN   TestListAll
=== PAUSE TestListAll
=== RUN   TestPersistence
=== PAUSE TestPersistence
=== CONT  TestPersistence
=== CONT  TestTagSingleIncrements
=== CONT  TestListAll
=== CONT  TestAll
--- PASS: TestAll (0.00s)
--- PASS: TestPersistence (0.00s)
--- PASS: TestTagSingleIncrements (0.00s)
=== CONT  TestTagsMultipleConcurrentIncrementsSyncMap
=== CONT  TestTagStatus
--- PASS: TestListAll (0.00s)
--- PASS: TestTagStatus (0.00s)
=== CONT  TestMarshallingWithAddr
--- PASS: TestMarshallingWithAddr (0.00s)
=== CONT  TestTagETA
=== CONT  TestTagConcurrentIncrements
--- PASS: TestTagConcurrentIncrements (0.00s)
=== CONT  TestMarshallingNoAddr
--- PASS: TestMarshallingNoAddr (0.00s)
--- PASS: TestTagsMultipleConcurrentIncrementsSyncMap (0.00s)
--- PASS: TestTagETA (0.10s)
PASS
ok  	github.com/ethersphere/bee/pkg/tags	(cached)
=== RUN   TestDepthMonitorService_FLAKY
=== PAUSE TestDepthMonitorService_FLAKY
=== CONT  TestDepthMonitorService_FLAKY
=== RUN   TestDepthMonitorService_FLAKY/stop_service_within_warmup_time
=== PAUSE TestDepthMonitorService_FLAKY/stop_service_within_warmup_time
=== RUN   TestDepthMonitorService_FLAKY/old_nodes_starts_at_previous_radius
=== PAUSE TestDepthMonitorService_FLAKY/old_nodes_starts_at_previous_radius
=== RUN   TestDepthMonitorService_FLAKY/start_with_radius
=== PAUSE TestDepthMonitorService_FLAKY/start_with_radius
=== RUN   TestDepthMonitorService_FLAKY/depth_decrease_due_to_under_utilization
=== PAUSE TestDepthMonitorService_FLAKY/depth_decrease_due_to_under_utilization
=== RUN   TestDepthMonitorService_FLAKY/depth_doesnt_change_due_to_non-zero_pull_rate
=== PAUSE TestDepthMonitorService_FLAKY/depth_doesnt_change_due_to_non-zero_pull_rate
=== RUN   TestDepthMonitorService_FLAKY/depth_doesnt_change_for_utilized_reserve
=== PAUSE TestDepthMonitorService_FLAKY/depth_doesnt_change_for_utilized_reserve
=== RUN   TestDepthMonitorService_FLAKY/radius_setter_handler
=== PAUSE TestDepthMonitorService_FLAKY/radius_setter_handler
=== CONT  TestDepthMonitorService_FLAKY/stop_service_within_warmup_time
=== CONT  TestDepthMonitorService_FLAKY/radius_setter_handler
=== CONT  TestDepthMonitorService_FLAKY/depth_doesnt_change_for_utilized_reserve
=== CONT  TestDepthMonitorService_FLAKY/depth_doesnt_change_due_to_non-zero_pull_rate
=== CONT  TestDepthMonitorService_FLAKY/depth_decrease_due_to_under_utilization
=== CONT  TestDepthMonitorService_FLAKY/start_with_radius
=== CONT  TestDepthMonitorService_FLAKY/old_nodes_starts_at_previous_radius
--- PASS: TestDepthMonitorService_FLAKY (0.00s)
    --- PASS: TestDepthMonitorService_FLAKY/stop_service_within_warmup_time (0.00s)
    --- PASS: TestDepthMonitorService_FLAKY/old_nodes_starts_at_previous_radius (0.02s)
    --- PASS: TestDepthMonitorService_FLAKY/start_with_radius (0.02s)
    --- PASS: TestDepthMonitorService_FLAKY/radius_setter_handler (0.02s)
    --- PASS: TestDepthMonitorService_FLAKY/depth_decrease_due_to_under_utilization (0.06s)
    --- PASS: TestDepthMonitorService_FLAKY/depth_doesnt_change_due_to_non-zero_pull_rate (1.00s)
    --- PASS: TestDepthMonitorService_FLAKY/depth_doesnt_change_for_utilized_reserve (1.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/topology/depthmonitor	(cached)
=== RUN   TestNeighborhoodDepth
=== PAUSE TestNeighborhoodDepth
=== RUN   TestNeighborhoodDepthWithReachability
=== PAUSE TestNeighborhoodDepthWithReachability
=== RUN   TestEachNeighbor
=== PAUSE TestEachNeighbor
=== RUN   TestManage
=== PAUSE TestManage
=== RUN   TestManageWithBalancing
=== PAUSE TestManageWithBalancing
=== RUN   TestBinSaturation
=== PAUSE TestBinSaturation
=== RUN   TestOversaturation
=== PAUSE TestOversaturation
=== RUN   TestOversaturationBootnode
=== PAUSE TestOversaturationBootnode
=== RUN   TestBootnodeMaxConnections
=== PAUSE TestBootnodeMaxConnections
=== RUN   TestNotifierHooks
=== PAUSE TestNotifierHooks
=== RUN   TestDiscoveryHooks
=== PAUSE TestDiscoveryHooks
=== RUN   TestAnnounceTo
=== PAUSE TestAnnounceTo
=== RUN   TestBackoff
=== PAUSE TestBackoff
=== RUN   TestAddressBookPrune
=== PAUSE TestAddressBookPrune
=== RUN   TestAddressBookQuickPrune
=== PAUSE TestAddressBookQuickPrune
=== RUN   TestClosestPeer
=== PAUSE TestClosestPeer
=== RUN   TestKademlia_SubscribeTopologyChange
=== PAUSE TestKademlia_SubscribeTopologyChange
=== RUN   TestSnapshot_FLAKY
=== PAUSE TestSnapshot_FLAKY
=== RUN   TestStart
=== PAUSE TestStart
=== RUN   TestOutofDepthPrune
=== PAUSE TestOutofDepthPrune
=== RUN   TestLatency
=== PAUSE TestLatency
=== RUN   TestBootnodeProtectedNodes
=== PAUSE TestBootnodeProtectedNodes
=== RUN   TestAnnounceBgBroadcast_FLAKY
=== PAUSE TestAnnounceBgBroadcast_FLAKY
=== RUN   TestAnnounceNeighborhoodToNeighbor
=== PAUSE TestAnnounceNeighborhoodToNeighbor
=== RUN   TestIteratorOpts
=== PAUSE TestIteratorOpts
=== CONT  TestNeighborhoodDepth
=== CONT  TestAddressBookPrune
=== CONT  TestAnnounceBgBroadcast_FLAKY
=== CONT  TestOutofDepthPrune
=== CONT  TestBootnodeProtectedNodes
=== CONT  TestBackoff
=== CONT  TestLatency
=== CONT  TestKademlia_SubscribeTopologyChange
=== RUN   TestKademlia_SubscribeTopologyChange/single_subscription
=== PAUSE TestKademlia_SubscribeTopologyChange/single_subscription
=== RUN   TestKademlia_SubscribeTopologyChange/single_subscription,_remove_peer
=== PAUSE TestKademlia_SubscribeTopologyChange/single_subscription,_remove_peer
=== RUN   TestKademlia_SubscribeTopologyChange/multiple_subscriptions
=== PAUSE TestKademlia_SubscribeTopologyChange/multiple_subscriptions
=== RUN   TestKademlia_SubscribeTopologyChange/multiple_changes
=== PAUSE TestKademlia_SubscribeTopologyChange/multiple_changes
=== RUN   TestKademlia_SubscribeTopologyChange/no_depth_change
=== PAUSE TestKademlia_SubscribeTopologyChange/no_depth_change
=== CONT  TestStart
=== RUN   TestStart/non-empty_addressbook
=== PAUSE TestStart/non-empty_addressbook
=== RUN   TestStart/empty_addressbook
=== PAUSE TestStart/empty_addressbook
=== CONT  TestSnapshot_FLAKY
--- PASS: TestSnapshot_FLAKY (0.04s)
=== CONT  TestIteratorOpts
=== RUN   TestIteratorOpts/EachPeer_reachable
=== PAUSE TestIteratorOpts/EachPeer_reachable
=== RUN   TestIteratorOpts/EachPeerRev_reachable
=== PAUSE TestIteratorOpts/EachPeerRev_reachable
=== CONT  TestOversaturation
--- PASS: TestAnnounceBgBroadcast_FLAKY (0.15s)
=== CONT  TestAnnounceTo
--- PASS: TestAnnounceTo (0.04s)
=== CONT  TestDiscoveryHooks
--- PASS: TestAddressBookPrune (0.30s)
=== CONT  TestNotifierHooks
    kademlia_test.go:741: disabled due to kademlia inconsistencies hotfix
--- SKIP: TestNotifierHooks (0.00s)
=== CONT  TestBootnodeMaxConnections
--- PASS: TestDiscoveryHooks (0.15s)
=== CONT  TestOversaturationBootnode
--- PASS: TestOversaturation (0.31s)
=== CONT  TestAnnounceNeighborhoodToNeighbor
--- PASS: TestBootnodeProtectedNodes (0.39s)
=== CONT  TestClosestPeer
    kademlia_test.go:996: disabled due to kademlia inconsistencies hotfix
--- SKIP: TestClosestPeer (0.00s)
=== CONT  TestManage
--- PASS: TestBootnodeMaxConnections (0.17s)
=== CONT  TestBinSaturation
--- PASS: TestManage (0.07s)
=== CONT  TestManageWithBalancing
--- PASS: TestBinSaturation (0.09s)
=== CONT  TestEachNeighbor
--- PASS: TestEachNeighbor (0.03s)
=== CONT  TestAddressBookQuickPrune
--- PASS: TestBackoff (0.60s)
=== CONT  TestNeighborhoodDepthWithReachability
--- PASS: TestAddressBookQuickPrune (0.18s)
=== CONT  TestKademlia_SubscribeTopologyChange/single_subscription
=== CONT  TestKademlia_SubscribeTopologyChange/multiple_changes
=== CONT  TestKademlia_SubscribeTopologyChange/multiple_subscriptions
=== CONT  TestKademlia_SubscribeTopologyChange/single_subscription,_remove_peer
=== CONT  TestKademlia_SubscribeTopologyChange/no_depth_change
--- PASS: TestOversaturationBootnode (0.51s)
=== CONT  TestStart/non-empty_addressbook
    kademlia_test.go:1287: test flakes
=== CONT  TestStart/empty_addressbook
--- PASS: TestStart (0.00s)
    --- SKIP: TestStart/non-empty_addressbook (0.00s)
    --- PASS: TestStart/empty_addressbook (0.04s)
=== CONT  TestIteratorOpts/EachPeer_reachable
=== CONT  TestIteratorOpts/EachPeerRev_reachable
--- PASS: TestIteratorOpts (0.01s)
    --- PASS: TestIteratorOpts/EachPeer_reachable (0.00s)
    --- PASS: TestIteratorOpts/EachPeerRev_reachable (0.00s)
--- PASS: TestAnnounceNeighborhoodToNeighbor (0.52s)
--- PASS: TestLatency (1.02s)
--- PASS: TestOutofDepthPrune (1.36s)
--- PASS: TestKademlia_SubscribeTopologyChange (0.02s)
    --- PASS: TestKademlia_SubscribeTopologyChange/single_subscription (0.00s)
    --- PASS: TestKademlia_SubscribeTopologyChange/multiple_changes (0.00s)
    --- PASS: TestKademlia_SubscribeTopologyChange/multiple_subscriptions (0.00s)
    --- PASS: TestKademlia_SubscribeTopologyChange/single_subscription,_remove_peer (0.00s)
    --- PASS: TestKademlia_SubscribeTopologyChange/no_depth_change (1.00s)
--- PASS: TestManageWithBalancing (1.80s)
--- PASS: TestNeighborhoodDepth (3.60s)
--- PASS: TestNeighborhoodDepthWithReachability (3.57s)
PASS
ok  	github.com/ethersphere/bee/pkg/topology/kademlia	(cached)
=== RUN   TestPeerMetricsCollector
=== PAUSE TestPeerMetricsCollector
=== CONT  TestPeerMetricsCollector
--- PASS: TestPeerMetricsCollector (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/topology/kademlia/internal/metrics	(cached)
=== RUN   TestSet
=== PAUSE TestSet
=== CONT  TestSet
--- PASS: TestSet (0.02s)
PASS
ok  	github.com/ethersphere/bee/pkg/topology/kademlia/internal/waitnext	(cached)
=== RUN   TestContainer
=== PAUSE TestContainer
=== CONT  TestContainer
=== RUN   TestContainer/new_container_is_empty_container
=== PAUSE TestContainer/new_container_is_empty_container
=== RUN   TestContainer/can_add_peers_to_container
=== PAUSE TestContainer/can_add_peers_to_container
=== RUN   TestContainer/empty_container_after_peer_disconnect
=== PAUSE TestContainer/empty_container_after_peer_disconnect
=== CONT  TestContainer/new_container_is_empty_container
=== CONT  TestContainer/empty_container_after_peer_disconnect
=== CONT  TestContainer/can_add_peers_to_container
--- PASS: TestContainer (0.00s)
    --- PASS: TestContainer/new_container_is_empty_container (0.00s)
    --- PASS: TestContainer/empty_container_after_peer_disconnect (0.00s)
    --- PASS: TestContainer/can_add_peers_to_container (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/topology/lightnode	(cached)
=== RUN   TestShallowestEmpty
=== PAUSE TestShallowestEmpty
=== RUN   TestNoPanicOnEmptyRemove
=== PAUSE TestNoPanicOnEmptyRemove
=== RUN   TestAddRemove
=== PAUSE TestAddRemove
=== RUN   TestIteratorError
=== PAUSE TestIteratorError
=== RUN   TestIterators
=== PAUSE TestIterators
=== RUN   TestBinPeers
=== PAUSE TestBinPeers
=== RUN   TestIteratorsJumpStop
=== PAUSE TestIteratorsJumpStop
=== CONT  TestShallowestEmpty
--- PASS: TestShallowestEmpty (0.00s)
=== CONT  TestIteratorsJumpStop
--- PASS: TestIteratorsJumpStop (0.00s)
=== CONT  TestBinPeers
=== RUN   TestBinPeers/bins-empty
=== PAUSE TestBinPeers/bins-empty
=== RUN   TestBinPeers/some-bins-empty
=== PAUSE TestBinPeers/some-bins-empty
=== RUN   TestBinPeers/some-bins-empty#01
=== PAUSE TestBinPeers/some-bins-empty#01
=== RUN   TestBinPeers/full-bins
=== PAUSE TestBinPeers/full-bins
=== CONT  TestBinPeers/bins-empty
=== CONT  TestIterators
--- PASS: TestIterators (0.00s)
=== CONT  TestIteratorError
--- PASS: TestIteratorError (0.00s)
=== CONT  TestAddRemove
--- PASS: TestAddRemove (0.00s)
=== CONT  TestNoPanicOnEmptyRemove
--- PASS: TestNoPanicOnEmptyRemove (0.00s)
=== CONT  TestBinPeers/full-bins
=== CONT  TestBinPeers/some-bins-empty#01
=== CONT  TestBinPeers/some-bins-empty
--- PASS: TestBinPeers (0.00s)
    --- PASS: TestBinPeers/bins-empty (0.00s)
    --- PASS: TestBinPeers/full-bins (0.00s)
    --- PASS: TestBinPeers/some-bins-empty#01 (0.00s)
    --- PASS: TestBinPeers/some-bins-empty (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/topology/pslice	(cached)
=== RUN   TestSpanFromHeaders
=== PAUSE TestSpanFromHeaders
=== RUN   TestSpanWithContextFromHeaders
=== PAUSE TestSpanWithContextFromHeaders
=== RUN   TestFromContext
=== PAUSE TestFromContext
=== RUN   TestWithContext
=== PAUSE TestWithContext
=== RUN   TestStartSpanFromContext_logger
=== PAUSE TestStartSpanFromContext_logger
=== RUN   TestStartSpanFromContext_nilLogger
=== PAUSE TestStartSpanFromContext_nilLogger
=== RUN   TestNewLoggerWithTraceID
=== PAUSE TestNewLoggerWithTraceID
=== RUN   TestNewLoggerWithTraceID_nilLogger
=== PAUSE TestNewLoggerWithTraceID_nilLogger
=== CONT  TestSpanFromHeaders
=== CONT  TestStartSpanFromContext_logger
=== CONT  TestStartSpanFromContext_nilLogger
=== CONT  TestNewLoggerWithTraceID_nilLogger
=== CONT  TestNewLoggerWithTraceID
=== CONT  TestWithContext
=== CONT  TestSpanWithContextFromHeaders
=== CONT  TestFromContext
--- PASS: TestStartSpanFromContext_logger (0.00s)
--- PASS: TestFromContext (0.00s)
--- PASS: TestSpanFromHeaders (0.00s)
--- PASS: TestSpanWithContextFromHeaders (0.00s)
--- PASS: TestWithContext (0.00s)
--- PASS: TestStartSpanFromContext_nilLogger (0.00s)
--- PASS: TestNewLoggerWithTraceID_nilLogger (0.00s)
--- PASS: TestNewLoggerWithTraceID (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/tracing	(cached)
=== RUN   TestIsSynced
=== PAUSE TestIsSynced
=== RUN   TestParseEvent
=== PAUSE TestParseEvent
=== RUN   TestFindSingleEvent
=== PAUSE TestFindSingleEvent
=== RUN   TestMonitorWatchTransaction
=== PAUSE TestMonitorWatchTransaction
=== RUN   TestTransactionSend
=== PAUSE TestTransactionSend
=== RUN   TestTransactionWaitForReceipt
=== PAUSE TestTransactionWaitForReceipt
=== RUN   TestTransactionResend
=== PAUSE TestTransactionResend
=== RUN   TestTransactionCancel
=== PAUSE TestTransactionCancel
=== CONT  TestIsSynced
=== RUN   TestIsSynced/synced
=== PAUSE TestIsSynced/synced
=== RUN   TestIsSynced/not_synced
=== PAUSE TestIsSynced/not_synced
=== RUN   TestIsSynced/error
=== PAUSE TestIsSynced/error
=== CONT  TestIsSynced/synced
=== CONT  TestTransactionCancel
=== RUN   TestTransactionCancel/ok
=== PAUSE TestTransactionCancel/ok
=== RUN   TestTransactionCancel/custom_gas_price
=== PAUSE TestTransactionCancel/custom_gas_price
=== CONT  TestTransactionCancel/ok
=== CONT  TestTransactionResend
--- PASS: TestTransactionResend (0.00s)
=== CONT  TestTransactionWaitForReceipt
--- PASS: TestTransactionWaitForReceipt (0.00s)
=== CONT  TestTransactionSend
=== RUN   TestTransactionSend/send
=== PAUSE TestTransactionSend/send
=== RUN   TestTransactionSend/sendWithBoost
=== PAUSE TestTransactionSend/sendWithBoost
=== RUN   TestTransactionSend/send_no_nonce
=== PAUSE TestTransactionSend/send_no_nonce
=== RUN   TestTransactionSend/send_skipped_nonce
=== PAUSE TestTransactionSend/send_skipped_nonce
=== CONT  TestTransactionSend/send
=== CONT  TestMonitorWatchTransaction
=== RUN   TestMonitorWatchTransaction/single_transaction_confirmed
=== PAUSE TestMonitorWatchTransaction/single_transaction_confirmed
=== RUN   TestMonitorWatchTransaction/single_transaction_cancelled
=== PAUSE TestMonitorWatchTransaction/single_transaction_cancelled
=== RUN   TestMonitorWatchTransaction/multiple_transactions_mixed
=== PAUSE TestMonitorWatchTransaction/multiple_transactions_mixed
=== RUN   TestMonitorWatchTransaction/single_transaction_no_confirm
=== PAUSE TestMonitorWatchTransaction/single_transaction_no_confirm
=== RUN   TestMonitorWatchTransaction/shutdown_while_waiting
=== PAUSE TestMonitorWatchTransaction/shutdown_while_waiting
=== CONT  TestMonitorWatchTransaction/single_transaction_confirmed
=== CONT  TestFindSingleEvent
=== RUN   TestFindSingleEvent/ok
=== PAUSE TestFindSingleEvent/ok
=== RUN   TestFindSingleEvent/not_found
=== PAUSE TestFindSingleEvent/not_found
=== RUN   TestFindSingleEvent/Reverted
=== PAUSE TestFindSingleEvent/Reverted
=== CONT  TestFindSingleEvent/ok
=== CONT  TestParseEvent
=== RUN   TestParseEvent/ok
=== PAUSE TestParseEvent/ok
=== RUN   TestParseEvent/no_topic
=== PAUSE TestParseEvent/no_topic
=== CONT  TestParseEvent/ok
=== CONT  TestIsSynced/error
=== CONT  TestIsSynced/not_synced
--- PASS: TestIsSynced (0.00s)
    --- PASS: TestIsSynced/synced (0.00s)
    --- PASS: TestIsSynced/error (0.00s)
    --- PASS: TestIsSynced/not_synced (0.00s)
=== CONT  TestTransactionCancel/custom_gas_price
=== CONT  TestTransactionSend/send_skipped_nonce
=== CONT  TestTransactionSend/send_no_nonce
=== CONT  TestTransactionSend/sendWithBoost
--- PASS: TestTransactionSend (0.00s)
    --- PASS: TestTransactionSend/send (0.00s)
    --- PASS: TestTransactionSend/send_skipped_nonce (0.00s)
    --- PASS: TestTransactionSend/send_no_nonce (0.00s)
    --- PASS: TestTransactionSend/sendWithBoost (0.00s)
=== CONT  TestMonitorWatchTransaction/shutdown_while_waiting
=== CONT  TestMonitorWatchTransaction/single_transaction_no_confirm
=== CONT  TestMonitorWatchTransaction/multiple_transactions_mixed
=== CONT  TestMonitorWatchTransaction/single_transaction_cancelled
=== CONT  TestFindSingleEvent/Reverted
=== CONT  TestFindSingleEvent/not_found
--- PASS: TestFindSingleEvent (0.00s)
    --- PASS: TestFindSingleEvent/ok (0.00s)
    --- PASS: TestFindSingleEvent/Reverted (0.00s)
    --- PASS: TestFindSingleEvent/not_found (0.00s)
=== CONT  TestParseEvent/no_topic
--- PASS: TestParseEvent (0.00s)
    --- PASS: TestParseEvent/ok (0.00s)
    --- PASS: TestParseEvent/no_topic (0.00s)
--- PASS: TestTransactionCancel (0.00s)
    --- PASS: TestTransactionCancel/ok (0.00s)
    --- PASS: TestTransactionCancel/custom_gas_price (0.00s)
--- PASS: TestMonitorWatchTransaction (0.00s)
    --- PASS: TestMonitorWatchTransaction/shutdown_while_waiting (0.00s)
    --- PASS: TestMonitorWatchTransaction/single_transaction_confirmed (0.00s)
    --- PASS: TestMonitorWatchTransaction/single_transaction_cancelled (0.00s)
    --- PASS: TestMonitorWatchTransaction/single_transaction_no_confirm (0.01s)
    --- PASS: TestMonitorWatchTransaction/multiple_transactions_mixed (0.01s)
PASS
ok  	github.com/ethersphere/bee/pkg/transaction	(cached)
=== RUN   TestTraversalBytes
=== PAUSE TestTraversalBytes
=== RUN   TestTraversalFiles
=== PAUSE TestTraversalFiles
=== RUN   TestTraversalManifest
=== PAUSE TestTraversalManifest
=== RUN   TestTraversalSOC
=== PAUSE TestTraversalSOC
=== CONT  TestTraversalBytes
=== RUN   TestTraversalBytes/1-chunk-16-bytes
=== PAUSE TestTraversalBytes/1-chunk-16-bytes
=== RUN   TestTraversalBytes/1-chunk-4096-bytes
=== PAUSE TestTraversalBytes/1-chunk-4096-bytes
=== RUN   TestTraversalBytes/2-chunk-4097-bytes
=== CONT  TestTraversalSOC
=== PAUSE TestTraversalBytes/2-chunk-4097-bytes
=== RUN   TestTraversalBytes/128-chunk-524288-bytes
=== PAUSE TestTraversalBytes/128-chunk-524288-bytes
=== RUN   TestTraversalBytes/129-chunk-528384-bytes
=== PAUSE TestTraversalBytes/129-chunk-528384-bytes
=== RUN   TestTraversalBytes/129-chunk-528383-bytes
=== PAUSE TestTraversalBytes/129-chunk-528383-bytes
=== RUN   TestTraversalBytes/130-chunk-528385-bytes
=== PAUSE TestTraversalBytes/130-chunk-528385-bytes
=== CONT  TestTraversalBytes/1-chunk-16-bytes
=== CONT  TestTraversalBytes/129-chunk-528384-bytes
=== CONT  TestTraversalBytes/2-chunk-4097-bytes
=== CONT  TestTraversalBytes/128-chunk-524288-bytes
=== CONT  TestTraversalFiles
=== RUN   TestTraversalFiles/1-chunk-16-bytes
=== PAUSE TestTraversalFiles/1-chunk-16-bytes
=== RUN   TestTraversalFiles/1-chunk-4096-bytes
=== PAUSE TestTraversalFiles/1-chunk-4096-bytes
=== RUN   TestTraversalFiles/2-chunk-4097-bytes
=== PAUSE TestTraversalFiles/2-chunk-4097-bytes
=== CONT  TestTraversalBytes/129-chunk-528383-bytes
=== CONT  TestTraversalBytes/1-chunk-4096-bytes
=== CONT  TestTraversalManifest
=== CONT  TestTraversalBytes/130-chunk-528385-bytes
=== RUN   TestTraversalManifest/bzz-manifest-mantaray-1-files-3-chunks
=== PAUSE TestTraversalManifest/bzz-manifest-mantaray-1-files-3-chunks
=== RUN   TestTraversalManifest/bzz-manifest-mantaray-3-files-8-chunks
=== PAUSE TestTraversalManifest/bzz-manifest-mantaray-3-files-8-chunks
=== CONT  TestTraversalFiles/1-chunk-16-bytes
=== CONT  TestTraversalFiles/2-chunk-4097-bytes
=== CONT  TestTraversalManifest/bzz-manifest-mantaray-1-files-3-chunks
=== CONT  TestTraversalManifest/bzz-manifest-mantaray-3-files-8-chunks
=== CONT  TestTraversalFiles/1-chunk-4096-bytes
--- PASS: TestTraversalFiles (0.00s)
    --- PASS: TestTraversalFiles/1-chunk-16-bytes (0.02s)
    --- PASS: TestTraversalFiles/2-chunk-4097-bytes (0.02s)
    --- PASS: TestTraversalFiles/1-chunk-4096-bytes (0.00s)
--- PASS: TestTraversalSOC (0.03s)
--- PASS: TestTraversalManifest (0.00s)
    --- PASS: TestTraversalManifest/bzz-manifest-mantaray-1-files-3-chunks (0.00s)
    --- PASS: TestTraversalManifest/bzz-manifest-mantaray-3-files-8-chunks (0.00s)
--- PASS: TestTraversalBytes (0.00s)
    --- PASS: TestTraversalBytes/1-chunk-16-bytes (0.00s)
    --- PASS: TestTraversalBytes/2-chunk-4097-bytes (0.01s)
    --- PASS: TestTraversalBytes/1-chunk-4096-bytes (0.02s)
    --- PASS: TestTraversalBytes/129-chunk-528384-bytes (0.03s)
    --- PASS: TestTraversalBytes/128-chunk-524288-bytes (0.03s)
    --- PASS: TestTraversalBytes/130-chunk-528385-bytes (0.03s)
    --- PASS: TestTraversalBytes/129-chunk-528383-bytes (0.03s)
PASS
ok  	github.com/ethersphere/bee/pkg/traversal	(cached)
=== RUN   TestMustParseABI
=== PAUSE TestMustParseABI
=== CONT  TestMustParseABI
--- PASS: TestMustParseABI (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/util/abiutil	(cached)
=== RUN   TestTimeoutReader
=== PAUSE TestTimeoutReader
=== CONT  TestTimeoutReader
=== RUN   TestTimeoutReader/normal_read
=== PAUSE TestTimeoutReader/normal_read
=== RUN   TestTimeoutReader/stuck_read
=== PAUSE TestTimeoutReader/stuck_read
=== RUN   TestTimeoutReader/EOF_read
=== PAUSE TestTimeoutReader/EOF_read
=== CONT  TestTimeoutReader/normal_read
=== CONT  TestTimeoutReader/EOF_read
=== CONT  TestTimeoutReader/stuck_read
--- PASS: TestTimeoutReader (0.00s)
    --- PASS: TestTimeoutReader/normal_read (0.00s)
    --- PASS: TestTimeoutReader/EOF_read (0.00s)
    --- PASS: TestTimeoutReader/stuck_read (0.20s)
PASS
ok  	github.com/ethersphere/bee/pkg/util/ioutil	(cached)
=== RUN   TestRandBytes
=== PAUSE TestRandBytes
=== RUN   TestCleanupCloser
=== PAUSE TestCleanupCloser
=== CONT  TestRandBytes
--- PASS: TestRandBytes (0.00s)
=== CONT  TestCleanupCloser
--- PASS: TestCleanupCloser (0.00s)
PASS
ok  	github.com/ethersphere/bee/pkg/util/testutil	(cached)
