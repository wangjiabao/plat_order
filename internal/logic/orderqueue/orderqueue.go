package orderqueue

import (
	"context"
	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/container/gqueue"
	"github.com/gogf/gf/v2/errors/gerror"
	"log"
	"plat_order/internal/model/entity"
	"plat_order/internal/service"
)

type (
	sOrderQueue struct {
		safeUserQueue *gmap.IntAnyMap
	}
)

func init() {
	service.RegisterOrderQueue(New())
}

func New() *sOrderQueue {
	return &sOrderQueue{
		safeUserQueue: gmap.NewIntAnyMap(true),
	}
}

// BindUserAndQueue 绑定用户队列
func (s *sOrderQueue) BindUserAndQueue(userId int) (err error) {
	if v, ok := s.safeUserQueue.Get(userId).(*gqueue.Queue); ok {
		if 0 < v.Len() {
			return gerror.Newf("BindUserAndQueue，队列len不为0，%d", userId)
		}

		// todo 仍然不是很安全，在这正确可能突然push进来数据
		v.Close()
	}

	q := gqueue.New()
	s.safeUserQueue.Set(userId, q)
	return err
}

// UnBindUserAndQueue 解除绑定
func (s *sOrderQueue) UnBindUserAndQueue(userId int) (err error) {
	if v, ok := s.safeUserQueue.Get(userId).(*gqueue.Queue); ok {
		v.Close()
	}

	s.safeUserQueue.Remove(userId)
	return err
}

// PushAllQueue 向所有订单队列推送消息
func (s *sOrderQueue) PushAllQueue(msg interface{}) {
	s.safeUserQueue.Iterator(func(userId int, v interface{}) bool {
		if queue, ok := v.(*gqueue.Queue); ok {
			queue.Push(msg)
		} else {
			log.Println("PushAllQueue，无队列信息", userId)
		}

		return true
	})
}

// ListenQueue 监听队列
func (s *sOrderQueue) ListenQueue(ctx context.Context, userId int, do func(context.Context, *entity.DoValue)) {
	queue, ok := s.safeUserQueue.Get(userId).(*gqueue.Queue)
	if !ok {
		log.Println("ListenQueue，无队列信息", userId)
		return
	}

	for {
		queueItem := <-queue.C
		if nil == queueItem { // 队列被关闭
			log.Println("ListenQueue，监听通道被关闭", userId)
			break
		}

		// 执行
		do(ctx, &entity.DoValue{
			UserId: userId,
			Value:  queueItem,
		})
	}

	log.Println("ListenQueue，结束监听", userId)
}
