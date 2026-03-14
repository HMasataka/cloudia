package dynamodb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

// DynamoDBService implements service.Service and service.ProxyService for DynamoDB Local emulation.
type DynamoDBService struct {
	cfg     config.AWSAuthConfig
	backend *dynamodbBackend
	logger  *zap.Logger
	proxy   *httputil.ReverseProxy
}

// NewDynamoDBService creates a new DynamoDBService with the given AWS auth configuration.
func NewDynamoDBService(cfg config.AWSAuthConfig, logger *zap.Logger) *DynamoDBService {
	return &DynamoDBService{
		cfg:     cfg,
		backend: &dynamodbBackend{},
		logger:  logger,
	}
}

// Name returns the service name.
func (s *DynamoDBService) Name() string {
	return "dynamodb"
}

// Provider returns the provider name.
func (s *DynamoDBService) Provider() string {
	return "aws"
}

// Init initializes the DynamoDB Local backend and constructs the reverse proxy.
func (s *DynamoDBService) Init(ctx context.Context, deps service.ServiceDeps) error {
	if err := s.backend.Init(ctx, s.cfg, deps); err != nil {
		return err
	}

	target, err := url.Parse(s.backend.baseURL)
	if err != nil {
		return fmt.Errorf("dynamodb: parse backend url: %w", err)
	}

	p := httputil.NewSingleHostReverseProxy(target)
	p.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, _ error) {
		rw.Header().Set("Content-Type", "application/x-amz-json-1.0")
		rw.WriteHeader(http.StatusBadGateway)
		rw.Write([]byte(dynamodbErrorBody)) //nolint:errcheck
	}
	s.proxy = p

	return nil
}

// HandleRequest returns ErrUnsupportedOperation; actual requests are served via ServeHTTP.
func (s *DynamoDBService) HandleRequest(_ context.Context, _ service.Request) (service.Response, error) {
	return service.Response{}, models.ErrUnsupportedOperation
}

// SupportedActions returns the list of DynamoDB actions supported by this service.
func (s *DynamoDBService) SupportedActions() []string {
	return []string{
		"CreateTable",
		"DeleteTable",
		"DescribeTable",
		"ListTables",
		"PutItem",
		"GetItem",
		"UpdateItem",
		"DeleteItem",
		"Query",
		"Scan",
		"BatchWriteItem",
		"BatchGetItem",
	}
}

// Health returns the current health status of the DynamoDB Local backend.
func (s *DynamoDBService) Health(ctx context.Context) service.HealthStatus {
	return s.backend.Health(ctx)
}

// Shutdown stops and removes the DynamoDB Local container and releases the allocated port.
func (s *DynamoDBService) Shutdown(ctx context.Context) error {
	return s.backend.Shutdown(ctx)
}
