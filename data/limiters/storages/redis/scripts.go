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
//
// ARGV[1] (window) and ARGV[3] (now) are in milliseconds. Passing whole
// seconds truncated sub-second periods: a 500 ms window became 0 and
// EXPIRE 0 deleted every recorded request, so nothing was ever limited.
var luaScript = redis.NewScript(`
local key = KEYS[1]
local window = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local member = ARGV[4]

-- Remove expired entries
redis.call('zremrangebyscore', key, 0, now - window)

-- Count current requests
local current = redis.call('zcard', key)

-- Calculate remaining requests
local remaining = math.max(0, limit - current)
local reset_time = 0

if current > 0 then
    -- Get oldest request timestamp for reset calculation
    local oldest = redis.call('zrange', key, 0, 0, 'WITHSCORES')
    if #oldest > 0 then
        reset_time = math.floor((oldest[2] + window) / 1000)
    end
end

-- Check if limit exceeded
if current >= limit then
    return {limit, remaining, reset_time, 0}  -- 0 = not allowed
end

-- Add current request: score is the timestamp (used for pruning), member is
-- the caller-supplied nonce so concurrent same-ms ZADDs don't collapse.
redis.call('zadd', key, now, member)
redis.call('pexpire', key, window)

-- Recalculate remaining after adding current request
remaining = math.max(0, limit - current - 1)

-- Into an empty window the current request is the oldest one.
if current == 0 then
    reset_time = math.floor((now + window) / 1000)
end

return {limit, remaining, reset_time, 1}  -- 1 = allowed
`)

// luaScriptMulti admits one request against every key or against none, with
// the sliding window of luaScript per key: it prunes and counts every
// window first and adds to them only when all admit.
//
// ARGV[1] is now in milliseconds; key i (1-based) has its window, limit and
// unique member at ARGV[3i-1], ARGV[3i] and ARGV[3i+1]. The reply is the
// 1-based index of the first refusing key (0 when every key admitted),
// followed by remaining and reset per key.
var luaScriptMulti = redis.NewScript(`
local now = tonumber(ARGV[1])
local n = #KEYS
local windows, limits, counts, resets = {}, {}, {}, {}
local refused = 0

for i = 1, n do
    local key = KEYS[i]
    local window = tonumber(ARGV[3 * i - 1])
    windows[i] = window
    limits[i] = tonumber(ARGV[3 * i])

    redis.call('zremrangebyscore', key, 0, now - window)
    local current = redis.call('zcard', key)
    counts[i] = current

    resets[i] = 0
    if current > 0 then
        local oldest = redis.call('zrange', key, 0, 0, 'WITHSCORES')
        if #oldest > 0 then
            resets[i] = math.floor((oldest[2] + window) / 1000)
        end
    end

    if refused == 0 and current >= limits[i] then
        refused = i
    end
end

local reply = {refused}
if refused > 0 then
    for i = 1, n do
        reply[2 * i] = math.max(0, limits[i] - counts[i])
        reply[2 * i + 1] = resets[i]
    end
    return reply
end

for i = 1, n do
    local key = KEYS[i]
    redis.call('zadd', key, now, ARGV[3 * i + 1])
    redis.call('pexpire', key, windows[i])
    reply[2 * i] = math.max(0, limits[i] - counts[i] - 1)
    if counts[i] == 0 then
        resets[i] = math.floor((now + windows[i]) / 1000)
    end
    reply[2 * i + 1] = resets[i]
end
return reply
`)
