# Cloudia

ローカルで動くマルチクラウドエミュレータ。AWS や GCP の API をローカル環境で再現し、実際の Docker コンテナ（MinIO, Redis, MySQL, k3s など）を起動してレスポンスを返します。

## 特徴

- **実コンテナベース**: ダミーレスポンスではなく、実際の Docker コンテナでバックエンドを動かす
- **マルチクラウド対応**: AWS と GCP の両方のプロトコルに対応
- **シングルバイナリ**: Go 製。Node.js やビルドツール不要
- **統一エンドポイント**: `localhost:4566` で全サービスにアクセス
- **Terraform 互換**: AWS / Google Cloud Provider でそのまま使える

## アーキテクチャ

```
CLI → Gateway → Auth (SigV4/OAuth) → Protocol (XML/JSON) → Service → Backend → Docker
                                                                        ↓
                                                                MinIO / Redis / MySQL / k3s
```

## 対応サービス

| AWS | GCP | バックエンド |
|-----|-----|-------------|
| S3 | Cloud Storage | MinIO |
| EC2 | Compute Engine | Docker |
| DynamoDB | - | Docker (DynamoDB Local) |
| Lambda | - | Docker |
| RDS (MySQL/PostgreSQL) | Cloud SQL | MySQL / PostgreSQL |
| ElastiCache | Memorystore | Redis |
| SQS | Pub/Sub | インメモリ |
| EKS | GKE | k3s |
| IAM | - | インメモリ |
| VPC / SecurityGroup | - | インメモリ |

## 必要要件

- Go 1.25+
- Docker

## インストール

```bash
go install github.com/HMasataka/cloudia/cmd/cloudia@latest
```

またはソースからビルド:

```bash
git clone https://github.com/HMasataka/cloudia.git
cd cloudia
go build -o cloudia ./cmd/cloudia/
```

## 使い方

### サーバー起動

```bash
cloudia start
```

オプション:

```bash
cloudia start --config ./myconfig.yaml   # カスタム設定ファイル
cloudia start --log-level debug          # ログレベル変更
```

### AWS CLI

```bash
aws s3 ls --endpoint-url http://localhost:4566
aws s3 mb s3://my-bucket --endpoint-url http://localhost:4566
aws ec2 run-instances --image-id ami-test --instance-type t2.micro --endpoint-url http://localhost:4566
```

### Terraform (AWS)

```hcl
provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    s3  = "http://localhost:4566"
    ec2 = "http://localhost:4566"
    # ...
  }
}
```

### Terraform (GCP)

```hcl
provider "google" {
  project = "test-project"
  region  = "us-central1"
}
```

### 管理画面

ブラウザで `http://localhost:4566/admin/ui` にアクセス。

- ダッシュボード: サービス一覧とヘルスステータス
- リソースブラウザ: リソースの一覧・詳細・削除
- コンテナビュー: 起動中コンテナとログ表示
- 設定ビュー: 現在の設定確認

### その他のコマンド

```bash
cloudia status    # ヘルスチェック
cloudia stop      # サーバー停止
cloudia cleanup   # 管理下 Docker リソースの全削除
```

## 設定

デフォルト設定 (`configs/default.yaml`):

```yaml
server:
  host: "127.0.0.1"
  port: 4566
  timeout: 30s

limits:
  max_containers: 20
  default_cpu: "1"
  default_memory: "512m"
  storage_quota: "10g"

docker:
  network_name: "cloudia"

metrics:
  enabled: false
  port: 9090

auth:
  aws:
    access_key: "test"
    secret_key: "test"
```

## テスト

```bash
go test ./...              # ユニットテスト
go test -tags=e2e ./e2e/   # E2E テスト (Docker 必要)
```

## ライセンス

MIT
