# 仕様: Cloudia - マルチクラウドローカルエミュレータ

## 概要

Cloudia は AWS / Google Cloud の API 互換エンドポイントをローカルに提供するクラウドエミュレータである。Terraform の各クラウドプロバイダ（`hashicorp/aws`, `hashicorp/google`）からエンドポイント指定で利用でき、裏側で実際に Docker コンテナを起動してサービスを提供する。LocalStack のようなダミーレスポンスではなく、MinIO・Redis・MySQL・k3s 等の実コンテナが動作する点が差別化ポイント。

## 技術コンテキスト

- **実装言語**: Go（Docker SDK の Go ネイティブサポート、シングルバイナリ配布に最適）
- **コンテナ管理**: Docker SDK for Go (`github.com/docker/docker/client`) を直接利用。Terraform Docker Provider は不採用（レイテンシ数秒〜数十秒は API レスポンスとして許容不可、state 管理の不要な複雑性）
- **API 互換方式**: 各クラウドの API 仕様に完全準拠したエンドポイントを提供。Terraform のクラウドプロバイダがそのまま動作する
- **バックエンド戦略**: 各サービスに対応する実績ある OSS コンテナを裏側で起動（MinIO, Redis, MySQL, k3s 等）
- **アーキテクチャ**: プラグイン方式で新サービスを追加可能な設計

```
Terraform (AWS/GCP Provider) → HTTP → Cloudia (API互換サーバー) → Docker SDK → Docker Engine
AWS CLI / gcloud CLI          →                                        ↓
                                                              MinIO / Redis / MySQL / k3s ...
```

## 機能要件

### コア機能

- [x] AWS API 互換エンドポイントの提供（Terraform `hashicorp/aws` プロバイダから endpoint override で利用可能）
- [x] Google Cloud API 互換エンドポイントの提供（Terraform `hashicorp/google` プロバイダから endpoint override で利用可能）
- [x] AWS CLI (`--endpoint-url`) からの利用サポート
- [x] gcloud CLI からの利用サポート
- [x] SigV4 署名検証（AWS 完全互換）
- [x] Google Cloud 認証トークン検証
- [x] XML / JSON レスポンスフォーマットの完全準拠（各サービスの仕様に合わせる）
- [x] CLI ツール: `cloudia start` / `cloudia stop` / `cloudia status` / `cloudia cleanup`
- [x] 設定ファイル（YAML）によるサービス有効化・ポート・リソース制限の管理
- [x] 統一エンドポイント（デフォルト: `localhost:4566`）でのサービスルーティング
- [x] サービスごとの個別エンドポイントもサポート

### AWS サービス

#### S3 (Simple Storage Service)

- [x] バックエンド: MinIO コンテナ
- [x] CreateBucket / DeleteBucket / ListBuckets
- [x] PutObject / GetObject / DeleteObject / ListObjectsV2
- [x] CopyObject / HeadObject
- [x] マルチパートアップロード (CreateMultipartUpload, UploadPart, CompleteMultipartUpload, AbortMultipartUpload)
- [x] バケットポリシー (PutBucketPolicy, GetBucketPolicy, DeleteBucketPolicy)
- [x] バケットバージョニング (PutBucketVersioning, GetBucketVersioning)
- [x] バケット ACL (PutBucketAcl, GetBucketAcl)
- [x] CORS 設定 (PutBucketCors, GetBucketCors, DeleteBucketCors)
- [x] ライフサイクル設定 (PutBucketLifecycleConfiguration, GetBucketLifecycleConfiguration)
- [x] バーチャルホスト形式 (`{bucket}.s3.localhost`) とパス形式の両方サポート

#### EC2 (Elastic Compute Cloud)

