// Package api implements the worker's internal HTTP API.
package api

import (
	"net/http"

	"github.com/Sahas-out/DistributedFileStorage/learn/step08-tests/worker/internal/storage"
)

// Server serves the worker chunk API on top of a Storage.
// Its fields are lowercase, so code outside package api can't touch them;
// other packages must go through NewServer.
type Server struct {
	store    storage.Storage
	workerID string
	maxChunk int64
}

// NewServer builds a Server. maxChunk is the largest accepted chunk body in
// bytes.
func NewServer(store storage.Storage, workerID string, maxChunk int64) *Server {
	return &Server{store: store, workerID: workerID, maxChunk: maxChunk}
}

// Handler returns the HTTP handler with all routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /internal/chunks/{chunk_id}", s.handlePutChunk)
	mux.HandleFunc("GET /internal/chunks/{chunk_id}", s.handleGetChunk)
	mux.HandleFunc("DELETE /internal/chunks/{chunk_id}", s.handleDeleteChunk)
	mux.HandleFunc("GET /internal/workers/{worker_id}/chunks", s.handleListChunks)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	return mux
}
