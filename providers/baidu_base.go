package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"one-api/common"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var baiduTokenStore sync.Map

type BaiduProvider struct {
	ProviderConfig
}

type BaiduAccessToken struct {
	AccessToken      string    `json:"access_token"`
	Error            string    `json:"error,omitempty"`
	ErrorDescription string    `json:"error_description,omitempty"`
	ExpiresIn        int64     `json:"expires_in,omitempty"`
	ExpiresAt        time.Time `json:"-"`
}

func CreateBaiduProvider(c *gin.Context) *BaiduProvider {
	return &BaiduProvider{
		ProviderConfig: ProviderConfig{
			BaseURL:         "https://aip.baidubce.com",
			ChatCompletions: "/rpc/2.0/ai_custom/v1/wenxinworkshop/chat",
			Embeddings:      "/rpc/2.0/ai_custom/v1/wenxinworkshop/embeddings",
			Context:         c,
		},
	}
}

// 获取完整请求 URL
func (p *BaiduProvider) GetFullRequestURL(requestURL string, modelName string) string {
	var modelNameMap = map[string]string{
		"ERNIE-Bot":       "completions",
		"ERNIE-Bot-turbo": "eb-instant",
		"ERNIE-Bot-4":     "completions_pro",
		"BLOOMZ-7B":       "bloomz_7b1",
		"Embedding-V1":    "embedding-v1",
	}

	baseURL := strings.TrimSuffix(p.GetBaseURL(), "/")
	apiKey, err := p.getBaiduAccessToken()
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%s%s/%s?access_token=%s", baseURL, requestURL, modelNameMap[modelName], apiKey)
}

// 获取请求头
func (p *BaiduProvider) GetRequestHeaders() (headers map[string]string) {
	headers = make(map[string]string)

	headers["Content-Type"] = p.Context.Request.Header.Get("Content-Type")
	headers["Accept"] = p.Context.Request.Header.Get("Accept")
	if headers["Content-Type"] == "" {
		headers["Content-Type"] = "application/json"
	}

	return headers
}

func (p *BaiduProvider) getBaiduAccessToken() (string, error) {
	apiKey := p.Context.GetString("api_key")
	if val, ok := baiduTokenStore.Load(apiKey); ok {
		var accessToken BaiduAccessToken
		if accessToken, ok = val.(BaiduAccessToken); ok {
			// soon this will expire
			if time.Now().Add(time.Hour).After(accessToken.ExpiresAt) {
				go func() {
					_, _ = p.getBaiduAccessTokenHelper(apiKey)
				}()
			}
			return accessToken.AccessToken, nil
		}
	}
	accessToken, err := p.getBaiduAccessTokenHelper(apiKey)
	if err != nil {
		return "", err
	}
	if accessToken == nil {
		return "", errors.New("getBaiduAccessToken return a nil token")
	}
	return (*accessToken).AccessToken, nil
}

func (p *BaiduProvider) getBaiduAccessTokenHelper(apiKey string) (*BaiduAccessToken, error) {
	parts := strings.Split(apiKey, "|")
	if len(parts) != 2 {
		return nil, errors.New("invalid baidu apikey")
	}

	client := common.NewClient()
	url := fmt.Sprintf(p.BaseURL+"/oauth/2.0/token?grant_type=client_credentials&client_id=%s&client_secret=%s", parts[0], parts[1])

	var headers = map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	req, err := client.NewRequest("POST", url, common.WithHeader(headers))
	if err != nil {
		return nil, err
	}

	resp, err := common.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var accessToken BaiduAccessToken
	err = json.NewDecoder(resp.Body).Decode(&accessToken)
	if err != nil {
		return nil, err
	}
	if accessToken.Error != "" {
		return nil, errors.New(accessToken.Error + ": " + accessToken.ErrorDescription)
	}
	if accessToken.AccessToken == "" {
		return nil, errors.New("getBaiduAccessTokenHelper get empty access token")
	}
	accessToken.ExpiresAt = time.Now().Add(time.Duration(accessToken.ExpiresIn) * time.Second)
	baiduTokenStore.Store(apiKey, accessToken)
	return &accessToken, nil
}
