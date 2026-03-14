package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/admin"
	"github.com/HMasataka/cloudia/internal/auth"
	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/gateway"
	"github.com/HMasataka/cloudia/internal/logging"
	"github.com/HMasataka/cloudia/internal/protocol"
	awsprotocol "github.com/HMasataka/cloudia/internal/protocol/aws"
	gcpprotocol "github.com/HMasataka/cloudia/internal/protocol/gcp"
	"github.com/HMasataka/cloudia/internal/resource"
	"github.com/HMasataka/cloudia/internal/service"
	cloudsqlsvc "github.com/HMasataka/cloudia/internal/service/cloudsql"
	dynamodbsvc "github.com/HMasataka/cloudia/internal/service/dynamodb"
	ec2svc "github.com/HMasataka/cloudia/internal/service/ec2"
	elasticachesvc "github.com/HMasataka/cloudia/internal/service/elasticache"
	gcesvc "github.com/HMasataka/cloudia/internal/service/gce"
	gcssvc "github.com/HMasataka/cloudia/internal/service/gcs"
	iamsvc "github.com/HMasataka/cloudia/internal/service/iam"
	memorystoresvc "github.com/HMasataka/cloudia/internal/service/memorystore"
	rdssvc "github.com/HMasataka/cloudia/internal/service/rds"
	s3svc "github.com/HMasataka/cloudia/internal/service/s3"
	sgsvc "github.com/HMasataka/cloudia/internal/service/sg"
	sqssvc "github.com/HMasataka/cloudia/internal/service/sqs"
	vpcsvc "github.com/HMasataka/cloudia/internal/service/vpc"
	"github.com/HMasataka/cloudia/internal/state"
)

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start Cloudia",
		RunE:  runStart,
	}
}

func pidFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".cloudia", "cloudia.pid"), nil
}

func runStart(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if logLevel != "" {
		cfg.Logging.Level = logLevel
	}

	logger, err := logging.NewLogger(cfg.Logging)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Sync() //nolint:errcheck

	pidPath, err := pidFilePath()
	if err != nil {
		return fmt.Errorf("failed to determine pid file path: %w", err)
	}

	pidDir := filepath.Dir(pidPath)
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		return fmt.Errorf("failed to create pid directory: %w", err)
	}

	f, err := os.OpenFile(pidPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cloudia is already running: %w", err)
	}
	fmt.Fprintf(f, "%d", os.Getpid())
	if err := f.Close(); err != nil {
		logger.Warn("failed to close pid file", zap.Error(err))
	}

	defer os.Remove(pidPath)

	ctx := context.Background()

	dockerClient, err := docker.NewClient(cfg.Docker, logger)
	if err != nil {
		return fmt.Errorf("docker is not available: %w", err)
	}

	if err := dockerClient.Ping(ctx); err != nil {
		return fmt.Errorf("docker is not available: %w", err)
	}
	defer dockerClient.Close()

	if _, err := dockerClient.EnsureNetwork(ctx, cfg.Docker.NetworkName); err != nil {
		return fmt.Errorf("failed to ensure docker network %q: %w", cfg.Docker.NetworkName, err)
	}

	verifiers := map[string]auth.Verifier{
		"aws": auth.NewSigV4Verifier(cfg.Auth.AWS),
		"gcp": auth.NewOAuthVerifier(cfg.Auth.GCP),
	}

	codecs := map[string]protocol.Codec{
		"aws": &awsprotocol.AWSCodec{},
		"gcp": &gcpprotocol.GCPCodec{},
	}

	memStore := state.NewMemoryStore()
	lockManager := state.NewLockManager(cfg.State.LockTimeout)
	limiter, err := resource.NewLimiter(memStore, cfg.Limits)
	if err != nil {
		return fmt.Errorf("failed to create limiter: %w", err)
	}
	portManager := resource.NewPortManager(cfg.Ports)

	registry := service.NewRegistry()

	deps := service.ServiceDeps{
		Store:          memStore,
		LockManager:    lockManager,
		Limiter:        limiter,
		PortAllocator:  portManager,
		DockerClient:   dockerClient,
		NetworkManager: dockerClient,
		Registry:       registry,
	}

	if err := registry.Register(s3svc.NewS3Service(cfg.Auth.AWS, logger)); err != nil {
		return fmt.Errorf("failed to register s3 service: %w", err)
	}

	if err := registry.Register(gcssvc.NewGCSService(cfg.Auth.AWS, logger)); err != nil {
		return fmt.Errorf("failed to register gcs service: %w", err)
	}

	if err := registry.Register(iamsvc.NewIAMService(cfg.Auth.AWS, logger)); err != nil {
		return fmt.Errorf("failed to register iam service: %w", err)
	}

	if err := registry.Register(sqssvc.NewSQSService(cfg.Auth.AWS, logger)); err != nil {
		return fmt.Errorf("failed to register sqs service: %w", err)
	}

	if err := registry.Register(vpcsvc.NewVPCService(cfg.Auth.AWS, logger)); err != nil {
		return fmt.Errorf("failed to register vpc service: %w", err)
	}

	if err := registry.Register(ec2svc.NewEC2Service(cfg.Auth.AWS, logger)); err != nil {
		return fmt.Errorf("failed to register ec2 service: %w", err)
	}

	if err := registry.Register(sgsvc.NewSGService(cfg.Auth.AWS, logger)); err != nil {
		return fmt.Errorf("failed to register sg service: %w", err)
	}

	if err := registry.Register(gcesvc.NewGCEService(logger)); err != nil {
		return fmt.Errorf("failed to register gce service: %w", err)
	}

	if err := registry.Register(elasticachesvc.NewElastiCacheService(cfg.Auth.AWS, logger)); err != nil {
		return fmt.Errorf("failed to register elasticache service: %w", err)
	}

	if err := registry.Register(rdssvc.NewRDSService(cfg.Auth.AWS, logger)); err != nil {
		return fmt.Errorf("failed to register rds service: %w", err)
	}

	if err := registry.Register(memorystoresvc.NewMemorystoreService(logger)); err != nil {
		return fmt.Errorf("failed to register memorystore service: %w", err)
	}

	if err := registry.Register(cloudsqlsvc.NewCloudSQLService(logger)); err != nil {
		return fmt.Errorf("failed to register cloudsql service: %w", err)
	}

	if err := registry.Register(dynamodbsvc.NewDynamoDBService(cfg.Auth.AWS, logger)); err != nil {
		return fmt.Errorf("failed to register dynamodb service: %w", err)
	}

	if err := registry.InitAll(ctx, deps); err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}

	serviceHandler := gateway.NewServiceHandler(verifiers, codecs, registry, logger)
	adminHandler := admin.NewHandler(dockerClient, logger)
	router := gateway.NewRouter(adminHandler, serviceHandler, logger, cfg.Server.Timeout)
	server := gateway.NewServerWithStore(cfg.Server, cfg.Endpoints, router, memStore, logger)

	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-sigCtx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := registry.ShutdownAll(shutdownCtx); err != nil {
		logger.Warn("service shutdown error", zap.Error(err))
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	return nil
}
