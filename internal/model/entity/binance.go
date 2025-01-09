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
