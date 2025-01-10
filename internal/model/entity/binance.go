package entity

type BinanceOrder struct {
	OrderId       int64
	ExecutedQty   string
	ClientOrderId string
	Symbol        string
	AvgPrice      string
	CumQuote      string
	Side          string
	PositionSide  string
	ClosePosition bool
	Type          string
	Status        string
}

type BinanceOrderInfo struct {
	Code int64
	Msg  string
}

// Asset 代表单个资产的保证金信息
type Asset struct {
	TotalMarginBalance string `json:"totalMarginBalance"` // 资产余额
}

// BinancePosition 代表单个头寸（持仓）信息
type BinancePosition struct {
	Symbol                 string `json:"symbol"`                 // 交易对
	InitialMargin          string `json:"initialMargin"`          // 当前所需起始保证金(基于最新标记价格)
	MaintMargin            string `json:"maintMargin"`            // 维持保证金
	UnrealizedProfit       string `json:"unrealizedProfit"`       // 持仓未实现盈亏
	PositionInitialMargin  string `json:"positionInitialMargin"`  // 持仓所需起始保证金(基于最新标记价格)
	OpenOrderInitialMargin string `json:"openOrderInitialMargin"` // 当前挂单所需起始保证金(基于最新标记价格)
	Leverage               string `json:"leverage"`               // 杠杆倍率
	Isolated               bool   `json:"isolated"`               // 是否是逐仓模式
	EntryPrice             string `json:"entryPrice"`             // 持仓成本价
	MaxNotional            string `json:"maxNotional"`            // 当前杠杆下用户可用的最大名义价值
	BidNotional            string `json:"bidNotional"`            // 买单净值，忽略
	AskNotional            string `json:"askNotional"`            // 卖单净值，忽略
	PositionSide           string `json:"positionSide"`           // 持仓方向 (BOTH, LONG, SHORT)
	PositionAmt            string `json:"positionAmt"`            // 持仓数量
	UpdateTime             int64  `json:"updateTime"`             // 更新时间
}

// BinanceResponse 包含多个仓位和账户信息
type BinanceResponse struct {
	Positions []*BinancePosition `json:"positions"` // 仓位信息
}

// AccountUpdateEvent represents the `ACCOUNT_UPDATE` event pushed via WebSocket
type AccountUpdateEvent struct {
	EventType string `json:"e"`
	EventTime int64  `json:"E"`
	Time      int64  `json:"T"`
	Account   struct {
		Positions []struct {
			Symbol         string `json:"s"`
			PositionAmount string `json:"pa"`
			EntryPrice     string `json:"ep"`
			UnrealizedPnL  string `json:"up"`
			MarginType     string `json:"mt"`
			IsolatedMargin string `json:"iw"`
			PositionSide   string `json:"ps"`
		} `json:"P"`
		M string `json:"m"`
	} `json:"a"`
}

type TradeLiteEvent struct {
	EventType     string `json:"e"` // 事件类型
	EventTime     int64  `json:"E"` // 事件时间
	TradeTime     int64  `json:"T"` // 交易时间
	Symbol        string `json:"s"` // 交易对
	OriginalQty   string `json:"q"` // 订单原始数量
	OriginalPrice string `json:"p"` // 订单原始价格
	IsMaker       bool   `json:"m"` // 该成交是作为挂单成交吗？
	ClientOrderID string `json:"c"` // 客户端自定义订单ID
	OrderSide     string `json:"S"` // 订单方向
	LastFillPrice string `json:"L"` // 订单末次成交价格
	LastFillQty   string `json:"l"` // 订单末次成交量
	TradeID       int64  `json:"t"` // 成交ID
	OrderID       int64  `json:"i"` // 订单ID
}

// OrderTradeUpdate 表示 ORDER_TRADE_UPDATE 事件的结构体
type OrderTradeUpdate struct {
	EventType string `json:"e"` // 事件类型
	EventTime int64  `json:"E"` // 事件时间
	MatchTime int64  `json:"T"` // 撮合时间
	Order     Order  `json:"o"` // 订单详细信息
}

// Order 表示订单的详细信息
type Order struct {
	Symbol                string `json:"s"`   // 交易对
	ClientOrderID         string `json:"c"`   // 客户端自定义订单ID
	OrderSide             string `json:"S"`   // 订单方向
	OrderType             string `json:"o"`   // 订单类型
	TimeInForce           string `json:"f"`   // 有效方式
	OriginalQty           string `json:"q"`   // 订单原始数量
	OriginalPrice         string `json:"p"`   // 订单原始价格
	AveragePrice          string `json:"ap"`  // 订单平均价格
	StopPrice             string `json:"sp"`  // 条件订单触发价格
	ExecutionType         string `json:"x"`   // 本次事件的具体执行类型
	OrderStatus           string `json:"X"`   // 订单的当前状态
	OrderID               int64  `json:"i"`   // 订单ID
	LastExecutedQty       string `json:"l"`   // 订单末次成交量
	CumulativeExecutedQty string `json:"z"`   // 订单累计已成交量
	LastExecutedPrice     string `json:"L"`   // 订单末次成交价格
	FeeAsset              string `json:"N"`   // 手续费资产类型
	FeeAmount             string `json:"n"`   // 手续费数量
	TradeTime             int64  `json:"T"`   // 成交时间
	TradeID               int64  `json:"t"`   // 成交ID
	BuyerNetValue         string `json:"b"`   // 买单净值
	SellerNetValue        string `json:"a"`   // 卖单净值
	IsMaker               bool   `json:"m"`   // 该成交是作为挂单成交吗？
	ReduceOnly            bool   `json:"R"`   // 是否是只减仓单
	TriggerPriceType      string `json:"wt"`  // 触发价类型
	OriginalOrderType     string `json:"ot"`  // 原始订单类型
	PositionSide          string `json:"ps"`  // 持仓方向
	IsClosePosition       bool   `json:"cp"`  // 是否为触发平仓单
	ActivatePrice         string `json:"AP"`  // 追踪止损激活价格
	CallbackRate          string `json:"cr"`  // 追踪止损回调比例
	ProtectTriggerOrder   bool   `json:"pP"`  // 是否开启条件单触发保护
	Ignore1               int64  `json:"si"`  // 忽略字段
	Ignore2               int64  `json:"ss"`  // 忽略字段
	RealizedPnL           string `json:"rp"`  // 该交易实现盈亏
	SelfTradePrevention   string `json:"V"`   // 自成交防止模式
	PriceMatchingMode     string `json:"pm"`  // 价格匹配模式
	GTDTime               int64  `json:"gtd"` // TIF为GTD的订单自动取消时间
}
