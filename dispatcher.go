package main

import (
	"context"
	"sync"
	"time"
)

// job is a unit of work that calls the Slack API.
type job struct {
	eventID string // hashed
	kind    string
	run     func(ctx context.Context) error
}

// dispatcher runs jobs on a fixed number of workers, so that a slow or
// unresponsive Slack API does not stop the loop that reads and acks events.
// The queue is bounded: when it is full, new jobs are dropped.
type dispatcher struct {
	queue   chan job
	timeout time.Duration
	logf    func(format string, v ...interface{})
	wg      sync.WaitGroup
}

func newDispatcher(workers, queueSize int, timeout time.Duration, logf func(string, ...interface{})) *dispatcher {
	d := &dispatcher{
		queue:   make(chan job, queueSize),
		timeout: timeout,
		logf:    logf,
	}
	for i := 0; i < workers; i++ {
		d.wg.Add(1)
		go d.work()
	}
	return d
}

// submit enqueues a job without blocking. It returns false when the job
// was dropped because of rate limiting or a full queue.
// Each kind of job has its own limiter, so that a flood of one kind
// (for example mentions from members) cannot use up the budget of another.
func (d *dispatcher) submit(j job, limiter *rateLimiter) bool {
	if limiter != nil && !limiter.allow() {
		d.logf("dropped: kind=%s event=%s reason=rate_limited", j.kind, j.eventID)
		return false
	}
	select {
	case d.queue <- j:
		return true
	default:
		if limiter != nil {
			limiter.refund()
		}
		d.logf("dropped: kind=%s event=%s reason=queue_full", j.kind, j.eventID)
		return false
	}
}

func (d *dispatcher) work() {
	defer d.wg.Done()
	for j := range d.queue {
		ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
		err := j.run(ctx)
		cancel()
		if err != nil {
			d.logf("failed: kind=%s event=%s error=%v", j.kind, j.eventID, err)
			continue
		}
		d.logf("done: kind=%s event=%s", j.kind, j.eventID)
	}
}

// close stops accepting jobs and waits for running ones.
func (d *dispatcher) close() {
	close(d.queue)
	d.wg.Wait()
}

// rateLimiter is a token bucket. It avoids an extra dependency.
type rateLimiter struct {
	mu     sync.Mutex
	tokens float64
	burst  float64
	rate   float64 // tokens per second
	last   time.Time
	now    func() time.Time
}

func newRateLimiter(ratePerSecond float64, burst int) *rateLimiter {
	return &rateLimiter{
		tokens: float64(burst),
		burst:  float64(burst),
		rate:   ratePerSecond,
		last:   time.Now(),
		now:    time.Now,
	}
}

func (r *rateLimiter) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.tokens += now.Sub(r.last).Seconds() * r.rate
	if r.tokens > r.burst {
		r.tokens = r.burst
	}
	r.last = now
	if r.tokens < 1 {
		return false
	}
	r.tokens--
	return true
}

// refund returns a token taken by allow, for a job that was not run.
func (r *rateLimiter) refund() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tokens+1 <= r.burst {
		r.tokens++
	}
}

// dedup remembers event IDs for a while, so that an event that Slack
// delivers again (for example after a reconnect) is handled only once.
type dedup struct {
	mu   sync.Mutex
	ttl  time.Duration
	max  int
	seen map[string]time.Time
	now  func() time.Time
}

func newDedup(ttl time.Duration, max int) *dedup {
	return &dedup{ttl: ttl, max: max, seen: map[string]time.Time{}, now: time.Now}
}

// firstTime reports whether id has not been seen within the TTL,
// and records it.
func (d *dedup) firstTime(id string) bool {
	if id == "" {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	if t, ok := d.seen[id]; ok && now.Sub(t) < d.ttl {
		return false
	}
	if len(d.seen) >= d.max {
		for k, t := range d.seen {
			if now.Sub(t) >= d.ttl {
				delete(d.seen, k)
			}
		}
		// Still full: forget everything rather than grow without bound.
		if len(d.seen) >= d.max {
			d.seen = map[string]time.Time{}
		}
	}
	d.seen[id] = now
	return true
}
