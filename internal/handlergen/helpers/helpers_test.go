package helpers

import (
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/willabides/gqltesthandler/internal/testutil"
)

// Test types for request/response
type testRequest struct {
	ID       int
	Name     string
	Body     io.Reader // Should be skipped in hash
	JSONBody *testBody
}

type testBody struct {
	Field1 string
	Field2 int
}

type testResponse struct {
	Status  int
	Message string
}

// errReader is an io.Reader that records when it is read and always returns
// the configured error. Used by tests that need to prove a code path did not
// touch the body.
type errReader struct {
	err  error
	read bool
}

func (r *errReader) Read(p []byte) (int, error) {
	r.read = true
	return 0, r.err
}

func TestKeyHash(t *testing.T) {
	t.Run("same request produces same hash", func(t *testing.T) {
		req1 := testRequest{
			ID:       1,
			Name:     "test",
			JSONBody: &testBody{Field1: "value", Field2: 42},
		}
		req2 := testRequest{
			ID:       1,
			Name:     "test",
			JSONBody: &testBody{Field1: "value", Field2: 42},
		}

		hash1 := keyHash(req1, nil)
		hash2 := keyHash(req2, nil)

		assert.Equal(t, hash1, hash2, "expected same hash for identical requests")
	})

	t.Run("different requests produce different hashes", func(t *testing.T) {
		req1 := testRequest{ID: 1, Name: "test"}
		req2 := testRequest{ID: 2, Name: "test"}

		hash1 := keyHash(req1, nil)
		hash2 := keyHash(req2, nil)

		assert.NotEqual(t, hash1, hash2, "expected different hashes for different requests")
	})

	t.Run("io.Reader fields are skipped", func(t *testing.T) {
		req1 := testRequest{
			ID:   1,
			Name: "test",
			Body: strings.NewReader("body content 1"),
		}
		req2 := testRequest{
			ID:   1,
			Name: "test",
			Body: strings.NewReader("body content 2"),
		}

		hash1 := keyHash(req1, nil)
		hash2 := keyHash(req2, nil)

		assert.Equal(t, hash1, hash2, "expected same hash when only io.Reader differs")
	})

	t.Run("nil vs non-nil pointer", func(t *testing.T) {
		req1 := testRequest{ID: 1, Name: "test", JSONBody: nil}
		req2 := testRequest{ID: 1, Name: "test", JSONBody: &testBody{}}

		hash1 := keyHash(req1, nil)
		hash2 := keyHash(req2, nil)

		assert.NotEqual(t, hash1, hash2, "expected different hashes for nil vs non-nil pointer")
	})
}

func TestHashValue(t *testing.T) {
	t.Run("nil values", func(t *testing.T) {
		var nilPtr *testRequest
		hash1 := keyHash(nilPtr, nil)
		hash2 := keyHash(nilPtr, nil)
		assert.Equal(t, hash1, hash2, "expected consistent hash for nil pointer")
	})

	t.Run("slices", func(t *testing.T) {
		type sliceReq struct {
			Values []int
		}
		req1 := sliceReq{Values: []int{1, 2, 3}}
		req2 := sliceReq{Values: []int{1, 2, 3}}
		req3 := sliceReq{Values: []int{1, 2, 4}}

		hash1 := keyHash(req1, nil)
		hash2 := keyHash(req2, nil)
		hash3 := keyHash(req3, nil)

		assert.Equal(t, hash1, hash2, "expected same hash for identical slices")
		assert.NotEqual(t, hash1, hash3, "expected different hash for different slices")
	})

	t.Run("maps", func(t *testing.T) {
		type mapReq struct {
			Values map[string]int
		}
		req1 := mapReq{Values: map[string]int{"a": 1, "b": 2}}
		req2 := mapReq{Values: map[string]int{"b": 2, "a": 1}} // Different order
		req3 := mapReq{Values: map[string]int{"a": 1, "b": 3}}

		hash1 := keyHash(req1, nil)
		hash2 := keyHash(req2, nil)
		hash3 := keyHash(req3, nil)

		assert.Equal(t, hash1, hash2, "expected same hash for maps regardless of insertion order")
		assert.NotEqual(t, hash1, hash3, "expected different hash for different map values")
	})

	t.Run("arrays", func(t *testing.T) {
		type arrayReq struct {
			Values [3]int
		}
		req1 := arrayReq{Values: [3]int{1, 2, 3}}
		req2 := arrayReq{Values: [3]int{1, 2, 3}}
		req3 := arrayReq{Values: [3]int{1, 2, 4}}

		hash1 := keyHash(req1, nil)
		hash2 := keyHash(req2, nil)
		hash3 := keyHash(req3, nil)

		assert.Equal(t, hash1, hash2, "expected same hash for identical arrays")
		assert.NotEqual(t, hash1, hash3, "expected different hash for different arrays")
	})

	t.Run("nested structs", func(t *testing.T) {
		req1 := testRequest{
			ID:       1,
			JSONBody: &testBody{Field1: "nested", Field2: 99},
		}
		req2 := testRequest{
			ID:       1,
			JSONBody: &testBody{Field1: "nested", Field2: 99},
		}
		req3 := testRequest{
			ID:       1,
			JSONBody: &testBody{Field1: "nested", Field2: 100},
		}

		hash1 := keyHash(req1, nil)
		hash2 := keyHash(req2, nil)
		hash3 := keyHash(req3, nil)

		assert.Equal(t, hash1, hash2, "expected same hash for identical nested structs")
		assert.NotEqual(t, hash1, hash3, "expected different hash for different nested structs")
	})
}

