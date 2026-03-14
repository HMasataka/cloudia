# ロードマップ: Cloudia - マルチクラウドローカルエミュレータ

## アーキテクチャ方針

- **7 層レイヤー構成**: CLI / Gateway / Auth / Protocol / Service / Backend / Infrastructure。依存方向は上→下の一方向のみ
- **プロバイダー抽象化**: AWS と GCP のプロトコル差異（SigV4 vs OAuth、XML vs JSON）は Auth 層と Protocol 層で吸収。Service 層には正規化された Request を渡す。Provider の明示的な抽象インターフェースは作らない（リーキーアブストラクション回避）
- **バックエンド共有**: AWS S3 と GCP Cloud Storage は同じ MinIO、ElastiCache と Memorystore は同じ Redis を共有しリソース効率を最大化
- **プラグインアーキテクチャ**: `Service` インターフェース（Name/Provider/Init/HandleRequest/SupportedActions/Health/Shutdown）を実装し Registry に登録するだけで新サービスを追加可能
- **実装言語**: Go。Docker SDK for Go を直接利用。シングルバイナリ配布

```
CLI → Gateway → Auth (SigV4/OAuth) → Protocol (XML/JSON変換) → Service → Backend → Docker SDK
                                                                          ↓
                                                                  MinIO / Redis / MySQL / k3s ...
```

---

## Phase 1: コア基盤 + 全サービス実装

---

## v0.1 - プロジェクト基盤とスケルトン

**ゴール**: Go プロジェクト初期化、設定管理、ログ、Docker クライアントラッパーが動作する。`cloudia start` で HTTP サーバーが起動し `/health` に 200 を返す
**完動品としての価値**: 開発者が `cloudia start` を実行してサーバーが起動し、ヘルスチェックで生存確認できる。後続マイルストーンの土台

- [x] Go モジュール初期化（既存リポジトリ内で `go mod init`）とディレクトリ構成作成（`cmd/cloudia/`, `internal/cli/`, `internal/config/`, `internal/gateway/`, `internal/gateway/middleware/`, `internal/auth/`, `internal/protocol/`, `internal/service/`, `internal/backend/`, `internal/state/`, `internal/resource/`, `internal/network/`, `internal/admin/`, `pkg/models/`, `configs/`）
- [x] `cmd/cloudia/main.go`: cobra root コマンドのエントリポイント
- [x] 設定管理 (`internal/config/config.go`, `defaults.go`): viper による YAML/環境変数/フラグ統合。ServerConfig, LimitsConfig, DockerConfig, StateConfig, CleanupConfig, LoggingConfig, MetricsConfig, AWSConfig, GCPConfig 構造体定義
- [x] `configs/default.yaml`: デフォルト設定ファイル（host: 127.0.0.1, port: 4566, max_containers: 20 等）
- [x] ロギング: zap ロガー初期化（設定の logging.level, logging.format に基づく）
- [x] Docker クライアントラッパー (`internal/backend/docker/client.go`): NewClient(), Close(), Ping()
- [x] Docker コンテナ操作 (`internal/backend/docker/container.go`): RunContainer(), StopContainer(), ListManagedContainers()
- [x] Docker ネットワーク・ボリューム・イメージ管理 (`network.go`, `volume.go`, `image.go`)
- [x] Docker ラベル管理 (`labels.go`): `cloudia.managed=true` ラベル付与
- [x] Docker イベント監視 (`events.go`): WatchEvents() でコンテナ状態変更をコールバック通知
- [x] Docker 孤立リソース削除 (`CleanupOrphans()`)
- [x] CLI `start` コマンド (`internal/cli/start.go`): 設定読み込み → ロガー初期化 → Docker 接続確認（未起動時は明確なエラーメッセージ） → HTTP サーバー起動 → SIGINT/SIGTERM で graceful shutdown
- [x] CLI `stop` コマンド (`internal/cli/stop.go`): PID ファイル経由で停止シグナル送信
- [x] CLI `status` コマンド (`internal/cli/status.go`): ヘルスチェック API への問い合わせ
- [x] CLI `cleanup` コマンド (`internal/cli/cleanup.go`): 管理下 Docker リソースの全削除
- [x] Gateway 基本構造 (`internal/gateway/server.go`): net/http サーバー起動・停止、localhost のみバインド（127.0.0.1）
- [x] Gateway ルーター (`internal/gateway/router.go`): detectProvider() のスタブ、未対応リクエストへのエラーレスポンス
- [x] ミドルウェア (`internal/gateway/middleware/`): logging.go（アクセスログ）, recovery.go（パニックリカバリ）, timeout.go（リクエストタイムアウト）
- [x] 管理 API (`internal/admin/health.go`): GET /health → `{"status": "ok"}`
- [x] 管理 API (`internal/admin/admin.go`): GET /admin/services → 登録済みサービス一覧（空配列）
- [x] 共通モデル (`pkg/models/resource.go`): Resource 基底型（Kind, ID, Provider, Service, Region, Tags, Spec, Status, CreatedAt, UpdatedAt, ContainerID, TTL）
- [x] 共通エラー (`pkg/models/errors.go`): ErrNotFound, ErrAlreadyExists, ErrLimitExceeded, ErrServiceUnavailable, ErrUnsupportedOperation
- [x] テスト: `cloudia start` → `/health` に 200 が返る → `cloudia stop` で正常終了する統合テスト

