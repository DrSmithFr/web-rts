// Command rts-server runs the authoritative multiplayer host.
package main

import (
	"flag"
	"log"
	"net/http"

	"rts/server"
)

func main() {
	addr := flag.String("addr", ":8787", "listen address")
	fast := flag.Bool("fast", false, "run the tick loop 10x accelerated")
	flag.Parse()
	server.Fast = *fast
	hub := server.NewHub()
	log.Printf("rts-server listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, hub.Handler()))
}
