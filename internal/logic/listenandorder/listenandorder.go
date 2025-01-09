package listenandorder

import (
	"context"
	"fmt"
	"github.com/gateio/gateapi-go/v6"
	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/container/gtype"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/grpool"
	"log"
	"math"
	"plat_order/internal/model/entity"
	"plat_order/internal/service"
	"strconv"
)

type (
	sListenAndOrder struct {
		SymbolsMap *gmap.StrAnyMap

		Users      *gmap.IntAnyMap
		UsersMoney *gmap.IntAnyMap

		TraderInfo         *Trader
		TraderMoney        *gtype.Float64
		TraderPositionSide *gtype.String
		Position           *gmap.StrAnyMap

		Pool *grpool.Pool
	}
)

func init() {
	service.RegisterListenAndOrder(New())
}

func New() *sListenAndOrder {
	return &sListenAndOrder{
		SymbolsMap: gmap.NewStrAnyMap(true), // 交易对信息

		Users:      gmap.NewIntAnyMap(true), // 用户信息
		UsersMoney: gmap.NewIntAnyMap(true), // 用户保证金

		TraderMoney:        gtype.NewFloat64(), // 交易员保证金
		TraderPositionSide: gtype.NewString(),
		Position:           gmap.NewStrAnyMap(true), // 交易员仓位信息

		Pool: grpool.New(), // 全局协程池子
	}
}

type Trader struct {
	apiKey    string
	apiSecret string
}

type TraderPosition struct {
	Id             uint
	Symbol         string
	PositionSide   string
	PositionAmount float64
	MarkPrice      float64
}

func (s *sListenAndOrder) Handle(ctx context.Context) (err error) {

	return err
}

// floatEqual 判断两个浮点数是否在精度范围内相等
func floatEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) <= epsilon
}

// SetSymbol 更新symbol
func (s *sListenAndOrder) SetSymbol(ctx context.Context) (err error) {

	return nil
}

// SetTrader 初始化交易员信息
func (s *sListenAndOrder) SetTrader(ctx context.Context) (err error) {

	return nil
}

