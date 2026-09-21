// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package provider defines the AlertProvider interface implemented by every alerting provider and the Config
// contract of their configurations, checks both at compile time for each provider, and merges a provider's
// default alert into an endpoint alert.
package provider

import (
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/awsses"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/clickup"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/custom"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/datadog"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/discord"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/email"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/gitea"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/github"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/gitlab"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/googlechat"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/gotify"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/homeassistant"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/ifttt"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/ilert"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/incidentio"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/line"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/matrix"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/mattermost"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/messagebird"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/n8n"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/newrelic"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/ntfy"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/opsgenie"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/pagerduty"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/plivo"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/pushover"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/rocketchat"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/sendgrid"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/signal"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/signl4"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/slack"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/splunk"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/squadcast"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/teams"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/teamsworkflows"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/telegram"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/twilio"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/webex"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/zapier"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/zulip"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

// AlertProvider is the interface that each provider should implement
type AlertProvider interface {
	// Validate the provider's configuration
	Validate() error

	// Send an alert using the provider
	Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error

	// GetDefaultAlert returns the provider's default alert configuration
	GetDefaultAlert() *alert.Alert

	// ValidateOverrides validates the alert's provider override and, if present, the group override
	ValidateOverrides(group string, alert *alert.Alert) error
}

// Config is the contract of a provider's configuration type T: it validates itself and absorbs an override of
// its own type. It exists so that the compiler checks the Config of every provider.
type Config[T any] interface {
	// Validate checks that the configuration is usable; some providers also fill in default values here.
	Validate() error
	// Merge overwrites the fields of the configuration with the ones that are set in override.
	Merge(override *T)
}

// MergeProviderDefaultAlertIntoEndpointAlert parses an Endpoint alert by using the provider's default alert as a baseline
func MergeProviderDefaultAlertIntoEndpointAlert(providerDefaultAlert, endpointAlert *alert.Alert) {
	if providerDefaultAlert == nil || endpointAlert == nil {
		return
	}
	if endpointAlert.Enabled == nil {
		endpointAlert.Enabled = providerDefaultAlert.Enabled
	}
	if endpointAlert.SendOnResolved == nil {
		endpointAlert.SendOnResolved = providerDefaultAlert.SendOnResolved
	}
	if endpointAlert.Description == nil {
		endpointAlert.Description = providerDefaultAlert.Description
	}
	if endpointAlert.FailureThreshold == 0 {
		endpointAlert.FailureThreshold = providerDefaultAlert.FailureThreshold
	}
	if endpointAlert.SuccessThreshold == 0 {
		endpointAlert.SuccessThreshold = providerDefaultAlert.SuccessThreshold
	}
	if endpointAlert.MinimumReminderInterval == 0 {
		endpointAlert.MinimumReminderInterval = providerDefaultAlert.MinimumReminderInterval
	}
}

var (
	// Validate provider interface implementation on compile
	_ AlertProvider = (*awsses.AlertProvider)(nil)
	_ AlertProvider = (*clickup.AlertProvider)(nil)
	_ AlertProvider = (*custom.AlertProvider)(nil)
	_ AlertProvider = (*datadog.AlertProvider)(nil)
	_ AlertProvider = (*discord.AlertProvider)(nil)
	_ AlertProvider = (*email.AlertProvider)(nil)
	_ AlertProvider = (*gitea.AlertProvider)(nil)
	_ AlertProvider = (*github.AlertProvider)(nil)
	_ AlertProvider = (*gitlab.AlertProvider)(nil)
	_ AlertProvider = (*googlechat.AlertProvider)(nil)
	_ AlertProvider = (*gotify.AlertProvider)(nil)
	_ AlertProvider = (*homeassistant.AlertProvider)(nil)
	_ AlertProvider = (*ifttt.AlertProvider)(nil)
	_ AlertProvider = (*ilert.AlertProvider)(nil)
	_ AlertProvider = (*incidentio.AlertProvider)(nil)
	_ AlertProvider = (*line.AlertProvider)(nil)
	_ AlertProvider = (*matrix.AlertProvider)(nil)
	_ AlertProvider = (*mattermost.AlertProvider)(nil)
	_ AlertProvider = (*messagebird.AlertProvider)(nil)
	_ AlertProvider = (*n8n.AlertProvider)(nil)
	_ AlertProvider = (*newrelic.AlertProvider)(nil)
	_ AlertProvider = (*ntfy.AlertProvider)(nil)
	_ AlertProvider = (*opsgenie.AlertProvider)(nil)
	_ AlertProvider = (*pagerduty.AlertProvider)(nil)
	_ AlertProvider = (*plivo.AlertProvider)(nil)
	_ AlertProvider = (*pushover.AlertProvider)(nil)
	_ AlertProvider = (*rocketchat.AlertProvider)(nil)
	_ AlertProvider = (*sendgrid.AlertProvider)(nil)
	_ AlertProvider = (*signal.AlertProvider)(nil)
	_ AlertProvider = (*signl4.AlertProvider)(nil)
	_ AlertProvider = (*slack.AlertProvider)(nil)
	_ AlertProvider = (*splunk.AlertProvider)(nil)
	_ AlertProvider = (*squadcast.AlertProvider)(nil)
	_ AlertProvider = (*teams.AlertProvider)(nil)
	_ AlertProvider = (*teamsworkflows.AlertProvider)(nil)
	_ AlertProvider = (*telegram.AlertProvider)(nil)
	_ AlertProvider = (*twilio.AlertProvider)(nil)
	_ AlertProvider = (*webex.AlertProvider)(nil)
	_ AlertProvider = (*zapier.AlertProvider)(nil)
	_ AlertProvider = (*zulip.AlertProvider)(nil)

	// Validate config interface implementation on compile
	_ Config[awsses.Config]         = (*awsses.Config)(nil)
	_ Config[clickup.Config]        = (*clickup.Config)(nil)
	_ Config[custom.Config]         = (*custom.Config)(nil)
	_ Config[datadog.Config]        = (*datadog.Config)(nil)
	_ Config[discord.Config]        = (*discord.Config)(nil)
	_ Config[email.Config]          = (*email.Config)(nil)
	_ Config[gitea.Config]          = (*gitea.Config)(nil)
	_ Config[github.Config]         = (*github.Config)(nil)
	_ Config[gitlab.Config]         = (*gitlab.Config)(nil)
	_ Config[googlechat.Config]     = (*googlechat.Config)(nil)
	_ Config[gotify.Config]         = (*gotify.Config)(nil)
	_ Config[homeassistant.Config]  = (*homeassistant.Config)(nil)
	_ Config[ifttt.Config]          = (*ifttt.Config)(nil)
	_ Config[ilert.Config]          = (*ilert.Config)(nil)
	_ Config[incidentio.Config]     = (*incidentio.Config)(nil)
	_ Config[line.Config]           = (*line.Config)(nil)
	_ Config[matrix.Config]         = (*matrix.Config)(nil)
	_ Config[mattermost.Config]     = (*mattermost.Config)(nil)
	_ Config[messagebird.Config]    = (*messagebird.Config)(nil)
	_ Config[n8n.Config]            = (*n8n.Config)(nil)
	_ Config[newrelic.Config]       = (*newrelic.Config)(nil)
	_ Config[ntfy.Config]           = (*ntfy.Config)(nil)
	_ Config[opsgenie.Config]       = (*opsgenie.Config)(nil)
	_ Config[pagerduty.Config]      = (*pagerduty.Config)(nil)
	_ Config[plivo.Config]          = (*plivo.Config)(nil)
	_ Config[pushover.Config]       = (*pushover.Config)(nil)
	_ Config[rocketchat.Config]     = (*rocketchat.Config)(nil)
	_ Config[sendgrid.Config]       = (*sendgrid.Config)(nil)
	_ Config[signal.Config]         = (*signal.Config)(nil)
	_ Config[signl4.Config]         = (*signl4.Config)(nil)
	_ Config[slack.Config]          = (*slack.Config)(nil)
	_ Config[splunk.Config]         = (*splunk.Config)(nil)
	_ Config[squadcast.Config]      = (*squadcast.Config)(nil)
	_ Config[teams.Config]          = (*teams.Config)(nil)
	_ Config[teamsworkflows.Config] = (*teamsworkflows.Config)(nil)
	_ Config[telegram.Config]       = (*telegram.Config)(nil)
	_ Config[twilio.Config]         = (*twilio.Config)(nil)
	_ Config[webex.Config]          = (*webex.Config)(nil)
	_ Config[zapier.Config]         = (*zapier.Config)(nil)
	_ Config[zulip.Config]          = (*zulip.Config)(nil)
)
