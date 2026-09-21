// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/suite"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
)

// The conformance tests run the same scenarios on every available database, SQLite always, PostgreSQL with
// GO_UPTIME_TEST_POSTGRES_URL and MySQL and MariaDB with GO_UPTIME_TEST_MYSQL_URL and GO_UPTIME_TEST_MARIADB_URL (fork), and
// compare what the store returns with what it returns with SQLite.

type conformanceStore struct {
	name  string
	store *Store
}

func newConformanceStores(t *testing.T, maximumNumberOfResults, maximumNumberOfEvents int) []conformanceStore {
	t.Helper()
	sqliteStore, err := NewStore("sqlite", t.TempDir()+"/conformance.db", false, maximumNumberOfResults, maximumNumberOfEvents)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sqliteStore.Close)
	stores := []conformanceStore{{name: "sqlite", store: sqliteStore}}
	if url := os.Getenv("GO_UPTIME_TEST_POSTGRES_URL"); len(url) > 0 {
		postgresStore, err := NewStore("postgres", url, false, maximumNumberOfResults, maximumNumberOfEvents)
		if err != nil {
			t.Fatalf("failed to create the postgres store: %v", err)
		}
		clear := func() {
			postgresStore.Clear()
			postgresStore.DeleteAllSuiteStatusesNotInKeys(nil)
		}
		clear()
		t.Cleanup(func() {
			clear()
			postgresStore.Close()
		})
		stores = append(stores, conformanceStore{name: "postgres", store: postgresStore})
	}
	for _, name := range []string{"mysql", "mariadb"} {
		dsn, exists := mysqlTestServers()[name]
		if !exists {
			continue
		}
		mysqlStore, err := NewStore(driverMySQL, newMySQLTestDatabase(t, dsn), false, maximumNumberOfResults, maximumNumberOfEvents)
		if err != nil {
			t.Fatalf("failed to create the %s store: %v", name, err)
		}
		t.Cleanup(mysqlStore.Close)
		stores = append(stores, conformanceStore{name: name, store: mysqlStore})
	}
	return stores
}

// compareWithSQLite fails the test when the fingerprint of a store differs from the fingerprint of SQLite
func compareWithSQLite(t *testing.T, stores []conformanceStore, fingerprint func(t *testing.T, store *Store) string) {
	t.Helper()
	var expected string
	for _, conformance := range stores {
		actual := fingerprint(t, conformance.store)
		if conformance.name == "sqlite" {
			expected = actual
			continue
		}
		if actual != expected {
			expectedLines, actualLines := strings.Split(expected, "\n"), strings.Split(actual, "\n")
			for i := 0; i < max(len(expectedLines), len(actualLines)); i++ {
				var expectedLine, actualLine string
				if i < len(expectedLines) {
					expectedLine = expectedLines[i]
				}
				if i < len(actualLines) {
					actualLine = actualLines[i]
				}
				if expectedLine != actualLine {
					t.Errorf("%s differs from sqlite at line %d:\n  sqlite: %s\n  %s: %s", conformance.name, i+1, expectedLine, conformance.name, actualLine)
					break
				}
			}
		}
	}
}

func conformanceResult(timestamp time.Time, index int) *endpoint.Result {
	success := index%4 != 0
	result := &endpoint.Result{
		Success:          success,
		Timestamp:        timestamp,
		Duration:         time.Duration(10+index) * time.Millisecond,
		HTTPStatus:       200,
		Hostname:         "example.org",
		Connected:        true,
		ConditionResults: []*endpoint.ConditionResult{{Condition: "[STATUS] == 200", Success: success}},
	}
	if !success {
		result.HTTPStatus = 500
		result.Errors = []string{fmt.Sprintf("error %d", index), "second error"}
	}
	return result
}

