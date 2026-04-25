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
	"sync"
	"time"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreio "github.com/altessa-s/go-atlas/core/io"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

const (
	// DefaultHeadersCapacity is the starting capacity for common headers
	DefaultHeadersCapacity = 4
	// DefaultQueryParamsCapacity is the starting capacity for common query parameters
	DefaultQueryParamsCapacity = 2
	// BearerPrefixLength is the length of "Bearer " prefix (7 chars)
	BearerPrefixLength = 7
	// DefaultJSONBufferCapacity is the starting capacity for JSON buffers (512 bytes)
	DefaultJSONBufferCapacity = 512
	// HeaderInternerSize is the size of the string interner for header keys
	HeaderInternerSize = 256
)

var (
	// internedHeaderKeys contains pre-interned common header keys for memory optimization
	internedHeaderKeys = map[string]string{
		"Content-Type":  internHeaderKey("Content-Type"),
		"Accept":        internHeaderKey("Accept"),
		"User-Agent":    internHeaderKey("User-Agent"),
		"Authorization": internHeaderKey("Authorization"),
	}
)

// internHeaderKey returns an interned version of the header key for memory optimization.
// Uses a dedicated string interner for HTTP headers to ensure identical header keys share the same memory location.
// Provides LRU eviction and hot caching specifically for HTTP header keys.
// This is especially useful for common headers like "Content-Type", "Authorization", "User-Agent".
func internHeaderKey(key string) string {
	return headerInterner.String(key)
}

// RequestBuilder provides a fluent interface for building HTTP requests.
// It allows chaining method calls to configure all aspects of an HTTP request
// including headers, query parameters, body content, timeouts, and authentication.
//
// Instances are drawn from a [sync.Pool] and recycled automatically by [RequestBuilder.Send].
// When using [RequestBuilder.Build] or [RequestBuilder.BuildWithContext] instead, the caller
// must call [RequestBuilder.Release] when the builder is no longer needed. Release also
// zeroes all credential fields ([RequestBuilder.BasicAuth], [RequestBuilder.BearerToken],
// [RequestBuilder.APIKey]) to avoid leaking secrets through the pool.
//
// Example:
//
//	resp, err := client.NewRequest().
//	    GET("https://api.example.com/users").
//	    Header("Authorization", "Bearer token").
//	    QueryParam("page", "1").
//	    Timeout(30 * time.Second).
//	    Send(ctx)
type RequestBuilder interface {
	// GET sets the HTTP method to GET and the target URL.
	GET(url string) RequestBuilder
	// POST sets the HTTP method to POST and the target URL.
	POST(url string) RequestBuilder
	// PUT sets the HTTP method to PUT and the target URL.
	PUT(url string) RequestBuilder
	// PATCH sets the HTTP method to PATCH and the target URL.
	PATCH(url string) RequestBuilder
	// DELETE sets the HTTP method to DELETE and the target URL.
	DELETE(url string) RequestBuilder
	// HEAD sets the HTTP method to HEAD and the target URL.
	HEAD(url string) RequestBuilder
	// OPTIONS sets the HTTP method to OPTIONS and the target URL.
	OPTIONS(url string) RequestBuilder
	// Method sets a custom HTTP method and target URL.
	Method(method, url string) RequestBuilder

	// Body sets the request body from an io.Reader.
	Body(body io.Reader) RequestBuilder
	// JSONBody marshals the provided value to JSON and sets it as the request body.
	JSONBody(body any) RequestBuilder
	// FormBody sets form-encoded data as the request body.
	FormBody(data url.Values) RequestBuilder
	// StringBody sets a string as the request body.
	StringBody(body string) RequestBuilder
	// BytesBody sets a byte slice as the request body.
	BytesBody(body []byte) RequestBuilder

	// Header sets a single header.
	Header(key, value string) RequestBuilder
	// Headers sets multiple headers from an iterator.
	Headers(seq iter.Seq2[string, string]) RequestBuilder
	// ContentType sets the Content-Type header.
	ContentType(contentType string) RequestBuilder
	// Accept sets the Accept header.
	Accept(accept string) RequestBuilder
	// UserAgent sets the User-Agent header.
	UserAgent(userAgent string) RequestBuilder

	// Authentication

	// BasicAuth sets basic authentication.
	BasicAuth(username, password string) RequestBuilder
	// BearerToken sets the Authorization header with a Bearer token.
	BearerToken(token string) RequestBuilder
	// APIKey sets a custom header for API key authentication.
	APIKey(key, value string) RequestBuilder

	// Query parameters

	// QueryParam adds a single query parameter.
	QueryParam(key, value string) RequestBuilder
	// QueryParams adds multiple query parameters from url.Values.
	QueryParams(params url.Values) RequestBuilder
	// QueryParamsSeq adds query parameters from an iterator.
	QueryParamsSeq(seq iter.Seq2[string, string]) RequestBuilder

	// Request configuration

	// Timeout sets a timeout for this specific request.
	Timeout(timeout time.Duration) RequestBuilder
	// Deadline sets a deadline for this specific request.
	Deadline(deadline time.Time) RequestBuilder
	// Context sets a custom context for the request.
	Context(ctx context.Context) RequestBuilder

	// Execution

	// Send executes the HTTP request with the provided context.
	Send(ctx context.Context) (*http.Response, error)
	// SendWithoutContext executes the HTTP request using context.Background().
	SendWithoutContext() (*http.Response, error)
	// Build creates an http.Request with context.Background().
	Build() (*http.Request, error)
	// BuildWithContext creates an http.Request with the specified context.
	BuildWithContext(ctx context.Context) (*http.Request, error)

	// Release returns the builder to the pool and clears any sensitive data.
	// This is called automatically by Send(), but must be called manually if using Build().
	Release()
}

