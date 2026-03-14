package aws

import (
	"bytes"
	"encoding/xml"
	"net/http"
)

// EncodeXMLResponse は任意の構造体を XML にマーシャルして HTTP レスポンスとして出力します。
// namespace が空でない場合は XML のルート要素に xmlns 属性として付与されます。
func EncodeXMLResponse(w http.ResponseWriter, statusCode int, body any, namespace string) {
	data, err := marshalWithNamespace(body, namespace)
	if err != nil {
		http.Error(w, "xml encoding error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(data)
}

// marshalWithNamespace は body を XML にマーシャルします。
// namespace が非空の場合はルート要素の開きタグ直後に xmlns 属性を挿入します。
func marshalWithNamespace(body any, namespace string) ([]byte, error) {
	raw, err := xml.Marshal(body)
	if err != nil {
		return nil, err
	}

	if namespace == "" {
		return raw, nil
	}

	// ルート要素の "<TagName" を "<TagName xmlns="namespace"" に置換する（最初の出現のみ）
	idx := bytes.IndexByte(raw, '<')
	if idx < 0 {
		return raw, nil
	}
	// 最初の '<' の後ろのタグ名末尾（次のスペース・'>' ・'/' まで）を探す
	rest := raw[idx+1:]
	tagEnd := bytes.IndexAny(rest, " >/")
	if tagEnd < 0 {
		return raw, nil
	}

	insertion := []byte(` xmlns="` + namespace + `"`)
	result := make([]byte, 0, len(raw)+len(insertion))
	result = append(result, raw[:idx+1+tagEnd]...)
	result = append(result, insertion...)
	result = append(result, raw[idx+1+tagEnd:]...)
	return result, nil
}
