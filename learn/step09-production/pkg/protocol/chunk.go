// Package protocol holds wire types shared by the worker, controller and client.
package protocol

// HTTP headers used by the worker chunk API.
const (
	// HeaderChunkChecksum carries the SHA-256 hex digest of a chunk.
	// On GET it is set by the worker; on PUT the caller may send it so the
	// worker rejects the upload if the bytes don't match.
	HeaderChunkChecksum = "X-Chunk-Checksum"
	// HeaderChunkSize carries the chunk size in bytes on GET responses.
	HeaderChunkSize = "X-Chunk-Size"
)

// ChunkInfo describes one stored chunk.
type ChunkInfo struct {
	ID       string `json:"chunk_id"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"` // SHA-256 hex digest
}

// ChunkList is the response body of GET /internal/workers/{worker_id}/chunks.
type ChunkList struct {
	WorkerID string      `json:"worker_id"`
	Count    int         `json:"count"`
	Chunks   []ChunkInfo `json:"chunks"`
}

// ErrorResponse is the JSON body of every error response.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody holds a machine-readable code and a human-readable message.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes returned by the worker API.
const (
	CodeInvalidChunkID   = "invalid_chunk_id"
	CodeChunkNotFound    = "chunk_not_found"
	CodeChunkTooLarge    = "chunk_too_large"
	CodeChecksumMismatch = "checksum_mismatch"
	CodeChunkCorrupted   = "chunk_corrupted"
	CodeInvalidRequest   = "invalid_request"
	CodeStorageError     = "storage_error"
	CodeInternalError    = "internal_error"
	CodeNotFound         = "not_found"
)
