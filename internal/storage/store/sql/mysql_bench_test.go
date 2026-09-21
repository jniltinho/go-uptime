package sql

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

// BenchmarkMySQLInsertEndpointResult compares InsertEndpointResult with the arguments interpolated by the driver (the
// default of the store) and with server-side prepared statements, on each available MySQL and MariaDB test server
func BenchmarkMySQLInsertEndpointResult(b *testing.B) {
	servers := mysqlTestServers()
	if len(servers) == 0 {
		b.Skip("GATUS_TEST_MYSQL_URL and GATUS_TEST_MARIADB_URL are not set")
	}
	for _, name := range []string{"mysql", "mariadb"} {
		dsn, exists := servers[name]
		if !exists {
			continue
		}
		for _, interpolate := range []bool{true, false} {
			b.Run(fmt.Sprintf("%s/interpolateParams=%v", name, interpolate), func(b *testing.B) {
				databaseDSN := newMySQLTestDatabase(b, dsn)
				store, err := NewStore(driverMySQL, databaseDSN, false, 100, 50)
				if err != nil {
					b.Fatal(err)
				}
				defer store.Close()
				if !interpolate {
					cfg, err := newMySQLConfig(databaseDSN)
					if err != nil {
						b.Fatal(err)
					}
					cfg.InterpolateParams = false
					connector, err := mysql.NewConnector(cfg)
					if err != nil {
						b.Fatal(err)
					}
					_ = store.db.Close()
					store.db = sql.OpenDB(&mysqlConnector{connector: connector})
				}
				ep := &endpoint.Endpoint{Name: "benchmark", Group: "core"}
				start := time.Now().Add(-time.Hour)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := store.InsertEndpointResult(ep, conformanceResult(start.Add(time.Duration(i)*time.Millisecond), i)); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