func endpointFingerprint(t *testing.T, store *Store, key string) string {
	t.Helper()
	status, err := store.GetEndpointStatusByKey(key, paging.NewEndpointStatusParams().WithResults(1, 1000).WithEvents(1, 1000))
	if err != nil {
		t.Fatalf("failed to get the status of %s: %v", key, err)
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("status name=%s group=%s key=%s results=%d events=%d", status.Name, status.Group, status.Key, len(status.Results), len(status.Events)))
	for _, result := range status.Results {
		var conditions []string
		for _, condition := range result.ConditionResults {
			conditions = append(conditions, fmt.Sprintf("%s:%v", condition.Condition, condition.Success))
		}
		lines = append(lines, fmt.Sprintf("result %s success=%v status=%d duration=%s hostname=%s errors=%q conditions=%v", result.Timestamp.UTC().Format(time.RFC3339Nano), result.Success, result.HTTPStatus, result.Duration, result.Hostname, result.Errors, conditions))
	}
	for _, event := range status.Events {
		lines = append(lines, fmt.Sprintf("event %s %s", event.Type, event.Timestamp.UTC().Format(time.RFC3339Nano)))
	}
	return strings.Join(lines, "\n")
}

func TestConformance_ResultsEventsAndLimits(t *testing.T) {
	stores := newConformanceStores(t, 5, 3)
	ep := &endpoint.Endpoint{Name: "api", Group: "core"}
	start := time.Now().Add(-3 * time.Hour).Truncate(time.Second).UTC()
	for _, conformance := range stores {
		for i := 0; i < 40; i++ {
			if err := conformance.store.InsertEndpointResult(ep, conformanceResult(start.Add(time.Duration(i)*time.Minute), i)); err != nil {
				t.Fatalf("%s: failed to insert result %d: %v", conformance.name, i, err)
			}
		}
	}
	compareWithSQLite(t, stores, func(t *testing.T, store *Store) string {
		fingerprint := endpointFingerprint(t, store, ep.Key())
		status, _ := store.GetEndpointStatusByKey(ep.Key(), paging.NewEndpointStatusParams().WithResults(1, 1000))
		if last := status.Results[len(status.Results)-1]; !last.Timestamp.Equal(start.Add(39 * time.Minute)) {
			t.Errorf("expected the most recent result to be kept, got %v", last.Timestamp)
		}
		if len(status.Results) > 5+resultsAboveMaximumCleanUpThreshold {
			t.Errorf("expected the old results to be deleted, got %d", len(status.Results))
		}
		newer, err := store.HasEndpointStatusNewerThan(ep.Key(), start.Add(38*time.Minute))
		if err != nil || !newer {
			t.Errorf("expected a status newer than the 38th minute, got %v (err=%v)", newer, err)
		}
		return fingerprint
	})
}

func TestConformance_Uptime(t *testing.T) {
	stores := newConformanceStores(t, 100, 50)
	ep := &endpoint.Endpoint{Name: "web", Group: "core"}
	now := time.Now().Truncate(time.Hour)
	for _, conformance := range stores {
		// More than 100 hourly entries, so that the hourly entries older than 48 hours are merged into daily entries
		for i := 130; i >= 0; i-- {
			result := &endpoint.Result{Success: i%5 != 0, Timestamp: now.Add(-time.Duration(i)*time.Hour + 5*time.Minute), Duration: time.Duration(20+i) * time.Millisecond}
			if err := conformance.store.InsertEndpointResult(ep, result); err != nil {
				t.Fatalf("%s: failed to insert result %d: %v", conformance.name, i, err)
			}
		}
	}
	compareWithSQLite(t, stores, func(t *testing.T, store *Store) string {
		var lines []string
		for _, period := range []time.Duration{time.Hour, 24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour} {
			uptime, err := store.GetUptimeByKey(ep.Key(), now.Add(-period), now.Add(time.Hour))
			averageResponseTime, averageErr := store.GetAverageResponseTimeByKey(ep.Key(), now.Add(-period), now.Add(time.Hour))
			lines = append(lines, fmt.Sprintf("period=%s uptime=%v err=%v average=%d err=%v", period, uptime, err, averageResponseTime, averageErr))
		}
		hourly, err := store.GetHourlyAverageResponseTimeByKey(ep.Key(), now.Add(-7*24*time.Hour), now.Add(time.Hour))
		lines = append(lines, fmt.Sprintf("hourly=%v err=%v", hourly, err))
		var numberOfEntries int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM endpoint_uptimes").Scan(&numberOfEntries); err != nil {
			t.Fatal(err)
		}
		if numberOfEntries >= 100 {
			t.Errorf("expected the hourly entries to be merged into daily entries, got %d entries", numberOfEntries)
		}
		lines = append(lines, fmt.Sprintf("entries=%d", numberOfEntries))
		return strings.Join(lines, "\n")
	})
}

