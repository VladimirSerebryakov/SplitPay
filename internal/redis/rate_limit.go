package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

type RateLimiterStorage struct {
	client *goredis.Client
}

func NewRateLimiterStorage(client *goredis.Client) *RateLimiterStorage {
	return &RateLimiterStorage{client: client}
}

func (r *RateLimiterStorage) Increment(ctx context.Context, userID uuid.UUID, window time.Duration) (int64, time.Duration, error) {
	key := "ratelimiter:user:" + userID.String()

	script := goredis.NewScript(`
	local count = redis.call('INCR', KEYS[1])
	if count == 1 then
		redis.call('PEXPIRE', KEYS[1], ARGV[1])
	end
	return {count, redis.call('PTTL', KEYS[1])}
	`)

	value, err := script.Run(
		ctx, r.client, []string{key}, window.Milliseconds(),
	).Int64Slice()

	if err != nil {
		return 0, 0, fmt.Errorf("failed to increment rate limit: %w", err)
	}

	if len(value) != 2 || value[0] < 1 || value[1] < 0 {
		return 0, 0, fmt.Errorf("failed to increment rate limit")
	}
	count := value[0]
	duration := time.Duration(value[1]) * time.Millisecond
	return count, duration, nil
}