---

## v0.2 - State Store とリソース管理基盤

**ゴール**: インメモリ State Store、リソースロック、リソース制限、ポート管理が動作する。Service インターフェースとレジストリの基盤を確立する
**完動品としての価値**: サービス実装の受け皿が完成し、リソースの CRUD・排他制御・上限管理がテスト可能

- [ ] State Store インターフェース (`internal/state/store.go`): Get(kind, id), List(kind, filter), Put(resource), Delete(kind, id), Lock(kind, id), Snapshot(path), Restore(path)
- [ ] インメモリ実装 (`internal/state/memory.go`): sync.RWMutex + map ベース
- [ ] ファイル永続化実装 (`internal/state/file.go`): JSON シリアライズ、アトミック書き込み
- [ ] リソースロック (`internal/state/lock.go`): LockManager（リソース単位の排他ロック、コンテキストキャンセルでタイムアウト）
- [ ] Reconciler (`internal/state/reconciler.go`): State ↔ Docker 実態の定期照合（State にあるがコンテナない→terminated、コンテナあるが State にない→orphan 追加）
- [ ] リソース制限 (`internal/resource/limiter.go`): コンテナ数上限（デフォルト 20）、CPU/メモリ制限チェック、上限到達時の明確なエラー
- [ ] ポート管理 (`internal/resource/port.go`): エフェメラルポート範囲から動的割り当て、ポート衝突検出、衝突時の代替ポートフォールバック
- [ ] クリーンアップ (`internal/resource/cleanup.go`): 孤立リソース検出・削除
- [ ] TTL 管理 (`internal/resource/ttl.go`): バックグラウンドゴルーチンで TTL 期限切れリソースの自動クリーンアップ
- [ ] Service インターフェース (`internal/service/interface.go`): Service, ServiceDeps, Request, Response, HealthStatus 型定義
- [ ] サービスレジストリ (`internal/service/registry.go`): Register, Resolve, SharedBackend, InitAll, ShutdownAll, HealthAll
- [ ] バックエンドマッピング (`internal/backend/mapping/`): ami.go（AMI→Docker イメージ）, machine_type.go（インスタンスタイプ→リソース制限）, runtime.go（Lambda ランタイム→イメージ）
- [ ] テスト: State Store の CRUD ユニットテスト、ロック競合テスト、リソース上限到達テスト

---

## v0.3 - 認証とプロトコル変換

