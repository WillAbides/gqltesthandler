# gqltesthandler

A code generator that creates mock GraphQL test handlers from a schema and predefined operations. It generates a typed
Go test server that lets you set expectations on GraphQL operations and verify they are called correctly in your tests.

The generated code has type-safe methods for setting expected variables and their corresponding responses. It validates
that all expectations are met during test execution and that no unexpected operations are made.

Designed for use with [genqlient](https://github.com/Khan/genqlient) (Khan Academy's typed GraphQL client generator),
but works with any GraphQL client that sends standard `{"query", "operationName", "variables"}` POST requests.

[userapi_test.go](example/userapi/userapi_test.go) has several examples of how to use the generated handler.

Set `go:generate` directives near where you are already generating your genqlient client. For example:

```
//go:generate go run github.com/Khan/genqlient genqlient.yaml
//go:generate go tool gqltesthandler --schema=schema.graphqls --operations=operations.graphql -o internal/testhandler
```

## Installation

### With `go install`

```shell
go install github.com/willabides/gqltesthandler/cmd/gqltesthandler@latest
```

### With `go get -tool`

```shell
go get -tool github.com/willabides/gqltesthandler/cmd/gqltesthandler
```

### With [bindown](https://github.com/WillAbides/bindown)

```shell
bindown template-source add gqltesthandler https://github.com/WillAbides/gqltesthandler/releases/latest/download/bindown.yaml
bindown dependency add gqltesthandler --source gqltesthandler -y
```

## Usage

The generated TestHandler provides a type-safe way to mock GraphQL operations in your tests.

### Basic Pattern

```go
func TestMyService(t *testing.T) {
    // Create the test handler
    handler := usertest.NewTestHandler(t)
    server := httptest.NewServer(handler)
    defer server.Close()

    // Create your GraphQL client pointing at the mock server
    client := graphql.NewClient(server.URL, http.DefaultClient)

    // Set expectations
    handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Respond(usertest.GetUserResponse{
        User: &usertest.GetUserResponseUser{
            ID:   "1",
            Name: "Alice",
        },
    })

    // Call your service — it hits the mock server
    resp, err := GetUser(t.Context(), client, "1")
    require.NoError(t, err)
    require.Equal(t, "Alice", resp.User.Name)

    // Test automatically verifies all expectations were met
}
```

### Setting Expectations

Each GraphQL operation gets an `Expect{OperationName}` method that returns a builder. Chain `Respond` or
`RespondError` to set the response:

```go
// Successful response
handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Respond(successResponse)

// GraphQL error response
handler.ExpectGetUser(usertest.GetUserVariables{ID: "999"}).RespondError(
    usertest.GraphQLError{Message: "user not found"},
)

// Mutations work the same way
handler.ExpectCreateUser(usertest.CreateUserVariables{
    Input: usertest.CreateUserInput{Name: "Bob", Email: "bob@example.com"},
}).Respond(createResponse)
```

### Controlling Call Counts

Use `Times()` to expect multiple calls with the same variables:

```go
// Expect exactly 3 calls
handler.ExpectGetUser(vars, usertest.Times(3)).Respond(response)
```

Use `MinTimes()` when you want at least N calls:

```go
// Expect at least 2 calls, but allow more
handler.ExpectGetUser(vars, usertest.MinTimes(2)).Respond(response)

// MinTimes(0) acts as a stub — accepts any number of calls including zero
handler.ExpectGetUser(vars, usertest.MinTimes(0)).Respond(response)
```

### Custom Responses

Use `Handle()` for dynamic responses with full HTTP control:

```go
handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Handle(
    func(vars usertest.GetUserVariables, w http.ResponseWriter) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]any{
            "data": map[string]any{
                "user": map[string]any{
                    "id":   vars.ID,
                    "name": "Dynamic-" + vars.ID,
                },
            },
        })
    },
)
```

### Field Aliases

Generated response types follow GraphQL response keys, so aliased selections map
to Go fields named after the alias (with a matching JSON tag). For an operation
like:

```graphql
query GetUser($id: ID!) {
  profile: user(id: $id) {
    handle: login
  }
}
```

the generator emits `Profile` (nested type `GetUserResponseProfile`, json tag
`profile`) containing `Handle string `json:"handle"``. Un-aliased fields are
unchanged. If two different response keys would collide on the same Go field
name, generation fails with an error.

### Interfaces and Unions

When an operation selects an interface or union field with at least one
type-conditioned fragment that *narrows* the abstract type (for example
`... on User` under a `Node` interface), the generator models it the way
[genqlient][genqlient] does: a small discriminator interface plus one struct per
concrete type reached by a narrowing type condition, instead of flattening every
fragment's fields into a single struct. This makes invalid field combinations
unrepresentable — you build a fixture by choosing the concrete variant you want.

Given a union `search: [SearchResult!]!` where `SearchResult = User | Post`:

```go
handler.ExpectSearch(usertest.SearchVariables{Term: "go"}).Respond(usertest.SearchResponse{
    Search: []usertest.SearchResponseSearch{
        usertest.SearchResponseSearchUser{ID: "1", Name: "alice"},
        usertest.SearchResponseSearchPost{ID: "2", Title: "gqltesthandler"},
    },
})
```

`SearchResponseSearch` is an interface implemented by each variant struct, so a
field typed as the interface (or a slice of it) only accepts the generated
variants. Interface-typed fields are emitted as the bare interface (no pointer),
so a `nil` value marshals to JSON `null`.

Variant fields follow each fragment's concrete possible types: a field selected
under `... on SomeInterface` is attached only to the concrete types that
actually implement that interface, never to sibling variants outside it. Fields
selected directly on the abstract field — and fragments whose type condition
does not narrow the abstract type (a spread or `... on` on the same interface or
union) — are shared across every variant rather than triggering discrimination.

**`__typename` is always injected for abstract variants.** Each variant struct
carries a generated `MarshalJSON` that injects the correct `__typename` constant
— you never set it yourself, and choosing the concrete variant struct already
determines the runtime type. The discriminator is emitted unconditionally on
every `Respond` payload, regardless of whether the incoming wire query selected
`__typename`.

**Shared-only abstract selections still carry a typed discriminator.** A
selection with no narrowing fragment stays a single flat struct (see the note
below), but that struct gains a typed `Typename` field whose type is a generated
`<Struct>Typename` defined string with one constant per possible concrete type.
genqlient injects `__typename` into the wire request for *every* abstract field —
including a bare `node { id }` or a spread on the same interface — and requires it
back, so the field lets a fixture supply it:

```go
// for a shared-only `node { id }` selection
Node: &usertest.GetNodeSharedResponseNode{
    Typename: usertest.GetNodeSharedResponseNodeTypenameUser, // or GetNodeSharedResponseNodeTypename("User")
    ID:       "1",
}
```

The tag depends on whether your operation selected `__typename`. When the
operation **omits** it (synthesized for genqlient compatibility), the field is
`json:"__typename,omitempty"`: set it to emit the discriminator a genqlient
client needs, or leave it unset to write no `__typename` — so a strict
gqlgenc-style decoder that did not select `__typename` still accepts the response.
When the operation **explicitly selects** `__typename`, the field is plain
`json:"__typename"` (no `omitempty`) and is always present, matching the GraphQL
request shape. A broad interface produces a long constant list; you can ignore it
and cast a string literal (`GetNodeSharedResponseNodeTypename("User")`) instead.

A real field literally named `typename` maps to the same Go field `Typename` as
the discriminator. Selecting both in the same flat struct is a generation-time
error (the shared `both map to Go field name "Typename"` message) rather than
uncompilable output. On a polymorphic *variant* the two coexist safely — the
variant keeps `typename` as a struct field while its `MarshalJSON` injects
`__typename` under a distinct JSON key — so that case is allowed.

This matches [genqlient][genqlient], which auto-injects `__typename` into the
wire request for abstract types (even when your `.graphql` operation file omits
it) and requires it back to discriminate. gqltesthandler targets genqlient-style
generated clients, so it infers that effective query shape and always returns the
discriminator. A strict decoder such as gqlgenc's `graphqljson` rejects fields it
did not select, so a gqlgenc-style client must select `__typename` on its
narrowing abstract selections to consume the variant response; for shared-only
selections the `omitempty` discriminator above keeps it compatible. gqlgenc strict
no-extra-field behavior is not the controlling policy here.

A custom `Handle()` responder writes the HTTP response itself, so it bypasses
injection entirely — a `Handle()` function is responsible for emitting (or
omitting) `__typename` to match whatever its client expects.

> **Note:** A selection is modeled polymorphically when it contains at least one
> type-conditioned fragment that *narrows* the abstract type to a proper subset
> of its possible types. genqlient injects `__typename` for those selections
> too, so even a single narrowing fragment produces variant structs. A
> selection with no narrowing fragment — only shared fields, possibly via a
> fragment on the same abstract type — stays a single flat struct rather than
> expanding to one struct per possible type, but it still carries the typed
> `Typename` discriminator described above so it stays genqlient-compatible
> without the per-variant explosion.

In polymorphic mode the generator emits a variant struct only for the concrete
types reached by a narrowing type condition — not for every possible type of the
abstract field. If an abstract selection can legally resolve to a concrete type
for which the operation selected only shared fields (no narrowing fragment on
it), that type has no generated variant, so the typed `Respond` API cannot
construct that concrete response: use `Handle()` to write the raw JSON, or add a
type condition for that type if you need to model it.

[genqlient]: https://github.com/Khan/genqlient

### Unmatched Operations

The handler is **strict by default** for the operations the generator was run
against: if a client sends a *known* operation and neither a concrete
`Expect<OpName>` nor a `Default<OpName>` matches it, the handler calls
`t.Errorf("no expectation found ...")` and returns a GraphQL error response.
This surfaces missed test setup and fixture drift loudly instead of silently
returning empty data.

Requests naming an *unknown* operation (typo, schema/operations drift) get
back an `"unknown operation: ..."` GraphQL error response but do **not**
trigger `t.Errorf` — there is no `Default<OpName>` you could have registered
for an op the generator never saw. The test will still fail through its own
assertions on the response payload.

To opt out of strictness for a known operation — e.g. when building a
stateful fake that wraps the generated handler and feeds responses from a
seeded in-memory store — register a `Default<OpName>` for it at handler
construction. A `Default<OpName>().Respond(emptyResponse)` is enough to
turn the strict error into a graceful empty payload for that one operation,
while leaving every other operation strict.

### Per-Operation Defaults

Each operation also gets a `Default{OperationName}()` method for registering a
fallback response. Defaults match only when no concrete `Expect*` expectation
matches the incoming request. They are infinitely callable and never fail at
cleanup.

```go
// Static default
handler.DefaultGetUser().Respond(usertest.GetUserResponse{
    User: &usertest.GetUserResponseUser{Name: "stub"},
})

// Dynamic default — Handle on a Default builder receives the actual variables
// from the incoming request, which is great for stateful test fakes.
handler.DefaultGetUser().Handle(func(vars usertest.GetUserVariables, w http.ResponseWriter) {
    // look up vars.ID in your in-memory store, write a response, etc.
})
```

A concrete `Expect*` always wins over the default. Calling `Default<OpName>` a
second time replaces the previous default. Defaults accept no
`ExpectOption` — `Times`/`MinTimes` are meaningless for an infinitely callable
fallback.

### Resetting Expectations

`handler.Reset<OperationName>()` wipes registered expectations and the default
for one operation. `handler.Reset()` wipes everything across all operations.
Both also clear any pending cleanup errors so previously-registered `Times(N)`
expectations do not fire `t.Errorf` after a reset.

```go
handler.ExpectGetUser(usertest.GetUserVariables{ID: "stale"}).Respond(usertest.GetUserResponse{})
handler.ResetGetUser() // wipes the above without failing the test
```

`Reset<OperationName>` is also variadic on the operation's variables type. When
called with one or more variables, it wipes only the registered expectations
whose key matches one of the provided variables — the default responder and
any non-matching expectations are left untouched. A variables entry that
matches no registered expectation is a silent no-op. This supports layered
fixture patterns where one layer pre-registers stubs and a downstream layer
swaps in a stricter assertion for a specific key without disturbing the rest:

```go
// Fixture layer registers MinTimes(0) stubs for every seeded user.
handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}, usertest.MinTimes(0)).Respond(stub1)
handler.ExpectGetUser(usertest.GetUserVariables{ID: "2"}, usertest.MinTimes(0)).Respond(stub2)

// Downstream test replaces the ID "1" cell with a stricter assertion.
handler.ResetGetUser(usertest.GetUserVariables{ID: "1"})
handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}, usertest.Times(1)).Respond(realResponse)
```

Both `Reset()` and `Reset<OpName>()` (with or without args) record an error
via `t.Errorf` and leave state untouched if the handler has already served any
request. They are safe to use only during test setup, before any client calls
have been made.

## CLI Usage

<!--- start usage output --->

```
Usage: gqltesthandler --schema=STRING --operations=STRING --out=STRING [flags]

Generates mock GraphQL test handlers from a schema and predefined operations.

Flags:
  -h, --help                 Show context-sensitive help.
      --schema=STRING        Path to GraphQL schema file
      --operations=STRING    Path to GraphQL operations file
  -o, --out=STRING           Directory to write the generated test handler to
      --version              Output the gqltesthandler version and exit.
```

<!--- end usage output --->
