package lambda

// CreateFunctionRequest は CreateFunction API のリクエストボディです。
type CreateFunctionRequest struct {
	FunctionName string          `json:"FunctionName"`
	Runtime      string          `json:"Runtime"`
	Role         string          `json:"Role"`
	Handler      string          `json:"Handler"`
	Code         FunctionCode    `json:"Code"`
	Description  string          `json:"Description"`
	Timeout      int             `json:"Timeout"`
	MemorySize   int             `json:"MemorySize"`
	Environment  *EnvConfig      `json:"Environment,omitempty"`
	Tags         map[string]string `json:"Tags,omitempty"`
}

// FunctionCode は関数コードの指定方法です。
type FunctionCode struct {
	// ZipFile は base64 エンコードされた zip ファイルです。
	ZipFile string `json:"ZipFile"`
}

// EnvConfig は Lambda 関数の環境変数設定です。
type EnvConfig struct {
	Variables map[string]string `json:"Variables"`
}

// UpdateFunctionCodeRequest は UpdateFunctionCode API のリクエストボディです。
type UpdateFunctionCodeRequest struct {
	FunctionName string `json:"FunctionName"`
	ZipFile      string `json:"ZipFile"`
}

// FunctionConfiguration は Lambda 関数の設定情報です (レスポンス共通型)。
type FunctionConfiguration struct {
	FunctionName  string            `json:"FunctionName"`
	FunctionArn   string            `json:"FunctionArn"`
	Runtime       string            `json:"Runtime"`
	Role          string            `json:"Role"`
	Handler       string            `json:"Handler"`
	CodeSize      int64             `json:"CodeSize"`
	Description   string            `json:"Description"`
	Timeout       int               `json:"Timeout"`
	MemorySize    int               `json:"MemorySize"`
	LastModified  string            `json:"LastModified"`
	CodeSha256    string            `json:"CodeSha256"`
	State         string            `json:"State"`
	Environment   *EnvConfig        `json:"Environment,omitempty"`
}

// GetFunctionResponse は GetFunction API のレスポンスです。
type GetFunctionResponse struct {
	Configuration FunctionConfiguration `json:"Configuration"`
}

// ListFunctionsResponse は ListFunctions API のレスポンスです。
type ListFunctionsResponse struct {
	Functions []FunctionConfiguration `json:"Functions"`
}

// lambdaError は Lambda エラーレスポンスの構造体です。
type lambdaError struct {
	Type    string `json:"__type"`
	Message string `json:"Message"`
}
