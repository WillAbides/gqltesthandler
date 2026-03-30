package userapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Khan/genqlient/graphql"
	"github.com/stretchr/testify/require"
	"github.com/willabides/gqltesthandler/example/userapi"
	"github.com/willabides/gqltesthandler/example/userapi/usertest"
)

func TestGetUser(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := graphql.NewClient(server.URL, http.DefaultClient)

	// Set expectation: when GetUser is called with id "1", respond with Alice.
	handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}).Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{
			ID:    "1",
			Name:  "Alice",
			Email: "alice@example.com",
		},
	})

	resp, err := userapi.GetUser(t.Context(), client, "1")
	require.NoError(t, err)
	require.Equal(t, "1", resp.User.Id)
	require.Equal(t, "Alice", resp.User.Name)
	require.Equal(t, "alice@example.com", resp.User.Email)
}

func TestCreateUser(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := graphql.NewClient(server.URL, http.DefaultClient)

	handler.ExpectCreateUser(usertest.CreateUserVariables{
		Input: usertest.CreateUserInput{Name: "Bob", Email: "bob@example.com"},
	}).Respond(usertest.CreateUserResponse{
		CreateUser: usertest.CreateUserResponseCreateUser{
			ID:    "2",
			Name:  "Bob",
			Email: "bob@example.com",
		},
	})

	resp, err := userapi.CreateUser(t.Context(), client, userapi.CreateUserInput{
		Name:  "Bob",
		Email: "bob@example.com",
	})
	require.NoError(t, err)
	require.Equal(t, "2", resp.CreateUser.Id)
	require.Equal(t, "Bob", resp.CreateUser.Name)
}

func TestGetUser_Error(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := graphql.NewClient(server.URL, http.DefaultClient)

	handler.ExpectGetUser(usertest.GetUserVariables{ID: "999"}).RespondError(
		usertest.GraphQLError{Message: "user not found"},
	)

	_, err := userapi.GetUser(t.Context(), client, "999")
	require.Error(t, err)
	require.Contains(t, err.Error(), "user not found")
}

func TestGetUser_MultipleCalls(t *testing.T) {
	handler := usertest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := graphql.NewClient(server.URL, http.DefaultClient)

	// Expect the same call 3 times.
	handler.ExpectGetUser(usertest.GetUserVariables{ID: "1"}, usertest.Times(3)).Respond(usertest.GetUserResponse{
		User: &usertest.GetUserResponseUser{
			ID:    "1",
			Name:  "Alice",
			Email: "alice@example.com",
		},
	})

	for range 3 {
		resp, err := userapi.GetUser(t.Context(), client, "1")
		require.NoError(t, err)
		require.Equal(t, "Alice", resp.User.Name)
	}
}
