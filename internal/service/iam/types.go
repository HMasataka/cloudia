package iam

import "encoding/xml"

const iamNamespace = "https://iam.amazonaws.com/doc/2010-05-08/"

// Role は IAM ロールの内部モデルです。
type Role struct {
	RoleID                   string
	RoleName                 string
	Arn                      string
	Path                     string
	AssumeRolePolicyDocument string
	Description              string
	CreateDate               string
}

// User は IAM ユーザーの内部モデルです。
type User struct {
	UserID     string
	UserName   string
	Arn        string
	Path       string
	CreateDate string
}

// Policy は IAM ポリシーの内部モデルです。
type Policy struct {
	PolicyID         string
	PolicyName       string
	Arn              string
	Path             string
	Description      string
	DefaultVersionID string
	CreateDate       string
}

// AttachedPolicy はロールにアタッチされたポリシーを表します。
type AttachedPolicy struct {
	PolicyArn  string
	PolicyName string
}

// --- XML レスポンス構造体 ---

// RoleResult は XML レスポンス内のロール要素です。
type RoleResult struct {
	RoleID                   string `xml:"RoleId"`
	RoleName                 string `xml:"RoleName"`
	Arn                      string `xml:"Arn"`
	Path                     string `xml:"Path"`
	AssumeRolePolicyDocument string `xml:"AssumeRolePolicyDocument"`
	CreateDate               string `xml:"CreateDate"`
}

// CreateRoleResponse は CreateRole アクションのレスポンスです。
type CreateRoleResponse struct {
	XMLName xml.Name   `xml:"CreateRoleResponse"`
	Result  RoleResult `xml:"CreateRoleResult>Role"`
}

// GetRoleResponse は GetRole アクションのレスポンスです。
type GetRoleResponse struct {
	XMLName xml.Name   `xml:"GetRoleResponse"`
	Result  RoleResult `xml:"GetRoleResult>Role"`
}

// DeleteRoleResponse は DeleteRole アクションのレスポンスです。
type DeleteRoleResponse struct {
	XMLName xml.Name `xml:"DeleteRoleResponse"`
}

// ListRolesResponse は ListRoles アクションのレスポンスです。
type ListRolesResponse struct {
	XMLName     xml.Name     `xml:"ListRolesResponse"`
	Roles       []RoleResult `xml:"ListRolesResult>Roles>member"`
	IsTruncated bool         `xml:"ListRolesResult>IsTruncated"`
}

// UserResult は XML レスポンス内のユーザー要素です。
type UserResult struct {
	UserID     string `xml:"UserId"`
	UserName   string `xml:"UserName"`
	Arn        string `xml:"Arn"`
	Path       string `xml:"Path"`
	CreateDate string `xml:"CreateDate"`
}

// CreateUserResponse は CreateUser アクションのレスポンスです。
type CreateUserResponse struct {
	XMLName xml.Name   `xml:"CreateUserResponse"`
	Result  UserResult `xml:"CreateUserResult>User"`
}

// GetUserResponse は GetUser アクションのレスポンスです。
type GetUserResponse struct {
	XMLName xml.Name   `xml:"GetUserResponse"`
	Result  UserResult `xml:"GetUserResult>User"`
}

// DeleteUserResponse は DeleteUser アクションのレスポンスです。
type DeleteUserResponse struct {
	XMLName xml.Name `xml:"DeleteUserResponse"`
}

// ListUsersResponse は ListUsers アクションのレスポンスです。
type ListUsersResponse struct {
	XMLName     xml.Name     `xml:"ListUsersResponse"`
	Users       []UserResult `xml:"ListUsersResult>Users>member"`
	IsTruncated bool         `xml:"ListUsersResult>IsTruncated"`
}

// PolicyResult は XML レスポンス内のポリシー要素です。
type PolicyResult struct {
	PolicyID         string `xml:"PolicyId"`
	PolicyName       string `xml:"PolicyName"`
	Arn              string `xml:"Arn"`
	Path             string `xml:"Path"`
	DefaultVersionID string `xml:"DefaultVersionId"`
	CreateDate       string `xml:"CreateDate"`
}

// CreatePolicyResponse は CreatePolicy アクションのレスポンスです。
type CreatePolicyResponse struct {
	XMLName xml.Name     `xml:"CreatePolicyResponse"`
	Result  PolicyResult `xml:"CreatePolicyResult>Policy"`
}

// GetPolicyResponse は GetPolicy アクションのレスポンスです。
type GetPolicyResponse struct {
	XMLName xml.Name     `xml:"GetPolicyResponse"`
	Result  PolicyResult `xml:"GetPolicyResult>Policy"`
}

// DeletePolicyResponse は DeletePolicy アクションのレスポンスです。
type DeletePolicyResponse struct {
	XMLName xml.Name `xml:"DeletePolicyResponse"`
}

// ListPoliciesResponse は ListPolicies アクションのレスポンスです。
type ListPoliciesResponse struct {
	XMLName     xml.Name       `xml:"ListPoliciesResponse"`
	Policies    []PolicyResult `xml:"ListPoliciesResult>Policies>member"`
	IsTruncated bool           `xml:"ListPoliciesResult>IsTruncated"`
}

// AttachedPolicyResult は XML レスポンス内のアタッチ済みポリシー要素です。
type AttachedPolicyResult struct {
	PolicyArn  string `xml:"PolicyArn"`
	PolicyName string `xml:"PolicyName"`
}

// AttachRolePolicyResponse は AttachRolePolicy アクションのレスポンスです。
type AttachRolePolicyResponse struct {
	XMLName xml.Name `xml:"AttachRolePolicyResponse"`
}

// DetachRolePolicyResponse は DetachRolePolicy アクションのレスポンスです。
type DetachRolePolicyResponse struct {
	XMLName xml.Name `xml:"DetachRolePolicyResponse"`
}

// ListAttachedRolePoliciesResponse は ListAttachedRolePolicies アクションのレスポンスです。
type ListAttachedRolePoliciesResponse struct {
	XMLName          xml.Name               `xml:"ListAttachedRolePoliciesResponse"`
	AttachedPolicies []AttachedPolicyResult `xml:"ListAttachedRolePoliciesResult>AttachedPolicies>member"`
	IsTruncated      bool                   `xml:"ListAttachedRolePoliciesResult>IsTruncated"`
}
