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
		Handle(ctx context.Context) (err error)
		// SetSymbol 更新symbol
		SetSymbol(ctx context.Context) (err error)
		// SetTrader 初始化交易员信息
		SetTrader(ctx context.Context) (err error)
		// SetUser 初始化用户
		SetUser(ctx context.Context) (err error)
		// OrderAtPlat 在平台下单
		OrderAtPlat(ctx context.Context, doValue *entity.DoValue)
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
