package github_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willabides/gqltesthandler/example/github/ghtest"
)

// graphqlDo sends a GraphQL request and decodes the response.
func graphqlDo(t *testing.T, url, operationName string, variables any) map[string]any {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"operationName": operationName,
		"variables":     variables,
	})
	require.NoError(t, err)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, resp.Body.Close())
	}()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	return result
}

func TestGetPullRequestReviews(t *testing.T) {
	handler := ghtest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	handler.ExpectGetPullRequestReviews(ghtest.GetPullRequestReviewsVariables{
		Owner:      "octocat",
		Repo:       "hello-world",
		PullNumber: 42,
	}).Respond(ghtest.GetPullRequestReviewsResponse{
		Repository: &ghtest.GetPullRequestReviewsResponseRepository{
			PullRequest: &ghtest.GetPullRequestReviewsResponseRepositoryPullRequest{
				Url: "https://github.com/octocat/hello-world/pull/42",
				Reviews: &ghtest.GetPullRequestReviewsResponseRepositoryPullRequestReviews{
					Nodes: []*ghtest.GetPullRequestReviewsResponseRepositoryPullRequestReviewsNodes{
						{
							Url:   "https://github.com/octocat/hello-world/pull/42#pullrequestreview-1",
							Body:  "Looks good!",
							State: ghtest.PullRequestReviewStateApproved,
							Author: &ghtest.GetPullRequestReviewsResponseRepositoryPullRequestReviewsNodesAuthor{
								Url: "https://github.com/reviewer",
							},
						},
					},
					PageInfo: ghtest.GetPullRequestReviewsResponseRepositoryPullRequestReviewsPageInfo{
						HasNextPage: false,
					},
				},
			},
		},
	})

	result := graphqlDo(t, server.URL, "GetPullRequestReviews", map[string]any{
		"owner":      "octocat",
		"repo":       "hello-world",
		"pullNumber": 42,
	})

	data, ok := result["data"].(map[string]any)
	require.True(t, ok, "expected data in response")
	repo, ok := data["repository"].(map[string]any)
	require.True(t, ok)
	pr, ok := repo["pullRequest"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "https://github.com/octocat/hello-world/pull/42", pr["url"])

	reviews := pr["reviews"].(map[string]any)
	nodes := reviews["nodes"].([]any)
	require.Len(t, nodes, 1)
	review := nodes[0].(map[string]any)
	require.Equal(t, "Looks good!", review["body"])
	require.Equal(t, "APPROVED", review["state"])
}

func TestGetPullRequestReviews_Error(t *testing.T) {
	handler := ghtest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	handler.ExpectGetPullRequestReviews(ghtest.GetPullRequestReviewsVariables{
		Owner:      "octocat",
		Repo:       "nonexistent",
		PullNumber: 1,
	}).RespondError(
		ghtest.GraphQLError{Message: "Could not resolve to a Repository with the name 'nonexistent'."},
	)

	result := graphqlDo(t, server.URL, "GetPullRequestReviews", map[string]any{
		"owner":      "octocat",
		"repo":       "nonexistent",
		"pullNumber": 1,
	})

	errors, ok := result["errors"].([]any)
	require.True(t, ok, "expected errors in response")
	require.Len(t, errors, 1)
	errObj := errors[0].(map[string]any)
	require.Contains(t, errObj["message"], "nonexistent")
}

