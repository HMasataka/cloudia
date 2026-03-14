//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

// TestE2E_SQS_BasicOperations はSQSの基本的なキュー操作をテストします。
func TestE2E_SQS_BasicOperations(t *testing.T) {
	skipIfToolNotFound(t, "aws")

	t.Cleanup(func() { cleanupOrphans(t) })

	const queueName = "e2e-test-queue"

	// キュー作成
	t.Run("CreateQueue", func(t *testing.T) {
		out := runAWSCLI(t, "sqs", "create-queue",
			"--queue-name", queueName,
			"--output", "text",
			"--query", "QueueUrl",
		)
		queueURL := strings.TrimSpace(out)
		if queueURL == "" {
			t.Fatal("CreateQueue: expected queue URL in output")
		}
		t.Logf("created queue: %s", queueURL)
	})

	// キューURL取得
	var queueURL string
	t.Run("GetQueueUrl", func(t *testing.T) {
		out := runAWSCLI(t, "sqs", "get-queue-url",
			"--queue-name", queueName,
			"--output", "text",
			"--query", "QueueUrl",
		)
		queueURL = strings.TrimSpace(out)
		if queueURL == "" {
			t.Fatal("GetQueueUrl: expected queue URL in output")
		}
	})

	// キュー一覧
	t.Run("ListQueues", func(t *testing.T) {
		out := runAWSCLI(t, "sqs", "list-queues",
			"--output", "text",
		)
		if !strings.Contains(out, queueName) {
			t.Errorf("ListQueues: expected %q in output, got: %s", queueName, out)
		}
	})

	// キュー属性取得
	t.Run("GetQueueAttributes", func(t *testing.T) {
		if queueURL == "" {
			t.Skip("no queue URL available")
		}
		out := runAWSCLI(t, "sqs", "get-queue-attributes",
			"--queue-url", queueURL,
			"--attribute-names", "All",
		)
		if !strings.Contains(out, "QueueArn") {
			t.Errorf("GetQueueAttributes: expected QueueArn in output, got: %s", out)
		}
	})

	// キュー削除
	t.Run("DeleteQueue", func(t *testing.T) {
		if queueURL == "" {
			t.Skip("no queue URL available")
		}
		runAWSCLI(t, "sqs", "delete-queue", "--queue-url", queueURL)
	})

	t.Cleanup(func() {
		if queueURL != "" {
			runAWSCLIIgnoreError("sqs", "delete-queue", "--queue-url", queueURL)
		}
	})
}
