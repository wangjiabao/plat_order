// ================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import "github.com/gateio/gateapi-go/v6"

type (
	IGate interface {
		// GetGateContract 获取合约账号信息
		GetGateContract(apiK string, apiS string) (gateapi.FuturesAccount, error)
		// PlaceOrderGate places an order on the Gate.io API with dynamic parameters
		PlaceOrderGate(apiK string, apiS string, contract string, size int64, reduceOnly bool, autoSize string) (gateapi.FuturesOrder, error)
		// PlaceBothOrderGate places an order on the Gate.io API with dynamic parameters
		PlaceBothOrderGate(apiK string, apiS string, contract string, size int64, reduceOnly bool, close bool) (gateapi.FuturesOrder, error)
	}
)

var (
	localGate IGate
)

func Gate() IGate {
	if localGate == nil {
		panic("implement not found for interface IGate, forgot register?")
	}
	return localGate
}

func RegisterGate(i IGate) {
	localGate = i
}