func TestGetPullRequestThreads(t *testing.T) {
	handler := ghtest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	handler.ExpectGetPullRequestThreads(ghtest.GetPullRequestThreadsVariables{
		Owner:      "octocat",
		Repo:       "hello-world",
		PullNumber: 42,
	}).Respond(ghtest.GetPullRequestThreadsResponse{
		Repository: &ghtest.GetPullRequestThreadsResponseRepository{
			PullRequest: &ghtest.GetPullRequestThreadsResponseRepositoryPullRequest{
				ReviewThreads: ghtest.GetPullRequestThreadsResponseRepositoryPullRequestReviewThreads{
					Nodes: []*ghtest.GetPullRequestThreadsResponseRepositoryPullRequestReviewThreadsNodes{
						{
							ID:          "RT_1",
							IsResolved:  false,
							IsOutdated:  false,
							Path:        "main.go",
							DiffSide:    ghtest.DiffSideRight,
							SubjectType: ghtest.PullRequestReviewThreadSubjectTypeLine,
							Comments: ghtest.GetPullRequestThreadsResponseRepositoryPullRequestReviewThreadsNodesComments{
								Nodes: []*ghtest.GetPullRequestThreadsResponseRepositoryPullRequestReviewThreadsNodesCommentsNodes{
									{
										Body:     "Consider using a constant here.",
										DiffHunk: "@@ -1,3 +1,5 @@\n+const foo = 42",
									},
								},
								PageInfo: ghtest.GetPullRequestThreadsResponseRepositoryPullRequestReviewThreadsNodesCommentsPageInfo{
									HasNextPage: false,
								},
							},
						},
					},
					PageInfo: ghtest.GetPullRequestThreadsResponseRepositoryPullRequestReviewThreadsPageInfo{
						HasNextPage: false,
					},
				},
			},
		},
	})

	result := graphqlDo(t, server.URL, "GetPullRequestThreads", map[string]any{
		"owner":      "octocat",
		"repo":       "hello-world",
		"pullNumber": 42,
	})

	data := result["data"].(map[string]any)
	repo := data["repository"].(map[string]any)
	pr := repo["pullRequest"].(map[string]any)
	threads := pr["reviewThreads"].(map[string]any)
	nodes := threads["nodes"].([]any)
	require.Len(t, nodes, 1)
	thread := nodes[0].(map[string]any)
	require.Equal(t, "RT_1", thread["id"])
	require.Equal(t, "main.go", thread["path"])
	require.Equal(t, false, thread["isResolved"])

	comments := thread["comments"].(map[string]any)
	commentNodes := comments["nodes"].([]any)
	require.Len(t, commentNodes, 1)
	comment := commentNodes[0].(map[string]any)
	require.Equal(t, "Consider using a constant here.", comment["body"])
}

func TestGetPullRequestReviews_Pagination(t *testing.T) {
	handler := ghtest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	cursor := "cursor123"

	// First page
	handler.ExpectGetPullRequestReviews(ghtest.GetPullRequestReviewsVariables{
		Owner:      "octocat",
		Repo:       "hello-world",
		PullNumber: 42,
	}).Respond(ghtest.GetPullRequestReviewsResponse{
		Repository: &ghtest.GetPullRequestReviewsResponseRepository{
			PullRequest: &ghtest.GetPullRequestReviewsResponseRepositoryPullRequest{
				Url: "https://github.com/octocat/hello-world/pull/42",
				Reviews: &ghtest.GetPullRequestReviewsResponseRepositoryPullRequestReviews{
					Nodes: []*ghtest.GetPullRequestReviewsResponseRepositoryPullRequestReviewsNodes{
						{Body: "First review", State: ghtest.PullRequestReviewStateCommented},
					},
					PageInfo: ghtest.GetPullRequestReviewsResponseRepositoryPullRequestReviewsPageInfo{
						HasNextPage: true,
						EndCursor:   &cursor,
					},
				},
			},
		},
	})

	// Second page
	handler.ExpectGetPullRequestReviews(ghtest.GetPullRequestReviewsVariables{
		Owner:      "octocat",
		Repo:       "hello-world",
		PullNumber: 42,
		Cursor:     &cursor,
	}).Respond(ghtest.GetPullRequestReviewsResponse{
		Repository: &ghtest.GetPullRequestReviewsResponseRepository{
			PullRequest: &ghtest.GetPullRequestReviewsResponseRepositoryPullRequest{
				Url: "https://github.com/octocat/hello-world/pull/42",
				Reviews: &ghtest.GetPullRequestReviewsResponseRepositoryPullRequestReviews{
					Nodes: []*ghtest.GetPullRequestReviewsResponseRepositoryPullRequestReviewsNodes{
						{Body: "LGTM!", State: ghtest.PullRequestReviewStateApproved},
					},
					PageInfo: ghtest.GetPullRequestReviewsResponseRepositoryPullRequestReviewsPageInfo{
						HasNextPage: false,
					},
				},
			},
		},
	})

	// Fetch first page
	page1 := graphqlDo(t, server.URL, "GetPullRequestReviews", map[string]any{
		"owner": "octocat", "repo": "hello-world", "pullNumber": 42,
	})
	reviews1 := page1["data"].(map[string]any)["repository"].(map[string]any)["pullRequest"].(map[string]any)["reviews"].(map[string]any)
	require.True(t, reviews1["pageInfo"].(map[string]any)["hasNextPage"].(bool))

	// Fetch second page using cursor
	page2 := graphqlDo(t, server.URL, "GetPullRequestReviews", map[string]any{
		"owner": "octocat", "repo": "hello-world", "pullNumber": 42, "cursor": cursor,
	})
	reviews2 := page2["data"].(map[string]any)["repository"].(map[string]any)["pullRequest"].(map[string]any)["reviews"].(map[string]any)
	nodes := reviews2["nodes"].([]any)
	require.Equal(t, "LGTM!", nodes[0].(map[string]any)["body"])
	require.False(t, reviews2["pageInfo"].(map[string]any)["hasNextPage"].(bool))
}
