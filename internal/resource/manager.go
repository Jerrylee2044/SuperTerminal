// Package resource provides resource management utilities.
package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// MemoryMonitor monitors memory usage.
type MemoryMonitor struct {
	maxBytes    int64
	warnBytes   int64
	checkIntval time.Duration
	callbacks   []func(MemoryStats)
	stop        chan struct{}
	mu          sync.RWMutex
}

// MemoryStats represents memory statistics.
type MemoryStats struct {
	AllocBytes   int64
	TotalBytes   int64
	SystemBytes  int64
	NumGC        uint32
	PercentUsed  float64
	IsOverLimit  bool
	IsOverWarn   bool
	Timestamp    time.Time
}

// MemoryMonitorOption configures the monitor.
type MemoryMonitorOption func(*MemoryMonitor)

// WithMaxMemory sets the maximum memory limit.
func WithMaxMemory(bytes int64) MemoryMonitorOption {
	return func(m *MemoryMonitor) {
		m.maxBytes = bytes
	}
}

// WithWarnMemory sets the warning threshold.
func WithWarnMemory(bytes int64) MemoryMonitorOption {
	return func(m *MemoryMonitor) {
		m.warnBytes = bytes
	}
}

// WithCheckInterval sets the check interval.
func WithCheckInterval(d time.Duration) MemoryMonitorOption {
	return func(m *MemoryMonitor) {
		m.checkIntval = d
	}
}

// WithCallback adds a callback for memory events.
func WithCallback(cb func(MemoryStats)) MemoryMonitorOption {
	return func(m *MemoryMonitor) {
		m.callbacks = append(m.callbacks, cb)
	}
}

// NewMemoryMonitor creates a new memory monitor.
func NewMemoryMonitor(opts ...MemoryMonitorOption) *MemoryMonitor {
	m := &MemoryMonitor{
		maxBytes:    1024 * 1024 * 1024, // 1GB default
		warnBytes:   768 * 1024 * 1024,  // 768MB default
		checkIntval: 10 * time.Second,
		stop:        make(chan struct{}),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Start begins monitoring memory usage.
func (m *MemoryMonitor) Start() {
	go m.monitor()
}

// Stop stops monitoring.
func (m *MemoryMonitor) Stop() {
	close(m.stop)
}

// monitor periodically checks memory usage.
func (m *MemoryMonitor) monitor() {
	ticker := time.NewTicker(m.checkIntval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats := m.GetStats()
			m.mu.RLock()
			callbacks := m.callbacks
			m.mu.RUnlock()

			for _, cb := range callbacks {
				cb(stats)
			}
		case <-m.stop:
			return
		}
	}
}

// GetStats returns current memory statistics.
func (m *MemoryMonitor) GetStats() MemoryStats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	stats := MemoryStats{
		AllocBytes:  int64(memStats.Alloc),
		TotalBytes:  int64(memStats.TotalAlloc),
		SystemBytes: int64(memStats.Sys),
		NumGC:       memStats.NumGC,
		Timestamp:   time.Now(),
	}

	if m.maxBytes > 0 {
		stats.PercentUsed = float64(stats.AllocBytes) / float64(m.maxBytes) * 100
		stats.IsOverLimit = stats.AllocBytes > m.maxBytes
		stats.IsOverWarn = stats.AllocBytes > m.warnBytes
	}

	return stats
}

// ForceGC forces a garbage collection.
func (m *MemoryMonitor) ForceGC() {
	runtime.GC()
}

// AddCallback adds a callback function.
func (m *MemoryMonitor) AddCallback(cb func(MemoryStats)) {
	m.mu.Lock()
	m.callbacks = append(m.callbacks, cb)
	m.mu.Unlock()
}

// SessionManager manages session count limits.
type SessionManager struct {
	maxSessions int
	sessions    map[string]*SessionInfo
	mu          sync.RWMutex
}

