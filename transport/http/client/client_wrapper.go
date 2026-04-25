// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreio "github.com/altessa-s/go-atlas/core/io"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

const (
	// HTTPClientInternerSize is the size of the string interner for HTTP client
	HTTPClientInternerSize = 512
)

// HTTPClient defines the methods for making HTTP requests with built-in
// resilience features including automatic retries, exponential backoff, and circuit breaker support.
//
// Implementations are safe for concurrent use from multiple goroutines.
// Create instances with [NewHTTPClient]; the underlying [net/http.Client] can also
// be obtained directly via [New].
//
// The client automatically handles:
//   - Request retries with exponential backoff for transient failures
//   - Circuit breaker protection to prevent cascading failures
//   - Response size limiting to prevent memory exhaustion
//   - Per-host circuit breaker configuration
//   - Request rate limiting (when configured)
//
// Basic usage:
//
//	client := httpclient.NewHTTPClient()
//	resp, err := client.Get(ctx, "https://api.example.com/users")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer resp.Body.Close()
//
// Advanced configuration:
//
//	client := httpclient.NewHTTPClient(
//	    httpclient.WithRetryMax(3),
//	    httpclient.WithRetryWait(1*time.Second, 5*time.Second),
//	    httpclient.WithCircuitBreakerSettings("api.example.com", &httpclient.CircuitBreakerSettings{
//	        MaxRequests: 10,
//	        Timeout:     30 * time.Second,
//	    }),
//	)
type HTTPClient interface {
	// Do executes an HTTP request using the resilient HTTP client.
	// This is the core method that all other methods build upon.
	// The request will be subject to retry logic, circuit breaker protection,
	// and other configured resilience features.
	//
	// Example:
	//	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.example.com/users", nil)
	//	req.Header.Set("Authorization", "Bearer token")
	//	resp, err := client.Do(req)
	Do(req *http.Request) (*http.Response, error)

	// Get performs an HTTP GET request to the specified URL.
	// The request will automatically retry on transient failures and respect
	// circuit breaker state for the target host.
	//
	// Parameters:
	//   - ctx: Context for request cancellation and timeout
	//   - url: Target URL for the GET request
	//
	// Returns the HTTP response and any error encountered.
	//
	// Example:
	//	resp, err := client.Get(ctx, "https://api.example.com/users")
	//	if err != nil {
	//	    if httpclient.IsCircuitBreakerOpen(err) {
	//	        log.Println("Service temporarily unavailable")
	//	        return
	//	    }
	//	    return err
	//	}
	//	defer resp.Body.Close()
	Get(ctx context.Context, url string) (*http.Response, error)

	// Post performs an HTTP POST request with the specified content type and body.
	// The request will be retried automatically for transient failures.
	//
	// Parameters:
	//   - ctx: Context for request cancellation and timeout
	//   - url: Target URL for the POST request
	//   - contentType: MIME type of the request body (e.g., "application/json")
	//   - body: Request body as an io.Reader
	//
	// Example:
	//	body := strings.NewReader(`{"name":"John","email":"john@example.com"}`)
	//	resp, err := client.Post(ctx, "https://api.example.com/users", "application/json", body)
	Post(ctx context.Context, url, contentType string, body io.Reader) (*http.Response, error)

	// PostJSON performs an HTTP POST request with a JSON-encoded body.
	// The body parameter is automatically marshaled to JSON and the Content-Type
	// header is set to "application/json". This is a convenience method for JSON APIs.
	//
	// Parameters:
	//   - ctx: Context for request cancellation and timeout
	//   - url: Target URL for the POST request
	//   - body: Any Go value that can be marshaled to JSON
	//
	// Returns the HTTP response and any error encountered, including JSON marshaling errors.
	//
	// Example:
	//	user := User{Name: "John", Email: "john@example.com"}
	//	resp, err := client.PostJSON(ctx, "https://api.example.com/users", user)
	PostJSON(ctx context.Context, url string, body any) (*http.Response, error)

	// Put performs an HTTP PUT request with the specified content type and body.
	// Typically used for updating existing resources.
	//
	// Example:
	//	body := strings.NewReader(`{"name":"Jane","email":"jane@example.com"}`)
	//	resp, err := client.Put(ctx, "https://api.example.com/users/123", "application/json", body)
	Put(ctx context.Context, url, contentType string, body io.Reader) (*http.Response, error)

	// PutJSON performs an HTTP PUT request with a JSON-encoded body.
	// Convenience method for updating resources with JSON data.
	//
	// Example:
	//	updatedUser := User{Name: "Jane", Email: "jane@example.com"}
	//	resp, err := client.PutJSON(ctx, "https://api.example.com/users/123", updatedUser)
	PutJSON(ctx context.Context, url string, body any) (*http.Response, error)

	// Patch performs an HTTP PATCH request with the specified content type and body.
	// Typically used for partial updates to existing resources.
	//
	// Example:
	//	body := strings.NewReader(`{"email":"newemail@example.com"}`)
	//	resp, err := client.Patch(ctx, "https://api.example.com/users/123", "application/json", body)
	Patch(ctx context.Context, url, contentType string, body io.Reader) (*http.Response, error)

	// PatchJSON performs an HTTP PATCH request with a JSON-encoded body.
	// Convenience method for partial updates with JSON data.
	//
	// Example:
	//	update := map[string]string{"email": "newemail@example.com"}
	//	resp, err := client.PatchJSON(ctx, "https://api.example.com/users/123", update)
	PatchJSON(ctx context.Context, url string, body any) (*http.Response, error)

	// Delete performs an HTTP DELETE request to the specified URL.
	// Typically used for removing resources.
	//
	// Example:
	//	resp, err := client.Delete(ctx, "https://api.example.com/users/123")
	//	if err != nil {
	//	    return err
	//	}
	//	if resp.StatusCode == 404 {
	//	    log.Println("User not found")
	//	}
	Delete(ctx context.Context, url string) (*http.Response, error)

	// Head performs an HTTP HEAD request to the specified URL.
	// Returns only headers without the response body, useful for checking
	// resource existence or getting metadata.
	//
	// Example:
	//	resp, err := client.Head(ctx, "https://api.example.com/users/123")
	//	if err != nil {
	//	    return err
	//	}
	//	lastModified := resp.Header.Get("Last-Modified")
	Head(ctx context.Context, url string) (*http.Response, error)

	// PostForm performs an HTTP POST request with URL-encoded form data.
	// Content-Type is automatically set to "application/x-www-form-urlencoded".
	// This is commonly used for HTML form submissions.
	//
	// Example:
	//	data := url.Values{}
	//	data.Set("username", "john")
	//	data.Set("password", "secret")
	//	resp, err := client.PostForm(ctx, "https://api.example.com/login", data)
	PostForm(ctx context.Context, url string, data url.Values) (*http.Response, error)

	// GetWithOptions performs an HTTP GET request with additional request options.
	// Options can modify headers, authentication, query parameters, timeouts, etc.
	//
	// Example:
	//	resp, err := client.GetWithOptions(ctx, "https://api.example.com/users",
	//	    httpclient.WithHeader("Authorization", "Bearer token"),
	//	    httpclient.WithQueryParams(url.Values{"page": {"1"}}),
	//	    httpclient.WithRequestTimeout(10*time.Second),
	//	)
	GetWithOptions(ctx context.Context, url string, opts ...RequestOption) (*http.Response, error)

	// PostWithOptions performs an HTTP POST request with additional request options.
	// Combines the flexibility of Post with the configurability of request options.
	//
	// Example:
	//	body := strings.NewReader(`{"key":"value"}`)
	//	resp, err := client.PostWithOptions(ctx, url, "application/json", body,
	//	    httpclient.WithHeader("X-Request-ID", "12345"),
	//	    httpclient.WithBearerToken("your-token"),
	//	)
	PostWithOptions(
		ctx context.Context,
		url, contentType string,
		body io.Reader,
		opts ...RequestOption,
	) (*http.Response, error)

	// PutWithOptions performs an HTTP PUT request with additional request options.
	// Useful for updates that require special headers or authentication.
	//
	// Example:
	//	body := strings.NewReader(`{"status":"active"}`)
	//	resp, err := client.PutWithOptions(ctx, url, "application/json", body,
	//	    httpclient.WithHeader("If-Match", etag),
	//	    httpclient.WithRequestTimeout(30*time.Second),
	//	)
	PutWithOptions(
		ctx context.Context,
		url, contentType string,
		body io.Reader,
		opts ...RequestOption,
	) (*http.Response, error)

	// DeleteWithOptions performs an HTTP DELETE request with additional request options.
	// Useful for deletions that require authentication or special headers.
	//
	// Example:
	//	resp, err := client.DeleteWithOptions(ctx, "https://api.example.com/users/123",
	//	    httpclient.WithHeader("Authorization", "Bearer token"),
	//	    httpclient.WithHeader("X-Reason", "user-requested"),
	//	)
	DeleteWithOptions(ctx context.Context, url string, opts ...RequestOption) (*http.Response, error)

	// NewRequest creates a new RequestBuilder for fluent API request building.
	// This allows chaining method calls to configure all aspects of an HTTP request
	// in a readable, expressive manner.
	//
	// The RequestBuilder supports:
	//   - All HTTP methods (GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS)
	//   - Multiple body types (JSON, form data, strings, bytes, io.Reader)
	//   - Headers and authentication (Basic Auth, Bearer tokens, API keys)
	//   - Query parameters
	//   - Per-request timeouts and deadlines
	//   - Custom contexts
	//
	// Example:
	//	resp, err := client.NewRequest().
	//	    POST("https://api.example.com/users").
	//	    JSONBody(user).
	//	    Header("Authorization", "Bearer token").
	//	    QueryParam("version", "v2").
	//	    Timeout(30 * time.Second).
	//	    Send(ctx)
	//
	// Complex example:
	//	req, err := client.NewRequest().
	//	    PUT("https://api.example.com/users/123").
	//	    JSONBody(updatedUser).
	//	    BasicAuth("username", "password").
	//	    Header("Content-Type", "application/json").
	//	    Header("X-Request-ID", uuid.New().String()).
	//	    QueryParamsFromMap(map[string]string{
	//	        "notify": "true",
	//	        "reason": "profile-update",
	//	    }).
	//	    Timeout(45 * time.Second).
	//	    Build()
	NewRequest() RequestBuilder
}

