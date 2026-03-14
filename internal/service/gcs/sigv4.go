package gcs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	sigV4Algorithm = "AWS4-HMAC-SHA256"
	sigV4Region    = "us-east-1"
	sigV4Service   = "s3"
)

// signRequest attaches an AWS Signature Version 4 Authorization header to the request.
// It replaces any existing Authorization header (e.g., a GCS Bearer token).
func signRequest(r *http.Request, accessKey, secretKey string, body []byte) {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	r.Header.Set("X-Amz-Date", amzDate)
	r.Header.Del("Authorization")

	bodyHash := hashSHA256(body)
	r.Header.Set("X-Amz-Content-SHA256", bodyHash)

	// Canonical headers: host + x-amz-content-sha256 + x-amz-date (sorted)
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}

	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		host, bodyHash, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	// Canonical query string
	canonicalQS := canonicalQueryString(r)

	// Canonical request
	canonicalRequest := strings.Join([]string{
		r.Method,
		canonicalURI(r.URL.Path),
		canonicalQS,
		canonicalHeaders,
		signedHeaders,
		bodyHash,
	}, "\n")

	// Credential scope
	credScope := strings.Join([]string{dateStamp, sigV4Region, sigV4Service, "aws4_request"}, "/")

	// String to sign
	stringToSign := strings.Join([]string{
		sigV4Algorithm,
		amzDate,
		credScope,
		hashSHA256([]byte(canonicalRequest)),
	}, "\n")

	// Signing key
	signingKey := deriveSigningKey(secretKey, dateStamp, sigV4Region, sigV4Service)

	// Signature
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// Authorization header
	authHeader := fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		sigV4Algorithm, accessKey, credScope, signedHeaders, signature,
	)
	r.Header.Set("Authorization", authHeader)
}

func hashSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func deriveSigningKey(secretKey, date, region, svc string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(svc))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

func canonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	return path
}

func canonicalQueryString(r *http.Request) string {
	params := r.URL.Query()
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		for _, v := range params[k] {
			parts = append(parts, fmt.Sprintf("%s=%s", uriEncode(k), uriEncode(v)))
		}
	}
	return strings.Join(parts, "&")
}

// uriEncode percent-encodes a string per AWS SigV4 requirements.
func uriEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if isUnreserved(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '~'
}
