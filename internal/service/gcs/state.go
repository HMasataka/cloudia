package gcs

import (
	"net/http"
	"time"

	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

const (
	resourceKind     = "gcp:storage:bucket"
	resourceProvider = "gcp"
	resourceService  = "storage"
)

// updateStateOnSuccess updates the State Store when a GCS bucket operation succeeds.
// bucket is the bucket name, method is the original GCS HTTP method, and statusCode
// is the status code returned by MinIO.
func (s *GCSService) updateStateOnSuccess(r *http.Request, bucket, method string, statusCode int) {
	if bucket == "" {
		return
	}

	switch {
	case method == http.MethodPost && (statusCode == http.StatusOK || statusCode == http.StatusCreated):
		s.createBucketResource(r, bucket)
	case method == http.MethodDelete && (statusCode == http.StatusOK || statusCode == http.StatusNoContent):
		s.deleteBucketResource(r, bucket)
	}
}

func (s *GCSService) createBucketResource(r *http.Request, bucket string) {
	resource := &models.Resource{
		Kind:      resourceKind,
		ID:        bucket,
		Provider:  resourceProvider,
		Service:   resourceService,
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.store.Put(r.Context(), resource); err != nil {
		s.logger.Error("gcs: failed to record bucket in state store",
			zap.String("bucket", bucket),
			zap.Error(err),
		)
	}
}

func (s *GCSService) deleteBucketResource(r *http.Request, bucket string) {
	if err := s.store.Delete(r.Context(), resourceKind, bucket); err != nil {
		s.logger.Error("gcs: failed to remove bucket from state store",
			zap.String("bucket", bucket),
			zap.Error(err),
		)
	}
}
