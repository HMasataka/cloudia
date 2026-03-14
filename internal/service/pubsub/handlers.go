package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/service"
)

// createTopic は topic 作成ハンドラです。
func (s *PubSubService) createTopic(_ context.Context, req service.Request, project, topicName string) (service.Response, error) {
	if err := validateResourceName(topicName); err != nil {
		return pubsubErrorResponse(http.StatusBadRequest, err.Error())
	}

	var r createTopicRequest
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &r); err != nil {
			return pubsubErrorResponse(http.StatusBadRequest, "invalid request body")
		}
	}

	fullName := topicFullName(project, topicName)

	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()

	if _, exists := s.backend.topics[fullName]; exists {
		return pubsubErrorResponse(http.StatusConflict, fmt.Sprintf("Topic already exists: %s", fullName))
	}

	s.backend.topics[fullName] = &Topic{
		Name:      fullName,
		CreatedAt: time.Now(),
	}

	return jsonResponse(http.StatusOK, topicResponse{Name: fullName, Labels: r.Labels})
}

// getTopic は topic 取得ハンドラです。
func (s *PubSubService) getTopic(_ context.Context, project, topicName string) (service.Response, error) {
	fullName := topicFullName(project, topicName)

	s.backend.mu.RLock()
	defer s.backend.mu.RUnlock()

	if _, exists := s.backend.topics[fullName]; !exists {
		return pubsubErrorResponse(http.StatusNotFound, fmt.Sprintf("Topic not found: %s", fullName))
	}

	return jsonResponse(http.StatusOK, topicResponse{Name: fullName})
}

// listTopics は topic 一覧ハンドラです。
func (s *PubSubService) listTopics(_ context.Context, project string) (service.Response, error) {
	prefix := fmt.Sprintf("projects/%s/topics/", project)

	s.backend.mu.RLock()
	defer s.backend.mu.RUnlock()

	var topics []topicResponse
	for name := range s.backend.topics {
		if len(name) > len(prefix) && name[:len(prefix)] == prefix {
			topics = append(topics, topicResponse{Name: name})
		}
	}

	if topics == nil {
		topics = []topicResponse{}
	}

	return jsonResponse(http.StatusOK, listTopicsResponse{Topics: topics})
}

// deleteTopic は topic 削除ハンドラです。topic に紐づく subscription をカスケード削除します。
func (s *PubSubService) deleteTopic(_ context.Context, project, topicName string) (service.Response, error) {
	fullName := topicFullName(project, topicName)

	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()

	if _, exists := s.backend.topics[fullName]; !exists {
		return pubsubErrorResponse(http.StatusNotFound, fmt.Sprintf("Topic not found: %s", fullName))
	}

	delete(s.backend.topics, fullName)

	// カスケード削除: この topic に紐づく subscription を削除
	for subName, sub := range s.backend.subscriptions {
		if sub.Topic == fullName {
			delete(s.backend.subscriptions, subName)
			delete(s.backend.messages, subName)
		}
	}

	return service.Response{StatusCode: http.StatusOK, ContentType: contentType, Body: []byte("{}")}, nil
}

