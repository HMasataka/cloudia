package gcs

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestConvertListBucketsXMLToJSON(t *testing.T) {
	// Given: a valid S3 ListAllMyBucketsResult XML with two buckets
	xmlBody := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListAllMyBucketsResult>
  <Buckets>
    <Bucket>
      <Name>alpha</Name>
      <CreationDate>2024-03-01T10:00:00Z</CreationDate>
    </Bucket>
    <Bucket>
      <Name>beta</Name>
      <CreationDate>2024-06-15T12:30:00Z</CreationDate>
    </Bucket>
  </Buckets>
</ListAllMyBucketsResult>`)

	baseURL := "http://localhost:8080"

	jsonBody, err := convertListBucketsXMLToJSON(xmlBody, baseURL)
	if err != nil {
		t.Fatalf("convertListBucketsXMLToJSON() error = %v, want nil", err)
	}

	var result gcsBucketList
	if err := json.Unmarshal(jsonBody, &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if result.Kind != "storage#buckets" {
		t.Errorf("Kind = %q, want %q", result.Kind, "storage#buckets")
	}

	if len(result.Items) != 2 {
		t.Fatalf("Items len = %d, want 2", len(result.Items))
	}

	alpha := result.Items[0]
	if alpha.Kind != "storage#bucket" {
		t.Errorf("Items[0].Kind = %q, want %q", alpha.Kind, "storage#bucket")
	}
	if alpha.Name != "alpha" {
		t.Errorf("Items[0].Name = %q, want %q", alpha.Name, "alpha")
	}
	if alpha.ID != "alpha" {
		t.Errorf("Items[0].ID = %q, want %q", alpha.ID, "alpha")
	}
	expectedSelfLink := baseURL + "/storage/v1/b/alpha"
	if alpha.SelfLink != expectedSelfLink {
		t.Errorf("Items[0].SelfLink = %q, want %q", alpha.SelfLink, expectedSelfLink)
	}
	if alpha.Location != "US" {
		t.Errorf("Items[0].Location = %q, want %q", alpha.Location, "US")
	}
	if alpha.StorageClass != "STANDARD" {
		t.Errorf("Items[0].StorageClass = %q, want %q", alpha.StorageClass, "STANDARD")
	}
	if alpha.TimeCreated != "2024-03-01T10:00:00Z" {
		t.Errorf("Items[0].TimeCreated = %q, want %q", alpha.TimeCreated, "2024-03-01T10:00:00Z")
	}
}

func TestConvertListBucketsXMLToJSON_EmptyBuckets(t *testing.T) {
	// Given: a valid XML with no buckets
	xmlBody := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListAllMyBucketsResult>
  <Buckets></Buckets>
</ListAllMyBucketsResult>`)

	jsonBody, err := convertListBucketsXMLToJSON(xmlBody, "http://localhost:8080")
	if err != nil {
		t.Fatalf("convertListBucketsXMLToJSON() error = %v, want nil", err)
	}

	var result gcsBucketList
	if err := json.Unmarshal(jsonBody, &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if result.Kind != "storage#buckets" {
		t.Errorf("Kind = %q, want %q", result.Kind, "storage#buckets")
	}
	if len(result.Items) != 0 {
		t.Errorf("Items len = %d, want 0", len(result.Items))
	}
}

func TestConvertListBucketsXMLToJSON_InvalidXML(t *testing.T) {
	// Given: invalid XML
	// When: convertListBucketsXMLToJSON is called
	// Then: an error is returned

	_, err := convertListBucketsXMLToJSON([]byte("not xml"), "http://localhost:8080")
	if err == nil {
		t.Error("convertListBucketsXMLToJSON() with invalid XML should return error, got nil")
	}
}

func TestConvertBucketInfoToJSON(t *testing.T) {
	// Given: S3 HEAD response headers with no Last-Modified
	// When: convertBucketInfoToJSON is called
	// Then: a GCS bucket JSON with the bucket name and default fields is returned

	header := http.Header{}
	bucket := "my-bucket"
	baseURL := "http://localhost:8080"

	jsonBody, err := convertBucketInfoToJSON(header, bucket, baseURL)
	if err != nil {
		t.Fatalf("convertBucketInfoToJSON() error = %v, want nil", err)
	}

	var result gcsBucket
	if err := json.Unmarshal(jsonBody, &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if result.Kind != "storage#bucket" {
		t.Errorf("Kind = %q, want %q", result.Kind, "storage#bucket")
	}
	if result.Name != bucket {
		t.Errorf("Name = %q, want %q", result.Name, bucket)
	}
	if result.ID != bucket {
		t.Errorf("ID = %q, want %q", result.ID, bucket)
	}
	expectedSelfLink := baseURL + "/storage/v1/b/" + bucket
	if result.SelfLink != expectedSelfLink {
		t.Errorf("SelfLink = %q, want %q", result.SelfLink, expectedSelfLink)
	}
	if result.Location != "US" {
		t.Errorf("Location = %q, want %q", result.Location, "US")
	}
	if result.StorageClass != "STANDARD" {
		t.Errorf("StorageClass = %q, want %q", result.StorageClass, "STANDARD")
	}
	if result.TimeCreated == "" {
		t.Error("TimeCreated should not be empty")
	}
}

func TestConvertBucketInfoToJSON_WithLastModified(t *testing.T) {
	// Given: S3 HEAD headers with a Last-Modified value
	// When: convertBucketInfoToJSON is called
	// Then: TimeCreated and Updated reflect the Last-Modified time

	header := http.Header{}
	header.Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")

	jsonBody, err := convertBucketInfoToJSON(header, "my-bucket", "http://localhost:8080")
	if err != nil {
		t.Fatalf("convertBucketInfoToJSON() error = %v, want nil", err)
	}

	var result gcsBucket
	if err := json.Unmarshal(jsonBody, &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	expected := "2024-01-01T00:00:00Z"
	if result.TimeCreated != expected {
		t.Errorf("TimeCreated = %q, want %q", result.TimeCreated, expected)
	}
	if result.Updated != expected {
		t.Errorf("Updated = %q, want %q", result.Updated, expected)
	}
}