func TestExpectations(t *testing.T) {
	t.Run("basic expect and get", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp := testResponse{Status: 200, Message: "ok"}

		exp.expect(tb, req, nil, resp)

		got, err := exp.getResponse(tb, req, nil)
		require.NoError(t, err)
		assert.Equal(t, resp.Status, got.Status)
		assert.Equal(t, resp.Message, got.Message)

		// Cleanup should pass since expectation was met
		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("unmet expectation triggers error on cleanup", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp := testResponse{Status: 200, Message: "ok"}

		exp.expect(tb, req, nil, resp)

		// Don't call getResponse - expectation remains unmet

		tb.RunCleanups()
		tb.AssertErrors()
	})

	t.Run("Times option", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp := testResponse{Status: 200, Message: "ok"}

		exp.expect(tb, req, nil, resp, Times(3))

		for i := range 3 {
			got, err := exp.getResponse(tb, req, nil)
			require.NoError(t, err, "call %d", i+1)
			assert.Equal(t, resp.Status, got.Status, "call %d", i+1)
		}

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("Times option exceeded", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp := testResponse{Status: 200, Message: "ok"}

		exp.expect(tb, req, nil, resp, Times(2))

		// Call twice successfully
		for i := range 2 {
			_, err := exp.getResponse(tb, req, nil)
			require.NoError(t, err, "call %d", i+1)
		}

		// Third call should fail
		_, err := exp.getResponse(tb, req, nil)
		assert.Error(t, err, "expected error on third call")
		tb.AssertErrors()
	})

	t.Run("no expectation found", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}

		_, err := exp.getResponse(tb, req, nil)
		assert.Error(t, err, "expected error when no expectation found")
		tb.AssertErrors()
	})

	t.Run("FIFO matching", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp1 := testResponse{Status: 200, Message: "first"}
		resp2 := testResponse{Status: 201, Message: "second"}

		exp.expect(tb, req, nil, resp1)
		exp.expect(tb, req, nil, resp2)

		// First call should return first expectation
		got1, err := exp.getResponse(tb, req, nil)
		require.NoError(t, err)
		assert.Equal(t, "first", got1.Message)

		// Second call should return second expectation
		got2, err := exp.getResponse(tb, req, nil)
		require.NoError(t, err)
		assert.Equal(t, "second", got2.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("concurrent access", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp := testResponse{Status: 200, Message: "ok"}

		exp.expect(tb, req, nil, resp, Times(100))

		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() {
				_, err := exp.getResponse(tb, req, nil)
				assert.NoError(t, err)
			})
		}
		wg.Wait()

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("with rawRequestBody - same body bytes produce same hash", func(t *testing.T) {
		req := testRequest{ID: 1, Name: "test"}
		bodyBytes := []byte(`{"field": "value"}`)

		hash1 := keyHash(req, bodyBytes)
		hash2 := keyHash(req, bodyBytes)

		assert.Equal(t, hash1, hash2, "expected same hash for same raw body bytes")
	})

	t.Run("with rawRequestBody - different body bytes produce different hashes", func(t *testing.T) {
		req := testRequest{ID: 1, Name: "test"}
		bodyBytes1 := []byte(`{"field": "value1"}`)
		bodyBytes2 := []byte(`{"field": "value2"}`)

		hash1 := keyHash(req, bodyBytes1)
		hash2 := keyHash(req, bodyBytes2)

		assert.NotEqual(t, hash1, hash2, "expected different hashes for different raw body bytes")
	})

	t.Run("with rawRequestBody - expect and get", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test", Body: strings.NewReader("body content")}
		resp := testResponse{Status: 200, Message: "ok"}
		bodyBytes := []byte(`{"field": "value"}`)

		// Set expectation with raw body bytes
		exp.expect(tb, req, strings.NewReader(string(bodyBytes)), resp)

		// Get response with same raw body bytes
		got, err := exp.getResponse(tb, req, strings.NewReader(string(bodyBytes)))
		require.NoError(t, err)
		assert.Equal(t, resp.Status, got.Status)
		assert.Equal(t, resp.Message, got.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("with rawRequestBody - mismatched body bytes", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp := testResponse{Status: 200, Message: "ok"}
		bodyBytes1 := []byte(`{"field": "value1"}`)
		bodyBytes2 := []byte(`{"field": "value2"}`)

		// Set expectation with one body
		exp.expect(tb, req, strings.NewReader(string(bodyBytes1)), resp)

		// Try to get with different body - should fail
		_, err := exp.getResponse(tb, req, strings.NewReader(string(bodyBytes2)))
		assert.Error(t, err, "expected error when raw body bytes don't match")
		tb.AssertErrors()
	})

	t.Run("with rawRequestBody - FIFO matching with different bodies", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		bodyBytes1 := []byte(`{"field": "value1"}`)
		bodyBytes2 := []byte(`{"field": "value2"}`)
		resp1 := testResponse{Status: 200, Message: "first"}
		resp2 := testResponse{Status: 201, Message: "second"}

		// Set expectations for different body bytes
		exp.expect(tb, req, strings.NewReader(string(bodyBytes1)), resp1)
		exp.expect(tb, req, strings.NewReader(string(bodyBytes2)), resp2)

		// Get with first body
		got1, err := exp.getResponse(tb, req, strings.NewReader(string(bodyBytes1)))
		require.NoError(t, err)
		assert.Equal(t, "first", got1.Message)

		// Get with second body
		got2, err := exp.getResponse(tb, req, strings.NewReader(string(bodyBytes2)))
		require.NoError(t, err)
		assert.Equal(t, "second", got2.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("with rawRequestBody - Times option", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		bodyBytes := []byte(`{"field": "value"}`)
		resp := testResponse{Status: 200, Message: "ok"}

		exp.expect(tb, req, strings.NewReader(string(bodyBytes)), resp, Times(3))

		for i := range 3 {
			got, err := exp.getResponse(tb, req, strings.NewReader(string(bodyBytes)))
			require.NoError(t, err, "call %d", i+1)
			assert.Equal(t, resp.Status, got.Status, "call %d", i+1)
		}

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("MinTimes option - exactly n calls", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp := testResponse{Status: 200, Message: "ok"}

		exp.expect(tb, req, nil, resp, MinTimes(3))

		for i := range 3 {
			got, err := exp.getResponse(tb, req, nil)
			require.NoError(t, err, "call %d", i+1)
			assert.Equal(t, resp.Status, got.Status, "call %d", i+1)
		}

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("MinTimes option - more than n calls", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp := testResponse{Status: 200, Message: "ok"}

		exp.expect(tb, req, nil, resp, MinTimes(3))

		// Call 5 times (more than the minimum of 3)
		for i := range 5 {
			got, err := exp.getResponse(tb, req, nil)
			require.NoError(t, err, "call %d", i+1)
			assert.Equal(t, resp.Status, got.Status, "call %d", i+1)
		}

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("MinTimes option - fewer than n calls triggers error", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp := testResponse{Status: 200, Message: "ok"}

		exp.expect(tb, req, nil, resp, MinTimes(3))

		// Only call 2 times (less than the minimum of 3)
		for i := range 2 {
			got, err := exp.getResponse(tb, req, nil)
			require.NoError(t, err, "call %d", i+1)
			assert.Equal(t, resp.Status, got.Status, "call %d", i+1)
		}

		tb.RunCleanups()
		tb.AssertErrors() // Should have error because minimum not met
	})

	t.Run("MinTimes(0) acts as stub - unlimited calls", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp := testResponse{Status: 200, Message: "ok"}

		exp.expect(tb, req, nil, resp, MinTimes(0))

		// Call multiple times with no limit
		for i := range 10 {
			got, err := exp.getResponse(tb, req, nil)
			require.NoError(t, err, "call %d", i+1)
			assert.Equal(t, resp.Status, got.Status, "call %d", i+1)
		}

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("MinTimes(0) acts as stub - zero calls", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		resp := testResponse{Status: 200, Message: "ok"}

		exp.expect(tb, req, nil, resp, MinTimes(0))

		// Don't call at all - should still pass cleanup
		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("Times panics on negative value", func(t *testing.T) {
		assert.PanicsWithValue(t, "Times: n must be non-negative", func() {
			Times(-1)
		})
	})

	t.Run("MinTimes panics on negative value", func(t *testing.T) {
		assert.PanicsWithValue(t, "MinTimes: n must be non-negative", func() {
			MinTimes(-1)
		})
	})
}

func TestDefaultResponse(t *testing.T) {
	t.Run("default matches when no concrete expectation matches", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		exp.setDefault(testResponse{Status: 200, Message: "default"})

		got, err := exp.getResponse(tb, testRequest{ID: 42, Name: "anything"}, nil)
		require.NoError(t, err)
		assert.Equal(t, "default", got.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("default ignores raw request body", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		exp.setDefault(testResponse{Status: 200, Message: "default"})

		got, err := exp.getResponse(tb, testRequest{ID: 1}, strings.NewReader("totally different body"))
		require.NoError(t, err)
		assert.Equal(t, "default", got.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("concrete wins over default", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		exp.setDefault(testResponse{Status: 200, Message: "default"})
		exp.expect(tb, req, nil, testResponse{Status: 201, Message: "concrete"})

		got, err := exp.getResponse(tb, req, nil)
		require.NoError(t, err)
		assert.Equal(t, "concrete", got.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("concrete order does not matter for default precedence", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		exp.expect(tb, req, nil, testResponse{Status: 201, Message: "concrete"})
		exp.setDefault(testResponse{Status: 200, Message: "default"})

		got, err := exp.getResponse(tb, req, nil)
		require.NoError(t, err)
		assert.Equal(t, "concrete", got.Message)

		// Subsequent unrelated request hits the default.
		got2, err := exp.getResponse(tb, testRequest{ID: 99}, nil)
		require.NoError(t, err)
		assert.Equal(t, "default", got2.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("default is unlimited", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		exp.setDefault(testResponse{Status: 200, Message: "default"})

		for i := range 100 {
			got, err := exp.getResponse(tb, testRequest{ID: i}, nil)
			require.NoError(t, err)
			assert.Equal(t, "default", got.Message)
		}

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("default with zero calls is fine", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		exp.setDefault(testResponse{Status: 200, Message: "default"})

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("setDefault replaces the previous default", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		exp.setDefault(testResponse{Status: 200, Message: "first"})
		exp.setDefault(testResponse{Status: 200, Message: "second"})

		got, err := exp.getResponse(tb, testRequest{ID: 1}, nil)
		require.NoError(t, err)
		assert.Equal(t, "second", got.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("concrete expectations still consume normally with default present", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		exp.setDefault(testResponse{Status: 200, Message: "default"})
		exp.expect(tb, req, nil, testResponse{Status: 201, Message: "concrete"}, Times(2))

		// Two concrete consumptions.
		for i := range 2 {
			got, err := exp.getResponse(tb, req, nil)
			require.NoError(t, err, "call %d", i+1)
			assert.Equal(t, "concrete", got.Message)
		}

		// Third call falls back to default.
		got, err := exp.getResponse(tb, req, nil)
		require.NoError(t, err)
		assert.Equal(t, "default", got.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("fast path skips body read when only the default is registered", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		exp.setDefault(testResponse{Status: 200, Message: "default"})

		// A reader that would error if anyone actually read from it.
		body := &errReader{err: io.ErrUnexpectedEOF}

		got, err := exp.getResponse(tb, testRequest{ID: 1}, body)
		require.NoError(t, err)
		assert.Equal(t, "default", got.Message)
		assert.False(t, body.read, "expected body to be left unread on default fast path")

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("fast path errors without reading body when no default and no expectations", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		body := &errReader{err: io.ErrUnexpectedEOF}

		_, err := exp.getResponse(tb, testRequest{ID: 1}, body)
		assert.Error(t, err)
		assert.False(t, body.read, "expected body to be left unread on empty fast path")
		tb.AssertErrors()
	})
}

func TestClear(t *testing.T) {
	t.Run("clear removes expectations", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		exp.expect(tb, req, nil, testResponse{Status: 200, Message: "ok"})

		exp.clear()

		_, err := exp.getResponse(tb, req, nil)
		assert.Error(t, err, "expected no expectation after clear")
	})

	t.Run("clear disarms cleanup errors for unmet expectations", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		exp.expect(tb, req, nil, testResponse{}, Times(5))

		exp.clear()

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("clear removes default", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		exp.setDefault(testResponse{Status: 200, Message: "default"})

		exp.clear()

		_, err := exp.getResponse(tb, testRequest{ID: 1}, nil)
		assert.Error(t, err, "expected no default after clear")
	})

	t.Run("expectations registered after clear still work", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		exp.expect(tb, testRequest{ID: 1}, nil, testResponse{Message: "first"})
		exp.clear()

		req := testRequest{ID: 2}
		exp.expect(tb, req, nil, testResponse{Message: "second"})

		got, err := exp.getResponse(tb, req, nil)
		require.NoError(t, err)
		assert.Equal(t, "second", got.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("default registered after clear still works", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		exp.setDefault(testResponse{Message: "first"})
		exp.clear()
		exp.setDefault(testResponse{Message: "second"})

		got, err := exp.getResponse(tb, testRequest{ID: 1}, nil)
		require.NoError(t, err)
		assert.Equal(t, "second", got.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})
}

func TestClearByRequests(t *testing.T) {
	t.Run("wipes matching expectations and leaves others", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		keep := testRequest{ID: 1, Name: "keep"}
		drop := testRequest{ID: 2, Name: "drop"}
		exp.expect(tb, keep, nil, testResponse{Message: "keep"})
		exp.expect(tb, drop, nil, testResponse{Message: "drop"})

		exp.clearByRequests([]testRequest{drop})

		// keep still matches
		got, err := exp.getResponse(tb, keep, nil)
		require.NoError(t, err)
		assert.Equal(t, "keep", got.Message)

		// drop is gone
		_, err = exp.getResponse(tb, drop, nil)
		assert.Error(t, err)

		tb.RunCleanups()
		// keep's cleanup is satisfied (we consumed it). drop's cleanup must
		// also be satisfied because clearByRequests disarmed it.
		tb.AssertErrors() // the explicit error above counts
	})

	t.Run("leaves the default response intact", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "concrete"}
		exp.setDefault(testResponse{Message: "default"})
		exp.expect(tb, req, nil, testResponse{Message: "concrete"})

		exp.clearByRequests([]testRequest{req})

		got, err := exp.getResponse(tb, req, nil)
		require.NoError(t, err)
		assert.Equal(t, "default", got.Message, "default should still serve after targeted wipe of concrete")

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("wipes all expectations sharing the same key", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "shared"}
		exp.expect(tb, req, nil, testResponse{Message: "first"})
		exp.expect(tb, req, nil, testResponse{Message: "second"})

		exp.clearByRequests([]testRequest{req})

		_, err := exp.getResponse(tb, req, nil)
		assert.Error(t, err, "both shared-key expectations should be gone")

		tb.RunCleanups()
		tb.AssertErrors() // the explicit error above; no cleanup errors expected
	})

	t.Run("disarms cleanup hooks for the wiped expectations", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		drop := testRequest{ID: 1, Name: "drop"}
		exp.expect(tb, drop, nil, testResponse{}, Times(5))

		exp.clearByRequests([]testRequest{drop})

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("non-matching vars is a no-op (not an error)", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		keep := testRequest{ID: 1, Name: "keep"}
		exp.expect(tb, keep, nil, testResponse{Message: "keep"})

		// vars that match nothing — should not error or remove anything.
		exp.clearByRequests([]testRequest{{ID: 999, Name: "ghost"}})

		got, err := exp.getResponse(tb, keep, nil)
		require.NoError(t, err)
		assert.Equal(t, "keep", got.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})

	t.Run("empty request slice is a no-op", func(t *testing.T) {
		tb := testutil.NewTB(t)
		exp := &expectResponses[testRequest, testResponse]{}

		req := testRequest{ID: 1, Name: "test"}
		exp.expect(tb, req, nil, testResponse{Message: "ok"})

		exp.clearByRequests(nil)

		got, err := exp.getResponse(tb, req, nil)
		require.NoError(t, err)
		assert.Equal(t, "ok", got.Message)

		tb.RunCleanups()
		tb.AssertNoErrors()
	})
}