// publish はメッセージを topic に publish し、紐づく全 subscription に fan-out します。
func (s *PubSubService) publish(_ context.Context, req service.Request, project, topicName string) (service.Response, error) {
	var r publishRequest
	if err := json.Unmarshal(req.Body, &r); err != nil {
		return pubsubErrorResponse(http.StatusBadRequest, "invalid request body")
	}

	if len(r.Messages) == 0 {
		return pubsubErrorResponse(http.StatusBadRequest, "messages is required")
	}

	// メッセージサイズ検証
	totalSize := 0
	for _, msg := range r.Messages {
		totalSize += len(msg.Data)
		if len(msg.Data) > maxMessageSize {
			return pubsubErrorResponse(http.StatusBadRequest,
				fmt.Sprintf("message data size %d exceeds limit %d", len(msg.Data), maxMessageSize))
		}
	}
	if totalSize > maxMessageSize {
		return pubsubErrorResponse(http.StatusBadRequest,
			fmt.Sprintf("total message data size %d exceeds limit %d", totalSize, maxMessageSize))
	}

	fullTopicName := topicFullName(project, topicName)

	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()

	if _, exists := s.backend.topics[fullTopicName]; !exists {
		return pubsubErrorResponse(http.StatusNotFound, fmt.Sprintf("Topic not found: %s", fullTopicName))
	}

	var messageIDs []string
	publishTime := time.Now()

	for _, msg := range r.Messages {
		msgID := s.backend.nextMessageID()
		messageIDs = append(messageIDs, msgID)

		// fan-out: この topic に紐づく全 subscription にメッセージをコピー
		for subName, sub := range s.backend.subscriptions {
			if sub.Topic != fullTopicName {
				continue
			}

			// subscription あたりの上限チェック
			if len(s.backend.messages[subName]) >= maxMessagesPerSubscription {
				s.logger.Warn("pubsub: subscription message limit reached, dropping message",
					zap.String("subscription", subName),
				)
				continue
			}

			ackID := fmt.Sprintf("%s-%s", msgID, subName)
			pending := &PendingMessage{
				MessageID:   msgID,
				Data:        msg.Data,
				Attributes:  msg.Attributes,
				PublishTime: publishTime,
				AckID:       ackID,
			}
			s.backend.messages[subName] = append(s.backend.messages[subName], pending)
		}
	}

	return jsonResponse(http.StatusOK, publishResponse{MessageIDs: messageIDs})
}

// createSubscription はサブスクリプション作成ハンドラです。
func (s *PubSubService) createSubscription(_ context.Context, req service.Request, project, subName string) (service.Response, error) {
	if err := validateResourceName(subName); err != nil {
		return pubsubErrorResponse(http.StatusBadRequest, err.Error())
	}

	var r createSubscriptionRequest
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &r); err != nil {
			return pubsubErrorResponse(http.StatusBadRequest, "invalid request body")
		}
	}

	if r.Topic == "" {
		return pubsubErrorResponse(http.StatusBadRequest, "topic is required")
	}

	fullSubName := subscriptionFullName(project, subName)

	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()

	// topic の存在確認
	if _, exists := s.backend.topics[r.Topic]; !exists {
		return pubsubErrorResponse(http.StatusNotFound, fmt.Sprintf("Topic not found: %s", r.Topic))
	}

	if _, exists := s.backend.subscriptions[fullSubName]; exists {
		return pubsubErrorResponse(http.StatusConflict, fmt.Sprintf("Subscription already exists: %s", fullSubName))
	}

	ackDeadline := r.AckDeadlineSeconds
	if ackDeadline == 0 {
		ackDeadline = 10
	}

	s.backend.subscriptions[fullSubName] = &Subscription{
		Name:        fullSubName,
		Topic:       r.Topic,
		AckDeadline: ackDeadline,
		CreatedAt:   time.Now(),
	}
	s.backend.messages[fullSubName] = nil

	return jsonResponse(http.StatusOK, subscriptionResponse{
		Name:               fullSubName,
		Topic:              r.Topic,
		AckDeadlineSeconds: ackDeadline,
		Labels:             r.Labels,
	})
}

// getSubscription はサブスクリプション取得ハンドラです。
func (s *PubSubService) getSubscription(_ context.Context, project, subName string) (service.Response, error) {
	fullSubName := subscriptionFullName(project, subName)

	s.backend.mu.RLock()
	defer s.backend.mu.RUnlock()

	sub, exists := s.backend.subscriptions[fullSubName]
	if !exists {
		return pubsubErrorResponse(http.StatusNotFound, fmt.Sprintf("Subscription not found: %s", fullSubName))
	}

	return jsonResponse(http.StatusOK, subscriptionResponse{
		Name:               sub.Name,
		Topic:              sub.Topic,
		AckDeadlineSeconds: sub.AckDeadline,
	})
}

// listSubscriptions はサブスクリプション一覧ハンドラです。
func (s *PubSubService) listSubscriptions(_ context.Context, project string) (service.Response, error) {
	prefix := fmt.Sprintf("projects/%s/subscriptions/", project)

	s.backend.mu.RLock()
	defer s.backend.mu.RUnlock()

	var subs []subscriptionResponse
	for name, sub := range s.backend.subscriptions {
		if len(name) > len(prefix) && name[:len(prefix)] == prefix {
			subs = append(subs, subscriptionResponse{
				Name:               sub.Name,
				Topic:              sub.Topic,
				AckDeadlineSeconds: sub.AckDeadline,
			})
		}
	}

	if subs == nil {
		subs = []subscriptionResponse{}
	}

	return jsonResponse(http.StatusOK, listSubscriptionsResponse{Subscriptions: subs})
}

