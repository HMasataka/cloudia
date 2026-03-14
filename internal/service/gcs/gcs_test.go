package gcs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/HMasataka/cloudia/internal/config"
	gcssvc "github.com/HMasataka/cloudia/internal/service/gcs"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

func TestGCSService_Name(t *testing.T) {
	svc := gcssvc.NewGCSService(config.AWSAuthConfig{}, zap.NewNop())

	if got := svc.Name(); got != "storage" {
		t.Errorf("Name() = %q, want %q", got, "storage")
	}
}

func TestGCSService_Provider(t *testing.T) {
	svc := gcssvc.NewGCSService(config.AWSAuthConfig{}, zap.NewNop())

	if got := svc.Provider(); got != "gcp" {
		t.Errorf("Provider() = %q, want %q", got, "gcp")
	}
}

func TestGCSService_SupportedActions(t *testing.T) {
	svc := gcssvc.NewGCSService(config.AWSAuthConfig{}, zap.NewNop())
	actions := svc.SupportedActions()

	expected := []string{
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

func TestGCSService_HandleRequest_ReturnsErrUnsupportedOperation(t *testing.T) {
	svc := gcssvc.NewGCSService(config.AWSAuthConfig{}, zap.NewNop())

	_, err := svc.HandleRequest(context.Background(), service.Request{})

	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Errorf("HandleRequest() error = %v, want ErrUnsupportedOperation", err)
	}
}
