//go:build js && wasm

// Command erdlens-wasm exposes .erd parse/write to the browser for the
// GitHub Pages playground. It intentionally imports only erdfile + schema —
// never introspect, server, cobra, or database drivers.
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"syscall/js"

	"github.com/erdlens/erdlens/internal/erdfile"
	"github.com/erdlens/erdlens/internal/schema"
)

func main() {
	api := js.Global().Get("Object").New()
	api.Set("parse", js.FuncOf(parse))
	api.Set("write", js.FuncOf(write))
	js.Global().Set("erdlens", api)

	// Keep the Go runtime alive for JS callbacks.
	select {}
}

// parse(erdText) -> { schemaJSON } | { error }
func parse(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errResult("parse: missing erd text")
	}
	sc, err := erdfile.Parse(strings.NewReader(args[0].String()))
	if err != nil {
		return errResult(err.Error())
	}
	b, err := json.Marshal(sc)
	if err != nil {
		return errResult(err.Error())
	}
	return map[string]any{"schemaJSON": string(b)}
}

// write(schemaJSON) -> { erd } | { error }
func write(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errResult("write: missing schema JSON")
	}
	var sc schema.Schema
	if err := json.Unmarshal([]byte(args[0].String()), &sc); err != nil {
		return errResult(err.Error())
	}
	var buf bytes.Buffer
	if err := erdfile.Write(&buf, &sc); err != nil {
		return errResult(err.Error())
	}
	return map[string]any{"erd": buf.String()}
}

func errResult(msg string) map[string]any {
	return map[string]any{"error": msg}
}
