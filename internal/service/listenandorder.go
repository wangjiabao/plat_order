// ================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import (
	"context"
	"plat_order/internal/model/entity"
)

type (
	IListenAndOrder interface {
		// SetSymbol 更新symbol
		SetSymbol(ctx context.Context) (err error)
		// PullAndSetTraderUserPositionSide 获取并更新持仓方向
		PullAndSetTraderUserPositionSide(ctx context.Context) (err error)
		// SetUser 初始化用户
		SetUser(ctx context.Context) (err error)
		// OrderAtPlat 在平台下单
		OrderAtPlat(ctx context.Context, doValue *entity.DoValue)
		// Run 监控仓位 pulls binance data and orders
		Run(ctx context.Context)
	}
)

var (
	localListenAndOrder IListenAndOrder
)

func ListenAndOrder() IListenAndOrder {
	if localListenAndOrder == nil {
		panic("implement not found for interface IListenAndOrder, forgot register?")
	}
	return localListenAndOrder
}

func RegisterListenAndOrder(i IListenAndOrder) {
	localListenAndOrder = i
}
