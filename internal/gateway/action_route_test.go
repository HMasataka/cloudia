package gateway

import "testing"

func TestResolveServiceName_VPCActionsRewrittenToVPC(t *testing.T) {
	vpcActions := []string{
		"CreateVpc",
		"DeleteVpc",
		"DescribeVpcs",
		"CreateSubnet",
		"DeleteSubnet",
		"DescribeSubnets",
	}

	for _, action := range vpcActions {
		t.Run(action, func(t *testing.T) {
			got := ResolveServiceName("aws", "ec2", action)
			if got != "vpc" {
				t.Errorf("ResolveServiceName(%q, %q, %q) = %q, want %q", "aws", "ec2", action, got, "vpc")
			}
		})
	}
}

func TestResolveServiceName_EC2ActionsNotRewritten(t *testing.T) {
	ec2Actions := []string{
		"DescribeInstances",
		"RunInstances",
		"TerminateInstances",
	}

	for _, action := range ec2Actions {
		t.Run(action, func(t *testing.T) {
			got := ResolveServiceName("aws", "ec2", action)
			if got != "ec2" {
				t.Errorf("ResolveServiceName(%q, %q, %q) = %q, want %q", "aws", "ec2", action, got, "ec2")
			}
		})
	}
}

func TestResolveServiceName_IAMActionsNotRewritten(t *testing.T) {
	iamActions := []string{
		"CreateUser",
		"DeleteUser",
		"ListUsers",
	}

	for _, action := range iamActions {
		t.Run(action, func(t *testing.T) {
			got := ResolveServiceName("aws", "iam", action)
			if got != "iam" {
				t.Errorf("ResolveServiceName(%q, %q, %q) = %q, want %q", "aws", "iam", action, got, "iam")
			}
		})
	}
}
