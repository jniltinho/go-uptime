// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"strings"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
)

// clientSecretPaths and clientPublicPaths classify the fields of client.Config, used by the providers that accept a
// client section. The fields that read files or credentials of the server are refused anyway (see forbiddenClientPaths)
// and are only classified so that a definition stored before they were refused never leaks them.
var (
	clientSecretPaths = []string{
		"client.oauth2.client-secret",
		"client.proxy-url",
		"client.tls.certificate-file",
		"client.tls.private-key-file",
	}
	clientPublicPaths = []string{
		"client.dns-resolver",
		"client.identity-aware-proxy.audience",
		"client.ignore-redirect",
		"client.insecure",
		"client.network",
		"client.oauth2.client-id",
		"client.oauth2.scopes",
		"client.oauth2.token-url",
		"client.store-cookies",
		"client.timeout",
		"client.tls.renegotiation",
		"client.tunnel",
	}
)

// typesWithClient are the providers whose configuration has a client section
var typesWithClient = []alert.Type{
	alert.TypeCustom, alert.TypeEmail, alert.TypeGitea, alert.TypeGoogleChat, alert.TypeMattermost, alert.TypeSendGrid,
	alert.TypeTeams, alert.TypeTelegram,
}

// secretPaths are the fields of each provider whose value is a credential or carries one: tokens, passwords, keys,
// webhook URLs and anything a request template can hide a token in. A field that might carry a credential is secret.
var secretPaths = map[alert.Type][]string{
	alert.TypeAWSSES:         {"access-key-id", "secret-access-key"},
	alert.TypeClickUp:        {"token"},
	alert.TypeCustom:         {"body", "headers.*", "placeholders.*.*", "url"},
	alert.TypeDatadog:        {"api-key"},
	alert.TypeDiscord:        {"webhook-url"},
	alert.TypeEmail:          {"password"},
	alert.TypeGitea:          {"token"},
	alert.TypeGitHub:         {"token"},
	alert.TypeGitLab:         {"authorization-key", "webhook-url"},
	alert.TypeGoogleChat:     {"webhook-url"},
	alert.TypeGotify:         {"token"},
	alert.TypeHomeAssistant:  {"token"},
	alert.TypeIFTTT:          {"webhook-key"},
	alert.TypeIlert:          {"integration-key"},
	alert.TypeIncidentIO:     {"auth-token", "url"},
	alert.TypeLine:           {"channel-access-token"},
	alert.TypeMatrix:         {"access-token"},
	alert.TypeMattermost:     {"webhook-url"},
	alert.TypeMessagebird:    {"access-key"},
	alert.TypeN8N:            {"webhook-url"},
	alert.TypeNewRelic:       {"insert-key"},
	alert.TypeNtfy:           {"token"},
	alert.TypeOpsgenie:       {"api-key"},
	alert.TypePagerDuty:      {"integration-key"},
	alert.TypePlivo:          {"auth-token"},
	alert.TypePushover:       {"application-token", "user-key"},
	alert.TypeRocketChat:     {"webhook-url"},
	alert.TypeSendGrid:       {"api-key"},
	alert.TypeSignal:         {},
	alert.TypeSIGNL4:         {"team-secret"},
	alert.TypeSlack:          {"webhook-url"},
	alert.TypeSplunk:         {"hec-token"},
	alert.TypeSquadcast:      {"webhook-url"},
	alert.TypeTeams:          {"webhook-url"},
	alert.TypeTeamsWorkflows: {"webhook-url"},
	alert.TypeTelegram:       {"token"},
	alert.TypeTwilio:         {"token"},
	alert.TypeVonage:         {"api-key", "api-secret"},
	alert.TypeWebex:          {"webhook-url"},
	alert.TypeZapier:         {"webhook-url"},
	alert.TypeZulip:          {"bot-api-key"},
}

