package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// operation returns a path item with a single GET operation.
func operation(id string) map[string]any {
	return map[string]any{
		"get": map[string]any{"operationId": id, "responses": map[string]any{"200": map[string]any{"description": "ok"}}},
	}
}

func v1Doc() document {
	return document{name: "v1.yaml", root: map[string]any{
		"openapi": "3.0.0",
		"info":    map[string]any{"title": "Supabase API (v1)", "version": "1.0.0"},
		"servers": []any{map[string]any{"url": "https://api.supabase.com"}},
		"tags":    []any{map[string]any{"name": "Projects"}},
		"paths": map[string]any{
			"/v1/projects": operation("v1-list-all-projects"),
		},
		"components": map[string]any{
			"schemas":         map[string]any{"Project": map[string]any{"type": "object"}},
			"securitySchemes": map[string]any{"bearer": map[string]any{"type": "http", "scheme": "bearer"}},
		},
	}}
}

func v2Doc() document {
	return document{name: "v2.yaml", root: map[string]any{
		"openapi": "3.0.0",
		"info":    map[string]any{"title": "Supabase API (v2)", "version": "2.0.0"},
		"paths": map[string]any{
			"/v2/projects/{ref}/config": operation("v2-get-project-config"),
		},
		"components": map[string]any{
			"schemas":         map[string]any{"ProjectConfig": map[string]any{"type": "object"}},
			"securitySchemes": map[string]any{"bearer": map[string]any{"type": "http", "scheme": "bearer"}},
		},
	}}
}

func TestMerge_UnionsDisjointDocuments(t *testing.T) {
	merged, err := merge([]document{v1Doc(), v2Doc()})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := merged["openapi"]; got != "3.0.0" {
		t.Errorf("openapi = %v, want 3.0.0", got)
	}
	info := asMap(merged["info"])
	if info["title"] != "Supabase API" || info["version"] != "1.0.0" {
		t.Errorf("info = %v, want normalised title and the first document's version", info)
	}
	for _, dropped := range []string{"servers", "tags"} {
		if _, ok := merged[dropped]; ok {
			t.Errorf("%s should be dropped from the merged document", dropped)
		}
	}
	paths := asMap(merged["paths"])
	if len(paths) != 2 {
		t.Errorf("paths = %v, want both documents' paths", sortedKeys(paths))
	}
	components := asMap(merged["components"])
	if schemas := asMap(components["schemas"]); len(schemas) != 2 {
		t.Errorf("schemas = %v, want both documents' schemas", sortedKeys(schemas))
	}
	if security := asMap(components["securitySchemes"]); len(security) != 1 {
		t.Errorf("securitySchemes = %v, want the shared scheme once", sortedKeys(security))
	}
}

func TestMerge_IdenticalDuplicateIsAccepted(t *testing.T) {
	v2 := v2Doc()
	asMap(asMap(v2.root["components"])["schemas"])["Project"] = map[string]any{"type": "object"}
	if _, err := merge([]document{v1Doc(), v2}); err != nil {
		t.Fatalf("identical duplicate schema should merge, got %v", err)
	}
}

func TestMerge_ConflictingSchemaIsRejected(t *testing.T) {
	v2 := v2Doc()
	asMap(asMap(v2.root["components"])["schemas"])["Project"] = map[string]any{"type": "string"}
	_, err := merge([]document{v1Doc(), v2})
	assertErrorContains(t, err, "v2.yaml: components.schemas/Project conflicts")
}

func TestMerge_ConflictingPathIsRejected(t *testing.T) {
	v2 := v2Doc()
	asMap(v2.root["paths"])["/v1/projects"] = operation("v1-list-all-projects-differently")
	_, err := merge([]document{v1Doc(), v2})
	assertErrorContains(t, err, "v2.yaml: paths//v1/projects conflicts")
}

func TestMerge_DuplicateOperationIDIsRejected(t *testing.T) {
	v2 := v2Doc()
	asMap(v2.root["paths"])["/v2/other"] = operation("v2-get-project-config")
	_, err := merge([]document{v1Doc(), v2})
	assertErrorContains(t, err, `duplicate operationId "v2-get-project-config"`)
}

func TestMerge_OperationIDMustMatchLeadingSegment(t *testing.T) {
	v2 := v2Doc()
	asMap(v2.root["paths"])["/v2/other"] = operation("v1-other")
	_, err := merge([]document{v1Doc(), v2})
	assertErrorContains(t, err, `GET /v2/other: operationId "v1-other" does not start with the path's leading segment "v2"`)
}

func TestMerge_MissingOperationIDIsRejected(t *testing.T) {
	v1 := v1Doc()
	asMap(v1.root["paths"])["/v1/anonymous"] = map[string]any{"get": map[string]any{}}
	_, err := merge([]document{v1})
	assertErrorContains(t, err, "GET /v1/anonymous has no operationId")
}

