package api

import (
	"encoding/json"
	"net/http"

	"github.com/Sahas-out/DistributedFileStorage/pkg/protocol"
)

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes the standard JSON error body.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, protocol.ErrorResponse{
		Error: protocol.ErrorBody{Code: code, Message: message},
	})
}
