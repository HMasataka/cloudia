package aws

import "testing"

func TestFormatARN_IAM(t *testing.T) {
	t.Parallel()

	// IAM ARN は region が空
	got := FormatARN("aws", "iam", "", "123456789012", "role/MyRole")
	want := "arn:aws:iam::123456789012:role/MyRole"
	if got != want {
		t.Errorf("FormatARN() = %q, want %q", got, want)
	}
}

func TestFormatARN_SQS(t *testing.T) {
	t.Parallel()

	// SQS ARN は region あり
	got := FormatARN("aws", "sqs", "us-east-1", "123456789012", "MyQueue")
	want := "arn:aws:sqs:us-east-1:123456789012:MyQueue"
	if got != want {
		t.Errorf("FormatARN() = %q, want %q", got, want)
	}
}
