package github_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	ghclient "github.com/willabides/gqltesthandler/example/github/client"
	"github.com/willabides/gqltesthandler/example/github/ghtest"
)

//go:generate go run github.com/gqlgo/gqlgenc
//go:generate go tool gqltesthandler --schema=schema.graphqls --operations=operations.graphql -o ghtest

func TestGetPullRequestReviews(t *testing.T) {
	handler := ghtest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := ghclient.NewClient(http.DefaultClient, server.URL, nil)

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
						},
					},
					PageInfo: ghtest.GetPullRequestReviewsResponseRepositoryPullRequestReviewsPageInfo{
						HasNextPage: false,
					},
				},
			},
		},
	})

	resp, err := client.GetPullRequestReviews(t.Context(), "octocat", "hello-world", 42, nil)
	require.NoError(t, err)

	require.Equal(t, "https://github.com/octocat/hello-world/pull/42", resp.Repository.PullRequest.URL)
	reviews := resp.Repository.PullRequest.Reviews.Nodes
	require.Len(t, reviews, 1)
	require.Equal(t, "Looks good!", reviews[0].Body)
	require.Equal(t, ghclient.PullRequestReviewStateApproved, reviews[0].State)
	require.False(t, resp.Repository.PullRequest.Reviews.PageInfo.HasNextPage)
}

func TestGetPullRequestReviews_Error(t *testing.T) {
	handler := ghtest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := ghclient.NewClient(http.DefaultClient, server.URL, nil)

	handler.ExpectGetPullRequestReviews(ghtest.GetPullRequestReviewsVariables{
		Owner:      "octocat",
		Repo:       "nonexistent",
		PullNumber: 1,
	}).RespondError(
		ghtest.GraphQLError{Message: "Could not resolve to a Repository with the name 'nonexistent'."},
	)

	_, err := client.GetPullRequestReviews(t.Context(), "octocat", "nonexistent", 1, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nonexistent")
}

func TestGetPullRequestThreads(t *testing.T) {
	handler := ghtest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := ghclient.NewClient(http.DefaultClient, server.URL, nil)

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

	resp, err := client.GetPullRequestThreads(t.Context(), "octocat", "hello-world", 42, nil, nil)
	require.NoError(t, err)

	threads := resp.Repository.PullRequest.ReviewThreads.Nodes
	require.Len(t, threads, 1)
	require.Equal(t, "RT_1", threads[0].ID)
	require.Equal(t, "main.go", threads[0].Path)
	require.False(t, threads[0].IsResolved)

	comments := threads[0].Comments.Nodes
	require.Len(t, comments, 1)
	require.Equal(t, "Consider using a constant here.", comments[0].Body)
}

func TestGetPullRequestReviews_Pagination(t *testing.T) {
	handler := ghtest.NewTestHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := ghclient.NewClient(http.DefaultClient, server.URL, nil)

	cursor := "cursor123"

	// First page — nil cursor
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

	// Second page — with cursor
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
	page1, err := client.GetPullRequestReviews(t.Context(), "octocat", "hello-world", 42, nil)
	require.NoError(t, err)
	require.True(t, page1.Repository.PullRequest.Reviews.PageInfo.HasNextPage)
	require.Equal(t, "First review", page1.Repository.PullRequest.Reviews.Nodes[0].Body)

	// Fetch second page using cursor
	page2, err := client.GetPullRequestReviews(t.Context(), "octocat", "hello-world", 42, &cursor)
	require.NoError(t, err)
	require.False(t, page2.Repository.PullRequest.Reviews.PageInfo.HasNextPage)
	require.Equal(t, "LGTM!", page2.Repository.PullRequest.Reviews.Nodes[0].Body)
}
