package background

import (
	"context"
	"log"
	"time"

	"github.com/opd-ai/store/pkg/store"
)

// AuditLogCleaner periodically removes old audit log entries.
type AuditLogCleaner struct {
	store         store.Service
	interval      time.Duration
	retentionDays int
	stopCh        chan struct{}
	doneCh        chan struct{}
}

// NewAuditLogCleaner creates a new audit log cleaner.
func NewAuditLogCleaner(store store.Service, interval time.Duration, retentionDays int) *AuditLogCleaner {
	return &AuditLogCleaner{
		store:         store,
		interval:      interval,
		retentionDays: retentionDays,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// Start begins the cleanup loop in a background goroutine.
func (c *AuditLogCleaner) Start(ctx context.Context) {
	go c.run(ctx)
}

// Stop signals the cleaner to stop and waits for it to finish.
func (c *AuditLogCleaner) Stop() {
	close(c.stopCh)
	<-c.doneCh
}

// run executes the cleanup loop.
func (c *AuditLogCleaner) run(ctx context.Context) {
	defer close(c.doneCh)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Clean immediately on start
	c.cleanOnce(ctx)

	for {
		select {
		case <-ticker.C:
			c.cleanOnce(ctx)
		case <-c.stopCh:
			log.Println("Audit log cleaner stopped")
			return
		case <-ctx.Done():
			log.Println("Audit log cleaner context cancelled")
			return
		}
	}
}

// cleanOnce performs a single cleanup cycle for old audit logs.
func (c *AuditLogCleaner) cleanOnce(ctx context.Context) {
	if c.retentionDays <= 0 {
		return
	}

	log.Printf("Cleaning audit logs older than %d days", c.retentionDays)

	deletedCount, err := c.store.CleanupOldAuditLogs(ctx, c.retentionDays)
	if err != nil {
		log.Printf("Failed to cleanup audit logs: %v", err)
		return
	}

	if deletedCount > 0 {
		log.Printf("Audit log cleanup complete: %d old logs deleted", deletedCount)
	}
}
