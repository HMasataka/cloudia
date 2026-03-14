package pubsub

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/service"
)

func newTestService(t *testing.T) *PubSubService {
	t.Helper()
	svc := NewPubSubService(zap.NewNop())
	if err := svc.Init(context.Background(), service.ServiceDeps{}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return svc
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// request はテスト用のリクエストヘルパーです。
func request(method, action string, body []byte) service.Request {
	return service.Request{
		Method: method,
		Action: action,
		Body:   body,
	}
}

// TestTopicCRUD は topic の CRUD をテストします。
func TestTopicCRUD(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// CreateTopic
	resp, err := svc.HandleRequest(ctx, request(http.MethodPut, "projects/my-project/topics/my-topic", nil))
	if err != nil {
		t.Fatalf("create topic error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create topic status=%d body=%s", resp.StatusCode, resp.Body)
	}

	var topicResp topicResponse
	if err := json.Unmarshal(resp.Body, &topicResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if topicResp.Name != "projects/my-project/topics/my-topic" {
		t.Errorf("topic name=%q want %q", topicResp.Name, "projects/my-project/topics/my-topic")
	}

	// GetTopic
	resp, err = svc.HandleRequest(ctx, request(http.MethodGet, "projects/my-project/topics/my-topic", nil))
	if err != nil {
		t.Fatalf("get topic error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get topic status=%d body=%s", resp.StatusCode, resp.Body)
	}

	// ListTopics
	resp, err = svc.HandleRequest(ctx, request(http.MethodGet, "projects/my-project/topics", nil))
	if err != nil {
		t.Fatalf("list topics error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list topics status=%d body=%s", resp.StatusCode, resp.Body)
	}

	var listResp listTopicsResponse
	if err := json.Unmarshal(resp.Body, &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listResp.Topics) != 1 {
		t.Errorf("list topics count=%d want 1", len(listResp.Topics))
	}

	// DeleteTopic
	resp, err = svc.HandleRequest(ctx, request(http.MethodDelete, "projects/my-project/topics/my-topic", nil))
	if err != nil {
		t.Fatalf("delete topic error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete topic status=%d body=%s", resp.StatusCode, resp.Body)
	}

	// GetTopic after delete: 404
	resp, err = svc.HandleRequest(ctx, request(http.MethodGet, "projects/my-project/topics/my-topic", nil))
	if err != nil {
		t.Fatalf("get deleted topic error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for deleted topic, got %d", resp.StatusCode)
	}
}

// TestPublishPullAcknowledgeFlow は topic CRUD -> publish -> pull -> acknowledge の一連フローをテストします。
func TestPublishPullAcknowledgeFlow(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := "test-project"
	topicName := "test-topic"
	subName := "test-subscription"
	topicPath := "projects/" + project + "/topics/" + topicName
	subPath := "projects/" + project + "/subscriptions/" + subName
	fullTopicName := "projects/" + project + "/topics/" + topicName

	// 1. CreateTopic
	resp, err := svc.HandleRequest(ctx, request(http.MethodPut, topicPath, nil))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create topic: status=%d err=%v body=%s", resp.StatusCode, err, resp.Body)
	}

	// 2. CreateSubscription
	resp, err = svc.HandleRequest(ctx, request(http.MethodPut, subPath, mustJSON(t, createSubscriptionRequest{
		Topic: fullTopicName,
	})))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create subscription: status=%d err=%v body=%s", resp.StatusCode, err, resp.Body)
	}

	// 3. Publish
	msgs := publishRequest{
		Messages: []pubsubMessage{
			{Data: []byte("hello world")},
			{Data: []byte("second message")},
		},
	}
	resp, err = svc.HandleRequest(ctx, request(http.MethodPost, topicPath+":publish", mustJSON(t, msgs)))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("publish: status=%d err=%v body=%s", resp.StatusCode, err, resp.Body)
	}

	var pubResp publishResponse
	if err := json.Unmarshal(resp.Body, &pubResp); err != nil {
		t.Fatalf("unmarshal publish response: %v", err)
	}
	if len(pubResp.MessageIDs) != 2 {
		t.Errorf("expected 2 message IDs, got %d", len(pubResp.MessageIDs))
	}

	// 4. Pull
	resp, err = svc.HandleRequest(ctx, request(http.MethodPost, subPath+":pull", mustJSON(t, pullRequest{MaxMessages: 10})))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("pull: status=%d err=%v body=%s", resp.StatusCode, err, resp.Body)
	}

	var pullResp pullResponse
	if err := json.Unmarshal(resp.Body, &pullResp); err != nil {
		t.Fatalf("unmarshal pull response: %v", err)
	}
	if len(pullResp.ReceivedMessages) != 2 {
		t.Errorf("expected 2 messages from pull, got %d", len(pullResp.ReceivedMessages))
	}

	// メッセージデータの確認
	if string(pullResp.ReceivedMessages[0].Message.Data) != "hello world" {
		t.Errorf("first message data=%q want %q", pullResp.ReceivedMessages[0].Message.Data, "hello world")
	}

	// AckID が設定されていることを確認
	ackIDs := make([]string, len(pullResp.ReceivedMessages))
	for i, msg := range pullResp.ReceivedMessages {
		if msg.AckID == "" {
			t.Errorf("message[%d] has empty AckID", i)
		}
		ackIDs[i] = msg.AckID
	}

	// 5. Pull again: no messages (already pulled)
	resp, err = svc.HandleRequest(ctx, request(http.MethodPost, subPath+":pull", mustJSON(t, pullRequest{MaxMessages: 10})))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("second pull: status=%d err=%v body=%s", resp.StatusCode, err, resp.Body)
	}

	var pullResp2 pullResponse
	if err := json.Unmarshal(resp.Body, &pullResp2); err != nil {
		t.Fatalf("unmarshal second pull response: %v", err)
	}
	if len(pullResp2.ReceivedMessages) != 0 {
		t.Errorf("expected 0 messages on second pull, got %d", len(pullResp2.ReceivedMessages))
	}

	// 6. Acknowledge
	resp, err = svc.HandleRequest(ctx, request(http.MethodPost, subPath+":acknowledge", mustJSON(t, acknowledgeRequest{AckIDs: ackIDs})))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("acknowledge: status=%d err=%v body=%s", resp.StatusCode, err, resp.Body)
	}
}

