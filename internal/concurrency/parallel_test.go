package concurrency

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewParallelExecutor tests executor creation.
func TestNewParallelExecutor(t *testing.T) {
	executor := NewParallelExecutor()
	if executor == nil {
		t.Fatal("Expected executor to be created")
	}

	if executor.maxWorkers != 4 {
		t.Errorf("Expected default 4 workers, got %d", executor.maxWorkers)
	}
}

// TestExecutorWithMaxWorkers tests setting max workers.
func TestExecutorWithMaxWorkers(t *testing.T) {
	executor := NewParallelExecutor(WithMaxWorkers(8))
	if executor.maxWorkers != 8 {
		t.Errorf("Expected 8 workers, got %d", executor.maxWorkers)
	}
}

// TestParallelExecution tests running tasks in parallel.
func TestParallelExecution(t *testing.T) {
	executor := NewParallelExecutor(WithMaxWorkers(4))

	var counter int32
	tasks := make([]Task, 10)

	for i := 0; i < 10; i++ {
		tasks[i] = Task{
			ID:   string(rune('a' + i)),
			Name: "increment",
			Func: func(ctx context.Context) (interface{}, error) {
				atomic.AddInt32(&counter, 1)
				return atomic.LoadInt32(&counter), nil
			},
		}
	}

	results := executor.Execute(context.Background(), tasks)

	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}

	// All tasks should have succeeded
	for i, r := range results {
		if r.Error != nil {
			t.Errorf("Task %d should not have error: %v", i, r.Error)
		}
	}

	// All tasks should have run
	if counter != 10 {
		t.Errorf("Expected counter 10, got %d", counter)
	}
}

// TestParallelExecutionWithTimeout tests timeout.
func TestParallelExecutionWithTimeout(t *testing.T) {
	executor := NewParallelExecutor(WithTimeout(50 * time.Millisecond))

	tasks := []Task{
		{
			ID:   "slow",
			Name: "slow task",
			Func: func(ctx context.Context) (interface{}, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(200 * time.Millisecond):
					return "done", nil
				}
			},
		},
	}

	results := executor.Execute(context.Background(), tasks)

	if len(results) != 1 {
		t.Fatal("Expected 1 result")
	}

	if results[0].Error == nil {
		t.Error("Expected timeout error")
	}
}

// TestParallelExecutionWithCallback tests callback mode.
func TestParallelExecutionWithCallback(t *testing.T) {
	executor := NewParallelExecutor(WithMaxWorkers(2))

	var results []TaskResult
	var mu sync.Mutex

	tasks := []Task{
		{ID: "1", Name: "task1", Func: func(ctx context.Context) (interface{}, error) { return 1, nil }},
		{ID: "2", Name: "task2", Func: func(ctx context.Context) (interface{}, error) { return 2, nil }},
	}

	callback := func(r TaskResult) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	}

	executor.ExecuteWithCallback(context.Background(), tasks, callback)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

// TestEmptyTasks tests empty task list.
func TestEmptyTasks(t *testing.T) {
	executor := NewParallelExecutor()

	results := executor.Execute(context.Background(), nil)
	if results != nil {
		t.Error("Expected nil results for empty tasks")
	}
}

// TestRateLimiter tests rate limiting.
func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(2, 100*time.Millisecond)
	defer rl.Stop()

	ctx := context.Background()

	// Should be able to acquire tokens immediately
	if !rl.TryWait() {
		t.Error("Should be able to acquire token")
	}
	if !rl.TryWait() {
		t.Error("Should be able to acquire second token")
	}

	// Should be empty now
	if rl.TryWait() {
		t.Error("Should not have tokens available")
	}

	// Wait should block but succeed after refill
	start := time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Errorf("Wait should succeed: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < 50*time.Millisecond {
		t.Error("Wait should have blocked for refill")
	}
}

