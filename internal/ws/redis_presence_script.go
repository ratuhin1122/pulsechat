package ws

// All state changes and transition publications share a single Redis operation.
// Scores use Redis server time, avoiding clock skew between API instances.
const redisPresenceScript = `
local mode, user, connection = ARGV[1], ARGV[2], ARGV[3]
local ttl, channel = tonumber(ARGV[4]), ARGV[5]
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local function emit(online)
    redis.call('PUBLISH', channel, cjson.encode({presence={type='presence', user_id=user, online=online}}))
end
local before = redis.call('ZCARD', KEYS[1])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)
local count = redis.call('ZCARD', KEYS[1])
if before > 0 and count == 0 then emit(false) end
local result = 0
if mode == 'connect' then
    local owner = redis.call('GET', KEYS[2])
    if owner and owner ~= user then return redis.error_reply('connection owner mismatch') end
    redis.call('SET', KEYS[2], user, 'PX', ttl)
    redis.call('ZADD', KEYS[1], now + ttl, connection)
    if count == 0 then emit(true); result = 1 end
elseif mode == 'disconnect' then
    if redis.call('GET', KEYS[2]) == user then redis.call('DEL', KEYS[2]) end
    local removed = redis.call('ZREM', KEYS[1], connection)
    if removed > 0 and redis.call('ZCARD', KEYS[1]) == 0 then emit(false); result = 1 end
elseif mode == 'refresh' then
    if redis.call('GET', KEYS[2]) ~= user or not redis.call('ZSCORE', KEYS[1], connection) then
        result = -1
    else
        redis.call('PEXPIRE', KEYS[2], ttl)
        redis.call('ZADD', KEYS[1], now + ttl, connection)
    end
elseif mode == 'count' then
    result = count
end
local next_expiry = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
if #next_expiry == 0 then
    redis.call('DEL', KEYS[1])
    redis.call('ZREM', KEYS[3], user)
else
    redis.call('ZADD', KEYS[3], next_expiry[2], user)
end
return result
`