**ゴール**: SigV4 署名検証、GCP OAuth トークン検証、AWS/GCP プロトコル変換が動作する。Gateway がプロバイダを正しく検出し認証を通過させる
**完動品としての価値**: AWS CLI (`--endpoint-url`) や gcloud CLI から認証付きリクエストを送信でき、適切な UnsupportedOperation エラーが返る

- [ ] AWS SigV4 署名検証 (`internal/auth/sigv4/verifier.go`): AWS SigV4 仕様に完全準拠した署名検証（Canonical Request 生成、StringToSign 計算、署名照合）。ローカルモードでは固定 AccessKey/SecretKey（`test`/`test`）を受け入れる。リージョン・サービス名の抽出
- [ ] GCP OAuth トークン検証 (`internal/auth/gcp/verifier.go`): Bearer トークン検証。ローカルモードでは任意のトークンを受け入れ。project_id 抽出
- [ ] AWS XML エンコーダ (`internal/protocol/aws/xml.go`): 構造体→XML マーシャリング、AWS namespace 付与
- [ ] AWS Query パーサー (`internal/protocol/aws/query.go`): Action, Version 等の抽出、フラットパラメータの構造体変換
- [ ] AWS JSON プロトコル (`internal/protocol/aws/json.go`): X-Amz-Target / application/x-amz-json-1.0 対応（DynamoDB, SQS 等用）
- [ ] AWS エラーレスポンス (`internal/protocol/aws/error.go`): XML/JSON 両対応のエラー生成
- [ ] GCP JSON エンコーダ (`internal/protocol/gcp/json.go`): GCP REST JSON レスポンス
- [ ] GCP エラーレスポンス (`internal/protocol/gcp/error.go`): `{"error": {"code": N, "message": "...", "status": "..."}}` 形式
- [ ] Gateway ルーティング完成 (`internal/gateway/router.go`): Authorization ヘッダー（AWS4-HMAC-SHA256 vs Bearer）・Host ヘッダー・URL パスに基づくプロバイダ検出
- [ ] AWS ルーター: Host ヘッダー（S3 バーチャルホスト）、Query パラメータ（Action）、X-Amz-Target ヘッダー、パスベースでサービス・アクション解決。VPC は EC2 と同じ Action パラメータ空間から振り分け
- [ ] GCP ルーター: URL パスプレフィックスでサービス・アクション解決（/storage/v1/→Storage, /compute/v1/→Compute, /v1/projects/_/locations/_/clusters→GKE 等）
- [ ] 認証ミドルウェア組み込み（プロバイダに応じて SigV4 or OAuth を適用）
- [ ] 未サポート API への AWS 互換エラー（UnsupportedOperation XML）と GCP 互換エラー（501 UNIMPLEMENTED JSON）
- [ ] サービスごとの個別エンドポイントのサポート（設定でサービス別ポートを指定可能にする）
- [ ] テスト: AWS CLI で `--endpoint-url` 指定して認証通過→UnsupportedOperation が返るテスト、gcloud CLI 互換テスト

---

## v0.4 - S3 基本 CRUD (MinIO バックエンド)

**ゴール**: MinIO バックエンドを起動し、S3 の基本的なバケット/オブジェクト CRUD が動作する
**完動品としての価値**: `aws s3 mb s3://test --endpoint-url http://localhost:4566` でバケット作成、`aws s3 cp` でオブジェクトのアップロード/ダウンロードが動作。Terraform `aws_s3_bucket` が apply 可能。後続サービス実装のテンプレートパターンを確立

