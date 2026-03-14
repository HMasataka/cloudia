package aws

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDecodeQueryRequest(t *testing.T) {
	t.Run("正常系: Action と Version を抽出する", func(t *testing.T) {
		body := "Action=DescribeInstances&Version=2016-11-15"
		req := newRequest(body)

		got, err := DecodeQueryRequest(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Action != "DescribeInstances" {
			t.Errorf("Action = %q, want %q", got.Action, "DescribeInstances")
		}
		if got.Service != "" {
			t.Errorf("Service should not be set by DecodeQueryRequest, got %q", got.Service)
		}
	})

	t.Run("正常系: ネストパラメータをフラットキーとして Params に格納する", func(t *testing.T) {
		body := "Action=DescribeVpcs&Filter.1.Name=vpc-id&Filter.1.Value.1=vpc-123&Version=2016-11-15"
		req := newRequest(body)

		got, err := DecodeQueryRequest(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Action != "DescribeVpcs" {
			t.Errorf("Action = %q, want %q", got.Action, "DescribeVpcs")
		}
		if got.Params["Filter.1.Name"] != "vpc-id" {
			t.Errorf("Params[Filter.1.Name] = %q, want %q", got.Params["Filter.1.Name"], "vpc-id")
		}
		if got.Params["Filter.1.Value.1"] != "vpc-123" {
			t.Errorf("Params[Filter.1.Value.1] = %q, want %q", got.Params["Filter.1.Value.1"], "vpc-123")
		}
	})

	t.Run("異常系: Action が欠落している場合 MissingAction エラーを返す", func(t *testing.T) {
		body := "Version=2016-11-15&Filter.1.Name=vpc-id"
		req := newRequest(body)

		_, err := DecodeQueryRequest(req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err != ErrMissingAction {
			t.Errorf("err = %v, want %v", err, ErrMissingAction)
		}
	})

	t.Run("異常系: 空ボディの場合エラーを返す", func(t *testing.T) {
		req := newRequest("")

		_, err := DecodeQueryRequest(req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err != ErrEmptyBody {
			t.Errorf("err = %v, want %v", err, ErrEmptyBody)
		}
	})

	t.Run("正常系: Action と Version は Params に含まれない", func(t *testing.T) {
		body := "Action=RunInstances&Version=2016-11-15&ImageId=ami-12345"
		req := newRequest(body)

		got, err := DecodeQueryRequest(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := got.Params["Action"]; ok {
			t.Error("Params should not contain Action key")
		}
		if got.Params["ImageId"] != "ami-12345" {
			t.Errorf("Params[ImageId] = %q, want %q", got.Params["ImageId"], "ami-12345")
		}
	})

	t.Run("正常系: Service フィールドはこの関数では設定されない", func(t *testing.T) {
		body := "Action=DescribeInstances"
		req := newRequest(body)

		got, err := DecodeQueryRequest(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Service != "" {
			t.Errorf("Service = %q, want empty string", got.Service)
		}
	})
}

func newRequest(body string) *http.Request {
	var bodyReader io.Reader
	if body == "" {
		bodyReader = strings.NewReader("")
	} else {
		bodyReader = strings.NewReader(body)
	}
	req, _ := http.NewRequest(http.MethodPost, "/", bodyReader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}
