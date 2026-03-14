package dynamodb

import (
	"net/http"
)

const dynamodbErrorBody = `{"__type":"ServiceUnavailableException","message":"backend storage service is temporarily unavailable"}`

// ServeHTTP proxies the HTTP request to the DynamoDB Local backend.
func (s *DynamoDBService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.proxy.ServeHTTP(w, r)
}