func TestConformance_TriggeredAlerts(t *testing.T) {
	enabled, description := true, "description"
	firstAlert := &alert.Alert{Type: alert.TypeCustom, Enabled: &enabled, FailureThreshold: 3, SuccessThreshold: 2, Description: &description, Triggered: true, ResolveKey: "first"}
	otherDescription := "other"
	secondAlert := &alert.Alert{Type: alert.TypeCustom, Enabled: &enabled, FailureThreshold: 1, SuccessThreshold: 1, Description: &otherDescription, Triggered: true, ResolveKey: "second"}
	for _, conformance := range newConformanceStores(t, 100, 50) {
		t.Run(conformance.name, func(t *testing.T) {
			store := conformance.store
			ep := &endpoint.Endpoint{Name: "alerting", Group: "core", NumberOfSuccessesInARow: 0}
			for _, triggeredAlert := range []*alert.Alert{firstAlert, secondAlert} {
				if err := store.UpsertTriggeredEndpointAlert(ep, triggeredAlert); err != nil {
					t.Fatalf("failed to upsert: %v", err)
				}
			}
			ep.NumberOfSuccessesInARow = 1
			firstAlert.ResolveKey = "first-updated"
			if err := store.UpsertTriggeredEndpointAlert(ep, firstAlert); err != nil {
				t.Fatalf("failed to upsert again: %v", err)
			}
			exists, resolveKey, numberOfSuccessesInARow, err := store.GetTriggeredEndpointAlert(ep, firstAlert)
			if err != nil || !exists || resolveKey != "first-updated" || numberOfSuccessesInARow != 1 {
				t.Errorf("expected the updated triggered alert, got exists=%v resolveKey=%s successes=%d err=%v", exists, resolveKey, numberOfSuccessesInARow, err)
			}
			var rows int
			if err := store.db.QueryRow("SELECT COUNT(*) FROM endpoint_alerts_triggered").Scan(&rows); err != nil || rows != 2 {
				t.Errorf("expected 2 triggered alerts after the upserts, got %d (err=%v)", rows, err)
			}
			if deleted := store.DeleteAllTriggeredAlertsNotInChecksumsByEndpoint(ep, []string{firstAlert.Checksum()}); deleted != 1 {
				t.Errorf("expected 1 triggered alert to be deleted, got %d", deleted)
			}
			if err := store.DeleteTriggeredEndpointAlert(ep, firstAlert); err != nil {
				t.Fatal(err)
			}
			if exists, _, _, _ := store.GetTriggeredEndpointAlert(ep, firstAlert); exists {
				t.Error("expected the triggered alert to be deleted")
			}
			firstAlert.ResolveKey = "first"
		})
	}
}

