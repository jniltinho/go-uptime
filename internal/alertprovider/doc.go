// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package alertprovider keeps the alerting providers managed through the administration (fork): one per alert type,
// stored in the managed_alert_providers table, validated like the alerting section of the configuration file and
// without its environment variable expansion, with their secrets masked in every answer.
//
// The configuration file takes precedence: a type it declares is read-only, and a managed provider of that type is
// shadowed. The endpoints of the file only use the providers of the file; the endpoints managed through the
// administration use the effective configuration, the file's providers plus the valid managed ones (see Effective).
package alertprovider
