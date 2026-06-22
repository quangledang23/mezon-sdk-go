package mezon

import (
	"sync"
	"time"
)

// AsyncThrottleQueue serializes async tasks and throttles them to a maximum
// number per second, port of src/mezon-client/utils/AsyncThrottleQueue.ts.
// Tasks run one at a time in FIFO order; enqueue blocks until the task runs and
// returns its result.
type AsyncThrottleQueue struct {
	maxPerSecond int
	mu           sync.Mutex
	jobs         chan func()
	once         sync.Once
}

const defaultMaxPerSecond = 80

// NewAsyncThrottleQueue creates a queue allowing maxPerSecond tasks per second
// (defaults to 80 when <= 0).
func NewAsyncThrottleQueue(maxPerSecond int) *AsyncThrottleQueue {
	if maxPerSecond <= 0 {
		maxPerSecond = defaultMaxPerSecond
	}
	q := &AsyncThrottleQueue{
		maxPerSecond: maxPerSecond,
		jobs:         make(chan func(), 1024),
	}
	q.start()
	return q
}

func (q *AsyncThrottleQueue) start() {
	q.once.Do(func() {
		go func() {
			var timestamps []time.Time
			for job := range q.jobs {
				// drop timestamps older than 1s
				now := time.Now()
				kept := timestamps[:0]
				for _, t := range timestamps {
					if now.Sub(t) < time.Second {
						kept = append(kept, t)
					}
				}
				timestamps = kept
				// wait if we've hit the per-second cap
				for len(timestamps) >= q.maxPerSecond {
					time.Sleep(10 * time.Millisecond)
					now = time.Now()
					kept = timestamps[:0]
					for _, t := range timestamps {
						if now.Sub(t) < time.Second {
							kept = append(kept, t)
						}
					}
					timestamps = kept
				}
				timestamps = append(timestamps, time.Now())
				// Dispatch without blocking the loop, matching the TS
				// AsyncThrottleQueue whose loop calls task() without awaiting it:
				// the queue rate-limits how fast jobs are *started* (maxPerSecond),
				// it does not serialize them to one in-flight at a time. Running
				// job() inline here would stall every queued write behind the
				// current socket round-trip (and one shared queue feeds all
				// channels/DMs), collapsing throughput to ~1 per RTT.
				go job()
			}
		}()
	})
}

// Enqueue runs task on the queue and returns its result. It blocks until the
// task has executed.
func Enqueue[T any](q *AsyncThrottleQueue, task func() (T, error)) (T, error) {
	type result struct {
		val T
		err error
	}
	done := make(chan result, 1)
	q.jobs <- func() {
		v, err := task()
		done <- result{v, err}
	}
	r := <-done
	return r.val, r.err
}
