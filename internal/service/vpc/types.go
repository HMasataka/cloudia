package vpc

import "encoding/xml"

// EC2 XML namespace for VPC responses.
const xmlNamespace = "http://ec2.amazonaws.com/doc/2016-11-15/"

// Store kinds for VPC resources.
const (
	kindVPC    = "aws:vpc:vpc"
	kindSubnet = "aws:vpc:subnet"
)

// VpcItem はレスポンス内の VPC 要素です。
type VpcItem struct {
	VpcId     string `xml:"vpcId"`
	CidrBlock string `xml:"cidrBlock"`
	State     string `xml:"state"`
}

// SubnetItem はレスポンス内の Subnet 要素です。
type SubnetItem struct {
	SubnetId string `xml:"subnetId"`
	VpcId    string `xml:"vpcId"`
	CidrBlock string `xml:"cidrBlock"`
	State     string `xml:"state"`
}

// CreateVpcResponse は CreateVpc アクションのレスポンスです。
type CreateVpcResponse struct {
	XMLName   xml.Name `xml:"CreateVpcResponse"`
	RequestId string   `xml:"requestId"`
	Vpc       VpcItem  `xml:"vpc"`
}

// DescribeVpcsResponse は DescribeVpcs アクションのレスポンスです。
type DescribeVpcsResponse struct {
	XMLName   xml.Name  `xml:"DescribeVpcsResponse"`
	RequestId string    `xml:"requestId"`
	VpcSet    []VpcItem `xml:"vpcSet>item"`
}

// DeleteVpcResponse は DeleteVpc アクションのレスポンスです。
type DeleteVpcResponse struct {
	XMLName   xml.Name `xml:"DeleteVpcResponse"`
	RequestId string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

// CreateSubnetResponse は CreateSubnet アクションのレスポンスです。
type CreateSubnetResponse struct {
	XMLName   xml.Name   `xml:"CreateSubnetResponse"`
	RequestId string     `xml:"requestId"`
	Subnet    SubnetItem `xml:"subnet"`
}

// DescribeSubnetsResponse は DescribeSubnets アクションのレスポンスです。
type DescribeSubnetsResponse struct {
	XMLName   xml.Name     `xml:"DescribeSubnetsResponse"`
	RequestId string       `xml:"requestId"`
	SubnetSet []SubnetItem `xml:"subnetSet>item"`
}

// DeleteSubnetResponse は DeleteSubnet アクションのレスポンスです。
type DeleteSubnetResponse struct {
	XMLName   xml.Name `xml:"DeleteSubnetResponse"`
	RequestId string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}
