// Step 5: validate input, enforce a size limit, verify an optional checksum
// sent by the client, and return every error as JSON with a stable code.
//
// Run it:   go run ./learn/step05-validation-and-errors
package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	workerID      = "worker1"
	maxChunkBytes = 1 << 20 // 1 MiB = 1,048,576 bytes ("1 shifted left 20 times")
)

const (
	HeaderChunkChecksum = "X-Chunk-Checksum"
	HeaderChunkSize     = "X-Chunk-Size"
)

// Error codes: short, stable strings a program (the controller) can check,
// unlike the human-readable message which may change.
const (
	CodeInvalidChunkID   = "invalid_chunk_id"
	CodeChunkNotFound    = "chunk_not_found"
	CodeChunkTooLarge    = "chunk_too_large"
	CodeChecksumMismatch = "checksum_mismatch"
	CodeChunkCorrupted   = "chunk_corrupted"
	CodeInvalidRequest   = "invalid_request"
)

type ChunkInfo struct {
	ID       string `json:"chunk_id"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

type ChunkList struct {
	WorkerID string      `json:"worker_id"`
	Count    int         `json:"count"`
	Chunks   []ChunkInfo `json:"chunks"`
}

// ErrorResponse produces {"error":{"code":"...","message":"..."}}.
// Structs can be nested: ErrorResponse contains an ErrorBody.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type storedChunk struct {
	data []byte
	info ChunkInfo
}

var (
	mu     sync.Mutex
	chunks = map[string]storedChunk{}
)

// Compiled once at startup. MustCompile panics if the pattern is invalid,
// which is fine for a constant pattern: the program fails immediately.
// Only letters, digits, _ and - are allowed, so an ID can never contain
// "/" or "..", and later can't escape the storage directory on disk.
var chunkIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func main() {
	http.HandleFunc("GET /healthz", handleHealth)
	http.HandleFunc("PUT /internal/chunks/{chunk_id}", handlePutChunk)
	http.HandleFunc("GET /internal/chunks/{chunk_id}", handleGetChunk)
	http.HandleFunc("DELETE /internal/chunks/{chunk_id}", handleDeleteChunk)
	http.HandleFunc("GET /internal/workers/{worker_id}/chunks", handleListChunks)
	http.HandleFunc("POST /debug/corrupt/{chunk_id}", handleCorrupt) // learning aid

	log.Println("listening on http://localhost:8095")
	log.Fatal(http.ListenAndServe(":8095", nil))
}

// ---------- response helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError is the ONE way errors are sent, so they all look the same.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

// ---------- small helpers ----------

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// validHexDigest: is s 64 hex characters (a SHA-256 digest)?
func validHexDigest(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s) // _ discards the decoded bytes; we only want err
	return err == nil
}

// digestsEqual compares two hex digests, ignoring upper/lower case.
// ConstantTimeCompare takes the same time whether the strings differ in the
// first or last character, a good habit when comparing hashes.
func digestsEqual(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// chunkID reads and validates the ID. It returns TWO values: the ID and
// whether it was valid. If invalid, it has already written the 400 reply.
func chunkID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("chunk_id")
	if !chunkIDPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, CodeInvalidChunkID,
			"chunk id must be 1-128 characters from [A-Za-z0-9_-]")
		return "", false
	}
	return id, true
}

// ---------- handlers ----------

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "worker_id": workerID})
}

func handlePutChunk(w http.ResponseWriter, r *http.Request) {
	// 1. Read the body FIRST, with a size cap. (Why first? See the README:
	//    replying early with unread body bytes can lose the reply.)
	//    MaxBytesReader stops reading after maxChunkBytes and returns an error.
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxChunkBytes))
	if err != nil {
		// errors.As checks whether err is (or wraps) a *http.MaxBytesError,
		// like `catch (MaxBytesException e)` but for returned errors.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, CodeChunkTooLarge, // 413
				"chunk exceeds maximum size of "+strconv.Itoa(maxChunkBytes)+" bytes")
			return
		}
		writeError(w, http.StatusBadRequest, CodeInvalidRequest, "failed to read request body")
		return
	}

	// 2. Validate the ID.
	id, ok := chunkID(w, r)
	if !ok {
		return
	}

	// 3. Optional end-to-end check: if the client says what the checksum
	//    should be, refuse to store bytes that were damaged on the way.
	expected := strings.TrimSpace(r.Header.Get(HeaderChunkChecksum))
	if expected != "" && !validHexDigest(expected) {
		writeError(w, http.StatusBadRequest, CodeInvalidRequest,
			HeaderChunkChecksum+" must be a 64-character SHA-256 hex digest")
		return
	}
	sum := sha256Hex(data)
	if expected != "" && !digestsEqual(expected, sum) {
		writeError(w, http.StatusBadRequest, CodeChecksumMismatch,
			"request body checksum "+sum+" does not match "+HeaderChunkChecksum)
		return
	}

	// 4. Store.
	info := ChunkInfo{ID: id, Size: int64(len(data)), Checksum: sum}
	mu.Lock()
	chunks[id] = storedChunk{data: data, info: info}
	mu.Unlock()

	writeJSON(w, http.StatusCreated, info)
}

func handleGetChunk(w http.ResponseWriter, r *http.Request) {
	id, ok := chunkID(w, r)
	if !ok {
		return
	}

	mu.Lock()
	c, found := chunks[id]
	mu.Unlock()
	if !found {
		writeError(w, http.StatusNotFound, CodeChunkNotFound, "chunk "+id+" not found")
		return
	}

	actual := sha256Hex(c.data)
	if c.info.Checksum == "" || !digestsEqual(actual, c.info.Checksum) || int64(len(c.data)) != c.info.Size {
		log.Printf("chunk %s failed integrity check: recorded=%s actual=%s", id, c.info.Checksum, actual)
		writeError(w, http.StatusInternalServerError, CodeChunkCorrupted, "chunk "+id+" failed integrity check")
		return
	}

	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Length", strconv.Itoa(len(c.data)))
	h.Set(HeaderChunkChecksum, c.info.Checksum)
	h.Set(HeaderChunkSize, strconv.FormatInt(c.info.Size, 10))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		w.Write(c.data)
	}
}

func handleDeleteChunk(w http.ResponseWriter, r *http.Request) {
	id, ok := chunkID(w, r)
	if !ok {
		return
	}
	mu.Lock()
	delete(chunks, id)
	mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func handleListChunks(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	list := make([]ChunkInfo, 0, len(chunks))
	for _, c := range chunks {
		list = append(list, c.info)
	}
	mu.Unlock()

	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	writeJSON(w, http.StatusOK, ChunkList{WorkerID: workerID, Count: len(list), Chunks: list})
}

func handleCorrupt(w http.ResponseWriter, r *http.Request) {
	id, ok := chunkID(w, r)
	if !ok {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	c, found := chunks[id]
	if !found || len(c.data) == 0 {
		writeError(w, http.StatusNotFound, CodeChunkNotFound, "chunk "+id+" not found or empty")
		return
	}
	c.data[0] ^= 0xFF
	w.WriteHeader(http.StatusNoContent)
}
