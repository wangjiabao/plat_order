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
		// GetBinanceInfo 获取账户信息
		GetBinanceInfo(apiK, apiS string) string
		RequestBinanceOrder(symbol string, side string, orderType string, positionSide string, quantity string, apiKey string, secretKey string) (*entity.BinanceOrder, *entity.BinanceOrderInfo, error)
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
