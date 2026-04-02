// Package cache provides caching mechanisms for SuperTerminal.
package cache

import (
	"sync"
	"time"
)

// Entry represents a cache entry.
type Entry struct {
	Value      interface{}
	Expiration time.Time
}

// IsExpired checks if the entry has expired.
func (e *Entry) IsExpired() bool {
	return !e.Expiration.IsZero() && time.Now().After(e.Expiration)
}

// Cache is a simple in-memory cache with TTL support.
type Cache struct {
	entries    map[string]*Entry
	mu         sync.RWMutex
	defaultTTL time.Duration
	maxSize    int
}

// CacheOption configures the cache.
type CacheOption func(*Cache)

// WithTTL sets the default TTL for cache entries.
func WithTTL(ttl time.Duration) CacheOption {
	return func(c *Cache) {
		c.defaultTTL = ttl
	}
}

// WithMaxSize sets the maximum number of entries.
func WithMaxSize(size int) CacheOption {
	return func(c *Cache) {
		c.maxSize = size
	}
}

// NewCache creates a new cache.
func NewCache(opts ...CacheOption) *Cache {
	c := &Cache{
		entries:    make(map[string]*Entry),
		defaultTTL: 5 * time.Minute,
		maxSize:    1000,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Set stores a value in the cache.
func (c *Cache) Set(key string, value interface{}) {
	c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL stores a value with a specific TTL.
func (c *Cache) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	var expiration time.Time
	if ttl > 0 {
		expiration = time.Now().Add(ttl)
	}

	c.entries[key] = &Entry{
		Value:      value,
		Expiration: expiration,
	}
}

// Get retrieves a value from the cache.
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	if entry.IsExpired() {
		return nil, false
	}

	return entry.Value, true
}

// Delete removes a value from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*Entry)
}

// Size returns the number of entries in the cache.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// evictOldest removes the oldest expired entries.
func (c *Cache) evictOldest() {
	// First, remove expired entries
	for key, entry := range c.entries {
		if entry.IsExpired() {
			delete(c.entries, key)
		}
	}

	// If still over limit, remove oldest (simple FIFO)
	if len(c.entries) >= c.maxSize {
		count := 0
		for key := range c.entries {
			delete(c.entries, key)
			count++
			if count >= c.maxSize/10 {
				break // Remove 10% at a time
			}
		}
	}
}

// Cleanup removes expired entries.
func (c *Cache) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for key, entry := range c.entries {
		if entry.IsExpired() {
			delete(c.entries, key)
			count++
		}
	}
	return count
}

// GetOrCompute retrieves a value or computes it if not found.
func (c *Cache) GetOrCompute(key string, compute func() (interface{}, error)) (interface{}, error) {
	// Try to get from cache first
	if value, ok := c.Get(key); ok {
		return value, nil
	}

	// Compute the value
	value, err := compute()
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.Set(key, value)

	return value, nil
}

// FileCache provides caching for file contents.
type FileCache struct {
	cache    *Cache
	maxBytes int64
	curBytes int64
	mu       sync.RWMutex
}

// FileEntry represents a cached file.
type FileEntry struct {
	Content string
	Size    int64
	ModTime time.Time
	Hash    string
}

// NewFileCache creates a new file cache.
func NewFileCache(maxBytes int64) *FileCache {
	return &FileCache{
		cache:    NewCache(WithMaxSize(500)),
		maxBytes: maxBytes,
	}
}

// Get retrieves a file from the cache.
func (fc *FileCache) Get(path string) (*FileEntry, bool) {
	if value, ok := fc.cache.Get(path); ok {
		entry, ok := value.(*FileEntry)
		return entry, ok
	}
	return nil, false
}

// Set stores a file in the cache.
func (fc *FileCache) Set(path string, entry *FileEntry) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Check if we need to evict
	if fc.curBytes+entry.Size > fc.maxBytes {
		fc.evictToFit(entry.Size)
	}

	// Remove old entry if exists
	if old, ok := fc.cache.Get(path); ok {
		if oldEntry, ok := old.(*FileEntry); ok {
			fc.curBytes -= oldEntry.Size
		}
	}

	fc.cache.Set(path, entry)
	fc.curBytes += entry.Size
}

