// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package cmd

import (
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/controller"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/liveupdates"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/metrics"
	"github.com/jniltinho/go-uptime/v7/internal/pushkey"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"
	"github.com/spf13/cobra"
)

// serveConfigPath is the path of the configuration resolved when the server started, from the flag or from the
// environment. Every reload uses it again: resolving it only once would make a reload go back to the environment and
// load another file than the one the server was started with.
var serveConfigPath string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the server (the default when no command is given)",
	Args:  cobra.ExactArgs(0),
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

// runServe starts Go Uptime and blocks until a termination signal
func runServe(cmd *cobra.Command, _ []string) error {
	if delayInSeconds, _ := strconv.Atoi(lookupEnvironment(DelayStartEnvVar, LegacyDelayStartEnvVar)); delayInSeconds > 0 {
		logr.Infof("Delaying start by %d seconds", delayInSeconds)
		time.Sleep(time.Duration(delayInSeconds) * time.Second)
	}
	configureLogging(resolveLogLevel(cmd))
	configPath, err := resolveConfigPath(cmd)
	if err != nil {
		return err
	}
	serveConfigPath = configPath
	cfg, err := loadConfiguration()
	if err != nil {
		return err
	}
	lifecycle.BeginCycle() // ended by start
	initializeStorage(cfg)
	start(cfg)
	// Wait for termination signal
	signalChannel := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		logr.Info("Received termination signal, attempting to gracefully shut down")
		lifecycle.BeginCycle() // never ended: the application is shutting down
		stop(cfg)
		save()
		done <- true
	}()
	<-done
	logr.Info("Shutting down")
	return nil
}

// loadConfiguration loads the configuration the server was started with, on the start and on every reload
func loadConfiguration() (*config.Config, error) {
	return config.LoadConfiguration(serveConfigPath)
}

// start must be called with a lifecycle cycle in progress, which it ends once monitoring has started
func start(cfg *config.Config) {
	// Fork: the new server accepts the event streams as soon as it listens
	liveupdates.Open()
	go controller.Handle(cfg)
	metrics.InitializePrometheusMetrics(cfg, nil)
	watchdog.Monitor(cfg)
	managedendpoint.StartMonitoring()
	lifecycle.EndCycle()
	go listenToConfigurationFileChanges(cfg)
}

func stop(cfg *config.Config) {
	watchdog.Shutdown(cfg)
	// Fork: the event streams end before the server stops, which would otherwise wait for them
	liveupdates.Close()
	controller.Shutdown()
	metrics.UnregisterPrometheusMetrics()
	closeTunnels(cfg)
}

func save() {
	if err := store.Get().Save(); err != nil {
		logr.Errorf("Failed to save storage provider: %s", err.Error())
	}
}

