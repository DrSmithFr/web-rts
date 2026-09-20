//go:build !js || !wasm

// The WebAssembly bridge only builds for js/wasm; this stub exists so that
// native tooling (go build ./..., go vet ./...) keeps working on the
// package.
package main

func main() {}