// httpClient wraps the standard http.Client to provide helper methods.
type httpClient struct {
	*http.Client
	// stringInterner provides efficient string interning for HTTP headers
	stringInterner *corestrings.Interner
}

type requestDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func doRequest(
	ctx context.Context,
	client requestDoer,
	method, url string,
	body io.Reader,
	contentType string,
	errContext string,
	opts ...RequestOption,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, coreerrs.Wrapf(err, "failed to create %s request for %s", method, url)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	for _, opt := range opts {
		opt(req)
	}

	if cancel := cancelFromContext(req.Context()); cancel != nil { //nolint:contextcheck // req.Context() inherits from ctx via NewRequestWithContext
		defer cancel()
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, coreerrs.Wrapf(err, "%s request%s failed for %s", method, errContext, url)
	}

	return resp, nil
}

func doJSONRequest(ctx context.Context, client requestDoer, method, url string, body any) (*http.Response, error) {
	buf := coreio.GetBuffer()
	defer coreio.PutBuffer(buf)

	encoder := json.NewEncoder(buf)
	if err := encoder.Encode(body); err != nil {
		return nil, coreerrs.WrapOperation(err, "marshal JSON body")
	}

	return doRequest(ctx, client, method, url, bytes.NewReader(buf.Bytes()), "application/json", "")
}

