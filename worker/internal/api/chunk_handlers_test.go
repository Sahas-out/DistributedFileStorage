package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Sahas-out/DistributedFileStorage/pkg/protocol"
	"github.com/Sahas-out/DistributedFileStorage/worker/internal/storage"
)

// mockStorage lets each test inject behaviour and records calls.
type mockStorage struct {
	putFn    func(ctx context.Context, id string, src io.Reader) (storage.ChunkInfo, error)
	openFn   func(ctx context.Context, id string) (io.ReadCloser, storage.ChunkInfo, error)
	deleteFn func(ctx context.Context, id string) error
	listFn   func(ctx context.Context) ([]storage.ChunkInfo, error)

	mu    sync.Mutex
	calls []string
}

func (m *mockStorage) record(c string) {
	m.mu.Lock()
	m.calls = append(m.calls, c)
	m.mu.Unlock()
}

func (m *mockStorage) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockStorage) Put(ctx context.Context, id string, src io.Reader) (storage.ChunkInfo, error) {
	m.record("Put " + id)
	if m.putFn == nil {
		panic("unexpected Put")
	}
	return m.putFn(ctx, id, src)
}

func (m *mockStorage) Open(ctx context.Context, id string) (io.ReadCloser, storage.ChunkInfo, error) {
	m.record("Open " + id)
	if m.openFn == nil {
		panic("unexpected Open")
	}
	return m.openFn(ctx, id)
}

func (m *mockStorage) Delete(ctx context.Context, id string) error {
	m.record("Delete " + id)
	if m.deleteFn == nil {
		panic("unexpected Delete")
	}
	return m.deleteFn(ctx, id)
}

func (m *mockStorage) List(ctx context.Context) ([]storage.ChunkInfo, error) {
	m.record("List")
	if m.listFn == nil {
		panic("unexpected List")
	}
	return m.listFn(ctx)
}

const testMaxChunk = 1 << 20

func newTestHandler(store storage.Storage) http.Handler {
	return NewServer(store, "worker-test", testMaxChunk, nil).Handler()
}

func do(t *testing.T, h http.Handler, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// echoPut is a Put that reads the body and returns correct info.
func echoPut(_ context.Context, id string, src io.Reader) (storage.ChunkInfo, error) {
	b, err := io.ReadAll(src)
	if err != nil {
		return storage.ChunkInfo{}, err
	}
	return storage.ChunkInfo{ID: id, Size: int64(len(b)), Checksum: sum(b)}, nil
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) protocol.ErrorBody {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("error Content-Type = %q, want application/json", ct)
	}
	var e protocol.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	if e.Error.Code == "" || e.Error.Message == "" {
		t.Fatalf("error body missing code/message: %q", rec.Body.String())
	}
	return e.Error
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, status, rec.Body.String())
	}
	if got := decodeError(t, rec).Code; got != code {
		t.Fatalf("error code = %q, want %q", got, code)
	}
}

var invalidIDPaths = []struct {
	name string
	path string
}{
	{"dot", "a.b"},
	{"encoded slash", "a%2Fb"},
	{"encoded dot-dot", "%2E%2E"},
	{"encoded traversal", "..%2Fetc%2Fpasswd"},
	{"space", "a%20b"},
	{"too long", strings.Repeat("a", 129)},
	{"unicode", "%C3%A9"},
}

// ---------- PUT ----------

