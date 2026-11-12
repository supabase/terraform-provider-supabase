// Command mergespec merges several OpenAPI documents, such as the Management
// API v1 and v2 documents, into one document that oapi-codegen can consume.
//
// Usage: mergespec OUT.yaml IN.yaml [IN.yaml...]
//
// Paths, component schemas and security schemes are unioned. An entry that
// appears in more than one document must be identical everywhere. Operation
// ids must be unique and start with the path's leading segment, so v1-... ids
// only appear under /v1/... and v2-... ids under /v2/.... The first document
// supplies the openapi version and info.version; info.title is normalised and
// servers and tags are dropped because the generator does not use them.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	os.Exit(realMain(os.Args[1:], os.Stderr))
}

// realMain runs the command for args (without the program name) and returns
// the exit code, so main stays a one-liner and argument handling is testable.
func realMain(args []string, stderr io.Writer) int {
	if len(args) < 3 {
		fmt.Fprintln(stderr, "usage: mergespec OUT.yaml IN.yaml [IN.yaml...]")
		return 2
	}
	if err := run(args[0], args[1:]); err != nil {
		fmt.Fprintln(stderr, "mergespec:", err)
		return 1
	}
	return 0
}

// run reads the input documents, merges them and writes the result to out.
func run(out string, inputs []string) error {
	docs := make([]document, 0, len(inputs))
	for _, path := range inputs {
		raw, err := os.ReadFile(filepath.Clean(path)) //nolint:gosec // G703: paths come from the go:generate directive, not user input.
		if err != nil {
			return err
		}
		var root map[string]any
		if err := yaml.Unmarshal(raw, &root); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		docs = append(docs, document{name: path, root: root})
	}
	merged, err := merge(docs)
	if err != nil {
		return err
	}
	encoded, err := yaml.Marshal(merged)
	if err != nil {
		return err
	}
	return os.WriteFile(out, encoded, 0o600) //nolint:gosec // G703: see above; the output path is the directive's too.
}

// document is one parsed OpenAPI document and the name it is reported under.
type document struct {
	name string
	root map[string]any
}

// httpMethods are the path item keys that hold operations; everything else at
// that level (parameters, summary, servers) is not an operation.
var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

// merge unions the paths, component schemas and security schemes of docs into
// a single OpenAPI document.
func merge(docs []document) (map[string]any, error) {
	if len(docs) == 0 {
		return nil, errors.New("no documents to merge")
	}
	paths := map[string]any{}
	schemas := map[string]any{}
	security := map[string]any{}
	for _, doc := range docs {
		if err := mergeInto(paths, asMap(doc.root["paths"]), doc.name, "paths"); err != nil {
			return nil, err
		}
		components := asMap(doc.root["components"])
		if err := mergeInto(schemas, asMap(components["schemas"]), doc.name, "components.schemas"); err != nil {
			return nil, err
		}
		if err := mergeInto(security, asMap(components["securitySchemes"]), doc.name, "components.securitySchemes"); err != nil {
			return nil, err
		}
	}
	if err := validateOperations(paths); err != nil {
		return nil, err
	}
	first := docs[0].root
	return map[string]any{
		"openapi": first["openapi"],
		"info": map[string]any{
			"title":   "Supabase API",
			"version": asMap(first["info"])["version"],
		},
		"paths": paths,
		"components": map[string]any{
			"schemas":         schemas,
			"securitySchemes": security,
		},
	}, nil
}

// mergeInto copies src into dst. A key present in both must map to equal
// values; anything else is a conflict between documents.
func mergeInto(dst, src map[string]any, name, what string) error {
	for _, key := range sortedKeys(src) {
		value := src[key]
		if existing, ok := dst[key]; ok && !reflect.DeepEqual(existing, value) {
			return fmt.Errorf("%s: %s/%s conflicts with an earlier document", name, what, key)
		}
		dst[key] = value
	}
	return nil
}

// validateOperations checks that every operation has an id, that ids are
// unique across the merged document, and that each id starts with its path's
// leading segment followed by a dash.
func validateOperations(paths map[string]any) error {
	seen := map[string]string{}
	for _, path := range sortedKeys(paths) {
		item := asMap(paths[path])
		segment, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
		for _, method := range sortedKeys(item) {
			if !httpMethods[method] {
				continue
			}
			where := strings.ToUpper(method) + " " + path
			id, _ := asMap(item[method])["operationId"].(string)
			if id == "" {
				return fmt.Errorf("%s has no operationId", where)
			}
			if previous, dup := seen[id]; dup {
				return fmt.Errorf("duplicate operationId %q: %s and %s", id, previous, where)
			}
			seen[id] = where
			if !strings.HasPrefix(id, segment+"-") {
				return fmt.Errorf("%s: operationId %q does not start with the path's leading segment %q", where, id, segment)
			}
		}
	}
	return nil
}

// asMap returns v as a map, or nil when v is absent or not a map, so callers
// can range over optional sections without checking first.
func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// sortedKeys returns the keys of m in sorted order, for deterministic
// traversal and error messages.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
