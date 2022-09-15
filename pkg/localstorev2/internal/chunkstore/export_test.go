package chunkstore

type RetrievalIndexItem = retrievalIndexItem

type ChunkStampItem = chunkStampItem

var (
	ErrInvalidRetrievalIndexItemAddress = errInvalidRetrievalIndexAddress
	ErrInvalidRetrievalIndexItemSize    = errInvalidRetrievalIndexSize

	ErrMarshalInvalidChunkStampItemAddress   = errMarshalInvalidChunkStampItemAddress
	ErrUnmarshalInvalidChunkStampItemAddress = errUnmarshalInvalidChunkStampItemAddress
	ErrMarshalInvalidChunkStampItemStamp     = errMarshalInvalidChunkStampItemStamp
	ErrUnmarshalInvalidChunkStampItemStamp   = errUnmarshalInvalidChunkStampItemStamp
	ErrInvalidChunkStampItemSize             = errInvalidChunkStampItemSize
)
