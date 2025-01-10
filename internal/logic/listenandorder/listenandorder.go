package listenandorder

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gateio/gateapi-go/v6"
	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/container/gtype"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/grpool"
	"github.com/gogf/gf/v2/os/gtimer"
	"github.com/gorilla/websocket"
	"log"
	"math"
	"plat_order/internal/logic/binance"
	"plat_order/internal/model/entity"
	"plat_order/internal/service"
	"strconv"
	"strings"
	"time"
)

type (
	sListenAndOrder struct {
		SymbolsMap *gmap.StrAnyMap

		Users             *gmap.IntAnyMap
		UsersMoney        *gmap.IntAnyMap
		UsersPositionSide *gmap.IntStrMap
		OrderMap          *gmap.Map

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

		Users:             gmap.NewIntAnyMap(true), // 用户信息
		UsersMoney:        gmap.NewIntAnyMap(true), // 用户保证金
		UsersPositionSide: gmap.NewIntStrMap(true), // 用户持仓方向
		OrderMap:          gmap.New(true),

		TraderInfo: &Trader{
			apiKey:    "vgRfQo3Boiu4sXAyYclAKEGg2u2MrmokleKvZDW90qdKppzwSCF70pe6udxmXxmU",
			apiSecret: "bLoBCQujP0Yud2uYvHbBl8ykv7lkEcK0Mb0PaaLeA7ApnW13OSBODnGyEpsyXy2T",
		},
		TraderMoney:        gtype.NewFloat64(),      // 交易员保证金
		TraderPositionSide: gtype.NewString(),       // 交易员持仓方向
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

// floatEqual 判断两个浮点数是否在精度范围内相等
func floatEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) <= epsilon
}

// lessThanOrEqualZero 小于等于0
func lessThanOrEqualZero(a, epsilon float64) bool {
	return a-0 < epsilon || math.Abs(a-0) < epsilon
}

// SetSymbol 更新symbol
func (s *sListenAndOrder) SetSymbol(ctx context.Context) (err error) {
	// 获取代币信息
	var (
		symbols []*entity.LhCoinSymbol
	)

	err = g.Model("lh_coin_symbol").Ctx(ctx).Scan(&symbols)
	if nil != err || 0 >= len(symbols) {
		log.Println("SetSymbol，币种，数据库查询错误：", err)
		return err
	}

	// 处理
	for _, vSymbols := range symbols {
		s.SymbolsMap.Set(vSymbols.Plat+vSymbols.Symbol+"USDT", vSymbols)
	}

	return nil
}

// PullAndSetTraderUserPositionSide 获取并更新持仓方向
func (s *sListenAndOrder) PullAndSetTraderUserPositionSide(ctx context.Context) (err error) {
	s.TraderPositionSide.Set("BOTH")
	// todo 用户和trader的持仓方向更新

	return nil
}

