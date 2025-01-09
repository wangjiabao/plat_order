// ================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import (
	"context"
)

type (
	IListenAndOrder interface {
		Handle(ctx context.Context) (err error)
		// InsertUser 初始化用户
		InsertUser(ctx context.Context) (err error)
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
