package binance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gorilla/websocket"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"plat_order/internal/model/entity"
	"plat_order/internal/service"
	"strconv"
	"strings"
	"time"
)

type (
	sBinance struct{}
)

func init() {
	service.RegisterBinance(New())
}

func New() *sBinance {
	return &sBinance{}
}

const (
	apiBaseURL   = "https://fapi.binance.com"
	wsBaseURL    = "wss://fstream.binance.com/ws/"
	listenKeyURL = "/fapi/v1/listenKey"
)

// 获取币安服务器时间
func getBinanceServerTime() int64 {
	urlTmp := "https://api.binance.com/api/v3/time"
	resp, err := http.Get(urlTmp)
	if err != nil {
		fmt.Println("Error getting server time:", err)
		return 0
	}

	defer func(Body io.ReadCloser) {
		err = Body.Close()
		if err != nil {

		}
	}(resp.Body)

	var serverTimeResponse struct {
		ServerTime int64 `json:"serverTime"`
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return 0
	}
	if err := json.Unmarshal(body, &serverTimeResponse); err != nil {
		fmt.Println("Error unmarshaling server time:", err)
		return 0
	}

	return serverTimeResponse.ServerTime
}

// 生成签名
func generateSignature(apiS string, params url.Values) string {
	// 将请求参数编码成 URL 格式的字符串
	queryString := params.Encode()

	// 生成签名
	mac := hmac.New(sha256.New, []byte(apiS))
	mac.Write([]byte(queryString)) // 用 API Secret 生成签名
	return hex.EncodeToString(mac.Sum(nil))
}

// GetBinanceInfo 获取账户信息
func (s *sBinance) GetBinanceInfo(apiK, apiS string) string {
	// 请求的API地址
	endpoint := "/fapi/v2/account"
	baseURL := "https://fapi.binance.com"

	// 获取当前时间戳（使用服务器时间避免时差问题）
	serverTime := getBinanceServerTime()
	if serverTime == 0 {
		return ""
	}
	timestamp := strconv.FormatInt(serverTime, 10)

	// 设置请求参数
	params := url.Values{}
	params.Set("timestamp", timestamp)
	params.Set("recvWindow", "5000") // 设置接收窗口

	// 生成签名
	signature := generateSignature(apiS, params)

	// 将签名添加到请求参数中
	params.Set("signature", signature)

	// 构建完整的请求URL
	requestURL := baseURL + endpoint + "?" + params.Encode()

	// 创建请求
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return ""
	}

	// 添加请求头
	req.Header.Add("X-MBX-APIKEY", apiK)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return ""
	}

	defer func(Body io.ReadCloser) {
		err = Body.Close()
		if err != nil {

		}
	}(resp.Body)

	// 读取响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return ""
	}

	// 解析响应
	var o *entity.Asset
	err = json.Unmarshal(body, &o)
	if err != nil {
		fmt.Println("Error unmarshalling response:", err)
		return ""
	}

	// 返回资产余额
	return o.TotalMarginBalance
}

