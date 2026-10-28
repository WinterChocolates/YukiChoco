package YukiChoco

import (
	"context"
	"sync"
	"time"
)

// tokenBucket 是限流用的令牌桶。
//
// 没有后台 goroutine，只在取令牌时按流逝时间惰性补充，因此无常驻开销。
type tokenBucket struct {
	mu     sync.Mutex
	qps    float64
	burst  float64
	tokens float64
	last   time.Time
}

// newTokenBucket 按配置创建限流器；qps<=0 时返回 nil，表示不限流。
func newTokenBucket(cfg RateLimitConfig) *tokenBucket {
	if cfg.QPS <= 0 {
		return nil
	}
	burst := float64(cfg.Burst)
	if burst < 1 {
		burst = 1
	}
	return &tokenBucket{
		qps:    cfg.QPS,
		burst:  burst,
		tokens: burst,
		last:   time.Now(),
	}
}

// wait 阻塞直到取得一个令牌或 ctx 结束。nil 接收者表示不限流。
func (b *tokenBucket) wait(ctx context.Context) error {
	if b == nil {
		return ctx.Err()
	}
	for {
		b.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(b.last).Seconds()
		if elapsed > 0 {
			b.tokens += elapsed * b.qps
			if b.tokens > b.burst {
				b.tokens = b.burst
			}
			b.last = now
		}
		if b.tokens >= 1 {
			b.tokens--
			b.mu.Unlock()
			return ctx.Err()
		}
		need := (1 - b.tokens) / b.qps
		b.mu.Unlock()

		timer := time.NewTimer(time.Duration(need * float64(time.Second)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
