#!/bin/bash
# Cloudia + AWS CLI の使用例
# 事前に cloudia start を実行しておくこと

ENDPOINT="http://localhost:4566"
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1

echo "=== S3 ==="
aws s3 mb s3://example-bucket --endpoint-url $ENDPOINT
aws s3 ls --endpoint-url $ENDPOINT
echo "hello cloudia" | aws s3 cp - s3://example-bucket/hello.txt --endpoint-url $ENDPOINT
aws s3 cp s3://example-bucket/hello.txt - --endpoint-url $ENDPOINT

echo ""
echo "=== EC2 ==="
aws ec2 run-instances \
  --image-id ami-example \
  --instance-type t2.micro \
  --endpoint-url $ENDPOINT

aws ec2 describe-instances --endpoint-url $ENDPOINT

echo ""
echo "=== SQS ==="
aws sqs create-queue --queue-name example-queue --endpoint-url $ENDPOINT
QUEUE_URL=$(aws sqs list-queues --endpoint-url $ENDPOINT --query 'QueueUrls[0]' --output text)
aws sqs send-message --queue-url "$QUEUE_URL" --message-body "hello" --endpoint-url $ENDPOINT
aws sqs receive-message --queue-url "$QUEUE_URL" --endpoint-url $ENDPOINT

echo ""
echo "=== DynamoDB ==="
aws dynamodb create-table \
  --table-name example-table \
  --attribute-definitions AttributeName=id,AttributeType=S \
  --key-schema AttributeName=id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url $ENDPOINT

aws dynamodb put-item \
  --table-name example-table \
  --item '{"id":{"S":"1"},"name":{"S":"cloudia"}}' \
  --endpoint-url $ENDPOINT

aws dynamodb get-item \
  --table-name example-table \
  --key '{"id":{"S":"1"}}' \
  --endpoint-url $ENDPOINT

echo ""
echo "=== IAM ==="
aws iam create-role \
  --role-name example-role \
  --assume-role-policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}' \
  --endpoint-url $ENDPOINT

aws iam list-roles --endpoint-url $ENDPOINT

echo ""
echo "=== RDS ==="
aws rds create-db-instance \
  --db-instance-identifier example-db \
  --db-instance-class db.t3.micro \
  --engine mysql \
  --master-username admin \
  --master-user-password password1234 \
  --endpoint-url $ENDPOINT

aws rds describe-db-instances --endpoint-url $ENDPOINT

echo ""
echo "=== ヘルスチェック ==="
curl -s $ENDPOINT/health | python3 -m json.tool

echo ""
echo "=== 管理画面 ==="
echo "ブラウザで $ENDPOINT/admin/ui にアクセス"
