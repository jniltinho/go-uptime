// Package clickup implements the alerting provider that opens a task in a ClickUp list when an alert is
// triggered and closes the matching tasks when it is resolved, through the ClickUp REST API.
package clickup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"gopkg.in/yaml.v3"
)

// Errors returned by the validation of the configuration: ErrListIDNotSet and ErrTokenNotSet when the list ID
// or the API token is missing, ErrInvalidPriority when the priority is not one of the accepted names, and
// ErrDuplicateGroupOverride when an override has an empty or already used group.
var (
	ErrListIDNotSet           = errors.New("list-id not set")
	ErrTokenNotSet            = errors.New("token not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
	ErrInvalidPriority        = errors.New("priority must be one of: urgent, high, normal, low, none")
)

var priorityMap = map[string]int{
	"urgent": 1,
	"high":   2,
	"normal": 3,
	"low":    4,
	"none":   0,
}

// Config holds the ClickUp credentials and the content of the tasks created by the provider.
// Name and MarkdownContent are templates in which the [ENDPOINT_GROUP], [ENDPOINT_NAME], [ALERT_DESCRIPTION]
// and [RESULT_ERRORS] placeholders are replaced; MarkdownContent is read from the content key.
type Config struct {
	APIURL          string   `yaml:"api-url"`
	ListID          string   `yaml:"list-id"`
	Token           string   `yaml:"token"`
	Assignees       []string `yaml:"assignees"`
	Status          string   `yaml:"status"`
	Priority        string   `yaml:"priority"`
	NotifyAll       *bool    `yaml:"notify-all,omitempty"`
	Name            string   `yaml:"name,omitempty"`
	MarkdownContent string   `yaml:"content,omitempty"`
}

// Validate checks that ListID and Token are set and that Priority is known, and fills the defaults of the
// empty fields: normal priority, notify-all enabled, the public API URL and the task name and content templates.
func (cfg *Config) Validate() error {
	if cfg.ListID == "" {
		return ErrListIDNotSet
	}
	if cfg.Token == "" {
		return ErrTokenNotSet
	}
	if cfg.Priority == "" {
		cfg.Priority = "normal"
	}
	if _, ok := priorityMap[cfg.Priority]; !ok {
		return ErrInvalidPriority
	}
	if cfg.NotifyAll == nil {
		defaultNotifyAll := true
		cfg.NotifyAll = &defaultNotifyAll
	}
	if cfg.APIURL == "" {
		cfg.APIURL = "https://api.clickup.com/api/v2"
	}
	if cfg.Name == "" {
		cfg.Name = "Health Check: [ENDPOINT_GROUP]:[ENDPOINT_NAME]"
	}
	if cfg.MarkdownContent == "" {
		cfg.MarkdownContent = "Triggered: [ENDPOINT_GROUP] - [ENDPOINT_NAME] - [ALERT_DESCRIPTION] - [RESULT_ERRORS]"
	}
	return nil
}

// Merge copies every field set in override over cfg; empty strings, an empty Assignees list and a nil
// NotifyAll leave cfg untouched.
func (cfg *Config) Merge(override *Config) {
	if override.APIURL != "" {
		cfg.APIURL = override.APIURL
	}
	if override.ListID != "" {
		cfg.ListID = override.ListID
	}
	if override.Token != "" {
		cfg.Token = override.Token
	}
	if override.Status != "" {
		cfg.Status = override.Status
	}
	if override.Priority != "" {
		cfg.Priority = override.Priority
	}
	if override.NotifyAll != nil {
		cfg.NotifyAll = override.NotifyAll
	}
	if len(override.Assignees) > 0 {
		cfg.Assignees = override.Assignees
	}
	if override.Name != "" {
		cfg.Name = override.Name
	}
	if override.MarkdownContent != "" {
		cfg.MarkdownContent = override.MarkdownContent
	}
}

// AlertProvider is the configuration necessary for sending an alert using ClickUp
type AlertProvider struct {
	DefaultConfig Config `yaml:",inline"`

	// DefaultAlert is the default alert configuration to use for endpoints with an alert of the appropriate type
	DefaultAlert *alert.Alert `yaml:"default-alert,omitempty"`

	// Overrides is a list of Override that may be prioritized over the default configuration
	Overrides []Override `yaml:"overrides,omitempty"`
}

// Override is a case under which the default configuration is overridden
type Override struct {
	Group  string `yaml:"group"`
	Config `yaml:",inline"`
}

// Validate the provider's configuration
func (provider *AlertProvider) Validate() error {
	registeredGroups := make(map[string]bool)
	if provider.Overrides != nil {
		for _, override := range provider.Overrides {
			if isAlreadyRegistered := registeredGroups[override.Group]; isAlreadyRegistered || override.Group == "" {
				return ErrDuplicateGroupOverride
			}
			registeredGroups[override.Group] = true
		}
	}
	return provider.DefaultConfig.Validate()
}

// Send creates a task for a triggered alert, after replacing the placeholders of its name and content,
// or closes the tasks of the endpoint when resolved is true. The priority is omitted when it is none.
func (provider *AlertProvider) Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error {
	cfg, err := provider.GetConfig(ep.Group, alert)
	if err != nil {
		return err
	}
	if resolved {
		return provider.CloseTask(cfg, ep)
	}
	// Replace placeholders
	name := strings.ReplaceAll(cfg.Name, "[ENDPOINT_GROUP]", ep.Group)
	name = strings.ReplaceAll(name, "[ENDPOINT_NAME]", ep.Name)
	markdownContent := strings.ReplaceAll(cfg.MarkdownContent, "[ENDPOINT_GROUP]", ep.Group)
	markdownContent = strings.ReplaceAll(markdownContent, "[ENDPOINT_NAME]", ep.Name)
	markdownContent = strings.ReplaceAll(markdownContent, "[ALERT_DESCRIPTION]", alert.GetDescription())
	markdownContent = strings.ReplaceAll(markdownContent, "[RESULT_ERRORS]", strings.Join(result.Errors, ", "))
	body := map[string]interface{}{
		"name":             name,
		"markdown_content": markdownContent,
		"assignees":        cfg.Assignees,
		"status":           cfg.Status,
		"notify_all":       *cfg.NotifyAll,
	}
	if cfg.Priority != "none" {
		body["priority"] = priorityMap[cfg.Priority]
	}
	return provider.CreateTask(cfg, body)
}

// CreateTask posts body as a new task in the configured list and returns an error when the API answers
// with a status of 400 or above.
func (provider *AlertProvider) CreateTask(cfg *Config, body map[string]interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	createURL := fmt.Sprintf("%s/list/%s/task", cfg.APIURL, cfg.ListID)
	req, err := http.NewRequest("POST", createURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", cfg.Token)
	httpClient := client.GetHTTPClient(nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to create task, status: %d", resp.StatusCode)
	}
	return nil
}

// CloseTask sets the status closed on every open task of the list whose name contains both the group and
// the name of the endpoint. It returns an error when no task matches.
func (provider *AlertProvider) CloseTask(cfg *Config, ep *endpoint.Endpoint) error {
	fetchURL := fmt.Sprintf("%s/list/%s/task?include_closed=false", cfg.APIURL, cfg.ListID)
	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", cfg.Token)
	httpClient := client.GetHTTPClient(nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to fetch tasks, status: %d", resp.StatusCode)
	}
	var fetchResponse struct {
		Tasks []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"tasks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fetchResponse); err != nil {
		return err
	}
	var matchingTaskIDs []string
	for _, task := range fetchResponse.Tasks {
		if strings.Contains(task.Name, ep.Group) && strings.Contains(task.Name, ep.Name) {
			matchingTaskIDs = append(matchingTaskIDs, task.ID)
		}
	}
	if len(matchingTaskIDs) == 0 {
		return fmt.Errorf("no matching tasks found for %s:%s", ep.Group, ep.Name)
	}
	for _, taskID := range matchingTaskIDs {
		if err := provider.UpdateTaskStatus(cfg, taskID, "closed"); err != nil {
			return fmt.Errorf("failed to close task %s: %v", taskID, err)
		}
	}
	return nil
}

// UpdateTaskStatus changes the status of the task taskID and returns an error when the API answers with a
// status of 400 or above.
func (provider *AlertProvider) UpdateTaskStatus(cfg *Config, taskID, status string) error {
	updateURL := fmt.Sprintf("%s/task/%s", cfg.APIURL, taskID)
	body := map[string]interface{}{"status": status}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", updateURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", cfg.Token)
	httpClient := client.GetHTTPClient(nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to update task %s, status: %d", taskID, resp.StatusCode)
	}
	return nil
}

// GetDefaultAlert returns the provider's default alert configuration, or nil when none is configured.
func (provider *AlertProvider) GetDefaultAlert() *alert.Alert {
	return provider.DefaultAlert
}

// GetConfig returns the configuration for the provider with the overrides applied
func (provider *AlertProvider) GetConfig(group string, alert *alert.Alert) (*Config, error) {
	cfg := provider.DefaultConfig
	// Handle group overrides
	if provider.Overrides != nil {
		for _, override := range provider.Overrides {
			if group == override.Group {
				cfg.Merge(&override.Config)
				break
			}
		}
	}
	// Handle alert overrides
	if len(alert.ProviderOverride) != 0 {
		overrideConfig := Config{}
		if err := yaml.Unmarshal(alert.ProviderOverrideAsBytes(), &overrideConfig); err != nil {
			return nil, err
		}
		cfg.Merge(&overrideConfig)
	}
	// Validate the configuration
	err := cfg.Validate()
	return &cfg, err
}

// ValidateOverrides validates the alert's provider override and, if present, the group override.
func (provider *AlertProvider) ValidateOverrides(group string, alert *alert.Alert) error {
	_, err := provider.GetConfig(group, alert)
	return err
}
