package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/benchristian88/atlas-dns/internal/adguard"
	controllerapi "github.com/benchristian88/atlas-dns/internal/api"
	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/backup"
	"github.com/benchristian88/atlas-dns/internal/config"
	"github.com/benchristian88/atlas-dns/internal/controlplane"
	"github.com/benchristian88/atlas-dns/internal/database"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
	"github.com/benchristian88/atlas-dns/internal/inventory"
	"github.com/benchristian88/atlas-dns/internal/jobs"
	"github.com/benchristian88/atlas-dns/internal/onboarding"
	"github.com/benchristian88/atlas-dns/internal/operationalhealth"
	"github.com/benchristian88/atlas-dns/internal/operations"
	"github.com/benchristian88/atlas-dns/internal/querylog"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
	"github.com/benchristian88/atlas-dns/internal/telemetry"
	"github.com/benchristian88/atlas-dns/internal/updates"
	"github.com/benchristian88/atlas-dns/internal/useradmin"
	"github.com/benchristian88/atlas-dns/internal/version"
)

func main() {
	if err := run(); err != nil {
		slog.Error("controller stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configuration, err := config.Load()
	if err != nil {
		return err
	}
	logger, loggingLevel := configureLogger(configuration)
	slog.SetDefault(logger)
	rootContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	store, err := database.Open(rootContext, configuration.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if configuration.AutoMigrate {
		if err := database.ApplyMigrations(rootContext, store.Pool()); err != nil {
			return err
		}
	}
	tokens, err := auth.NewTokenManager(configuration.SessionSecret)
	if err != nil {
		return err
	}
	credentialCipher, err := auth.NewCredentialCipher(configuration.CredentialEncryptionKey)
	if err != nil {
		return err
	}
	authService, err := auth.NewService(store, tokens, configuration.SessionDuration, credentialCipher)
	if err != nil {
		return err
	}
	userAdministration := useradmin.NewService(store, authService)
	backupService := backup.NewService(configuration.DatabaseURL, configuration.CredentialEncryptionKey, configuration.PGDumpPath, store)
	controllerUpdates := updates.NewService(store, configuration.InstallationType)
	fallbackRuntime := systemsettings.RuntimeSettings{
		SessionDuration: configuration.SessionDuration, NodeHealthInterval: configuration.NodeHealthInterval,
		NodeRequestTimeout: configuration.NodeRequestTimeout, StatisticsPollInterval: configuration.StatisticsPollInterval,
		QueryLogCollection: configuration.QueryLogCollection, QueryLogPollInterval: configuration.QueryLogPollInterval, QueryLogRetention: configuration.QueryLogRetention,
		LogLevel: configuration.LogLevel, OperationalHistoryRetention: 90 * 24 * time.Hour,
	}
	runtimeSettings := systemsettings.NewRuntimeStore(fallbackRuntime)
	systemSettings := systemsettings.NewService(store, fallbackRuntime, configuration.InstallationType, runtimeSettings)
	if err := systemSettings.Initialize(rootContext); err != nil {
		return err
	}
	effectiveRuntime := runtimeSettings.RuntimeSettings()
	loggingLevel.Set(parseLogLevel(effectiveRuntime.LogLevel))
	authService.SetRuntimeSettings(runtimeSettings)
	probe := adguard.NewProbe(configuration.NodeRequestTimeout)
	probe.SetRuntimeSettings(runtimeSettings)
	management := domain.NewManagementService(store, credentialCipher, probe)
	configurationAdapter := adguard.NewConfigurationReader(probe)
	inventoryService := inventory.NewService(store, credentialCipher, configurationAdapter)
	dnsProber := haoperations.NewConfirmingDNSProber(haoperations.NewWireDNSProber(2 * time.Second))
	haOperationsService := haoperations.NewService(store, management, inventoryService, probe, credentialCipher, dnsProber)
	haOperationsService.SetLogger(logger)
	haOperationsService.SetVersionCompatibility(adguard.ConfigurationCompatibility)
	haOperationsService.SetRuntimeSettings(runtimeSettings)
	notificationService := haoperations.NewNotificationService(store, credentialCipher)
	releaseChecker := haoperations.NewReleaseChecker(store)
	operationService := operations.NewService(store, credentialCipher)
	operationExecutor := operations.NewExecutor(store, credentialCipher, credentialCipher, configurationAdapter, inventoryService)
	if err := operationExecutor.RecoverInterrupted(rootContext); err != nil {
		return err
	}
	controlplaneService := controlplane.NewService(store, inventoryService)
	deploymentExecutor := controlplane.NewExecutor(store, credentialCipher, configurationAdapter, inventoryService)
	if err := deploymentExecutor.RecoverInterrupted(rootContext); err != nil {
		return err
	}
	reconciler := controlplane.NewReconciler(store, controlplaneService, inventoryService, logger)
	workerHealth := operationalhealth.NewTracker()
	for _, worker := range []string{"node_connectivity", "dns_service_health", "adguard_release_check", "notification_delivery", "statistics_collection", "statistics_retention", "query_log_collection", "query_log_retention", "operational_history_retention", "deployment", "operational_commands", "drift_reconciliation", "session_cleanup"} {
		workerHealth.Register(worker, worker == "query_log_collection" && !effectiveRuntime.QueryLogCollection)
	}
	healthPoller := jobs.NewHealthPoller(store, credentialCipher, probe, effectiveRuntime.NodeHealthInterval, logger, workerHealth)
	healthPoller.SetRuntimeSettings(runtimeSettings)
	statisticsService := telemetry.NewService(store, effectiveRuntime.StatisticsPollInterval, configuration.NodeRequestTimeout)
	statisticsService.SetRuntimeSettings(runtimeSettings)
	statisticsPoller := jobs.NewStatisticsPoller(store, credentialCipher, configurationAdapter, effectiveRuntime.StatisticsPollInterval, configuration.NodeRequestTimeout, logger, workerHealth)
	statisticsPoller.SetRuntimeSettings(runtimeSettings)
	queryLogService := querylog.NewService(store, effectiveRuntime.QueryLogPollInterval, querylog.Options{
		CollectionEnabled: effectiveRuntime.QueryLogCollection,
		Retention:         effectiveRuntime.QueryLogRetention,
	})
	queryLogService.SetRuntimeSettings(runtimeSettings)
	operationalService := operationalhealth.NewService(store, workerHealth, operationalhealth.Options{
		NodeInterval: effectiveRuntime.NodeHealthInterval, RequestTimeout: configuration.NodeRequestTimeout,
		StatisticsInterval: effectiveRuntime.StatisticsPollInterval, QueryLogInterval: effectiveRuntime.QueryLogPollInterval,
		StatisticsRetention: 400 * 24 * time.Hour, QueryLogRetention: effectiveRuntime.QueryLogRetention,
		QueryLogEnabled: effectiveRuntime.QueryLogCollection,
	})
	operationalService.SetHAOperations(haOperationsService)
	operationalService.SetRuntimeSettings(runtimeSettings)
	go healthPoller.Run(rootContext)
	go jobs.RunHAOperations(rootContext, haOperationsService, effectiveRuntime.NodeHealthInterval, logger, workerHealth, runtimeSettings)
	go jobs.RunReleaseChecks(rootContext, releaseChecker, logger, workerHealth)
	go jobs.RunNotificationDelivery(rootContext, notificationService, logger, workerHealth)
	go statisticsPoller.Run(rootContext)
	queryLogPoller := jobs.NewQueryLogPoller(store, credentialCipher, configurationAdapter, effectiveRuntime.QueryLogPollInterval, configuration.NodeRequestTimeout, effectiveRuntime.QueryLogRetention, logger, workerHealth)
	queryLogPoller.SetRuntimeSettings(runtimeSettings)
	go queryLogPoller.Run(rootContext)
	go jobs.RunDeploymentExecutor(rootContext, deploymentExecutor, logger, workerHealth)
	go jobs.RunOperationalCommandExecutor(rootContext, operationExecutor, logger, workerHealth)
	go jobs.RunReconciler(rootContext, reconciler, effectiveRuntime.NodeHealthInterval, logger, workerHealth, runtimeSettings)
	go jobs.RunSessionCleanup(rootContext, store, logger, workerHealth)
	go jobs.RunOperationalHistoryRetention(rootContext, store, runtimeSettings, logger, workerHealth)
	go watchLogLevel(rootContext, runtimeSettings, loggingLevel)

	apiServer := controllerapi.NewServer(
		authService, management, inventoryService, store, store, logger,
		configuration.SecureCookies(), configuration.PublicBaseURL.String(),
		configuration.NodeHealthInterval, configuration.WebDistDirectory,
		controlplaneService,
	)
	apiServer.SetDNSOperations(operationService)
	apiServer.SetStatistics(statisticsService)
	apiServer.SetQueryLog(queryLogService)
	apiServer.SetOperationalHealth(operationalService)
	apiServer.SetHAOperations(haOperationsService)
	apiServer.SetNotificationSettings(notificationService)
	apiServer.SetUserAdministration(userAdministration)
	apiServer.SetBackups(backupService)
	apiServer.SetControllerUpdates(controllerUpdates)
	apiServer.SetSystemSettings(systemSettings)
	onboardingService := onboarding.NewService(store, systemSettings, configuration.PublicBaseURL.String())
	onboardingService.SetInitialCollector(jobs.NewOnboardingCollector(healthPoller, statisticsPoller, queryLogPoller, haOperationsService), logger)
	apiServer.SetOnboarding(onboardingService)
	apiServer.SetRuntimeSettings(runtimeSettings)
	apiServer.SetMetrics(workerHealth, configuration.MetricsToken)
	httpServer := &http.Server{
		Addr:              configuration.HTTPAddress,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverError := make(chan error, 1)
	go func() {
		logger.Info("controller listening", "address", configuration.HTTPAddress, "version", version.Current().Version)
		serverError <- httpServer.ListenAndServe()
	}()
	select {
	case <-rootContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownContext); err != nil {
			return err
		}
		logger.Info("controller shutdown complete")
		return nil
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func configureLogger(configuration config.Config) (*slog.Logger, *slog.LevelVar) {
	level := &slog.LevelVar{}
	level.Set(parseLogLevel(configuration.LogLevel))
	options := &slog.HandlerOptions{Level: level}
	if configuration.Environment == "development" {
		return slog.New(slog.NewTextHandler(os.Stdout, options)), level
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, options)), level
}

func parseLogLevel(value string) slog.Level {
	switch value {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func watchLogLevel(ctx context.Context, settings *systemsettings.RuntimeStore, level *slog.LevelVar) {
	for {
		changed := settings.RuntimeSettingsChanged()
		select {
		case <-ctx.Done():
			return
		case <-changed:
			level.Set(parseLogLevel(settings.RuntimeSettings().LogLevel))
		}
	}
}
