// Package concurrency provides utilities for parallel execution.
package concurrency

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ParallelExecutor executes tasks in parallel with controlled concurrency.
type ParallelExecutor struct {
	maxWorkers int
	timeout    time.Duration
}

// ExecutorOption configures the executor.
type ExecutorOption func(*ParallelExecutor)

// WithMaxWorkers sets the maximum number of concurrent workers.
func WithMaxWorkers(n int) ExecutorOption {
	return func(e *ParallelExecutor) {
		if n > 0 {
			e.maxWorkers = n
		}
	}
}

// WithTimeout sets a global timeout for all tasks.
func WithTimeout(d time.Duration) ExecutorOption {
	return func(e *ParallelExecutor) {
		e.timeout = d
	}
}

// NewParallelExecutor creates a new parallel executor.
func NewParallelExecutor(opts ...ExecutorOption) *ParallelExecutor {
	e := &ParallelExecutor{
		maxWorkers: 4, // Default to 4 workers
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Task represents a unit of work.
type Task struct {
	ID   string
	Name string
	Func func(ctx context.Context) (interface{}, error)
}

// TaskResult represents the result of a task.
type TaskResult struct {
	TaskID    string
	TaskName  string
	Value     interface{}
	Error     error
	Duration  time.Duration
	StartTime time.Time
	EndTime   time.Time
}

// Execute runs tasks in parallel and returns results.
func (e *ParallelExecutor) Execute(ctx context.Context, tasks []Task) []TaskResult {
	if len(tasks) == 0 {
		return nil
	}

	// Create context with timeout if specified
	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}

	results := make([]TaskResult, len(tasks))
	var wg sync.WaitGroup

	// Semaphore to limit concurrency
	sem := make(chan struct{}, e.maxWorkers)

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t Task) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = TaskResult{
					TaskID:   t.ID,
					TaskName: t.Name,
					Error:    ctx.Err(),
				}
				return
			}

			// Execute the task
			start := time.Now()
			value, err := t.Func(ctx)
			end := time.Now()

			results[idx] = TaskResult{
				TaskID:    t.ID,
				TaskName:  t.Name,
				Value:     value,
				Error:     err,
				Duration:  end.Sub(start),
				StartTime: start,
				EndTime:   end,
			}
		}(i, task)
	}

	wg.Wait()
	return results
}

// ExecuteWithCallback runs tasks and calls callback for each result.
func (e *ParallelExecutor) ExecuteWithCallback(ctx context.Context, tasks []Task, callback func(TaskResult)) {
	if len(tasks) == 0 {
		return
	}

	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, e.maxWorkers)

	for _, task := range tasks {
		wg.Add(1)
		go func(t Task) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				callback(TaskResult{
					TaskID:   t.ID,
					TaskName: t.Name,
					Error:    ctx.Err(),
				})
				return
			}

			start := time.Now()
			value, err := t.Func(ctx)
			end := time.Now()

			callback(TaskResult{
				TaskID:    t.ID,
				TaskName:  t.Name,
				Value:     value,
				Error:     err,
				Duration:  end.Sub(start),
				StartTime: start,
				EndTime:   end,
			})
		}(task)
	}

	wg.Wait()
}

// RateLimiter limits the rate of operations.
type RateLimiter struct {
	tokens     chan struct{}
	refillRate time.Duration
	stop       chan struct{}
}

// NewRateLimiter creates a rate limiter.
func NewRateLimiter(maxTokens int, refillRate time.Duration) *RateLimiter {
	rl := &RateLimiter{
		tokens:     make(chan struct{}, maxTokens),
		refillRate: refillRate,
		stop:       make(chan struct{}),
	}

	// Fill initial tokens
	for i := 0; i < maxTokens; i++ {
		rl.tokens <- struct{}{}
	}

	// Start refill goroutine
	go rl.refill(maxTokens)

	return rl
}

// refill periodically adds tokens.
func (rl *RateLimiter) refill(maxTokens int) {
	ticker := time.NewTicker(rl.refillRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			select {
			case rl.tokens <- struct{}{}:
			default:
				// Token bucket is full
			}
		case <-rl.stop:
			return
		}
	}
}

// Wait blocks until a token is available.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	select {
	case <-rl.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryWait tries to acquire a token without blocking.
func (rl *RateLimiter) TryWait() bool {
	select {
	case <-rl.tokens:
		return true
	default:
		return false
	}
}

// Stop stops the rate limiter.
func (rl *RateLimiter) Stop() {
	close(rl.stop)
}

// Throttle wraps a function to be rate-limited.
type Throttle struct {
	limiter *RateLimiter
}

// NewThrottle creates a throttle with the given rate.
func NewThrottle(maxPerSecond int) *Throttle {
	refillRate := time.Second / time.Duration(maxPerSecond)
	return &Throttle{
		limiter: NewRateLimiter(maxPerSecond, refillRate),
	}
}

// Execute runs a function with rate limiting.
func (t *Throttle) Execute(ctx context.Context, fn func() error) error {
	if err := t.limiter.Wait(ctx); err != nil {
		return err
	}
	return fn()
}

// Stop stops the throttle.
func (t *Throttle) Stop() {
	t.limiter.Stop()
}

// CircuitBreaker prevents cascading failures.
type CircuitBreaker struct {
	maxFailures  int
	resetTimeout time.Duration

	failures int
	lastFail time.Time
	state    State // open, closed, half-open
	mu       sync.RWMutex
}

// State represents circuit breaker state.
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

// NewCircuitBreaker creates a circuit breaker.
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        StateClosed,
	}
}

// Execute runs a function with circuit breaker protection.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.allowRequest() {
		return ErrCircuitOpen
	}

	err := fn()
	cb.recordResult(err)
	return err
}

// allowRequest checks if requests are allowed.
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.RLock()
	state := cb.state
	lastFail := cb.lastFail
	cb.mu.RUnlock()

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if we should try half-open
		if time.Since(lastFail) > cb.resetTimeout {
			cb.mu.Lock()
			cb.state = StateHalfOpen
			cb.mu.Unlock()
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// recordResult records the result of a request.
func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFail = time.Now()

		if cb.failures >= cb.maxFailures {
			cb.state = StateOpen
		}
	} else {
		// Success - reset
		cb.failures = 0
		cb.state = StateClosed
	}
}

// State returns the current state.
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// ErrCircuitOpen is returned when the circuit is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")