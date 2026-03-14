package sqs

// createQueueRequest は CreateQueue のリクエストパラメータです。
type createQueueRequest struct {
	QueueName  string            `json:"QueueName"`
	Attributes map[string]string `json:"Attributes"`
	Tags       map[string]string `json:"tags"`
}

// createQueueResponse は CreateQueue のレスポンスです。
type createQueueResponse struct {
	QueueUrl string `json:"QueueUrl"`
}

// deleteQueueRequest は DeleteQueue のリクエストパラメータです。
type deleteQueueRequest struct {
	QueueUrl string `json:"QueueUrl"`
}

// getQueueUrlRequest は GetQueueUrl のリクエストパラメータです。
type getQueueUrlRequest struct {
	QueueName string `json:"QueueName"`
}

// getQueueUrlResponse は GetQueueUrl のレスポンスです。
type getQueueUrlResponse struct {
	QueueUrl string `json:"QueueUrl"`
}

// getQueueAttributesRequest は GetQueueAttributes のリクエストパラメータです。
type getQueueAttributesRequest struct {
	QueueUrl       string   `json:"QueueUrl"`
	AttributeNames []string `json:"AttributeNames"`
}

// getQueueAttributesResponse は GetQueueAttributes のレスポンスです。
type getQueueAttributesResponse struct {
	Attributes map[string]string `json:"Attributes"`
}

// listQueuesRequest は ListQueues のリクエストパラメータです。
type listQueuesRequest struct {
	QueueNamePrefix string `json:"QueueNamePrefix"`
}

// listQueuesResponse は ListQueues のレスポンスです。
type listQueuesResponse struct {
	QueueUrls []string `json:"QueueUrls"`
}

// tagQueueRequest は TagQueue のリクエストパラメータです。
type tagQueueRequest struct {
	QueueUrl string            `json:"QueueUrl"`
	Tags     map[string]string `json:"Tags"`
}

// sqsQueue は SQS キューの内部表現です。Store の Spec に保存するデータです。
type sqsQueue struct {
	QueueName        string
	QueueUrl         string
	CreatedTimestamp string
}
