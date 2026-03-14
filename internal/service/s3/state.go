package s3

import (
	"net/http"
	"time"

	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

const (
	resourceKind     = "aws:s3:bucket"
	resourceProvider = "aws"
	resourceService  = "s3"
)

// updateStateOnSuccess updates the State Store when a bucket operation succeeds.
func (s *S3Service) updateStateOnSuccess(r *http.Request, statusCode int) {
	bucket, key := parsePath(r.URL.Path)
	if bucket == "" || key != "" {
		return
	}

	switch {
	case r.Method == http.MethodPut && statusCode == http.StatusOK:
		s.createBucketResource(r, bucket)
	case r.Method == http.MethodDelete && statusCode == http.StatusNoContent:
		s.deleteBucketResource(r, bucket)
	}
}

func (s *S3Service) createBucketResource(r *http.Request, bucket string) {
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
		s.logger.Error("failed to record bucket in state store", zap.String("bucket", bucket), zap.Error(err))
	}
}

func (s *S3Service) deleteBucketResource(r *http.Request, bucket string) {
	if err := s.store.Delete(r.Context(), resourceKind, bucket); err != nil {
		s.logger.Error("failed to remove bucket from state store", zap.String("bucket", bucket), zap.Error(err))
	}
}