func TestPutChunk(t *testing.T) {
	data := []byte("hello chunk")

	t.Run("happy path returns 201 and ChunkInfo", func(t *testing.T) {
		m := &mockStorage{putFn: echoPut}
		rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/obj1_0", data, nil)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body %q)", rec.Code, rec.Body.String())
		}
		var info protocol.ChunkInfo
		if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
			t.Fatal(err)
		}
		want := protocol.ChunkInfo{ID: "obj1_0", Size: int64(len(data)), Checksum: sum(data)}
		if info != want {
			t.Fatalf("info = %+v, want %+v", info, want)
		}
	})

	t.Run("max-length id accepted", func(t *testing.T) {
		m := &mockStorage{putFn: echoPut}
		rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/"+strings.Repeat("a", 128), data, nil)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", rec.Code)
		}
	})

	t.Run("empty body accepted", func(t *testing.T) {
		m := &mockStorage{putFn: echoPut}
		rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/empty", []byte{}, nil)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", rec.Code)
		}
	})

	for _, tc := range invalidIDPaths {
		t.Run("invalid id "+tc.name, func(t *testing.T) {
			m := &mockStorage{}
			rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/"+tc.path, data, nil)
			expectError(t, rec, http.StatusBadRequest, protocol.CodeInvalidChunkID)
			if m.callCount() != 0 {
				t.Fatalf("storage was called: %v", m.calls)
			}
		})
	}

	t.Run("rejected PUTs still consume the whole body", func(t *testing.T) {
		// Replying while body bytes are unread makes the server close the
		// connection, and the TCP reset can destroy the response in transit.
		for _, tc := range []struct{ path, checksum string }{
			{"/internal/chunks/bad.id", ""},
			{"/internal/chunks/c1", "not-a-digest"},
			{"/internal/chunks/c1", sum([]byte("other"))},
		} {
			body := bytes.NewReader(make([]byte, testMaxChunk))
			req := httptest.NewRequest(http.MethodPut, tc.path, body)
			if tc.checksum != "" {
				req.Header.Set(protocol.HeaderChunkChecksum, tc.checksum)
			}
			rec := httptest.NewRecorder()
			newTestHandler(&mockStorage{}).ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s: status = %d, want 400", tc.path, rec.Code)
			}
			if body.Len() != 0 {
				t.Fatalf("%s: %d body bytes left unread", tc.path, body.Len())
			}
		}
	})

	t.Run("exactly max size accepted", func(t *testing.T) {
		m := &mockStorage{putFn: echoPut}
		rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/big", make([]byte, testMaxChunk), nil)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", rec.Code)
		}
	})

	t.Run("oversized body returns 413", func(t *testing.T) {
		m := &mockStorage{}
		rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/big", make([]byte, testMaxChunk+1), nil)
		expectError(t, rec, http.StatusRequestEntityTooLarge, protocol.CodeChunkTooLarge)
		if m.callCount() != 0 {
			t.Fatalf("storage was called: %v", m.calls)
		}
	})

	t.Run("matching checksum header accepted (case-insensitive)", func(t *testing.T) {
		m := &mockStorage{putFn: echoPut}
		hdr := map[string]string{protocol.HeaderChunkChecksum: strings.ToUpper(sum(data))}
		rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/c1", data, hdr)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body %q)", rec.Code, rec.Body.String())
		}
	})

	t.Run("mismatched checksum header returns 400 and does not store", func(t *testing.T) {
		m := &mockStorage{}
		hdr := map[string]string{protocol.HeaderChunkChecksum: sum([]byte("other"))}
		rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/c1", data, hdr)
		expectError(t, rec, http.StatusBadRequest, protocol.CodeChecksumMismatch)
		if m.callCount() != 0 {
			t.Fatalf("storage was called: %v", m.calls)
		}
	})

	t.Run("malformed checksum header returns 400", func(t *testing.T) {
		m := &mockStorage{}
		hdr := map[string]string{protocol.HeaderChunkChecksum: "not-a-digest"}
		rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/c1", data, hdr)
		expectError(t, rec, http.StatusBadRequest, protocol.CodeInvalidRequest)
		if m.callCount() != 0 {
			t.Fatalf("storage was called: %v", m.calls)
		}
	})

	t.Run("storage failure returns 500 without leaking details", func(t *testing.T) {
		m := &mockStorage{putFn: func(context.Context, string, io.Reader) (storage.ChunkInfo, error) {
			return storage.ChunkInfo{}, errors.New("disk on fire at /secret/path")
		}}
		rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/c1", data, nil)
		expectError(t, rec, http.StatusInternalServerError, protocol.CodeStorageError)
		if strings.Contains(rec.Body.String(), "/secret/path") {
			t.Fatalf("internal error leaked: %q", rec.Body.String())
		}
	})

	t.Run("storage invalid id error returns 400", func(t *testing.T) {
		m := &mockStorage{putFn: func(context.Context, string, io.Reader) (storage.ChunkInfo, error) {
			return storage.ChunkInfo{}, fmt.Errorf("wrapped: %w", storage.ErrInvalidChunkID)
		}}
		rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/c1", data, nil)
		expectError(t, rec, http.StatusBadRequest, protocol.CodeInvalidChunkID)
	})

	t.Run("storage returning inconsistent info returns 500", func(t *testing.T) {
		m := &mockStorage{putFn: func(_ context.Context, id string, src io.Reader) (storage.ChunkInfo, error) {
			_, _ = io.ReadAll(src)
			return storage.ChunkInfo{ID: id, Size: 1, Checksum: sum([]byte("x"))}, nil
		}}
		rec := do(t, newTestHandler(m), http.MethodPut, "/internal/chunks/c1", data, nil)
		expectError(t, rec, http.StatusInternalServerError, protocol.CodeStorageError)
	})
}

