// Step 1: the smallest possible HTTP server in Go.
//
// Run it:   go run ./learn/step01-hello-server
// Try it:   curl.exe http://localhost:8091/healthz

// Every Go file starts by saying which package it belongs to.
// "main" is special: it means "this package is a runnable program".
package main

// Imports from Go's standard library. No Maven/NuGet needed for these.
import (
	"log"      // simple logging; log.Fatal prints and exits
	"net/http" // HTTP server and client
)

// main() is where the program starts, like `public static void main` in Java.
// No class is needed around it: Go has no classes.
func main() {
	// Register a handler: "when a GET request arrives for /healthz, run this
	// function". The function receives:
	//   w - the ResponseWriter, used to write the reply
	//   r - the Request, which holds the method, URL, headers and body
	http.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // 200
		w.Write([]byte(`{"status":"ok"}`))
	})

	log.Println("listening on http://localhost:8091")

	// Start the server. ListenAndServe blocks forever, and it only returns if
	// something goes wrong (for example, the port is already in use).
	// log.Fatal then prints that error and exits the program.
	log.Fatal(http.ListenAndServe(":8091", nil))
}