func TestMerge_IgnoresPathLevelKeysThatAreNotOperations(t *testing.T) {
	v1 := v1Doc()
	item := asMap(asMap(v1.root["paths"])["/v1/projects"])
	item["parameters"] = []any{map[string]any{"name": "ref", "in": "path"}}
	item["summary"] = "Projects"
	if _, err := merge([]document{v1}); err != nil {
		t.Fatalf("path-level parameters and summary are not operations, got %v", err)
	}
}

func TestMerge_ToleratesMissingSections(t *testing.T) {
	bare := document{name: "bare.yaml", root: map[string]any{"openapi": "3.0.0"}}
	merged, err := merge([]document{bare})
	if err != nil {
		t.Fatalf("a document without paths or components should merge, got %v", err)
	}
	if len(asMap(merged["paths"])) != 0 {
		t.Errorf("paths = %v, want none", merged["paths"])
	}
}

func TestMerge_NoDocumentsIsRejected(t *testing.T) {
	_, err := merge(nil)
	assertErrorContains(t, err, "no documents to merge")
}

func TestRun_WritesMergedYAML(t *testing.T) {
	dir := t.TempDir()
	v1 := writeYAML(t, filepath.Join(dir, "v1.yaml"), v1Doc().root)
	v2 := writeYAML(t, filepath.Join(dir, "v2.yaml"), v2Doc().root)
	out := filepath.Join(dir, "merged.yaml")

	if err := run(out, []string{v1, v2}); err != nil {
		t.Fatalf("run: %v", err)
	}

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var merged map[string]any
	if err := yaml.Unmarshal(raw, &merged); err != nil {
		t.Fatalf("output is not YAML: %v", err)
	}
	if got := sortedKeys(asMap(merged["paths"])); strings.Join(got, ",") != "/v1/projects,/v2/projects/{ref}/config" {
		t.Errorf("merged paths = %v", got)
	}

	again := filepath.Join(dir, "merged-again.yaml")
	if err := run(again, []string{v1, v2}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	rawAgain, err := os.ReadFile(again)
	if err != nil {
		t.Fatalf("read second output: %v", err)
	}
	if string(raw) != string(rawAgain) {
		t.Error("output is not deterministic across runs")
	}
}

func TestRun_ReportsUnreadableInput(t *testing.T) {
	err := run(filepath.Join(t.TempDir(), "out.yaml"), []string{filepath.Join(t.TempDir(), "missing.yaml")})
	if err == nil {
		t.Fatal("run should fail on a missing input")
	}
}

func TestRun_ReportsInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(broken, []byte("paths: [unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := run(filepath.Join(dir, "out.yaml"), []string{broken})
	assertErrorContains(t, err, "broken.yaml")
}

func TestRun_ReportsMergeErrors(t *testing.T) {
	dir := t.TempDir()
	v1 := writeYAML(t, filepath.Join(dir, "v1.yaml"), v1Doc().root)
	err := run(filepath.Join(dir, "out.yaml"), []string{v1, v1})
	if err != nil {
		t.Fatalf("merging a document with itself should succeed, got %v", err)
	}
	conflicting := v1Doc().root
	asMap(asMap(conflicting["components"])["schemas"])["Project"] = map[string]any{"type": "string"}
	v1b := writeYAML(t, filepath.Join(dir, "v1b.yaml"), conflicting)
	err = run(filepath.Join(dir, "out.yaml"), []string{v1, v1b})
	assertErrorContains(t, err, "components.schemas/Project conflicts")
}

func TestRealMain_UsageWithoutEnoughArguments(t *testing.T) {
	var stderr strings.Builder
	if code := realMain([]string{"out.yaml", "only-one-input.yaml"}, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: mergespec") {
		t.Errorf("stderr = %q, want usage", stderr.String())
	}
}

func TestRealMain_ReportsRunErrors(t *testing.T) {
	dir := t.TempDir()
	var stderr strings.Builder
	code := realMain([]string{filepath.Join(dir, "out.yaml"), filepath.Join(dir, "missing-a.yaml"), filepath.Join(dir, "missing-b.yaml")}, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.HasPrefix(stderr.String(), "mergespec: ") {
		t.Errorf("stderr = %q, want a mergespec-prefixed error", stderr.String())
	}
}

func TestRealMain_Succeeds(t *testing.T) {
	dir := t.TempDir()
	v1 := writeYAML(t, filepath.Join(dir, "v1.yaml"), v1Doc().root)
	v2 := writeYAML(t, filepath.Join(dir, "v2.yaml"), v2Doc().root)
	var stderr strings.Builder
	if code := realMain([]string{filepath.Join(dir, "out.yaml"), v1, v2}, &stderr); code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
}

func writeYAML(t *testing.T, path string, doc map[string]any) string {
	t.Helper()
	raw, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}