// SetUser 初始化用户
func (s *sListenAndOrder) SetUser(ctx context.Context) (err error) {
	var (
		users []*entity.User
	)
	users, err = service.User().GetAllTraders(ctx)
	if nil != err {
		log.Println("InsertUser，初始化用户失败", err)
	}

	for _, v := range users {
		tmpUserId := int(v.Id)
		if s.Users.Contains(tmpUserId) {
			// 变更可否开新仓
			if 2 != v.OpenStatus && 2 == s.Users.Get(tmpUserId).(*entity.User).OpenStatus {
				log.Println("InsertUser，用户暂停:", v)
				s.Users.Set(int(v.Id), v)
			} else if 2 == v.OpenStatus && 2 != s.Users.Get(tmpUserId).(*entity.User).OpenStatus {
				log.Println("InsertUser，用户开启:", v)
				s.Users.Set(int(v.Id), v)
			}

			// 变更num
			if !floatEqual(v.Num, s.Users.Get(tmpUserId).(*entity.User).Num, 1e-7) {
				log.Println("InsertUser，用户变更num:", v)
				s.Users.Set(int(v.Id), v)
			}

			// 已存在跳过
			continue
		}

		// 初始化仓位
		log.Println("InsertUser，新增用户:", v)
		if 1 == v.NeedInit {
			// 获取保证金
			var tmpAmount float64

			_, err = g.Model("new_user").Ctx(ctx).Data("need_init", 0).Where("id=?", v.Id).Update()
			if nil != err {
				log.Println("InsertUser，更新初始化状态失败:", v)
			}

			//strUserId := strconv.FormatUint(uint64(v.Id), 10)
			detail := ""

			if floatEqual(v.Num, 0, 1e-7) {
				log.Println("InsertUser，保证金系数错误：", v)
				continue
			}

			if "binance" == v.Plat {
				detail = service.Binance().GetBinanceInfo(v.ApiKey, v.ApiSecret)
			} else if "gate" == v.Plat {
				var (
					gateUser gateapi.FuturesAccount
				)
				gateUser, err = service.Gate().GetGateContract(v.ApiKey, v.ApiSecret)
				if nil != err {
					fmt.Println("龟兔，拉取保证金失败，gate：", err, v)
				}

				detail = gateUser.Total
			} else {
				log.Println("InsertUser，错误用户信息", v)
				continue
			}

			if 0 < len(detail) {
				var tmp float64
				tmp, err = strconv.ParseFloat(detail, 64)
				if nil != err {
					log.Println("InsertUser，拉取保证金，转化失败：", err, v, detail)
				}

				tmp *= v.Num
				tmpAmount = tmp

				if !s.UsersMoney.Contains(int(v.Id)) {
					log.Println("InsertUser，初始化成功保证金", v, tmpAmount)
					s.UsersMoney.Set(int(v.Id), tmpAmount)
				} else {
					if !floatEqual(tmpAmount, s.UsersMoney.Get(int(v.Id)).(float64), 10) {
						s.UsersMoney.Set(int(v.Id), tmpAmount)
					}
				}
			}

			if floatEqual(tmpAmount, 0, 1e-7) {
				log.Println("InsertUser，保证金不足为0：", tmpAmount, v.Id)
				continue
			}

			tmpTraderBaseMoney := s.TraderMoney.Val()
			// 仓位
			s.Position.Iterator(func(symbol string, vPosition interface{}) bool {
				tmpInsertData := vPosition.(*TraderPosition)
				if floatEqual(tmpInsertData.PositionAmount, 0, 1e-7) {
					return true
				}

				symbolMapKey := v.Plat + tmpInsertData.Symbol
				if !s.SymbolsMap.Contains(symbolMapKey) {
					log.Println("InsertUser，代币信息无效，信息", tmpInsertData, v)
					return true
				}

				// 下单，不用计算数量，新仓位
				var (
					binanceOrderRes *entity.BinanceOrder
					orderInfoRes    *entity.BinanceOrderInfo
				)

				if "binance" == v.Plat {
					var (
						tmpQty        float64
						quantity      string
						quantityFloat float64
						side          string
						positionSide  string
						orderType     = "MARKET"
					)
					if "LONG" == tmpInsertData.PositionSide {
						positionSide = "LONG"
						side = "BUY"
					} else if "SHORT" == tmpInsertData.PositionSide {
						positionSide = "SHORT"
						side = "SELL"
					} else if "BOTH" == tmpInsertData.PositionSide {
						if math.Signbit(tmpInsertData.PositionAmount) {
							positionSide = "SHORT"
							side = "SELL"
						} else {
							positionSide = "LONG"
							side = "BUY"
						}
					} else {
						log.Println("InsertUser，无效信息，信息", v, tmpInsertData)
						return true
					}

					tmpPositionAmount := math.Abs(tmpInsertData.PositionAmount)
					// 本次 代单员币的数量 * (用户保证金/代单员保证金)
					tmpQty = tmpPositionAmount * tmpAmount / tmpTraderBaseMoney // 本次开单数量

					// 精度调整
					if 0 >= s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantityPrecision {
						quantity = fmt.Sprintf("%d", int64(tmpQty))
					} else {
						quantity = strconv.FormatFloat(tmpQty, 'f', s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantityPrecision, 64)
					}

					quantityFloat, err = strconv.ParseFloat(quantity, 64)
					if nil != err {
						log.Println("InsertUser，精度转化", err, quantity)
						return true
					}

					if floatEqual(quantityFloat, 0, 1e-7) {
						return true
					}

					// 请求下单
					binanceOrderRes, orderInfoRes, err = service.Binance().RequestBinanceOrder(tmpInsertData.Symbol, side, orderType, positionSide, quantity, v.ApiKey, v.ApiSecret)
					if nil != err {
						log.Println("InsertUser，下单", v, err, binanceOrderRes, orderInfoRes, tmpInsertData)
					}

					//binanceOrderRes = &binanceOrder{
					//	OrderId:       1,
					//	ExecutedQty:   quantity,
					//	ClientOrderId: "",
					//	Symbol:        "",
					//	AvgPrice:      "",
					//	CumQuote:      "",
					//	Side:          side,
					//	PositionSide:  positionSide,
					//	ClosePosition: false,
					//	Type:          "",
					//	Status:        "",
					//}

					// 下单异常
					if 0 >= binanceOrderRes.OrderId {
						log.Println("InsertUser，下单，订单id为0", v, err, binanceOrderRes, orderInfoRes, tmpInsertData)
						return true
					}

					//var tmpExecutedQty float64
					//tmpExecutedQty = quantityFloat
					//
					//// 不存在新增，这里只能是开仓
					//if !orderMap.Contains(tmpInsertData.Symbol + "&" + positionSide + "&" + strUserId) {
					//	orderMap.Set(tmpInsertData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
					//} else {
					//	tmpExecutedQty += orderMap.Get(tmpInsertData.Symbol + "&" + positionSide + "&" + strUserId).(float64)
					//	orderMap.Set(tmpInsertData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
					//}
				} else if "gate" == v.Plat {

				}

				return true
			})
		}

		// 用户加入
		s.Users.Set(int(v.Id), v)

		// 绑定监听队列 将监听程序加入协程池
		err = service.OrderQueue().BindUserAndQueue(int(v.Id))
		if err != nil {
			return err
		}

		err = s.Pool.AddWithRecover(
			ctx,
			func(ctx context.Context) {
				service.OrderQueue().ListenQueue(ctx, int(v.Id), s.OrderAtPlat)
			},
			func(ctx context.Context, exception error) {
				log.Println("协程panic了，信息:", v, exception)
			})
		if err != nil {
			log.Println("InsertUser，新增协程，错误:", v)
			return err
		}

		// 新增完毕

	}

	return err
}

// OrderAtPlat 在平台下单
func (s *sListenAndOrder) OrderAtPlat(ctx context.Context, doValue *entity.DoValue) {
	log.Println("OrderAtPlat :", doValue)
}