- [ ] MinIO バックエンド (`internal/backend/minio/backend.go`): MinIO コンテナの起動・停止・再利用、ヘルスチェック（readiness probe）、minio-go SDK 経由のバケット/オブジェクト操作
- [ ] AWS S3 サービス (`internal/service/aws/s3/service.go`): Service インターフェース実装、Init で MinIO 起動。後続サービスのテンプレートとなるパターンを確立する（エラーハンドリング、State 連携、ロック取得の標準的な流れ）
- [ ] S3 ハンドラ (`internal/service/aws/s3/handlers.go`): 各 API アクションのハンドラを実装 — CreateBucket, DeleteBucket, ListBuckets, HeadBucket, PutObject, GetObject, DeleteObject, ListObjectsV2, CopyObject, HeadObject
- [ ] S3 モデル (`internal/service/aws/s3/models.go`): Bucket, Object のリソースモデル
- [ ] 冪等性: バケット名重複時に BucketAlreadyExists エラー
- [ ] ネットワークプロキシ (`internal/network/proxy.go`): Gateway から MinIO コンテナへのリバースプロキシ
- [ ] サービスディスカバリ (`internal/network/discovery.go`): コンテナの公開ポートを動的に解決
- [ ] テスト: `aws s3 mb`, `aws s3 cp`, `aws s3 ls`, `aws s3 rm` の統合テスト。Terraform `aws_s3_bucket` の apply/destroy テスト。S3 バケット作成が 5 秒以内で完了する性能テスト

---

## v0.5 - S3 拡張機能 + GCS

**ゴール**: S3 の拡張機能（マルチパート、ポリシー、バージョニング等）と GCP Cloud Storage を実装
**完動品としての価値**: S3 の高度な機能が Terraform で設定可能。GCS も同じ MinIO バックエンドで動作

- [ ] S3 マルチパートアップロード: CreateMultipartUpload, UploadPart, CompleteMultipartUpload, AbortMultipartUpload のハンドラ実装
- [ ] S3 バケットポリシー: PutBucketPolicy, GetBucketPolicy, DeleteBucketPolicy のハンドラ実装
- [ ] S3 バージョニング: PutBucketVersioning, GetBucketVersioning のハンドラ実装
- [ ] S3 ACL: PutBucketAcl, GetBucketAcl のハンドラ実装
- [ ] S3 CORS: PutBucketCors, GetBucketCors, DeleteBucketCors のハンドラ実装
- [ ] S3 ライフサイクル: PutBucketLifecycleConfiguration, GetBucketLifecycleConfiguration のハンドラ実装
- [ ] S3 バーチャルホスト形式: `{bucket}.s3.localhost:4566` でのアクセス対応
- [ ] GCP Cloud Storage サービス (`internal/service/gcp/storage/service.go`): Service 実装、`share_backend_with: "aws.s3"` で MinIO 共有
- [ ] GCS ハンドラ (`internal/service/gcp/storage/handlers.go`): buckets.insert, .get, .list, .delete, objects.insert, .get, .list, .delete, .copy の各ハンドラ実装
- [ ] GCS モデル (`internal/service/gcp/storage/models.go`): GCS リソースモデル
- [ ] GCS JSON API (`storage.googleapis.com`) 互換エンドポイント
- [ ] テスト: S3 マルチパートアップロードテスト、Terraform `aws_s3_bucket` のポリシー/バージョニング設定テスト、GCS バケット CRUD テスト

---

## v0.6 - IAM / VPC / SQS (軽量サービス群)

**ゴール**: コンテナバックエンドを持たない軽量サービスを実装。EC2/Lambda の前提条件となるリソースを揃える
**完動品としての価値**: Terraform で IAM ロール/ポリシー、VPC/サブネット、SQS キューの作成が可能

- [ ] AWS IAM サービス (`internal/service/aws/iam/`): service.go, handlers.go, models.go — CreateRole, DeleteRole, GetRole, ListRoles, CreatePolicy, DeletePolicy, GetPolicy, ListPolicies, AttachRolePolicy, DetachRolePolicy, CreateUser, DeleteUser, GetUser, ListUsers の各ハンドラ実装。ポリシー評価はスキップ（格納のみ）
- [ ] AWS VPC サービス (`internal/service/aws/vpc/`): service.go, handlers.go, models.go — CreateVpc, DeleteVpc, DescribeVpcs, CreateSubnet, DeleteSubnet, DescribeSubnets の各ハンドラ実装。バックエンドは Docker ネットワーク（VPC ごとに 1 ネットワーク作成）。Gateway ルーティングで EC2 Action パラメータ空間から VPC アクションを振り分ける処理を追加
- [ ] AWS SQS サービス (`internal/service/aws/sqs/`): service.go, handlers.go, models.go — CreateQueue, DeleteQueue, ListQueues, GetQueueUrl, GetQueueAttributes, SendMessage, ReceiveMessage, DeleteMessage, PurgeQueue, ChangeMessageVisibility の各ハンドラ実装。FIFO キュー対応（MessageDeduplicationId, MessageGroupId）。バックエンドはインメモリ実装
- [ ] テスト: Terraform `aws_iam_role`, `aws_vpc`, `aws_subnet`, `aws_sqs_queue` の apply/destroy テスト