// ---------- GET ----------

func openReturning(data []byte, info storage.ChunkInfo) func(context.Context, string) (io.ReadCloser, storage.ChunkInfo, error) {
	return func(context.Context, string) (io.ReadCloser, storage.ChunkInfo, error) {
		return io.NopCloser(bytes.NewReader(data)), info, nil
	}
}

type trackingCloser struct {
	io.Reader
	closed bool
}

func (c *trackingCloser) Close() error { c.closed = true; return nil }

func TestGetChunk(t *testing.T) {
	data := []byte("some chunk bytes \x00\x01\x02")
	good := storage.ChunkInfo{ID: "c1", Size: int64(len(data)), Checksum: sum(data)}

	t.Run("happy path returns bytes and headers", func(t *testing.T) {
		tc := &trackingCloser{Reader: bytes.NewReader(data)}
		m := &mockStorage{openFn: func(context.Context, string) (io.ReadCloser, storage.ChunkInfo, error) {
			return tc, good, nil
		}}
		rec := do(t, newTestHandler(m), http.MethodGet, "/internal/chunks/c1", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
		}
		if !bytes.Equal(rec.Body.Bytes(), data) {
			t.Fatalf("body mismatch")
		}
		h := rec.Header()
		if got := h.Get("Content-Type"); got != "application/octet-stream" {
			t.Errorf("Content-Type = %q", got)
		}
		if got := h.Get("Content-Length"); got != fmt.Sprint(len(data)) {
			t.Errorf("Content-Length = %q", got)
		}
		if got := h.Get(protocol.HeaderChunkChecksum); got != good.Checksum {
			t.Errorf("%s = %q", protocol.HeaderChunkChecksum, got)
		}
		if got := h.Get(protocol.HeaderChunkSize); got != fmt.Sprint(len(data)) {
			t.Errorf("%s = %q", protocol.HeaderChunkSize, got)
		}
		if !tc.closed {
			t.Errorf("reader was not closed")
		}
	})

	t.Run("HEAD returns headers without body", func(t *testing.T) {
		m := &mockStorage{openFn: openReturning(data, good)}
		rec := do(t, newTestHandler(m), http.MethodHead, "/internal/chunks/c1", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("HEAD returned a body")
		}
		if rec.Header().Get(protocol.HeaderChunkChecksum) != good.Checksum {
			t.Fatalf("missing checksum header")
		}
	})

	t.Run("missing chunk returns 404", func(t *testing.T) {
		m := &mockStorage{openFn: func(context.Context, string) (io.ReadCloser, storage.ChunkInfo, error) {
			return nil, storage.ChunkInfo{}, fmt.Errorf("open: %w", storage.ErrNotFound)
		}}
		rec := do(t, newTestHandler(m), http.MethodGet, "/internal/chunks/nope", nil, nil)
		expectError(t, rec, http.StatusNotFound, protocol.CodeChunkNotFound)
	})

	t.Run("corrupted content returns 500 chunk_corrupted", func(t *testing.T) {
		corrupted := append([]byte(nil), data...)
		corrupted[0] ^= 0xFF
		m := &mockStorage{openFn: openReturning(corrupted, good)}
		rec := do(t, newTestHandler(m), http.MethodGet, "/internal/chunks/c1", nil, nil)
		expectError(t, rec, http.StatusInternalServerError, protocol.CodeChunkCorrupted)
		if bytes.Contains(rec.Body.Bytes(), corrupted) {
			t.Fatalf("corrupted bytes were returned")
		}
	})

	t.Run("truncated content returns 500 chunk_corrupted", func(t *testing.T) {
		m := &mockStorage{openFn: openReturning(data[:5], good)}
		rec := do(t, newTestHandler(m), http.MethodGet, "/internal/chunks/c1", nil, nil)
		expectError(t, rec, http.StatusInternalServerError, protocol.CodeChunkCorrupted)
	})

	t.Run("empty recorded checksum returns 500 chunk_corrupted", func(t *testing.T) {
		m := &mockStorage{openFn: openReturning(data, storage.ChunkInfo{ID: "c1", Size: int64(len(data))})}
		rec := do(t, newTestHandler(m), http.MethodGet, "/internal/chunks/c1", nil, nil)
		expectError(t, rec, http.StatusInternalServerError, protocol.CodeChunkCorrupted)
	})

	t.Run("missing checksum metadata returns 500 storage_error", func(t *testing.T) {
		m := &mockStorage{openFn: func(context.Context, string) (io.ReadCloser, storage.ChunkInfo, error) {
			return nil, storage.ChunkInfo{}, storage.ErrMissingChecksum
		}}
		rec := do(t, newTestHandler(m), http.MethodGet, "/internal/chunks/c1", nil, nil)
		expectError(t, rec, http.StatusInternalServerError, protocol.CodeStorageError)
	})

	t.Run("read failure returns 500", func(t *testing.T) {
		m := &mockStorage{openFn: func(context.Context, string) (io.ReadCloser, storage.ChunkInfo, error) {
			return io.NopCloser(io.MultiReader(bytes.NewReader(data[:3]), errReader{})), good, nil
		}}
		rec := do(t, newTestHandler(m), http.MethodGet, "/internal/chunks/c1", nil, nil)
		expectError(t, rec, http.StatusInternalServerError, protocol.CodeStorageError)
	})

	for _, tc := range invalidIDPaths {
		t.Run("invalid id "+tc.name, func(t *testing.T) {
			m := &mockStorage{}
			rec := do(t, newTestHandler(m), http.MethodGet, "/internal/chunks/"+tc.path, nil, nil)
			expectError(t, rec, http.StatusBadRequest, protocol.CodeInvalidChunkID)
			if m.callCount() != 0 {
				t.Fatalf("storage was called: %v", m.calls)
			}
		})
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

// ---------- DELETE ----------

func TestDeleteChunk(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"existing chunk", nil, http.StatusNoContent},
		{"missing chunk is idempotent", storage.ErrNotFound, http.StatusNoContent},
		{"wrapped missing chunk", fmt.Errorf("x: %w", storage.ErrNotFound), http.StatusNoContent},
		{"storage failure", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mockStorage{deleteFn: func(context.Context, string) error { return tc.err }}
			rec := do(t, newTestHandler(m), http.MethodDelete, "/internal/chunks/c1", nil, nil)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if tc.wantCode == http.StatusNoContent && rec.Body.Len() != 0 {
				t.Fatalf("204 with body %q", rec.Body.String())
			}
			if tc.wantCode == http.StatusInternalServerError {
				expectError(t, rec, tc.wantCode, protocol.CodeStorageError)
			}
		})
	}

	t.Run("invalid id", func(t *testing.T) {
		m := &mockStorage{}
		rec := do(t, newTestHandler(m), http.MethodDelete, "/internal/chunks/a.b", nil, nil)
		expectError(t, rec, http.StatusBadRequest, protocol.CodeInvalidChunkID)
	})
}

