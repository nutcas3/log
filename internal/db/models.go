package db

import (
	"encoding/json"
	"time"
)

type MonitoringTarget struct {
	ID              string          `json:"id" db:"id"`
	Name            string          `json:"name" db:"name"`
	URL             string          `json:"url" db:"url"`
	Method          string          `json:"method" db:"method"`
	Headers         json.RawMessage `json:"headers" db:"headers"`
	Body            json.RawMessage `json:"body,omitempty" db:"body"`
	Frequency       string          `json:"frequency" db:"frequency"`
	Timeout         string          `json:"timeout" db:"timeout"`
	ExpectedStatus  []int          `json:"expected_status" db:"expected_status"`
	ResponseRules   json.RawMessage `json:"response_rules" db:"response_rules"`
	AuthConfig      json.RawMessage `json:"auth_config" db:"auth_config"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at" db:"updated_at"`
	LastCheckStatus string          `json:"last_check_status" db:"last_check_status"`
}

type MonitoringResult struct {
	ID              string          `json:"id" db:"id"`
	TargetID        string          `json:"target_id" db:"target_id"`
	StatusCode      int             `json:"status_code" db:"status_code"`
	ResponseTime    float64         `json:"response_time" db:"response_time"`
	Success         bool            `json:"success" db:"success"`
	Error           string          `json:"error,omitempty" db:"error"`
	ResponseHeaders json.RawMessage `json:"response_headers" db:"response_headers"`
	ResponseBody    json.RawMessage `json:"response_body" db:"response_body"`
	RuleResults     json.RawMessage `json:"rule_results" db:"rule_results"`
	Timestamp       time.Time       `json:"timestamp" db:"timestamp"`
}

type ApplicationLog struct {
	ID           string          `json:"id" db:"id"`
	ApplicationID string         `json:"application_id" db:"application_id"`
	ServiceName  string          `json:"service_name" db:"service_name"`
	Severity     string          `json:"severity" db:"severity"`
	Message      string          `json:"message" db:"message"`
	Timestamp    time.Time       `json:"timestamp" db:"timestamp"`
	InstanceID   string          `json:"instance_id,omitempty" db:"instance_id"`
	TraceID      string          `json:"trace_id,omitempty" db:"trace_id"`
	UserID       string          `json:"user_id,omitempty" db:"user_id"`
	Source       string          `json:"source,omitempty" db:"source"`
	Payload      json.RawMessage `json:"payload,omitempty" db:"payload"`
}

type AIAnalysis struct {
	ID            string          `json:"id" db:"id"`
	Type          string          `json:"type" db:"type"`
	Severity      string          `json:"severity" db:"severity"`
	Description   string          `json:"description" db:"description"`
	Details       json.RawMessage `json:"details" db:"details"`
	RelatedLogs   []string        `json:"related_logs" db:"related_logs"`
	DetectedAt    time.Time       `json:"detected_at" db:"detected_at"`
	Status        string          `json:"status" db:"status"`
	FeedbackScore int            `json:"feedback_score" db:"feedback_score"`
}

type Alert struct {
	ID          string          `json:"id" db:"id"`
	Type        string          `json:"type" db:"type"`
	Source      string          `json:"source" db:"source"`
	SourceID    string          `json:"source_id" db:"source_id"`
	Severity    string          `json:"severity" db:"severity"`
	Message     string          `json:"message" db:"message"`
	Details     json.RawMessage `json:"details" db:"details"`
	Status      string          `json:"status" db:"status"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
	ResolvedAt  *time.Time      `json:"resolved_at,omitempty" db:"resolved_at"`
	ResolvedBy  string          `json:"resolved_by,omitempty" db:"resolved_by"`
}
