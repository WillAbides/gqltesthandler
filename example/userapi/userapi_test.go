package userapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	userclient "github.com/willabides/gqltesthandler/example/userapi/client"
	"github.com/willabides/gqltesthandler/example/userapi/usertest"
)

//go:generate go run github.com/gqlgo/gqlgenc
//go:generate go tool gqltesthandler --schema=schema.graphqls --operations=operations.graphql -o usertest

func TestGetUser(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{
			ID:    "1",
			Name:  "Alice",
			Email: "alice@example.com",
		},
	})

	resp, err := client.GetUser(t.Context(), "1")
	require.NoError(t, err)
	require.Equal(t, "1", resp.User.ID)
	require.Equal(t, "Alice", resp.User.Name)
	require.Equal(t, "alice@example.com", resp.User.Email)
}

func TestCreateUser(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	handler.ExpectCreateUser(usertest.CreateUserVariables{
		Input: usertest.CreateUserInput{Name: "Bob", Email: "bob@example.com"},
	}).Respond(usertest.CreateUserResponse{
		CreateUser: usertest.CreateUserResponseCreateUser{
			ID:    "2",
			Name:  "Bob",
			Email: "bob@example.com",
		},
	})

	resp, err := client.CreateUser(t.Context(), userclient.CreateUserInput{
		Name:  "Bob",
		Email: "bob@example.com",
	})
	require.NoError(t, err)
	require.Equal(t, "2", resp.CreateUser.ID)
	require.Equal(t, "Bob", resp.CreateUser.Name)
}

func TestGetUser_Error(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	handler.ExpectGetUser(usertest.GetUserVariables{ID: "999"}).RespondError(
		usertest.GraphQLError{Message: "user not found"},
	)

	_, err := client.GetUser(t.Context(), "999")
	require.Error(t, err)
	require.Contains(t, err.Error(), "user not found")
}

func TestGetUser_MultipleCalls(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}, usertest.Times(3)).Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{
			ID:    "1",
			Name:  "Alice",
			Email: "alice@example.com",
		},
	})

	for range 3 {
		resp, err := client.GetUser(t.Context(), "1")
		require.NoError(t, err)
		require.Equal(t, "Alice", resp.User.Name)
	}
}

func TestGetUser_Default(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	// Concrete expectation for ID "1" takes precedence over the default.
	handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{ID: "1", Name: "Alice"},
	})

	// The default receives the incoming request's variables.
	handler.DefaultGetUser().Handle(func(vars usertest.GetUserVariables, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id":    vars.ID,
					"name":  "user-" + vars.ID,
					"email": "user-" + vars.ID + "@example.com",
				},
			},
		}))
	})

	resp1, err := client.GetUser(t.Context(), "1")
	require.NoError(t, err)
	require.Equal(t, "Alice", resp1.User.Name)

	resp2, err := client.GetUser(t.Context(), "42")
	require.NoError(t, err)
	require.Equal(t, "user-42", resp2.User.Name)

	// The default is sticky across calls.
	resp3, err := client.GetUser(t.Context(), "99")
	require.NoError(t, err)
	require.Equal(t, "user-99", resp3.User.Name)
}

func TestReset_BeforeRequests(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	// Register an expectation that would otherwise fail at cleanup, then
	// wipe it before any request is served.
	handler.ExpectGetUser(usertest.GetUserVariables{ID: "stale"}).Respond(usertest.GetUserResponse{})
	handler.ResetGetUser()

	handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{ID: "1", Name: "Alice"},
	})

	resp, err := client.GetUser(t.Context(), "1")
	require.NoError(t, err)
	require.Equal(t, "Alice", resp.User.Name)
}

func TestResetGetUser_TargetedWipe(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	// Fixture layer pre-registers stubs.
	handler.DefaultGetUser().Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{ID: "0", Name: "default"},
	})
	handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}, usertest.MinTimes(0)).Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{ID: "1", Name: "fixture-1"},
	})
	handler.ExpectGetUser(usertest.GetUserVariables{ID: "2"}, usertest.MinTimes(0)).Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{ID: "2", Name: "fixture-2"},
	})

	// Downstream layer wants a stricter assertion for ID "1" only.
	handler.ResetGetUser(usertest.GetUserVariables{ID: "1"})
	handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{ID: "1", Name: "strict-1"},
	})

	got1, err := client.GetUser(t.Context(), "1")
	require.NoError(t, err)
	require.Equal(t, "strict-1", got1.User.Name)

	// ID "2" still uses the untouched fixture.
	got2, err := client.GetUser(t.Context(), "2")
	require.NoError(t, err)
	require.Equal(t, "fixture-2", got2.User.Name)

	// Unmapped ID still falls through to the default — proves the default
	// was not wiped by the targeted reset.
	got3, err := client.GetUser(t.Context(), "99")
	require.NoError(t, err)
	require.Equal(t, "default", got3.User.Name)
}

