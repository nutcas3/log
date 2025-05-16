package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"api-watchtower/internal/db"
)

type Manager struct {
	storage    Storage
	notifiers  []Notifier
	rules      map[string]*Rule
	mu         sync.RWMutex
}

type Storage interface {
	SaveAlert(ctx context.Context, alert *db.Alert) error
	UpdateAlert(ctx context.Context, alert *db.Alert) error
	GetActiveAlerts(ctx context.Context) ([]*db.Alert, error)
}

type Notifier interface {
	Send(ctx context.Context, alert *db.Alert) error
}

type Rule struct {
	ID          string
	Type        string
	Source      string
	Conditions  json.RawMessage
	Severity    string
	Message     string
	Cooldown    time.Duration
	LastTriggered map[string]time.Time
}

func NewManager(storage Storage, notifiers []Notifier) *Manager {
	return &Manager{
		storage:   storage,
		notifiers: notifiers,
		rules:    make(map[string]*Rule),
	}
}

func (m *Manager) AddRule(rule *Rule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules[rule.ID] = rule
}

func (m *Manager) RemoveRule(ruleID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rules, ruleID)
}

func (m *Manager) ProcessMonitoringResult(ctx context.Context, result *db.MonitoringResult) error {
	m.mu.RLock()
	rules := make([]*Rule, 0)
	for _, rule := range m.rules {
		if rule.Type == "monitoring" {
			rules = append(rules, rule)
		}
	}
	m.mu.RUnlock()

	for _, rule := range rules {
		if m.shouldTriggerAlert(rule, result) {
			if err := m.createAlert(ctx, rule, result); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *Manager) ProcessAIAnalysis(ctx context.Context, analysis *db.AIAnalysis) error {
	m.mu.RLock()
	rules := make([]*Rule, 0)
	for _, rule := range m.rules {
		if rule.Type == "ai_analysis" {
			rules = append(rules, rule)
		}
	}
	m.mu.RUnlock()

	for _, rule := range rules {
		if m.shouldTriggerAlert(rule, analysis) {
			if err := m.createAlert(ctx, rule, analysis); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *Manager) shouldTriggerAlert(rule *Rule, event interface{}) bool {
	// Check cooldown period
	sourceID := getSourceID(event)
	if sourceID == "" {
		return false
	}

	m.mu.Lock()
	lastTriggered, exists := rule.LastTriggered[sourceID]
	if exists && time.Since(lastTriggered) < rule.Cooldown {
		m.mu.Unlock()
		return false
	}
	rule.LastTriggered[sourceID] = time.Now()
	m.mu.Unlock()

	switch e := event.(type) {
	case *db.MonitoringResult:
		return m.evaluateMonitoringConditions(rule.Conditions, e)
	case *db.AIAnalysis:
		return m.evaluateAIConditions(rule.Conditions, e)
	default:
		return false
	}
}

func (m *Manager) evaluateMonitoringConditions(conditions json.RawMessage, result *db.MonitoringResult) bool {
	var cond struct {
		StatusCodes []int  `json:"status_codes"`
		MinLatency  float64 `json:"min_latency"`
		ErrorMatch  string  `json:"error_match"`
	}

	if err := json.Unmarshal(conditions, &cond); err != nil {
		return false
	}

	// Check status codes
	if len(cond.StatusCodes) > 0 {
		statusMatch := false
		for _, code := range cond.StatusCodes {
			if result.StatusCode == code {
				statusMatch = true
				break
			}
		}
		if !statusMatch {
			return false
		}
	}

	// Check latency
	if cond.MinLatency > 0 && result.ResponseTime < cond.MinLatency {
		return false
	}

	// Check error pattern
	if cond.ErrorMatch != "" && (result.Error == "" || !strings.Contains(result.Error, cond.ErrorMatch)) {
		return false
	}

	return true
}

func (m *Manager) evaluateAIConditions(conditions json.RawMessage, analysis *db.AIAnalysis) bool {
	var cond struct {
		Types      []string `json:"types"`
		Severities []string `json:"severities"`
	}

	if err := json.Unmarshal(conditions, &cond); err != nil {
		return false
	}

	// Check type
	if len(cond.Types) > 0 {
		typeMatch := false
		for _, t := range cond.Types {
			if analysis.Type == t {
				typeMatch = true
				break
			}
		}
		if !typeMatch {
			return false
		}
	}

	// Check severity
	if len(cond.Severities) > 0 {
		sevMatch := false
		for _, s := range cond.Severities {
			if analysis.Severity == s {
				sevMatch = true
				break
			}
		}
		if !sevMatch {
			return false
		}
	}

	return true
}

func (m *Manager) createAlert(ctx context.Context, rule *Rule, event interface{}) error {
	alert := &db.Alert{
		Type:      rule.Type,
		Source:    rule.Source,
		SourceID:  getSourceID(event),
		Severity:  rule.Severity,
		Message:   rule.Message,
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Add event-specific details
	details, err := json.Marshal(event)
	if err == nil {
		alert.Details = details
	}

	// Save alert
	if err := m.storage.SaveAlert(ctx, alert); err != nil {
		return fmt.Errorf("failed to save alert: %v", err)
	}

	// Send notifications
	for _, notifier := range m.notifiers {
		if err := notifier.Send(ctx, alert); err != nil {
			// Log error but continue with other notifiers
			fmt.Printf("Failed to send notification: %v\n", err)
		}
	}

	return nil
}

func (m *Manager) ResolveAlert(ctx context.Context, alertID, resolvedBy string) error {
	alert := &db.Alert{
		ID:         alertID,
		Status:     "resolved",
		ResolvedAt: func() *time.Time { t := time.Now(); return &t }(),
		ResolvedBy: resolvedBy,
		UpdatedAt:  time.Now(),
	}

	return m.storage.UpdateAlert(ctx, alert)
}

func getSourceID(event interface{}) string {
	switch e := event.(type) {
	case *db.MonitoringResult:
		return e.TargetID
	case *db.AIAnalysis:
		return e.ID
	default:
		return ""
	}
}
