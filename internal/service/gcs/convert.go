package gcs

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// S3 XML structures for parsing MinIO responses.

type listAllMyBucketsResult struct {
	XMLName xml.Name  `xml:"ListAllMyBucketsResult"`
	Buckets []s3Bucket `xml:"Buckets>Bucket"`
}

type s3Bucket struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

// GCS JSON response structures.

type gcsBucketList struct {
	Kind  string       `json:"kind"`
	Items []gcsBucket  `json:"items"`
}

type gcsBucket struct {
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	SelfLink     string `json:"selfLink"`
	TimeCreated  string `json:"timeCreated"`
	Updated      string `json:"updated"`
	Location     string `json:"location"`
	StorageClass string `json:"storageClass"`
}

// convertListBucketsXMLToJSON converts an S3 ListAllMyBucketsResult XML body to a
// GCS JSON buckets list response.
func convertListBucketsXMLToJSON(xmlBody []byte, baseURL string) ([]byte, error) {
	var result listAllMyBucketsResult
	if err := xml.Unmarshal(xmlBody, &result); err != nil {
		return nil, fmt.Errorf("gcs convert: unmarshal bucket list XML: %w", err)
	}

	items := make([]gcsBucket, 0, len(result.Buckets))
	for _, b := range result.Buckets {
		items = append(items, bucketFromS3(b, baseURL))
	}

	out := gcsBucketList{
		Kind:  "storage#buckets",
		Items: items,
	}
	return json.Marshal(out)
}

// convertBucketInfoToJSON converts S3 HEAD response headers to a GCS bucket JSON response.
// bucket is the bucket name extracted from the request path.
func convertBucketInfoToJSON(header http.Header, bucket, baseURL string) ([]byte, error) {
	// S3 HEAD /{bucket} does not return a body with creation date in headers,
	// so we use the current time as a best-effort approximation.
	now := time.Now().UTC().Format(time.RFC3339)

	b := gcsBucket{
		Kind:         "storage#bucket",
		ID:           bucket,
		Name:         bucket,
		SelfLink:     fmt.Sprintf("%s/storage/v1/b/%s", baseURL, bucket),
		TimeCreated:  now,
		Updated:      now,
		Location:     "US",
		StorageClass: "STANDARD",
	}

	// Use Last-Modified if available from a GET response.
	if lm := header.Get("Last-Modified"); lm != "" {
		if t, err := http.ParseTime(lm); err == nil {
			b.TimeCreated = t.UTC().Format(time.RFC3339)
			b.Updated = t.UTC().Format(time.RFC3339)
		}
	}

	return json.Marshal(b)
}

// S3 XML structures for parsing object list responses.

type listBucketResult struct {
	XMLName               xml.Name    `xml:"ListBucketResult"`
	Name                  string      `xml:"Name"`
	Contents              []s3Object  `xml:"Contents"`
	NextContinuationToken string      `xml:"NextContinuationToken"`
	IsTruncated           bool        `xml:"IsTruncated"`
}

type s3Object struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	Size         int64  `xml:"Size"`
	ETag         string `xml:"ETag"`
	ContentType  string `xml:"ContentType"`
}

// GCS JSON object structures.

type gcsObjectList struct {
	Kind          string      `json:"kind"`
	Items         []gcsObject `json:"items"`
	NextPageToken string      `json:"nextPageToken,omitempty"`
}

type gcsObject struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Bucket      string `json:"bucket"`
	Size        string `json:"size"`
	ContentType string `json:"contentType"`
	TimeCreated string `json:"timeCreated"`
	Updated     string `json:"updated"`
	MediaLink   string `json:"mediaLink"`
	MD5Hash     string `json:"md5Hash,omitempty"`
	Etag        string `json:"etag,omitempty"`
}

// convertListObjectsXMLToJSON converts an S3 ListBucketResult XML body to a
// GCS JSON objects list response.
func convertListObjectsXMLToJSON(xmlBody []byte, bucket, baseURL string) ([]byte, error) {
	var result listBucketResult
	if err := xml.Unmarshal(xmlBody, &result); err != nil {
		return nil, fmt.Errorf("gcs convert: unmarshal object list XML: %w", err)
	}

	items := make([]gcsObject, 0, len(result.Contents))
	for _, obj := range result.Contents {
		items = append(items, objectFromS3(obj, bucket, baseURL))
	}

	out := gcsObjectList{
		Kind:  "storage#objects",
		Items: items,
	}
	if result.IsTruncated && result.NextContinuationToken != "" {
		out.NextPageToken = result.NextContinuationToken
	}
	return json.Marshal(out)
}

