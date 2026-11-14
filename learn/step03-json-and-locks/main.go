// Step 3: JSON responses using structs, a list endpoint, and a lock that
// makes the shared map safe for concurrent requests.
//
// Run it:   go run ./learn/step03-json-and-locks
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
)

const workerID = "worker1"

// ChunkInfo describes one stored chunk. A struct is a plain data type with
// named fields, like a Java record or a C# class with only properties.
//
// The `json:"..."` parts are struct tags: they tell encoding/json which JSON
// key to use for each field (like @JsonProperty in Jackson).
// Fields must start with a capital letter or encoding/json can't see them.
type ChunkInfo struct {
	ID   string `json:"chunk_id"`
	Size int64  `json:"size"`
}

// ChunkList is the response body of the list endpoint.
type ChunkList struct {
	WorkerID string      `json:"worker_id"`
	Count    int         `json:"count"`
	Chunks   []ChunkInfo `json:"chunks"`
}

// mu protects chunks. Every read or write of the map must hold the lock.
var (
	mu     sync.Mutex
	chunks = map[string][]byte{}
)

func main() {
	http.HandleFunc("GET /healthz", handleHealth)
	http.HandleFunc("PUT /internal/chunks/{chunk_id}", handlePutChunk)
	http.HandleFunc("GET /internal/chunks/{chunk_id}", handleGetChunk)
	http.HandleFunc("DELETE /internal/chunks/{chunk_id}", handleDeleteChunk)
	http.HandleFunc("GET /internal/workers/{worker_id}/chunks", handleListChunks)

	log.Println("listening on http://localhost:8093")
	log.Fatal(http.ListenAndServe(":8093", nil))
}

// writeJSON sends any value as JSON. `any` means "a value of any type"
// (like Object in Java).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	// A map literal is a quick way to build small JSON objects.
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "worker_id": workerID})
}

func handlePutChunk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chunk_id")
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "could not read request body", http.StatusBadRequest)
		return
	}

	mu.Lock()
	chunks[id] = data
	mu.Unlock()

	// Build a struct value with field names, then send it as JSON.
	info := ChunkInfo{ID: id, Size: int64(len(data))}
	writeJSON(w, http.StatusCreated, info)
}

func handleGetChunk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chunk_id")

	mu.Lock()
	data, ok := chunks[id]
	mu.Unlock()

	if !ok {
		http.Error(w, "chunk not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

func handleDeleteChunk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chunk_id")

	mu.Lock()
	delete(chunks, id)
	mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// handleListChunks returns every stored chunk, sorted by ID.
// The {worker_id} in the path is ignored for now.
func handleListChunks(w http.ResponseWriter, r *http.Request) {
	list := collectChunks()
	writeJSON(w, http.StatusOK, ChunkList{WorkerID: workerID, Count: len(list), Chunks: list})
}

// collectChunks copies the map into a sorted slice while holding the lock.
func collectChunks() []ChunkInfo {
	mu.Lock()
	// defer runs mu.Unlock() when this function returns, however it returns.
	// It's Go's version of try/finally.
	defer mu.Unlock()

	// make creates an empty slice with room for len(chunks) items.
	// Starting from an empty (non-nil) slice means the JSON is [] and not null.
	list := make([]ChunkInfo, 0, len(chunks))

	// for ... range loops over a map, giving key and value each time.
	for id, data := range chunks {
		list = append(list, ChunkInfo{ID: id, Size: int64(len(data))})
	}

	// Map iteration order is random in Go, so sort for a stable response.
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}
