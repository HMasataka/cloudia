package pubsub

import "time"

// Topic は Pub/Sub トピックを表します。
type Topic struct {
	Name      string
	CreatedAt time.Time
}

// Subscription は Pub/Sub サブスクリプションを表します。
type Subscription struct {
	Name        string
	Topic       string // topic の full resource name
	AckDeadline int32  // seconds
	CreatedAt   time.Time
}

// PendingMessage はサブスクリプションの未配信メッセージを表します。
type PendingMessage struct {
	MessageID   string
	Data        []byte
	Attributes  map[string]string
	PublishTime time.Time
	AckID       string
}

// publishRequest は publish リクエストのボディです。
type publishRequest struct {
	Messages []pubsubMessage `json:"messages"`
}

// pubsubMessage は Pub/Sub メッセージを表します。
type pubsubMessage struct {
	Data        []byte            `json:"data"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	MessageID   string            `json:"messageId,omitempty"`
	PublishTime string            `json:"publishTime,omitempty"`
}

// publishResponse は publish レスポンスです。
type publishResponse struct {
	MessageIDs []string `json:"messageIds"`
}

// pullRequest は pull リクエストのボディです。
type pullRequest struct {
	MaxMessages int32 `json:"maxMessages"`
}

// receivedMessage は pull レスポンスの各メッセージです。
type receivedMessage struct {
	AckID   string        `json:"ackId"`
	Message pubsubMessage `json:"message"`
}

// pullResponse は pull レスポンスです。
type pullResponse struct {
	ReceivedMessages []receivedMessage `json:"receivedMessages"`
}

// acknowledgeRequest は acknowledge リクエストのボディです。
type acknowledgeRequest struct {
	AckIDs []string `json:"ackIds"`
}

// createTopicRequest は topic 作成リクエストのボディです。
type createTopicRequest struct {
	Labels map[string]string `json:"labels,omitempty"`
}

// topicResponse は topic レスポンスです。
type topicResponse struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

// listTopicsResponse は topic 一覧レスポンスです。
type listTopicsResponse struct {
	Topics []topicResponse `json:"topics"`
}

// createSubscriptionRequest はサブスクリプション作成リクエストのボディです。
type createSubscriptionRequest struct {
	Topic              string            `json:"topic"`
	AckDeadlineSeconds int32             `json:"ackDeadlineSeconds"`
	Labels             map[string]string `json:"labels,omitempty"`
}

// subscriptionResponse はサブスクリプションレスポンスです。
type subscriptionResponse struct {
	Name               string            `json:"name"`
	Topic              string            `json:"topic"`
	AckDeadlineSeconds int32             `json:"ackDeadlineSeconds"`
	Labels             map[string]string `json:"labels,omitempty"`
}

// listSubscriptionsResponse はサブスクリプション一覧レスポンスです。
type listSubscriptionsResponse struct {
	Subscriptions []subscriptionResponse `json:"subscriptions"`
}