// ---------- LIST ----------

func TestListChunks(t *testing.T) {
	t.Run("empty store returns empty array", func(t *testing.T) {
		m := &mockStorage{listFn: func(context.Context) ([]storage.ChunkInfo, error) { return nil, nil }}
		rec := do(t, newTestHandler(m), http.MethodGet, "/internal/workers/worker-test/chunks", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `"chunks":[]`) {
			t.Fatalf("expected empty array, got %q", rec.Body.String())
		}
	})

	t.Run("returns inventory regardless of worker id in path", func(t *testing.T) {
		chunks := []storage.ChunkInfo{
			{ID: "a", Size: 1, Checksum: sum([]byte("a"))},
			{ID: "b", Size: 2, Checksum: sum([]byte("bb"))},
		}
		m := &mockStorage{listFn: func(context.Context) ([]storage.ChunkInfo, error) { return chunks, nil }}
		for _, wid := range []string{"worker-test", "someone-else"} {
			rec := do(t, newTestHandler(m), http.MethodGet, "/internal/workers/"+wid+"/chunks", nil, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			var got protocol.ChunkList
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.WorkerID != "worker-test" || got.Count != 2 || len(got.Chunks) != 2 || got.Chunks[1] != chunks[1] {
				t.Fatalf("unexpected list: %+v", got)
			}
		}
	})

	t.Run("storage failure returns 500", func(t *testing.T) {
		m := &mockStorage{listFn: func(context.Context) ([]storage.ChunkInfo, error) { return nil, errors.New("boom") }}
		rec := do(t, newTestHandler(m), http.MethodGet, "/internal/workers/w/chunks", nil, nil)
		expectError(t, rec, http.StatusInternalServerError, protocol.CodeStorageError)
	})
}

// ---------- misc ----------

func TestHealth(t *testing.T) {
	rec := do(t, newTestHandler(&mockStorage{}), http.MethodGet, "/healthz", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["worker_id"] != "worker-test" {
		t.Fatalf("unexpected body %v", body)
	}
}

func TestRouting(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	if rec := do(t, h, http.MethodPost, "/internal/chunks/c1", []byte("x"), nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST chunk: status = %d, want 405", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/internal/chunks/", nil, nil); rec.Code != http.StatusNotFound {
		t.Errorf("empty id: status = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/nope", nil, nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown route: status = %d, want 404", rec.Code)
	}
}

func TestPanicRecovery(t *testing.T) {
	m := &mockStorage{listFn: func(context.Context) ([]storage.ChunkInfo, error) { panic("kaboom") }}
	rec := do(t, newTestHandler(m), http.MethodGet, "/internal/workers/w/chunks", nil, nil)
	expectError(t, rec, http.StatusInternalServerError, protocol.CodeInternalError)
}

// ---------- round trip against the in-memory store ----------

func TestRoundTripWithMemoryStore(t *testing.T) {
	srv := httptest.NewServer(newTestHandler(storage.NewMemory()))
	defer srv.Close()
	client := srv.Client()

	data := bytes.Repeat([]byte{0xAB, 0x00, 0x7F}, 200_000) // ~600 KB of binary data
	url := srv.URL + "/internal/chunks/obj_0"

	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	req.Header.Set(protocol.HeaderChunkChecksum, sum(data))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT status = %d", resp.StatusCode)
	}

	resp, err = client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(got, data) {
		t.Fatalf("GET status = %d, bytes equal = %v", resp.StatusCode, bytes.Equal(got, data))
	}
	if resp.Header.Get(protocol.HeaderChunkChecksum) != sum(data) {
		t.Fatalf("checksum header mismatch")
	}

	resp, err = client.Get(srv.URL + "/internal/workers/any/chunks")
	if err != nil {
		t.Fatal(err)
	}
	var list protocol.ChunkList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if list.Count != 1 || list.Chunks[0].ID != "obj_0" || list.Chunks[0].Size != int64(len(data)) {
		t.Fatalf("unexpected list %+v", list)
	}

	for i := 0; i < 2; i++ { // second delete is idempotent
		req, _ = http.NewRequest(http.MethodDelete, url, nil)
		resp, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("DELETE #%d status = %d", i+1, resp.StatusCode)
		}
	}

	resp, err = client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after delete status = %d", resp.StatusCode)
	}
}
