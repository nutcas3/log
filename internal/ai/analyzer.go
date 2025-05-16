package ai

import (
	"context"
	"encoding/json"
	"regexp"
	"fmt"
	"sync"
	"time"

	"api-watchtower/internal/db"

	"gonum.org/v1/gonum/stat"
)

type Analyzer struct {
	storage          Storage
	baselineMetrics  map[string]*baselineMetrics
	patternClusters  map[string]*patternCluster
	mu              sync.RWMutex
	updateInterval  time.Duration
}

type Storage interface {
	GetRecentLogs(ctx context.Context, duration time.Duration) ([]*db.ApplicationLog, error)
	SaveAnalysis(ctx context.Context, analysis *db.AIAnalysis) error
}

type baselineMetrics struct {
	ErrorRate     movingAverage
	ResponseTimes movingAverage
	UpdatedAt    time.Time
}

type movingAverage struct {
	Values []float64
	Window int
}

type patternCluster struct {
	Pattern     string
	Count       int
	LastSeen    time.Time
	Examples    []string
	Severity    string
}

func NewAnalyzer(storage Storage, updateInterval time.Duration) *Analyzer {
	a := &Analyzer{
		storage:         storage,
		baselineMetrics: make(map[string]*baselineMetrics),
		patternClusters: make(map[string]*patternCluster),
		updateInterval:  updateInterval,
	}

	go a.backgroundAnalysis()
	return a
}

func (a *Analyzer) backgroundAnalysis() {
	ticker := time.NewTicker(a.updateInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		a.analyze(ctx)
		cancel()
	}
}

func (a *Analyzer) analyze(ctx context.Context) {
	// Get recent logs for analysis
	logs, err := a.storage.GetRecentLogs(ctx, 24*time.Hour)
	if err != nil {
		return
	}

	// Group logs by application and service
	groupedLogs := a.groupLogs(logs)

	// Analyze each group
	for key, logs := range groupedLogs {
		// Update baseline metrics
		a.updateBaseline(key, logs)

		// Detect anomalies
		anomalies := a.detectAnomalies(key, logs)
		for _, anomaly := range anomalies {
			a.storage.SaveAnalysis(ctx, anomaly)
		}

		// Update error patterns
		patterns := a.updateErrorPatterns(key, logs)
		for _, pattern := range patterns {
			a.storage.SaveAnalysis(ctx, pattern)
		}
	}
}

func (a *Analyzer) groupLogs(logs []*db.ApplicationLog) map[string][]*db.ApplicationLog {
	groups := make(map[string][]*db.ApplicationLog)
	for _, log := range logs {
		key := log.ApplicationID + ":" + log.ServiceName
		groups[key] = append(groups[key], log)
	}
	return groups
}

func (a *Analyzer) updateBaseline(key string, logs []*db.ApplicationLog) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.baselineMetrics[key]; !exists {
		a.baselineMetrics[key] = &baselineMetrics{
			ErrorRate: movingAverage{Window: 60}, // 1 hour with minute resolution
			ResponseTimes: movingAverage{Window: 60},
		}
	}

	// Calculate error rate
	errorCount := 0
	for _, log := range logs {
		if log.Severity == "ERROR" {
			errorCount++
		}
	}
	errorRate := float64(errorCount) / float64(len(logs))

	// Update moving averages
	baseline := a.baselineMetrics[key]
	baseline.ErrorRate.Values = append(baseline.ErrorRate.Values, errorRate)
	if len(baseline.ErrorRate.Values) > baseline.ErrorRate.Window {
		baseline.ErrorRate.Values = baseline.ErrorRate.Values[1:]
	}

	baseline.UpdatedAt = time.Now()
}

func (a *Analyzer) detectAnomalies(key string, logs []*db.ApplicationLog) []*db.AIAnalysis {
	a.mu.RLock()
	baseline, exists := a.baselineMetrics[key]
	a.mu.RUnlock()

	if !exists || time.Since(baseline.UpdatedAt) > time.Hour {
		return nil
	}

	var anomalies []*db.AIAnalysis

	// Check for error rate anomalies
	currentErrorRate := float64(countErrors(logs)) / float64(len(logs))
	mean, stdDev := stat.MeanStdDev(baseline.ErrorRate.Values, nil)
	
	if currentErrorRate > mean+2*stdDev {
		anomalies = append(anomalies, &db.AIAnalysis{
			Type:        "error_rate_anomaly",
			Severity:    "high",
			Description: "Abnormal increase in error rate detected",
			Details: json.RawMessage(fmt.Sprintf(`{
				"current_rate": %f,
				"baseline_mean": %f,
				"baseline_stddev": %f
			}`, currentErrorRate, mean, stdDev)),
			DetectedAt: time.Now(),
			Status:    "active",
		})
	}

	return anomalies
}

func (a *Analyzer) updateErrorPatterns(key string, logs []*db.ApplicationLog) []*db.AIAnalysis {
	errorLogs := filterErrorLogs(logs)
	if len(errorLogs) == 0 {
		return nil
	}

	patterns := make(map[string]*patternCluster)
	
	// Group similar error messages
	for _, log := range errorLogs {
		pattern := extractErrorPattern(log.Message)
		if _, exists := patterns[pattern]; !exists {
			patterns[pattern] = &patternCluster{
				Pattern:  pattern,
				Examples: make([]string, 0),
				Severity: log.Severity,
			}
		}
		
		cluster := patterns[pattern]
		cluster.Count++
		cluster.LastSeen = log.Timestamp
		if len(cluster.Examples) < 5 {
			cluster.Examples = append(cluster.Examples, log.Message)
		}
	}

	// Convert significant patterns to analysis entries
	var analyses []*db.AIAnalysis
	for _, cluster := range patterns {
		if cluster.Count >= 3 { // Threshold for significance
			analyses = append(analyses, &db.AIAnalysis{
				Type:        "error_pattern",
				Severity:    cluster.Severity,
				Description: "Recurring error pattern detected",
				Details: json.RawMessage(fmt.Sprintf(`{
					"pattern": %q,
					"count": %d,
					"examples": %v
				}`, cluster.Pattern, cluster.Count, cluster.Examples)),
				DetectedAt: cluster.LastSeen,
				Status:    "active",
			})
		}
	}

	return analyses
}

func countErrors(logs []*db.ApplicationLog) int {
	count := 0
	for _, log := range logs {
		if log.Severity == "ERROR" {
			count++
		}
	}
	return count
}

func filterErrorLogs(logs []*db.ApplicationLog) []*db.ApplicationLog {
	errors := make([]*db.ApplicationLog, 0)
	for _, log := range logs {
		if log.Severity == "ERROR" {
			errors = append(errors, log)
		}
	}
	return errors
}

func extractErrorPattern(message string) string {
	// Remove variable parts like IDs, timestamps, etc.
	pattern := message
	
	// Replace numbers
	pattern = regexp.MustCompile(`\d+`).ReplaceAllString(pattern, "N")
	
	// Replace UUIDs
	pattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`).ReplaceAllString(pattern, "UUID")
	
	// Replace timestamps
	pattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`).ReplaceAllString(pattern, "TIMESTAMP")
	
	// Replace email addresses
	pattern = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`).ReplaceAllString(pattern, "EMAIL")
	
	return pattern
}