- [x] バックエンド: Docker コンテナ（AMI ID → Docker イメージマッピング）
- [x] RunInstances / TerminateInstances
- [x] DescribeInstances / StartInstances / StopInstances
- [x] AMI ID → Docker イメージマッピングテーブル（設定ファイルで管理）
- [x] インスタンスタイプ → コンテナリソース制限マッピング
- [x] セキュリティグループ (CreateSecurityGroup, DeleteSecurityGroup, DescribeSecurityGroups, AuthorizeSecurityGroupIngress/Egress)
- [x] キーペア (CreateKeyPair, DeleteKeyPair, DescribeKeyPairs)
- [x] タグ管理 (CreateTags, DeleteTags, DescribeTags)
- [x] インスタンスメタデータサービス (IMDS v1/v2) のエミュレーション
- [x] Docker コンテナ状態 → EC2 インスタンス状態マッピング (created→pending, running→running, paused→stopped, exited→terminated)

#### EKS (Elastic Kubernetes Service)

- [x] バックエンド: k3s または kind クラスタ
- [x] CreateCluster / DeleteCluster / DescribeCluster / ListClusters
- [x] 返却される kubeconfig で `kubectl` が利用可能
- [x] ノードグループ (CreateNodegroup, DeleteNodegroup, DescribeNodegroup)
- [x] クラスタのバージョン指定（k3s のバージョンにマッピング）
- [x] クラスタステータス管理 (CREATING → ACTIVE → DELETING)

#### ElastiCache (Redis 互換)

- [x] バックエンド: Redis コンテナ (`redis:latest`)
- [x] CreateCacheCluster / DeleteCacheCluster / DescribeCacheClusters
- [x] CreateReplicationGroup / DeleteReplicationGroup / DescribeReplicationGroups
- [x] キャッシュノードタイプ → コンテナリソース制限マッピング
- [x] エンドポイント情報の返却（ホスト:ポート）
- [x] パラメータグループ (CreateCacheParameterGroup, ModifyCacheParameterGroup)
- [x] Redis AUTH トークン設定

#### RDS (MySQL 互換)

- [x] バックエンド: MySQL コンテナ (`mysql:8.0`)
- [x] CreateDBInstance / DeleteDBInstance / DescribeDBInstances
- [x] ModifyDBInstance / RebootDBInstance
- [x] エンジンバージョン指定（MySQL コンテナタグにマッピング）
- [x] 接続エンドポイント情報の返却（ホスト:ポート）
- [x] データベース初期作成 (DBName パラメータ)
- [x] マスターユーザー/パスワード設定
- [x] CreateDBSnapshot / DeleteDBSnapshot / DescribeDBSnapshots（Docker volume のスナップショット）
- [x] パラメータグループ (CreateDBParameterGroup, ModifyDBParameterGroup)

#### IAM (Identity and Access Management)

- [x] CreateRole / DeleteRole / GetRole / ListRoles
- [x] CreatePolicy / DeletePolicy / GetPolicy / ListPolicies
- [x] AttachRolePolicy / DetachRolePolicy
- [x] CreateUser / DeleteUser / GetUser / ListUsers
- [x] ポリシー評価はスキップ（全操作を許可）、リソース管理のみエミュレート

#### VPC (Virtual Private Cloud)

- [x] バックエンド: Docker ネットワーク
- [x] CreateVpc / DeleteVpc / DescribeVpcs
- [x] CreateSubnet / DeleteSubnet / DescribeSubnets
- [x] VPC → Docker ネットワークマッピング
- [x] サブネット → Docker ネットワーク内のアドレス範囲マッピング

#### SQS (Simple Queue Service)

- [x] バックエンド: インメモリキュー or Redis ベース
- [x] CreateQueue / DeleteQueue / ListQueues
- [x] SendMessage / ReceiveMessage / DeleteMessage
- [x] メッセージ属性 / 遅延キュー / 可視性タイムアウト
- [x] FIFO キューサポート

#### Lambda

- [x] バックエンド: Docker コンテナ（ランタイムイメージ）
- [x] CreateFunction / DeleteFunction / GetFunction / ListFunctions
- [x] Invoke（同期/非同期）
- [x] ランタイムごとの Docker イメージマッピング (python3.x, nodejs18.x 等)
- [x] 環境変数設定
- [x] レイヤーサポート

