//go:build !js || !wasm

// Stub so `go build ./...` on a native GOOS still typechecks this package.
// The real entrypoint is main.go (js && wasm).
package main

func main() {}
