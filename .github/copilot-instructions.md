This file is generated from AGENTS.md files. Do not edit it directly. Instead,
edit the AGENTS.md files and run `script/generate` to regenerate this file.

<!--- start AGENTS.md --->
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
    - `extractSelectionFields()` recursively walks selection sets to build response type structures, creating nested structs for object fields
4. **Collect referenced types** — `collectInputTypes()` and `collectEnumTypes()` recursively find all input object and enum types referenced by operation variables and response fields
5. **Generate files** — Execute templates with operation data, then format with goimports and gofumpt
6. **Write helpers** — Embed `helpers/helpers.go` with the package name replaced

### Generated Code Pattern

For each GraphQL operation, the generator creates:

1. A `{OperationName}Variables` struct with JSON tags matching GraphQL variable names
2. A `{OperationName}Response` struct shaped by the selection set (with nested structs for object fields)
3. An `expectResponses` field in TestHandler (e.g., `getUserExpectResponses`)
4. An `Expect{OperationName}` method on TestHandler that accepts a variables struct and returns a `{OperationName}Expectation` builder
5. Builder methods: `Respond(data)`, `RespondError(errors...)`, `Handle(fn)`
6. A case in testServer's `ServeHTTP` switch that unmarshals variables and calls `getResponse()`

The generator uses a **fluent builder API pattern** where Expect methods return a builder, and you chain a response method:

```go
// Query with variables
handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Respond(usertest.GetUserResponse{
    User: <!--- start AGENTS.md --->
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
    - `extractSelectionFields()` recursively walks selection sets to build response type structures, creating nested structs for object fields
4. **Collect referenced types** — `collectInputTypes()` and `collectEnumTypes()` recursively find all input object and enum types referenced by operation variables and response fields
5. **Generate files** — Execute templates with operation data, then format with goimports and gofumpt
6. **Write helpers** — Embed `helpers/helpers.go` with the package name replaced

### Generated Code Pattern

For each GraphQL operation, the generator creates:

1. A `{OperationName}Variables` struct with JSON tags matching GraphQL variable names
2. A `{OperationName}Response` struct shaped by the selection set (with nested structs for object fields)
3. An `expectResponses` field in TestHandler (e.g., `getUserExpectResponses`)
4. An `Expect{OperationName}` method on TestHandler that accepts a variables struct and returns a `{OperationName}Expectation` builder
5. Builder methods: `Respond(data)`, `RespondError(errors...)`, `Handle(fn)`
6. A case in testServer's `ServeHTTP` switch that unmarshals variables and calls `getResponse()`

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
- **Options placement**: Options like `Times()` are passed to the Expect method, not the Respond method, for cleaner syntax: `ExpectGetUser(vars, Times(3)).Respond(...)`
- **Custom handlers via Handle() method**: Each builder generates a `Handle()` method that accepts `func(VariablesType, http.ResponseWriter)`, providing full control over HTTP responses
- **GraphQL error support**: `RespondError()` returns standard GraphQL error responses with message, path, and extensions fields
- **FIFO expectation matching**: First matching expectation with remaining times is used
- **Expectation tracking**: Each expectation tracks remaining invocations via `times` counter
- **Automatic verification**: Cleanup functions verify all expectations were fully consumed
- **Embedded helpers**: The helpers package is embedded into generated code (with package name replaced) so generated packages have zero runtime dependencies beyond the standard library
- **No runtime dependency on gqlparser**: gqlparser is only used at generation time to parse schemas and operations; generated code has no external dependencies
<!--- end AGENTS.md --->usertest.GetUserResponseUser{ID: "1", Name: "Alice"},
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
- **Options placement**: Options like `Times()` are passed to the Expect method, not the Respond method, for cleaner syntax: `ExpectGetUser(vars, Times(3)).Respond(...)`
- **Custom handlers via Handle() method**: Each builder generates a `Handle()` method that accepts `func(VariablesType, http.ResponseWriter)`, providing full control over HTTP responses
- **GraphQL error support**: `RespondError()` returns standard GraphQL error responses with message, path, and extensions fields
- **FIFO expectation matching**: First matching expectation with remaining times is used
- **Expectation tracking**: Each expectation tracks remaining invocations via `times` counter
- **Automatic verification**: Cleanup functions verify all expectations were fully consumed
- **Embedded helpers**: The helpers package is embedded into generated code (with package name replaced) so generated packages have zero runtime dependencies beyond the standard library
- **No runtime dependency on gqlparser**: gqlparser is only used at generation time to parse schemas and operations; generated code has no external dependencies
<!--- end AGENTS.md --->
