// ================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import (
	"plat_order/internal/model/entity"
)

type (
	IOrderQueue interface {
		// BindUserAndQueue 绑定用户队列
		BindUserAndQueue(userId int) (err error)
		// PushAllQueue 向所有订单队列推送消息
		PushAllQueue(msg interface{})
		// ListenQueue 监听队列
		ListenQueue(userId int, do func(err *entity.DoValue))
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