---

## v0.7 - EC2 基本 (Docker コンテナバックエンド)

**ゴール**: EC2 インスタンスの基本的な起動・停止が Docker コンテナで動作する
**完動品としての価値**: `aws ec2 run-instances` でコンテナが起動し `describe-instances` で状態確認できる。Terraform `aws_instance` が apply 可能

- [ ] AWS EC2 サービス (`internal/service/aws/ec2/service.go`): Service 実装
- [ ] EC2 基本ハンドラ (`internal/service/aws/ec2/handlers.go`): RunInstances, TerminateInstances, DescribeInstances, StartInstances, StopInstances の各ハンドラ実装
- [ ] EC2 モデル (`internal/service/aws/ec2/models.go`): Instance, Reservation のリソースモデル
- [ ] AMI マッピング: 設定ファイルの `ami_mappings` から Docker イメージを解決。未知の AMI ID にはフォールバックイメージ（デフォルト: ubuntu:22.04）を使用
- [ ] インスタンスタイプ→コンテナリソース制限マッピング
- [ ] 状態マッピング: Docker コンテナ状態 → EC2 インスタンス状態（created→pending, running→running, paused→stopped, exited→terminated）
- [ ] タグ管理: CreateTags, DeleteTags, DescribeTags の各ハンドラ実装
- [ ] テスト: `aws ec2 run-instances`, `describe-instances`, `terminate-instances` の統合テスト。Terraform `aws_instance` の apply/destroy テスト。EC2 インスタンス起動が 15 秒以内で完了する性能テスト

---

## v0.8 - EC2 拡張 + GCP Compute Engine

**ゴール**: EC2 のセキュリティグループ、キーペア、IMDS を実装し、GCP Compute Engine を追加
**完動品としての価値**: Terraform で EC2 セキュリティグループ付きインスタンスを作成可能。GCE インスタンスも同様に動作

- [ ] EC2 セキュリティグループ: CreateSecurityGroup, DeleteSecurityGroup, DescribeSecurityGroups, AuthorizeSecurityGroupIngress, RevokeSecurityGroupIngress の各ハンドラ実装
- [ ] EC2 キーペア: CreateKeyPair, DeleteKeyPair, DescribeKeyPairs の各ハンドラ実装
- [ ] EC2 IMDS (Instance Metadata Service): コンテナ内から 169.254.169.254 でメタデータ取得。Gateway 内に専用 HTTP サーバーを立て、Docker ネットワーク設定でルーティング。v1/v2 両対応
- [ ] GCP Compute Engine サービス (`internal/service/gcp/compute/`): service.go, handlers.go, models.go — instances.insert, .get, .list, .delete, .start, .stop の各ハンドラ実装。マシンタイプ→リソース制限マッピング、イメージファミリー→Docker イメージマッピング
- [ ] テスト: Terraform `aws_security_group`, `aws_key_pair` テスト。GCE インスタンス CRUD テスト

---

## v0.9 - ElastiCache / Memorystore (Redis) + RDS / Cloud SQL (MySQL)

**ゴール**: Redis と MySQL のコンテナバックエンドが動作し、関連サービスの基本 CRUD が可能
**完動品としての価値**: Terraform で ElastiCache/RDS を作成し、実際に Redis/MySQL に接続してクエリ実行可能。Phase 2 の PostgreSQL 追加を見据え、RDB バックエンドにエンジン種別の切替設計（Strategy パターン）を含める

