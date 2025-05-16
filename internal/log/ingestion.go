package log

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"api-watchtower/internal/db"
)

type Ingester struct {
	buffer     []*db.ApplicationLog
	bufferSize int
	batchSize  int
	mu         sync.Mutex
	flushCh    chan struct{}
	storage    Storage
}

type Storage interface {
	BatchInsertLogs(ctx context.Context, logs []*db.ApplicationLog) error
}

func NewIngester(storage Storage, bufferSize, batchSize int) *Ingester {
	i := &Ingester{
		buffer:     make([]*db.ApplicationLog, 0, bufferSize),
		bufferSize: bufferSize,
		batchSize:  batchSize,
		flushCh:    make(chan struct{}),
		storage:    storage,
	}

	go i.flushLoop()
	return i
}

func (i *Ingester) IngestLog(ctx context.Context, rawLog json.RawMessage) error {
	var log db.ApplicationLog
	if err := json.Unmarshal(rawLog, &log); err != nil {
		return err
	}

	// Validate required fields
	if err := i.validateLog(&log); err != nil {
		return err
	}

	// Set timestamp if not provided
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}

	i.mu.Lock()
	i.buffer = append(i.buffer, &log)
	shouldFlush := len(i.buffer) >= i.bufferSize
	i.mu.Unlock()

	if shouldFlush {
		i.triggerFlush()
	}

	return nil
}

func (i *Ingester) validateLog(log *db.ApplicationLog) error {
	if log.ApplicationID == "" {
		return errors.New("application_id is required")
	}
	if log.ServiceName == "" {
		return errors.New("service_name is required")
	}
	if log.Severity == "" {
		return errors.New("severity is required")
	}
	if log.Message == "" {
		return errors.New("message is required")
	}
	return nil
}

func (i *Ingester) triggerFlush() {
	select {
	case i.flushCh <- struct{}{}:
	default:
		// Channel is full, skip triggering flush as one is already pending
	}
}

func (i *Ingester) flushLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			i.flush()
		case <-i.flushCh:
			i.flush()
		}
	}
}

func (i *Ingester) flush() {
	i.mu.Lock()
	if len(i.buffer) == 0 {
		i.mu.Unlock()
		return
	}

	// Take a batch of logs
	batchSize := i.batchSize
	if batchSize > len(i.buffer) {
		batchSize = len(i.buffer)
	}

	batch := make([]*db.ApplicationLog, batchSize)
	copy(batch, i.buffer[:batchSize])
	
	// Remove the taken batch from buffer
	i.buffer = append(i.buffer[:0], i.buffer[batchSize:]...)
	i.mu.Unlock()

	// Store the batch
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := i.storage.BatchInsertLogs(ctx, batch); err != nil {
		// On error, try to requeue logs
		i.mu.Lock()
		// Prepend failed batch back to buffer
		i.buffer = append(batch, i.buffer...)
		i.mu.Unlock()
	}
}

type QueryOptions struct {
	ApplicationID string
	ServiceName  string
	Severity     string
	StartTime    time.Time
	EndTime      time.Time
	Limit        int
	Offset       int
}

type QueryResult struct {
	Logs       []*db.ApplicationLog
	TotalCount int
	HasMore    bool
}

func (i *Ingester) QueryLogs(ctx context.Context, opts QueryOptions) (*QueryResult, error) {
	// This would be implemented by the storage layer
	return nil, errors.New("not implemented")
}
