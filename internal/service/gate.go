// ================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import "github.com/gateio/gateapi-go/v6"

type (
	IGate interface {
		// GetGateContract 获取合约账号信息
		GetGateContract(apiK, apiS string) (gateapi.FuturesAccount, error)
		// PlaceOrderGate places an order on the Gate.io API with dynamic parameters
		PlaceOrderGate(apiK, apiS, contract string, size int64, reduceOnly bool, autoSize string) (gateapi.FuturesOrder, error)
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