// requestBuilder implements the RequestBuilder interface.
type requestBuilder struct {
	client        HTTPClient
	method        string
	url           string
	body          io.Reader
	headers       map[string]string
	queryParams   url.Values
	timeout       *time.Duration
	deadline      *time.Time
	ctx           context.Context
	basicAuthUser *corestrings.SecureString
	basicAuthPass *corestrings.SecureString
	bearerToken   *corestrings.SecureString
	apiKeyKey     string
	apiKeyValue   *corestrings.SecureString
}

// requestBuilderPool provides pooled requestBuilder instances to reduce allocations
var requestBuilderPool = sync.Pool{
	New: func() any {
		headers := coremaps.NewPool[string, string](DefaultHeadersCapacity).Get()
		queryParams := make(url.Values, DefaultQueryParamsCapacity)
		return &requestBuilder{
			headers:     *headers,
			queryParams: queryParams,
		}
	},
}

// headerPool provides pooled header maps to reduce allocations
var headerPool = coremaps.NewPool[string, string](DefaultHeadersCapacity)

// headerInterner provides efficient string interning for HTTP header keys
var headerInterner = corestrings.NewInterner(HeaderInternerSize) // Small size for common headers

// NewRequestBuilder creates a new RequestBuilder instance.
// This is typically called through the HTTPClient.NewRequest() method.
// Uses object pooling to reduce allocations.
func NewRequestBuilder(client HTTPClient) RequestBuilder {
	rb, ok := requestBuilderPool.Get().(*requestBuilder)
	if !ok {
		// Fallback: create new instance if pool returns unexpected type
		headers := headerPool.Get()
		rb = &requestBuilder{
			headers:     *headers,
			queryParams: make(url.Values, DefaultQueryParamsCapacity),
		}
	}

	// The builder should already be cleared by reset() before being put into the pool,
	// but we perform a safety reset here just in case.
	rb.reset(client)

	return rb
}

