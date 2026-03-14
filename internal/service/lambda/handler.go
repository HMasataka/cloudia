package lambda

import (
	"net/http"
	"regexp"
	"strings"
)

// validFunctionName は Lambda 関数名の許可パターンです。
var validFunctionName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// isValidFunctionName は関数名がパスインジェクションや不正文字を含まないか検証します。
func isValidFunctionName(name string) bool {
	return validFunctionName.MatchString(name)
}

// ServeHTTP はすべての Lambda リクエストを受け取り、URL パスと HTTP メソッドでルーティングします。
// Lambda は REST API のため X-Amz-Target ヘッダーを使用しません。
//
// ルーティング:
//   POST   /2015-03-31/functions                           -> CreateFunction
//   GET    /2015-03-31/functions                           -> ListFunctions
//   GET    /2015-03-31/functions/{name}                    -> GetFunction
//   DELETE /2015-03-31/functions/{name}                    -> DeleteFunction
//   PUT    /2015-03-31/functions/{name}/code               -> UpdateFunctionCode
//   POST   /2015-03-31/functions/{name}/invocations        -> Invoke (Group 3 スタブ)
func (s *LambdaService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	method := r.Method

	const prefix = "/2015-03-31/functions"

	if !strings.HasPrefix(path, prefix) {
		writeError(w, http.StatusNotFound, "ResourceNotFoundException", "path not found")
		return
	}

	// /2015-03-31/functions の後ろの部分
	rest := strings.TrimPrefix(path, prefix)
	// rest は "" / "/" / "/{name}" / "/{name}/code" / "/{name}/invocations"

	// トリムして先頭のスラッシュを除去
	rest = strings.TrimPrefix(rest, "/")

	if rest == "" {
		// /2015-03-31/functions
		switch method {
		case http.MethodPost:
			s.handleCreateFunction(w, r)
		case http.MethodGet:
			s.handleListFunctions(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
		}
		return
	}

	// rest は "{name}", "{name}/code", "{name}/invocations" のいずれか
	parts := strings.SplitN(rest, "/", 2)
	functionName := parts[0]
	subPath := ""
	if len(parts) == 2 {
		subPath = parts[1]
	}

	if !isValidFunctionName(functionName) {
		writeError(w, http.StatusBadRequest, "InvalidParameterValueException",
			"FunctionName contains invalid characters or exceeds length limit")
		return
	}

	switch subPath {
	case "":
		// /2015-03-31/functions/{name}
		switch method {
		case http.MethodGet:
			s.handleGetFunction(w, r, functionName)
		case http.MethodDelete:
			s.handleDeleteFunction(w, r, functionName)
		default:
			writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
		}

	case "code":
		// /2015-03-31/functions/{name}/code
		if method == http.MethodPut {
			s.handleUpdateFunctionCode(w, r, functionName)
		} else {
			writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
		}

	case "invocations":
		// /2015-03-31/functions/{name}/invocations
		if method == http.MethodPost {
			s.handleInvoke(w, r, functionName)
		} else {
			writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
		}

	default:
		writeError(w, http.StatusNotFound, "ResourceNotFoundException", "path not found")
	}
}