- [ ] Redis バックエンド (`internal/backend/redis/backend.go`): Redis コンテナの起動・停止・再利用、ヘルスチェック、AUTH 設定
- [ ] RDB バックエンド (`internal/backend/rdb/backend.go`): MySQL コンテナの起動・停止・再利用、ヘルスチェック、root パスワード設定、初期 DB 作成。エンジン種別による条件分岐の設計（Strategy パターン）を含め Phase 2 の PostgreSQL 追加に備える
- [ ] AWS ElastiCache サービス (`internal/service/aws/elasticache/`): service.go, handlers.go, models.go — CreateCacheCluster, DeleteCacheCluster, DescribeCacheClusters, ModifyCacheCluster, CreateReplicationGroup, DeleteReplicationGroup, DescribeReplicationGroups, CreateCacheParameterGroup, DescribeCacheParameterGroups の各ハンドラ実装。AUTH トークン対応
- [ ] AWS RDS サービス (`internal/service/aws/rds/`): service.go, handlers.go, models.go — CreateDBInstance, DeleteDBInstance, DescribeDBInstances, ModifyDBInstance, RebootDBInstance, CreateDBSnapshot, DeleteDBSnapshot, DescribeDBSnapshots, CreateDBParameterGroup, DescribeDBParameterGroups の各ハンドラ実装。エンジンは MySQL 8.0 のみ（Phase 2 で PostgreSQL 追加）
- [ ] GCP Memorystore サービス (`internal/service/gcp/memorystore/`): service.go, handlers.go, models.go — instances.create, .get, .list, .delete の各ハンドラ実装。`share_backend_with: "aws.elasticache"` で Redis 共有
- [ ] GCP Cloud SQL サービス (`internal/service/gcp/cloudsql/`): service.go, handlers.go, models.go — instances.insert, .get, .list, .delete の各ハンドラ実装。`share_backend_with: "aws.rds"` で MySQL 共有
- [ ] テスト: Terraform `aws_elasticache_cluster`, `aws_db_instance` の apply/destroy テスト。Redis/MySQL への接続確認テスト。ElastiCache/RDS インスタンス作成が 10 秒以内で完了する性能テスト

---

## v0.10 - DynamoDB

**ゴール**: DynamoDB Local バックエンドが動作し、テーブル/アイテム CRUD が可能
**完動品としての価値**: Terraform で DynamoDB テーブルを作成してアイテム操作が可能。Terraform state ロック（DynamoDB ベース）もローカルで動作

- [ ] DynamoDB バックエンド (`internal/backend/dynamodb/backend.go`): DynamoDB Local コンテナの起動・停止・再利用、ヘルスチェック
- [ ] AWS DynamoDB サービス (`internal/service/aws/dynamodb/`): service.go, handlers.go, models.go — CreateTable, DeleteTable, DescribeTable, ListTables, PutItem, GetItem, UpdateItem, DeleteItem, Query, Scan, BatchWriteItem, BatchGetItem の各ハンドラ実装。GSI/LSI 対応。プロトコルは JSON（X-Amz-Target: DynamoDB_20120810.\*）
- [ ] テスト: Terraform `aws_dynamodb_table` の apply/destroy テスト。DynamoDB アイテム操作テスト。Terraform state ロック（S3 backend + DynamoDB lock table）のローカル動作テスト

---

## v0.11 - Lambda

**ゴール**: Lambda 関数のデプロイと実行が Docker コンテナ上で動作する
**完動品としての価値**: Terraform で Lambda 関数を作成し、Invoke で実行可能

