package aws

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecodeJSONRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		target          string
		contentType     string
		body            string
		wantService     string
		wantAction      string
		wantContentType string
		wantErr         bool
	}{
		{
			name:            "DynamoDB PutItem",
			target:          "DynamoDB_20120810.PutItem",
			contentType:     "application/x-amz-json-1.0",
			body:            `{"TableName":"test"}`,
			wantService:     "dynamodb",
			wantAction:      "PutItem",
			wantContentType: "application/x-amz-json-1.0",
		},
		{
			name:            "SQS SendMessage json-1.1",
			target:          "AmazonSQS.SendMessage",
			contentType:     "application/x-amz-json-1.1",
			body:            `{"QueueUrl":"https://example.com"}`,
			wantService:     "sqs",
			wantAction:      "SendMessage",
			wantContentType: "application/x-amz-json-1.1",
		},
		{
			name:        "Kinesis PutRecord",
			target:      "Kinesis_20131202.PutRecord",
			contentType: "application/x-amz-json-1.1",
			body:        `{}`,
			wantService: "kinesis",
			wantAction:  "PutRecord",
		},
		{
			name:    "missing X-Amz-Target",
			target:  "",
			wantErr: true,
		},
		{
			name:    "invalid target format no dot",
			target:  "DynamoDB_20120810",
			wantErr: true,
		},
		{
			name:    "empty action",
			target:  "DynamoDB_20120810.",
			wantErr: true,
		},
		{
			name:    "unknown target prefix",
			target:  "UnknownService_20230101.SomeAction",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var bodyReader *bytes.Reader
			if tt.body != "" {
				bodyReader = bytes.NewReader([]byte(tt.body))
			} else {
				bodyReader = bytes.NewReader(nil)
			}

			req := httptest.NewRequest(http.MethodPost, "/", bodyReader)
			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			got, err := DecodeJSONRequest(req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DecodeJSONRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if got.Provider != "aws" {
				t.Errorf("Provider = %q, want %q", got.Provider, "aws")
			}
			if got.Service != tt.wantService {
				t.Errorf("Service = %q, want %q", got.Service, tt.wantService)
			}
			if got.Action != tt.wantAction {
				t.Errorf("Action = %q, want %q", got.Action, tt.wantAction)
			}
			if string(got.Body) != tt.body {
				t.Errorf("Body = %q, want %q", string(got.Body), tt.body)
			}
			if tt.wantContentType != "" {
				if got.Headers["Content-Type"] != tt.wantContentType {
					t.Errorf("Headers[Content-Type] = %q, want %q", got.Headers["Content-Type"], tt.wantContentType)
				}
			}
		})
	}
}
