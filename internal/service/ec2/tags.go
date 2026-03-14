package ec2

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"time"

	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// parseResourceIDs は ResourceId.N パラメータからリソース ID リストを取得します。
// キーパターンは ResourceId.1, ResourceId.2, ... です。
func parseResourceIDs(params map[string]string) []string {
	var ids []string
	for i := 1; ; i++ {
		key := fmt.Sprintf("ResourceId.%d", i)
		id, ok := params[key]
		if !ok || id == "" {
			break
		}
		ids = append(ids, id)
	}
	return ids
}

// parseTagKeys は Tag.N.Key パラメータからキーリストを取得します。
// キーパターンは Tag.1.Key, Tag.2.Key, ... です。
func parseTagKeys(params map[string]string) []string {
	var keys []string
	for i := 1; ; i++ {
		key := fmt.Sprintf("Tag.%d.Key", i)
		v, ok := params[key]
		if !ok || v == "" {
			break
		}
		keys = append(keys, v)
	}
	return keys
}

// parseTagPairs は Tag.N.Key / Tag.N.Value パラメータからタグマップを取得します。
func parseTagPairs(params map[string]string) map[string]string {
	tags := make(map[string]string)
	for i := 1; ; i++ {
		keyParam := fmt.Sprintf("Tag.%d.Key", i)
		k, ok := params[keyParam]
		if !ok || k == "" {
			break
		}
		valParam := fmt.Sprintf("Tag.%d.Value", i)
		v := params[valParam]
		tags[k] = v
	}
	return tags
}

// createTags は CreateTags アクションを処理します。
func (e *EC2Service) createTags(ctx context.Context, req service.Request) (service.Response, error) {
	resourceIDs := parseResourceIDs(req.Params)
	if len(resourceIDs) == 0 {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter ResourceId.")
	}

	tags := parseTagPairs(req.Params)

	for _, id := range resourceIDs {
		r, err := e.store.Get(ctx, kindInstance, id)
		if err != nil {
			if errors.Is(err, models.ErrNotFound) {
				return errorResponse(http.StatusBadRequest, "InvalidID",
					fmt.Sprintf("The ID '%s' is not valid.", id))
			}
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}

		if r.Tags == nil {
			r.Tags = make(map[string]string)
		}
		for k, v := range tags {
			r.Tags[k] = v
		}
		r.UpdatedAt = time.Now().UTC()

		if err := e.store.Put(ctx, r); err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
	}

	resp := CreateTagsResponse{
		RequestId: "cloudia-ec2",
		Return:    true,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// deleteTags は DeleteTags アクションを処理します。
func (e *EC2Service) deleteTags(ctx context.Context, req service.Request) (service.Response, error) {
	resourceIDs := parseResourceIDs(req.Params)
	if len(resourceIDs) == 0 {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter ResourceId.")
	}

	tagKeys := parseTagKeys(req.Params)

	for _, id := range resourceIDs {
		r, err := e.store.Get(ctx, kindInstance, id)
		if err != nil {
			if errors.Is(err, models.ErrNotFound) {
				return errorResponse(http.StatusBadRequest, "InvalidID",
					fmt.Sprintf("The ID '%s' is not valid.", id))
			}
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}

		if r.Tags != nil {
			for _, k := range tagKeys {
				delete(r.Tags, k)
			}
		}
		r.UpdatedAt = time.Now().UTC()

		if err := e.store.Put(ctx, r); err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
	}

	resp := DeleteTagsResponse{
		RequestId: "cloudia-ec2",
		Return:    true,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// DescribeTagItem は describeTags レスポンス用の拡張タグ要素です。
type DescribeTagItem struct {
	ResourceID   string `xml:"resourceId"`
	ResourceType string `xml:"resourceType"`
	Key          string `xml:"key"`
	Value        string `xml:"value"`
}

// DescribeTagsFullResponse は describeTags の XML レスポンスです。
type DescribeTagsFullResponse struct {
	XMLName   xml.Name          `xml:"DescribeTagsResponse"`
	RequestId string            `xml:"requestId"`
	TagSet    []DescribeTagItem `xml:"tagSet>item"`
}

// describeTags は DescribeTags アクションを処理します。
func (e *EC2Service) describeTags(ctx context.Context, req service.Request) (service.Response, error) {
	filters := awsprot.ParseFilters(req.Params)

	// resource-id / key フィルタ値を収集
	var resourceIDFilter []string
	var keyFilter []string
	for _, f := range filters {
		switch f.Name {
		case "resource-id":
			resourceIDFilter = append(resourceIDFilter, f.Values...)
		case "key":
			keyFilter = append(keyFilter, f.Values...)
		}
	}

	resources, err := e.store.List(ctx, kindInstance, state.Filter{})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	// resource-id フィルタ用セット
	resourceIDSet := make(map[string]struct{}, len(resourceIDFilter))
	for _, id := range resourceIDFilter {
		resourceIDSet[id] = struct{}{}
	}

	// key フィルタ用セット
	keySet := make(map[string]struct{}, len(keyFilter))
	for _, k := range keyFilter {
		keySet[k] = struct{}{}
	}

	var tagItems []DescribeTagItem
	for _, r := range resources {
		// resource-id フィルタ
		if len(resourceIDSet) > 0 {
			if _, ok := resourceIDSet[r.ID]; !ok {
				continue
			}
		}

		for k, v := range r.Tags {
			// key フィルタ
			if len(keySet) > 0 {
				if _, ok := keySet[k]; !ok {
					continue
				}
			}
			tagItems = append(tagItems, DescribeTagItem{
				ResourceID:   r.ID,
				ResourceType: "instance",
				Key:          k,
				Value:        v,
			})
		}
	}

	resp := DescribeTagsFullResponse{
		RequestId: "cloudia-ec2",
		TagSet:    tagItems,
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}
