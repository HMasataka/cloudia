package gcs

import (
	"context"
	"fmt"
	"net/http"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

// GCSService implements service.Service and service.ProxyService for GCS emulation backed by MinIO.
type GCSService struct {
	cfg     config.AWSAuthConfig // MinIO credentials (reuses AWS config for access/secret key)
	baseURL string               // MinIO base URL obtained from SharedBackend("minio-url")
	store   service.Store
	logger  *zap.Logger
}

// NewGCSService creates a new GCSService.
// awsCfg holds the MinIO root credentials (MINIO_ROOT_USER / MINIO_ROOT_PASSWORD).
func NewGCSService(awsCfg config.AWSAuthConfig, logger *zap.Logger) *GCSService {
	return &GCSService{
		cfg:    awsCfg,
		logger: logger,
	}
}

// NewGCSServiceWithEndpoint creates a GCSService that targets an existing MinIO endpoint.
// Intended for testing without Docker integration.
func NewGCSServiceWithEndpoint(cfg config.AWSAuthConfig, endpoint string, store service.Store) *GCSService {
	return &GCSService{
		cfg:     cfg,
		baseURL: endpoint,
		store:   store,
		logger:  zap.NewNop(),
	}
}

// Name returns the GCP service name.
func (s *GCSService) Name() string {
	return "storage"
}

// Provider returns the provider name.
func (s *GCSService) Provider() string {
	return "gcp"
}

// Init retrieves the shared MinIO baseURL from the registry and captures the Store dependency.
func (s *GCSService) Init(_ context.Context, deps service.ServiceDeps) error {
	s.store = deps.Store

	if deps.Registry == nil {
		return fmt.Errorf("gcs: registry is nil; S3 service must be registered before GCS")
	}

	raw := deps.Registry.SharedBackend("minio-url")
	if raw == nil {
		return fmt.Errorf("gcs: shared backend \"minio-url\" not found; S3 service must be initialized before GCS")
	}

	baseURL, ok := raw.(string)
	if !ok || baseURL == "" {
		return fmt.Errorf("gcs: shared backend \"minio-url\" is not a valid string")
	}

	s.baseURL = baseURL
	return nil
}

// HandleRequest returns ErrUnsupportedOperation; actual requests are served via ServeHTTP.
func (s *GCSService) HandleRequest(_ context.Context, _ service.Request) (service.Response, error) {
	return service.Response{}, models.ErrUnsupportedOperation
}

// SupportedActions returns the list of GCS actions supported by this service.
func (s *GCSService) SupportedActions() []string {
	return []string{
		"buckets.insert",
		"buckets.get",
		"buckets.list",
		"buckets.delete",
		"objects.insert",
		"objects.get",
		"objects.list",
		"objects.delete",
		"objects.copy",
	}
}

// Health checks whether the MinIO backend is healthy via its live endpoint.
func (s *GCSService) Health(ctx context.Context) service.HealthStatus {
	if s.baseURL == "" {
		return service.HealthStatus{Healthy: false, Message: "not initialized"}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/minio/health/live", nil)
	if err != nil {
		return service.HealthStatus{Healthy: false, Message: err.Error()}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return service.HealthStatus{Healthy: false, Message: err.Error()}
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return service.HealthStatus{Healthy: false, Message: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
	}

	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown is a no-op; the MinIO container lifecycle is managed by the S3 service.
func (s *GCSService) Shutdown(_ context.Context) error {
	return nil
}
