# erdlens

<p align="center">
  <img src="docs/logo.svg" alt="erdlens" width="96" height="96" />
</p>

> **A lens on your ERD.** Interactive, git-friendly ER diagrams for relational databases.
> Single binary · works offline · one file format · multi-DB.

erdlens does three things no other tool does together:

1. **Generate** a versionable text file from a live database (`erdlens generate`)
2. **View** it interactively in a local browser, fully offline (`erdlens view`)
3. **Ship as a single binary** you can `brew install`

![erdlens interactive viewer](docs/screen-recording.gif)

## Install

```sh
brew install erdlens/tap/erdlens
```

Or grab a binary from the [Releases page](https://github.com/erdlens/erdlens/releases).

## Quickstart

```sh
# Explore a live database in one command (temp file in /tmp)
erdlens view --dsn 'postgres://user:pass@localhost:5432/mydb'

# MySQL / MariaDB and SQLite work the same way
erdlens view --dsn 'mysql://user:pass@localhost:3306/mydb'
erdlens view --dsn 'sqlite:///path/to/db.sqlite'

# Include multiple Postgres schemas (not just public)
erdlens view --dsn 'postgres://user:pass@localhost:5432/mydb' --schema public --schema auth

# Or generate a versionable file, commit it, and view later
erdlens view --dsn 'postgres://user:pass@localhost:5432/mydb' -o schema.erd
git add schema.erd && git commit -m "snapshot schema"
erdlens view schema.erd
```

Your browser opens with an interactive canvas: pan, zoom, click a table to highlight its relationships, drag tables around (positions save back to the file).

## Browser playground (GitHub Pages)

Prefer not to install anything? Open the static playground, upload a `.erd` file, and visualize it in the browser:

**https://erdlens.github.io**

Parsing runs locally via a **parse-only WASM** build (`erdfile` + `schema` only — no database drivers, no DSN handling). Layout changes stay in memory until you click **⤓ .erd** to download.

```sh
make pages          # build WASM + static site → web/pages-dist/
make check-wasm     # assert the .wasm has no DB driver / DSN symbols
cd web && npm run preview:pages
```

## Commands

| Command | Purpose |
|---|---|
| `erdlens generate --dsn <dsn> -o <file>` | Introspect a live DB → write a `.erd` file |
| `erdlens view <file>` | Open interactive viewer on a `.erd` file (offline) |
| `erdlens view --dsn <dsn>` | Introspect a live DB and open the viewer (one step) |
| `erdlens view --dsn <dsn> -o <file>` | Introspect, save `.erd`, and open the viewer |
| `erdlens version` | Print version |

All commands take `--help` for full flag documentation.

### `generate` flags

| Flag | Purpose |
|---|---|
| `--dsn` | Database DSN (`postgres://…`, `mysql://…`, `sqlite://…`) — **required** |
| `-o, --output` | Output file path (default stdout) |
| `--schema` | Schemas/databases to include (Postgres/MySQL; ignored for SQLite). Empty → driver default (`public` / current DB) |
| `--include` | Glob patterns of table names to include (repeatable) |
| `--exclude` | Glob patterns of table names to exclude (repeatable) |
| `--timeout` | Overall connection + introspection timeout (default `30s`) |

### `view` flags

| Flag | Purpose |
|---|---|
| `--dsn` | Introspect a live database instead of reading a file |
| `-o, --output` | When using `--dsn`, save the `.erd` here (default: temp file in `/tmp`) |
| `--schema` | Schemas/databases to include when using `--dsn` (Postgres/MySQL; ignored for SQLite) |
| `--include` | Glob patterns of table names to include when using `--dsn` |
| `--exclude` | Glob patterns of table names to exclude when using `--dsn` |
| `--timeout` | Connection + introspection timeout when using `--dsn` (default `30s`) |
| `--addr` | Bind address (default `127.0.0.1:0`, OS-assigned port) |
| `--no-browser` | Don't auto-open the browser |

## Viewer keyboard shortcuts

| Key | Action |
|---|---|
| `/` | Focus the search box |
| `Esc` | Clear selection |
| `L` | Toggle layout lock |
| `I` | Toggle isolate mode (show only selected + neighbors, compact) |
| `F` | Fit view to canvas |

Search matches both **table names** and **column names**. Clicking a table pans the canvas to it and highlights the exact PK/FK columns involved in every relationship it participates in.

## `.erd` file format

Human-readable HCL. See [`docs/erd-format.md`](./docs/erd-format.md) for the full spec. Minimal example:

```hcl
meta {
  dialect = "postgres"
}

view "auth" {
  include = ["users*", "sessions*"]
}

table "users" {
  column "id"    { type = "uuid"  null = false }
  column "email" { type = "text"  null = false  unique = true }

  primary_key {
    columns = ["id"]
  }
}

table "sessions" {
  column "id"      { type = "uuid" null = false }
  column "user_id" { type = "uuid" null = false }

  primary_key { columns = ["id"] }

  foreign_key "fk_sessions_user" {
    columns     = ["user_id"]
    ref_table   = "users"
    ref_columns = ["id"]
    on_delete   = "cascade"
  }
}
```

Design guarantees:

- **Deterministic output** — `generate` twice against the same DB produces byte-identical files. Diffs stay clean.
- **Round-trip stable** — `parse → write` also byte-identical.
- **Layout persists** in `layout { x = … y = … }` blocks on each table, appended by the viewer when you drag things around.
- **Saved views** (`view "name" { include = […] }`) let you define named subgraphs for large schemas.

## Editor support

Language extensions for `.erd` files — syntax highlighting, snippets, bracket matching:

| Editor | Extension | Install |
|---|---|---|
| **VS Code** | [erdlens-vscode](https://github.com/erdlens/erdlens-vscode) | `ext install erdlens.erdlens` |
| **Zed** | [erdlens-zed](https://github.com/erdlens/erdlens-zed) | Command palette → `zed: extensions` → search "erdlens" |

Both extensions ship snippets for every block type (`table`, `column`, `foreign_key`, `index`, `view`, `layout`, `meta`) — type the block name and press <kbd>Tab</kbd>.

## Supported databases

| Dialect | Status | Example DSN |
|---|---|---|
| PostgreSQL | ✅ | `postgres://user:pass@localhost:5432/mydb` |
| MySQL / MariaDB | ✅ | `mysql://user:pass@localhost:3306/mydb` |
| SQLite | ✅ | `sqlite:///path/to/db.sqlite` |
| MSSQL | 🔜 | — |

## Development

```sh
# Full first-time build
make deps        # go mod tidy + npm install
make build       # frontend + Go binary → bin/erdlens

# Common tasks
make run                    # introspect + save + view against default DSN
make generate DSN='...'     # override the DSN
make view FILE=my.erd       # view a specific file
make test                   # go tests
make check                  # vet + test
make check-offline          # verify no external URLs in built dist/
make pages                  # GitHub Pages playground → web/pages-dist/
make check-wasm             # assert WASM has no DB driver symbols

# Dev loop with hot reload
make dev-api   # terminal 1: Go server on :8787
make dev-web   # terminal 2: Vite on :5173
cd web && npm run dev:pages   # static playground (needs make wasm first)
```

Run `make help` for the full menu.

See [CONTRIBUTING.md](./CONTRIBUTING.md) for architecture, testing, and PR guidelines.

## Non-goals

erdlens deliberately doesn't do these — other tools already do them well:

- Migrations (use [Atlas](https://github.com/ariga/atlas), [goose](https://github.com/pressly/goose), or [dbmate](https://github.com/amacneil/dbmate))
- Query execution (use [DBeaver](https://dbeaver.io), [TablePlus](https://tableplus.com), or `psql`)
- Hosted SaaS

## License

MIT — see [LICENSE](./LICENSE).
