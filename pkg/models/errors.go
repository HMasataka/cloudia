package models

import "errors"

// センチネルエラー。errors.Is で判別可能です。
var (
	ErrNotFound            = errors.New("not found")
	ErrAlreadyExists       = errors.New("already exists")
	ErrLimitExceeded       = errors.New("limit exceeded")
	ErrServiceUnavailable  = errors.New("service unavailable")
	ErrUnsupportedOperation = errors.New("unsupported operation")
)
