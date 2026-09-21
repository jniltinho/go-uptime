// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package dns holds the dns section of an endpoint of the YAML configuration, which turns the endpoint into a DNS
// query, and validates its query name and query type.
package dns

import (
	"errors"
	"strings"

	"github.com/miekg/dns"
)

var (
	// ErrDNSWithNoQueryName is the error with which Go Uptime will panic if a dns is configured without query name
	ErrDNSWithNoQueryName = errors.New("you must specify a query name in the DNS configuration")

	// ErrDNSWithInvalidQueryType is the error with which Go Uptime will panic if a dns is configured with invalid query type
	ErrDNSWithInvalidQueryType = errors.New("invalid query type in the DNS configuration")
)

// Config for an Endpoint of type DNS
type Config struct {
	// QueryType is the type for the DNS records like A, AAAA, CNAME...
	QueryType string `yaml:"query-type"`

	// QueryName is the query for DNS
	QueryName string `yaml:"query-name"`
}

// ValidateAndSetDefault validates the DNS configuration and appends the trailing dot to QueryName if it is missing.
// It returns ErrDNSWithNoQueryName if QueryName is empty and ErrDNSWithInvalidQueryType if QueryType is not a record
// type known to the DNS library.
func (d *Config) ValidateAndSetDefault() error {
	if len(d.QueryName) == 0 {
		return ErrDNSWithNoQueryName
	}
	if !strings.HasSuffix(d.QueryName, ".") {
		d.QueryName += "."
	}
	if _, ok := dns.StringToType[d.QueryType]; !ok {
		return ErrDNSWithInvalidQueryType
	}
	return nil
}
