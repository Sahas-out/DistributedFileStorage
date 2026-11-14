package api

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Sahas-out/DistributedFileStorage/learn/step09-production/pkg/protocol"
	"github.com/Sahas-out/DistributedFileStorage/learn/step09-production/worker/internal/storage"
)

// handlePutChunk stores the raw request body as a chunk.
//
// The body (at most maxChunk bytes) is buffered and hashed before anything is
// written, so a bad X-Chunk-Checksum is rejected without touching storage.
//
// The body is read before any validation on purpose: replying early while up
// to 1 MiB of request body is still unread makes the server close the
// connection, and the resulting TCP reset can destroy the error response
// before the client reads it (observed through Docker's port proxy).
func (s *Server) handlePutChunk(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxChunk))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, protocol.CodeChunkTooLarge,
				"chunk exceeds maximum size of "+strconv.FormatInt(s.maxChunk, 10)+" bytes")
			return
		}
		writeError(w, http.StatusBadRequest, protocol.CodeInvalidRequest, "failed to read request body")
		return
	}

	id, ok := s.chunkID(w, r)
	if !ok {
		return
	}

	expected := strings.TrimSpace(r.Header.Get(protocol.HeaderChunkChecksum))
	if expected != "" && !validHexDigest(expected) {
		writeError(w, http.StatusBadRequest, protocol.CodeInvalidRequest,
			protocol.HeaderChunkChecksum+" must be a 64-character SHA-256 hex digest")
		return
	}

	sum := sha256Hex(data)
	if expected != "" && !digestsEqual(expected, sum) {
		writeError(w, http.StatusBadRequest, protocol.CodeChecksumMismatch,
			"request body checksum "+sum+" does not match "+protocol.HeaderChunkChecksum)
		return
	}

	info, err := s.store.Put(r.Context(), id, bytes.NewReader(data))
	if err != nil {
		s.storageError(w, r, id, err)
		return
	}
	if !digestsEqual(info.Checksum, sum) || info.Size != int64(len(data)) {
		s.log.Error("storage recorded different chunk info than received",
			"chunk_id", id, "want_checksum", sum, "got_checksum", info.Checksum,
			"want_size", len(data), "got_size", info.Size)
		writeError(w, http.StatusInternalServerError, protocol.CodeStorageError, "storage recorded inconsistent chunk info")
		return
	}

	writeJSON(w, http.StatusCreated, info)
}

// handleGetChunk returns the raw chunk bytes after verifying them against the
// recorded checksum. Corrupted chunks are reported as 500 chunk_corrupted so
// the controller can fall back to another replica.
func (s *Server) handleGetChunk(w http.ResponseWriter, r *http.Request) {
	id, ok := s.chunkID(w, r)
	if !ok {
		return
	}

	rc, info, err := s.store.Open(r.Context(), id)
	if err != nil {
		s.storageError(w, r, id, err)
		return
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		s.storageError(w, r, id, err)
		return
	}

	actual := sha256Hex(data)
	if info.Checksum == "" || !digestsEqual(actual, info.Checksum) || int64(len(data)) != info.Size {
		s.log.Warn("chunk failed integrity check",
			"chunk_id", id, "recorded_checksum", info.Checksum, "actual_checksum", actual,
			"recorded_size", info.Size, "actual_size", len(data))
		writeError(w, http.StatusInternalServerError, protocol.CodeChunkCorrupted,
			"chunk "+id+" failed integrity check")
		return
	}

	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set(protocol.HeaderChunkChecksum, info.Checksum)
	h.Set(protocol.HeaderChunkSize, strconv.FormatInt(info.Size, 10))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

// handleDeleteChunk removes a chunk. Deleting a missing chunk also returns 204.
func (s *Server) handleDeleteChunk(w http.ResponseWriter, r *http.Request) {
	id, ok := s.chunkID(w, r)
	if !ok {
		return
	}
	if err := s.store.Delete(r.Context(), id); err != nil && !errors.Is(err, storage.ErrNotFound) {
		s.storageError(w, r, id, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListChunks returns the worker's chunk inventory. The {worker_id} path
// value is currently ignored; the response reports the configured worker ID.
func (s *Server) handleListChunks(w http.ResponseWriter, r *http.Request) {
	chunks, err := s.store.List(r.Context())
	if err != nil {
		s.storageError(w, r, "", err)
		return
	}
	if chunks == nil {
		chunks = []protocol.ChunkInfo{}
	}
	writeJSON(w, http.StatusOK, protocol.ChunkList{
		WorkerID: s.workerID,
		Count:    len(chunks),
		Chunks:   chunks,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "worker_id": s.workerID})
}

// chunkID extracts and validates the chunk ID, writing a 400 if invalid.
func (s *Server) chunkID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("chunk_id")
	if !storage.ValidChunkID(id) {
		writeError(w, http.StatusBadRequest, protocol.CodeInvalidChunkID,
			"chunk id must be 1-128 characters from [A-Za-z0-9_-]")
		return "", false
	}
	return id, true
}

// storageError maps storage errors to HTTP responses. Unexpected errors are
// logged and reported without internal details.
func (s *Server) storageError(w http.ResponseWriter, r *http.Request, id string, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, http.StatusNotFound, protocol.CodeChunkNotFound, "chunk "+id+" not found")
	case errors.Is(err, storage.ErrInvalidChunkID):
		writeError(w, http.StatusBadRequest, protocol.CodeInvalidChunkID, "invalid chunk id")
	default:
		s.log.Error("storage failure", "method", r.Method, "chunk_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, protocol.CodeStorageError, "storage failure")
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func validHexDigest(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func digestsEqual(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
