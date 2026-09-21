// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package config

import (
	"errors"
	"testing"
)

const adminTestBasicSecurity = `
security:
  basic:
    username: "admin"
    password-bcrypt-base64: "JDJhJDEwJHRiMnRFakxWazZLdXBzRERQazB1TE8vckRLY05Yb1hSdnoxWU0yQ1FaYXZRSW1McmladDYu"
`

const adminTestOIDCSecurity = `
security:
  oidc:
    issuer-url: "https://sso.example.com"
    redirect-url: "https://status.example.com/authorization-code/callback"
    client-id: "go-uptime"
    client-secret: "secret"
    scopes: ["openid"]
`

const adminTestSQLiteStorage = `
storage:
  type: sqlite
  path: /tmp/go-uptime-admin-test.db
`

const adminTestEndpoints = `
endpoints:
  - name: website
    url: https://twin.sh/health
    conditions:
      - "[STATUS] == 200"
`

func TestParseAndValidateConfigBytes_Admin(t *testing.T) {
	scenarios := []struct {
		name        string
		yaml        string
		expectedErr error
	}{
		{
			name:        "admin-disabled-without-endpoints-still-requires-endpoints",
			yaml:        adminTestBasicSecurity + adminTestSQLiteStorage + "\nadmin:\n  enabled: false\n",
			expectedErr: ErrNoEndpointOrSuiteInConfig,
		},
		{
			name: "admin-with-basic-and-sqlite-without-endpoints",
			yaml: adminTestBasicSecurity + adminTestSQLiteStorage + "\nadmin:\n  enabled: true\n",
		},
		{
			name: "admin-with-basic-sqlite-and-endpoints",
			yaml: adminTestBasicSecurity + adminTestSQLiteStorage + adminTestEndpoints + "\nadmin:\n  enabled: true\n",
		},
		{
			name:        "admin-without-security",
			yaml:        adminTestSQLiteStorage + "\nadmin:\n  enabled: true\n",
			expectedErr: ErrAdminRequiresSecurity,
		},
		{
			name:        "admin-with-memory-storage",
			yaml:        adminTestBasicSecurity + "\nadmin:\n  enabled: true\n",
			expectedErr: ErrAdminRequiresPersistentStorage,
		},
		{
			name:        "admin-with-oidc-without-allowed-subjects",
			yaml:        adminTestOIDCSecurity + adminTestSQLiteStorage + "\nadmin:\n  enabled: true\n",
			expectedErr: ErrAdminRequiresAllowedSubjects,
		},
		{
			name: "admin-with-oidc-and-allowed-subjects",
			yaml: adminTestOIDCSecurity + adminTestSQLiteStorage + "\nadmin:\n  enabled: true\n  allowed-subjects: [\"ops@example.com\"]\n",
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			config, err := parseAndValidateConfigBytes([]byte(scenario.yaml))
			if !errors.Is(err, scenario.expectedErr) {
				t.Fatalf("expected error %v, got %v", scenario.expectedErr, err)
			}
			if scenario.expectedErr == nil && !config.Admin.IsEnabled() {
				t.Error("expected the administration to be enabled")
			}
		})
	}
}
