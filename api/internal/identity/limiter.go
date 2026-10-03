package identity

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// limiter allows max attempts per window for a key, counted in Redis so the
// limit holds across API instances.
type limiter struct {
	rdb    *redis.Client
	max    int64
	window time.Duration
}

// allow counts one attempt and reports whether it's within the limit, and
// if not, how long until the window resets.
func (l *limiter) allow(ctx context.Context, key string) (bool, time.Duration, error) {
	key = "ratelimit:" + key
	pipe := l.rdb.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, l.window)
	ttl := pipe.TTL(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, err
	}
	return count.Val() <= l.max, ttl.Val(), nil
}

func (l *limiter) reset(ctx context.Context, key string) {
	l.rdb.Del(ctx, "ratelimit:"+key)
}
