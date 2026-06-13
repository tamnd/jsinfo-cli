package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/tabwriter"
	"text/template"
)

// validFormat reports whether f is a supported output format.
func validFormat(f string) bool {
	switch f {
	case "table", "json", "jsonl", "csv", "tsv":
		return true
	}
	return false
}

// Renderer writes records in the configured format.
type Renderer struct {
	format   string
	fields   []string
	noHeader bool
	tmpl     string
	w        io.Writer
}

// NewRenderer builds a Renderer writing to w.
func NewRenderer(w io.Writer, format string, fields []string, noHeader bool, tmpl string) *Renderer {
	return &Renderer{format: format, fields: fields, noHeader: noHeader, tmpl: tmpl, w: w}
}

// Render writes records (a slice of structs) in the configured format.
func (r *Renderer) Render(records any) error {
	rv := reflect.ValueOf(records)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice {
		s := reflect.MakeSlice(reflect.SliceOf(rv.Type()), 1, 1)
		s.Index(0).Set(rv)
		rv = s
	}
	n := rv.Len()
	items := make([]any, n)
	for i := 0; i < n; i++ {
		items[i] = rv.Index(i).Interface()
	}

	if r.tmpl != "" {
		return r.renderTemplate(items)
	}
	switch r.format {
	case "json":
		return r.renderJSON(items)
	case "jsonl":
		return r.renderJSONL(items)
	case "csv":
		return r.renderDelimited(items, ',')
	case "tsv":
		return r.renderDelimited(items, '\t')
	default:
		return r.renderTable(items)
	}
}

func (r *Renderer) renderJSON(items []any) error {
	enc := json.NewEncoder(r.w)
	enc.SetIndent("", "  ")
	if len(items) == 1 {
		return enc.Encode(items[0])
	}
	return enc.Encode(items)
}

func (r *Renderer) renderJSONL(items []any) error {
	enc := json.NewEncoder(r.w)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			return err
		}
	}
	return nil
}

func (r *Renderer) renderTemplate(items []any) error {
	t, err := template.New("row").Parse(r.tmpl)
	if err != nil {
		return fmt.Errorf("parse --template: %w", err)
	}
	for _, it := range items {
		if err := t.Execute(r.w, toAnyMap(it)); err != nil {
			return err
		}
		fmt.Fprintln(r.w)
	}
	return nil
}

func (r *Renderer) renderTable(items []any) error {
	if len(items) == 0 {
		return nil
	}
	cols := r.columns(items)
	tw := tabwriter.NewWriter(r.w, 0, 4, 2, ' ', 0)
	if !r.noHeader {
		fmt.Fprintln(tw, strings.Join(upperAll(cols), "\t"))
	}
	for _, it := range items {
		m := toMap(it)
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = truncate(m[c], 60)
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

func (r *Renderer) renderDelimited(items []any, comma rune) error {
	if len(items) == 0 {
		return nil
	}
	cols := r.columns(items)
	cw := csv.NewWriter(r.w)
	cw.Comma = comma
	if !r.noHeader {
		if err := cw.Write(cols); err != nil {
			return err
		}
	}
	for _, it := range items {
		m := toMap(it)
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = m[c]
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func (r *Renderer) columns(items []any) []string {
	if len(r.fields) > 0 {
		return r.fields
	}
	if len(items) == 0 {
		return nil
	}
	return structJSONKeys(items[0])
}

func toAnyMap(v any) any {
	data, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return v
	}
	return m
}

func toMap(v any) map[string]string {
	out := map[string]string{}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return out
	}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" {
			continue
		}
		key := jsonKey(f)
		if key == "-" {
			continue
		}
		fv := rv.Field(i)
		out[key] = fmt.Sprintf("%v", fv.Interface())
	}
	return out
}

func structJSONKeys(v any) []string {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	rt := rv.Type()
	var keys []string
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" {
			continue
		}
		key := jsonKey(f)
		if key == "-" {
			continue
		}
		keys = append(keys, key)
	}
	return keys
}

func jsonKey(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		return f.Name
	}
	name := strings.Split(tag, ",")[0]
	if name == "" {
		return f.Name
	}
	return name
}

func upperAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strings.ToUpper(s)
	}
	return out
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "..."
}
