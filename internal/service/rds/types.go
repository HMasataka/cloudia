package rds

import "encoding/xml"

// RDS XML namespace for RDS responses.
const xmlNamespace = "https://rds.amazonaws.com/doc/2014-10-31/"

// State Store kinds for RDS resources.
const (
	kindDBInstance = "aws:rds:db-instance"
	kindDBSnapshot = "aws:rds:db-snapshot"
)

// DB instance status values.
const (
	statusAvailable = "available"
	statusDeleting  = "deleting"
)

// DBInstanceMember はレスポンス内の DBInstance 要素です。
type DBInstanceMember struct {
	DBInstanceIdentifier string `xml:"DBInstanceIdentifier"`
	DBInstanceClass      string `xml:"DBInstanceClass"`
	Engine               string `xml:"Engine"`
	EngineVersion        string `xml:"EngineVersion"`
	DBInstanceStatus     string `xml:"DBInstanceStatus"`
	MasterUsername       string `xml:"MasterUsername"`
	DBName               string `xml:"DBName,omitempty"`
	Endpoint             Endpoint `xml:"Endpoint"`
	AllocatedStorage     int    `xml:"AllocatedStorage"`
	MultiAZ              bool   `xml:"MultiAZ"`
	PubliclyAccessible   bool   `xml:"PubliclyAccessible"`
}

// Endpoint はDBインスタンスの接続エンドポイントです。
type Endpoint struct {
	Address string `xml:"Address"`
	Port    int    `xml:"Port"`
}

// CreateDBInstanceResult は CreateDBInstance のレスポンスです。
type CreateDBInstanceResult struct {
	XMLName    xml.Name         `xml:"CreateDBInstanceResponse"`
	RequestID  string           `xml:"ResponseMetadata>RequestId"`
	DBInstance DBInstanceMember `xml:"CreateDBInstanceResult>DBInstance"`
}

// DeleteDBInstanceResult は DeleteDBInstance のレスポンスです。
type DeleteDBInstanceResult struct {
	XMLName    xml.Name         `xml:"DeleteDBInstanceResponse"`
	RequestID  string           `xml:"ResponseMetadata>RequestId"`
	DBInstance DBInstanceMember `xml:"DeleteDBInstanceResult>DBInstance"`
}

// ModifyDBInstanceResult は ModifyDBInstance のレスポンスです。
type ModifyDBInstanceResult struct {
	XMLName    xml.Name         `xml:"ModifyDBInstanceResponse"`
	RequestID  string           `xml:"ResponseMetadata>RequestId"`
	DBInstance DBInstanceMember `xml:"ModifyDBInstanceResult>DBInstance"`
}

// DescribeDBInstancesResult は DescribeDBInstances のレスポンスです。
type DescribeDBInstancesResult struct {
	XMLName     xml.Name           `xml:"DescribeDBInstancesResponse"`
	RequestID   string             `xml:"ResponseMetadata>RequestId"`
	DBInstances []DBInstanceMember `xml:"DescribeDBInstancesResult>DBInstances>DBInstance"`
}

// DBSnapshotMember はレスポンス内の DBSnapshot 要素です。
type DBSnapshotMember struct {
	DBSnapshotIdentifier string `xml:"DBSnapshotIdentifier"`
	DBInstanceIdentifier string `xml:"DBInstanceIdentifier"`
	Engine               string `xml:"Engine"`
	EngineVersion        string `xml:"EngineVersion"`
	Status               string `xml:"Status"`
	SnapshotType         string `xml:"SnapshotType"`
	AllocatedStorage     int    `xml:"AllocatedStorage"`
}

// CreateDBSnapshotResult は CreateDBSnapshot のレスポンスです。
type CreateDBSnapshotResult struct {
	XMLName    xml.Name         `xml:"CreateDBSnapshotResponse"`
	RequestID  string           `xml:"ResponseMetadata>RequestId"`
	DBSnapshot DBSnapshotMember `xml:"CreateDBSnapshotResult>DBSnapshot"`
}

// DeleteDBSnapshotResult は DeleteDBSnapshot のレスポンスです。
type DeleteDBSnapshotResult struct {
	XMLName    xml.Name         `xml:"DeleteDBSnapshotResponse"`
	RequestID  string           `xml:"ResponseMetadata>RequestId"`
	DBSnapshot DBSnapshotMember `xml:"DeleteDBSnapshotResult>DBSnapshot"`
}

// DescribeDBSnapshotsResult は DescribeDBSnapshots のレスポンスです。
type DescribeDBSnapshotsResult struct {
	XMLName     xml.Name           `xml:"DescribeDBSnapshotsResponse"`
	RequestID   string             `xml:"ResponseMetadata>RequestId"`
	DBSnapshots []DBSnapshotMember `xml:"DescribeDBSnapshotsResult>DBSnapshots>DBSnapshot"`
}
