// Package storage defines the chunk storage contract used by the worker.
//
// This file is the agreed interface between Person 1 (storage implementation)
// and Person 2 (HTTP API). Change it only by agreement.
package storage

import (
	"context"
	"errors"
	"io"
	"regexp"

	"github.com/Sahas-out/DistributedFileStorage/pkg/protocol"
)

// ChunkInfo is an alias of the shared wire type so there is a single definition.
type ChunkInfo = protocol.ChunkInfo

// MaxChunkIDLength is the longest accepted chunk ID.
const MaxChunkIDLength = 128

var chunkIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ValidChunkID reports whether id is an acceptable chunk ID: 1-128 characters
// from [A-Za-z0-9_-]. Such IDs contain no path separators or dots, so they
// cannot escape the storage directory.
func ValidChunkID(id string) bool {
	return chunkIDPattern.MatchString(id)
}

// Sentinel errors. Implementations may wrap them; callers use errors.Is.
var (
	// ErrNotFound is returned when the chunk does not exist.
	ErrNotFound = errors.New("chunk not found")
	// ErrInvalidChunkID is returned when the chunk ID is malformed or would
	// escape the storage directory.
	ErrInvalidChunkID = errors.New("invalid chunk id")
	// ErrMissingChecksum is returned when a chunk exists but its recorded
	// checksum metadata is missing or unreadable.
	ErrMissingChecksum = errors.New("chunk checksum metadata missing")
)

// Storage persists chunks.
type Storage interface {
	// Put stores (or overwrites) a chunk and returns its recorded information.
	Put(ctx context.Context, chunkID string, src io.Reader) (ChunkInfo, error)
	// Open returns the stored chunk stream and its recorded information.
	// The caller must close the reader.
	Open(ctx context.Context, chunkID string) (io.ReadCloser, ChunkInfo, error)
	// Delete removes a chunk. Returns ErrNotFound if it does not exist.
	Delete(ctx context.Context, chunkID string) error
	// List returns information about all stored chunks.
	List(ctx context.Context) ([]ChunkInfo, error)
}