func (rb *requestBuilder) reset(client HTTPClient) {
	rb.client = client
	rb.method = ""
	rb.url = ""
	rb.body = nil
	rb.timeout = nil
	rb.deadline = nil
	rb.ctx = nil

	// Clear secure fields
	if rb.basicAuthUser != nil {
		rb.basicAuthUser.Clear()
		rb.basicAuthUser = nil
	}
	if rb.basicAuthPass != nil {
		rb.basicAuthPass.Clear()
		rb.basicAuthPass = nil
	}
	if rb.bearerToken != nil {
		rb.bearerToken.Clear()
		rb.bearerToken = nil
	}
	rb.apiKeyKey = ""
	if rb.apiKeyValue != nil {
		rb.apiKeyValue.Clear()
		rb.apiKeyValue = nil
	}

	// Clear maps efficiently
	clear(rb.headers)
	clear(rb.queryParams)
}

// GET sets the HTTP method to GET and the target URL.
func (rb *requestBuilder) GET(url string) RequestBuilder {
	rb.method = http.MethodGet
	rb.url = url
	return rb
}

// POST sets the HTTP method to POST and the target URL.
func (rb *requestBuilder) POST(url string) RequestBuilder {
	rb.method = http.MethodPost
	rb.url = url
	return rb
}

// PUT sets the HTTP method to PUT and the target URL.
func (rb *requestBuilder) PUT(url string) RequestBuilder {
	rb.method = http.MethodPut
	rb.url = url
	return rb
}

// PATCH sets the HTTP method to PATCH and the target URL.
func (rb *requestBuilder) PATCH(url string) RequestBuilder {
	rb.method = http.MethodPatch
	rb.url = url
	return rb
}

// DELETE sets the HTTP method to DELETE and the target URL.
func (rb *requestBuilder) DELETE(url string) RequestBuilder {
	rb.method = http.MethodDelete
	rb.url = url
	return rb
}

// HEAD sets the HTTP method to HEAD and the target URL.
func (rb *requestBuilder) HEAD(url string) RequestBuilder {
	rb.method = http.MethodHead
	rb.url = url
	return rb
}

// OPTIONS sets the HTTP method to OPTIONS and the target URL.
func (rb *requestBuilder) OPTIONS(url string) RequestBuilder {
	rb.method = http.MethodOptions
	rb.url = url
	return rb
}

// Method sets a custom HTTP method and target URL.
func (rb *requestBuilder) Method(method, url string) RequestBuilder {
	rb.method = method
	rb.url = url
	return rb
}

// Body sets the request body from an io.Reader.
func (rb *requestBuilder) Body(body io.Reader) RequestBuilder {
	rb.body = body
	return rb
}

// JSONBody marshals the provided value to JSON and sets it as the request body.
// Also automatically sets Content-Type to "application/json".
// If marshaling fails, the error is deferred and surfaced when [RequestBuilder.Send]
// or [RequestBuilder.BuildWithContext] is called.
func (rb *requestBuilder) JSONBody(body any) RequestBuilder {
	buf := coreio.GetBuffer()
	encoder := json.NewEncoder(buf)
	if err := encoder.Encode(body); err != nil {
		coreio.PutBuffer(buf)
		// Store error in a way that will be returned during Send()
		rb.body = coreio.NewErrorReader(coreerrs.WrapOperation(err, "marshal JSON body"))
		return rb
	}

	// Create a copy of the buffer data since we're returning the buffer to the pool
	data := make([]byte, buf.Len())
	copy(data, buf.Bytes())
	coreio.PutBuffer(buf)

	rb.body = bytes.NewReader(data)
	rb.headers[internedHeaderKeys["Content-Type"]] = "application/json"
	return rb
}

// FormBody sets form-encoded data as the request body.
// Also, automatically sets Content-Type to "application/x-www-form-urlencoded".
func (rb *requestBuilder) FormBody(data url.Values) RequestBuilder {
	rb.body = strings.NewReader(data.Encode())
	rb.headers[internedHeaderKeys["Content-Type"]] = "application/x-www-form-urlencoded"
	return rb
}

// StringBody sets a string as the request body.
func (rb *requestBuilder) StringBody(body string) RequestBuilder {
	rb.body = strings.NewReader(body)
	return rb
}

// BytesBody sets a byte slice as the request body.
func (rb *requestBuilder) BytesBody(body []byte) RequestBuilder {
	rb.body = bytes.NewReader(body)
	return rb
}

