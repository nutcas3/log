package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"api-watchtower/internal/db"

	"github.com/robfig/cron/v3"
)

type Engine struct {
	client  *http.Client
	cron    *cron.Cron
	targets map[string]*db.MonitoringTarget
	mu      sync.RWMutex
}

func NewEngine() *Engine {
	return &Engine{
		client: &http.Client{},
		cron:   cron.New(cron.WithSeconds()),
		targets: make(map[string]*db.MonitoringTarget),
	}
}

func (e *Engine) Start() {
	e.cron.Start()
}

func (e *Engine) Stop() {
	e.cron.Stop()
}

func (e *Engine) AddTarget(target *db.MonitoringTarget) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.targets[target.ID]; exists {
		e.removeTarget(target.ID)
	}

	e.targets[target.ID] = target

	_, err := e.cron.AddFunc(target.Frequency, func() {
		e.checkTarget(target)
	})

	return err
}

func (e *Engine) removeTarget(id string) {
	if target, exists := e.targets[id]; exists {
		// Find and remove the cron entry
		e.cron.Remove(cron.EntryID(target.ID))
		delete(e.targets, id)
	}
}

func (e *Engine) checkTarget(target *db.MonitoringTarget) *db.MonitoringResult {
	start := time.Now()
	result := &db.MonitoringResult{
		TargetID:  target.ID,
		Timestamp: start,
	}

	// Create request context with timeout
	timeout, _ := time.ParseDuration(target.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Prepare request
	req, err := e.prepareRequest(ctx, target)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Failed to prepare request: %v", err)
		return result
	}

	// Execute request
	resp, err := e.client.Do(req)
	result.ResponseTime = time.Since(start).Seconds()

	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	// Record response
	result.StatusCode = resp.StatusCode
	
	// Store headers
	headers := make(map[string][]string)
	for k, v := range resp.Header {
		headers[k] = v
	}
	headerBytes, _ := json.Marshal(headers)
	result.ResponseHeaders = headerBytes

	// Store body (limited size)
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
	result.ResponseBody = body

	// Check assertions
	result.Success = e.checkAssertions(target, result)

	return result
}

func (e *Engine) prepareRequest(ctx context.Context, target *db.MonitoringTarget) (*http.Request, error) {
	var body io.Reader
	if target.Body != nil {
		body = bytes.NewReader(target.Body)
	}

	req, err := http.NewRequestWithContext(ctx, target.Method, target.URL, body)
	if err != nil {
		return nil, err
	}

	// Add headers
	var headers map[string]string
	if err := json.Unmarshal(target.Headers, &headers); err == nil {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	// Add auth if configured
	if err := e.addAuth(req, target.AuthConfig); err != nil {
		return nil, err
	}

	return req, nil
}

func (e *Engine) addAuth(req *http.Request, authConfig json.RawMessage) error {
	if len(authConfig) == 0 {
		return nil
	}

	var auth struct {
		Type   string `json:"type"`
		Config json.RawMessage `json:"config"`
	}

	if err := json.Unmarshal(authConfig, &auth); err != nil {
		return err
	}

	switch auth.Type {
	case "bearer":
		var bearer struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(auth.Config, &bearer); err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+bearer.Token)

	case "basic":
		var basic struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(auth.Config, &basic); err != nil {
			return err
		}
		req.SetBasicAuth(basic.Username, basic.Password)

	case "apikey":
		var apiKey struct {
			Key      string `json:"key"`
			Location string `json:"location"`
			Name     string `json:"name"`
		}
		if err := json.Unmarshal(auth.Config, &apiKey); err != nil {
			return err
		}
		switch apiKey.Location {
		case "header":
			req.Header.Set(apiKey.Name, apiKey.Key)
		case "query":
			q := req.URL.Query()
			q.Add(apiKey.Name, apiKey.Key)
			req.URL.RawQuery = q.Encode()
		}
	}

	return nil
}

func (e *Engine) checkAssertions(target *db.MonitoringTarget, result *db.MonitoringResult) bool {
	// Check status code
	statusValid := false
	for _, expected := range target.ExpectedStatus {
		if result.StatusCode == expected {
			statusValid = true
			break
		}
	}
	if !statusValid {
		return false
	}

	// Check response rules
	var rules []struct {
		Type  string `json:"type"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}

	if err := json.Unmarshal(target.ResponseRules, &rules); err != nil {
		return false
	}

	for _, rule := range rules {
		switch rule.Type {
		case "json_path_exists":
			// Implementation for JSON path checking
		case "contains":
			if !bytes.Contains(result.ResponseBody, []byte(rule.Value)) {
				return false
			}
		case "regex":
			// Implementation for regex matching
		}
	}

	return true
}