// SessionInfo contains session metadata.
type SessionInfo struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Metadata  map[string]interface{}
}

// SessionManagerOption configures the session manager.
type SessionManagerOption func(*SessionManager)

// WithMaxSessions sets the maximum number of sessions.
func WithMaxSessions(n int) SessionManagerOption {
	return func(sm *SessionManager) {
		sm.maxSessions = n
	}
}

// NewSessionManager creates a new session manager.
func NewSessionManager(opts ...SessionManagerOption) *SessionManager {
	sm := &SessionManager{
		maxSessions: 100,
		sessions:    make(map[string]*SessionInfo),
	}

	for _, opt := range opts {
		opt(sm)
	}

	return sm
}

// Create creates a new session.
func (sm *SessionManager) Create(id string, metadata map[string]interface{}) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if len(sm.sessions) >= sm.maxSessions {
		// Try to evict oldest
		sm.evictOldest()
	}

	if len(sm.sessions) >= sm.maxSessions {
		return fmt.Errorf("maximum sessions (%d) reached", sm.maxSessions)
	}

	now := time.Now()
	sm.sessions[id] = &SessionInfo{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  metadata,
	}

	return nil
}

// Get retrieves a session.
func (sm *SessionManager) Get(id string) (*SessionInfo, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	info, ok := sm.sessions[id]
	if !ok {
		return nil, false
	}

	return info, true
}

// Update updates a session's last activity time.
func (sm *SessionManager) Update(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	info, ok := sm.sessions[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}

	info.UpdatedAt = time.Now()
	return nil
}

// Delete removes a session.
func (sm *SessionManager) Delete(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, id)
}

// List returns all session IDs.
func (sm *SessionManager) List() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	ids := make([]string, 0, len(sm.sessions))
	for id := range sm.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Count returns the number of active sessions.
func (sm *SessionManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// evictOldest removes the oldest inactive session.
func (sm *SessionManager) evictOldest() {
	var oldestID string
	var oldestTime time.Time

	for id, info := range sm.sessions {
		if oldestID == "" || info.UpdatedAt.Before(oldestTime) {
			oldestID = id
			oldestTime = info.UpdatedAt
		}
	}

	if oldestID != "" {
		delete(sm.sessions, oldestID)
	}
}

// LogRotator manages log file rotation.
type LogRotator struct {
	maxSize    int64
	maxBackups int
	basePath   string
	mu         sync.Mutex
}

// LogRotatorOption configures the rotator.
type LogRotatorOption func(*LogRotator)

// WithMaxSize sets the maximum log file size in bytes.
func WithMaxSize(bytes int64) LogRotatorOption {
	return func(lr *LogRotator) {
		lr.maxSize = bytes
	}
}

// WithMaxBackups sets the maximum number of backup files.
func WithMaxBackups(n int) LogRotatorOption {
	return func(lr *LogRotator) {
		lr.maxBackups = n
	}
}

// NewLogRotator creates a new log rotator.
func NewLogRotator(basePath string, opts ...LogRotatorOption) *LogRotator {
	lr := &LogRotator{
		basePath:   basePath,
		maxSize:    10 * 1024 * 1024, // 10MB default
		maxBackups: 5,
	}

	for _, opt := range opts {
		opt(lr)
	}

	return lr
}

// ShouldRotate checks if the log file needs rotation.
func (lr *LogRotator) ShouldRotate() bool {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	info, err := os.Stat(lr.basePath)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return false
	}

	return info.Size() >= lr.maxSize
}