// Header sets a single header.
func (rb *requestBuilder) Header(key, value string) RequestBuilder {
	rb.headers[internHeaderKey(key)] = value
	return rb
}

// Headers sets multiple headers from an iterator.
func (rb *requestBuilder) Headers(seq iter.Seq2[string, string]) RequestBuilder {
	for k, v := range seq {
		rb.headers[internHeaderKey(k)] = v
	}
	return rb
}

// ContentType sets the Content-Type header.
func (rb *requestBuilder) ContentType(contentType string) RequestBuilder {
	rb.headers[internedHeaderKeys["Content-Type"]] = contentType
	return rb
}

// Accept sets the Accept header.
func (rb *requestBuilder) Accept(accept string) RequestBuilder {
	rb.headers[internedHeaderKeys["Accept"]] = accept
	return rb
}

// UserAgent sets the User-Agent header.
func (rb *requestBuilder) UserAgent(userAgent string) RequestBuilder {
	rb.headers[internedHeaderKeys["User-Agent"]] = userAgent
	return rb
}

// BasicAuth sets basic authentication.
func (rb *requestBuilder) BasicAuth(username, password string) RequestBuilder {
	// We'll store these and apply them during Build()
	rb.basicAuthUser = corestrings.NewSecureString(username)
	rb.basicAuthPass = corestrings.NewSecureString(password)
	return rb
}

// BearerToken sets the Authorization header with a Bearer token.
// Uses efficient string building to avoid unnecessary allocations.
func (rb *requestBuilder) BearerToken(token string) RequestBuilder {
	rb.bearerToken = corestrings.NewSecureString(token)
	return rb
}

// APIKey sets a custom header for API key authentication.
func (rb *requestBuilder) APIKey(key, value string) RequestBuilder {
	rb.apiKeyKey = key
	rb.apiKeyValue = corestrings.NewSecureString(value)
	return rb
}

// QueryParam adds a single query parameter.
func (rb *requestBuilder) QueryParam(key, value string) RequestBuilder {
	rb.queryParams.Add(key, value)
	return rb
}

// QueryParams adds multiple query parameters from url.Values.
func (rb *requestBuilder) QueryParams(params url.Values) RequestBuilder {
	for k, values := range params {
		for _, v := range values {
			rb.queryParams.Add(k, v)
		}
	}
	return rb
}

// QueryParamsSeq adds query parameters from an iterator.
func (rb *requestBuilder) QueryParamsSeq(seq iter.Seq2[string, string]) RequestBuilder {
	for k, v := range seq {
		rb.queryParams.Add(k, v)
	}
	return rb
}

// Timeout sets a timeout for this specific request.
func (rb *requestBuilder) Timeout(timeout time.Duration) RequestBuilder {
	rb.timeout = &timeout
	return rb
}

// Deadline sets a deadline for this specific request.
func (rb *requestBuilder) Deadline(deadline time.Time) RequestBuilder {
	rb.deadline = &deadline
	return rb
}

// Context sets a custom context for the request.
func (rb *requestBuilder) Context(ctx context.Context) RequestBuilder {
	rb.ctx = ctx
	return rb
}

// Send builds and executes the HTTP request. It returns the builder to the pool
// after execution regardless of success or failure, so the builder must not be
// reused after this call.
func (rb *requestBuilder) Send(ctx context.Context) (*http.Response, error) {
	req, err := rb.BuildWithContext(ctx)
	if err != nil {
		rb.Release()
		return nil, coreerrs.WrapOperation(err, "build request")
	}
	if cancel := cancelFromContext(req.Context()); cancel != nil { //nolint:contextcheck // context attached by builder
		defer cancel()
	}
	resp, err := rb.client.Do(req)
	rb.Release()
	if err != nil {
		return nil, coreerrs.Wrapf(err, "request execution failed for %s %s", req.Method, req.URL.String())
	}
	return resp, nil
}

// Release returns the builder to the pool and clears any sensitive data.
func (rb *requestBuilder) Release() {
	rb.reset(nil)
	requestBuilderPool.Put(rb)
}

