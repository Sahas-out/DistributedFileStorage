// Step 4: SHA-256 checksums. The worker records a checksum when a chunk is
// stored and checks it on every read, so corrupted data is never returned.
//
// Run it:   go run ./learn/step04-checksums
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
)

const workerID = "worker1"

// Header names, kept as constants so they can't be misspelt.
const (
	HeaderChunkChecksum = "X-Chunk-Checksum"
	HeaderChunkSize     = "X-Chunk-Size"
)

type ChunkInfo struct {
	ID       string `json:"chunk_id"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"` // SHA-256 hex digest
}

type ChunkList struct {
	WorkerID string      `json:"worker_id"`
	Count    int         `json:"count"`
	Chunks   []ChunkInfo `json:"chunks"`
}

// storedChunk keeps the bytes together with the info recorded when they were
// written. The lowercase name means it's private to this package.
type storedChunk struct {
	data []byte
	info ChunkInfo
}

var (
	mu     sync.Mutex
	chunks = map[string]storedChunk{}
)

func main() {
	http.HandleFunc("GET /healthz", handleHealth)
	http.HandleFunc("PUT /internal/chunks/{chunk_id}", handlePutChunk)
	http.HandleFunc("GET /internal/chunks/{chunk_id}", handleGetChunk)
	http.HandleFunc("DELETE /internal/chunks/{chunk_id}", handleDeleteChunk)
	http.HandleFunc("GET /internal/workers/{worker_id}/chunks", handleListChunks)

	// LEARNING AID ONLY (not in the real worker): flips one byte of a stored
	// chunk so you can watch the integrity check catch it.
	http.HandleFunc("POST /debug/corrupt/{chunk_id}", handleCorrupt)

	log.Println("listening on http://localhost:8094")
	log.Fatal(http.ListenAndServe(":8094", nil))
}

// sha256Hex returns the SHA-256 of b as a 64-character lowercase hex string.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b) // sum is an ARRAY: [32]byte
	// sum[:] turns the array into a slice, which is what EncodeToString takes.
	return hex.EncodeToString(sum[:])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "worker_id": workerID})
}

func handlePutChunk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chunk_id")
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "could not read request body", http.StatusBadRequest)
		return
	}

	info := ChunkInfo{ID: id, Size: int64(len(data)), Checksum: sha256Hex(data)}

	mu.Lock()
	chunks[id] = storedChunk{data: data, info: info}
	mu.Unlock()

	writeJSON(w, http.StatusCreated, info)
}

func handleGetChunk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chunk_id")

	mu.Lock()
	c, ok := chunks[id]
	mu.Unlock()

	if !ok {
		http.Error(w, "chunk not found", http.StatusNotFound)
		return
	}

	// Integrity check: hash the bytes we're about to send and compare with
	// the checksum recorded at write time. If they differ, the stored data
	// changed (disk corruption, someone editing the file...). Refuse to send
	// it, so the controller can fetch the other replica instead.
	if sha256Hex(c.data) != c.info.Checksum {
		log.Printf("chunk %s failed integrity check", id)
		http.Error(w, "chunk "+id+" failed integrity check", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Length", strconv.Itoa(len(c.data)))
	h.Set(HeaderChunkChecksum, c.info.Checksum)
	h.Set(HeaderChunkSize, strconv.FormatInt(c.info.Size, 10))
	w.WriteHeader(http.StatusOK)

	// Go routes HEAD requests to GET handlers. HEAD means "headers only",
	// so only write the body for real GETs.
	if r.Method != http.MethodHead {
		w.Write(c.data)
	}
}

func handleDeleteChunk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chunk_id")
	mu.Lock()
	delete(chunks, id)
	mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func handleListChunks(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	list := make([]ChunkInfo, 0, len(chunks))
	for _, c := range chunks { // _ ignores the key; we only need the value
		list = append(list, c.info)
	}
	mu.Unlock()

	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	writeJSON(w, http.StatusOK, ChunkList{WorkerID: workerID, Count: len(list), Chunks: list})
}

// handleCorrupt flips the bits of the first byte, without updating the
// recorded checksum: exactly what silent disk corruption looks like.
func handleCorrupt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chunk_id")
	mu.Lock()
	defer mu.Unlock()

	c, ok := chunks[id]
	if !ok || len(c.data) == 0 {
		http.Error(w, "chunk not found or empty", http.StatusNotFound)
		return
	}
	// c is a COPY of the struct, but c.data is a slice, and copying a slice
	// copies a reference to the same bytes. So this changes the stored chunk.
	c.data[0] ^= 0xFF // ^ is XOR: flips all 8 bits of the first byte
	w.WriteHeader(http.StatusNoContent)
}