func TestResetGetUser_TargetedWipeNoMatch(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{ID: "1", Name: "Alice"},
	})

	// Resetting a variables set that was never registered is a no-op — not
	// an error. The expectation for ID "1" must still serve.
	handler.ResetGetUser(usertest.GetUserVariables{ID: "ghost"})

	resp, err := client.GetUser(t.Context(), "1")
	require.NoError(t, err)
	require.Equal(t, "Alice", resp.User.Name)
}

func TestReset_AfterServedRecordsError(t *testing.T) {
	tb := &recordingTB{t: t}
	handler := usertest.NewTestHandler(tb)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{ID: "1", Name: "Alice"},
	})

	_, err := client.GetUser(t.Context(), "1")
	require.NoError(t, err)

	// After the handler has served a request, Reset and ResetGetUser must
	// record an error and leave state untouched.
	handler.ResetGetUser()
	handler.Reset()

	require.Len(t, tb.errors, 2)
	require.Contains(t, tb.errors[0], "ResetGetUser called after handler has served")
	require.Contains(t, tb.errors[1], "Reset called after handler has served")
}

func TestReset_AfterMalformedRequestRecordsError(t *testing.T) {
	tb := &recordingTB{t: t}
	handler := usertest.NewTestHandler(tb)
	server := httptest.NewServer(handler)
	defer server.Close()

	// A request that fails JSON decode (or method validation) must still
	// flip the served flag so a subsequent Reset is correctly rejected.
	resp, err := http.Post(server.URL, "application/json", strings.NewReader("not json"))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	handler.Reset()

	require.Len(t, tb.errors, 1)
	require.Contains(t, tb.errors[0], "Reset called after handler has served")
}

type recordingTB struct {
	t      *testing.T
	errors []string
}

func (r *recordingTB) Cleanup(f func()) { r.t.Cleanup(f) }
func (r *recordingTB) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func TestSearch_Union(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	// Fixtures use concrete variant structs; MarshalJSON injects __typename so
	// the gqlgenc client can discriminate the union members.
	handler.ExpectSearch(usertest.SearchVariables{Term: "alice"}).Respond(usertest.SearchResponse{
		Search: []usertest.SearchResponseSearch{
			usertest.SearchResponseSearchUser{ID: "u1", Name: "Alice"},
			usertest.SearchResponseSearchPost{ID: "p1", Title: "Hello"},
		},
	})

	resp, err := client.Search(t.Context(), "alice")
	require.NoError(t, err)
	require.Len(t, resp.Search, 2)

	require.NotNil(t, resp.Search[0].Typename)
	require.Equal(t, "User", *resp.Search[0].Typename)
	require.Equal(t, "u1", resp.Search[0].User.ID)
	require.Equal(t, "Alice", resp.Search[0].User.Name)

	require.NotNil(t, resp.Search[1].Typename)
	require.Equal(t, "Post", *resp.Search[1].Typename)
	require.Equal(t, "p1", resp.Search[1].Post.ID)
	require.Equal(t, "Hello", resp.Search[1].Post.Title)
}

func TestGetNode_InterfaceSharedField(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	// The interface field carries a shared `id` plus type-specific fields. The
	// shared id is promoted onto every concrete variant struct.
	handler.ExpectGetNode(usertest.GetNodeVariables{ID: "u1"}).Respond(usertest.GetNodeResponse{
		Node: usertest.GetNodeResponseNodeUser{ID: "u1", Name: "Alice", Email: "alice@example.com"},
	})

	resp, err := client.GetNode(t.Context(), "u1")
	require.NoError(t, err)
	require.Equal(t, "u1", resp.Node.GetID())
	// The real gqlgenc client decodes the always-injected interface __typename.
	require.NotNil(t, resp.Node.GetTypename())
	require.Equal(t, "User", *resp.Node.GetTypename())
	require.Equal(t, "Alice", resp.Node.User.Name)
	require.Equal(t, "alice@example.com", resp.Node.User.Email)
}

func TestGetNode_InterfacePostVariant(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := userclient.NewClient(http.DefaultClient, server.URL, nil)

	handler.ExpectGetNode(usertest.GetNodeVariables{ID: "p1"}).Respond(usertest.GetNodeResponse{
		Node: usertest.GetNodeResponseNodePost{ID: "p1", Title: "Hello"},
	})

	resp, err := client.GetNode(t.Context(), "p1")
	require.NoError(t, err)
	require.Equal(t, "p1", resp.Node.GetID())
	require.Equal(t, "Hello", resp.Node.Post.Title)
}