// TestTopicDeleteCascadesSubscriptions は topic 削除時にサブスクリプションもカスケード削除されることをテストします。
func TestTopicDeleteCascadesSubscriptions(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := "my-project"
	topicPath := "projects/" + project + "/topics/cascade-topic"
	subPath := "projects/" + project + "/subscriptions/cascade-sub"
	fullTopicName := "projects/" + project + "/topics/cascade-topic"

	// CreateTopic
	resp, err := svc.HandleRequest(ctx, request(http.MethodPut, topicPath, nil))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create topic: %v %d", err, resp.StatusCode)
	}

	// CreateSubscription
	resp, err = svc.HandleRequest(ctx, request(http.MethodPut, subPath, mustJSON(t, createSubscriptionRequest{Topic: fullTopicName})))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create subscription: %v %d", err, resp.StatusCode)
	}

	// DeleteTopic
	resp, err = svc.HandleRequest(ctx, request(http.MethodDelete, topicPath, nil))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("delete topic: %v %d", err, resp.StatusCode)
	}

	// GetSubscription: 404 (cascade deleted)
	resp, err = svc.HandleRequest(ctx, request(http.MethodGet, subPath, nil))
	if err != nil {
		t.Fatalf("get subscription error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for cascade-deleted subscription, got %d", resp.StatusCode)
	}
}

// TestPullEmptySubscription はメッセージがない場合に空レスポンスを返すことをテストします。
func TestPullEmptySubscription(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := "my-project"
	topicPath := "projects/" + project + "/topics/empty-topic"
	subPath := "projects/" + project + "/subscriptions/empty-sub"
	fullTopicName := "projects/" + project + "/topics/empty-topic"

	// CreateTopic + CreateSubscription
	svc.HandleRequest(ctx, request(http.MethodPut, topicPath, nil))    //nolint:errcheck
	svc.HandleRequest(ctx, request(http.MethodPut, subPath, mustJSON(t, createSubscriptionRequest{Topic: fullTopicName}))) //nolint:errcheck

	// Pull: empty
	resp, err := svc.HandleRequest(ctx, request(http.MethodPost, subPath+":pull", mustJSON(t, pullRequest{MaxMessages: 10})))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("pull: status=%d err=%v", resp.StatusCode, err)
	}

	var pullResp pullResponse
	if err := json.Unmarshal(resp.Body, &pullResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(pullResp.ReceivedMessages) != 0 {
		t.Errorf("expected empty pull, got %d messages", len(pullResp.ReceivedMessages))
	}
}

// TestAcknowledgeInvalidAckID は不正な ackId を渡しても 200 を返すことをテストします。
func TestAcknowledgeInvalidAckID(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := "my-project"
	topicPath := "projects/" + project + "/topics/ack-topic"
	subPath := "projects/" + project + "/subscriptions/ack-sub"
	fullTopicName := "projects/" + project + "/topics/ack-topic"

	svc.HandleRequest(ctx, request(http.MethodPut, topicPath, nil))    //nolint:errcheck
	svc.HandleRequest(ctx, request(http.MethodPut, subPath, mustJSON(t, createSubscriptionRequest{Topic: fullTopicName}))) //nolint:errcheck

	// 不正な ackId で acknowledge: 200 を返す
	resp, err := svc.HandleRequest(ctx, request(http.MethodPost, subPath+":acknowledge", mustJSON(t, acknowledgeRequest{
		AckIDs: []string{"invalid-ack-id-123"},
	})))
	if err != nil {
		t.Fatalf("acknowledge error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for invalid ackId, got %d: %s", resp.StatusCode, resp.Body)
	}
}