// SetUser 初始化用户
func (s *sListenAndOrder) SetUser(ctx context.Context) (err error) {
	var (
		users []*entity.User
	)
	users, err = service.User().GetTradersApiIsOk(ctx)
	if nil != err {
		log.Println("SetUser，初始化用户失败", err)
	}

	tmpUserMap := make(map[uint]*entity.User, 0)
	for _, vUsers := range users {
		tmpUserMap[vUsers.Id] = vUsers
	}

	for _, v := range users {
		if s.Users.Contains(int(v.Id)) {
			// 变更可否开新仓
			if 2 != v.OpenStatus && 2 == s.Users.Get(int(v.Id)).(*entity.User).OpenStatus {
				log.Println("SetUser，用户暂停:", v)
				s.Users.Set(int(v.Id), v)
			} else if 2 == v.OpenStatus && 2 != s.Users.Get(int(v.Id)).(*entity.User).OpenStatus {
				log.Println("SetUser，用户开启:", v)
				s.Users.Set(int(v.Id), v)
			}

			// 变更num
			if !floatEqual(v.Num, s.Users.Get(int(v.Id)).(*entity.User).Num, 1e-7) {
				log.Println("SetUser，用户变更num:", v)
				s.Users.Set(int(v.Id), v)
			}

			// 已存在跳过
			continue
		}

		// 修改杠杆20倍，todo

		// 修改持仓模式 todo
		if 0 >= len(s.TraderPositionSide.Val()) {
			log.Println("SetUser，更新初始化状态失败，交易员持仓模式未知")
			break
		}

		s.UsersPositionSide.Set(int(v.Id), s.TraderPositionSide.Val())
		if 0 >= len(s.UsersPositionSide.Get(int(v.Id))) {
			log.Println("SetUser，仓位方向未识别：", v)
			continue
		}

		tmpUserPositionSide := s.TraderPositionSide.Val()

		// 交易员保证金
		tmpTraderBaseMoney := s.TraderMoney.Val()
		// 获取用户保证金
		var tmpAmount float64
		strUserId := strconv.FormatUint(uint64(v.Id), 10)
		detail := ""

		if lessThanOrEqualZero(v.Num, 1e-7) {
			log.Println("SetUser，保证金系数错误：", v)
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
				log.Println("SetUser，拉取保证金失败，gate：", err, v)
			}

			detail = gateUser.Total
		} else {
			log.Println("SetUser，错误用户信息", v)
			continue
		}

		if 0 < len(detail) {
			var tmp float64
			tmp, err = strconv.ParseFloat(detail, 64)
			if nil != err {
				log.Println("SetUser，拉取保证金，转化失败：", err, v, detail)
			}

			tmp *= v.Num
			tmpAmount = tmp

			if !s.UsersMoney.Contains(int(v.Id)) {
				log.Println("SetUser，初始化成功保证金", v, tmpAmount)
				s.UsersMoney.Set(int(v.Id), tmpAmount)
			} else {
				if !floatEqual(tmpAmount, s.UsersMoney.Get(int(v.Id)).(float64), 10) {
					s.UsersMoney.Set(int(v.Id), tmpAmount)
				}
			}
		}

		// 初始化仓位
		log.Println("SetUser，新增用户:", v)
		if 1 == v.NeedInit {
			_, err = g.Model("new_user").Ctx(ctx).Data("need_init", 0).Where("id=?", v.Id).Update()
			if nil != err {
				log.Println("SetUser，更新初始化状态失败:", v)
			}

			// 交易员保证金信息
			if lessThanOrEqualZero(tmpTraderBaseMoney, 1e-7) {
				log.Println("SetUser，交易员保证金不足为0：", tmpTraderBaseMoney, v.Id)
				continue
			}

			// 保证金信息
			if lessThanOrEqualZero(tmpAmount, 1e-7) {
				log.Println("SetUser，保证金不足为0：", tmpAmount, v.Id)
				continue
			}

			// 仓位
			s.Position.Iterator(func(symbolKey string, vPosition interface{}) bool {
				tmpInsertData := vPosition.(*TraderPosition)

				// 这里有正负之分
				if floatEqual(tmpInsertData.PositionAmount, 0, 1e-7) {
					return true
				}

				symbolMapKey := v.Plat + tmpInsertData.Symbol
				if !s.SymbolsMap.Contains(symbolMapKey) {
					log.Println("SetUser，代币信息无效，信息", tmpInsertData, v)
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

					if "BOTH" == tmpUserPositionSide {
						// 单向持仓
						if "BOTH" == tmpInsertData.PositionSide {
							if math.Signbit(tmpInsertData.PositionAmount) {
								positionSide = "BOTH"
								side = "SELL"
							} else {
								positionSide = "BOTH"
								side = "BUY"
							}
						} else {
							return true
						}
					} else if "ALL" == tmpUserPositionSide {
						// 双向持仓
						if "LONG" == tmpInsertData.PositionSide {
							positionSide = "LONG"
							side = "BUY"
						} else if "SHORT" == tmpInsertData.PositionSide {
							positionSide = "SHORT"
							side = "SELL"
						} else {
							return true
						}
					} else {
						log.Println("SetUser，持续方向信息无效，信息", tmpInsertData, v, tmpUserPositionSide)
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
						log.Println("SetUser，精度转化", err, quantity)
						return true
					}

					if lessThanOrEqualZero(quantityFloat, 1e-7) {
						return true
					}

					// 请求下单
					binanceOrderRes, orderInfoRes, err = service.Binance().RequestBinanceOrder(tmpInsertData.Symbol, side, orderType, positionSide, quantity, v.ApiKey, v.ApiSecret)
					if nil != err {
						log.Println("SetUser，下单", v, err, binanceOrderRes, orderInfoRes, tmpInsertData)
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
						log.Println("SetUser，下单，订单id为0", v, err, binanceOrderRes, orderInfoRes, tmpInsertData)
						return true
					}

					var tmpExecutedQty float64
					tmpExecutedQty = quantityFloat

					if "BOTH" == positionSide {
						if "SELL" == side {
							tmpExecutedQty = -tmpExecutedQty
						}
					}

					// 不存在新增，这里只能是开仓
					s.OrderMap.Set(tmpInsertData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
				} else if "gate" == v.Plat {
					if 0 >= s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier {
						log.Println("SetUser，代币信息无效，信息", tmpInsertData, v)
						return true
					}

					var (
						tmpQty        float64
						gateRes       gateapi.FuturesOrder
						side          string
						symbol        = s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).Symbol + "_USDT"
						positionSide  string
						quantity      string
						quantityInt64 int64
						quantityFloat float64
						reduceOnly    bool
					)

					tmpPositionAmount := math.Abs(tmpInsertData.PositionAmount)
					// 本次 代单员币的数量 * (用户保证金/代单员保证金)
					tmpQty = tmpPositionAmount * tmpAmount / tmpTraderBaseMoney // 本次开单数量

					// 转化为张数=币的数量/每张币的数量
					tmpQtyOkx := tmpQty / s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier
					// 按张的精度转化，
					quantityInt64 = int64(math.Round(tmpQtyOkx))
					quantityFloat = float64(quantityInt64)
					if lessThanOrEqualZero(quantityFloat, 1e-7) {
						log.Println("SetUser，开仓数量小于0，信息", tmpInsertData, v, quantityFloat)
						return true
					}

					tmpExecutedQty := quantityFloat
					if "BOTH" == tmpUserPositionSide {
						// 单向持仓
						if "BOTH" == tmpInsertData.PositionSide {
							if math.Signbit(tmpInsertData.PositionAmount) {
								positionSide = "BOTH"
								side = "SELL"

								quantityFloat = -quantityFloat
								quantityInt64 = -quantityInt64
							} else {
								positionSide = "BOTH"
								side = "BUY"
							}
						} else {
							return true
						}

						quantity = strconv.FormatFloat(quantityFloat, 'f', -1, 64)

						gateRes, err = service.Gate().PlaceBothOrderGate(v.ApiKey, v.ApiSecret, symbol, quantityInt64, reduceOnly, false)
						if nil != err {
							log.Println("SetUser，gate，下单错误", err, tmpInsertData, v, quantity, quantityInt64, gateRes)
							return true
						}

						if 0 >= gateRes.Id {
							log.Println("SetUser，gate，下单错误", err, tmpInsertData, v, quantity, quantityInt64, gateRes)
							return true
						}
					} else if "ALL" == tmpUserPositionSide {
						// 双向持仓
						if "LONG" == tmpInsertData.PositionSide {
							positionSide = "LONG"
							side = "BUY"
						} else if "SHORT" == tmpInsertData.PositionSide {
							positionSide = "SHORT"
							side = "SELL"

							quantityFloat = -quantityFloat
							quantityInt64 = -quantityInt64
						} else {
							return true
						}

						quantity = strconv.FormatFloat(quantityFloat, 'f', -1, 64)

						gateRes, err = service.Gate().PlaceOrderGate(v.ApiKey, v.ApiSecret, symbol, quantityInt64, reduceOnly, "")
						if nil != err {
							log.Println("SetUser，gate，下单错误", err, tmpInsertData, v, quantity, quantityInt64, gateRes)
							return true
						}

						if 0 >= gateRes.Id {
							log.Println("SetUser，gate，下单错误", err, tmpInsertData, v, quantity, quantityInt64, gateRes)
							return true
						}
					} else {
						log.Println("SetUser，持续方向信息无效，信息", tmpInsertData, v, tmpUserPositionSide)
						return true
					}

					if "BOTH" == positionSide {
						if "SELL" == side {
							tmpExecutedQty = -tmpExecutedQty
						}
					}
					// 不存在新增，这里只能是开仓
					s.OrderMap.Set(tmpInsertData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
				}

				return true
			})
		}

		// 用户加入
		s.Users.Set(int(v.Id), v)

		// 绑定监听队列 将监听程序加入协程池
		err = service.OrderQueue().BindUserAndQueue(int(v.Id))
		if err != nil {
			log.Println("SetUser，绑定新增协程，错误:", v, err)
			continue
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
			log.Println("SetUser，新增协程，错误:", v, err)
			continue
		}

		// 新增完毕
	}

	// 第二遍比较，删除
	tmpIds := make([]int, 0)
	s.Users.Iterator(func(k int, v interface{}) bool {
		if _, ok := tmpUserMap[uint(k)]; !ok {
			tmpIds = append(tmpIds, k)
		}
		return true
	})

	// 删除的人
	for _, vTmpIds := range tmpIds {
		log.Println("SetUser，删除用户，解除队列绑定，队列close时，对应的监听协程会自动结束:", vTmpIds)
		s.Users.Remove(vTmpIds)

		// 删除任务
		err = service.OrderQueue().UnBindUserAndQueue(vTmpIds)
		if err != nil {
			log.Println("SetUser，解除队列绑定，错误:", vTmpIds, err)
			continue
		}

		tmpRemoveUserKey := make([]string, 0)
		// 遍历map
		s.OrderMap.Iterator(func(k interface{}, v interface{}) bool {
			parts := strings.Split(k.(string), "&")
			if 3 != len(parts) {
				return true
			}

			var (
				uid uint64
			)
			uid, err = strconv.ParseUint(parts[2], 10, 64)
			if nil != err {
				log.Println("SetUser，删除用户,解析id错误:", vTmpIds)
			}

			if uid != uint64(vTmpIds) {
				return true
			}

			tmpRemoveUserKey = append(tmpRemoveUserKey, k.(string))
			return true
		})

		for _, vK := range tmpRemoveUserKey {
			if s.OrderMap.Contains(vK) {
				s.OrderMap.Remove(vK)
			}
		}
	}

	return nil
}

