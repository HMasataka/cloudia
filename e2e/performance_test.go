//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

const (
	warmupRequests  = 3
	measureRequests = 10
	p95Threshold    = 500 * time.Millisecond
)

// performanceCase はパフォーマンステストのケースを表します。
type performanceCase struct {
	name    string
	fn      func(t *testing.T)
	needsDocker bool
}

// TestE2E_Performance_ReadAPIs は各サービスの参照系APIのP95レイテンシをテストします。
// ウォームアップ3リクエスト後に10リクエストを送信し、P95が500ms以内であることを確認します。
func TestE2E_Performance_ReadAPIs(t *testing.T) {
	cases := []performanceCase{
		{
			name:        "S3_ListBuckets",
			fn:          perfS3ListBuckets,
			needsDocker: true,
		},
		{
			name:        "EC2_DescribeInstances",
			fn:          perfEC2DescribeInstances,
			needsDocker: true,
		},
		{
			name:        "SQS_ListQueues",
			fn:          perfSQSListQueues,
			needsDocker: false,
		},
		{
			name:        "DynamoDB_ListTables",
			fn:          perfDynamoDBListTables,
			needsDocker: true,
		},
		{
			name:        "GCS_ListBuckets",
			fn:          perfGCSListBuckets,
			needsDocker: true,
		},
		{
			name:        "GCE_ListInstances",
			fn:          perfGCEListInstances,
			needsDocker: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.needsDocker {
				skipIfDockerNotAvailable(t)
			}

			// ウォームアップ
			for i := 0; i < warmupRequests; i++ {
				tc.fn(t)
			}

			// 計測
			latencies := make([]time.Duration, 0, measureRequests)
			for i := 0; i < measureRequests; i++ {
				d := measureLatency(t, func() {
					tc.fn(t)
				})
				latencies = append(latencies, d)
			}

			sortDurations(latencies)
			p95 := percentile(latencies, 0.95)

			t.Logf("%s: P95 latency = %v (threshold: %v)", tc.name, p95, p95Threshold)
			t.Logf("%s: min=%v, max=%v", tc.name, latencies[0], latencies[len(latencies)-1])

			if p95 > p95Threshold {
				t.Errorf("%s: P95 latency %v exceeds threshold %v", tc.name, p95, p95Threshold)
			}
		})
	}
}

// perfS3ListBuckets はS3バケット一覧APIのパフォーマンスを計測します。
func perfS3ListBuckets(t *testing.T) {
	t.Helper()
	url := globalServer.baseURL + "/"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("create S3 list request: %v", err)
	}
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=test/20240101/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=test")
	req.Header.Set("X-Amz-Date", "20240101T000000Z")
	req.Header.Set("Host", "s3.amazonaws.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("S3 list request: %v", err)
	}
	resp.Body.Close()
}

// perfEC2DescribeInstances はEC2インスタンス一覧APIのパフォーマンスを計測します。
func perfEC2DescribeInstances(t *testing.T) {
	t.Helper()
	url := fmt.Sprintf("%s/?Action=DescribeInstances&Version=2016-11-15", globalServer.baseURL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewBufferString(""))
	if err != nil {
		t.Fatalf("create EC2 describe request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=test/20240101/us-east-1/ec2/aws4_request, SignedHeaders=content-type;host;x-amz-date, Signature=test")
	req.Header.Set("X-Amz-Date", "20240101T000000Z")
	req.Header.Set("X-Amz-Target", "")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("EC2 describe request: %v", err)
	}
	resp.Body.Close()
}

// perfSQSListQueues はSQSキュー一覧APIのパフォーマンスを計測します。
func perfSQSListQueues(t *testing.T) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{})
	url := globalServer.baseURL + "/"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create SQS list request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonSQS.ListQueues")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=test/20240101/us-east-1/sqs/aws4_request, SignedHeaders=content-type;host;x-amz-date;x-amz-target, Signature=test")
	req.Header.Set("X-Amz-Date", "20240101T000000Z")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SQS list request: %v", err)
	}
	resp.Body.Close()
}

// perfDynamoDBListTables はDynamoDBテーブル一覧APIのパフォーマンスを計測します。
func perfDynamoDBListTables(t *testing.T) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{})
	url := globalServer.baseURL + "/"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create DynamoDB list request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.ListTables")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=test/20240101/us-east-1/dynamodb/aws4_request, SignedHeaders=content-type;host;x-amz-date;x-amz-target, Signature=test")
	req.Header.Set("X-Amz-Date", "20240101T000000Z")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DynamoDB list request: %v", err)
	}
	resp.Body.Close()
}

// perfGCSListBuckets はGCSバケット一覧APIのパフォーマンスを計測します。
func perfGCSListBuckets(t *testing.T) {
	t.Helper()
	url := fmt.Sprintf("%s/storage/v1/b?project=cloudia-local", globalServer.baseURL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("create GCS list request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GCS list request: %v", err)
	}
	resp.Body.Close()
}

// perfGCEListInstances はGCEインスタンス一覧APIのパフォーマンスを計測します。
func perfGCEListInstances(t *testing.T) {
	t.Helper()
	url := fmt.Sprintf("%s/compute/v1/projects/cloudia-local/zones/us-central1-a/instances", globalServer.baseURL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("create GCE list request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GCE list request: %v", err)
	}
	resp.Body.Close()
}
