package userapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	// Default handler responds to any other ID using the variables from the
	// incoming request.
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

	// Default is sticky — call it again with another unknown id.
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

type recordingTB struct {
	t      *testing.T
	errors []string
}

func (r *recordingTB) Cleanup(f func()) { r.t.Cleanup(f) }
func (r *recordingTB) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}
