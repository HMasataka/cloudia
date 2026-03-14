package ec2

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"time"

	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// createKeyPair は CreateKeyPair アクションを処理します。
// 2048-bit RSA 鍵を生成し、秘密鍵 PEM を一度だけ KeyMaterial として返します。
// Store には公開鍵のみ保存します。
func (e *EC2Service) createKeyPair(ctx context.Context, req service.Request) (service.Response, error) {
	keyName := req.Params["KeyName"]
	if keyName == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter KeyName.")
	}

	// 重複チェック
	existing, err := e.store.List(ctx, kindKeyPair, state.Filter{})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	for _, r := range existing {
		if name, _ := r.Spec["KeyName"].(string); name == keyName {
			return errorResponse(http.StatusBadRequest, "InvalidKeyPair.Duplicate",
				fmt.Sprintf("The keypair '%s' already exists.", keyName))
		}
	}

	// RSA 2048-bit 鍵生成
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError},
			fmt.Errorf("createKeyPair: generate rsa key: %w", err)
	}

	// 秘密鍵を PEM エンコード
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// 公開鍵を DER エンコードしてフィンガープリント計算
	pubDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError},
			fmt.Errorf("createKeyPair: marshal public key: %w", err)
	}
	fingerprint := md5Fingerprint(pubDER)

	// KeyPairId を生成
	hex17, err := generateHex17()
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	keyPairID := "key-" + hex17

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:      kindKeyPair,
		ID:        keyPairID,
		Provider:  "aws",
		Service:   "ec2",
		Region:    e.cfg.Region,
		Status:    "available",
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"KeyName":           keyName,
			"KeyFingerprint":    fingerprint,
			"PublicKeyMaterial": base64.StdEncoding.EncodeToString(pubDER),
		},
	}

	if err := e.store.Put(ctx, resource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	resp := CreateKeyPairResponse{
		RequestId:      "cloudia-ec2",
		KeyName:        keyName,
		KeyFingerprint: fingerprint,
		KeyMaterial:    string(privPEM),
		KeyPairId:      keyPairID,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// importKeyPair は ImportKeyPair アクションを処理します。
// Base64 エンコードされた公開鍵を受け取り、フィンガープリントを計算して保存します。
func (e *EC2Service) importKeyPair(ctx context.Context, req service.Request) (service.Response, error) {
	keyName := req.Params["KeyName"]
	if keyName == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter KeyName.")
	}

	publicKeyMaterial := req.Params["PublicKeyMaterial"]
	if publicKeyMaterial == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter PublicKeyMaterial.")
	}

	// 重複チェック
	existing, err := e.store.List(ctx, kindKeyPair, state.Filter{})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	for _, r := range existing {
		if name, _ := r.Spec["KeyName"].(string); name == keyName {
			return errorResponse(http.StatusBadRequest, "InvalidKeyPair.Duplicate",
				fmt.Sprintf("The keypair '%s' already exists.", keyName))
		}
	}

	// Base64 デコード
	pubDER, err := base64.StdEncoding.DecodeString(publicKeyMaterial)
	if err != nil {
		return errorResponse(http.StatusBadRequest, "InvalidParameterValue",
			"The public key material is not valid Base64.")
	}

	fingerprint := md5Fingerprint(pubDER)

	// KeyPairId を生成
	hex17, err := generateHex17()
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}
	keyPairID := "key-" + hex17

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:      kindKeyPair,
		ID:        keyPairID,
		Provider:  "aws",
		Service:   "ec2",
		Region:    e.cfg.Region,
		Status:    "available",
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"KeyName":           keyName,
			"KeyFingerprint":    fingerprint,
			"PublicKeyMaterial": publicKeyMaterial,
		},
	}

	if err := e.store.Put(ctx, resource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	resp := ImportKeyPairResponse{
		RequestId:      "cloudia-ec2",
		KeyName:        keyName,
		KeyFingerprint: fingerprint,
		KeyPairId:      keyPairID,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// deleteKeyPair は DeleteKeyPair アクションを処理します。
// KeyName で検索して削除します。存在しない場合も成功 (AWS 互換冪等性)。
func (e *EC2Service) deleteKeyPair(ctx context.Context, req service.Request) (service.Response, error) {
	keyName := req.Params["KeyName"]
	if keyName == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter KeyName.")
	}

	resources, err := e.store.List(ctx, kindKeyPair, state.Filter{})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	for _, r := range resources {
		if name, _ := r.Spec["KeyName"].(string); name == keyName {
			if delErr := e.store.Delete(ctx, kindKeyPair, r.ID); delErr != nil {
				if !errors.Is(delErr, models.ErrNotFound) {
					return service.Response{StatusCode: http.StatusInternalServerError}, delErr
				}
			}
			break
		}
	}

	resp := DeleteKeyPairResponse{
		RequestId: "cloudia-ec2",
		Return:    true,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// describeKeyPairs は DescribeKeyPairs アクションを処理します。
// Filter.N.Name=key-name フィルタに対応します。
func (e *EC2Service) describeKeyPairs(ctx context.Context, req service.Request) (service.Response, error) {
	filters := awsprot.ParseFilters(req.Params)

	var keyNameFilter []string
	for _, f := range filters {
		if f.Name == "key-name" {
			keyNameFilter = append(keyNameFilter, f.Values...)
		}
	}

	resources, err := e.store.List(ctx, kindKeyPair, state.Filter{})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	keyNameSet := make(map[string]struct{}, len(keyNameFilter))
	for _, n := range keyNameFilter {
		keyNameSet[n] = struct{}{}
	}

	var items []KeyPairItem
	for _, r := range resources {
		name, _ := r.Spec["KeyName"].(string)
		if len(keyNameSet) > 0 {
			if _, ok := keyNameSet[name]; !ok {
				continue
			}
		}
		fingerprint, _ := r.Spec["KeyFingerprint"].(string)
		items = append(items, KeyPairItem{
			KeyName:        name,
			KeyFingerprint: fingerprint,
			KeyPairId:      r.ID,
		})
	}

	resp := DescribeKeyPairsResponse{
		RequestId: "cloudia-ec2",
		KeySet:    items,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// md5Fingerprint は DER エンコードされた公開鍵の MD5 フィンガープリントを計算します。
// 出力形式は "xx:xx:xx:..." (コロン区切り hex) です。
func md5Fingerprint(der []byte) string {
	sum := md5.Sum(der)
	result := make([]byte, 0, len(sum)*3-1)
	for i, b := range sum {
		if i > 0 {
			result = append(result, ':')
		}
		result = append(result, fmt.Sprintf("%02x", b)...)
	}
	return string(result)
}
