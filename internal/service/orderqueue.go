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
	IOrderQueue interface {
		// BindUserAndQueue 绑定用户队列
		BindUserAndQueue(userId int) (err error)
		// UnBindUserAndQueue 解除绑定
		UnBindUserAndQueue(userId int) (err error)
		// PushAllQueue 向所有订单队列推送消息
		PushAllQueue(msg interface{})
		// ListenQueue 监听队列
		ListenQueue(ctx context.Context, userId int, do func(context.Context, *entity.DoValue))
	}
)

var (
	localOrderQueue IOrderQueue
)

func OrderQueue() IOrderQueue {
	if localOrderQueue == nil {
		panic("implement not found for interface IOrderQueue, forgot register?")
	}
	return localOrderQueue
}

func RegisterOrderQueue(i IOrderQueue) {
	localOrderQueue = i
}
