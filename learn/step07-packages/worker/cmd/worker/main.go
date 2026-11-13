// Command worker runs a storage worker that serves chunks over HTTP.
//
// Run it:   go run ./learn/step07-packages/worker/cmd/worker
package main

import (
	"log"
	"net/http"

	// Imports of our own packages use the module path from go.mod plus the
	// folder path. The last element (api, storage) is the name we use.
	"github.com/Sahas-out/DistributedFileStorage/learn/step07-packages/worker/internal/api"
	"github.com/Sahas-out/DistributedFileStorage/learn/step07-packages/worker/internal/storage"
)

func main() {
	store := storage.NewMemory()
	srv := api.NewServer(store, "worker1", 1<<20)

	log.Println("listening on http://localhost:8097")
	log.Fatal(http.ListenAndServe(":8097", srv.Handler()))
}