// TestTopicValidation は topic 名バリデーションをテストします。
func TestTopicValidation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// 短すぎる名前
	resp, err := svc.HandleRequest(ctx, request(http.MethodPut, "projects/my-project/topics/ab", nil))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Error("expected error for short topic name, got 200")
	}

	// 有効な名前
	resp, err = svc.HandleRequest(ctx, request(http.MethodPut, "projects/my-project/topics/valid-topic-name", nil))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for valid topic name, got %d: %s", resp.StatusCode, resp.Body)
	}
}

// TestPublishMessageSizeLimit はメッセージサイズ上限をテストします。
func TestPublishMessageSizeLimit(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	topicPath := "projects/p/topics/size-topic"
	svc.HandleRequest(ctx, request(http.MethodPut, topicPath, nil)) //nolint:errcheck

	// 10MB + 1 byte のメッセージ
	bigData := make([]byte, maxMessageSize+1)
	resp, err := svc.HandleRequest(ctx, request(http.MethodPost, topicPath+":publish", mustJSON(t, publishRequest{
		Messages: []pubsubMessage{{Data: bigData}},
	})))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Error("expected error for oversized message, got 200")
	}
}

// TestPullMaxMessagesValidation は pull の maxMessages バリデーションをテストします。
func TestPullMaxMessagesValidation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	topicPath := "projects/p/topics/max-topic"
	subPath := "projects/p/subscriptions/max-sub"
	svc.HandleRequest(ctx, request(http.MethodPut, topicPath, nil)) //nolint:errcheck
	svc.HandleRequest(ctx, request(http.MethodPut, subPath, mustJSON(t, createSubscriptionRequest{Topic: "projects/p/topics/max-topic"}))) //nolint:errcheck

	// maxMessages = 0 はデフォルト値 (100) を使うため OK
	resp, err := svc.HandleRequest(ctx, request(http.MethodPost, subPath+":pull", mustJSON(t, pullRequest{MaxMessages: 0})))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("pull with maxMessages=0: status=%d err=%v", resp.StatusCode, err)
	}

	// maxMessages = 1001 はエラー
	resp, err = svc.HandleRequest(ctx, request(http.MethodPost, subPath+":pull", mustJSON(t, pullRequest{MaxMessages: 1001})))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Error("expected error for maxMessages=1001, got 200")
	}
}

// TestPubSubServiceInterface はサービスインターフェースの実装を確認します。
func TestPubSubServiceInterface(t *testing.T) {
	svc := newTestService(t)

	if svc.Name() != "pubsub" {
		t.Errorf("Name()=%q want %q", svc.Name(), "pubsub")
	}
	if svc.Provider() != "gcp" {
		t.Errorf("Provider()=%q want %q", svc.Provider(), "gcp")
	}

	h := svc.Health(context.Background())
	if !h.Healthy {
		t.Errorf("Health().Healthy=false want true")
	}

	if err := svc.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}

// TestFanOut は複数のサブスクリプションへの fan-out をテストします。
func TestFanOut(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := "fanout-project"
	topicPath := "projects/" + project + "/topics/fanout-topic"
	subPath1 := "projects/" + project + "/subscriptions/fanout-sub-1"
	subPath2 := "projects/" + project + "/subscriptions/fanout-sub-2"
	fullTopicName := "projects/" + project + "/topics/fanout-topic"

	// Setup
	svc.HandleRequest(ctx, request(http.MethodPut, topicPath, nil)) //nolint:errcheck
	svc.HandleRequest(ctx, request(http.MethodPut, subPath1, mustJSON(t, createSubscriptionRequest{Topic: fullTopicName}))) //nolint:errcheck
	svc.HandleRequest(ctx, request(http.MethodPut, subPath2, mustJSON(t, createSubscriptionRequest{Topic: fullTopicName}))) //nolint:errcheck

	// Publish 1 message
	svc.HandleRequest(ctx, request(http.MethodPost, topicPath+":publish", mustJSON(t, publishRequest{ //nolint:errcheck
		Messages: []pubsubMessage{{Data: []byte("broadcast")}},
	})))

	// Both subscriptions should receive the message
	for _, subPath := range []string{subPath1, subPath2} {
		resp, err := svc.HandleRequest(ctx, request(http.MethodPost, subPath+":pull", mustJSON(t, pullRequest{MaxMessages: 10})))
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("pull from %s: status=%d err=%v", subPath, resp.StatusCode, err)
		}

		var pullResp pullResponse
		if err := json.Unmarshal(resp.Body, &pullResp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(pullResp.ReceivedMessages) != 1 {
			t.Errorf("subscription %s: expected 1 message, got %d", subPath, len(pullResp.ReceivedMessages))
		}
	}
}
