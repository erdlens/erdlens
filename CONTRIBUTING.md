# Contributing to erdlens

Thanks for your interest! This project is small, focused, and moves fast. Please open an issue before starting on anything non-trivial so we can align on approach.

## Local development

```sh
make deps        # go mod tidy + npm install
make build       # frontend + Go binary → bin/erdlens
make check       # vet + test
```

### Dev loop with hot reload

Two terminals:

```sh
# terminal 1: Go server on :8787, watching a specific .erd file
make dev-api FILE=schema.erd

# terminal 2: Vite dev server on :5173 with HMR, proxying /api → :8787
make dev-web
```

Then open `http://localhost:5173` — edit any Svelte file and the browser hot-reloads instantly.

## Project layout

```
cmd/erdlens/       CLI entry point + Cobra command wiring
internal/
  introspect/      Per-driver DB introspection (Introspector interface + impls)
  schema/          Canonical IR types (Schema, Table, Column, FK, Index, View, Layout)
  erdfile/         HCL parser (parse.go) + deterministic writer (erdfile.go)
  server/          Local HTTP API + static file server
web/               Svelte + Vite frontend
  src/lib/         Reusable components + logic
docs/              User-facing documentation
.github/workflows/ CI + release automation
```

## Coding conventions

**Go**
- Standard `gofmt` / `goimports`.
- `golangci-lint run` should pass; keep vet clean.
- Public types + funcs get doc comments.
- Errors: wrap with `%w`, prefer sentinel errors for expected failure modes.
- Don't reach for a new dependency without discussion.

**TypeScript / Svelte**
- Svelte 4 syntax (not the runes API yet) — matches the current `@xyflow/svelte` version.
- Prefer stores + reactive statements over global state.
- Keep CSS scoped to components except explicit global overrides marked `:global(...)`.
- No external HTTP calls from the viewer, ever. All assets must ship inside the Go binary. CI enforces this (`make check-offline`).

## Tests

- Unit tests live next to the code they test (`*_test.go`).
- Golden-file / round-trip tests are the strongest regression net for the file format.
- Real-world round-trip test (`realworld_test.go`) is opt-in via env:
  ```sh
  make test-roundtrip FILE=/path/to/real.erd
  ```
- Postgres integration tests are TODO. If you have testcontainers-go experience, PRs welcome.

## Adding a new database driver

1. Add the driver dep to `go.mod`.
2. Create `internal/introspect/<dialect>.go` implementing the `Introspector` interface.
3. Register the DSN scheme in `introspect.Open` (see the switch statement in `introspect.go`).
4. Add representative introspection queries — mirror the Postgres shape (tables, columns, PKs, FKs, unique constraints, indexes).
5. Add a driver-specific test file. Real-world round-trip against a seeded local DB is the gold standard.

## File format changes

Any change to the `.erd` HCL surface must:

1. Update the parser (`internal/erdfile/parse.go`).
2. Update the writer (`internal/erdfile/erdfile.go`) to match — output stays byte-stable.
3. Update `docs/erd-format.md`.
4. Add a round-trip test to `internal/erdfile/erdfile_test.go`.

## Commit style

Lowercase, imperative, short:

```
generate: fold single-column unique constraints into column.unique
viewer: fix highlight leaking across dimmed nodes
docs: clarify glob semantics in erd-format.md
```

Prefix with the package/area (`generate`, `viewer`, `parse`, `server`, `docs`, `ci`).

## Releasing

Maintainers only.

```sh
git tag v0.2.0
git push origin v0.2.0
```

The `release` workflow runs GoReleaser, which:

- Cross-compiles for linux/darwin/windows × amd64/arm64
- Creates a GitHub release with archives + checksums
- Opens a commit against `erdlens/homebrew-tap` bumping the Formula

Required secret on the `erdlens` repo: `HOMEBREW_TAP_GITHUB_TOKEN` (fine-grained PAT with `contents: write` on `erdlens/homebrew-tap`).

## License

By contributing, you agree that your contributions are licensed under the [MIT license](./LICENSE).