// NewHTTPClient creates a new [HTTPClient] with convenient helper methods for common HTTP operations.
// It wraps the standard *[net/http.Client] created by [New] and adds methods like Get, Post, Put, Delete, etc.
// The returned client is safe for concurrent use.
//
// Example:
//
//	client := httpclient.NewHTTPClient(
//	    httpclient.WithRetryMax(3),
//	    httpclient.WithLogger(logger),
//	)
//
//	resp, err := client.Get(ctx, "https://api.example.com/users")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer resp.Body.Close()
func NewHTTPClient(opt ...Option) HTTPClient {
	return &httpClient{
		Client: New(opt...),
		// Create dedicated string interner for HTTP client headers
		// HTTP client typically uses common headers repeatedly
		stringInterner: corestrings.NewInterner(HTTPClientInternerSize), // Smaller size for headers
	}
}

// Get performs an HTTP GET request to the specified URL.
// The context controls the request lifetime and can be used for timeouts and cancellation.
// Returns the HTTP response and any error encountered.
//
// Example:
//
//	resp, err := client.Get(ctx, "https://api.example.com/users")
//	if err != nil {
//	    return err
//	}
//	defer resp.Body.Close()
//
// Example with timeout:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	defer cancel()
//	resp, err := client.Get(ctx, "https://api.example.com/users")
func (c *httpClient) Get(ctx context.Context, url string) (*http.Response, error) {
	return doRequest(ctx, c, http.MethodGet, url, nil, "", "")
}