func (s *sBinance) RequestBinanceOrder(symbol string, side string, orderType string, positionSide string, quantity string, apiKey string, secretKey string) (*entity.BinanceOrder, *entity.BinanceOrderInfo, error) {
	var (
		client       *http.Client
		req          *http.Request
		resp         *http.Response
		res          *entity.BinanceOrder
		resOrderInfo *entity.BinanceOrderInfo
		data         string
		b            []byte
		err          error
		apiUrl       = "https://fapi.binance.com/fapi/v1/order"
	)

	//fmt.Println(symbol, side, orderType, positionSide, quantity, apiKey, secretKey)
	// 时间
	now := strconv.FormatInt(time.Now().UTC().UnixMilli(), 10)
	// 拼请求数据
	data = "symbol=" + symbol + "&side=" + side + "&type=" + orderType + "&positionSide=" + positionSide + "&newOrderRespType=" + "RESULT" + "&quantity=" + quantity + "&timestamp=" + now

	// 加密
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(data))
	signature := hex.EncodeToString(h.Sum(nil))
	// 构造请求

	req, err = http.NewRequest("POST", apiUrl, strings.NewReader(data+"&signature="+signature))
	if err != nil {
		return nil, nil, err
	}
	// 添加头信息
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-MBX-APIKEY", apiKey)

	// 请求执行
	client = &http.Client{Timeout: 3 * time.Second}
	resp, err = client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	// 结果
	defer func(Body io.ReadCloser) {
		err = Body.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(resp.Body)

	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(string(b), err)
		return nil, nil, err
	}

	var o *entity.BinanceOrder
	err = json.Unmarshal(b, &o)
	if err != nil {
		fmt.Println(string(b), err)
		return nil, nil, err
	}

	res = &entity.BinanceOrder{
		OrderId:       o.OrderId,
		ExecutedQty:   o.ExecutedQty,
		ClientOrderId: o.ClientOrderId,
		Symbol:        o.Symbol,
		AvgPrice:      o.AvgPrice,
		CumQuote:      o.CumQuote,
		Side:          o.Side,
		PositionSide:  o.PositionSide,
		ClosePosition: o.ClosePosition,
		Type:          o.Type,
	}

	if 0 >= res.OrderId {
		//fmt.Println(string(b))
		err = json.Unmarshal(b, &resOrderInfo)
		if err != nil {
			fmt.Println(string(b), err)
			return nil, nil, err
		}
	}

	return res, resOrderInfo, nil
}

// GetBinancePositionInfo 获取账户信息
func (s *sBinance) GetBinancePositionInfo(apiK, apiS string) []*entity.BinancePosition {
	// 请求的API地址
	endpoint := "/fapi/v2/account"
	baseURL := "https://fapi.binance.com"

	// 获取当前时间戳（使用服务器时间避免时差问题）
	serverTime := getBinanceServerTime()
	if serverTime == 0 {
		return nil
	}
	timestamp := strconv.FormatInt(serverTime, 10)

	// 设置请求参数
	params := url.Values{}
	params.Set("timestamp", timestamp)
	params.Set("recvWindow", "5000") // 设置接收窗口

	// 生成签名
	signature := generateSignature(apiS, params)

	// 将签名添加到请求参数中
	params.Set("signature", signature)

	// 构建完整的请求URL
	requestURL := baseURL + endpoint + "?" + params.Encode()

	// 创建请求
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil
	}

	// 添加请求头
	req.Header.Add("X-MBX-APIKEY", apiK)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return nil
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return nil
	}

	// 解析响应
	var o *entity.BinanceResponse
	err = json.Unmarshal(body, &o)
	if err != nil {
		fmt.Println("Error unmarshalling response:", err)
		return nil
	}

	// 返回资产余额
	return o.Positions
}

var (
	ListenKey = gvar.New("", true)
	Conn      *websocket.Conn // WebSocket connection
)

// ListenKeyResponse represents the response from Binance API when creating or renewing a ListenKey
type ListenKeyResponse struct {
	ListenKey string `json:"listenKey"`
}

// CreateListenKey creates a new ListenKey for user data stream
func (s *sBinance) CreateListenKey(apiKey string) error {
	req, err := http.NewRequest("POST", apiBaseURL+listenKeyURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-MBX-APIKEY", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s", string(body))
	}

	var response *ListenKeyResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return err
	}

	ListenKey.Set(response.ListenKey)
	return nil
}

// RenewListenKey renews the ListenKey for user data stream
func (s *sBinance) RenewListenKey(apiKey string) error {
	req, err := http.NewRequest("PUT", apiBaseURL+listenKeyURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-MBX-APIKEY", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s", string(body))
	}

	return nil
}

// ConnectWebSocket safely connects to the WebSocket and updates conn
func (s *sBinance) ConnectWebSocket() error {
	// Close the existing connection if open
	if Conn != nil {
		err := Conn.Close()
		if err != nil {
			fmt.Println("Failed to close old connection:", err)
		}
	}

	// Create a new WebSocket connection
	wsURL := wsBaseURL + ListenKey.String()
	var err error
	Conn, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %v", err)
	}
	fmt.Println("WebSocket connection established.")
	return nil
}
