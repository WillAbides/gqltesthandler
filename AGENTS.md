# AGENTS.md

This file provides guidance to AI Agents when working with code in this repository.

## Overview

gqltesthandler is a code generator that creates mock GraphQL test handlers from a schema and predefined operations. It
generates a typed Go test server that lets you set expectations on GraphQL operations and verify they are called
correctly in your tests.

The target workflow: users generate a typed GraphQL client with [genqlient](https://github.com/Khan/genqlient)
(Khan Academy), then use gqltesthandler to generate a mock server for testing that client. No runtime dependency on
gqlgen or genqlient — only `github.com/vektah/gqlparser/v2` at generation time.

## Development Commands

### Testing

- Run all tests: `./script/test` (or `go test -race -covermode=atomic ./...`)
- Run specific package tests: `go test ./path/to/package`
- Run single test: `go test -run TestName ./path/to/package`

#### Snapshot Testing

The handlergen package uses snapshot testing to verify generated code output. Tests generate code to a temporary directory and compare it against reference snapshots stored in `testdata/*/generated/` directories.

**To regenerate snapshots**, set the `UPDATE_SNAPS` environment variable when running tests:
- **Regenerate all snapshots**: `UPDATE_SNAPS=true go test ./internal/handlergen`
- **Regenerate specific test snapshots**: `UPDATE_SNAPS=true go test ./internal/handlergen -run TestRun/simple_query`

The snapshots are stored in `generated/` subdirectories within each test case's testdata directory (e.g., `testdata/simple_query/generated/`), making the reference code directly viewable in your IDE.

### Code Quality

- Format code: `./script/fmt`
- Run linters: `./script/lint` (or `./bin/golangci-lint run ./...`)

### Go Style Guidelines

- **Never use `else`**: Prefer early returns and guard clauses over `else` blocks. This reduces nesting and improves readability.
  - Good: `if err != nil { return err }` followed by success path
  - Bad: `if err != nil { return err } else { /* success path */ }`

- **No assignments in if conditionals**: Always separate variable assignments from conditional checks.
  - Good: `val, ok := foo[bar]` on one line, then `if ok {` on the next
  - Bad: `if val, ok := foo[bar]; ok {`

### Code Generation

- Generate all code: `./script/generate`
- The generate script runs `go generate ./...` and then `script/update-docs` which:
    - Updates CLI usage output in README.md
    - Updates script descriptions in CONTRIBUTING.md
    - Copies AGENTS.md content into .github/copilot-instructions.md
- **IMPORTANT**: Always run `./script/generate` after updating AGENTS.md to verify that the documentation accurately reflects the current code generation behavior
- **IMPORTANT**: Never edit `.github/copilot-instructions.md` directly - it is generated from AGENTS.md when `./script/generate` is run. All documentation changes should be made to AGENTS.md.

### Building

- Build and run: `./script/gqltesthandler [args]`
- Or use go tools: `go run github.com/willabides/gqltesthandler/cmd/gqltesthandler [args]`

## Architecture

### Core Components

**internal/handlergen/helpers/helpers.go** - Core expectation matching library

- `expectResponses[REQ, RESP]` type manages expected responses for requests
- `expectResponse[REQ, RESP]` type represents a single expected request/response pair
- `keyHash()` creates deterministic hashes using JSON marshaling and FNV-128 hashing
- `expect()` sets up expectations with optional `Times()` and `MinTimes()` options
- `getResponse()` matches incoming requests to expectations using hash matching
- Thread-safe via `sync.Mutex`

**internal/handlergen** - Code generation engine

- Reads GraphQL schema (`.graphqls`) via gqlparser and operations file (`.graphql`) with named queries/mutations
- Generates four files in output directory:
    - `types_gen.go`: Go types for variables, responses (shaped by selection sets), input objects, enums, and GraphQLError
    - `handler.go`: TestHandler with Expect methods that return builder types; builders have `Respond()`, `RespondError()`, and `Handle()` methods
    - `server.go`: HTTP POST handler that parses `{"query", "operationName", "variables"}` requests, dispatches by operationName, matches expectations via variable hashing
    - `helpers.go`: Embedded copy of the helpers package (TB interface, ExpectOption, expectResponses, expectResponse)
- Uses Go `text/template` with three templates: `handler.tmpl`, `server.tmpl`, `types.tmpl`
- Runs `goimports` and `gofumpt` on all generated files
- Detects Go package name from output directory path

**cmd/gqltesthandler** - CLI entry point

- Uses kong for CLI parsing
- Takes: `--schema` (GraphQL schema file), `--operations` (GraphQL operations file), `-o` (output directory)
- Delegates to `handlergen.Run()`

### Code Generation Pipeline

1. **Parse schema** — `loadSchema()` uses `gqlparser.LoadSchema()` to parse the `.graphqls` file
2. **Parse operations** — `loadOperations()` uses `gqlparser.LoadQuery()` to parse the `.graphql` file; validates all operations are named
3. **Extract operation data** — For each operation:
    - `extractVariables()` maps GraphQL variable types to Go types (String→string, Int→int, Float→float64, Boolean→bool, ID→string, enums and input objects by name, custom scalars→any)
    - `extractSelectionFields()` recursively walks selection sets to build response type structures, creating nested structs for object fields and, for interface/union fields with at least one narrowing type condition, a discriminator interface plus one struct per concrete type reached by a narrowing type condition (see Interfaces and Unions). Generated Go field names, JSON tags, dedupe, and nested struct names are keyed by each selection's **response key** (`sel.Alias` when present, otherwise `sel.Name`); the underlying `sel.Name`/schema definition is still used for type lookup. It returns an error when two distinct response keys would map to the same Go field name.
4. **Collect referenced types** — `collectInputTypes()` and `collectEnumTypes()` recursively find all input object and enum types referenced by operation variables and response fields
5. **Generate files** — Execute templates with operation data, then format with goimports and gofumpt
6. **Write helpers** — Embed `helpers/helpers.go` with the package name replaced

### Generated Code Pattern

For each GraphQL operation, the generator creates:

1. A `{OperationName}Variables` struct with JSON tags matching GraphQL variable names
2. A `{OperationName}Response` struct shaped by the selection set (with nested structs for object fields, and a discriminator interface + per-concrete-type variant structs for interface/union fields with two or more type conditions — see Interfaces and Unions)
3. An `expectResponses` field in TestHandler (e.g., `getUserExpectResponses`)
4. An `Expect{OperationName}` method on TestHandler that accepts a variables struct and returns a `{OperationName}Expectation` builder
5. Builder methods on the expectation: `Respond(data)`, `RespondError(errors...)`, `Handle(fn)`
6. A `Default{OperationName}` method that returns the same `{OperationName}Expectation` builder (with an internal `isDefault` flag) for the per-operation default responder. The builder routes `Respond` / `RespondError` / `Handle` to the default slot instead of registering an expectation.
7. A variadic `Reset{OperationName}(vars ...{OperationName}Variables)` method. Zero args wipes all expectations and the default; non-zero args wipe only matching expectations and leave the default + non-matches alone.
8. A case in testServer's `ServeHTTP` switch that unmarshals variables and calls `getResponse()`

In addition, TestHandler has a top-level `Reset()` method that wipes all operations at once.

The generator uses a **fluent builder API pattern** where Expect methods return a builder, and you chain a response method:

```go
// Query with variables
handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Respond(usertest.GetUserResponse{
    User: &usertest.GetUserResponseUser{ID: "1", Name: "Alice"},
})

// Query without variables
handler.ExpectListUsers(usertest.ListUsersVariables{}).Respond(listResponse)

// Mutation with input object
handler.ExpectCreateUser(usertest.CreateUserVariables{
    Input: usertest.CreateUserInput{Name: "Bob", Email: "bob@example.com"},
}).Respond(createResponse)

// Error response
handler.ExpectGetUser(usertest.GetUserVariables{ID: "999"}).RespondError(
    usertest.GraphQLError{Message: "user not found"},
)

// Custom handler with full HTTP control
handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Handle(
    func(vars usertest.GetUserVariables, w http.ResponseWriter) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]any{
            "data": map[string]any{"user": map[string]any{"id": vars.ID, "name": "Dynamic"}},
        })
    },
)
```

Example with options:

```go
// Expect the same call 3 times with Times option
handler.ExpectGetUser(vars, usertest.Times(3)).Respond(response)

// MinTimes(0) acts as a stub — accepts any number of calls including zero
handler.ExpectGetUser(vars, usertest.MinTimes(0)).Respond(response)
```

### Default Responders

Each operation gets a `Default{OperationName}` method that returns a builder for registering a fallback response. Defaults match only when no concrete `Expect*` expectation matches the incoming request. They are infinitely callable, never tracked in counts, and never fail at cleanup.

```go
// Static default
handler.DefaultGetUser().Respond(usertest.GetUserResponse{
    User: &usertest.GetUserResponseUser{Name: "stub"},
})

// Dynamic default — receives the actual variables from the incoming request
handler.DefaultGetUser().Handle(func(vars usertest.GetUserVariables, w http.ResponseWriter) {
    // build a response based on vars.ID, etc.
})

// Default error
handler.DefaultGetUser().RespondError(usertest.GraphQLError{Message: "not found"})
```

A concrete `Expect*` always wins over the default, regardless of registration order. Calling `Default<OpName>` a second time replaces the previously registered default. Defaults accept no `ExpectOption` — Times/MinTimes are meaningless for an infinitely callable fallback.

### Reset Methods

- `handler.Reset{OperationName}(vars ...{OperationName}Variables)` is variadic. With no arguments it wipes registered expectations and the default for one operation. With one or more `vars` it wipes only the registered expectations whose key matches one of the provided variables, leaves the default and non-matching expectations untouched, and treats a `vars` entry that matches nothing as a silent no-op. Matching uses the same key hash as expectation dispatch.
- `handler.Reset()` wipes everything across all operations.

Both clear pending cleanup-error state for the wiped expectations so previously-registered `Times(N)` expectations do not fire `t.Errorf` at cleanup after a reset.

The variadic targeted form supports layered fixture patterns — one layer pre-registers `MinTimes(0)` stubs derived from seed data, a downstream layer calls `Reset<Op>(vars)` to drop a single fixture cell and then re-registers a stricter assertion for that key without disturbing other entries or the default.

**Fail-on-used rule:** Both `Reset()` and `Reset<OpName>()` (with or without args) record an error via `tb.Errorf` and leave state untouched if the handler has served any request — not just requests matching the operation being reset. This conservative rule avoids invalidating in-flight assertions or response state.

Adding new expectations or defaults via `Expect*` / `Default*` after requests have flowed remains legal. Only *removal* (Reset) triggers the fail-on-used guard.

### Type Mapping

GraphQL types are mapped to Go types as follows:

| GraphQL Type | Go Type |
|---|---|
| `String`, `ID` | `string` |
| `Int` | `int` |
| `Float` | `float64` |
| `Boolean` | `bool` |
| Enums | Named `string` type with constants |
| Input objects | Structs with JSON tags |
| Custom scalars | `any` |
| Nullable types | Pointer (`*T`) |
| List types | Slice (`[]T`) |

Response types are shaped by the operation's selection set, not the full schema type. Nested object fields produce nested structs named `{OperationName}Response{FieldName}`.

### Field Aliases (Response Keys)

GraphQL responses are keyed by a selection's **response key** — its alias when one is present, otherwise the field name. The generator uses the response key for the generated Go field name, its JSON tag, duplicate/merge detection, and nested struct names, while still using the underlying field name (`sel.Name`) and schema definition for type lookup and GraphQL semantics.

- Scalar alias: `handle: login` → `Handle string `json:"handle"``.
- Object alias: `profile: user { id }` → field `Profile` of nested type `{OperationName}ResponseProfile` with json tag `profile`. Nested type names derive from the response key, so aliases produce intuitive names and avoid clashing with the un-aliased field's nested type.
- Aliased `__typename`: `kind: __typename` → `Kind string `json:"kind"`` (a normal selected response key). Un-aliased `__typename` still maps to the exported Go field `Typename` with `json:"__typename"`.
- Fragment spreads and inline fragments merge/dedupe by response key, consistent with GraphQL response semantics — the same response key contributed by multiple fragments or union members collapses to one field.

The Go field name is derived from the response key by `exportedName` (uppercase first rune, plus the `id` → `ID` and `__typename` → `Typename` special cases). This mapping does **not** normalize snake_case to camelCase, so `foo_bar` and `fooBar` do not collide, but case-only differences do (e.g. `name` and `Name`). When two distinct response keys in the same selection-set level map to the same Go field name, generation fails with an error rather than emitting a struct with duplicate fields.

### Interfaces and Unions

Interface and union fields are modeled like [genqlient](https://github.com/Khan/genqlient): instead of flattening every inline fragment's fields into one struct, the generator emits a discriminator interface plus one struct per concrete type reached by a narrowing type condition. `buildAbstractField()` (in `internal/handlergen/handlergen.go`) implements this and `extractSelectionFields()` calls it for any abstract field, recursing for nested abstract fields. Only concrete types reached by a narrowing fragment get a variant struct — a possible type for which the operation selected only shared fields has no variant, so the typed `Respond` API cannot build that concrete response in polymorphic mode (use `Handle()` or add a type condition for it).

**Threshold (any narrowing fragment).** The polymorphic shape is used when the selection contains at least one type-conditioned fragment that *narrows* the abstract type to a proper subset of its possible types (`cond ⊊ applicable`). A single narrowing fragment is enough, because genqlient injects `__typename` for any such selection. When no fragment narrows — a shared-only selection like `node { id }` or `node { ... on Node { id } }` — `buildAbstractField` returns false and the caller emits a single flat struct instead of one struct per possible type. (Empirically, genqlient *does* inject `__typename` and discriminate even those shared-only selections — `go run github.com/Khan/genqlient` on `node { id }` generates a `__typename`-keyed interface — so the flat shape is a deliberate ergonomics choice to avoid per-type explosion, not a claim that genqlient collapses them. To stay genqlient-compatible without the explosion, the flat struct still gets a typed, settable `__typename` discriminator — see *Flat-struct discriminator* below.)

**Field membership (possible-type intersection).** Variant fields follow genqlient's rule rather than treating abstract fragments as shared:

- **Shared fields** = field selections made *directly* on the abstract field (excluding `__typename`), plus fields from a fragment whose type condition does **not** narrow the current applicable set (`cond == applicable`, e.g. `... on Node` under `Node`, or a spread on the same abstract type). These are promoted onto every variant and never trigger discrimination on their own.
- **Variant fields** = fields from *narrowing* type conditions (`cond ⊊ applicable`), attached to the concrete types in `parentPossibleTypes ∩ conditionPossibleTypes`. An object condition (`... on User`) adds fields to just that variant; an interface/union condition (`... on SomeInterface`) expands to the concrete implementors within the parent's possible types — never onto sibling variants outside it. The same rule recurses for nested abstract fields.

`possibleConcreteTypes()` wraps `schema.GetPossibleTypes()` to resolve a type condition to its concrete object types in schema declaration order (deterministic snapshots). The variant set and field lists are computed by a single `walk` over the selection tree that threads the currently-applicable concrete-type set through each fragment; a fragment whose intersection equals the applicable set (`sameTypeSet`) recurses in the current scope without recording a variant, so non-narrowing fragments contribute shared fields instead of exploding into one variant per possible type.

Variant field names, JSON tags, and nested type names are keyed by **response key** just like every other selection, so aliases inside a type condition (`handle: login`) flow through to the variant struct, and the variant's own response-key collision check shares the alias machinery.

**Generated shape.** For an abstract field named `search` on operation `Search`:

- An interface `SearchResponseSearch` with a single unexported sentinel method `isSearchResponseSearch()`.
- One struct per concrete type, e.g. `SearchResponseSearchUser`, each implementing the sentinel via a value receiver.
- The abstract field on the response struct is typed as the bare interface (`SearchResponseSearch`), not a pointer — a `nil` interface marshals to JSON `null`. List/slice nesting is preserved (`[]SearchResponseSearch`). `wrapInterfaceGoType()` handles this wrapping.

**`__typename` injection is unconditional.** Each variant *always* gets a `MarshalJSON` that injects the flat `__typename` discriminator (`{"__typename":"User",...}`), and the server writes that payload as-is — injection does **not** depend on the incoming request query. gqltesthandler targets genqlient-style generated clients: genqlient auto-injects `__typename` into the wire request for abstract types even when the `.graphql` operation file omits it (and requires it back to discriminate), so the generator infers that effective query shape and always returns the discriminator. Choosing the concrete variant struct already determines the runtime type, so fixture authors never set `__typename` themselves — they pick the variant. gqlgenc's strict `graphqljson` decoder *rejects* a response field it did not select, so a gqlgenc-style client must select `__typename` on its *narrowing* abstract selections to consume the variant response (several `example/userapi` operations select `__typename` on their abstract fields so the strict client decodes the injected discriminator); gqlgenc strict no-extra-field compatibility is explicitly **not** the controlling policy for abstract selections. A `Handle()`/raw responder writes the response itself and bypasses injection entirely, so it owns its `__typename` behavior. There is no request-query inspection or serve-time stripping — the earlier request-driven design (a lexical `__typename` scan plus a marshal/strip/re-encode round-trip) was removed in favor of this simpler always-inject policy.

**Flat-struct discriminator (`withFlatTypename`).** A shared-only abstract selection stays flat (no variants), but because genqlient still injects and requires `__typename`, the flat struct gets a typed discriminator: a field `Typename <Struct>Typename`, a generated `type <Struct>Typename string`, and one constant per possible concrete type (schema order). `withFlatTypename()` injects the field when an abstract field falls back to the flat path, and `collectTypenameTypes()` walks the response tree to emit each `type` + `const` block (reusing `enumTypeData`). The JSON tag depends on whether the operation selected `__typename`:

- **Operation omits `__typename`** (synthesized for genqlient compatibility): the field is prepended and tagged `json:"__typename,omitempty"`. A fixture that leaves it unset writes no `__typename` (so a strict gqlgenc decoder that did not select it still accepts the response), while a genqlient-oriented fixture sets `Typename: <Struct>TypenameUser` to emit the discriminator.
- **Operation explicitly selects `__typename`**: the typed field replaces the selected `__typename` in place and keeps a plain `json:"__typename"` (no `omitempty`). An explicitly requested field is always present, so omitting it when unset would not be GraphQL-shaped.

A fixture can also cast a string literal, `<Struct>Typename("User")`, which is handy for a broad interface whose constant list is long (`example/github`'s `author: Actor` selections generate five constants each). Concrete (non-abstract) object selections are unaffected — their `__typename` stays a plain `string` field present only when explicitly selected. Keeping the flat struct rather than discriminating shared-only selections (as genqlient does, with a full interface plus a variant per possible type) is a deliberate ergonomics divergence.

**`Typename` field conflict validation.** Both the synthesized/explicit `__typename` discriminator and a real selected field named `typename`/`Typename` map to the Go field `Typename` (`exportedName` upcases the first rune and special-cases `__typename`). In a *flat* struct those would be sibling fields with the same Go name, which does not compile. Two collision paths are covered, both with the shared `response keys %q and %q both map to Go field name %q in %s` message: an *explicit* `__typename` (or any alias) colliding with another response key is caught during extraction by `extractSelectionFields`'s response-key check (the same `goNames` map the alias machinery uses), and the *synthesized* flat discriminator added by `withFlatTypename` re-checks against the struct's existing fields before injecting. Both fail generation rather than emitting code that does not build. The *polymorphic variant* case is intentionally **not** flagged and is safe: a variant injects `__typename` through its `MarshalJSON` alias embed (an outer `Typename string json:"__typename"` wrapping `type alias`), not through a struct field, so a real `typename` field on the variant serializes under a distinct JSON key (`{"__typename":"User","typename":"...",...}`).

### Testing Pattern

Tests use httptest.NewServer with the TestHandler:

1. Create TestHandler with `NewTestHandler(t)`
2. Start `httptest.NewServer` with the handler
3. Create GraphQL client pointing to test server
4. Set expectations on the handler
5. Make client calls — they hit the mock server
6. Cleanup verifies all expectations were met

### Key Design Decisions

- **Fluent builder API pattern**: Expect methods return a builder with `Respond()`, `RespondError()`, and `Handle()` methods
- **Variables struct argument**: Expect methods accept a typed variables struct rather than expanded individual parameters, since GraphQL operations have a uniform `{"operationName", "variables"}` structure
- **Response shaped by selection set**: Generated response types only include fields that the operation actually selects, matching what a real GraphQL server would return
- **Nested response structs**: Object fields in selections generate nested structs (e.g., `GetUserResponseUser`) rather than reusing schema-level types, ensuring response shapes match the operation
- **Aliases keyed by response key**: Generated Go field names, JSON tags, dedupe, and nested struct names use each selection's response key (`sel.Alias` when present, else `sel.Name`), matching how GraphQL keys response objects; the underlying field name/schema definition still drives type lookup. Two distinct response keys that would map to the same Go field name are a generation-time error.
- **Polymorphic interface/union modeling**: Abstract fields with at least one *narrowing* type condition generate a discriminator interface plus per-concrete-type variant structs (genqlient-style), instead of flattening every fragment into one struct — making impossible field combinations unrepresentable. Variant field membership uses possible-type intersection (`parentPossibleTypes ∩ conditionPossibleTypes`), so a field under an interface fragment never leaks onto sibling variants; fields selected directly on the abstract field — and fragments that do not narrow it (`cond == applicable`) — are shared across every variant. `__typename` injection (via generated `MarshalJSON`) is **unconditional**: variants always inject it and the server writes the payload as-is, with no request-query inspection or serve-time stripping. gqltesthandler targets genqlient-style clients, which auto-inject `__typename` into the wire request for abstract types and require it back; a strict gqlgenc-style client must select `__typename` on its abstract selections to decode the response, so gqlgenc strict no-extra-field compatibility is not the controlling policy. Custom `Handle()` responders bypass injection and own their own `__typename` behavior. The threshold is "any narrowing fragment → polymorphic": a single narrowing fragment is enough (genqlient injects `__typename` for those), while a shared-only selection (no narrowing fragment) stays a single flat struct to avoid expanding to one struct per possible type. To stay genqlient-compatible without that explosion, a flat abstract struct still carries a typed discriminator — `Typename <Struct>Typename` plus a generated `type <Struct>Typename string` and one constant per possible concrete type. It is tagged `json:"__typename,omitempty"` when synthesized (operation omitted `__typename`) so strict gqlgenc decoders that did not select it still decode, and plain `json:"__typename"` (always present) when the operation explicitly selected `__typename`. A real `typename`/`Typename` selection colliding with this discriminator in the same flat struct is rejected at generation time — explicit collisions by the extraction-time response-key check, synthesized ones by `withFlatTypename` — with the shared `both map to Go field name` message; the polymorphic variant case is safe because the variant injects `__typename` via a `MarshalJSON` alias embed under a distinct JSON key. Keeping the flat struct rather than discriminating shared-only selections is a deliberate ergonomics divergence from genqlient. See Interfaces and Unions.
- **Options placement**: Options like `Times()` are passed to the Expect method, not the Respond method, for cleaner syntax: `ExpectGetUser(vars, Times(3)).Respond(...)`
- **Custom handlers via Handle() method**: Each builder generates a `Handle()` method that accepts `func(VariablesType, http.ResponseWriter)`, providing full control over HTTP responses. The function always receives the variables from the incoming request. For an `Expect*` match those variables are equal to the registered variables by construction (matching requires variable equality); for a `Default*` they can be anything the client sent.
- **GraphQL error support**: `RespondError()` returns standard GraphQL error responses with message, path, and extensions fields
- **FIFO expectation matching**: First matching concrete expectation with remaining times is used; the per-operation default is consulted only when no concrete match exists
- **Strict-by-default unmatched-operation handling**: If a request hits a *known* operation (one present in the operations file the generator was run against) and neither an `Expect*` nor a `Default*` matches it, the server calls `t.Errorf("no expectation found ...")` and returns a GraphQL error response. This surfaces missed test setup, fixture drift, and stale assertions loudly. Requests naming an *unknown* operation (typo, schema/operations drift) only get back a `"unknown operation: ..."` GraphQL error response — no `t.Errorf` — because no `Default<OpName>` could have been registered for an op the generator never saw. Unknown-operation tests still fail through the test's own assertions on the response. Consumers building stateful fakes that wrap the generated handler should register a `Default<OpName>` per known operation at construction time to opt out of strictness for the ops they want graceful-empty behavior on.
- **Per-operation defaults**: `Default<OpName>` registers a fallback responder that never consumes count and never errors at cleanup. Calling it again replaces the previous default.
- **Reset semantics**: `Reset()` and `Reset<OpName>()` wipe expectations + default and disarm pending cleanup errors. `Reset<OpName>(vars...)` is the targeted variant — wipes only matching expectations, leaves the default and non-matching expectations alone. All Reset variants become no-ops (with `tb.Errorf`) once the handler has served any request, so they're safe to use only during test setup.
- **Expectation tracking**: Each expectation tracks remaining invocations via `times` counter
- **Automatic verification**: Cleanup functions verify all expectations were fully consumed
- **Embedded helpers**: The helpers package is embedded into generated code (with package name replaced) so generated packages have zero runtime dependencies beyond the standard library
- **No runtime dependency on gqlparser**: gqlparser is only used at generation time to parse schemas and operations; generated code has no external dependencies
