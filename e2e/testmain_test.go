//go:build e2e

package e2e_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

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
	ekssvc "github.com/HMasataka/cloudia/internal/service/eks"
	elasticachesvc "github.com/HMasataka/cloudia/internal/service/elasticache"
	gcesvc "github.com/HMasataka/cloudia/internal/service/gce"
	gcssvc "github.com/HMasataka/cloudia/internal/service/gcs"
	gkesvc "github.com/HMasataka/cloudia/internal/service/gke"
	iamsvc "github.com/HMasataka/cloudia/internal/service/iam"
	lambdasvc "github.com/HMasataka/cloudia/internal/service/lambda"
	memorystoresvc "github.com/HMasataka/cloudia/internal/service/memorystore"
	pubsubsvc "github.com/HMasataka/cloudia/internal/service/pubsub"
	rdssvc "github.com/HMasataka/cloudia/internal/service/rds"
	s3svc "github.com/HMasataka/cloudia/internal/service/s3"
	sgsvc "github.com/HMasataka/cloudia/internal/service/sg"
	sqssvc "github.com/HMasataka/cloudia/internal/service/sqs"
	vpcsvc "github.com/HMasataka/cloudia/internal/service/vpc"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

// testServer はテスト用サーバーの情報を保持します。
type testServer struct {
	baseURL     string
	server      *gateway.Server
	dockerClient *docker.Client
	logger      *zap.Logger
}

var globalServer *testServer

// TestMain はE2Eテストのエントリポイントです。
// Cloudiaサーバーを起動し、全テスト実行後にクリーンアップします。
func TestMain(m *testing.M) {
	srv, err := startTestServer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start test server: %v\n", err)
		os.Exit(1)
	}
	globalServer = srv

	code := m.Run()

	// クリーンアップ
	if srv.dockerClient != nil {
		ctx := context.Background()
		if _, cleanErr := srv.dockerClient.CleanupOrphans(ctx); cleanErr != nil {
			fmt.Fprintf(os.Stderr, "cleanup orphans failed: %v\n", cleanErr)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if shutdownErr := srv.server.Shutdown(shutdownCtx); shutdownErr != nil {
		fmt.Fprintf(os.Stderr, "server shutdown error: %v\n", shutdownErr)
	}

	os.Exit(code)
}

// startTestServer はテスト用Cloudiaサーバーを起動します。
func startTestServer() (*testServer, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("config.Load: %w", err)
	}

	// ランダムポートを使用してポート衝突を回避
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("find free port: %w", err)
	}
	cfg.Server.Port = port
	cfg.Server.Host = "127.0.0.1"
	cfg.Metrics.Enabled = false

	logger, err := logging.NewLogger(cfg.Logging)
	if err != nil {
		return nil, fmt.Errorf("logging.NewLogger: %w", err)
	}

	ctx := context.Background()

	dockerClient, err := docker.NewClient(cfg.Docker, logger)
	if err != nil {
		// Docker未インストールの場合はDockerなしで起動
		logger.Warn("docker not available, running without docker", zap.Error(err))
		dockerClient = nil
	}

	if dockerClient != nil {
		if pingErr := dockerClient.Ping(ctx); pingErr != nil {
			logger.Warn("docker daemon not reachable, running without docker", zap.Error(pingErr))
			dockerClient = nil
		}
	}

	if dockerClient != nil {
		if _, netErr := dockerClient.EnsureNetwork(ctx, cfg.Docker.NetworkName); netErr != nil {
			return nil, fmt.Errorf("ensure docker network: %w", netErr)
		}
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
		return nil, fmt.Errorf("resource.NewLimiter: %w", err)
	}
	if dockerClient != nil {
		limiter.SetDiskChecker(dockerClient)
	}
	portManager := resource.NewPortManager(cfg.Ports)

	registry := service.NewRegistry()

	var dockerRunner service.ContainerRunner
	var networkManager service.NetworkManager
	if dockerClient != nil {
		dockerRunner = dockerClient
		networkManager = dockerClient
	}

	deps := service.ServiceDeps{
		Store:          memStore,
		LockManager:    lockManager,
		Limiter:        limiter,
		PortAllocator:  portManager,
		DockerClient:   dockerRunner,
		NetworkManager: networkManager,
		Registry:       registry,
	}

	services := []service.Service{
		s3svc.NewS3Service(cfg.Auth.AWS, logger),
		gcssvc.NewGCSService(cfg.Auth.AWS, logger),
		iamsvc.NewIAMService(cfg.Auth.AWS, logger),
		sqssvc.NewSQSService(cfg.Auth.AWS, logger),
		vpcsvc.NewVPCService(cfg.Auth.AWS, logger),
		ec2svc.NewEC2Service(cfg.Auth.AWS, logger),
		sgsvc.NewSGService(cfg.Auth.AWS, logger),
		gcesvc.NewGCEService(cfg.Auth.GCP, logger),
		pubsubsvc.NewPubSubService(logger),
		elasticachesvc.NewElastiCacheService(cfg.Auth.AWS, logger),
		rdssvc.NewRDSService(cfg.Auth.AWS, logger),
		memorystoresvc.NewMemorystoreService(logger),
		cloudsqlsvc.NewCloudSQLService(logger),
		dynamodbsvc.NewDynamoDBService(cfg.Auth.AWS, logger),
		lambdasvc.NewLambdaService(cfg.Auth.AWS, logger),
		ekssvc.NewEKSService(logger),
		gkesvc.NewGKEService(logger),
	}

	for _, svc := range services {
		if regErr := registry.Register(svc); regErr != nil {
			return nil, fmt.Errorf("register service %q: %w", svc.Name(), regErr)
		}
	}

	// 各サービスを個別に初期化し、Docker依存サービスの失敗は警告としてログに記録する。
	// これにより、Docker Hubへのアクセスが制限されている環境でもサーバーが起動できる。
	for _, svc := range services {
		svcKey := fmt.Sprintf("%s:%s", svc.Provider(), svc.Name())
		if initErr := svc.Init(ctx, deps); initErr != nil {
			logger.Warn("service init failed, service will be unavailable",
				zap.String("service", svcKey),
				zap.Error(initErr),
			)
		}
	}

	serviceHandler := gateway.NewServiceHandler(verifiers, codecs, registry, logger)
	adminHandler := admin.NewHandler(dockerClient, logger)
	router := gateway.NewRouter(adminHandler, serviceHandler, logger, cfg.Server.Timeout)
	server := gateway.NewServer(cfg.Server, cfg.Endpoints, cfg.Metrics, router, logger)

	if startErr := server.Start(); startErr != nil {
		return nil, fmt.Errorf("server.Start: %w", startErr)
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if waitErr := waitForServer(baseURL+"/health", 10*time.Second); waitErr != nil {
		return nil, fmt.Errorf("server did not become ready: %w", waitErr)
	}

	return &testServer{
		baseURL:      baseURL,
		server:       server,
		dockerClient: dockerClient,
		logger:       logger,
	}, nil
}

// freePort は利用可能なランダムポートを返します。
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// waitForServer は指定URLが200を返すまでポーリングします。
func waitForServer(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:gosec,noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server at %s did not become ready within %s", url, timeout)
}
