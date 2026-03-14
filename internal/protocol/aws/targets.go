package aws

// TargetPrefixToService は X-Amz-Target ヘッダーのプレフィックスをサービス名にマッピングします。
// 例: "DynamoDB_20120810.PutItem" のプレフィックス "DynamoDB_20120810" → "dynamodb"
var TargetPrefixToService = map[string]string{
	"DynamoDB_20120810": "dynamodb",
	"AmazonSQS":         "sqs",
	"Kinesis_20131202":  "kinesis",
}