- [ ] Lambda バックエンド (`internal/backend/lambda/backend.go`): ランタイムコンテナの起動・管理、関数コードのマウント（State Store 内の一時ディレクトリにアップロードし Docker ボリュームでマウント）、Invoke 処理（HTTP リクエスト→コンテナ内ランタイム API）
- [ ] AWS Lambda サービス (`internal/service/aws/lambda/`): service.go, handlers.go, models.go — CreateFunction, DeleteFunction, GetFunction, ListFunctions, UpdateFunctionCode, UpdateFunctionConfiguration, Invoke（同期/非同期）の各ハンドラ実装
- [ ] ランタイムマッピング: 設定ファイルの `runtime_mappings` から Docker イメージを解決（python3.12, nodejs20.x 等）
- [ ] Lambda レイヤー: CreateLayerVersion, GetLayerVersion, ListLayers の各ハンドラ実装。ボリュームマウントによる簡易実装
- [ ] 環境変数設定: 関数作成時の環境変数をコンテナに注入
- [ ] サービス間通信: Lambda→DynamoDB 等の Docker ネットワーク構成
- [ ] テスト: Terraform `aws_lambda_function` の apply テスト。Python/Node.js 関数の Invoke テスト

---

## v0.12 - EKS / GKE (Kubernetes バックエンド)

**ゴール**: k3s をバックエンドとして EKS/GKE クラスタの作成・管理が動作する
**完動品としての価値**: Terraform で EKS/GKE クラスタを作成し、kubeconfig を取得して kubectl で操作可能

- [ ] Kubernetes バックエンド (`internal/backend/k8s/backend.go`): k3s コンテナの起動・停止、kubeconfig 生成、ヘルスチェック（API server readiness）。デフォルトバックエンドは k3s、設定で kind に切替可能
- [ ] AWS EKS サービス (`internal/service/aws/eks/`): service.go, handlers.go, models.go — CreateCluster, DeleteCluster, DescribeCluster, ListClusters, CreateNodegroup, DeleteNodegroup, DescribeNodegroup, ListNodegroups の各ハンドラ実装。バージョン指定（k3s バージョンにマッピング）、ステータス管理（CREATING→ACTIVE→DELETING）
- [ ] GCP GKE サービス (`internal/service/gcp/gke/`): service.go, handlers.go, models.go — projects.locations.clusters.create, .get, .list, .delete の各ハンドラ実装。`share_backend_with: "aws.eks"` で k3s 共有
- [ ] テスト: Terraform `aws_eks_cluster` の apply テスト。kubeconfig 取得→`kubectl get nodes` 成功テスト。EKS クラスタ作成が 60 秒以内で完了する性能テスト

---

## v0.13 - GCP Pub/Sub + 横断機能 + E2E テスト

**ゴール**: 最後のサービス（Pub/Sub）を実装し、メトリクス・エッジケースハンドリング・E2E テストを整備する
**完動品としての価値**: Phase 1 の全サービスが動作し、本番利用に耐えるエラーハンドリング・モニタリング・クリーンアップが整備された状態

- [ ] GCP Pub/Sub サービス (`internal/service/gcp/pubsub/`): service.go, handlers.go, models.go — projects.topics.create, .get, .list, .delete, .publish, projects.subscriptions.create, .get, .list, .delete, .pull, .acknowledge の各ハンドラ実装。バックエンドはインメモリ
- [ ] Prometheus メトリクス (`internal/gateway/middleware/metrics.go`): リクエスト数、レイテンシヒストグラム、エラーレート（プロバイダ/サービス/アクション別）。メトリクスエンドポイント公開
- [ ] エッジケースハンドリング強化: Docker デーモン未起動時のエラーメッセージと起動ガイド、OOM 検出（OOMKilled→適切なエラー）、イメージプル失敗時のリトライ（最大3回、exponential backoff）、ディスク枯渇検出（ストレージクォータチェック）、並行リクエストのロック競合タイムアウト、リソース不足時（max_containers 到達）の拒否エラー、State 不整合時の自動 reconciliation トリガー、ポート衝突時の自動リトライ、冪等性チェック（ClientToken/RequestId ベース）
- [ ] Reconciliation 定期実行: バックグラウンドゴルーチンで reconciliation_interval（デフォルト 30 秒）ごとに State↔Docker 照合、孤立コンテナの自動クリーンアップ
- [ ] AWS リージョン / GCP ゾーンの概念をメタデータとして管理（実際のリージョン分離はしない）
- [ ] Terraform state と Cloudia 内部状態の不整合検出・警告ログ
- [ ] E2E テスト: AWS CLI を使った S3/EC2/SQS/DynamoDB の基本操作テスト、Terraform hashicorp/aws で S3+IAM+EC2 のマルチリソース apply テスト、GCP CLI を使った Cloud Storage/Compute Engine テスト、Terraform hashicorp/google での apply テスト、全サービスの参照系 API レスポンスが 500ms 以内である性能テスト

