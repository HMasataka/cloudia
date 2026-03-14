package auth

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/HMasataka/cloudia/internal/config"
)

const sigV4Prefix = "AWS4-HMAC-SHA256"

// SigV4Verifier validates AWS Signature Version 4 Authorization headers.
type SigV4Verifier struct {
	cfg config.AWSAuthConfig
}

// NewSigV4Verifier creates a new SigV4Verifier.
func NewSigV4Verifier(cfg config.AWSAuthConfig) *SigV4Verifier {
	return &SigV4Verifier{cfg: cfg}
}

// CanHandle returns true for AWS4-HMAC-SHA256 Authorization headers.
func (v *SigV4Verifier) CanHandle(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Authorization"), sigV4Prefix)
}

// sigV4Components holds parsed Authorization header components.
type sigV4Components struct {
	Algorithm     string
	AccessKey     string
	Date          string
	Region        string
	Service       string
	SignedHeaders string
	Signature     string
}

// parseAuthorization parses an AWS4-HMAC-SHA256 Authorization header.
func parseAuthorization(authHeader string) (sigV4Components, error) {
	// 例: AWS4-HMAC-SHA256 Credential=access-key/20260315/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=abc123
	if !strings.HasPrefix(authHeader, sigV4Prefix) {
		return sigV4Components{}, fmt.Errorf("auth: invalid algorithm prefix")
	}

	rest := strings.TrimPrefix(authHeader, sigV4Prefix)
	rest = strings.TrimSpace(rest)

	params := map[string]string{}
	for _, part := range strings.Split(rest, ",") {
		part = strings.TrimSpace(part)
		idx := strings.IndexByte(part, '=')
		if idx < 0 {
			return sigV4Components{}, fmt.Errorf("auth: malformed authorization parameter: %q", part)
		}
		key := strings.TrimSpace(part[:idx])
		val := strings.TrimSpace(part[idx+1:])
		params[key] = val
	}

	credential, ok := params["Credential"]
	if !ok {
		return sigV4Components{}, fmt.Errorf("auth: missing Credential in Authorization header")
	}
	signedHeaders, ok := params["SignedHeaders"]
	if !ok {
		return sigV4Components{}, fmt.Errorf("auth: missing SignedHeaders in Authorization header")
	}
	signature, ok := params["Signature"]
	if !ok {
		return sigV4Components{}, fmt.Errorf("auth: missing Signature in Authorization header")
	}

	// Credential: access-key/20260315/us-east-1/s3/aws4_request
	credParts := strings.SplitN(credential, "/", 5)
	if len(credParts) != 5 {
		return sigV4Components{}, fmt.Errorf("auth: malformed Credential scope: %q", credential)
	}
	if credParts[4] != "aws4_request" {
		return sigV4Components{}, fmt.Errorf("auth: invalid credential scope terminator: %q", credParts[4])
	}

	return sigV4Components{
		Algorithm:     sigV4Prefix,
		AccessKey:     credParts[0],
		Date:          credParts[1],
		Region:        credParts[2],
		Service:       credParts[3],
		SignedHeaders:  signedHeaders,
		Signature:     signature,
	}, nil
}

// Verify validates the request and returns AuthResult.
func (v *SigV4Verifier) Verify(r *http.Request) (AuthResult, error) {
	authHeader := r.Header.Get("Authorization")
	components, err := parseAuthorization(authHeader)
	if err != nil {
		return AuthResult{}, err
	}

	if v.cfg.AccessKey == "" {
		return AuthResult{}, fmt.Errorf("auth: AccessKey is not configured")
	}

	if subtle.ConstantTimeCompare([]byte(components.AccessKey), []byte(v.cfg.AccessKey)) != 1 {
		return AuthResult{}, fmt.Errorf("auth: SignatureDoesNotMatch: access key %q does not match", components.AccessKey)
	}

	return AuthResult{
		Provider: "aws",
		Region:   components.Region,
		Service:  components.Service,
	}, nil
}
