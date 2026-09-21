// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package awsses implements the alerting provider that sends alerts by email through AWS Simple Email Service,
// using the SendEmail operation of the AWS SDK.
package awsses

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"gopkg.in/yaml.v3"
)

const (
	// CharSet is the character set declared for the subject and the body of the emails sent through SES.
	CharSet = "UTF-8"
)

// Errors returned by the validation of the configuration: ErrMissingFromOrToFields when the sender or the
// recipients are missing, ErrInvalidAWSAuthConfig when only one of the access key ID and the secret access key
// is set, and ErrDuplicateGroupOverride when an override has no recipient or an empty or already used group.
var (
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
	ErrMissingFromOrToFields  = errors.New("from and to fields are required")
	ErrInvalidAWSAuthConfig   = errors.New("either both or neither of access-key-id and secret-access-key must be specified")
)

// Config holds the AWS credentials, the region and the addresses used to send an alert through SES.
// When both AccessKeyID and SecretAccessKey are empty, the default AWS credential chain (IAM) is used.
// To may hold several addresses separated by commas.
type Config struct {
	AccessKeyID     string `yaml:"access-key-id"`
	SecretAccessKey string `yaml:"secret-access-key"`
	Region          string `yaml:"region"`

	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// Validate checks that From and To are set and that the access key ID and the secret access key are either
// both set or both empty.
func (cfg *Config) Validate() error {
	if len(cfg.From) == 0 || len(cfg.To) == 0 {
		return ErrMissingFromOrToFields
	}
	if !((len(cfg.AccessKeyID) == 0 && len(cfg.SecretAccessKey) == 0) || (len(cfg.AccessKeyID) > 0 && len(cfg.SecretAccessKey) > 0)) {
		// if both AccessKeyID and SecretAccessKey are specified, we'll use these to authenticate,
		// otherwise if neither are specified, then we'll fall back on IAM authentication.
		return ErrInvalidAWSAuthConfig
	}
	return nil
}

// Merge copies every non-empty field of override over cfg; empty fields of override leave cfg untouched.
func (cfg *Config) Merge(override *Config) {
	if len(override.AccessKeyID) > 0 {
		cfg.AccessKeyID = override.AccessKeyID
	}
	if len(override.SecretAccessKey) > 0 {
		cfg.SecretAccessKey = override.SecretAccessKey
	}
	if len(override.Region) > 0 {
		cfg.Region = override.Region
	}
	if len(override.From) > 0 {
		cfg.From = override.From
	}
	if len(override.To) > 0 {
		cfg.To = override.To
	}
}

// AlertProvider is the configuration necessary for sending an alert using AWS Simple Email Service
type AlertProvider struct {
	DefaultConfig Config `yaml:",inline"`

	// DefaultAlert is the default alert configuration to use for endpoints with an alert of the appropriate type
	DefaultAlert *alert.Alert `yaml:"default-alert,omitempty"`

	// Overrides is a list of Override that may be prioritized over the default configuration
	Overrides []Override `yaml:"overrides,omitempty"`
}

// Override is a case under which the default integration is overridden
type Override struct {
	Group  string `yaml:"group"`
	Config `yaml:",inline"`
}

// Validate the provider's configuration
func (provider *AlertProvider) Validate() error {
	registeredGroups := make(map[string]bool)
	if provider.Overrides != nil {
		for _, override := range provider.Overrides {
			if isAlreadyRegistered := registeredGroups[override.Group]; isAlreadyRegistered || override.Group == "" || len(override.To) == 0 {
				return ErrDuplicateGroupOverride
			}
			registeredGroups[override.Group] = true
		}
	}
	return provider.DefaultConfig.Validate()
}

// Send an alert using the provider
func (provider *AlertProvider) Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error {
	cfg, err := provider.GetConfig(ep.Group, alert)
	if err != nil {
		return err
	}
	ctx := context.Background()
	svc, err := provider.createClient(ctx, cfg)
	if err != nil {
		return err
	}
	subject, body := provider.buildMessageSubjectAndBody(ep, alert, result, resolved)
	emails := strings.Split(cfg.To, ",")
	input := &ses.SendEmailInput{
		Destination: &types.Destination{
			ToAddresses: emails,
		},
		Message: &types.Message{
			Body: &types.Body{
				Text: &types.Content{
					Charset: aws.String(CharSet),
					Data:    aws.String(body),
				},
			},
			Subject: &types.Content{
				Charset: aws.String(CharSet),
				Data:    aws.String(subject),
			},
		},
		Source: aws.String(cfg.From),
	}
	if _, err = svc.SendEmail(ctx, input); err != nil {
		return err
	}
	return nil
}

func (provider *AlertProvider) createClient(ctx context.Context, cfg *Config) (*ses.Client, error) {
	var opts []func(*config.LoadOptions) error
	if len(cfg.Region) > 0 {
		opts = append(opts, config.WithRegion(cfg.Region))
	}
	if len(cfg.AccessKeyID) > 0 && len(cfg.SecretAccessKey) > 0 {
		opts = append(opts, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")))
	}
	awsConfig, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return ses.NewFromConfig(awsConfig), nil
}

// buildMessageSubjectAndBody builds the message subject and body
func (provider *AlertProvider) buildMessageSubjectAndBody(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) (string, string) {
	var subject, message string
	if resolved {
		subject = fmt.Sprintf("[%s] Alert resolved", ep.DisplayName())
		message = fmt.Sprintf("An alert for %s has been resolved after passing successfully %d time(s) in a row", ep.DisplayName(), alert.SuccessThreshold)
	} else {
		subject = fmt.Sprintf("[%s] Alert triggered", ep.DisplayName())
		message = fmt.Sprintf("An alert for %s has been triggered due to having failed %d time(s) in a row", ep.DisplayName(), alert.FailureThreshold)
	}
	var formattedConditionResults string
	if len(result.ConditionResults) > 0 {
		formattedConditionResults = "\n\nCondition results:\n"
		for _, conditionResult := range result.ConditionResults {
			var prefix string
			if conditionResult.Success {
				prefix = "✅"
			} else {
				prefix = "❌"
			}
			formattedConditionResults += fmt.Sprintf("%s %s\n", prefix, conditionResult.Condition)
		}
	}
	var description string
	if alertDescription := alert.GetDescription(); len(alertDescription) > 0 {
		description = "\n\nAlert description: " + alertDescription
	}
	return subject, message + description + formattedConditionResults
}

// GetDefaultAlert returns the provider's default alert configuration
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

// ValidateOverrides validates the alert's provider override and, if present, the group override
func (provider *AlertProvider) ValidateOverrides(group string, alert *alert.Alert) error {
	_, err := provider.GetConfig(group, alert)
	return err
}
