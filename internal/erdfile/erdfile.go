// Package erdfile parses and writes the human-readable, git-versionable
// .erd file format (HCL-flavored).
//
// The writer is a hand-rolled, deterministic formatter rather than a
// generic HCL emitter. Reasons:
//   - Byte-stable output across Go and HCL library versions.
//   - Full control over ordering, spacing, and which optional fields are elided.
//   - Diffs stay clean across regenerations, which is the whole product promise.
//
// The parser uses hashicorp/hcl/v2 (see parse.go).
package erdfile

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/erdlens/erdlens/internal/schema"
)

// Write serializes s to w in canonical .erd (HCL) form.
// Output is deterministic: given the same input Schema, Write always
// produces byte-identical output.
func Write(w io.Writer, s *schema.Schema) error {
	if s == nil {
		return errors.New("erdfile.Write: nil schema")
	}
	bw := bufio.NewWriter(w)
	f := &formatter{w: bw}

	blockCount := 0
	if s.Name != "" || s.Dialect != "" {
		f.writeMeta(s)
		blockCount++
	}

	views := append([]schema.View(nil), s.Views...)
	sort.SliceStable(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	for _, v := range views {
		if blockCount > 0 {
			f.newline()
		}
		f.writeView(&v)
		blockCount++
	}

	// Sort tables by (schema, name) for stable output.
	tables := append([]schema.Table(nil), s.Tables...)
	sort.SliceStable(tables, func(i, j int) bool {
		if tables[i].Schema != tables[j].Schema {
			return tables[i].Schema < tables[j].Schema
		}
		return tables[i].Name < tables[j].Name
	})

	for _, t := range tables {
		if blockCount > 0 {
			f.newline()
		}
		f.writeTable(&t)
		blockCount++
	}

	if err := f.err; err != nil {
		return err
	}
	return bw.Flush()
}

// formatter is a tiny stateful writer that tracks the first error it hits
// so callers don't have to check every Fprintf.
type formatter struct {
	w   *bufio.Writer
	err error
}

func (f *formatter) printf(format string, args ...any) {
	if f.err != nil {
		return
	}
	_, f.err = fmt.Fprintf(f.w, format, args...)
}

func (f *formatter) newline() {
	if f.err != nil {
		return
	}
	_, f.err = f.w.WriteString("\n")
}

func (f *formatter) writeMeta(s *schema.Schema) {
	if s.Dialect == "" && s.Name == "" {
		return
	}
	f.printf("meta {\n")
	if s.Name != "" {
		f.printf("  name    = %s\n", q(s.Name))
	}
	if s.Dialect != "" {
		f.printf("  dialect = %s\n", q(s.Dialect))
	}
	f.printf("}\n")
}

func (f *formatter) writeView(v *schema.View) {
	f.printf("view %s {\n", q(v.Name))
	if len(v.Include) > 0 {
		f.printf("  include = %s\n", qList(v.Include))
	}
	if len(v.Exclude) > 0 {
		f.printf("  exclude = %s\n", qList(v.Exclude))
	}
	f.printf("}\n")
}

func (f *formatter) writeTable(t *schema.Table) {
	f.printf("table %s {\n", q(t.Name))
	if t.Schema != "" && t.Schema != "public" {
		f.printf("  schema  = %s\n", q(t.Schema))
	}
	if t.Comment != "" {
		f.printf("  comment = %s\n", q(t.Comment))
	}

	for _, c := range t.Columns {
		f.newline()
		f.writeColumn(&c)
	}

	if len(t.PrimaryKey) > 0 {
		f.newline()
		f.printf("  primary_key {\n")
		f.printf("    columns = %s\n", qList(t.PrimaryKey))
		f.printf("  }\n")
	}

	fks := append([]schema.ForeignKey(nil), t.ForeignKeys...)
	sort.SliceStable(fks, func(i, j int) bool { return fks[i].Name < fks[j].Name })
	for _, fk := range fks {
		f.newline()
		f.writeForeignKey(&fk)
	}

	idxs := append([]schema.Index(nil), t.Indexes...)
	sort.SliceStable(idxs, func(i, j int) bool { return idxs[i].Name < idxs[j].Name })
	for _, idx := range idxs {
		f.newline()
		f.writeIndex(&idx)
	}

	if t.Layout != nil {
		f.newline()
		f.printf("  layout {\n")
		f.printf("    x = %s\n", fmtFloat(t.Layout.X))
		f.printf("    y = %s\n", fmtFloat(t.Layout.Y))
		f.printf("  }\n")
	}

	f.printf("}\n")
}

func (f *formatter) writeColumn(c *schema.Column) {
	f.printf("  column %s {\n", q(c.Name))
	f.printf("    type = %s\n", q(c.Type))
	// Emit null only when non-default. Default: null = true (nullable).
	if !c.Nullable {
		f.printf("    null = false\n")
	}
	if c.Default != "" {
		f.printf("    default = %s\n", q(c.Default))
	}
	if c.Unique {
		f.printf("    unique = true\n")
	}
	if c.Comment != "" {
		f.printf("    comment = %s\n", q(c.Comment))
	}
	f.printf("  }\n")
}

func (f *formatter) writeForeignKey(fk *schema.ForeignKey) {
	name := fk.Name
	if name == "" {
		name = "fk_" + strings.Join(fk.Columns, "_")
	}
	f.printf("  foreign_key %s {\n", q(name))
	f.printf("    columns     = %s\n", qList(fk.Columns))
	f.printf("    ref_table   = %s\n", q(fk.RefTable))
	f.printf("    ref_columns = %s\n", qList(fk.RefColumns))
	if fk.OnDelete != "" {
		f.printf("    on_delete   = %s\n", q(fk.OnDelete))
	}
	if fk.OnUpdate != "" {
		f.printf("    on_update   = %s\n", q(fk.OnUpdate))
	}
	f.printf("  }\n")
}

func (f *formatter) writeIndex(idx *schema.Index) {
	f.printf("  index %s {\n", q(idx.Name))
	f.printf("    columns = %s\n", qList(idx.Columns))
	if idx.Unique {
		f.printf("    unique  = true\n")
	}
	f.printf("  }\n")
}

// q returns a Go/HCL-compatible quoted string literal.
func q(s string) string { return strconv.Quote(s) }

// qList renders a slice of strings as an HCL list literal.
func qList(ss []string) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, s := range ss {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(s))
	}
	b.WriteByte(']')
	return b.String()
}

// fmtFloat formats a float without trailing zeros so layouts diff cleanly.
func fmtFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