// publicPaths are the other fields of each provider, shown as they are in the administration
var publicPaths = map[alert.Type][]string{
	alert.TypeAWSSES:         {"from", "region", "to"},
	alert.TypeClickUp:        {"api-url", "assignees", "content", "list-id", "name", "notify-all", "priority", "status"},
	alert.TypeCustom:         {"method"},
	alert.TypeDatadog:        {"site", "tags"},
	alert.TypeDiscord:        {"message-content", "title"},
	alert.TypeEmail:          {"from", "host", "port", "to", "username"},
	alert.TypeGitea:          {"assignees", "repository-url"},
	alert.TypeGitHub:         {"repository-url"},
	alert.TypeGitLab:         {"environment-name", "monitoring-tool", "service", "severity"},
	alert.TypeGoogleChat:     {},
	alert.TypeGotify:         {"priority", "server-url", "title"},
	alert.TypeHomeAssistant:  {"event-type", "url"},
	alert.TypeIFTTT:          {"event-name"},
	alert.TypeIlert:          {},
	alert.TypeIncidentIO:     {"metadata.*", "source-url"},
	alert.TypeLine:           {"user-ids"},
	alert.TypeMatrix:         {"internal-room-id", "server-url"},
	alert.TypeMattermost:     {"channel"},
	alert.TypeMessagebird:    {"originator", "recipients"},
	alert.TypeN8N:            {"title"},
	alert.TypeNewRelic:       {"account-id", "region"},
	alert.TypeNtfy:           {"click", "disable-cache", "disable-firebase", "email", "priority", "topic", "url"},
	alert.TypeOpsgenie:       {"alias-prefix", "entity-prefix", "priority", "source", "tags"},
	alert.TypePagerDuty:      {},
	alert.TypePlivo:          {"auth-id", "from", "to"},
	alert.TypePushover:       {"device", "priority", "resolved-priority", "sound", "title", "ttl"},
	alert.TypeRocketChat:     {"channel"},
	alert.TypeSendGrid:       {"from", "to"},
	alert.TypeSignal:         {"api-url", "number", "recipients"},
	alert.TypeSIGNL4:         {},
	alert.TypeSlack:          {"title"},
	alert.TypeSplunk:         {"hec-url", "index", "source", "sourcetype"},
	alert.TypeSquadcast:      {},
	alert.TypeTeams:          {"title"},
	alert.TypeTeamsWorkflows: {"title"},
	alert.TypeTelegram:       {"api-url", "id", "topic-id"},
	alert.TypeTwilio:         {"from", "sid", "text-twilio-resolved", "text-twilio-triggered", "to"},
	alert.TypeVonage:         {"from", "to"},
	alert.TypeWebex:          {},
	alert.TypeZapier:         {},
	alert.TypeZulip:          {"bot-email", "channel-id", "domain", "topic"},
}

// isSecretPath returns whether the field at path of a provider of alertType is secret. path uses the notation of
// fieldPaths; the fields of a group override share the classification of the default configuration. The
// provider-override of the default alert can hold any field of the provider, so every value in it is secret. A path
// that is not classified is secret.
func isSecretPath(alertType alert.Type, path string) bool {
	path = strings.TrimPrefix(path, overridesPrefix)
	switch {
	case path == "group":
		return false
	case strings.HasPrefix(path, defaultAlertField+".provider-override"):
		return true
	case strings.HasPrefix(path, defaultAlertField+"."):
		return false
	}
	for _, secret := range classifiedPaths(alertType, true) {
		if matchesPath(secret, path) {
			return true
		}
	}
	for _, public := range classifiedPaths(alertType, false) {
		if matchesPath(public, path) {
			return false
		}
	}
	return true
}

// matchesPath returns whether path matches pattern, in which a "*" segment matches any key of a map. A path below a
// matching pattern matches too, so that every value inside a classified map or list shares its classification.
func matchesPath(pattern, path string) bool {
	patternSegments, pathSegments := strings.Split(pattern, "."), strings.Split(path, ".")
	if len(pathSegments) < len(patternSegments) {
		return false
	}
	for i, segment := range patternSegments {
		if segment != "*" && segment != pathSegments[i] {
			return false
		}
	}
	return true
}

// classifiedPaths returns the secret or public paths of alertType, those of its client section included
func classifiedPaths(alertType alert.Type, secret bool) []string {
	paths, clientPaths := publicPaths[alertType], clientPublicPaths
	if secret {
		paths, clientPaths = secretPaths[alertType], clientSecretPaths
	}
	for _, withClient := range typesWithClient {
		if withClient == alertType {
			return append(append([]string{}, paths...), clientPaths...)
		}
	}
	return paths
}