// postRawGraphQL sends a hand-built GraphQL request, decoupling the wire query
// from the checked-in operation file so a test can omit __typename and prove the
// handler injects the discriminator anyway (injection is unconditional).
func postRawGraphQL(t *testing.T, url, query, operationName string, variables map[string]any) string {
	t.Helper()
	reqBody, err := json.Marshal(map[string]any{
		"query":         query,
		"operationName": operationName,
		"variables":     variables,
	})
	require.NoError(t, err)
	resp, err := http.Post(url, "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(out)
}

// TestTypename_InjectedWhenGenqlientClientSelectsIt: a genqlient-style request
// selects __typename, and the handler emits the discriminator back.
func TestTypename_InjectedWhenGenqlientClientSelectsIt(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	handler.ExpectGetNode(usertest.GetNodeVariables{ID: "u1"}).Respond(usertest.GetNodeResponse{
		Node: usertest.GetNodeResponseNodeUser{ID: "u1", Name: "Alice", Email: "alice@example.com"},
	})

	// Wire query selects __typename even though GetNode's operation file omits it.
	query := `query GetNode($id: ID!) {
	  node(id: $id) {
	    __typename
	    id
	    ... on User { name email }
	    ... on Post { title }
	  }
	}`
	body := postRawGraphQL(t, server.URL, query, "GetNode", map[string]any{"id": "u1"})
	require.Contains(t, body, `"__typename":"User"`)
	require.Contains(t, body, `"name":"Alice"`)
}

// TestTypename_InjectedEvenWhenRequestOmitsIt is the core policy assertion: the
// concrete variant serializes __typename even when the wire query omits it.
func TestTypename_InjectedEvenWhenRequestOmitsIt(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	handler.ExpectGetNode(usertest.GetNodeVariables{ID: "u1"}).Respond(usertest.GetNodeResponse{
		Node: usertest.GetNodeResponseNodeUser{ID: "u1", Name: "Alice", Email: "alice@example.com"},
	})

	query := `query GetNode($id: ID!) {
	  node(id: $id) {
	    id
	    ... on User { name email }
	    ... on Post { title }
	  }
	}`
	body := postRawGraphQL(t, server.URL, query, "GetNode", map[string]any{"id": "u1"})
	require.Contains(t, body, `"__typename":"User"`)
	require.Contains(t, body, `"name":"Alice"`)
}

// TestTypename_InjectedForUnionWhenRequestOmitsIt is the union counterpart: a
// Search omitting __typename still gets it injected on every union member.
func TestTypename_InjectedForUnionWhenRequestOmitsIt(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	handler.ExpectSearch(usertest.SearchVariables{Term: "alice"}).Respond(usertest.SearchResponse{
		Search: []usertest.SearchResponseSearch{
			usertest.SearchResponseSearchUser{ID: "u1", Name: "Alice"},
		},
	})

	query := `query Search($term: String!) {
	  search(term: $term) {
	    ... on User { id name }
	    ... on Post { id title }
	  }
	}`
	body := postRawGraphQL(t, server.URL, query, "Search", map[string]any{"term": "alice"})
	require.Contains(t, body, `"__typename":"User"`)
	require.Contains(t, body, `"name":"Alice"`)
}

// TestTypename_NestedInjection: injection recurses to every abstract level
// (GetNodeRelated: node -> Post.related -> Node) regardless of the wire query.
func TestTypename_NestedInjection(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	handler.DefaultGetNodeRelated().Respond(usertest.GetNodeRelatedResponse{
		Node: usertest.GetNodeRelatedResponseNodePost{
			ID:    "p1",
			Title: "Hello",
			Related: usertest.GetNodeRelatedResponseNodePostRelatedUser{
				ID:   "u1",
				Name: "Alice",
			},
		},
	})

	withTypename := `query GetNodeRelated($id: ID!) {
	  node(id: $id) {
	    __typename
	    id
	    ... on User { name }
	    ... on Post {
	      title
	      related {
	        __typename
	        id
	        ... on User { name }
	        ... on Post { title }
	      }
	    }
	  }
	}`
	body := postRawGraphQL(t, server.URL, withTypename, "GetNodeRelated", map[string]any{"id": "p1"})
	require.Contains(t, body, `"__typename":"Post"`)
	require.Contains(t, body, `"__typename":"User"`)

	withoutTypename := `query GetNodeRelated($id: ID!) {
	  node(id: $id) {
	    id
	    ... on User { name }
	    ... on Post {
	      title
	      related {
	        id
	        ... on User { name }
	        ... on Post { title }
	      }
	    }
	  }
	}`
	body2 := postRawGraphQL(t, server.URL, withoutTypename, "GetNodeRelated", map[string]any{"id": "p1"})
	require.Contains(t, body2, `"__typename":"Post"`)
	require.Contains(t, body2, `"__typename":"User"`)
	require.Contains(t, body2, `"title":"Hello"`)
	require.Contains(t, body2, `"name":"Alice"`)
}

// TestTypename_ListStructureIncludesDiscriminator proves the discriminator
// appears on every element of an abstract list, leaving the rest intact.
func TestTypename_ListStructureIncludesDiscriminator(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	handler.DefaultSearch().Respond(usertest.SearchResponse{
		Search: []usertest.SearchResponseSearch{
			usertest.SearchResponseSearchUser{ID: "u1", Name: "Alice"},
			usertest.SearchResponseSearchPost{ID: "p1", Title: "Hello"},
		},
	})

	query := `query Search($term: String!) {
	  search(term: $term) {
	    ... on User { id name }
	    ... on Post { id title }
	  }
	}`
	body := postRawGraphQL(t, server.URL, query, "Search", map[string]any{"term": "x"})

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	require.Equal(t, map[string]any{
		"data": map[string]any{
			"search": []any{
				map[string]any{"__typename": "User", "id": "u1", "name": "Alice"},
				map[string]any{"__typename": "Post", "id": "p1", "title": "Hello"},
			},
		},
	}, got)
}

// TestTypename_SingleFragmentInjectsDiscriminator covers the threshold policy: a
// single narrowing fragment (... on User) is still modeled polymorphically, so
// the handler injects __typename even though the operation omits it.
func TestTypename_SingleFragmentInjectsDiscriminator(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	handler.ExpectGetNodeAsUser(usertest.GetNodeAsUserVariables{ID: "u1"}).Respond(usertest.GetNodeAsUserResponse{
		Node: usertest.GetNodeAsUserResponseNodeUser{ID: "u1", Name: "Alice"},
	})

	query := `query GetNodeAsUser($id: ID!) {
	  node(id: $id) {
	    id
	    ... on User { name }
	  }
	}`
	body := postRawGraphQL(t, server.URL, query, "GetNodeAsUser", map[string]any{"id": "u1"})
	require.Contains(t, body, `"__typename":"User"`)
	require.Contains(t, body, `"name":"Alice"`)
}

// TestTypename_FlatAbstractTypedDiscriminator covers a shared-only abstract
// selection: it stays a single flat struct but carries a typed, settable
// Typename. Setting it emits the __typename genqlient requires; leaving it unset
// emits none, so a strict decoder that did not select it still accepts the response.
func TestTypename_FlatAbstractTypedDiscriminator(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	handler.ExpectGetNodeShared(usertest.GetNodeSharedVariables{ID: "u1"}).Respond(usertest.GetNodeSharedResponse{
		Node: &usertest.GetNodeSharedResponseNode{
			Typename: usertest.GetNodeSharedResponseNodeTypenameUser,
			ID:       "u1",
		},
	})
	handler.ExpectGetNodeShared(usertest.GetNodeSharedVariables{ID: "u2"}).Respond(usertest.GetNodeSharedResponse{
		Node: &usertest.GetNodeSharedResponseNode{ID: "u2"},
	})

	query := `query GetNodeShared($id: ID!) {
	  node(id: $id) {
	    id
	  }
	}`

	withTypename := postRawGraphQL(t, server.URL, query, "GetNodeShared", map[string]any{"id": "u1"})
	require.Contains(t, withTypename, `"__typename":"User"`)
	require.Contains(t, withTypename, `"id":"u1"`)

	withoutTypename := postRawGraphQL(t, server.URL, query, "GetNodeShared", map[string]any{"id": "u2"})
	require.NotContains(t, withoutTypename, `"__typename"`)
	require.Contains(t, withoutTypename, `"id":"u2"`)
}

// TestTypename_VariantRealTypenameFieldCoexists proves a variant selecting a real
// `typename` field is safe alongside the MarshalJSON-injected `__typename`: the
// injection goes through an alias embed, so the two serialize under distinct JSON
// keys. The flat-struct equivalent is instead a hard error (see
// TestRun_SynthesizedTypenameConflict), since there they'd be sibling Go fields.
func TestTypename_VariantRealTypenameFieldCoexists(t *testing.T) {
	realTypename := "custom"
	variant := usertest.GetUserTypenameResponseNodeUser{Typename: &realTypename, Name: "Alice"}

	got, err := json.Marshal(variant)
	require.NoError(t, err)

	require.Contains(t, string(got), `"__typename":"User"`)
	require.Contains(t, string(got), `"typename":"custom"`)
	require.Contains(t, string(got), `"name":"Alice"`)
}
