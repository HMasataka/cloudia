package sg

import "encoding/xml"

// EC2 XML namespace for SG responses.
const xmlNamespace = "http://ec2.amazonaws.com/doc/2016-11-15/"

// Store kinds for SG resources.
const (
	kindSecurityGroup = "aws:ec2:security-group"
	kindEC2Instance   = "aws:ec2:instance"
)

// IpRange はレスポンス内の IP レンジ要素です。
type IpRange struct {
	CidrIp string `xml:"cidrIp"`
}

// IpPermission はレスポンス内の IP パーミッション要素です。
type IpPermission struct {
	IpProtocol string    `xml:"ipProtocol"`
	FromPort   int       `xml:"fromPort"`
	ToPort     int       `xml:"toPort"`
	IpRanges   []IpRange `xml:"ipRanges>item"`
}

// SecurityGroupItem はレスポンス内のセキュリティグループ要素です。
type SecurityGroupItem struct {
	GroupId             string         `xml:"groupId"`
	GroupName           string         `xml:"groupName"`
	Description         string         `xml:"groupDescription"`
	VpcId               string         `xml:"vpcId"`
	IpPermissions       []IpPermission `xml:"ipPermissions>item"`
	IpPermissionsEgress []IpPermission `xml:"ipPermissionsEgress>item"`
}

// CreateSecurityGroupResponse は CreateSecurityGroup アクションのレスポンスです。
type CreateSecurityGroupResponse struct {
	XMLName   xml.Name `xml:"CreateSecurityGroupResponse"`
	RequestId string   `xml:"requestId"`
	GroupId   string   `xml:"groupId"`
}

// DescribeSecurityGroupsResponse は DescribeSecurityGroups アクションのレスポンスです。
type DescribeSecurityGroupsResponse struct {
	XMLName        xml.Name            `xml:"DescribeSecurityGroupsResponse"`
	RequestId      string              `xml:"requestId"`
	SecurityGroups []SecurityGroupItem `xml:"securityGroupInfo>item"`
}

// DeleteSecurityGroupResponse は DeleteSecurityGroup アクションのレスポンスです。
type DeleteSecurityGroupResponse struct {
	XMLName   xml.Name `xml:"DeleteSecurityGroupResponse"`
	RequestId string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

// AuthorizeSecurityGroupIngressResponse は AuthorizeSecurityGroupIngress アクションのレスポンスです。
type AuthorizeSecurityGroupIngressResponse struct {
	XMLName   xml.Name `xml:"AuthorizeSecurityGroupIngressResponse"`
	RequestId string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

// RevokeSecurityGroupIngressResponse は RevokeSecurityGroupIngress アクションのレスポンスです。
type RevokeSecurityGroupIngressResponse struct {
	XMLName   xml.Name `xml:"RevokeSecurityGroupIngressResponse"`
	RequestId string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

// AuthorizeSecurityGroupEgressResponse は AuthorizeSecurityGroupEgress アクションのレスポンスです。
type AuthorizeSecurityGroupEgressResponse struct {
	XMLName   xml.Name `xml:"AuthorizeSecurityGroupEgressResponse"`
	RequestId string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

// RevokeSecurityGroupEgressResponse は RevokeSecurityGroupEgress アクションのレスポンスです。
type RevokeSecurityGroupEgressResponse struct {
	XMLName   xml.Name `xml:"RevokeSecurityGroupEgressResponse"`
	RequestId string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}
