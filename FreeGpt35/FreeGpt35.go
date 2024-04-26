package FreeGpt35

import (
	"encoding/json"
	"fmt"
	ProofWork2 "free-gpt3.5-2api/ProofWork"
	"free-gpt3.5-2api/ProxyPool"
	"free-gpt3.5-2api/RequestClient"
	"free-gpt3.5-2api/common"
	"free-gpt3.5-2api/config"
	"github.com/aurorax-neo/go-logger"
	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/google/uuid"
	"io"
)

var (
	Ua      = ProxyPool.Ua
	ChatUrl = config.BaseUrl + "/backend-anon/conversation"
	AuthUrl = config.BaseUrl + "/backend-anon/sentinel/chat-requirements"
)

type Gpt35 struct {
	RequestClient RequestClient.RequestClient
	Proxy         *ProxyPool.Proxy
	MaxUseCount   int
	ExpiresIn     int64
	FreeAuth      *freeAuth
	Ua            string
	Cookies       []*fhttp.Cookie
}

type freeAuth struct {
	OaiDeviceId string               `json:"-"`
	Persona     string               `json:"persona"`
	Arkose      arkose               `json:"arkose"`
	Turnstile   turnstile            `json:"turnstile"`
	ProofWork   ProofWork2.ProofWork `json:"proofofwork"`
	Token       string               `json:"token"`
}

type arkose struct {
	Required bool   `json:"required"`
	Dx       string `json:"dx"`
}

type turnstile struct {
	Required bool `json:"required"`
}

// NewGpt35 创建 Gpt35 实例 0 无论网络是否被标记限制都获取 1 在网络未标记时才能获取
func NewGpt35(newType int) *Gpt35 {
	// 创建 FreeGpt35 实例
	gpt35 := &Gpt35{
		MaxUseCount: -1,
		ExpiresIn:   -1,
		FreeAuth:    &freeAuth{},
		Ua:          Ua,
	}
	// 获取请求客户端
	err := gpt35.getNewRequestClient(newType)
	if err != nil {
		return nil
	}
	// 获取新session
	err = gpt35.getNewFreeAuth()
	if err != nil {
		return nil
	}
	return gpt35
}

func (G *Gpt35) NewRequest(method, url string, body io.Reader) (*fhttp.Request, error) {
	request, err := RequestClient.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("accept", "*/*")
	request.Header.Set("accept-language", "zh-CN,zh;q=0.9,zh-Hans;q=0.8,en;q=0.7")
	for _, cookie := range G.Cookies {
		request.AddCookie(cookie)
	}
	request.Header.Set("oai-language", "en-US")
	request.Header.Set("origin", common.GetOrigin(url))
	request.Header.Set("referer", common.GetOrigin(url))
	request.Header.Set("sec-ch-ua", `"Microsoft Edge";v="123", "Not:A-Brand";v="8", "Chromium";v="123"`)
	request.Header.Set("sec-ch-ua-mobile", "?0")
	request.Header.Set("sec-ch-ua-platform", `"Windows"`)
	request.Header.Set("sec-fetch-dest", "empty")
	request.Header.Set("sec-fetch-mode", "cors")
	request.Header.Set("sec-fetch-site", "same-origin")
	request.Header.Set("user-agent", G.Ua)
	return request, nil
}

func (G *Gpt35) getNewRequestClient(newType int) error {
	// 获取代理池
	ProxyPoolInstance := ProxyPool.GetProxyPoolInstance()
	// 获取代理
	G.Proxy = ProxyPoolInstance.GetProxy()
	// 判断代理是否可用
	if G.Proxy.CanUseAt > common.GetTimestampSecond(0) && newType == 1 {
		errStr := fmt.Sprint(G.Proxy.Link, ": Proxy restricted, Reuse at ", G.Proxy.CanUseAt)
		return fmt.Errorf(errStr)
	}
	// Ua
	G.Ua = G.Proxy.Ua
	// Cookies
	G.Cookies = G.Proxy.Cookies
	// 请求客户端
	G.RequestClient = RequestClient.GetInstance()
	if G.RequestClient == nil {
		errStr := fmt.Sprint("RequestClient is nil")
		logger.Logger.Debug(errStr)
		return fmt.Errorf(errStr)
	}
	// 设置代理
	err := G.RequestClient.SetProxy(G.Proxy.Link.String())
	if err != nil {
		errStr := fmt.Sprint("SetProxy Error: ", err)
		logger.Logger.Debug(errStr)
	}
	// 成功后更新代理的可用时间
	G.Proxy.CanUseAt = common.GetTimestampSecond(0)
	return nil
}

func (G *Gpt35) getNewFreeAuth() error {
	// 生成新的设备 ID
	G.FreeAuth.OaiDeviceId = uuid.New().String()
	// 创建请求
	request, err := G.NewRequest("POST", AuthUrl, nil)
	if err != nil {
		return err
	}
	// 设置请求头
	request.Header.Set("oai-device-id", G.FreeAuth.OaiDeviceId)
	// 发送 POST 请求
	response, err := G.RequestClient.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		if response.StatusCode == 429 {
			G.Proxy.CanUseAt = common.GetTimestampSecond(600)
			logger.Logger.Debug(fmt.Sprint("getNewFreeAuth: Proxy restricted, Reuse at ", G.Proxy.CanUseAt))
		}
		logger.Logger.Debug(fmt.Sprint("getNewFreeAuth: StatusCode: ", response.StatusCode))
		return fmt.Errorf("StatusCode: %d", response.StatusCode)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(response.Body)
	if err := json.NewDecoder(response.Body).Decode(&G.FreeAuth); err != nil {
		return err
	}
	if G.FreeAuth.ProofWork.Required {
		G.FreeAuth.ProofWork.Ospt = ProofWork2.CalcProofToken(G.FreeAuth.ProofWork.Seed, G.FreeAuth.ProofWork.Difficulty, request.Header.Get("User-Agent"))
	}
	// 设置 MaxUseCount
	G.MaxUseCount = 1
	// 设置 ExpiresIn
	G.ExpiresIn = common.GetTimestampSecond(config.AuthED)
	return nil
}
