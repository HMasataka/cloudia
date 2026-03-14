package s3

import (
	"net/http"
	"time"

	"github.com/HMasataka/cloudia/pkg/models"
)

const (
	resourceKind     = "aws:s3:bucket"
	resourceProvider = "aws"
	resourceService  = "s3"
)

// updateStateOnSuccess updates the State Store when a bucket operation succeeds.
// CreateBucket (PUT /{bucket}, 200) writes a Resource; DeleteBucket (DELETE /{bucket}, 204) removes it.
func (s *S3Service) updateStateOnSuccess(r *http.Request, statusCode int) {
	if s.store == nil {
		return
	}

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
	_ = s.store.Put(r.Context(), resource)
}

func (s *S3Service) deleteBucketResource(r *http.Request, bucket string) {
	_ = s.store.Delete(r.Context(), resourceKind, bucket)
}