// OrderAtPlat 在平台下单
func (s *sListenAndOrder) OrderAtPlat(ctx context.Context, doValue *entity.DoValue) {
	log.Println("OrderAtPlat :", doValue)
}

// Run 监控仓位 pulls binance data and orders
func (s *sListenAndOrder) Run(ctx context.Context) {
	var (
		err             error
		binancePosition []*entity.BinancePosition
	)

	binancePosition = service.Binance().GetBinancePositionInfo(s.TraderInfo.apiKey, s.TraderInfo.apiSecret)
	if nil == binancePosition {
		log.Println("错误查询仓位")
		return
	}

	// 用于数据库更新
	insertData := make([]*TraderPosition, 0)

	for _, position := range binancePosition {
		//fmt.Println("初始化：", position.Symbol, position.PositionAmt, position.PositionSide)

		// 新增
		var (
			currentAmount    float64
			currentAmountAbs float64
		)
		currentAmount, err = strconv.ParseFloat(position.PositionAmt, 64)
		if nil != err {
			log.Println("新，解析金额出错，信息", position, currentAmount)
		}
		currentAmountAbs = math.Abs(currentAmount) // 绝对值

		if !s.Position.Contains(position.Symbol + position.PositionSide) {
			// 以下内容，当系统无此仓位时
			if "BOTH" != position.PositionSide {
				insertData = append(insertData, &TraderPosition{
					Symbol:         position.Symbol,
					PositionSide:   position.PositionSide,
					PositionAmount: currentAmountAbs,
				})

			} else {
				// 单向持仓
				insertData = append(insertData, &TraderPosition{
					Symbol:         position.Symbol,
					PositionSide:   position.PositionSide,
					PositionAmount: currentAmount, // 正负数保持
				})
			}
		} else {
			log.Println("已存在数据")
		}
	}

	if 0 < len(insertData) {
		// 新增数据
		for _, vIBinancePosition := range insertData {
			s.Position.Set(vIBinancePosition.Symbol+vIBinancePosition.PositionSide, &TraderPosition{
				Symbol:         vIBinancePosition.Symbol,
				PositionSide:   vIBinancePosition.PositionSide,
				PositionAmount: vIBinancePosition.PositionAmount,
			})
		}
	}

	// 仓位补足系统
	s.Position.Iterator(func(k string, v interface{}) bool {
		vPosition := v.(*TraderPosition)
		if s.Position.Contains(vPosition.Symbol + "BOTH") {
			s.Position.Set(vPosition.Symbol+"BOTH", &TraderPosition{
				Symbol:         vPosition.Symbol,
				PositionSide:   "BOTH",
				PositionAmount: 0,
			})
		}

		if s.Position.Contains(vPosition.Symbol + "LONG") {
			s.Position.Set(vPosition.Symbol+"LONG", &TraderPosition{
				Symbol:         vPosition.Symbol,
				PositionSide:   "LONG",
				PositionAmount: 0,
			})
		}

		if s.Position.Contains(vPosition.Symbol + "SHORT") {
			s.Position.Set(vPosition.Symbol+"SHORT", &TraderPosition{
				Symbol:         vPosition.Symbol,
				PositionSide:   "SHORT",
				PositionAmount: 0,
			})
		}

		return true
	})

	// Refresh listen key every 29 minutes
	handleRenewListenKey := func(ctx context.Context) {
		err = service.Binance().RenewListenKey(s.TraderInfo.apiKey)
		if err != nil {
			log.Println("Error renewing listen key:", err)
		}
	}
	gtimer.AddSingleton(ctx, time.Minute*29, handleRenewListenKey)

	// Create listen key and connect to WebSocket
	connect := func(ctx context.Context) {
		err = service.Binance().CreateListenKey(s.TraderInfo.apiKey)
		if err != nil {
			log.Println("Error creating listen key:", err)
			return
		}

		// Connect WebSocket initially
		err = service.Binance().ConnectWebSocket()
		if err != nil {
			log.Println("Error connecting WebSocket:", err)
			return
		}
	}

	connect(ctx)
	gtimer.AddSingleton(ctx, time.Hour*23, connect)

	defer func(conn *websocket.Conn) {
		err = conn.Close()
		if err != nil {

		}
	}(binance.Conn)

	// Listen for WebSocket messages
	for {
		var message []byte
		_, message, err = binance.Conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err, time.Now())

			// 可能是23小时的更换conn
			time.Sleep(100 * time.Millisecond)
			continue
		}

		var event *entity.OrderTradeUpdate
		if err = json.Unmarshal(message, &event); err != nil {
			log.Println("Failed to parse message:", err, string(message), time.Now())
			continue
		}

		if event.EventType != "ORDER_TRADE_UPDATE" {
			continue
		}

		fmt.Println(3, event, "\n\n\n")

		if "MARKET" == event.Order.OriginalOrderType {
			// 市价
			if "NEW" == event.Order.ExecutionType {
				// todo 构造
				fmt.Println("市价，new：", event, "\n\n\n")
			}

		} else if "LIMIT" == event.Order.OriginalOrderType {
			// 限价 开始交易，我们的反应时全部执行市价，开或关
			if "TRADE" == event.Order.ExecutionType {
				// todo 构造
				fmt.Println("限价，trade：", event, "\n\n\n")
			}
		}

		continue
	}

}
