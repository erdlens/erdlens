// Simple glob matcher for `*` and `?`. Matches erdlens .erd include/exclude
// semantics: patterns are matched against table names as whole strings.

const cache = new Map<string, RegExp>()

function globToRegex(pattern: string): RegExp {
  const cached = cache.get(pattern)
  if (cached) return cached
  const escaped = pattern
    .replace(/[.+^${}()|[\]\\]/g, '\\$&')
    .replace(/\*/g, '.*')
    .replace(/\?/g, '.')
  const re = new RegExp(`^${escaped}$`)
  cache.set(pattern, re)
  return re
}

// matchesView returns true if `name` should be included given the include and
// exclude glob lists. Exclude wins over include; empty include means all.
export function matchesView(
  name: string,
  include?: string[],
  exclude?: string[],
): boolean {
  for (const p of exclude ?? []) {
    if (globToRegex(p).test(name)) return false
  }
  if (!include || include.length === 0) return true
  return include.some((p) => globToRegex(p).test(name))
}