// Post issues a POST request to the specified URL with the given content type and body.
//
// Example:
//
//	body := strings.NewReader("name=John&age=30")
//	resp, err := client.Post(ctx, "https://api.example.com/users", "application/x-www-form-urlencoded", body)
func (c *httpClient) Post(ctx context.Context, url, contentType string, body io.Reader) (*http.Response, error) {
	return doRequest(ctx, c, http.MethodPost, url, body, contentType, "")
}

// PostJSON performs an HTTP POST request with a JSON-encoded body.
// The body parameter is automatically marshaled to JSON and the Content-Type header is set to "application/json".
// Returns the HTTP response and any error encountered, including JSON marshaling errors.
// Uses buffer pooling to reduce memory allocations.
//
// Example:
//
//	user := User{Name: "John", Email: "john@example.com"}
//	resp, err := client.PostJSON(ctx, "https://api.example.com/users", user)
//	if err != nil {
//	    return err
//	}
//	defer resp.Body.Close()
func (c *httpClient) PostJSON(ctx context.Context, url string, body any) (*http.Response, error) {
	return doJSONRequest(ctx, c, http.MethodPost, url, body)
}

// Put issues a PUT request to the specified URL.
func (c *httpClient) Put(ctx context.Context, url, contentType string, body io.Reader) (*http.Response, error) {
	return doRequest(ctx, c, http.MethodPut, url, body, contentType, "")
}

// PutJSON issues a PUT request with JSON body to the specified URL.
// Uses buffer pooling to reduce memory allocations.
func (c *httpClient) PutJSON(ctx context.Context, url string, body any) (*http.Response, error) {
	return doJSONRequest(ctx, c, http.MethodPut, url, body)
}

// Patch issues a PATCH request to the specified URL.
func (c *httpClient) Patch(ctx context.Context, url, contentType string, body io.Reader) (*http.Response, error) {
	return doRequest(ctx, c, http.MethodPatch, url, body, contentType, "")
}

// PatchJSON issues a PATCH request with JSON body to the specified URL.
// Uses buffer pooling to reduce memory allocations.
func (c *httpClient) PatchJSON(ctx context.Context, url string, body any) (*http.Response, error) {
	return doJSONRequest(ctx, c, http.MethodPatch, url, body)
}

// Delete issues a DELETE request to the specified URL.
//
// Example:
//
//	resp, err := client.Delete(ctx, "https://api.example.com/users/123")
func (c *httpClient) Delete(ctx context.Context, url string) (*http.Response, error) {
	return doRequest(ctx, c, http.MethodDelete, url, nil, "", "")
}

// Head issues a HEAD request to the specified URL.
func (c *httpClient) Head(ctx context.Context, url string) (*http.Response, error) {
	return doRequest(ctx, c, http.MethodHead, url, nil, "", "")
}

