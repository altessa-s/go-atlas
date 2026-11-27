// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package s3probe

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

const (
	// DefaultRegion is the signing region used when a request names none.
	DefaultRegion = "us-east-1"
	// DefaultTimeout bounds a request that names no timeout.
	DefaultTimeout = 10 * time.Second

	signingService = "s3"
	// emptyPayloadHash is the hex SHA-256 of the empty body of HeadBucket.
	emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	// maxDrainBytes bounds how much of a response body is read before it is closed.
	maxDrainBytes = 4 << 10
)

// Request describes one bucket check.
type Request struct {
	// Endpoint is the service URL, http or https, without the bucket.
	Endpoint string
	// Bucket is the bucket name, addressed path-style.
	Bucket string
	// AccessKey is the access key id.
	AccessKey string
	// SecretKey is the secret access key. It is never logged or placed in errors.
	SecretKey string
	// Region is the signing region; empty means [DefaultRegion].
	Region string
	// Timeout bounds the whole call; zero means [DefaultTimeout].
	Timeout time.Duration
	// Trusted selects the egress policy: true for a stored configuration
	// (the configured policy), false for a draft (public addresses only).
	Trusted bool
}

// Result is the answer of the service to HeadBucket.
type Result struct {
	// StatusCode is the HTTP status of the response. Redirects are not
	// followed, so a 3xx status is reported as is.
	StatusCode int
}

// Prober sends bucket checks. It is safe for concurrent use.
type Prober struct {
	trusted   *http.Client
	untrusted *http.Client
	signer    *v4.Signer
}

// New returns a Prober whose trusted checks run under policy. A nil policy
// leaves trusted checks on the default protection, which admits public
// addresses only.
func New(policy *httpclient.EgressPolicy) *Prober {
	base := []httpclient.Option{
		httpclient.WithoutRetry(),
		httpclient.WithoutRedirects(),
	}

	trusted := base
	if policy != nil {
		trusted = append([]httpclient.Option{httpclient.WithEgressPolicy(policy)}, base...)
	}

	return &Prober{
		trusted:   httpclient.New(trusted...),
		untrusted: httpclient.New(base...),
		signer: v4.NewSigner(func(o *v4.SignerOptions) {
			o.DisableURIPathEscaping = true
		}),
	}
}

// HeadBucket checks access to the bucket of req with a signed HeadBucket
// request. It returns the HTTP status of the answer, or a [*TransportError]
// when there was none, or [ErrInvalidRequest] when the request is malformed.
func (p *Prober) HeadBucket(ctx context.Context, req Request) (Result, error) {
	target, err := bucketURL(req)
	if err != nil {
		return Result{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, cmp.Or(req.Timeout, DefaultTimeout))
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodHead, target, http.NoBody)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err) //nolint:errorlint // the cause is a detail, not a class
	}

	httpReq.Header.Set("X-Amz-Content-Sha256", emptyPayloadHash)

	creds := aws.Credentials{AccessKeyID: req.AccessKey, SecretAccessKey: req.SecretKey}
	region := cmp.Or(req.Region, DefaultRegion)

	if err = p.signer.SignHTTP(ctx, creds, httpReq, emptyPayloadHash, signingService, region, time.Now()); err != nil {
		return Result{}, fmt.Errorf("%w: sign: %v", ErrInvalidRequest, err) //nolint:errorlint // the cause is a detail, not a class
	}

	client := p.untrusted
	if req.Trusted {
		client = p.trusted
	}

	resp, err := client.Do(httpReq) //nolint:bodyclose // closed by httpclient.CloseBody below
	if err != nil {
		return Result{}, classify(err)
	}
	defer httpclient.CloseBody(resp.Body)

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))

	return Result{StatusCode: resp.StatusCode}, nil
}

// bucketURL builds the path-style bucket URL and validates the request.
func bucketURL(req Request) (string, error) {
	switch {
	case req.AccessKey == "" || req.SecretKey == "":
		return "", fmt.Errorf("%w: access keys are required", ErrInvalidRequest)
	case req.Bucket == "" || strings.ContainsAny(req.Bucket, "/?#"):
		return "", fmt.Errorf("%w: bucket name is required", ErrInvalidRequest)
	}

	u, err := url.Parse(req.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return "", fmt.Errorf("%w: endpoint must be an http or https URL", ErrInvalidRequest)
	}

	u.RawQuery, u.Fragment = "", ""

	return u.JoinPath(req.Bucket).String(), nil
}