// initializeStorage initializes the storage provider
//
// Q: "TwiN, why are you putting this here? Wouldn't it make more sense to have this in the config?!"
// A: Yes. Yes it would make more sense to have it in the config package. But I don't want to import
// the massive SQL dependencies just because I want to import the config, so here we are.
func initializeStorage(cfg *config.Config) {
	err := store.Initialize(cfg.Storage)
	if err != nil {
		panic(err)
	}
	// Remove all SuiteStatuses that represent suites which no longer exist in the configuration
	var suiteKeys []string
	for _, suite := range cfg.Suites {
		suiteKeys = append(suiteKeys, suite.Key())
	}
	numberOfSuiteStatusesDeleted := store.Get().DeleteAllSuiteStatusesNotInKeys(suiteKeys)
	if numberOfSuiteStatusesDeleted > 0 {
		logr.Infof("[cmd.initializeStorage] Deleted %d suite statuses because their matching suites no longer existed", numberOfSuiteStatusesDeleted)
	}
	// Remove all EndpointStatus that represent endpoints which no longer exist in the configuration
	var keys []string
	for _, ep := range cfg.Endpoints {
		keys = append(keys, ep.Key())
	}
	for _, ee := range cfg.ExternalEndpoints {
		keys = append(keys, ee.Key())
	}
	// Also add endpoints that are part of suites
	for _, suite := range cfg.Suites {
		for _, ep := range suite.Endpoints {
			keys = append(keys, ep.Key())
		}
	}
	// Endpoints managed through the administration API, valid or not, so that their history is preserved
	managedKeys, loadErr := managedendpoint.Load(cfg)
	keys = append(keys, managedKeys...)
	logr.Infof("[cmd.initializeStorage] Total endpoint keys to preserve: %d", len(keys))
	if loadErr != nil {
		logr.Errorf("[cmd.initializeStorage] Failed to load managed endpoints, so endpoint statuses are not cleaned up to preserve their history: %s", loadErr.Error())
	} else {
		if numberOfEndpointStatusesDeleted := store.Get().DeleteAllEndpointStatusesNotInKeys(keys); numberOfEndpointStatusesDeleted > 0 {
			logr.Infof("[cmd.initializeStorage] Deleted %d endpoint statuses because their matching endpoints no longer existed", numberOfEndpointStatusesDeleted)
		}
		// Fork: the push state (last push and retries used) of the keys that no longer exist is forgotten
		watchdog.ForgetExternalEndpointsNotIn(keys)
		liveupdates.ForgetExcept(keys)
	}
	// Public status pages, after the managed endpoints that they can select
	statuspage.Load(cfg)
	// Global push keys of the configuration file and of the administration (fork)
	pushkey.Load(cfg)
	// Clean up the triggered alerts from the storage provider and load valid triggered endpoint alerts
	numberOfPersistedTriggeredAlertsLoaded := 0
	for _, ep := range cfg.Endpoints {
		numberOfPersistedTriggeredAlertsLoaded += watchdog.RestorePersistedTriggeredAlerts(ep)
	}
	for _, ee := range cfg.ExternalEndpoints {
		convertedEndpoint := ee.ToEndpoint()
		if restored := watchdog.RestorePersistedTriggeredAlerts(convertedEndpoint); restored > 0 {
			ee.NumberOfSuccessesInARow, ee.NumberOfFailuresInARow = convertedEndpoint.NumberOfSuccessesInARow, convertedEndpoint.NumberOfFailuresInARow
			numberOfPersistedTriggeredAlertsLoaded += restored
		}
	}
	// Load persisted triggered alerts for suite endpoints
	for _, suite := range cfg.Suites {
		for _, ep := range suite.Endpoints {
			numberOfPersistedTriggeredAlertsLoaded += watchdog.RestorePersistedTriggeredAlerts(ep)
		}
	}
	if numberOfPersistedTriggeredAlertsLoaded > 0 {
		logr.Infof("[cmd.initializeStorage] Loaded %d persisted triggered alerts", numberOfPersistedTriggeredAlertsLoaded)
	}
}

func closeTunnels(cfg *config.Config) {
	if cfg.Tunneling != nil {
		if err := cfg.Tunneling.Close(); err != nil {
			logr.Errorf("[cmd.closeTunnels] Error closing SSH tunnels: %v", err)
		}
	}
}

func listenToConfigurationFileChanges(cfg *config.Config) {
	for {
		time.Sleep(30 * time.Second)
		if cfg.HasLoadedConfigurationBeenModified() {
			logr.Info("[cmd.listenToConfigurationFileChanges] Configuration file has been modified")
			// The new configuration is validated before anything is stopped (see main_reload.go)
			updatedConfig, ok := loadUpdatedConfiguration(cfg, loadConfiguration)
			if !ok {
				continue
			}
			lifecycle.BeginCycle() // ended by start
			stop(cfg)
			time.Sleep(time.Second) // Wait a bit to make sure everything is done.
			save()
			store.Get().Close()
			initializeStorage(updatedConfig)
			start(updatedConfig)
			return
		}
	}
}