// PostForm issues a POST request with URL-encoded form data.
// Content-Type is automatically set to "application/x-www-form-urlencoded".
//
// Example:
//
//	data := url.Values{}
//	data.Set("username", "john")
//	data.Set("password", "secret")
//	resp, err := client.PostForm(ctx, "https://api.example.com/login", data)
func (c *httpClient) PostForm(ctx context.Context, url string, data url.Values) (*http.Response, error) {
	return c.Post(ctx, url, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
}

// GetWithOptions issues a GET request, applying each [RequestOption] to the
// outbound request before it is sent through the resilient client stack.
func (c *httpClient) GetWithOptions(ctx context.Context, url string, opts ...RequestOption) (*http.Response, error) {
	return doRequest(ctx, c, http.MethodGet, url, nil, "", " with options", opts...)
}

// PostWithOptions issues a POST request, applying each [RequestOption] to the
// outbound request before it is sent through the resilient client stack.
func (c *httpClient) PostWithOptions(
	ctx context.Context,
	url, contentType string,
	body io.Reader,
	opts ...RequestOption,
) (*http.Response, error) {
	return doRequest(ctx, c, http.MethodPost, url, body, contentType, " with options", opts...)
}

// PutWithOptions issues a PUT request, applying each [RequestOption] to the
// outbound request before it is sent through the resilient client stack.
func (c *httpClient) PutWithOptions(
	ctx context.Context,
	url, contentType string,
	body io.Reader,
	opts ...RequestOption,
) (*http.Response, error) {
	return doRequest(ctx, c, http.MethodPut, url, body, contentType, " with options", opts...)
}

// DeleteWithOptions issues a DELETE request, applying each [RequestOption] to
// the outbound request before it is sent through the resilient client stack.
func (c *httpClient) DeleteWithOptions(ctx context.Context, url string, opts ...RequestOption) (*http.Response, error) {
	return doRequest(ctx, c, http.MethodDelete, url, nil, "", " with options", opts...)
}

// NewRequest creates a new RequestBuilder for fluent API request building.
// This allows chaining method calls to configure all aspects of an HTTP request.
//
// Example:
//
//	resp, err := client.NewRequest().
//	    GET("https://api.example.com/users").
//	    Header("Authorization", "Bearer token").
//	    QueryParam("page", "1").
//	    Timeout(30 * time.Second).
//	    Send(ctx)
func (c *httpClient) NewRequest() RequestBuilder {
	return NewRequestBuilder(c)
}

// RequestOption is a function that mutates an [net/http.Request] before it is
// sent. Options are applied in order by the *WithOptions methods on [HTTPClient]
// (e.g., [HTTPClient.GetWithOptions]). See [WithHeader], [WithBearerToken],
// [WithQueryParams], [WithRequestTimeout], and [WithRequestDeadline] for
// commonly used options.
type RequestOption func(*http.Request)

// WithHeader adds a single header to the request.
//
// Example:
//
//	resp, err := client.GetWithOptions(ctx, url,
//	    httpclient.WithHeader("Authorization", "Bearer token"),
//	    httpclient.WithHeader("X-Request-ID", "123"),
//	)
func WithHeader(key, value string) RequestOption {
	return func(req *http.Request) {
		req.Header.Set(key, value)
	}
}

// WithHeaders adds multiple headers from an iterator of key-value pairs.
// Later entries overwrite earlier ones when keys collide.
func WithHeaders(seq iter.Seq2[string, string]) RequestOption {
	return func(req *http.Request) {
		for k, v := range seq {
			req.Header.Set(k, v)
		}
	}
}

// WithBasicAuth adds basic authentication to the request.
func WithBasicAuth(username, password string) RequestOption {
	return func(req *http.Request) {
		req.SetBasicAuth(username, password)
	}
}

// WithBearerToken adds an Authorization header with a Bearer token.
// Uses efficient string building to avoid unnecessary allocations.
//
// Example:
//
//	resp, err := client.GetWithOptions(ctx, url,
//	    httpclient.WithBearerToken("your-api-token"),
//	)
func WithBearerToken(token string) RequestOption {
	return func(req *http.Request) {
		req.Header.Set("Authorization", corestrings.BuildString(func(builder *strings.Builder) {
			builder.Grow(BearerPrefixLength + len(token)) // "Bearer " + token length
			builder.WriteString("Bearer ")
			builder.WriteString(token)
		}))
	}
}

// WithQueryParams merges the supplied query parameters into the request URL.
// Existing query parameters in the URL are preserved; new values are appended.
func WithQueryParams(params url.Values) RequestOption {
	return func(req *http.Request) {
		q := req.URL.Query()
		for k, values := range params {
			for _, v := range values {
				q.Add(k, v)
			}
		}
		req.URL.RawQuery = q.Encode()
	}
}

// WithRequestTimeout sets a timeout for an individual request.
// This is different from the client-level timeout and allows per-request customization.
// The timeout starts when the request begins and covers the entire request/response cycle.
//
// Example:
//
//	resp, err := client.GetWithOptions(ctx, url,
//	    httpclient.WithRequestTimeout(5*time.Second),
//	)
func WithRequestTimeout(timeout time.Duration) RequestOption {
	return func(req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), timeout)
		// Store the cancel function in the request context so it can be called
		// when the request is done (though usually the HTTP client handles this)
		ctx = context.WithValue(ctx, cancelContextKey, cancel)
		*req = *req.WithContext(ctx)
	}
}

// WithRequestDeadline sets an absolute deadline for the request. Like
// [WithRequestTimeout], the resulting [context.CancelFunc] is stored in the
// request context and invoked automatically after the response is received.
func WithRequestDeadline(deadline time.Time) RequestOption {
	return func(req *http.Request) {
		ctx, cancel := context.WithDeadline(req.Context(), deadline)
		ctx = context.WithValue(ctx, cancelContextKey, cancel)
		*req = *req.WithContext(ctx)
	}
}

// cancelContextKey is used to store the cancel function in the context.
type cancelContextKeyType struct{}

var cancelContextKey = cancelContextKeyType{}

func cancelFromContext(ctx context.Context) context.CancelFunc {
	if cancel, ok := ctx.Value(cancelContextKey).(context.CancelFunc); ok {
		return cancel
	}
	return nil
}
