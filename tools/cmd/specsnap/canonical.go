package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// droppedKeys are the keywords the canonical form leaves out.
var droppedKeys = map[string]bool{
	"description": true,
	"summary":     true,
	"title":       true,
	"example":     true,
	"examples":    true,
}

// valueKeys are the keywords whose values are data, kept as they are.
var valueKeys = map[string]bool{
	"default": true,
	"enum":    true,
	"const":   true,
}

// nameKeys are the keywords whose value is an object keyed by names, such as the names of
// properties, paths, schemas, status codes or media types, rather than by keywords.
var nameKeys = map[string]bool{
	"$defs":             true,
	"callbacks":         true,
	"content":           true,
	"definitions":       true,
	"dependentRequired": true,
	"dependentSchemas":  true,
	"encoding":          true,
	"headers":           true,
	"links":             true,
	"mapping":           true,
	"parameters":        true,
	"pathItems":         true,
	"paths":             true,
	"patternProperties": true,
	"properties":        true,
	"requestBodies":     true,
	"responses":         true,
	"schemas":           true,
	"scopes":            true,
	"securitySchemes":   true,
	"variables":         true,
	"webhooks":          true,
}

// httpMethods are the keys of an OpenAPI path item that are operations.
var httpMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// parse decodes a spec, keeping each number as it is written.
func parse(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("it is not a JSON object: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("it has more data after its JSON object")
	}
	return doc, nil
}

// countOperations returns the number of operations in a spec's paths.
func countOperations(doc map[string]any) int {
	paths, _ := doc["paths"].(map[string]any)
	n := 0
	for _, item := range paths {
		operations, _ := item.(map[string]any)
		for _, method := range httpMethods {
			if _, ok := operations[method]; ok {
				n++
			}
		}
	}
	return n
}

// canonical returns a spec in canonical form, as the package doc describes it.
func canonical(doc map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(strip(doc, false)); err != nil {
		return nil, fmt.Errorf("writing the canonical form: %w", err)
	}
	return buf.Bytes(), nil
}

// strip returns a copy of v without the dropped keywords. When names is true, v is an object
// keyed by names, and each of its values is stripped as an object keyed by keywords.
func strip(v any, names bool) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			switch {
			case names:
				out[key] = strip(child, false)
			case droppedKeys[key]:
			case valueKeys[key] || strings.HasPrefix(key, "x-"):
				out[key] = child
			case key == "security":
				out[key] = stripEach(child, true)
			default:
				out[key] = strip(child, nameKeys[key])
			}
		}
		return out
	case []any:
		return stripEach(v, false)
	}
	return v
}

// stripEach strips each item of an array, or v itself when it is not one. A security
// requirement is an object keyed by the names of security schemes, so the items of security are
// stripped with names set.
func stripEach(v any, names bool) any {
	items, ok := v.([]any)
	if !ok {
		return strip(v, names)
	}
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = strip(item, names)
	}
	return out
}

// sha256Hex returns the sha256 of data in hex.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