// convertObjectMetadataToJSON converts S3 HEAD response headers to a GCS object JSON response.
func convertObjectMetadataToJSON(bucket, object string, header http.Header, baseURL string) []byte {
	now := time.Now().UTC().Format(time.RFC3339)
	timeCreated := now
	updated := now

	if lm := header.Get("Last-Modified"); lm != "" {
		if t, err := http.ParseTime(lm); err == nil {
			timeCreated = t.UTC().Format(time.RFC3339)
			updated = t.UTC().Format(time.RFC3339)
		}
	}

	size := header.Get("Content-Length")
	if size == "" {
		size = "0"
	}

	contentType := header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	etag := strings.Trim(header.Get("ETag"), `"`)

	obj := gcsObject{
		Kind:        "storage#object",
		ID:          fmt.Sprintf("%s/%s", bucket, object),
		Name:        object,
		Bucket:      bucket,
		Size:        size,
		ContentType: contentType,
		TimeCreated: timeCreated,
		Updated:     updated,
		MediaLink:   fmt.Sprintf("%s/download/storage/v1/b/%s/o/%s?alt=media", baseURL, bucket, object),
		Etag:        etag,
	}

	body, _ := json.Marshal(obj)
	return body
}

// convertObjectUploadToJSON generates a GCS object JSON response after a successful PUT.
func convertObjectUploadToJSON(bucket, object string, header http.Header, baseURL string) []byte {
	now := time.Now().UTC().Format(time.RFC3339)

	size := header.Get("Content-Length")
	if size == "" {
		size = "0"
	}

	contentType := header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	etag := strings.Trim(header.Get("ETag"), `"`)

	obj := gcsObject{
		Kind:        "storage#object",
		ID:          fmt.Sprintf("%s/%s", bucket, object),
		Name:        object,
		Bucket:      bucket,
		Size:        size,
		ContentType: contentType,
		TimeCreated: now,
		Updated:     now,
		MediaLink:   fmt.Sprintf("%s/download/storage/v1/b/%s/o/%s?alt=media", baseURL, bucket, object),
		Etag:        etag,
	}

	body, _ := json.Marshal(obj)
	return body
}

// objectFromS3 builds a gcsObject from an S3 object entry.
func objectFromS3(obj s3Object, bucket, baseURL string) gcsObject {
	timeCreated := obj.LastModified
	if timeCreated == "" {
		timeCreated = time.Now().UTC().Format(time.RFC3339)
	} else {
		if t, err := time.Parse(time.RFC3339, obj.LastModified); err == nil {
			timeCreated = t.UTC().Format(time.RFC3339)
		}
	}

	contentType := obj.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	etag := strings.Trim(obj.ETag, `"`)

	return gcsObject{
		Kind:        "storage#object",
		ID:          fmt.Sprintf("%s/%s", bucket, obj.Key),
		Name:        obj.Key,
		Bucket:      bucket,
		Size:        fmt.Sprintf("%d", obj.Size),
		ContentType: contentType,
		TimeCreated: timeCreated,
		Updated:     timeCreated,
		MediaLink:   fmt.Sprintf("%s/download/storage/v1/b/%s/o/%s?alt=media", baseURL, bucket, obj.Key),
		Etag:        etag,
	}
}

// bucketFromS3 builds a gcsBucket from an S3 bucket entry.
func bucketFromS3(b s3Bucket, baseURL string) gcsBucket {
	timeCreated := b.CreationDate
	if timeCreated == "" {
		timeCreated = time.Now().UTC().Format(time.RFC3339)
	} else {
		// Normalize to RFC3339 if parseable.
		if t, err := time.Parse(time.RFC3339, b.CreationDate); err == nil {
			timeCreated = t.UTC().Format(time.RFC3339)
		}
	}

	return gcsBucket{
		Kind:         "storage#bucket",
		ID:           b.Name,
		Name:         b.Name,
		SelfLink:     fmt.Sprintf("%s/storage/v1/b/%s", baseURL, b.Name),
		TimeCreated:  timeCreated,
		Updated:      timeCreated,
		Location:     "US",
		StorageClass: "STANDARD",
	}
}
