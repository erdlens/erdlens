package erdfile

import (
	"fmt"
	"io"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"

	"github.com/erdlens/erdlens/internal/schema"
)

// The types below mirror the .erd HCL surface. They are intentionally
// separate from internal/schema so tags don't leak into the IR and so
// we can evolve the file format independently.

type fileDoc struct {
	Meta   *metaBlock   `hcl:"meta,block"`
	Views  []viewBlock  `hcl:"view,block"`
	Tables []tableBlock `hcl:"table,block"`
}

type metaBlock struct {
	Name    string   `hcl:"name,optional"`
	Dialect string   `hcl:"dialect,optional"`
	Remain  hcl.Body `hcl:",remain"`
}

type viewBlock struct {
	Name    string   `hcl:"name,label"`
	Include []string `hcl:"include,optional"`
	Exclude []string `hcl:"exclude,optional"`
	Remain  hcl.Body `hcl:",remain"`
}

type tableBlock struct {
	Name        string        `hcl:"name,label"`
	Schema      string        `hcl:"schema,optional"`
	Comment     string        `hcl:"comment,optional"`
	Columns     []columnBlock `hcl:"column,block"`
	PrimaryKey  *pkBlock      `hcl:"primary_key,block"`
	ForeignKeys []fkBlock     `hcl:"foreign_key,block"`
	Indexes     []indexBlock  `hcl:"index,block"`
	Layout      *layoutBlock  `hcl:"layout,block"`
	Remain      hcl.Body      `hcl:",remain"`
}

type columnBlock struct {
	Name    string   `hcl:"name,label"`
	Type    string   `hcl:"type"`
	Null    *bool    `hcl:"null,optional"`
	Default string   `hcl:"default,optional"`
	Unique  bool     `hcl:"unique,optional"`
	Comment string   `hcl:"comment,optional"`
	Remain  hcl.Body `hcl:",remain"`
}

type pkBlock struct {
	Columns []string `hcl:"columns"`
	Remain  hcl.Body `hcl:",remain"`
}

type fkBlock struct {
	Name       string   `hcl:"name,label"`
	Columns    []string `hcl:"columns"`
	RefTable   string   `hcl:"ref_table"`
	RefColumns []string `hcl:"ref_columns"`
	OnDelete   string   `hcl:"on_delete,optional"`
	OnUpdate   string   `hcl:"on_update,optional"`
	Remain     hcl.Body `hcl:",remain"`
}

type indexBlock struct {
	Name    string   `hcl:"name,label"`
	Columns []string `hcl:"columns"`
	Unique  bool     `hcl:"unique,optional"`
	Remain  hcl.Body `hcl:",remain"`
}

type layoutBlock struct {
	X      float64  `hcl:"x"`
	Y      float64  `hcl:"y"`
	Remain hcl.Body `hcl:",remain"`
}

// Parse reads an .erd (HCL) document from r and returns the canonical Schema.
func Parse(r io.Reader) (*schema.Schema, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	parser := hclparse.NewParser()
	f, diags := parser.ParseHCL(data, "schema.erd")
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse: %s", diags.Error())
	}
	var doc fileDoc
	if diags := gohcl.DecodeBody(f.Body, nil, &doc); diags.HasErrors() {
		return nil, fmt.Errorf("decode: %s", diags.Error())
	}
	return docToSchema(&doc), nil
}

func docToSchema(d *fileDoc) *schema.Schema {
	s := &schema.Schema{}
	if d.Meta != nil {
		s.Name = d.Meta.Name
		s.Dialect = d.Meta.Dialect
	}
	for _, vb := range d.Views {
		s.Views = append(s.Views, schema.View{
			Name:    vb.Name,
			Include: vb.Include,
			Exclude: vb.Exclude,
		})
	}
	for _, tb := range d.Tables {
		t := schema.Table{
			Name:    tb.Name,
			Schema:  tb.Schema,
			Comment: tb.Comment,
		}
		for _, cb := range tb.Columns {
			// Absent `null` attribute defaults to nullable=true, matching the
			// writer's convention of omitting the default.
			nullable := true
			if cb.Null != nil {
				nullable = *cb.Null
			}
			t.Columns = append(t.Columns, schema.Column{
				Name:     cb.Name,
				Type:     cb.Type,
				Nullable: nullable,
				Default:  cb.Default,
				Unique:   cb.Unique,
				Comment:  cb.Comment,
			})
		}
		if tb.PrimaryKey != nil {
			t.PrimaryKey = tb.PrimaryKey.Columns
		}
		for _, fk := range tb.ForeignKeys {
			t.ForeignKeys = append(t.ForeignKeys, schema.ForeignKey{
				Name:       fk.Name,
				Columns:    fk.Columns,
				RefTable:   fk.RefTable,
				RefColumns: fk.RefColumns,
				OnDelete:   fk.OnDelete,
				OnUpdate:   fk.OnUpdate,
			})
		}
		for _, ix := range tb.Indexes {
			t.Indexes = append(t.Indexes, schema.Index{
				Name:    ix.Name,
				Columns: ix.Columns,
				Unique:  ix.Unique,
			})
		}
		if tb.Layout != nil {
			t.Layout = &schema.Layout{X: tb.Layout.X, Y: tb.Layout.Y}
		}
		s.Tables = append(s.Tables, t)
	}
	return s
}
