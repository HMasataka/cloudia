package s3

import (
	"context"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/pkg/models"
)

// S3Service implements service.Service and service.ProxyService for MinIO-backed S3 emulation.
type S3Service struct {
	cfg   config.AWSAuthConfig
	minio *minioBackend
	store service.Store
}

// NewS3Service creates a new S3Service with the given AWS auth configuration.
func NewS3Service(cfg config.AWSAuthConfig) *S3Service {
	return &S3Service{
		cfg:   cfg,
		minio: &minioBackend{},
	}
}

// NewS3ServiceWithEndpoint creates an S3Service that targets an existing MinIO endpoint.
// Intended for testing without Docker integration.
func NewS3ServiceWithEndpoint(cfg config.AWSAuthConfig, endpoint string) *S3Service {
	return &S3Service{
		cfg:   cfg,
		minio: newMinioBackendWithURL(endpoint),
	}
}

// NewS3ServiceWithEndpointAndStore creates an S3Service with a preset endpoint and Store.
// Intended for testing proxy and state integration without Docker.
func NewS3ServiceWithEndpointAndStore(cfg config.AWSAuthConfig, endpoint string, store service.Store) *S3Service {
	return &S3Service{
		cfg:   cfg,
		minio: newMinioBackendWithURL(endpoint),
		store: store,
	}
}

// Name returns the service name.
func (s *S3Service) Name() string {
	return "s3"
}

// Provider returns the provider name.
func (s *S3Service) Provider() string {
	return "aws"
}

// Init initializes the MinIO backend and captures the Store dependency.
func (s *S3Service) Init(ctx context.Context, deps service.ServiceDeps) error {
	s.store = deps.Store
	return s.minio.Init(ctx, s.cfg, deps)
}

// HandleRequest returns ErrUnsupportedOperation; actual requests are served via ServeHTTP.
func (s *S3Service) HandleRequest(_ context.Context, _ service.Request) (service.Response, error) {
	return service.Response{}, models.ErrUnsupportedOperation
}

// SupportedActions returns the list of S3 actions supported by this service.
func (s *S3Service) SupportedActions() []string {
	return []string{
		"CreateBucket",
		"DeleteBucket",
		"ListBuckets",
		"HeadBucket",
		"PutObject",
		"GetObject",
		"DeleteObject",
		"ListObjectsV2",
		"CopyObject",
		"HeadObject",
	}
}

// Health returns the current health status of the MinIO backend.
func (s *S3Service) Health(ctx context.Context) service.HealthStatus {
	return s.minio.Health(ctx)
}

// Shutdown stops and removes the MinIO container and releases the allocated port.
func (s *S3Service) Shutdown(ctx context.Context) error {
	return s.minio.Shutdown(ctx)
}

