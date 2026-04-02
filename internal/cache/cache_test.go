package cache

import (
	"testing"
	"time"
)

// TestNewCache tests cache creation.
func TestNewCache(t *testing.T) {
	cache := NewCache()
	if cache == nil {
		t.Fatal("Expected cache to be created")
	}

	if cache.Size() != 0 {
		t.Error("New cache should be empty")
	}
}

// TestCacheSetGet tests basic set and get operations.
func TestCacheSetGet(t *testing.T) {
	cache := NewCache()

	// Set a value
	cache.Set("key1", "value1")

	// Get the value
	value, ok := cache.Get("key1")
	if !ok {
		t.Error("Expected to find key1")
	}

	if value != "value1" {
		t.Errorf("Expected 'value1', got '%v'", value)
	}

	// Get non-existent key
	_, ok = cache.Get("nonexistent")
	if ok {
		t.Error("Should not find nonexistent key")
	}
}

// TestCacheTTL tests TTL expiration.
func TestCacheTTL(t *testing.T) {
	cache := NewCache(WithTTL(100 * time.Millisecond))

	cache.Set("key1", "value1")

	// Should exist immediately
	_, ok := cache.Get("key1")
	if !ok {
		t.Error("Expected to find key1 immediately")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	_, ok = cache.Get("key1")
	if ok {
		t.Error("Key should be expired")
	}
}

// TestCacheSetWithTTL tests setting with specific TTL.
func TestCacheSetWithTTL(t *testing.T) {
	cache := NewCache(WithTTL(1 * time.Hour))

	// Set with short TTL
	cache.SetWithTTL("key1", "value1", 100*time.Millisecond)

	// Should exist
	_, ok := cache.Get("key1")
	if !ok {
		t.Error("Expected to find key1")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	_, ok = cache.Get("key1")
	if ok {
		t.Error("Key should be expired")
	}
}

// TestCacheDelete tests deletion.
func TestCacheDelete(t *testing.T) {
	cache := NewCache()

	cache.Set("key1", "value1")
	cache.Delete("key1")

	_, ok := cache.Get("key1")
	if ok {
		t.Error("Key should be deleted")
	}
}

// TestCacheClear tests clearing all entries.
func TestCacheClear(t *testing.T) {
	cache := NewCache()

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Clear()

	if cache.Size() != 0 {
		t.Error("Cache should be empty after clear")
	}
}

// TestCacheMaxSize tests maximum size limit.
func TestCacheMaxSize(t *testing.T) {
	cache := NewCache(WithMaxSize(5))

	// Add more entries than max
	for i := 0; i < 10; i++ {
		cache.Set(string(rune('a'+i)), i)
	}

	// Size should be limited
	if cache.Size() > 5 {
		t.Errorf("Cache size should be at most 5, got %d", cache.Size())
	}
}

// TestCacheCleanup tests manual cleanup.
func TestCacheCleanup(t *testing.T) {
	cache := NewCache(WithTTL(100 * time.Millisecond))

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Cleanup
	removed := cache.Cleanup()
	if removed != 2 {
		t.Errorf("Expected 2 entries removed, got %d", removed)
	}

	if cache.Size() != 0 {
		t.Error("Cache should be empty after cleanup")
	}
}

// TestGetOrCompute tests compute-on-miss.
func TestGetOrCompute(t *testing.T) {
	cache := NewCache()
	computed := false

	value, err := cache.GetOrCompute("key1", func() (interface{}, error) {
		computed = true
		return "computed", nil
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if value != "computed" {
		t.Errorf("Expected 'computed', got '%v'", value)
	}

	if !computed {
		t.Error("Should have computed value")
	}

	// Second call should use cache
	computed = false
	value, err = cache.GetOrCompute("key1", func() (interface{}, error) {
		computed = true
		return "computed2", nil
	})

	if computed {
		t.Error("Should have used cache, not computed")
	}
}

// TestFileCache tests file caching.
func TestFileCache(t *testing.T) {
	cache := NewFileCache(1024 * 1024) // 1MB

	entry := &FileEntry{
		Content: "test content",
		Size:    12,
		ModTime: time.Now(),
	}

	cache.Set("/path/to/file.txt", entry)

	// Get the entry
	retrieved, ok := cache.Get("/path/to/file.txt")
	if !ok {
		t.Error("Expected to find file")
	}

	if retrieved.Content != "test content" {
		t.Errorf("Expected 'test content', got '%s'", retrieved.Content)
	}

	// Check stats
	entries, bytes, maxBytes := cache.Stats()
	if entries != 1 {
		t.Errorf("Expected 1 entry, got %d", entries)
	}

	if bytes != 12 {
		t.Errorf("Expected 12 bytes, got %d", bytes)
	}

	if maxBytes != 1024*1024 {
		t.Errorf("Expected 1MB max, got %d", maxBytes)
	}
}

// TestFileCacheDelete tests file cache deletion.
func TestFileCacheDelete(t *testing.T) {
	cache := NewFileCache(1024 * 1024)

	entry := &FileEntry{
		Content: "test",
		Size:    4,
	}

	cache.Set("/file.txt", entry)
	cache.Delete("/file.txt")

	_, ok := cache.Get("/file.txt")
	if ok {
		t.Error("File should be deleted")
	}
}

// TestFileCacheClear tests file cache clear.
func TestFileCacheClear(t *testing.T) {
	cache := NewFileCache(1024 * 1024)

	cache.Set("/file1.txt", &FileEntry{Content: "a", Size: 1})
	cache.Set("/file2.txt", &FileEntry{Content: "bb", Size: 2})

	cache.Clear()

	entries, bytes, _ := cache.Stats()
	if entries != 0 {
		t.Error("Cache should be empty")
	}

	if bytes != 0 {
		t.Error("Bytes should be 0")
	}
}

// TestAPICache tests API response caching.
func TestAPICache(t *testing.T) {
	cache := NewAPICache()

	entry := &APIEntry{
		Response: "API response",
		Model:    "claude-3",
		Tokens:   100,
	}

	cache.Set("claude-3", "test input", entry)

	retrieved, ok := cache.Get("claude-3", "test input")
	if !ok {
		t.Error("Expected to find API response")
	}

	if retrieved.Response != "API response" {
		t.Errorf("Expected 'API response', got '%s'", retrieved.Response)
	}
}

// TestAPICacheClear tests API cache clear.
func TestAPICacheClear(t *testing.T) {
	cache := NewAPICache()

	cache.Set("model", "input", &APIEntry{Response: "test"})
	cache.Clear()

	if cache.Stats() != 0 {
		t.Error("Cache should be empty")
	}
}

// TestGenerateKey tests cache key generation.
func TestGenerateKey(t *testing.T) {
	key1 := GenerateKey("model1", "input1")
	key2 := GenerateKey("model1", "input1")
	key3 := GenerateKey("model2", "input1")

	if key1 != key2 {
		t.Error("Same inputs should produce same key")
	}

	if key1 == key3 {
		t.Error("Different models should produce different keys")
	}

	// Test long input truncation
	longInput := string(make([]byte, 100))
	key := GenerateKey("model", longInput)

	if len(key) > 150 {
		t.Errorf("Key for long input should be truncated, got len %d", len(key))
	}
}

// TestToolResultCache tests tool result caching.
func TestToolResultCache(t *testing.T) {
	cache := NewToolResultCache()

	entry := &ToolResultEntry{
		ToolName: "bash",
		Input:    "echo hello",
		Output:   "hello",
		Duration: 100 * time.Millisecond,
	}

	cache.Set("bash", "echo hello", entry)

	retrieved, ok := cache.Get("bash", "echo hello")
	if !ok {
		t.Error("Expected to find tool result")
	}

	if retrieved.Output != "hello" {
		t.Errorf("Expected 'hello', got '%s'", retrieved.Output)
	}
}

// TestToolResultCacheClear tests tool cache clear.
func TestToolResultCacheClear(t *testing.T) {
	cache := NewToolResultCache()

	cache.Set("tool", "input", &ToolResultEntry{Output: "test"})
	cache.Clear()

	if cache.Stats() != 0 {
		t.Error("Cache should be empty")
	}
}

// TestGenerateToolKey tests tool key generation.
func TestGenerateToolKey(t *testing.T) {
	key1 := GenerateToolKey("bash", "ls")
	key2 := GenerateToolKey("bash", "ls")
	key3 := GenerateToolKey("read", "ls")

	if key1 != key2 {
		t.Error("Same inputs should produce same key")
	}

	if key1 == key3 {
		t.Error("Different tools should produce different keys")
	}
}

// TestCacheConcurrency tests concurrent access.
func TestCacheConcurrency(t *testing.T) {
	cache := NewCache(WithMaxSize(1000))

	done := make(chan bool)

	// Concurrent writers
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				cache.Set(string(rune(id*100+j)), j)
			}
			done <- true
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				cache.Get(string(rune(id*100 + j)))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Cache should still be valid
	if cache.Size() > 1000 {
		t.Errorf("Cache size should be at most 1000, got %d", cache.Size())
	}
}