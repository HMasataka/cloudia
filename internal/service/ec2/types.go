package ec2

import "encoding/xml"

// EC2 XML namespace for EC2 responses.
const xmlNamespace = "http://ec2.amazonaws.com/doc/2016-11-15/"

// Store kinds for EC2 resources.
const kindInstance = "aws:ec2:instance"
const kindKeyPair = "aws:ec2:key-pair"

// Instance state codes.
const (
	stateCodePending      = 0
	stateCodeRunning      = 16
	stateCodeShuttingDown = 32
	stateCodeTerminated   = 48
	stateCodeStopping     = 64
	stateCodeStopped      = 80
)

// Instance state names.
const (
	stateNamePending      = "pending"
	stateNameRunning      = "running"
	stateNameShuttingDown = "shutting-down"
	stateNameTerminated   = "terminated"
	stateNameStopping     = "stopping"
	stateNameStopped      = "stopped"
)

// TagItem はリソースタグを表します。
type TagItem struct {
	Key   string `xml:"key"`
	Value string `xml:"value"`
}

// TagSet はタグのセットです。
type TagSet struct {
	Items []TagItem `xml:"item"`
}

// InstanceStateItem はインスタンスの状態を表します。
type InstanceStateItem struct {
	Code int    `xml:"code"`
	Name string `xml:"name"`
}

// GroupItem はレスポンス内のセキュリティグループ参照要素です。
type GroupItem struct {
	GroupId string `xml:"groupId"`
}

// GroupSet はセキュリティグループ参照のセットです。
type GroupSet struct {
	Items []GroupItem `xml:"item"`
}

// PlacementItem は EC2 インスタンスの配置情報を表します。
type PlacementItem struct {
	AvailabilityZone string `xml:"availabilityZone,omitempty"`
}

// InstanceItem はレスポンス内の Instance 要素です。
type InstanceItem struct {
	InstanceId     string            `xml:"instanceId"`
	ImageId        string            `xml:"imageId"`
	InstanceType   string            `xml:"instanceType"`
	State          InstanceStateItem `xml:"instanceState"`
	PrivateIp      string            `xml:"privateIpAddress,omitempty"`
	PrivateDNSName string            `xml:"privateDnsName,omitempty"`
	LaunchTime     string            `xml:"launchTime,omitempty"`
	Placement      PlacementItem     `xml:"placement"`
	TagSet         TagSet            `xml:"tagSet"`
	GroupSet       GroupSet          `xml:"groupSet"`
}

// ReservationItem はレスポンス内の Reservation 要素です。
type ReservationItem struct {
	ReservationId string         `xml:"reservationId"`
	InstancesSet  []InstanceItem `xml:"instancesSet>item"`
}

// RunInstancesResponse は RunInstances アクションのレスポンスです。
type RunInstancesResponse struct {
	XMLName       xml.Name       `xml:"RunInstancesResponse"`
	RequestId     string         `xml:"requestId"`
	ReservationId string         `xml:"reservationId"`
	InstancesSet  []InstanceItem `xml:"instancesSet>item"`
}

// DescribeInstancesResponse は DescribeInstances アクションのレスポンスです。
type DescribeInstancesResponse struct {
	XMLName        xml.Name          `xml:"DescribeInstancesResponse"`
	RequestId      string            `xml:"requestId"`
	ReservationSet []ReservationItem `xml:"reservationSet>item"`
}

// InstanceStateChange は状態変化を表します。
type InstanceStateChange struct {
	InstanceId    string            `xml:"instanceId"`
	CurrentState  InstanceStateItem `xml:"currentState"`
	PreviousState InstanceStateItem `xml:"previousState"`
}

// TerminateInstancesResponse は TerminateInstances アクションのレスポンスです。
type TerminateInstancesResponse struct {
	XMLName      xml.Name              `xml:"TerminateInstancesResponse"`
	RequestId    string                `xml:"requestId"`
	InstancesSet []InstanceStateChange `xml:"instancesSet>item"`
}

// StartInstancesResponse は StartInstances アクションのレスポンスです。
type StartInstancesResponse struct {
	XMLName      xml.Name              `xml:"StartInstancesResponse"`
	RequestId    string                `xml:"requestId"`
	InstancesSet []InstanceStateChange `xml:"instancesSet>item"`
}

// StopInstancesResponse は StopInstances アクションのレスポンスです。
type StopInstancesResponse struct {
	XMLName      xml.Name              `xml:"StopInstancesResponse"`
	RequestId    string                `xml:"requestId"`
	InstancesSet []InstanceStateChange `xml:"instancesSet>item"`
}

// CreateTagsResponse は CreateTags アクションのレスポンスです。
type CreateTagsResponse struct {
	XMLName   xml.Name `xml:"CreateTagsResponse"`
	RequestId string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

// DeleteTagsResponse は DeleteTags アクションのレスポンスです。
type DeleteTagsResponse struct {
	XMLName   xml.Name `xml:"DeleteTagsResponse"`
	RequestId string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

// KeyPairItem はレスポンス内の KeyPair 要素です。
type KeyPairItem struct {
	KeyName        string `xml:"keyName"`
	KeyFingerprint string `xml:"keyFingerprint"`
	KeyPairId      string `xml:"keyPairId"`
}

// CreateKeyPairResponse は CreateKeyPair アクションのレスポンスです。
type CreateKeyPairResponse struct {
	XMLName        xml.Name `xml:"CreateKeyPairResponse"`
	RequestId      string   `xml:"requestId"`
	KeyName        string   `xml:"keyName"`
	KeyFingerprint string   `xml:"keyFingerprint"`
	KeyMaterial    string   `xml:"keyMaterial"`
	KeyPairId      string   `xml:"keyPairId"`
}

// ImportKeyPairResponse は ImportKeyPair アクションのレスポンスです。
type ImportKeyPairResponse struct {
	XMLName        xml.Name `xml:"ImportKeyPairResponse"`
	RequestId      string   `xml:"requestId"`
	KeyName        string   `xml:"keyName"`
	KeyFingerprint string   `xml:"keyFingerprint"`
	KeyPairId      string   `xml:"keyPairId"`
}

// DeleteKeyPairResponse は DeleteKeyPair アクションのレスポンスです。
type DeleteKeyPairResponse struct {
	XMLName   xml.Name `xml:"DeleteKeyPairResponse"`
	RequestId string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

// DescribeKeyPairsResponse は DescribeKeyPairs アクションのレスポンスです。
type DescribeKeyPairsResponse struct {
	XMLName   xml.Name      `xml:"DescribeKeyPairsResponse"`
	RequestId string        `xml:"requestId"`
	KeySet    []KeyPairItem `xml:"keySet>item"`
}