---

## Phase 2: 拡張機能

---

## v0.14 - RDS / Cloud SQL PostgreSQL サポート

**ゴール**: RDS と Cloud SQL で PostgreSQL エンジンを選択可能にする
**完動品としての価値**: `engine: postgres` 指定で RDS/Cloud SQL インスタンスを作成し、PostgreSQL に接続してクエリ実行可能

- [ ] RDB バックエンド拡張 (`internal/backend/rdb/backend.go`): エンジン種別（mysql/postgres）に応じた Docker イメージ選択（`postgres:16`）。PostgreSQL 固有の初期化（ユーザー、DB 作成）。ヘルスチェック（`pg_isready`）
- [ ] RDS サービス拡張 (`internal/service/aws/rds/`): `Engine: "postgres"` 対応。パラメータグループの PostgreSQL 互換バリデーション。設定ファイルに postgres 用イメージ追加
- [ ] Cloud SQL サービス拡張 (`internal/service/gcp/cloudsql/`): `databaseVersion: "POSTGRES_16"` 対応
- [ ] テスト: Terraform `aws_db_instance` で engine="postgres" の apply テスト。PostgreSQL 接続・クエリテスト

---

## v0.15 - Web UI (管理画面)

**ゴール**: ブラウザからリソースの一覧表示・詳細確認・基本操作ができる管理画面を提供
**完動品としての価値**: `http://localhost:4566/admin/ui` にアクセスして、起動中のコンテナ・リソース・サービスヘルスを視覚的に確認・管理

- [ ] 管理 API 拡張 (`internal/admin/admin.go`): 全リソース一覧（フィルタリング、ページネーション）、リソース詳細取得、リソース削除、サービス個別の起動/停止、コンテナログ取得 API
- [ ] Web UI フロントエンド（技術: htmx + Go html/template、Go の embed パッケージでバイナリ埋め込み — Node.js ビルド不要でシングルバイナリ配布を維持）
- [ ] ダッシュボード画面: サービス一覧、ヘルスステータス、リソース数サマリ
- [ ] リソースブラウザ画面: プロバイダ/サービス別のリソース一覧・詳細・削除操作
- [ ] コンテナビュー画面: 起動中コンテナの一覧、ログ表示、停止操作
- [ ] 設定ビュー画面: 現在の設定表示
- [ ] テスト: 管理 API の統合テスト。Web UI の画面遷移テスト

---

## 注記

### Critic レビューからの留保事項

- パフォーマンス目標の計測は各サービスマイルストーンのテストタスクに含めた
- セキュリティ（localhost バインド）は v0.1 の Gateway 実装に明示的に含めた
- macOS/Linux 互換性の検証は v0.13 の E2E テストで実施する
- SQS/Pub/Sub の Redis バックエンド切替は Phase 1 ではインメモリのみ実装し、永続性が必要になった時点で追加する

### Open Questions

- IMDS の実装方式: Gateway 内に専用 HTTP サーバーを立て Docker ネットワーク経由でルーティングする方式を採用（macOS 互換性のため iptables は不採用）
- k3s vs kind: k3s をデフォルトとする（シングルバイナリで軽量）。設定で kind に切替可能
- Lambda レイヤー: ボリュームマウントによる簡易実装とする