// Rotate rotates the log file.
func (lr *LogRotator) Rotate() error {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	// Check if main log exists
	if _, err := os.Stat(lr.basePath); os.IsNotExist(err) {
		return nil
	}

	// Remove oldest backup if at max
	oldestBackup := fmt.Sprintf("%s.%d", lr.basePath, lr.maxBackups)
	os.Remove(oldestBackup)

	// Shift existing backups
	for i := lr.maxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", lr.basePath, i)
		newPath := fmt.Sprintf("%s.%d", lr.basePath, i+1)
		os.Rename(oldPath, newPath)
	}

	// Rotate main log to .1
	backupPath := fmt.Sprintf("%s.1", lr.basePath)
	if err := os.Rename(lr.basePath, backupPath); err != nil {
		return fmt.Errorf("failed to rotate log: %w", err)
	}

	// Create new empty log file
	file, err := os.Create(lr.basePath)
	if err != nil {
		return fmt.Errorf("failed to create new log file: %w", err)
	}
	file.Close()

	return nil
}

// Write writes data to the log file with rotation check.
func (lr *LogRotator) Write(data []byte) (int, error) {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	// Check size and rotate if needed
	info, err := os.Stat(lr.basePath)
	if err == nil && info.Size()+int64(len(data)) > lr.maxSize {
		lr.rotateUnlocked()
	}

	// Open file in append mode
	file, err := os.OpenFile(lr.basePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	return file.Write(data)
}

// rotateUnlocked performs rotation without locking.
func (lr *LogRotator) rotateUnlocked() {
	// Remove oldest backup
	oldestBackup := fmt.Sprintf("%s.%d", lr.basePath, lr.maxBackups)
	os.Remove(oldestBackup)

	// Shift backups
	for i := lr.maxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", lr.basePath, i)
		newPath := fmt.Sprintf("%s.%d", lr.basePath, i+1)
		os.Rename(oldPath, newPath)
	}

	// Rotate current
	backupPath := fmt.Sprintf("%s.1", lr.basePath)
	os.Rename(lr.basePath, backupPath)
}

// CleanupOldBackups removes backups beyond maxBackups.
func (lr *LogRotator) CleanupOldBackups() error {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	dir := filepath.Dir(lr.basePath)
	base := filepath.Base(lr.basePath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		// Check if it's a backup file
		var backupNum int
		_, err := fmt.Sscanf(name, base+".%d", &backupNum)
		if err != nil {
			continue
		}

		// Remove if beyond max backups
		if backupNum > lr.maxBackups {
			os.Remove(filepath.Join(dir, name))
		}
	}

	return nil
}

// ResourceManager combines all resource management.
type ResourceManager struct {
	memory  *MemoryMonitor
	session *SessionManager
	logs    map[string]*LogRotator
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
}

// NewResourceManager creates a resource manager.
func NewResourceManager(memoryBytes int64, maxSessions int, logDir string) *ResourceManager {
	ctx, cancel := context.WithCancel(context.Background())

	rm := &ResourceManager{
		memory: NewMemoryMonitor(WithMaxMemory(memoryBytes)),
		session: NewSessionManager(WithMaxSessions(maxSessions)),
		logs:    make(map[string]*LogRotator),
		ctx:     ctx,
		cancel:  cancel,
	}

	// Add default memory warning callback
	rm.memory.AddCallback(func(stats MemoryStats) {
		if stats.IsOverWarn {
			// Could log warning here
		}
	})

	return rm
}

// Start starts all monitors.
func (rm *ResourceManager) Start() {
	rm.memory.Start()
}

// Stop stops all monitors.
func (rm *ResourceManager) Stop() {
	rm.cancel()
	rm.memory.Stop()
}

// Memory returns the memory monitor.
func (rm *ResourceManager) Memory() *MemoryMonitor {
	return rm.memory
}

// Sessions returns the session manager.
func (rm *ResourceManager) Sessions() *SessionManager {
	return rm.session
}

// AddLog adds a log rotator.
func (rm *ResourceManager) AddLog(name, path string, maxSize int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.logs[name] = NewLogRotator(path, WithMaxSize(maxSize))
}

// GetLog returns a log rotator.
func (rm *ResourceManager) GetLog(name string) *LogRotator {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.logs[name]
}