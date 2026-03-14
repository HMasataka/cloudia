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
	// EC2 SecurityGroup アクションを "sg" サービスへ振り分けます。
	{provider: "aws", service: "ec2", action: "CreateSecurityGroup"}:             "sg",
	{provider: "aws", service: "ec2", action: "DeleteSecurityGroup"}:             "sg",
	{provider: "aws", service: "ec2", action: "DescribeSecurityGroups"}:          "sg",
	{provider: "aws", service: "ec2", action: "AuthorizeSecurityGroupIngress"}:   "sg",
	{provider: "aws", service: "ec2", action: "RevokeSecurityGroupIngress"}:      "sg",
	{provider: "aws", service: "ec2", action: "AuthorizeSecurityGroupEgress"}:    "sg",
	{provider: "aws", service: "ec2", action: "RevokeSecurityGroupEgress"}:       "sg",
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
