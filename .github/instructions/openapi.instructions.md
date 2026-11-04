---
applyTo: "pkg/api/**/*.go,openapi/**"
---

# Bee HTTP API and OpenAPI spec consistency

The OpenAPI spec in `openapi/Swarm.yaml` (shared types in `openapi/SwarmCommon.yaml`) is written by hand. It is the contract for API clients such as bee-js, so it must match the Go handlers in `pkg/api`.

When a pull request changes `pkg/api` without a matching change in `openapi/`, or the other way round, check whether the spec still describes the code. Flag each mismatch with the file and line in both the Go code and the YAML.

## Where the contract lives in the code

- Routes and HTTP methods: `pkg/api/router.go` (`handle(...)` with `jsonhttp.MethodHandler`). The spec documents them in `paths`.
- Path, query and header parameters: anonymous structs decoded with `s.mapStructure(...)`. The `map:"..."` tag is the parameter name and `validate:"required"` makes it required. Header parameters use their HTTP header name (for example `Swarm-Postage-Batch-Id` maps to `swarm-postage-batch-id`).
- Request bodies: structs decoded from JSON, plus raw or multipart uploads.
- Responses: `jsonhttp.OK`, `jsonhttp.Created`, `jsonhttp.BadRequest`, `jsonhttp.NotFound` and friends. The status code and the response struct's `json:"..."` tags define the documented response.

## What to check

1. A new, removed or renamed route or HTTP method is reflected in `paths`.
2. Parameters: name, location (path, query or header), type and required flag match the struct tags. Validation changes such as `min`, `max` or `oneof` that reject previously accepted input are documented.
3. Response bodies: every JSON field name matches the Go `json` tag exactly (Bee uses some lowercase names like `thresholdreceived`; do not suggest camelCase in the spec). Types match too: `*big.Int` is `BigInt` (a string), not `integer`. Fields marked `omitempty` must not be required.
4. Status codes: every status a handler can return (including new `400`, `403` or `404` paths) is listed under `responses`.
5. Response headers set with `w.Header().Set(...)` on success paths are documented.
6. Streaming or WebSocket endpoints describe their real behavior (NDJSON, upgrade) and HTTP method.
7. Reuse existing components in `SwarmCommon.yaml` instead of duplicating schemas. A parameter `$ref` must point to `#/components/parameters/...` directly in the `parameters` list, never inside a `schema`.
8. Changes visible to API clients bump `info.version` in `openapi/Swarm.yaml` (SemVer: breaking is major, additive is minor, fixes are patch).

## What not to flag

- Internal refactors in `pkg/api` that do not change routes, parameters, bodies, status codes or headers.
- Test-only changes (`*_test.go`).
- Style-only YAML suggestions such as missing `operationId` or `license`.

Keep comments short and specific. Suggest the exact YAML change when possible.
