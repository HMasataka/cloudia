package sqs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

func newTestService(t *testing.T) *SQSService {
	t.Helper()
	store := state.NewMemoryStore()
	svc := NewSQSService(config.AWSAuthConfig{
		AccountID: "000000000000",
		Region:    "us-east-1",
	}, zap.NewNop())
	if err := svc.Init(context.Background(), service.ServiceDeps{Store: store}); err != nil {
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

// TestCreateQueue_GetQueueUrl_RoundTrip は CreateQueue + GetQueueUrl のラウンドトリップをテストします。
func TestCreateQueue_GetQueueUrl_RoundTrip(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// CreateQueue
	createResp, err := svc.HandleRequest(ctx, service.Request{
		Action: "CreateQueue",
		Body:   mustJSON(t, map[string]string{"QueueName": "my-queue"}),
	})
	if err != nil {
		t.Fatalf("CreateQueue error: %v", err)
	}
	if createResp.StatusCode != 200 {
		t.Fatalf("CreateQueue status=%d body=%s", createResp.StatusCode, createResp.Body)
	}

	var createResult createQueueResponse
	if err := json.Unmarshal(createResp.Body, &createResult); err != nil {
		t.Fatalf("unmarshal CreateQueue response: %v", err)
	}
	expectedURL := "http://localhost:4566/000000000000/my-queue"
	if createResult.QueueUrl != expectedURL {
		t.Errorf("QueueUrl=%q want %q", createResult.QueueUrl, expectedURL)
	}

	// GetQueueUrl
	getResp, err := svc.HandleRequest(ctx, service.Request{
		Action: "GetQueueUrl",
		Body:   mustJSON(t, map[string]string{"QueueName": "my-queue"}),
	})
	if err != nil {
		t.Fatalf("GetQueueUrl error: %v", err)
	}
	if getResp.StatusCode != 200 {
		t.Fatalf("GetQueueUrl status=%d body=%s", getResp.StatusCode, getResp.Body)
	}

	var getResult getQueueUrlResponse
	if err := json.Unmarshal(getResp.Body, &getResult); err != nil {
		t.Fatalf("unmarshal GetQueueUrl response: %v", err)
	}
	if getResult.QueueUrl != expectedURL {
		t.Errorf("GetQueueUrl QueueUrl=%q want %q", getResult.QueueUrl, expectedURL)
	}
}

// TestCreateQueue_FIFOValidation は FIFO キュー名バリデーションをテストします。
func TestCreateQueue_FIFOValidation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// FifoQueue=true なのに .fifo で終わらない場合はエラー
	resp, err := svc.HandleRequest(ctx, service.Request{
		Action: "CreateQueue",
		Body:   mustJSON(t, map[string]any{"QueueName": "my-queue", "Attributes": map[string]string{"FifoQueue": "true"}}),
	})
	if err != nil {
		t.Fatalf("CreateQueue error: %v", err)
	}
	if resp.StatusCode == 200 {
		t.Error("expected error for FIFO queue without .fifo suffix, got 200")
	}

	var errResp sqsError
	if err := json.Unmarshal(resp.Body, &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if errResp.Code != "InvalidParameterValue" {
		t.Errorf("error code=%q want InvalidParameterValue", errResp.Code)
	}

	// .fifo で終わる場合は成功
	resp2, err := svc.HandleRequest(ctx, service.Request{
		Action: "CreateQueue",
		Body:   mustJSON(t, map[string]any{"QueueName": "my-queue.fifo", "Attributes": map[string]string{"FifoQueue": "true"}}),
	})
	if err != nil {
		t.Fatalf("CreateQueue .fifo error: %v", err)
	}
	if resp2.StatusCode != 200 {
		t.Errorf("expected 200 for .fifo queue, got %d: %s", resp2.StatusCode, resp2.Body)
	}
}

// TestCreateQueue_AlreadyExists は同名キュー作成時の QueueAlreadyExists をテストします。
func TestCreateQueue_AlreadyExists(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	body := mustJSON(t, map[string]string{"QueueName": "dup-queue"})

	// 1回目は成功
	resp1, err := svc.HandleRequest(ctx, service.Request{Action: "CreateQueue", Body: body})
	if err != nil {
		t.Fatalf("first CreateQueue error: %v", err)
	}
	if resp1.StatusCode != 200 {
		t.Fatalf("first CreateQueue status=%d", resp1.StatusCode)
	}

	// 2回目は QueueAlreadyExists
	resp2, err := svc.HandleRequest(ctx, service.Request{Action: "CreateQueue", Body: body})
	if err != nil {
		t.Fatalf("second CreateQueue error: %v", err)
	}
	if resp2.StatusCode == 200 {
		t.Error("expected error on second CreateQueue with same name")
	}

	var errResp sqsError
	if err := json.Unmarshal(resp2.Body, &errResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if errResp.Code != "QueueAlreadyExists" {
		t.Errorf("error code=%q want QueueAlreadyExists", errResp.Code)
	}
}

// TestGetQueueAttributes_Defaults は GetQueueAttributes のデフォルト値を確認します。
func TestGetQueueAttributes_Defaults(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// まずキューを作成
	_, err := svc.HandleRequest(ctx, service.Request{
		Action: "CreateQueue",
		Body:   mustJSON(t, map[string]string{"QueueName": "attr-queue"}),
	})
	if err != nil {
		t.Fatalf("CreateQueue error: %v", err)
	}

	queueURL := "http://localhost:4566/000000000000/attr-queue"

	resp, err := svc.HandleRequest(ctx, service.Request{
		Action: "GetQueueAttributes",
		Body:   mustJSON(t, map[string]any{"QueueUrl": queueURL, "AttributeNames": []string{"All"}}),
	})
	if err != nil {
		t.Fatalf("GetQueueAttributes error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("GetQueueAttributes status=%d body=%s", resp.StatusCode, resp.Body)
	}

	var result getQueueAttributesResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := map[string]string{
		"ApproximateNumberOfMessages": "0",
		"VisibilityTimeout":           "30",
		"MaximumMessageSize":          "262144",
		"MessageRetentionPeriod":      "345600",
	}
	for k, want := range checks {
		if got := result.Attributes[k]; got != want {
			t.Errorf("Attributes[%q]=%q want %q", k, got, want)
		}
	}

	// QueueArn の形式確認
	expectedArn := "arn:aws:sqs:us-east-1:000000000000:attr-queue"
	if got := result.Attributes["QueueArn"]; got != expectedArn {
		t.Errorf("QueueArn=%q want %q", got, expectedArn)
	}

	// CreatedTimestamp が存在すること
	if result.Attributes["CreatedTimestamp"] == "" {
		t.Error("CreatedTimestamp should not be empty")
	}
}
