// Package jsonio centralises JSON / NDJSON output and field-mask projection
// for the nostos CLI.
package jsonio

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// EncodePretty writes v as pretty-printed JSON to w with a trailing newline.
func EncodePretty(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// EncodeNDJSON writes each element of slice as a single JSON line.
// Accepts []T via reflection.
func EncodeNDJSON(w io.Writer, slice any) error {
	rv := reflect.ValueOf(slice)
	if rv.Kind() != reflect.Slice {
		return fmt.Errorf("EncodeNDJSON: not a slice (got %T)", slice)
	}
	for i := 0; i < rv.Len(); i++ {
		b, err := json.Marshal(rv.Index(i).Interface())
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		if _, err := w.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return nil
}

// ToMap marshals v through JSON to a map[string]any. Handy as the input
// to ProjectFields.
func ToMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ProjectFields returns a copy of m containing only keys in fields, in the
// order they appear in fields. If fields is empty, m is returned as-is.
func ProjectFields(m map[string]any, fields []string) map[string]any {
	if len(fields) == 0 {
		return m
	}
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		if v, ok := m[f]; ok {
			out[f] = v
		}
	}
	return out
}

// FieldsFromStruct returns the list of JSON field names for v.
// Nested anonymous structs are flattened one level.
func FieldsFromStruct(v any) []string {
	t := reflect.TypeOf(v)
	if t == nil {
		return nil
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	out := []string{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = f.Name
		}
		out = append(out, name)
	}
	return out
}

// ProjectSlice marshals slice through JSON, projects each element to fields,
// and returns the resulting []map[string]any.
func ProjectSlice(slice any, fields []string) ([]map[string]any, error) {
	rv := reflect.ValueOf(slice)
	if rv.Kind() != reflect.Slice {
		return nil, fmt.Errorf("ProjectSlice: not a slice (got %T)", slice)
	}
	out := make([]map[string]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		m, err := ToMap(rv.Index(i).Interface())
		if err != nil {
			return nil, err
		}
		out = append(out, ProjectFields(m, fields))
	}
	return out, nil
}
