package entity

type DoValue struct {
	UserId int
	Value  interface{}
}

type OrderInfo struct {
	Symbol       string
	Amount       float64
	LastAmount   float64
	Oq           float64
	Status       string
	Side         string
	PositionSide string
}