#### DynamoDB

- [x] バックエンド: DynamoDB Local コンテナ (`amazon/dynamodb-local`)
- [x] CreateTable / DeleteTable / DescribeTable / ListTables
- [x] PutItem / GetItem / DeleteItem / UpdateItem
- [x] Query / Scan
- [x] バッチ操作 (BatchGetItem, BatchWriteItem)
- [x] GSI / LSI

### Google Cloud サービス

#### Cloud Storage (GCS)

- [x] バックエンド: MinIO コンテナ（S3 互換モード + GCS API ラッパー）
- [x] バケット CRUD (storage.buckets.insert, storage.buckets.delete, storage.buckets.get, storage.buckets.list)
- [x] オブジェクト CRUD (storage.objects.insert, storage.objects.get, storage.objects.delete, storage.objects.list)
- [x] JSON API (`storage.googleapis.com`) 互換エンドポイント

#### Compute Engine (GCE)

- [x] バックエンド: Docker コンテナ
- [x] instances.insert / instances.delete / instances.get / instances.list
- [x] instances.start / instances.stop
- [x] マシンタイプ → コンテナリソース制限マッピング
- [x] イメージファミリー → Docker イメージマッピング

#### GKE (Google Kubernetes Engine)

- [x] バックエンド: k3s または kind クラスタ
- [x] projects.locations.clusters.create / delete / get / list
- [x] 返却される kubeconfig で `kubectl` が利用可能

#### Cloud SQL

- [x] バックエンド: MySQL / PostgreSQL コンテナ
- [x] instances.insert / instances.delete / instances.get / instances.list
- [x] MySQL / PostgreSQL エンジン選択
- [x] 接続エンドポイント情報の返却

#### Memorystore (Redis)

- [x] バックエンド: Redis コンテナ
- [x] instances.create / instances.delete / instances.get / instances.list
- [x] Redis バージョン指定

#### Cloud Pub/Sub

- [x] バックエンド: インメモリまたは Redis ベース
- [x] projects.topics.create / delete / get / list
- [x] projects.subscriptions.create / delete / get / list / pull

### 横断機能

- [x] プラグインアーキテクチャ: `Service` インターフェースを実装するだけで新サービスを追加可能
- [x] サービスレジストリ: 有効化するサービスを設定ファイルで選択可能
- [x] ヘルスチェック API (`/health`) でサービスの状態を確認可能
- [x] 管理 API (`/admin`) でリソース一覧・強制削除等の操作が可能

## 非機能要件

### パフォーマンス

- [x] S3 バケット作成: 5 秒以内（MinIO コンテナ起動済みの場合）
- [x] EC2 インスタンス起動: 15 秒以内（Docker イメージプル済みの場合）
- [x] EKS クラスタ作成: 60 秒以内
- [x] ElastiCache / RDS インスタンス作成: 10 秒以内（イメージプル済みの場合）
- [x] API レスポンス（リソース参照系）: 500ms 以内

### リソース制限・ガードレール

- [x] 同時起動コンテナ数の上限設定（デフォルト: 20）
- [x] コンテナごとの CPU 上限設定（デフォルト: 1 CPU）
- [x] コンテナごとのメモリ上限設定（デフォルト: 512MB）
- [x] S3/GCS ストレージ容量上限設定
- [x] API タイムアウト設定（デフォルト: 30 秒）
- [x] Docker イメージのホワイトリスト設定

### ネットワーク

- [x] サービスごとの Docker ネットワーク分離
- [x] VPC エミュレーション時の Docker ネットワークマッピング
- [x] ポート動的割り当て + サービスディスカバリ
- [x] リバースプロキシ経由の統一エンドポイント
- [x] ポート衝突の自動検出と代替ポートへのフォールバック

### 状態管理

