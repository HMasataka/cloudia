package sqs

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	protocolaws "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// createQueue は CreateQueue アクションを処理します。
func (s *SQSService) createQueue(ctx context.Context, req service.Request) (service.Response, error) {
	var r createQueueRequest
	if err := decodeBody(req.Body, &r); err != nil {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "invalid request body")
	}

	if r.QueueName == "" {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "QueueName is required")
	}

	// FIFO キューの検証: FIFO キューは .fifo で終わる必要がある
	isFIFO := strings.HasSuffix(r.QueueName, ".fifo")
	if r.Attributes != nil {
		if v, ok := r.Attributes["FifoQueue"]; ok && v == "true" && !isFIFO {
			return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "The name of a FIFO queue can only include alphanumeric characters, hyphens, or underscores, must end with .fifo suffix")
		}
	}

	// 同名キューの存在チェック
	existing, err := s.store.Get(ctx, resourceKind, r.QueueName)
	if err == nil && existing != nil {
		return errorResponse(http.StatusBadRequest, "QueueAlreadyExists", fmt.Sprintf("A queue already exists with the same name: %s", r.QueueName))
	}
	if err != nil && !isNotFound(err) {
		return errorResponse(http.StatusInternalServerError, "InternalError", err.Error())
	}

	url := queueURL(s.cfg.AccountID, r.QueueName)
	now := time.Now()

	resource := &models.Resource{
		Kind:      resourceKind,
		ID:        r.QueueName,
		Provider:  "aws",
		Service:   "sqs",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"QueueName":        r.QueueName,
			"QueueUrl":         url,
			"CreatedTimestamp": strconv.FormatInt(now.Unix(), 10),
		},
	}

	if len(r.Tags) > 0 {
		resource.Tags = r.Tags
	}

	if err := s.store.Put(ctx, resource); err != nil {
		return errorResponse(http.StatusInternalServerError, "InternalError", err.Error())
	}

	return jsonResponse(http.StatusOK, createQueueResponse{QueueUrl: url})
}

// deleteQueue は DeleteQueue アクションを処理します。
func (s *SQSService) deleteQueue(ctx context.Context, req service.Request) (service.Response, error) {
	var r deleteQueueRequest
	if err := decodeBody(req.Body, &r); err != nil {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "invalid request body")
	}

	if r.QueueUrl == "" {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "QueueUrl is required")
	}

	queueName := queueNameFromURL(r.QueueUrl, s.cfg.AccountID)
	if queueName == "" {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "invalid QueueUrl")
	}

	if err := s.store.Delete(ctx, resourceKind, queueName); err != nil {
		return errorResponse(http.StatusInternalServerError, "InternalError", err.Error())
	}

	return service.Response{StatusCode: http.StatusOK, ContentType: contentType, Body: []byte("{}")}, nil
}

// getQueueUrl は GetQueueUrl アクションを処理します。
func (s *SQSService) getQueueUrl(ctx context.Context, req service.Request) (service.Response, error) {
	var r getQueueUrlRequest
	if err := decodeBody(req.Body, &r); err != nil {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "invalid request body")
	}

	if r.QueueName == "" {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "QueueName is required")
	}

	resource, err := s.store.Get(ctx, resourceKind, r.QueueName)
	if err != nil {
		if isNotFound(err) {
			return errorResponse(http.StatusBadRequest, "AWS.SimpleQueueService.NonExistentQueue", fmt.Sprintf("The specified queue does not exist: %s", r.QueueName))
		}
		return errorResponse(http.StatusInternalServerError, "InternalError", err.Error())
	}

	url, _ := resource.Spec["QueueUrl"].(string)
	return jsonResponse(http.StatusOK, getQueueUrlResponse{QueueUrl: url})
}

// getQueueAttributes は GetQueueAttributes アクションを処理します。
func (s *SQSService) getQueueAttributes(ctx context.Context, req service.Request) (service.Response, error) {
	var r getQueueAttributesRequest
	if err := decodeBody(req.Body, &r); err != nil {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "invalid request body")
	}

	if r.QueueUrl == "" {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "QueueUrl is required")
	}

	queueName := queueNameFromURL(r.QueueUrl, s.cfg.AccountID)
	if queueName == "" {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "invalid QueueUrl")
	}

	resource, err := s.store.Get(ctx, resourceKind, queueName)
	if err != nil {
		if isNotFound(err) {
			return errorResponse(http.StatusBadRequest, "AWS.SimpleQueueService.NonExistentQueue", fmt.Sprintf("The specified queue does not exist: %s", queueName))
		}
		return errorResponse(http.StatusInternalServerError, "InternalError", err.Error())
	}

	createdTimestamp, _ := resource.Spec["CreatedTimestamp"].(string)
	if createdTimestamp == "" {
		createdTimestamp = strconv.FormatInt(resource.CreatedAt.Unix(), 10)
	}

	arn := protocolaws.FormatARN("aws", "sqs", s.cfg.Region, s.cfg.AccountID, queueName)

	attrs := map[string]string{
		"QueueArn":                     arn,
		"CreatedTimestamp":             createdTimestamp,
		"ApproximateNumberOfMessages":  "0",
		"VisibilityTimeout":            "30",
		"MaximumMessageSize":           "262144",
		"MessageRetentionPeriod":       "345600",
	}

	return jsonResponse(http.StatusOK, getQueueAttributesResponse{Attributes: attrs})
}

// listQueues は ListQueues アクションを処理します。
func (s *SQSService) listQueues(ctx context.Context, req service.Request) (service.Response, error) {
	var r listQueuesRequest
	if err := decodeBody(req.Body, &r); err != nil {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "invalid request body")
	}

	resources, err := s.store.List(ctx, resourceKind, state.Filter{})
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "InternalError", err.Error())
	}

	var urls []string
	for _, res := range resources {
		if r.QueueNamePrefix != "" && !strings.HasPrefix(res.ID, r.QueueNamePrefix) {
			continue
		}
		if url, ok := res.Spec["QueueUrl"].(string); ok {
			urls = append(urls, url)
		}
	}

	if urls == nil {
		urls = []string{}
	}

	return jsonResponse(http.StatusOK, listQueuesResponse{QueueUrls: urls})
}

// tagQueue は TagQueue アクションを処理します。
func (s *SQSService) tagQueue(ctx context.Context, req service.Request) (service.Response, error) {
	var r tagQueueRequest
	if err := decodeBody(req.Body, &r); err != nil {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "invalid request body")
	}

	if r.QueueUrl == "" {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "QueueUrl is required")
	}

	queueName := queueNameFromURL(r.QueueUrl, s.cfg.AccountID)
	if queueName == "" {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue", "invalid QueueUrl")
	}

	resource, err := s.store.Get(ctx, resourceKind, queueName)
	if err != nil {
		if isNotFound(err) {
			return errorResponse(http.StatusBadRequest, "AWS.SimpleQueueService.NonExistentQueue", fmt.Sprintf("The specified queue does not exist: %s", queueName))
		}
		return errorResponse(http.StatusInternalServerError, "InternalError", err.Error())
	}

	if resource.Tags == nil {
		resource.Tags = make(map[string]string)
	}
	for k, v := range r.Tags {
		resource.Tags[k] = v
	}
	resource.UpdatedAt = time.Now()

	if err := s.store.Put(ctx, resource); err != nil {
		return errorResponse(http.StatusInternalServerError, "InternalError", err.Error())
	}

	return service.Response{StatusCode: http.StatusOK, ContentType: contentType, Body: []byte("{}")}, nil
}
