package gateway

// actionServiceKey はプロバイダ・サービス・アクションの組み合わせキーです。
type actionServiceKey struct {
	provider string
	service  string
	action   string
}

// ActionServiceOverride はアクションベースのサービス書き換えマッピングテーブルです。
// EC2 名前空間の VPC アクションを "vpc" サービスへ振り分けます。
var ActionServiceOverride = map[actionServiceKey]string{
	{provider: "aws", service: "ec2", action: "CreateVpc"}:       "vpc",
	{provider: "aws", service: "ec2", action: "DeleteVpc"}:       "vpc",
	{provider: "aws", service: "ec2", action: "DescribeVpcs"}:    "vpc",
	{provider: "aws", service: "ec2", action: "CreateSubnet"}:    "vpc",
	{provider: "aws", service: "ec2", action: "DeleteSubnet"}:    "vpc",
	{provider: "aws", service: "ec2", action: "DescribeSubnets"}: "vpc",
}

// ResolveServiceName はマッピングテーブルを参照し、一致するエントリがあればサービス名を書き換えて返します。
// 一致しない場合は元のサービス名をそのまま返します。
func ResolveServiceName(provider, service, action string) string {
	key := actionServiceKey{provider: provider, service: service, action: action}
	if override, ok := ActionServiceOverride[key]; ok {
		return override
	}
	return service
}
