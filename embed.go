package erdlens

import (
	"embed"
	"io/fs"
)

// webDist is the built Svelte frontend, embedded at compile time.
// Run `cd web && npm run build` before `go build` to populate it.
//
//go:embed all:web/dist
var webDist embed.FS

// Assets returns the frontend bundle rooted at web/dist so the server can
// serve it as if it were the filesystem root.
func Assets() (fs.FS, error) {
	return fs.Sub(webDist, "web/dist")
}
