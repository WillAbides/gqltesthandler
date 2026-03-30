package userapi_test

import (
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