// TestRateLimiterContextCancel tests context cancellation.
func TestRateLimiterContextCancel(t *testing.T) {
	rl := NewRateLimiter(1, 1*time.Second)
	defer rl.Stop()

	// Consume the token
	rl.TryWait()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)
	if err == nil {
		t.Error("Expected context cancellation error")
	}
}

// TestThrottle tests throttling.
func TestThrottle(t *testing.T) {
	throttle := NewThrottle(5) // 5 per second
	defer throttle.Stop()

	var count int
	for i := 0; i < 3; i++ {
		err := throttle.Execute(context.Background(), func() error {
			count++
			return nil
		})
		if err != nil {
			t.Errorf("Execute should succeed: %v", err)
		}
	}

	if count != 3 {
		t.Errorf("Expected 3 executions, got %d", count)
	}
}

// TestCircuitBreaker tests circuit breaker behavior.
func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)

	// Should start closed
	if cb.State() != StateClosed {
		t.Error("Should start closed")
	}

	// Record failures
	err := errors.New("test error")
	for i := 0; i < 3; i++ {
		cb.Execute(func() error { return err })
	}

	// Should be open after 3 failures
	if cb.State() != StateOpen {
		t.Error("Should be open after failures")
	}

	// Should reject requests when open (immediately after failures)
	execErr := cb.Execute(func() error { return nil })
	if execErr != ErrCircuitOpen {
		t.Error("Should reject when open")
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// After timeout, next request should be allowed (half-open)
	// Success should close it
	cb.Execute(func() error { return nil })
	time.Sleep(10 * time.Millisecond) // Give time for state update
	if cb.State() != StateClosed {
		t.Error("Should be closed after success")
	}
}

// TestCircuitBreakerSuccess tests successful execution.
func TestCircuitBreakerSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Second)

	// Successful calls should keep it closed
	for i := 0; i < 10; i++ {
		err := cb.Execute(func() error { return nil })
		if err != nil {
			t.Errorf("Should succeed: %v", err)
		}
	}

	if cb.State() != StateClosed {
		t.Error("Should remain closed with successes")
	}
}

// TestTaskDuration tests that duration is recorded.
func TestTaskDuration(t *testing.T) {
	executor := NewParallelExecutor()

	tasks := []Task{
		{
			ID:   "timed",
			Name: "timed task",
			Func: func(ctx context.Context) (interface{}, error) {
				time.Sleep(50 * time.Millisecond)
				return "done", nil
			},
		},
	}

	results := executor.Execute(context.Background(), tasks)

	if results[0].Duration < 50*time.Millisecond {
		t.Errorf("Duration should be at least 50ms, got %v", results[0].Duration)
	}

	if results[0].StartTime.IsZero() {
		t.Error("StartTime should be set")
	}

	if results[0].EndTime.IsZero() {
		t.Error("EndTime should be set")
	}
}

// TestConcurrencyLimit tests that max workers is respected.
func TestConcurrencyLimit(t *testing.T) {
	executor := NewParallelExecutor(WithMaxWorkers(2))

	var maxConcurrent int32
	var currentConcurrent int32

	tasks := make([]Task, 10)
	for i := 0; i < 10; i++ {
		tasks[i] = Task{
			ID:   string(rune('0' + i)),
			Name: "concurrent test",
			Func: func(ctx context.Context) (interface{}, error) {
				cur := atomic.AddInt32(&currentConcurrent, 1)
				
				// Track max
				for {
					max := atomic.LoadInt32(&maxConcurrent)
					if cur <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, cur) {
						break
					}
				}
				
				time.Sleep(20 * time.Millisecond)
				atomic.AddInt32(&currentConcurrent, -1)
				return nil, nil
			},
		}
	}

	executor.Execute(context.Background(), tasks)

	// Max concurrent should not exceed 2
	if maxConcurrent > 2 {
		t.Errorf("Max concurrent should be at most 2, got %d", maxConcurrent)
	}
}