package s3

import (
	"context"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

// S3Service implements service.Service and service.ProxyService for MinIO-backed S3 emulation.
type S3Service struct {
	cfg    config.AWSAuthConfig
	minio  *minioBackend
	store  service.Store
	logger *zap.Logger
}

// NewS3Service creates a new S3Service with the given AWS auth configuration.
func NewS3Service(cfg config.AWSAuthConfig, logger *zap.Logger) *S3Service {
	return &S3Service{
		cfg:    cfg,
		minio:  &minioBackend{},
		logger: logger,
	}
}

// NewS3ServiceWithEndpoint creates an S3Service that targets an existing MinIO endpoint.
// Intended for testing without Docker integration.
func NewS3ServiceWithEndpoint(cfg config.AWSAuthConfig, endpoint string, store service.Store) *S3Service {
	return &S3Service{
		cfg:    cfg,
		minio:  newMinioBackendWithURL(endpoint),
		store:  store,
		logger: zap.NewNop(),
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
	if err := s.minio.Init(ctx, s.cfg, deps); err != nil {
		return err
	}
	if deps.Registry != nil {
		deps.Registry.SharedBackend("minio-url", s.minio.baseURL)
	}
	return nil
}

// HandleRequest returns ErrUnsupportedOperation; actual requests are served via ServeHTTP.
func (s *S3Service) HandleRequest(_ context.Context, _ service.Request) (service.Response, error) {
	return service.Response{}, models.ErrUnsupportedOperation
}

// SupportedActions returns the list of S3 actions supported by this service.
func (s *S3Service) SupportedActions() []string {
	return []string{
		// Basic bucket operations
		"CreateBucket",
		"DeleteBucket",
		"ListBuckets",
		"HeadBucket",
		// Object operations
		"PutObject",
		"GetObject",
		"DeleteObject",
		"ListObjectsV2",
		"CopyObject",
		"HeadObject",
		// Multipart upload
		"CreateMultipartUpload",
		"UploadPart",
		"CompleteMultipartUpload",
		"AbortMultipartUpload",
		"ListMultipartUploads",
		"ListParts",
		// Bucket policy
		"PutBucketPolicy",
		"GetBucketPolicy",
		"DeleteBucketPolicy",
		// Versioning
		"PutBucketVersioning",
		"GetBucketVersioning",
		// ACL
		"PutBucketAcl",
		"GetBucketAcl",
		// CORS
		"PutBucketCors",
		"GetBucketCors",
		"DeleteBucketCors",
		// Lifecycle
		"PutBucketLifecycleConfiguration",
		"GetBucketLifecycleConfiguration",
		"DeleteBucketLifecycleConfiguration",
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

