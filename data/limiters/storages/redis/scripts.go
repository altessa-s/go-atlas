// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import "github.com/redis/go-redis/v9"

// luaScript implements sliding window rate limiting using Redis Lua script.
// This ensures atomicity of the rate limiting operations.
var luaScript = redis.NewScript(`
local key = KEYS[1]
local window = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

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

-- Add current request
redis.call('zadd', key, now, now)
redis.call('expire', key, window)

-- Recalculate remaining after adding current request
remaining = math.max(0, limit - current - 1)

return {limit, remaining, reset_time, 1}  -- 1 = allowed
`)
