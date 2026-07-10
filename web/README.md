# Web (Svelte + Vite viewer)

The interactive ER diagram viewer, bundled into the `erdlens` binary via `go:embed`.

## Stack

- Svelte + Vite
- Svelte Flow (`@xyflow/svelte`) — diagram engine
- Tailwind — styling
- `lucide-svelte` — icons (tree-shaken SVGs)
- System font stack (no web-font downloads)

All assets are bundled into the Go binary. The viewer only talks to `localhost`.
**No CDN calls, no external fonts, no telemetry.**

## Scaffold (one-time)

The `dist/.gitkeep` placeholder exists so `go build` works before the frontend
is initialized. To create the actual Svelte project here:

```sh
# From the erdlens/ repo root
rm -rf web
npm create vite@latest web -- --template svelte-ts
cd web
npm install @xyflow/svelte
npm install -D tailwindcss postcss autoprefixer lucide-svelte
npx tailwindcss init -p
npm run build      # populates web/dist/ for go:embed
```

Then re-add the `dist/.gitkeep` (or a `.gitignore` rule) so committed builds
are optional.

## Dev loop

```sh
npm run dev        # Vite dev server on :5173
# in another terminal:
go run ../cmd/erdlens view ../examples/schema.erd
```

During development the Go server can proxy to Vite; in production it serves
the embedded `dist/` bundle directly.
