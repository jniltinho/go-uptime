// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIsSupportedMySQLVersion(t *testing.T) {
	scenarios := []struct {
		version           string
		expectedSupported bool
		expectedKnown     bool
	}{
		{version: "8.4.11", expectedSupported: true, expectedKnown: true},
		{version: "9.7.2", expectedSupported: true, expectedKnown: true},
		{version: "8.0.46", expectedSupported: false, expectedKnown: true},
		{version: "5.7.44-log", expectedSupported: false, expectedKnown: true},
		{version: "10.11.19-MariaDB-ubu2204", expectedSupported: true, expectedKnown: true},
		{version: "12.3.3-MariaDB", expectedSupported: true, expectedKnown: true},
		{version: "10.6.22-MariaDB-log", expectedSupported: false, expectedKnown: true},
		{version: "unknown", expectedSupported: false, expectedKnown: false},
		{version: "8.x", expectedSupported: false, expectedKnown: false},
	}
	for _, scenario := range scenarios {
		supported, known := isSupportedMySQLVersion(scenario.version)
		if supported != scenario.expectedSupported || known != scenario.expectedKnown {
			t.Errorf("%s: expected supported=%v known=%v, got supported=%v known=%v", scenario.version, scenario.expectedSupported, scenario.expectedKnown, supported, known)
		}
	}
}

func TestNewMySQLConfig(t *testing.T) {
	cfg, err := newMySQLConfig("go_uptime:secret@tcp(mariadb:3306)/go_uptime?loc=America%2FSao_Paulo&parseTime=false&clientFoundRows=false&interpolateParams=false&charset=latin1&time_zone=%27-03%3A00%27&SQL_MODE=%27%27&readTimeout=5s&tls=preferred")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ParseTime || cfg.Loc != time.UTC || !cfg.ClientFoundRows || !cfg.InterpolateParams || !cfg.CheckConnLiveness {
		t.Errorf("expected the connection flags to be overridden, got %+v", cfg)
	}
	if cfg.Collation != "utf8mb4_bin" {
		t.Errorf("expected the utf8mb4_bin collation, got %q", cfg.Collation)
	}
	if cfg.ReadTimeout != 5*time.Second || cfg.TLSConfig != "preferred" || cfg.DBName != "go_uptime" || cfg.Addr != "mariadb:3306" {
		t.Errorf("expected the other parameters of the DSN to be kept, got %+v", cfg)
	}
	expectedParams := map[string]string{"time_zone": "'+00:00'", "sql_mode": "'" + mysqlSQLMode + "'", "innodb_lock_wait_timeout": "10"}
	if len(cfg.Params) != len(expectedParams) {
		t.Errorf("expected only the fixed session variables, got %v", cfg.Params)
	}
	for name, value := range expectedParams {
		if cfg.Params[name] != value {
			t.Errorf("expected %s=%s, got %q", name, value, cfg.Params[name])
		}
	}
	if !strings.Contains(cfg.FormatDSN(), "charset=utf8mb4") {
		t.Errorf("expected the utf8mb4 charset, got %s", cfg.FormatDSN())
	}
	if _, err := newMySQLConfig("go_uptime:secret@tcp(mariadb:3306"); !errors.Is(err, ErrInvalidMySQLPath) || strings.Contains(err.Error(), "secret") {
		t.Errorf("expected ErrInvalidMySQLPath without the password, got %v", err)
	}
}