// SendWithoutContext executes the HTTP request using context.Background().
func (rb *requestBuilder) SendWithoutContext() (*http.Response, error) {
	// Note: rb.Send() handles returning to pool
	return rb.Send(context.Background())
}

// Build creates an http.Request with context.Background().
// Note: Build does not return the requestBuilder to the pool since the caller
// may want to reuse it. Use Send() or SendWithoutContext() for automatic pooling.
func (rb *requestBuilder) Build() (*http.Request, error) {
	return rb.BuildWithContext(context.Background())
}

// BuildWithContext creates an [net/http.Request] from the builder state using the given context.
// Unlike [RequestBuilder.Send], it does not return the builder to the pool; the caller must
// call [RequestBuilder.Release] when finished. If a per-request [RequestBuilder.Timeout] or
// [RequestBuilder.Deadline] was set, the returned request's context will carry a
// [context.CancelFunc] that is called automatically by [RequestBuilder.Send] but must be
// managed by the caller when using BuildWithContext directly.
func (rb *requestBuilder) BuildWithContext(ctx context.Context) (*http.Request, error) { //nolint:contextcheck
	if rb.method == "" {
		rb.method = http.MethodGet
	}
	if rb.url == "" {
		return nil, &RequestBuilderError{Message: "URL is required"}
	}

	// Check for body errors
	if rb.body != nil {
		if errReader, ok := rb.body.(*coreio.ErrorReader); ok {
			return nil, errReader.Err
		}
	}

	// Use provided context or builder's context or background
	// If a builder has a context set, use it unless overridden by parameter
	if rb.ctx != nil && ctx == nil {
		ctx = rb.ctx
	} else {
		ctx = corecontext.OrBackground(ctx)
	}

	// Apply timeout or deadline to context
	if rb.timeout != nil {
		var cancel context.CancelFunc
		ctx, cancel = corecontext.WithMaxTimeout(ctx, *rb.timeout)
		// Store the cancel function in the context so it can be retrieved later if needed
		ctx = context.WithValue(ctx, cancelContextKey, cancel)
	} else if rb.deadline != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, *rb.deadline)
		// Store the cancel function in the context so it can be retrieved later if needed
		ctx = context.WithValue(ctx, cancelContextKey, cancel)
	}

	req, err := http.NewRequestWithContext(ctx, rb.method, rb.url, rb.body)
	if err != nil {
		return nil, coreerrs.Wrapf(err, "failed to create %s request for %s", rb.method, rb.url)
	}

	// Set headers
	for k, v := range rb.headers {
		req.Header.Set(k, v)
	}

	// Apply secure authentication
	if rb.basicAuthUser != nil && rb.basicAuthPass != nil {
		req.SetBasicAuth(rb.basicAuthUser.String(), rb.basicAuthPass.String())
	}

	if rb.bearerToken != nil {
		token := rb.bearerToken.String()
		req.Header.Set(internedHeaderKeys["Authorization"], corestrings.BuildString(func(builder *strings.Builder) {
			builder.Grow(BearerPrefixLength + len(token))
			builder.WriteString("Bearer ")
			builder.WriteString(token)
		}))
	}

	if rb.apiKeyKey != "" && rb.apiKeyValue != nil {
		req.Header.Set(rb.apiKeyKey, rb.apiKeyValue.String())
	}

	// Set query parameters
	if len(rb.queryParams) > 0 {
		if req.URL.RawQuery != "" {
			// Parse existing query parameters
			existing, err := url.ParseQuery(req.URL.RawQuery)
			if err != nil {
				return nil, coreerrs.Wrapf(err, "failed to parse existing query parameters for %s", rb.url)
			}
			// Merge with new parameters
			for k, values := range rb.queryParams {
				for _, v := range values {
					existing.Add(k, v)
				}
			}
			req.URL.RawQuery = existing.Encode()
		} else {
			req.URL.RawQuery = rb.queryParams.Encode()
		}
	}

	return req, nil
}
