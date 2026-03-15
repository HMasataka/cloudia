#!/bin/bash
# Cloudia + gcloud / gsutil の使用例
# 事前に cloudia start を実行しておくこと

ENDPOINT="http://localhost:4566"
PROJECT="cloudia-local"

echo "=== Cloud Storage (gsutil) ==="
gsutil -o "Credentials:gs_json_host=$ENDPOINT" mb gs://example-bucket
gsutil -o "Credentials:gs_json_host=$ENDPOINT" ls

echo ""
echo "=== Cloud Storage (REST API) ==="
# バケット作成
curl -s -X POST "$ENDPOINT/storage/v1/b?project=$PROJECT" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-token" \
  -d '{"name": "rest-example-bucket"}' | python3 -m json.tool

# バケット一覧
curl -s "$ENDPOINT/storage/v1/b?project=$PROJECT" \
  -H "Authorization: Bearer test-token" | python3 -m json.tool

echo ""
echo "=== Compute Engine ==="
# インスタンス作成
curl -s -X POST "$ENDPOINT/compute/v1/projects/$PROJECT/zones/us-central1-a/instances" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-token" \
  -d '{
    "name": "example-instance",
    "machineType": "zones/us-central1-a/machineTypes/e2-micro",
    "disks": [{"initializeParams": {"sourceImage": "projects/debian-cloud/global/images/debian-11"}, "boot": true}],
    "networkInterfaces": [{"network": "global/networks/default"}]
  }' | python3 -m json.tool

# インスタンス一覧
curl -s "$ENDPOINT/compute/v1/projects/$PROJECT/zones/us-central1-a/instances" \
  -H "Authorization: Bearer test-token" | python3 -m json.tool

echo ""
echo "=== Pub/Sub ==="
# トピック作成
curl -s -X PUT "$ENDPOINT/v1/projects/$PROJECT/topics/example-topic" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-token" | python3 -m json.tool

# サブスクリプション作成
curl -s -X PUT "$ENDPOINT/v1/projects/$PROJECT/subscriptions/example-sub" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-token" \
  -d "{\"topic\": \"projects/$PROJECT/topics/example-topic\"}" | python3 -m json.tool

# メッセージ送信
curl -s -X POST "$ENDPOINT/v1/projects/$PROJECT/topics/example-topic:publish" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-token" \
  -d '{"messages": [{"data": "aGVsbG8gY2xvdWRpYQ=="}]}' | python3 -m json.tool

# メッセージ受信
curl -s -X POST "$ENDPOINT/v1/projects/$PROJECT/subscriptions/example-sub:pull" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-token" \
  -d '{"maxMessages": 10}' | python3 -m json.tool

echo ""
echo "=== Cloud SQL ==="
curl -s -X POST "$ENDPOINT/v1/projects/$PROJECT/instances" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-token" \
  -d '{
    "name": "example-db",
    "databaseVersion": "MYSQL_8_0",
    "region": "us-central1",
    "settings": {"tier": "db-f1-micro"},
    "rootPassword": "password1234"
  }' | python3 -m json.tool

echo ""
echo "=== ヘルスチェック ==="
curl -s $ENDPOINT/health | python3 -m json.tool
