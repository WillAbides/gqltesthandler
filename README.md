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

### Unmatched Operations

The handler is **strict by default**: if a client sends an operation and
neither a concrete `Expect<OpName>` expectation nor a `Default<OpName>`
matches it, the handler calls `t.Errorf("no expectation found ...")` and
returns a GraphQL error response. This surfaces test-configuration mistakes
(typos in operation names, missed setup, drift between client and fixtures)
loudly instead of silently returning empty data.

To opt out of strictness for a given operation — e.g. when building a
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

Both `Reset()` and `Reset<OpName>()` record an error via `t.Errorf` and leave
state untouched if the handler has already served any request. They are safe
to use only during test setup, before any client calls have been made.

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
