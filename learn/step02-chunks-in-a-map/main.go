// Step 2: store chunks in memory with PUT, GET and DELETE.
//
// Run it:   go run ./learn/step02-chunks-in-a-map
//
// WARNING: this version has a concurrency bug on purpose (see the README).
// Step 3 fixes it.
package main

import (
	"io"
	"log"
	"net/http"
)

// chunks is a package-level variable: one map shared by every request.
// map[string][]byte is Go for HashMap<String, byte[]>.
// The {} at the end creates an empty, ready-to-use map.
var chunks = map[string][]byte{}

func main() {
	// Instead of anonymous functions, we now pass named functions.
	// Note: the function name is passed without (), we don't call it here.
	http.HandleFunc("GET /healthz", handleHealth)
	http.HandleFunc("PUT /internal/chunks/{chunk_id}", handlePutChunk)
	http.HandleFunc("GET /internal/chunks/{chunk_id}", handleGetChunk)
	http.HandleFunc("DELETE /internal/chunks/{chunk_id}", handleDeleteChunk)

	log.Println("listening on http://localhost:8092")
	log.Fatal(http.ListenAndServe(":8092", nil))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// handlePutChunk stores the request body under the chunk ID from the URL.
func handlePutChunk(w http.ResponseWriter, r *http.Request) {
	// {chunk_id} in the route pattern is a wildcard; PathValue reads it.
	id := r.PathValue("chunk_id")

	// io.ReadAll reads the whole body into memory. Like many Go functions it
	// returns TWO values: the result and an error. If err is not nil,
	// something went wrong and data should not be used.
	data, err := io.ReadAll(r.Body)
	if err != nil {
		// http.Error writes a plain-text error with the given status code.
		http.Error(w, "could not read request body", http.StatusBadRequest) // 400
		return
	}

	chunks[id] = data
	w.WriteHeader(http.StatusCreated) // 201
}

// handleGetChunk returns the stored bytes, or 404 if the chunk is unknown.
func handleGetChunk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chunk_id")

	// The "comma ok" idiom: ok is false if the key is not in the map.
	// (Reading a missing key without ok gives the zero value, here nil.)
	data, ok := chunks[id]
	if !ok {
		http.Error(w, "chunk not found", http.StatusNotFound) // 404
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream") // "raw bytes"
	w.Write(data)
}

// handleDeleteChunk removes a chunk. Deleting a missing key is a no-op, so
// this always returns 204, which is the behaviour the worker wants.
func handleDeleteChunk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chunk_id")
	delete(chunks, id) // built-in function, like map.remove(id)
	w.WriteHeader(http.StatusNoContent) // 204
}