// deleteSubscription はサブスクリプション削除ハンドラです。
func (s *PubSubService) deleteSubscription(_ context.Context, project, subName string) (service.Response, error) {
	fullSubName := subscriptionFullName(project, subName)

	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()

	if _, exists := s.backend.subscriptions[fullSubName]; !exists {
		return pubsubErrorResponse(http.StatusNotFound, fmt.Sprintf("Subscription not found: %s", fullSubName))
	}

	delete(s.backend.subscriptions, fullSubName)
	delete(s.backend.messages, fullSubName)

	return service.Response{StatusCode: http.StatusOK, ContentType: contentType, Body: []byte("{}")}, nil
}

// pull はサブスクリプションからメッセージを取得するハンドラです。
// メッセージがなければ即座に空レスポンスを返します。
func (s *PubSubService) pull(_ context.Context, req service.Request, project, subName string) (service.Response, error) {
	var r pullRequest
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &r); err != nil {
			return pubsubErrorResponse(http.StatusBadRequest, "invalid request body")
		}
	}

	if r.MaxMessages == 0 {
		r.MaxMessages = defaultPullMessages
	}
	if r.MaxMessages < 1 {
		return pubsubErrorResponse(http.StatusBadRequest, "maxMessages must be >= 1")
	}
	if r.MaxMessages > maxPullMessages {
		return pubsubErrorResponse(http.StatusBadRequest,
			fmt.Sprintf("maxMessages must be <= %d", maxPullMessages))
	}

	fullSubName := subscriptionFullName(project, subName)

	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()

	if _, exists := s.backend.subscriptions[fullSubName]; !exists {
		return pubsubErrorResponse(http.StatusNotFound, fmt.Sprintf("Subscription not found: %s", fullSubName))
	}

	msgs := s.backend.messages[fullSubName]
	limit := int(r.MaxMessages)
	if limit > len(msgs) {
		limit = len(msgs)
	}

	var received []receivedMessage
	if limit > 0 {
		for _, pending := range msgs[:limit] {
			received = append(received, receivedMessage{
				AckID: pending.AckID,
				Message: pubsubMessage{
					Data:        pending.Data,
					Attributes:  pending.Attributes,
					MessageID:   pending.MessageID,
					PublishTime: pending.PublishTime.UTC().Format(time.RFC3339Nano),
				},
			})
		}
		// 取得したメッセージをキューから削除
		s.backend.messages[fullSubName] = msgs[limit:]
	}

	if received == nil {
		received = []receivedMessage{}
	}

	return jsonResponse(http.StatusOK, pullResponse{ReceivedMessages: received})
}

// acknowledge はメッセージの acknowledge ハンドラです。
// 不正な ackId は無視して 200 を返します (GCP 互換)。
func (s *PubSubService) acknowledge(_ context.Context, req service.Request, project, subName string) (service.Response, error) {
	var r acknowledgeRequest
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &r); err != nil {
			return pubsubErrorResponse(http.StatusBadRequest, "invalid request body")
		}
	}

	fullSubName := subscriptionFullName(project, subName)

	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()

	if _, exists := s.backend.subscriptions[fullSubName]; !exists {
		return pubsubErrorResponse(http.StatusNotFound, fmt.Sprintf("Subscription not found: %s", fullSubName))
	}

	// ackId のセットを構築
	ackSet := make(map[string]struct{}, len(r.AckIDs))
	for _, id := range r.AckIDs {
		ackSet[id] = struct{}{}
	}

	// 不正な ackId は無視して該当メッセージのみ削除 (GCP 互換: 200 を返す)
	msgs := s.backend.messages[fullSubName]
	remaining := msgs[:0]
	for _, msg := range msgs {
		if _, found := ackSet[msg.AckID]; !found {
			remaining = append(remaining, msg)
		}
	}
	s.backend.messages[fullSubName] = remaining

	return service.Response{StatusCode: http.StatusOK, ContentType: contentType, Body: []byte("{}")}, nil
}
