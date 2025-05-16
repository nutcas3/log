package alert

import (
	"container/heap"
	"sync"
	"time"
)

// CorrelationEngine analyzes and groups related alerts
type CorrelationEngine struct {
	rules           []CorrelationRule
	activeGroups    map[string]*AlertGroup
	groupTTL        time.Duration
	cleanupInterval time.Duration
	mu             sync.RWMutex
}

type CorrelationRule struct {
	ID          string
	Name        string
	Description string
	Conditions  []CorrelationCondition
	GroupBy     []string
	MinCount    int
	TimeWindow  time.Duration
}

type CorrelationCondition struct {
	Field    string
	Operator string
	Value    interface{}
}

type AlertGroup struct {
	ID        string
	Rule      *CorrelationRule
	Alerts    []*Alert
	FirstSeen time.Time
	LastSeen  time.Time
	Status    string
	Score     float64
}

// alertHeap implements a min-heap of alerts by timestamp
type alertHeap []*Alert

func (h alertHeap) Len() int           { return len(h) }
func (h alertHeap) Less(i, j int) bool { return h[i].CreatedAt.Before(h[j].CreatedAt) }
func (h alertHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *alertHeap) Push(x interface{}) {
	*h = append(*h, x.(*Alert))
}

func (h *alertHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func NewCorrelationEngine(rules []CorrelationRule) *CorrelationEngine {
	engine := &CorrelationEngine{
		rules:           rules,
		activeGroups:    make(map[string]*AlertGroup),
		groupTTL:        24 * time.Hour,
		cleanupInterval: time.Hour,
	}

	go engine.cleanupRoutine()
	return engine
}

func (ce *CorrelationEngine) ProcessAlert(alert *Alert) ([]*AlertGroup, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	var updatedGroups []*AlertGroup

	// Find matching rules
	for _, rule := range ce.rules {
		if ce.matchesRule(alert, rule) {
			groupKey := ce.generateGroupKey(alert, rule)
			group := ce.getOrCreateGroup(groupKey, &rule)
			
			// Add alert to group
			group.Alerts = append(group.Alerts, alert)
			group.LastSeen = alert.CreatedAt
			
			// Update group status
			ce.updateGroupStatus(group)
			
			updatedGroups = append(updatedGroups, group)
		}
	}

	return updatedGroups, nil
}

func (ce *CorrelationEngine) matchesRule(alert *Alert, rule CorrelationRule) bool {
	for _, cond := range rule.Conditions {
		if !ce.matchesCondition(alert, cond) {
			return false
		}
	}
	return true
}

func (ce *CorrelationEngine) matchesCondition(alert *Alert, cond CorrelationCondition) bool {
	var fieldValue interface{}
	
	// Extract field value based on field path
	switch cond.Field {
	case "type":
		fieldValue = alert.Type
	case "source":
		fieldValue = alert.Source
	case "severity":
		fieldValue = alert.Severity
	default:
		// Try to find in details
		if details, ok := alert.Details.(map[string]interface{}); ok {
			fieldValue = details[cond.Field]
		}
	}

	if fieldValue == nil {
		return false
	}

	// Compare using operator
	switch cond.Operator {
	case "equals":
		return fieldValue == cond.Value
	case "contains":
		if str, ok := fieldValue.(string); ok {
			if pattern, ok := cond.Value.(string); ok {
				return strings.Contains(str, pattern)
			}
		}
	case "in":
		if values, ok := cond.Value.([]interface{}); ok {
			for _, v := range values {
				if v == fieldValue {
					return true
				}
			}
		}
	}

	return false
}

func (ce *CorrelationEngine) generateGroupKey(alert *Alert, rule CorrelationRule) string {
	var parts []string
	parts = append(parts, rule.ID)

	for _, field := range rule.GroupBy {
		var value string
		switch field {
		case "type":
			value = alert.Type
		case "source":
			value = alert.Source
		case "severity":
			value = alert.Severity
		default:
			if details, ok := alert.Details.(map[string]interface{}); ok {
				if v, ok := details[field].(string); ok {
					value = v
				}
			}
		}
		parts = append(parts, value)
	}

	return strings.Join(parts, ":")
}

func (ce *CorrelationEngine) getOrCreateGroup(key string, rule *CorrelationRule) *AlertGroup {
	group, exists := ce.activeGroups[key]
	if !exists {
		group = &AlertGroup{
			ID:        key,
			Rule:      rule,
			Alerts:    make([]*Alert, 0),
			FirstSeen: time.Now(),
			Status:    "active",
		}
		ce.activeGroups[key] = group
	}
	return group
}

func (ce *CorrelationEngine) updateGroupStatus(group *AlertGroup) {
	// Remove old alerts outside the time window
	cutoff := time.Now().Add(-group.Rule.TimeWindow)
	
	var activeAlerts []*Alert
	for _, alert := range group.Alerts {
		if alert.CreatedAt.After(cutoff) {
			activeAlerts = append(activeAlerts, alert)
		}
	}
	group.Alerts = activeAlerts

	// Update status based on alert count and time window
	if len(group.Alerts) >= group.Rule.MinCount {
		group.Status = "critical"
		group.Score = float64(len(group.Alerts)) / float64(group.Rule.MinCount)
	} else {
		group.Status = "active"
		group.Score = float64(len(group.Alerts)) / float64(group.Rule.MinCount)
	}
}

func (ce *CorrelationEngine) cleanupRoutine() {
	ticker := time.NewTicker(ce.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		ce.cleanup()
	}
}

func (ce *CorrelationEngine) cleanup() {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	now := time.Now()
	for key, group := range ce.activeGroups {
		if now.Sub(group.LastSeen) > ce.groupTTL {
			delete(ce.activeGroups, key)
		}
	}
}

// GetActiveGroups returns all active alert groups
func (ce *CorrelationEngine) GetActiveGroups() []*AlertGroup {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	groups := make([]*AlertGroup, 0, len(ce.activeGroups))
	for _, group := range ce.activeGroups {
		if group.Status != "resolved" {
			groups = append(groups, group)
		}
	}

	// Sort by score descending
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Score > groups[j].Score
	})

	return groups
}

// ResolveGroup marks an alert group as resolved
func (ce *CorrelationEngine) ResolveGroup(groupID string) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	group, exists := ce.activeGroups[groupID]
	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	group.Status = "resolved"
	return nil
}