- [x] インメモリ状態ストア（デフォルト）
- [x] ファイルベースの永続化オプション（再起動後もリソース状態を保持）
- [x] Docker コンテナの実態との reconciliation 機構（状態の不整合を自動修復）

### クリーンアップ

- [x] `cloudia stop` で全管理リソース（コンテナ・ネットワーク・ボリューム）を確実に削除
- [x] 異常終了時: 次回起動時にラベル `cloudia.managed=true` の孤立リソースを検出・削除
- [x] `cloudia cleanup` コマンドで手動クリーンアップ
- [x] シャットダウンフック（SIGTERM/SIGINT）による graceful shutdown

### 互換性

- [x] Docker Engine API v1.41+ をサポート
- [x] Docker Desktop / OrbStack / Podman (rootful) での動作検証
- [x] macOS (Apple Silicon / Intel) / Linux での動作サポート
- [x] Go 1.22+ でのビルド

### セキュリティ

- [x] ローカルホストのみリッスン（デフォルト: `127.0.0.1`）
- [x] SigV4 署名検証（AWS サービス向け）
- [x] Google Cloud OAuth トークン検証（GCP サービス向け）
- [x] 管理 API のアクセス制御

### ログ・可観測性

- [x] 構造化ログ（JSON 形式、go.uber.org/zap）
- [x] リクエスト/レスポンスのアクセスログ
- [x] コンテナライフサイクルイベントのログ
- [x] ログレベル設定（debug / info / warn / error）
- [x] メトリクスエンドポイント（Prometheus 形式、オプション）

## エッジケース・リスク

### Docker 関連

- [x] Docker デーモン未起動時に明確なエラーメッセージで起動失敗する
- [x] コンテナが OOM 等で予期せず停止した場合、Docker event stream を監視して内部状態を更新する
- [x] Docker イメージのプル失敗時（オフライン環境）に適切なエラーを返す。事前プル機構を提供
- [x] ディスク容量枯渇時にクォータエラーを返す

### API 互換性

- [x] 未サポートの API アクションに対して AWS/GCP 互換のエラーレスポンスを返す（`UnsupportedOperation` 等）
- [x] 冪等性: 同一リソースの二重作成時に適切なエラー（`BucketAlreadyExists`, `InstanceAlreadyExists` 等）
- [x] AWS リージョン / GCP ゾーンの概念をメタデータとして管理（実際のリージョン分離はしない）
- [x] AMI ID / マシンタイプ等の未知のパラメータに対するフォールバック動作（デフォルトイメージの使用）

### 並行性

- [x] 複数の Terraform apply / CLI コマンドの同時実行時のレースコンディション対策（リソース操作のロック機構）
- [x] コンテナ起動中のリクエストに対する適切な状態返却（pending 等）

### リソース管理

- [x] ホストマシンのリソース不足時に新規コンテナ作成を拒否し、適切なエラーを返す
- [x] 長時間放置されたリソースの自動クリーンアップ（TTL 設定、オプション）
- [x] Terraform state と Cloudia 内部状態の不整合検出・警告

### ネットワーク

- [x] ホストのポートが既に使用中の場合の自動検出と代替ポートへのフォールバック
- [x] サービス間通信が必要な場合（Lambda → DynamoDB 等）の Docker ネットワーク構成

## Open Questions

- [x] PostgreSQL (RDS / Cloud SQL) のサポートを Phase 1 に含めるか、Phase 2 にするか
  - Phase 2 で対応
- [ ] Terraform の state ロック機構（DynamoDB ベース）のエミュレーションは必要か
- [x] マルチアカウント / マルチプロジェクトの分離をサポートするか
  - 不要
- [ ] CI/CD 環境（Docker-in-Docker）での動作保証はスコープに含めるか
  - 不要
- [ ] Azure サポートの将来計画をアーキテクチャ設計に反映するか
  - 不要
- [ ] プラグインの外部配布（サードパーティ製サービスプラグイン）をサポートするか
  - 不要
- [x] Web UI（リソースの可視化・管理画面）は将来的に提供するか
  - 必要
