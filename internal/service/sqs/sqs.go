package sqs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

const (
	contentType  = "application/x-amz-json-1.0"
	resourceKind = "aws:sqs:queue"
)

// SQSService は SQS のインメモリ実装です。
type SQSService struct {
	cfg    config.AWSAuthConfig
	store  service.Store
	logger *zap.Logger
}

// NewSQSService は新しい SQSService を生成して返します。
func NewSQSService(cfg config.AWSAuthConfig, logger *zap.Logger) *SQSService {
	return &SQSService{
		cfg:    cfg,
		logger: logger,
	}
}

// Name はサービス名を返します。
func (s *SQSService) Name() string {
	return "sqs"
}

// Provider はプロバイダ名を返します。
func (s *SQSService) Provider() string {
	return "aws"
}

// Init はサービスを初期化します。
func (s *SQSService) Init(_ context.Context, deps service.ServiceDeps) error {
	s.store = deps.Store
	return nil
}

// SupportedActions はサポートするアクション一覧を返します。
func (s *SQSService) SupportedActions() []string {
	return []string{
		"CreateQueue",
		"DeleteQueue",
		"GetQueueAttributes",
		"GetQueueUrl",
		"ListQueues",
		"TagQueue",
	}
}

// Health は常に healthy を返します。
func (s *SQSService) Health(_ context.Context) service.HealthStatus {
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は何もしません。
func (s *SQSService) Shutdown(_ context.Context) error {
	return nil
}

// HandleRequest は Action に応じた処理を行います。
func (s *SQSService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	switch req.Action {
	case "CreateQueue":
		return s.createQueue(ctx, req)
	case "DeleteQueue":
		return s.deleteQueue(ctx, req)
	case "GetQueueAttributes":
		return s.getQueueAttributes(ctx, req)
	case "GetQueueUrl":
		return s.getQueueUrl(ctx, req)
	case "ListQueues":
		return s.listQueues(ctx, req)
	case "TagQueue":
		return s.tagQueue(ctx, req)
	default:
		return errorResponse(http.StatusBadRequest, "UnsupportedOperation", fmt.Sprintf("unsupported action: %s", req.Action))
	}
}

// jsonResponse は JSON レスポンスを生成します。
func jsonResponse(statusCode int, body any) (service.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return service.Response{}, fmt.Errorf("marshal response: %w", err)
	}
	return service.Response{
		StatusCode:  statusCode,
		Body:        b,
		ContentType: contentType,
	}, nil
}

// sqsError は SQS エラーレスポンスの構造体です。
type sqsError struct {
	Code    string `json:"__type"`
	Message string `json:"message"`
}

// errorResponse は SQS エラーレスポンスを生成します。
func errorResponse(statusCode int, code, message string) (service.Response, error) {
	return jsonResponse(statusCode, sqsError{Code: code, Message: message})
}

// decodeBody は req.Body を JSON としてデコードします。
func decodeBody(body []byte, v any) error {
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

// queueURL は指定されたキュー名の URL を生成します。
func queueURL(accountID, queueName string) string {
	return fmt.Sprintf("http://localhost:4566/%s/%s", accountID, queueName)
}

// queueNameFromURL は URL からキュー名を抽出します。
func queueNameFromURL(url, accountID string) string {
	prefix := fmt.Sprintf("http://localhost:4566/%s/", accountID)
	if len(url) > len(prefix) && url[:len(prefix)] == prefix {
		return url[len(prefix):]
	}
	return ""
}

// isNotFound は error が models.ErrNotFound かを判定します。
func isNotFound(err error) bool {
	return errors.Is(err, models.ErrNotFound)
}
