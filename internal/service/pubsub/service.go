package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/service"
)

const (
	contentType = "application/json; charset=utf-8"

	// maxMessageSize は 1 メッセージの最大サイズです (10MB)。
	maxMessageSize = 10 * 1024 * 1024

	// maxMessagesPerSubscription はサブスクリプションあたりの最大メッセージ保持件数です。
	maxMessagesPerSubscription = 10000

	// maxPullMessages は pull の最大 maxMessages 値です。
	maxPullMessages = 1000

	// defaultPullMessages は pull のデフォルト maxMessages 値です。
	defaultPullMessages = 100
)

// topicNameRegexp は topic/subscription 名のバリデーション用正規表現です。
var topicNameRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9\-_.~+%]{2,254}$`)

// PubSubBackend はインメモリの Pub/Sub バックエンドです。
type PubSubBackend struct {
	mu            sync.RWMutex
	topics        map[string]*Topic                  // key: "projects/{p}/topics/{t}"
	subscriptions map[string]*Subscription           // key: "projects/{p}/subscriptions/{s}"
	messages      map[string][]*PendingMessage       // key: subscription full name
	messageIDSeq  int64
}

// newPubSubBackend は新しい PubSubBackend を返します。
func newPubSubBackend() *PubSubBackend {
	return &PubSubBackend{
		topics:        make(map[string]*Topic),
		subscriptions: make(map[string]*Subscription),
		messages:      make(map[string][]*PendingMessage),
	}
}

// nextMessageID はユニークなメッセージ ID を返します。
func (b *PubSubBackend) nextMessageID() string {
	id := atomic.AddInt64(&b.messageIDSeq, 1)
	return fmt.Sprintf("%d", id)
}

// PubSubService は GCP Pub/Sub サービスのインメモリ実装です。
type PubSubService struct {
	backend *PubSubBackend
	logger  *zap.Logger
}

// NewPubSubService は新しい PubSubService を返します。
func NewPubSubService(logger *zap.Logger) *PubSubService {
	return &PubSubService{
		backend: newPubSubBackend(),
		logger:  logger,
	}
}

// Name はサービス名を返します。
func (s *PubSubService) Name() string {
	return "pubsub"
}

// Provider はプロバイダ名を返します。
func (s *PubSubService) Provider() string {
	return "gcp"
}

// Init はサービスを初期化します。
func (s *PubSubService) Init(_ context.Context, _ service.ServiceDeps) error {
	return nil
}

// SupportedActions はこのサービスがサポートするアクション名の一覧を返します。
// Pub/Sub はパスベースのルーティングを使うため、空スライスを返します。
func (s *PubSubService) SupportedActions() []string {
	return []string{}
}

// Health はサービスのヘルスステータスを返します。
func (s *PubSubService) Health(_ context.Context) service.HealthStatus {
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown は何もしません。
func (s *PubSubService) Shutdown(_ context.Context) error {
	return nil
}

// HandleRequest はリクエストを処理してレスポンスを返します。
// req.Action にリソースパス、req.Method に HTTP メソッドが入ります。
func (s *PubSubService) HandleRequest(ctx context.Context, req service.Request) (service.Response, error) {
	project, resourceType, name, op, err := parsePubSubPath(req.Action)
	if err != nil {
		return pubsubErrorResponse(http.StatusBadRequest, err.Error())
	}

	switch resourceType {
	case "topics":
		return s.handleTopics(ctx, req, project, name, op)
	case "subscriptions":
		return s.handleSubscriptions(ctx, req, project, name, op)
	default:
		return pubsubErrorResponse(http.StatusBadRequest, fmt.Sprintf("unknown resource type: %s", resourceType))
	}
}

// handleTopics は topics リソースのリクエストを処理します。
func (s *PubSubService) handleTopics(ctx context.Context, req service.Request, project, name, op string) (service.Response, error) {
	switch {
	case req.Method == http.MethodPut && name != "" && op == "":
		// PUT projects/{p}/topics/{name} -> create topic
		return s.createTopic(ctx, req, project, name)
	case req.Method == http.MethodGet && name != "" && op == "":
		// GET projects/{p}/topics/{name} -> get topic
		return s.getTopic(ctx, project, name)
	case req.Method == http.MethodGet && name == "" && op == "":
		// GET projects/{p}/topics -> list topics
		return s.listTopics(ctx, project)
	case req.Method == http.MethodDelete && name != "" && op == "":
		// DELETE projects/{p}/topics/{name} -> delete topic
		return s.deleteTopic(ctx, project, name)
	case req.Method == http.MethodPost && name != "" && op == "publish":
		// POST projects/{p}/topics/{name}:publish -> publish
		return s.publish(ctx, req, project, name)
	default:
		return pubsubErrorResponse(http.StatusBadRequest,
			fmt.Sprintf("unsupported method %s for topics path (name=%q op=%q)", req.Method, name, op))
	}
}

// handleSubscriptions は subscriptions リソースのリクエストを処理します。
func (s *PubSubService) handleSubscriptions(ctx context.Context, req service.Request, project, name, op string) (service.Response, error) {
	switch {
	case req.Method == http.MethodPut && name != "" && op == "":
		// PUT projects/{p}/subscriptions/{name} -> create subscription
		return s.createSubscription(ctx, req, project, name)
	case req.Method == http.MethodGet && name != "" && op == "":
		// GET projects/{p}/subscriptions/{name} -> get subscription
		return s.getSubscription(ctx, project, name)
	case req.Method == http.MethodGet && name == "" && op == "":
		// GET projects/{p}/subscriptions -> list subscriptions
		return s.listSubscriptions(ctx, project)
	case req.Method == http.MethodDelete && name != "" && op == "":
		// DELETE projects/{p}/subscriptions/{name} -> delete subscription
		return s.deleteSubscription(ctx, project, name)
	case req.Method == http.MethodPost && name != "" && op == "pull":
		// POST projects/{p}/subscriptions/{name}:pull -> pull
		return s.pull(ctx, req, project, name)
	case req.Method == http.MethodPost && name != "" && op == "acknowledge":
		// POST projects/{p}/subscriptions/{name}:acknowledge -> acknowledge
		return s.acknowledge(ctx, req, project, name)
	default:
		return pubsubErrorResponse(http.StatusBadRequest,
			fmt.Sprintf("unsupported method %s for subscriptions path (name=%q op=%q)", req.Method, name, op))
	}
}

// parsePubSubPath は Pub/Sub のリソースパスをパースします。
//
// 入力パス例:
//   - "projects/{p}/topics" (list)
//   - "projects/{p}/topics/{topic}" (get, delete, create)
//   - "projects/{p}/topics/{topic}:publish" (publish)
//   - "projects/{p}/subscriptions" (list)
//   - "projects/{p}/subscriptions/{sub}" (get, delete, create)
//   - "projects/{p}/subscriptions/{sub}:pull" (pull)
//   - "projects/{p}/subscriptions/{sub}:acknowledge" (acknowledge)
//
// 返り値: project, resourceType ("topics"|"subscriptions"), name, op (""|"publish"|"pull"|"acknowledge")
func parsePubSubPath(resourcePath string) (project, resourceType, name, op string, err error) {
	parts := strings.Split(resourcePath, "/")
	// parts[0]="projects", parts[1]={p}, parts[2]="topics"|"subscriptions", [parts[3]={name}[:op]]
	if len(parts) < 3 {
		return "", "", "", "", fmt.Errorf("invalid pubsub path: %q", resourcePath)
	}
	if parts[0] != "projects" {
		return "", "", "", "", fmt.Errorf("invalid pubsub path: %q", resourcePath)
	}

	project = parts[1]
	resourceType = parts[2]

	if resourceType != "topics" && resourceType != "subscriptions" {
		return "", "", "", "", fmt.Errorf("invalid pubsub path: unknown resource type %q in %q", resourceType, resourcePath)
	}

	if len(parts) >= 4 {
		nameWithOp := parts[3]
		// コロン区切りのカスタムメソッドを分離 (例: "my-topic:publish")
		if idx := strings.IndexByte(nameWithOp, ':'); idx >= 0 {
			name = nameWithOp[:idx]
			op = nameWithOp[idx+1:]
		} else {
			name = nameWithOp
		}
	}

	return project, resourceType, name, op, nil
}

// validateResourceName は topic/subscription 名を検証します。
func validateResourceName(name string) error {
	if !topicNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid resource name %q: must match [a-zA-Z][a-zA-Z0-9\\-_.~+%%]{2,254}", name)
	}
	return nil
}

// topicFullName は topic の full resource name を返します。
func topicFullName(project, topic string) string {
	return fmt.Sprintf("projects/%s/topics/%s", project, topic)
}

// subscriptionFullName はサブスクリプションの full resource name を返します。
func subscriptionFullName(project, subscription string) string {
	return fmt.Sprintf("projects/%s/subscriptions/%s", project, subscription)
}

// jsonResponse は JSON レスポンスを生成します。
func jsonResponse(statusCode int, body interface{}) (service.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	return service.Response{
		StatusCode:  statusCode,
		Body:        b,
		ContentType: contentType,
	}, nil
}

// pubsubErrorResponse は GCP Pub/Sub 互換のエラーレスポンスを返します。
func pubsubErrorResponse(statusCode int, message string) (service.Response, error) {
	grpcStatus := grpcStatusFromHTTP(statusCode)
	body, marshalErr := json.Marshal(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    statusCode,
			"message": message,
			"status":  grpcStatus,
		},
	})
	if marshalErr != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, marshalErr
	}
	return service.Response{
		StatusCode:  statusCode,
		Body:        body,
		ContentType: contentType,
	}, nil
}

// grpcStatusFromHTTP は HTTP ステータスコードを gRPC ステータス文字列に変換します。
func grpcStatusFromHTTP(code int) string {
	switch code {
	case http.StatusBadRequest:
		return "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusForbidden:
		return "PERMISSION_DENIED"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "ALREADY_EXISTS"
	case http.StatusNotImplemented:
		return "UNIMPLEMENTED"
	default:
		return "UNKNOWN"
	}
}
