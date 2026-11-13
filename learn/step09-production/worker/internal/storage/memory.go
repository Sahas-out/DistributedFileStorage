package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sort"
	"sync"
)

// Memory is an in-memory Storage. It is a temporary stand-in until the
// filesystem implementation (Person 1) lands; data does not survive restarts.
type Memory struct {
	mu     sync.RWMutex
	chunks map[string]memChunk
}

type memChunk struct {
	data []byte
	info ChunkInfo
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{chunks: make(map[string]memChunk)}
}

var _ Storage = (*Memory)(nil)

// Put stores or overwrites a chunk.
func (m *Memory) Put(ctx context.Context, chunkID string, src io.Reader) (ChunkInfo, error) {
	if !ValidChunkID(chunkID) {
		return ChunkInfo{}, ErrInvalidChunkID
	}
	data, err := io.ReadAll(src)
	if err != nil {
		return ChunkInfo{}, err
	}
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

// Open returns the chunk data and its recorded information.
func (m *Memory) Open(_ context.Context, chunkID string) (io.ReadCloser, ChunkInfo, error) {
	m.mu.RLock()
	c, ok := m.chunks[chunkID]
	m.mu.RUnlock()
	if !ok {
		return nil, ChunkInfo{}, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(c.data)), c.info, nil
}

// Delete removes a chunk.
func (m *Memory) Delete(_ context.Context, chunkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.chunks[chunkID]; !ok {
		return ErrNotFound
	}
	delete(m.chunks, chunkID)
	return nil
}

// List returns all chunks sorted by ID.
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