func TestConformance_SuitesRemovalAndRestart(t *testing.T) {
	stores := newConformanceStores(t, 5, 50)
	su := &suite.Suite{Name: "checkout", Group: "flows"}
	ep := &endpoint.Endpoint{Name: "api", Group: "core"}
	start := time.Now().Add(-time.Hour).Truncate(time.Second).UTC()
	for _, conformance := range stores {
		for i := 0; i < 20; i++ {
			timestamp := start.Add(time.Duration(i) * time.Minute)
			result := &suite.Result{
				Name:      su.Name,
				Group:     su.Group,
				Success:   i%3 != 0,
				Timestamp: timestamp,
				Duration:  time.Duration(100+i) * time.Millisecond,
				EndpointResults: []*endpoint.Result{
					{Name: "login", Success: true, Timestamp: timestamp, Duration: 10 * time.Millisecond},
					{Name: "pay", Success: i%3 != 0, Timestamp: timestamp, Duration: 20 * time.Millisecond},
				},
			}
			if i%3 == 0 {
				result.Errors = []string{"pay failed"}
			}
			if err := conformance.store.InsertSuiteResult(su, result); err != nil {
				t.Fatalf("%s: failed to insert suite result %d: %v", conformance.name, i, err)
			}
		}
		if err := conformance.store.InsertEndpointResult(ep, conformanceResult(start, 1)); err != nil {
			t.Fatalf("%s: %v", conformance.name, err)
		}
	}
	compareWithSQLite(t, stores, func(t *testing.T, store *Store) string {
		status, err := store.GetSuiteStatusByKey(su.Key(), paging.NewSuiteStatusParams().WithPagination(1, 100))
		if err != nil {
			t.Fatalf("failed to get the suite status: %v", err)
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("suite name=%s group=%s key=%s results=%d", status.Name, status.Group, status.Key, len(status.Results)))
		for _, result := range status.Results {
			var endpointResults []string
			for _, endpointResult := range result.EndpointResults {
				endpointResults = append(endpointResults, fmt.Sprintf("%s:%v:%s", endpointResult.Name, endpointResult.Success, endpointResult.Duration))
			}
			lines = append(lines, fmt.Sprintf("result %s success=%v duration=%s errors=%q endpoints=%v", result.Timestamp.UTC().Format(time.RFC3339), result.Success, result.Duration, result.Errors, endpointResults))
		}
		// Restarting on the existing schema keeps the data
		restarted, err := NewStore(store.driver, store.path, false, store.maximumNumberOfResults, store.maximumNumberOfEvents)
		if err != nil {
			t.Fatalf("failed to restart the store: %v", err)
		}
		defer restarted.Close()
		if restartedStatus, err := restarted.GetSuiteStatusByKey(su.Key(), paging.NewSuiteStatusParams().WithPagination(1, 100)); err != nil || len(restartedStatus.Results) != len(status.Results) {
			t.Errorf("expected the restarted store to keep the suite results, got %v", err)
		}
		// Removals: the endpoint outside of the keys, then the suite, in cascade
		lines = append(lines, fmt.Sprintf("deleted endpoints=%d", store.DeleteAllEndpointStatusesNotInKeys([]string{"core_other"})))
		if _, err := store.GetEndpointStatusByKey(ep.Key(), paging.NewEndpointStatusParams()); err == nil {
			t.Error("expected the endpoint to be deleted")
		}
		lines = append(lines, fmt.Sprintf("deleted suites=%d", store.DeleteAllSuiteStatusesNotInKeys([]string{"flows_other"})))
		var orphanResults int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM endpoint_results WHERE suite_result_id IS NOT NULL").Scan(&orphanResults); err != nil || orphanResults != 0 {
			t.Errorf("expected the endpoint results of the suite to be deleted in cascade, got %d (err=%v)", orphanResults, err)
		}
		store.Clear()
		var remaining int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM endpoint_results").Scan(&remaining); err != nil || remaining != 0 {
			t.Errorf("expected Clear to delete every result, got %d (err=%v)", remaining, err)
		}
		return strings.Join(lines, "\n")
	})
}