// Delete removes a file from the cache.
func (fc *FileCache) Delete(path string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if value, ok := fc.cache.Get(path); ok {
		if entry, ok := value.(*FileEntry); ok {
			fc.curBytes -= entry.Size
		}
	}

	fc.cache.Delete(path)
}

// Clear clears the file cache.
func (fc *FileCache) Clear() {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.cache.Clear()
	fc.curBytes = 0
}

// Stats returns cache statistics.
func (fc *FileCache) Stats() (entries int, bytes int64, maxBytes int64) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	return fc.cache.Size(), fc.curBytes, fc.maxBytes
}

// evictToFit evicts entries to fit new data.
func (fc *FileCache) evictToFit(needed int64) {
	// Remove entries until we have enough space
	targetBytes := fc.maxBytes - needed

	for fc.curBytes > targetBytes && fc.cache.Size() > 0 {
		// Simple eviction: clear entries
		fc.cache.Cleanup()
		if fc.cache.Size() == 0 {
			break
		}

		// Force remove some entries
		count := 0
		for key := range fc.cache.entries {
			if value, ok := fc.cache.Get(key); ok {
				if entry, ok := value.(*FileEntry); ok {
					fc.curBytes -= entry.Size
				}
			}
			fc.cache.Delete(key)
			count++
			if count >= 10 {
				break
			}
		}
	}
}

// APICache provides caching for API responses.
type APICache struct {
	cache *Cache
}

// APIEntry represents a cached API response.
type APIEntry struct {
	Response  string
	Model     string
	InputHash string
	Tokens    int
	Cached    time.Time
}

// NewAPICache creates a new API cache.
func NewAPICache() *APICache {
	return &APICache{
		cache: NewCache(WithTTL(30*time.Minute), WithMaxSize(200)),
	}
}

// GenerateKey creates a cache key from request parameters.
func GenerateKey(model, input string) string {
	// Simple hash - could be improved with proper hashing
	if len(input) > 64 {
		input = input[:32] + "..." + input[len(input)-32:]
	}
	return model + ":" + input
}

// Get retrieves an API response from the cache.
func (ac *APICache) Get(model, input string) (*APIEntry, bool) {
	key := GenerateKey(model, input)
	if value, ok := ac.cache.Get(key); ok {
		entry, ok := value.(*APIEntry)
		return entry, ok
	}
	return nil, false
}

// Set stores an API response in the cache.
func (ac *APICache) Set(model, input string, entry *APIEntry) {
	key := GenerateKey(model, input)
	ac.cache.Set(key, entry)
}

// Clear clears the API cache.
func (ac *APICache) Clear() {
	ac.cache.Clear()
}

// Stats returns cache statistics.
func (ac *APICache) Stats() int {
	return ac.cache.Size()
}

// ToolResultCache caches tool execution results.
type ToolResultCache struct {
	cache *Cache
}

// ToolResultEntry represents a cached tool result.
type ToolResultEntry struct {
	ToolName string
	Input    string
	Output   string
	Error    string
	Duration  time.Duration
	Executed time.Time
}

// NewToolResultCache creates a new tool result cache.
func NewToolResultCache() *ToolResultCache {
	return &ToolResultCache{
		cache: NewCache(WithTTL(10*time.Minute), WithMaxSize(300)),
	}
}

// GenerateToolKey creates a cache key for tool results.
func GenerateToolKey(toolName, input string) string {
	if len(input) > 128 {
		input = input[:64] + "..." + input[len(input)-64:]
	}
	return toolName + ":" + input
}

// Get retrieves a tool result from the cache.
func (tc *ToolResultCache) Get(toolName, input string) (*ToolResultEntry, bool) {
	key := GenerateToolKey(toolName, input)
	if value, ok := tc.cache.Get(key); ok {
		entry, ok := value.(*ToolResultEntry)
		return entry, ok
	}
	return nil, false
}

// Set stores a tool result in the cache.
func (tc *ToolResultCache) Set(toolName, input string, entry *ToolResultEntry) {
	key := GenerateToolKey(toolName, input)
	tc.cache.Set(key, entry)
}

// Clear clears the tool result cache.
func (tc *ToolResultCache) Clear() {
	tc.cache.Clear()
}

// Stats returns cache statistics.
func (tc *ToolResultCache) Stats() int {
	return tc.cache.Size()
}