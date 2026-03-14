package s3

import (
	"errors"
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

// bucketConfigQueryParams lists query parameters that represent bucket configuration sub-resources.
var bucketConfigQueryParams = []string{"versioning", "policy", "acl", "cors", "lifecycle"}

// updateStateOnSuccess updates the State Store when a bucket operation succeeds.
func (s *S3Service) updateStateOnSuccess(r *http.Request, statusCode int) {
	bucket, key := parsePath(r.URL.Path)
	if bucket == "" || key != "" {
		return
	}

	// Detect bucket configuration sub-resource query parameters.
	configParam := ""
	q := r.URL.Query()
	for _, param := range bucketConfigQueryParams {
		if q.Has(param) {
			configParam = param
			break
		}
	}

	switch {
	case r.Method == http.MethodPut && statusCode == http.StatusOK && configParam != "":
		s.updateBucketConfig(r, bucket, configParam)
	case r.Method == http.MethodPut && statusCode == http.StatusOK:
		s.createBucketResource(r, bucket)
	case r.Method == http.MethodDelete && statusCode == http.StatusNoContent:
		s.deleteBucketResource(r, bucket)
	}
}

// updateBucketConfig records a bucket configuration change in the State Store.
func (s *S3Service) updateBucketConfig(r *http.Request, bucket, configParam string) {
	resource, err := s.store.Get(r.Context(), resourceKind, bucket)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			s.logger.Warn("bucket not found in state store; skipping config update",
				zap.String("bucket", bucket), zap.String("config", configParam))
		} else {
			s.logger.Error("failed to get bucket from state store for config update",
				zap.String("bucket", bucket), zap.String("config", configParam), zap.Error(err))
		}
		return
	}
	if resource.Spec == nil {
		resource.Spec = make(map[string]interface{})
	}
	resource.Spec[configParam] = "Enabled"
	resource.UpdatedAt = time.Now()
	if err := s.store.Put(r.Context(), resource); err != nil {
		s.logger.Error("failed to update bucket config in state store",
			zap.String("bucket", bucket), zap.String("config", configParam), zap.Error(err))
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