func TestConformance_ConcurrentInserts(t *testing.T) {
	const numberOfEndpoints, resultsPerEndpoint = 8, 25
	for _, conformance := range newConformanceStores(t, 5, 3) {
		t.Run(conformance.name, func(t *testing.T) {
			start := time.Now().Add(-time.Hour).Truncate(time.Second)
			var wg sync.WaitGroup
			errs := make(chan error, numberOfEndpoints*resultsPerEndpoint)
			for e := 0; e < numberOfEndpoints; e++ {
				wg.Add(1)
				go func(e int) {
					defer wg.Done()
					ep := &endpoint.Endpoint{Name: fmt.Sprintf("endpoint-%d", e), Group: "concurrency"}
					for i := 0; i < resultsPerEndpoint; i++ {
						if err := conformance.store.InsertEndpointResult(ep, conformanceResult(start.Add(time.Duration(i)*time.Second), i)); err != nil {
							errs <- fmt.Errorf("%s result %d: %w", ep.Key(), i, err)
						}
					}
				}(e)
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}
			for e := 0; e < numberOfEndpoints; e++ {
				key := fmt.Sprintf("concurrency_endpoint-%d", e)
				var totalExecutions int
				if err := conformance.store.db.QueryRow("SELECT SUM(total_executions) FROM endpoint_uptimes WHERE endpoint_id = (SELECT endpoint_id FROM endpoints WHERE endpoint_key = $1)", key).Scan(&totalExecutions); err != nil || totalExecutions != resultsPerEndpoint {
					t.Errorf("%s: expected %d executions in the uptime, got %d (err=%v)", key, resultsPerEndpoint, totalExecutions, err)
				}
				status, err := conformance.store.GetEndpointStatusByKey(key, paging.NewEndpointStatusParams().WithResults(1, 100))
				if err != nil || len(status.Results) == 0 || len(status.Results) > 5+resultsAboveMaximumCleanUpThreshold {
					t.Errorf("%s: expected between 1 and %d results, got %v (err=%v)", key, 5+resultsAboveMaximumCleanUpThreshold, status, err)
				}
			}
		})
	}
}

func TestMySQLConnector_Deadlock(t *testing.T) {
	forEachMySQLTestServer(t, func(t *testing.T, dsn string) {
		store, err := NewStore(driverMySQL, dsn, false, 100, 50)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		for _, key := range []string{"core_a", "core_b"} {
			if _, err := store.db.Exec("INSERT INTO endpoints (endpoint_key, endpoint_name, endpoint_group) VALUES ($1, $2, $3)", key, key, "core"); err != nil {
				t.Fatal(err)
			}
		}
		const update = "UPDATE endpoints SET endpoint_name = $1 WHERE endpoint_key = $2"
		first, err := store.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		second, err := store.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := first.Exec(update, "first", "core_a"); err != nil {
			t.Fatal(err)
		}
		if _, err := second.Exec(update, "second", "core_b"); err != nil {
			t.Fatal(err)
		}
		firstErr := make(chan error, 1)
		go func() {
			_, err := first.Exec(update, "first", "core_b")
			firstErr <- err
		}()
		time.Sleep(300 * time.Millisecond)
		_, secondErr := second.Exec(update, "second", "core_a")
		firstExecErr := <-firstErr
		victim, winner := second, first
		victimErr := secondErr
		if firstExecErr != nil {
			victim, winner, victimErr = first, second, firstExecErr
		}
		if !isTransientMySQLError(victimErr) {
			t.Fatalf("expected one of the transactions to fail with a deadlock, got first=%v second=%v", firstExecErr, secondErr)
		}
		if err := victim.Commit(); !errors.Is(err, errTransactionAborted) || !isTransientMySQLError(err) {
			t.Errorf("expected the commit of the victim to return the deadlock, got %v", err)
		}
		if err := winner.Commit(); err != nil {
			t.Errorf("expected the other transaction to commit, got %v", err)
		}
		var names []string
		rows, err := store.db.Query("SELECT endpoint_name FROM endpoints ORDER BY endpoint_key")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var name string
			_ = rows.Scan(&name)
			names = append(names, name)
		}
		_ = rows.Close()
		if len(names) != 2 || names[0] != names[1] || (names[0] != "first" && names[0] != "second") {
			t.Errorf("expected both rows to be updated by the winner only, got %v", names)
		}
	})
}
