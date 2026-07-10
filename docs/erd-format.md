# The `.erd` file format

`.erd` is a human-readable, git-friendly serialization of a relational schema. It's [HCL v2](https://github.com/hashicorp/hcl) — the same language Terraform uses — chosen because:

- It has a mature Go parser.
- It supports comments (`# …` or `// …`).
- It's easy to edit by hand.
- It diffs cleanly in code review.

The writer is deterministic: parsing an `.erd` file and writing it back produces the **exact same bytes**. This is the property that makes `.erd` files safe to check into git.

## Top-level structure

An `.erd` file contains, in order:

1. An optional `meta { … }` block.
2. Zero or more `view "name" { … }` blocks.
3. One or more `table "name" { … }` blocks.

Blocks are separated by a single blank line. Tables are alphabetically sorted by (`schema`, `name`).

## `meta` block

Describes the source of the schema.

```hcl
meta {
  name    = "dbname"    # optional
  dialect = "postgres"          # optional: postgres | mysql | sqlite | mssql
}
```

Both fields are optional. If the block is empty, it's omitted from output entirely.

## `view` block

Defines a named subset of tables. The viewer surfaces these in a dropdown; picking one hides everything outside the view. Great for large schemas ("auth stuff", "billing stuff", "the reporting subgraph").

```hcl
view "auth" {
  include = ["users*", "sessions*", "oauth_*"]
  exclude = ["users_audit_log"]
}

view "billing" {
  include = ["invoices*", "payments*", "subscriptions"]
}
```

| Attribute | Type | Purpose |
|---|---|---|
| `include` | `list(string)`, optional | Glob patterns; empty = all tables |
| `exclude` | `list(string)`, optional | Glob patterns; wins over include |

Glob syntax: `*` (any run of chars), `?` (single char). Patterns match against the **table name** (not schema-qualified).

## `table` block

The core of an `.erd` file.

```hcl
table "orders" {
  schema  = "public"                    # optional; omitted when it's "public"
  comment = "Customer orders"           # optional

  column "id" {
    type = "uuid"
    null = false
  }

  column "user_id" {
    type = "uuid"
    null = false
  }

  column "total_cents" {
    type    = "integer"
    null    = false
    default = "0"
  }

  primary_key {
    columns = ["id"]
  }

  foreign_key "fk_orders_user" {
    columns     = ["user_id"]
    ref_table   = "users"
    ref_columns = ["id"]
    on_delete   = "cascade"
  }

  index "idx_orders_user" {
    columns = ["user_id"]
  }

  layout {
    x = 480
    y = 260
  }
}
```

### `column` sub-block

| Attribute | Type | Default | Purpose |
|---|---|---|---|
| `type` | `string` | — | DB-native type (e.g. `"uuid"`, `"timestamp with time zone"`, `"numeric(8,2)"`) |
| `null` | `bool` | `true` | Nullable? Only written when `false` |
| `default` | `string` | — | Default expression as printed by the DB (e.g. `"CURRENT_TIMESTAMP"`, `"nextval('seq'::regclass)"`) |
| `unique` | `bool` | `false` | Single-column unique constraint |
| `comment` | `string` | — | Column comment |

### `primary_key` sub-block

```hcl
primary_key {
  columns = ["tenant_id", "id"]
}
```

Written only when a PK exists. Supports composite keys.

### `foreign_key` sub-block

Every FK is labeled with its constraint name.

```hcl
foreign_key "fk_orders_user" {
  columns     = ["user_id"]
  ref_table   = "users"
  ref_columns = ["id"]
  on_delete   = "cascade"       # optional
  on_update   = "restrict"      # optional
}
```

Referential action values (both `on_delete` and `on_update`):

| Value | Postgres code | Meaning |
|---|---|---|
| _(omitted)_ | `a` | `NO ACTION` (default) |
| `"restrict"` | `r` | `RESTRICT` |
| `"cascade"` | `c` | `CASCADE` |
| `"set_null"` | `n` | `SET NULL` |
| `"set_default"` | `d` | `SET DEFAULT` |

Multi-column FKs use parallel arrays: `columns[i]` references `ref_columns[i]`.

### `index` sub-block

Non-primary, non-unique-constraint-backing indexes only. Composite unique constraints are emitted here with `unique = true`.

```hcl
index "idx_orders_created_at" {
  columns = ["created_at"]
}

index "orders_ext_id_unique" {
  columns = ["external_id"]
  unique  = true
}
```

### `layout` sub-block

Optional per-table position hint, appended by the viewer when you drag a table.

```hcl
layout {
  x = 480
  y = 260
}
```

Coordinates are top-left corner of the node, in Svelte Flow canvas units. Regenerating from a live DB **never clobbers** layout blocks — they're preserved unless you pass `--force-layout` (planned).

## Ordering guarantees

For byte-identical output:

- Top-level: `meta`, then `view`s alphabetical by name, then `table`s alphabetical by (`schema`, `name`).
- Inside a table: `column`s in physical order (as introspected), then `primary_key`, then `foreign_key`s alphabetical by name, then `index`es alphabetical by name, then `layout`.
- Inside `foreign_key`: `columns` and `ref_columns` preserve the constraint's declared column order.

## Comments and hand-editing

You can add HCL comments anywhere:

```hcl
# All customer-facing tables live in this schema.
table "orders" {
  # ...
}
```

**Comments are not preserved across `generate` regeneration today** — regenerating from a live DB overwrites the file. This is a v2 improvement (comment-preserving round-trip via HCL AST).

## Grammar summary

```
file        ::= meta? view* table*
meta        ::= 'meta' '{' meta_attr* '}'
meta_attr   ::= 'name' '=' STRING
              | 'dialect' '=' STRING

view        ::= 'view' STRING '{' view_attr* '}'
view_attr   ::= 'include' '=' STRING_LIST
              | 'exclude' '=' STRING_LIST

table       ::= 'table' STRING '{' table_body '}'
table_body  ::= ('schema' '=' STRING)?
                ('comment' '=' STRING)?
                column*
                primary_key?
                foreign_key*
                index*
                layout?

column      ::= 'column' STRING '{' column_attr* '}'
primary_key ::= 'primary_key' '{' 'columns' '=' STRING_LIST '}'
foreign_key ::= 'foreign_key' STRING '{' fk_attr+ '}'
index       ::= 'index' STRING '{' 'columns' '=' STRING_LIST ('unique' '=' 'true')? '}'
layout      ::= 'layout' '{' 'x' '=' NUMBER 'y' '=' NUMBER '}'
```
