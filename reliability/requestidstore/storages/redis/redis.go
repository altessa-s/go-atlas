// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-aip/requestid"

	"github.com/altessa-s/go-atlas/internal/redis/redisbase"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// Storage keeps request ID entries in Redis hashes.
type Storage struct{ base redisbase.Base }

var _ requestid.Store = (*Storage)(nil)

// New uses client to store request IDs under the configured prefix.
func New(client redis.UniversalClient, opts ...Option) *Storage {
	return &Storage{base: redisbase.NewBase(client, newOptions(opts...).keyPrefix)}
}

// Both writers replace every field and its physical expiry together.
// ARGV: caller time, state, token, expiry, fingerprint, response, failure, omitted, TTL.
const writeEntry = `
redis.call('HSET', KEYS[1], 'state', ARGV[2], 'token', ARGV[3], 'expiry', ARGV[4],
  'fp', ARGV[5], 'response', ARGV[6], 'failure', ARGV[7], 'omitted', ARGV[8])
redis.call('PEXPIRE', KEYS[1], ARGV[9])
`

var reserve = redis.NewScript(`
local expiry = redis.call('HGET', KEYS[1], 'expiry')
if expiry and tonumber(expiry) > tonumber(ARGV[1]) then
  return redis.call('HMGET', KEYS[1], 'state', 'token', 'expiry', 'fp', 'response', 'failure', 'omitted')
end
` + writeEntry + `return {}`)

// Both guarded operations append the expected pending state and caller token.
const checkOwner = `
local r = redis.call('HMGET', KEYS[1], 'state', 'token', 'expiry')
if r[1] ~= ARGV[#ARGV-1] or r[2] ~= ARGV[#ARGV] or tonumber(r[3]) <= tonumber(ARGV[1]) then
  return 0
end
`

var complete = redis.NewScript(checkOwner + writeEntry + `return 1`)
var release = redis.NewScript(checkOwner + `return redis.call('DEL', KEYS[1])`)

func entryArgs(e requestid.Entry, now time.Time) []any {
	expiry := e.ExpiresAt.UnixMilli()
	// Relative TTL keeps data available even when caller and server epochs differ.
	const grace = time.Minute
	ttl := max(int64(0), expiry-now.UnixMilli()) + grace.Milliseconds()
	return []any{now.UnixMilli(), int(e.State), e.Token, expiry, e.Fingerprint, e.Response, e.Failure, e.ResponseOmitted, ttl}
}

// Reserve atomically returns the live entry or installs pending.
func (s *Storage) Reserve(ctx context.Context, key string, pending requestid.Entry, now time.Time) (requestid.Entry, bool, error) {
	if err := ctx.Err(); err != nil {
		return requestid.Entry{}, false, err
	}
	fields, err := reserve.Run(ctx, s.base.Client(), []string{s.base.Key(key)}, entryArgs(pending, now)...).StringSlice()
	if err != nil {
		return requestid.Entry{}, false, coreerrs.WrapOperation(err, "reserve Redis request ID")
	}
	if len(fields) == 0 {
		return pending, true, nil
	}
	// Field order matches the HMGET in the reserve script.
	const (
		stateField = iota
		tokenField
		expiryField
		fingerprintField
		responseField
		failureField
		omittedField
	)
	state, err := strconv.Atoi(fields[stateField])
	if err != nil {
		return requestid.Entry{}, false, coreerrs.WrapOperation(err, "decode request ID state")
	}
	expiry, err := strconv.ParseInt(fields[expiryField], 10, 64)
	if err != nil {
		return requestid.Entry{}, false, coreerrs.WrapOperation(err, "decode request ID expiry")
	}
	return requestid.Entry{
		State:           requestid.State(state),
		Token:           fields[tokenField],
		ExpiresAt:       time.UnixMilli(expiry).UTC(),
		Fingerprint:     []byte(fields[fingerprintField]),
		Response:        []byte(fields[responseField]),
		Failure:         []byte(fields[failureField]),
		ResponseOmitted: fields[omittedField] == "1",
	}, false, nil
}

// Complete replaces the owned pending entry and renews its physical TTL.
func (s *Storage) Complete(ctx context.Context, key, token string, done requestid.Entry, now time.Time) error {
	return s.owned(ctx, complete, key, token, entryArgs(done, now))
}

// Release deletes the owned pending entry.
func (s *Storage) Release(ctx context.Context, key, token string, now time.Time) error {
	return s.owned(ctx, release, key, token, []any{now.UnixMilli()})
}

func (s *Storage) owned(ctx context.Context, script *redis.Script, key, token string, args []any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	args = append(args, int(requestid.StatePending), token)
	n, err := script.Run(ctx, s.base.Client(), []string{s.base.Key(key)}, args...).Int()
	if err != nil {
		return coreerrs.WrapOperation(err, "update owned Redis request ID")
	}
	if n == 0 {
		return requestid.ErrReservationLost
	}
	return nil
}
