// ================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import (
	"plat_order/internal/model/entity"
)

type (
	IBinance interface {
		// GetBinancePositionSide 获取账户信息
		GetBinancePositionSide(apiK string, apiS string) string
		// GetBinanceInfo 获取账户信息
		GetBinanceInfo(apiK string, apiS string) string
		RequestBinancePositionSide(positionSide string, apiKey string, secretKey string) (error, bool)
		RequestBinanceOrder(symbol string, side string, orderType string, positionSide string, quantity string, apiKey string, secretKey string) (*entity.BinanceOrder, *entity.BinanceOrderInfo, error)
		// GetBinancePositionInfo 获取账户信息
		GetBinancePositionInfo(apiK string, apiS string) []*entity.BinancePosition
		// CreateListenKey creates a new ListenKey for user data stream
		CreateListenKey(apiKey string) error
		// RenewListenKey renews the ListenKey for user data stream
		RenewListenKey(apiKey string) error
		// ConnectWebSocket safely connects to the WebSocket and updates conn
		ConnectWebSocket() error
	}
)

var (
	localBinance IBinance
)

func Binance() IBinance {
	if localBinance == nil {
		panic("implement not found for interface IBinance, forgot register?")
	}
	return localBinance
}

func RegisterBinance(i IBinance) {
	localBinance = i
}
