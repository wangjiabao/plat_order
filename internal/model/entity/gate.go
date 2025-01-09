package entity

// OrderRequestGate is the request structure for placing an order
type OrderRequestGate struct {
	Contract   string `json:"contract"`
	Size       int64  `json:"size"`
	Tif        string `json:"tif"`
	AutoSize   string `json:"auto_size"`
	ReduceOnly bool   `json:"reduce_only"`
}

// OrderResponseGate is the full response structure for the order API
type OrderResponseGate struct {
	ID           int    `json:"id"`
	User         int    `json:"user"`
	Contract     string `json:"contract"`
	CreateTime   int64  `json:"create_time"`
	Size         int    `json:"size"`
	Iceberg      int    `json:"iceberg"`
	Left         int    `json:"left"`
	Price        string `json:"price"`
	FillPrice    string `json:"fill_price"`
	Mkfr         string `json:"mkfr"`
	Tkfr         string `json:"tkfr"`
	Tif          string `json:"tif"`
	Refu         int    `json:"refu"`
	IsReduceOnly bool   `json:"is_reduce_only"`
	IsClose      bool   `json:"is_close"`
	IsLiq        bool   `json:"is_liq"`
	Text         string `json:"text"`
	Status       string `json:"status"`
	FinishTime   int64  `json:"finish_time"`
	FinishAs     string `json:"finish_as"`
	StpID        int    `json:"stp_id"`
	StpAct       string `json:"stp_act"`
	AmendText    string `json:"amend_text"`
}
