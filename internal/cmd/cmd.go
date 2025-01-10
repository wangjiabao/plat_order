package cmd

import (
	"context"
	"github.com/gogf/gf/v2/os/gtimer"
	"log"
	"time"

	"github.com/gogf/gf/v2/os/gcmd"
	"plat_order/internal/service"
)

var (
	Main = gcmd.Command{
		Name:  "main",
		Usage: "main",
		Brief: "start server",
		Func: func(ctx context.Context, parser *gcmd.Parser) (err error) {
			lao := service.ListenAndOrder()

			err = lao.SetSymbol(ctx)
			if nil != err {
				log.Println("启动错误，币种信息：", err)
			}

			// 300秒/次，币种信息
			handle := func(ctx context.Context) {
				err = lao.SetSymbol(ctx)
				if nil != err {
					log.Println("任务错误，币种信息：", err)
				}
			}
			gtimer.AddSingleton(ctx, time.Minute*5, handle)

			err = lao.PullAndSetTraderUserPositionSide(ctx)
			if nil != err {
				log.Println("启动错误，同步交易员和用户持仓方向：", err)
			}

			// 1分钟/次，同步持仓信息和持仓方向
			handle2 := func(ctx context.Context) {
				err = lao.PullAndSetTraderUserPositionSide(ctx)
				if nil != err {
					log.Println("任务错误，同步交易员和用户持仓方向：", err)
				}
			}
			gtimer.AddSingleton(ctx, time.Minute*1, handle2)

			err = lao.SetUser(ctx)
			if nil != err {
				log.Println("启动错误，设置用户：", err)
			}

			// 30秒/次，更新用户信息
			handle3 := func(ctx context.Context) {
				err = lao.SetUser(ctx)
				if nil != err {
					log.Println("任务错误，设置用户：", err)
				}
			}
			gtimer.AddSingleton(ctx, time.Second*30, handle3)

			lao.Run(ctx)
			return nil
		},
	}
)
