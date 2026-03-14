package dynamodb

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

const dynamodbErrorBody = `{"__type":"ServiceUnavailableException","message":"backend storage service is temporarily unavailable"}`

// ServeHTTP proxies the HTTP request to the DynamoDB Local backend.
func (s *DynamoDBService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(s.backend.baseURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"__type":"InternalServerError","message":"invalid dynamodb endpoint"}`)) //nolint:errcheck
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, _ error) {
		rw.Header().Set("Content-Type", "application/x-amz-json-1.0")
		rw.WriteHeader(http.StatusBadGateway)
		rw.Write([]byte(dynamodbErrorBody)) //nolint:errcheck
	}

	proxy.ServeHTTP(w, r)
}
