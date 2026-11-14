// Step 6: separate "how chunks are stored" from "how HTTP works".
//
//   - Storage is an INTERFACE: what any chunk store must be able to do.
//   - Memory is one implementation (Person 1's Filesystem will be another).
//   - Server holds its dependencies in a struct instead of global variables,
//     and its handlers are METHODS on that struct.
//
// Still one file; step 7 splits it into packages.
//
// Run it:   go run ./learn/step06-storage-interface
package main

import (
	"bytes"
	"context"
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

const maxChunkBytes = 1 << 20

const (
	HeaderChunkChecksum = "X-Chunk-Checksum"
	HeaderChunkSize     = "X-Chunk-Size"
)

const (
	CodeInvalidChunkID   = "invalid_chunk_id"
	CodeChunkNotFound    = "chunk_not_found"
	CodeChunkTooLarge    = "chunk_too_large"
	CodeChecksumMismatch = "checksum_mismatch"
	CodeChunkCorrupted   = "chunk_corrupted"
	CodeInvalidRequest   = "invalid_request"
	CodeStorageError     = "storage_error"
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

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// =====================================================================
// STORAGE: the contract, plus one in-memory implementation
// =====================================================================

// Sentinel errors: fixed error values that callers can test for with
// errors.Is, the way Java code catches a specific exception type.
var (
	ErrNotFound        = errors.New("chunk not found")
	ErrInvalidChunkID  = errors.New("invalid chunk id")
	ErrMissingChecksum = errors.New("chunk checksum metadata missing")
)

// Storage is what the HTTP layer needs from a chunk store. Any type that has
// these four methods IS a Storage automatically: no "implements" keyword.
type Storage interface {
	Put(ctx context.Context, chunkID string, src io.Reader) (ChunkInfo, error)
	Open(ctx context.Context, chunkID string) (io.ReadCloser, ChunkInfo, error)
	Delete(ctx context.Context, chunkID string) error
	List(ctx context.Context) ([]ChunkInfo, error)
}

var chunkIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ValidChunkID is shared by the HTTP layer and every Storage implementation.
func ValidChunkID(id string) bool { return chunkIDPattern.MatchString(id) }

// Memory keeps chunks in a map. The lock now lives INSIDE the struct, next
// to the data it protects, instead of being a global.
type Memory struct {
	mu     sync.RWMutex // many readers at once, or one writer
	chunks map[string]memChunk
}

type memChunk struct {
	data []byte
	info ChunkInfo
}

// NewMemory is a constructor. Go has no `new Memory()` with constructor
// logic, so by convention a NewXxx function builds and returns one.
// It returns *Memory: a POINTER, so everyone shares the same store.
func NewMemory() *Memory {
	return &Memory{chunks: make(map[string]memChunk)}
}

// This line makes the compiler check that *Memory satisfies Storage.
// If a method is missing, the build fails here with a clear message.
var _ Storage = (*Memory)(nil)

// (m *Memory) is the RECEIVER: it makes Put a method on *Memory,
// and m plays the role of `this`.
func (m *Memory) Put(ctx context.Context, chunkID string, src io.Reader) (ChunkInfo, error) {
	if !ValidChunkID(chunkID) {
		return ChunkInfo{}, ErrInvalidChunkID
	}
	data, err := io.ReadAll(src)
	if err != nil {
		return ChunkInfo{}, err
	}
	// ctx.Err() is non-nil if the request was cancelled (client went away).
	if err := ctx.Err(); err != nil {
		return ChunkInfo{}, err
	}
	sum := sha256.Sum256(data)
	info := ChunkInfo{ID: chunkID, Size: int64(len(data)), Checksum: hex.EncodeToString(sum[:])}

	m.mu.Lock()
	m.chunks[chunkID] = memChunk{data: data, info: info}
	m.mu.Unlock()
	return info, nil
}

func (m *Memory) Open(_ context.Context, chunkID string) (io.ReadCloser, ChunkInfo, error) {
	m.mu.RLock()
	c, ok := m.chunks[chunkID]
	m.mu.RUnlock()
	if !ok {
		return nil, ChunkInfo{}, ErrNotFound
	}
	// Wrap the bytes in a reader. NopCloser adds a Close() that does nothing,
	// because a file-based store will need Close() but memory doesn't.
	return io.NopCloser(bytes.NewReader(c.data)), c.info, nil
}

func (m *Memory) Delete(_ context.Context, chunkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.chunks[chunkID]; !ok {
		return ErrNotFound
	}
	delete(m.chunks, chunkID)
	return nil
}

func (m *Memory) List(_ context.Context) ([]ChunkInfo, error) {
	m.mu.RLock()
	out := make([]ChunkInfo, 0, len(m.chunks))
	for _, c := range m.chunks {
		out = append(out, c.info)
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Corrupt is a LEARNING AID that only Memory has; it is not part of Storage.
func (m *Memory) Corrupt(chunkID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.chunks[chunkID]
	if !ok || len(c.data) == 0 {
		return false
	}
	c.data[0] ^= 0xFF
	return true
}

// =====================================================================
// HTTP: a Server that only knows about the Storage interface
// =====================================================================

// Server holds everything the handlers need. Compare with Spring:
// these fields are what you'd inject through the constructor.
type Server struct {
	store    Storage
	workerID string
	maxChunk int64
}

func NewServer(store Storage, workerID string, maxChunk int64) *Server {
	return &Server{store: store, workerID: workerID, maxChunk: maxChunk}
}

// Handler builds a router (ServeMux) with all routes. Using our own mux
// instead of the global default one means no hidden global state.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// s.handlePutChunk is a "method value": the method bound to s.
	mux.HandleFunc("PUT /internal/chunks/{chunk_id}", s.handlePutChunk)
	mux.HandleFunc("GET /internal/chunks/{chunk_id}", s.handleGetChunk)
	mux.HandleFunc("DELETE /internal/chunks/{chunk_id}", s.handleDeleteChunk)
	mux.HandleFunc("GET /internal/workers/{worker_id}/chunks", s.handleListChunks)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	return mux
}

func (s *Server) handlePutChunk(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxChunk))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, CodeChunkTooLarge,
				"chunk exceeds maximum size of "+strconv.FormatInt(s.maxChunk, 10)+" bytes")
			return
		}
		writeError(w, http.StatusBadRequest, CodeInvalidRequest, "failed to read request body")
		return
	}

	id, ok := s.chunkID(w, r)
	if !ok {
		return
	}

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

	// r.Context() is cancelled if the client disconnects; pass it down so
	// the storage can stop early.
	info, err := s.store.Put(r.Context(), id, bytes.NewReader(data))
	if err != nil {
		s.storageError(w, id, err)
		return
	}
	// Defence in depth: the store must have recorded what we received.
	if !digestsEqual(info.Checksum, sum) || info.Size != int64(len(data)) {
		log.Printf("storage recorded different chunk info than received: chunk_id=%s", id)
		writeError(w, http.StatusInternalServerError, CodeStorageError, "storage recorded inconsistent chunk info")
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

func (s *Server) handleGetChunk(w http.ResponseWriter, r *http.Request) {
	id, ok := s.chunkID(w, r)
	if !ok {
		return
	}

	rc, info, err := s.store.Open(r.Context(), id)
	if err != nil {
		s.storageError(w, id, err)
		return
	}
	defer rc.Close() // whatever happens next, close the reader

	data, err := io.ReadAll(rc)
	if err != nil {
		s.storageError(w, id, err)
		return
	}

	actual := sha256Hex(data)
	if info.Checksum == "" || !digestsEqual(actual, info.Checksum) || int64(len(data)) != info.Size {
		log.Printf("chunk failed integrity check: chunk_id=%s recorded=%s actual=%s", id, info.Checksum, actual)
		writeError(w, http.StatusInternalServerError, CodeChunkCorrupted, "chunk "+id+" failed integrity check")
		return
	}

	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set(HeaderChunkChecksum, info.Checksum)
	h.Set(HeaderChunkSize, strconv.FormatInt(info.Size, 10))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		w.Write(data)
	}
}

