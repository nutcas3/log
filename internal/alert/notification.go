package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/smtp"
	"sync"
	"time"
)

// NotificationManager handles the delivery of alerts through various channels
type NotificationManager struct {
	config     NotificationConfig
	templates  map[string]*template.Template
	rateLimit  map[string]*RateLimiter
	mu         sync.RWMutex
}

// Alert represents the structure of an alert to be sent via notifications
type Alert struct {
	Severity  string
	Title     string
	Timestamp string
	Source    string
	Message   string
	Details   string
	AlertURL  string
}

type NotificationConfig struct {
	Email    EmailConfig    `json:"email"`
	Slack    SlackConfig    `json:"slack"`
	Webhook  WebhookConfig  `json:"webhook"`
	Defaults DefaultConfig  `json:"defaults"`
}

type EmailConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
}

type SlackConfig struct {
	WebhookURL string `json:"webhook_url"`
	Channel    string `json:"channel"`
}

type WebhookConfig struct {
	URLs map[string]string `json:"urls"`
}

type DefaultConfig struct {
	MinInterval    time.Duration `json:"min_interval"`
	GroupingDelay time.Duration `json:"grouping_delay"`
	Recipients    []string      `json:"recipients"`
}

// RateLimiter implements a token bucket algorithm
type RateLimiter struct {
	tokens     float64
	rate       float64
	burst      float64
	lastUpdate time.Time
	mu         sync.Mutex
}

func NewNotificationManager(config NotificationConfig) *NotificationManager {
	nm := &NotificationManager{
		config:    config,
		templates: make(map[string]*template.Template),
		rateLimit: make(map[string]*RateLimiter),
	}

	// Initialize templates
	nm.loadTemplates()

	return nm
}

func (nm *NotificationManager) loadTemplates() {
	// Email template
	emailTmpl := `
Subject: {{ .Severity }} Alert - {{ .Title }}

Alert Details:
Severity: {{ .Severity }}
Time: {{ .Timestamp }}
Source: {{ .Source }}

Message:
{{ .Message }}

{{ if .Details }}Additional Details:
{{ .Details }}{{ end }}

View Alert: {{ .AlertURL }}
	`
	nm.templates["email"] = template.Must(template.New("email").Parse(emailTmpl))

	// Slack template
	slackTmpl := `{
		"blocks": [
			{
				"type": "header",
				"text": {
					"type": "plain_text",
					"text": "{{ .Severity }} Alert - {{ .Title }}"
				}
			},
			{
				"type": "section",
				"fields": [
					{
						"type": "mrkdwn",
						"text": "*Time:*\n{{ .Timestamp }}"
					},
					{
						"type": "mrkdwn",
						"text": "*Source:*\n{{ .Source }}"
					}
				]
			},
			{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": "{{ .Message }}"
				}
			}
		]
	}`
	nm.templates["slack"] = template.Must(template.New("slack").Parse(slackTmpl))
}

func (nm *NotificationManager) Send(ctx context.Context, alert *Alert, channels []string) error {
	if !nm.shouldSend(alert) {
		return nil
	}

	var wg sync.WaitGroup
	errors := make(chan error, len(channels))

	for _, channel := range channels {
		wg.Add(1)
		go func(ch string) {
			defer wg.Done()
			if err := nm.sendToChannel(ctx, alert, ch); err != nil {
				errors <- fmt.Errorf("failed to send to %s: %v", ch, err)
			}
		}(channel)
	}

	// Wait for all notifications to complete
	wg.Wait()
	close(errors)

	// Collect any errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification errors: %v", errs)
	}
	return nil
}

func (nm *NotificationManager) shouldSend(alert *Alert) bool {
	nm.mu.RLock()
	limiter, exists := nm.rateLimit[alert.Source]
	nm.mu.RUnlock()

	if !exists {
		nm.mu.Lock()
		limiter = &RateLimiter{
			rate:       1.0 / nm.config.Defaults.MinInterval.Seconds(),
			burst:      3.0,
			lastUpdate: time.Now(),
		}
		nm.rateLimit[alert.Source] = limiter
		nm.mu.Unlock()
	}

	return limiter.Allow()
}

func (nm *NotificationManager) sendToChannel(ctx context.Context, alert *Alert, channel string) error {
	switch channel {
	case "email":
		return nm.sendEmail(ctx, alert)
	case "slack":
		return nm.sendSlack(ctx, alert)
	case "webhook":
		return nm.sendWebhook(ctx, alert)
	default:
		return fmt.Errorf("unsupported notification channel: %s", channel)
	}
}

func (nm *NotificationManager) sendEmail(ctx context.Context, alert *Alert) error {
	var body bytes.Buffer
	if err := nm.templates["email"].Execute(&body, alert); err != nil {
		return err
	}

	auth := smtp.PlainAuth("",
		nm.config.Email.Username,
		nm.config.Email.Password,
		nm.config.Email.Host,
	)

	return smtp.SendMail(
		fmt.Sprintf("%s:%d", nm.config.Email.Host, nm.config.Email.Port),
		auth,
		nm.config.Email.From,
		nm.config.Defaults.Recipients,
		body.Bytes(),
	)
}

func (nm *NotificationManager) sendSlack(ctx context.Context, alert *Alert) error {
	var payload bytes.Buffer
	if err := nm.templates["slack"].Execute(&payload, alert); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", nm.config.Slack.WebhookURL, &payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status: %d", resp.StatusCode)
	}

	return nil
}

func (nm *NotificationManager) sendWebhook(ctx context.Context, alert *Alert) error {
	payload, err := json.Marshal(alert)
	if err != nil {
		return err
	}

	for name, url := range nm.config.Webhook.URLs {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("webhook %s: %v", name, err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("webhook %s: %v", name, err)
		}
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("webhook %s returned status: %d", name, resp.StatusCode)
		}
	}

	return nil
}

// RateLimiter methods
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()
	rl.tokens = math.Min(rl.burst, rl.tokens+elapsed*rl.rate)
	rl.lastUpdate = now

	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}
	return false
}
