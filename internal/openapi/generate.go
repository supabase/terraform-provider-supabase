// Package openapi holds the Management API OpenAPI snapshots (v1 and v2), the
// overlay and the oapi-codegen configuration that produce internal/gen/api.
//
// Run `go generate ./internal/openapi/...` after changing any of them. The
// snapshots are refreshed with `make update-api-spec`; see README.md.
package openapi

//go:generate go run ./mergespec openapi.merged.yaml v1-openapi.yaml v2-openapi.yaml
//go:generate go tool oapi-codegen -config types.cfg.yaml openapi.merged.yaml
//go:generate go tool oapi-codegen -config client.cfg.yaml openapi.merged.yaml