func (s *Server) handleDeleteChunk(w http.ResponseWriter, r *http.Request) {
	id, ok := s.chunkID(w, r)
	if !ok {
		return
	}
	// "Not found" is fine for DELETE (idempotent); any other error is not.
	if err := s.store.Delete(r.Context(), id); err != nil && !errors.Is(err, ErrNotFound) {
		s.storageError(w, id, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListChunks(w http.ResponseWriter, r *http.Request) {
	chunks, err := s.store.List(r.Context())
	if err != nil {
		s.storageError(w, "", err)
		return
	}
	if chunks == nil { // a Storage may return nil for "none"; send [] not null
		chunks = []ChunkInfo{}
	}
	writeJSON(w, http.StatusOK, ChunkList{WorkerID: s.workerID, Count: len(chunks), Chunks: chunks})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "worker_id": s.workerID})
}

func (s *Server) chunkID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("chunk_id")
	if !ValidChunkID(id) {
		writeError(w, http.StatusBadRequest, CodeInvalidChunkID,
			"chunk id must be 1-128 characters from [A-Za-z0-9_-]")
		return "", false
	}
	return id, true
}

// storageError is the ONE place that turns storage errors into HTTP replies.
// errors.Is also matches errors that WRAP the sentinel, e.g.
// fmt.Errorf("open %s: %w", path, ErrNotFound).
func (s *Server) storageError(w http.ResponseWriter, id string, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, CodeChunkNotFound, "chunk "+id+" not found")
	case errors.Is(err, ErrInvalidChunkID):
		writeError(w, http.StatusBadRequest, CodeInvalidChunkID, "invalid chunk id")
	default:
		// Log the details for us; tell the client only "storage failure".
		log.Printf("storage failure: chunk_id=%s error=%v", id, err)
		writeError(w, http.StatusInternalServerError, CodeStorageError, "storage failure")
	}
}

// ---------- helpers (unchanged from step 5) ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
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

// =====================================================================
// main: build the pieces and connect them ("wiring")
// =====================================================================

func main() {
	store := NewMemory() // type *Memory
	srv := NewServer(store, "worker1", maxChunkBytes)

	// The Server only sees a Storage. Here in main we still know the store
	// is a *Memory, so we can add the learning-aid route that needs it.
	mux := http.NewServeMux()
	mux.Handle("/", srv.Handler()) // everything else goes to the Server
	mux.HandleFunc("POST /debug/corrupt/{chunk_id}", func(w http.ResponseWriter, r *http.Request) {
		if !store.Corrupt(r.PathValue("chunk_id")) {
			writeError(w, http.StatusNotFound, CodeChunkNotFound, "chunk not found or empty")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	log.Println("listening on http://localhost:8096")
	log.Fatal(http.ListenAndServe(":8096", mux))
}
