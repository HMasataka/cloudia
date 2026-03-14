package dynamodb

import (
	"context"

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
}

// NewDynamoDBService creates a new DynamoDBService with the given AWS auth configuration.
func NewDynamoDBService(cfg config.AWSAuthConfig, logger *zap.Logger) *DynamoDBService {
	return &DynamoDBService{
		cfg:     cfg,
		backend: &dynamodbBackend{},
		logger:  logger,
	}
}

// NewDynamoDBServiceWithEndpoint creates a DynamoDBService that targets an existing DynamoDB Local endpoint.
// Intended for testing without Docker integration.
func NewDynamoDBServiceWithEndpoint(cfg config.AWSAuthConfig, endpoint string) *DynamoDBService {
	return &DynamoDBService{
		cfg:     cfg,
		backend: newDynamoDBBackendWithURL(endpoint),
		logger:  zap.NewNop(),
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

// Init initializes the DynamoDB Local backend.
func (s *DynamoDBService) Init(ctx context.Context, deps service.ServiceDeps) error {
	return s.backend.Init(ctx, s.cfg, deps)
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
