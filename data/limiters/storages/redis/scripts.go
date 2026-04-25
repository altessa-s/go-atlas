// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import "github.com/redis/go-redis/v9"

// luaScript implements sliding window rate limiting using Redis Lua script.
// This ensures atomicity of the rate limiting operations.
//
// ARGV[4] (member) MUST be unique per call. ZSET members are unique by value,
// so reusing the timestamp as both score and member would silently collapse
// multiple same-millisecond requests into a single entry and let bursts bypass
// the limit. The caller passes a random nonce; the score keeps the request
// timestamp for window pruning and reset calculation.
var luaScript = redis.NewScript(`
local key = KEYS[1]
local window = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local member = ARGV[4]

-- Remove expired entries
redis.call('zremrangebyscore', key, 0, now - window * 1000)

-- Count current requests
local current = redis.call('zcard', key)

-- Calculate remaining requests
local remaining = math.max(0, limit - current)
local reset_time = 0

if current > 0 then
    -- Get oldest request timestamp for reset calculation
    local oldest = redis.call('zrange', key, 0, 0, 'WITHSCORES')
    if #oldest > 0 then
        reset_time = math.floor((oldest[2] + window * 1000) / 1000)
    end
end

-- Check if limit exceeded
if current >= limit then
    return {limit, remaining, reset_time, 0}  -- 0 = not allowed
end

-- Add current request: score is the timestamp (used for pruning), member is
-- the caller-supplied nonce so concurrent same-ms ZADDs don't collapse.
redis.call('zadd', key, now, member)
redis.call('expire', key, window)

-- Recalculate remaining after adding current request
remaining = math.max(0, limit - current - 1)

return {limit, remaining, reset_time, 1}  -- 1 = allowed
`)
