package s3_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	s3svc "github.com/HMasataka/cloudia/internal/service/s3"
	"github.com/HMasataka/cloudia/pkg/models"
)

// --- stubs ---

type stubPortAllocator struct {
	allocatedPort int
	allocateErr   error
	released      bool
}

func (s *stubPortAllocator) Allocate(_ int, _ string) (int, error) {
	return s.allocatedPort, s.allocateErr
}

func (s *stubPortAllocator) Release(_ int) {
	s.released = true
}

type stubContainerRunner struct {
	runID  string
	runErr error

	stopErr   error
	removeErr error

	stopCalled   bool
	removeCalled bool
}

func (s *stubContainerRunner) RunContainer(_ context.Context, _ docker.ContainerConfig) (string, error) {
	return s.runID, s.runErr
}

func (s *stubContainerRunner) StopContainer(_ context.Context, _ string, _ *int) error {
	s.stopCalled = true
	return s.stopErr
}

func (s *stubContainerRunner) RemoveContainer(_ context.Context, _ string) error {
	s.removeCalled = true
	return s.removeErr
}

// --- S3Service identity tests ---

func TestS3Service_Name(t *testing.T) {
	svc := s3svc.NewS3Service(config.AWSAuthConfig{})

	if got := svc.Name(); got != "s3" {
		t.Errorf("Name() = %q, want %q", got, "s3")
	}
}

func TestS3Service_Provider(t *testing.T) {
	svc := s3svc.NewS3Service(config.AWSAuthConfig{})

	if got := svc.Provider(); got != "aws" {
		t.Errorf("Provider() = %q, want %q", got, "aws")
	}
}

func TestS3Service_SupportedActions(t *testing.T) {
	svc := s3svc.NewS3Service(config.AWSAuthConfig{})
	actions := svc.SupportedActions()

	expected := []string{
		"CreateBucket", "DeleteBucket", "ListBuckets", "HeadBucket",
		"PutObject", "GetObject", "DeleteObject", "ListObjectsV2",
		"CopyObject", "HeadObject",
	}

	if len(actions) != len(expected) {
		t.Fatalf("SupportedActions() len = %d, want %d", len(actions), len(expected))
	}

	set := make(map[string]struct{}, len(actions))
	for _, a := range actions {
		set[a] = struct{}{}
	}
	for _, want := range expected {
		if _, ok := set[want]; !ok {
			t.Errorf("SupportedActions() missing %q", want)
		}
	}
}

func TestS3Service_HandleRequest_ReturnsErrUnsupportedOperation(t *testing.T) {
	svc := s3svc.NewS3Service(config.AWSAuthConfig{})

	_, err := svc.HandleRequest(context.Background(), service.Request{})

	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Errorf("HandleRequest() error = %v, want ErrUnsupportedOperation", err)
	}
}

// --- Init failure tests ---

func TestS3Service_Init_PortAllocateError_ReturnsError(t *testing.T) {
	svc := s3svc.NewS3Service(config.AWSAuthConfig{
		AccessKey: "testkey",
		SecretKey: "testsecret",
	})

	allocErr := errors.New("no ports available")
	deps := service.ServiceDeps{
		PortAllocator: &stubPortAllocator{allocateErr: allocErr},
		DockerClient:  &stubContainerRunner{},
	}

	err := svc.Init(context.Background(), deps)

	if err == nil {
		t.Fatal("Init() should have returned an error")
	}
	if !errors.Is(err, allocErr) {
		t.Errorf("Init() error = %v, want to wrap %v", err, allocErr)
	}
}

func TestS3Service_Init_RunContainerError_ReleasesPort(t *testing.T) {
	svc := s3svc.NewS3Service(config.AWSAuthConfig{
		AccessKey: "testkey",
		SecretKey: "testsecret",
	})

	portAlloc := &stubPortAllocator{allocatedPort: 19000}
	runner := &stubContainerRunner{runErr: errors.New("docker error")}
	deps := service.ServiceDeps{
		PortAllocator: portAlloc,
		DockerClient:  runner,
	}

	err := svc.Init(context.Background(), deps)

	if err == nil {
		t.Fatal("Init() should have returned an error when RunContainer fails")
	}
	if !portAlloc.released {
		t.Error("Init() should release the allocated port on failure")
	}
}

// --- Health tests ---

func TestS3Service_Health_WhenMinioReady_ReturnsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := s3svc.NewS3ServiceWithEndpoint(config.AWSAuthConfig{}, server.URL)

	status := svc.Health(context.Background())

	if !status.Healthy {
		t.Errorf("Health() Healthy = false, want true; message: %s", status.Message)
	}
}

func TestS3Service_Health_WhenMinioUnavailable_ReturnsUnhealthy(t *testing.T) {
	svc := s3svc.NewS3ServiceWithEndpoint(config.AWSAuthConfig{}, "http://localhost:19999")

	status := svc.Health(context.Background())

	if status.Healthy {
		t.Error("Health() Healthy = true, want false for unavailable endpoint")
	}
}

// --- ServeHTTP test ---

func TestS3Service_ServeHTTP_ProxiesToMinIO(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("proxied")) //nolint
	}))
	defer backend.Close()

	svc := s3svc.NewS3ServiceWithEndpoint(config.AWSAuthConfig{}, backend.URL)

	req := httptest.NewRequest(http.MethodGet, "/test-bucket/key", nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ServeHTTP status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "proxied" {
		t.Errorf("ServeHTTP body = %q, want %q", body, "proxied")
	}
}
