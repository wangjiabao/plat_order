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
			apiKey:    "",
			apiSecret: "",
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
	Symbol         string
	PositionSide   string
	PositionAmount float64
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

// PullAndSetBaseMoneyNewGuiTuAndUser 拉取binance保证金数据
func (s *sListenAndOrder) PullAndSetBaseMoneyNewGuiTuAndUser(ctx context.Context) {
	var (
		err error
		one string
	)

	one = service.Binance().GetBinanceInfo(s.TraderInfo.apiKey, s.TraderInfo.apiSecret)
	if 0 < len(one) {
		var tmp float64
		tmp, err = strconv.ParseFloat(one, 64)
		if nil != err {
			log.Println("拉取保证金，转化失败：", err)
		}

		if !floatEqual(tmp, s.TraderMoney.Val(), 10) {
			//log.Println("龟兔，变更保证金", tmp, baseMoneyGuiTu.Val())
			s.TraderMoney.Set(tmp)
		}
	}

	time.Sleep(300 * time.Millisecond)

	var (
		users []*entity.User
	)
	err = g.Model("user").Ctx(ctx).
		Where("api_status=?", 1).
		Scan(&users)
	if nil != err {
		log.Println("拉取保证金，数据库查询错误：", err)
		return
	}

	tmpUserMap := make(map[uint]*entity.User, 0)
	for _, vUsers := range users {
		tmpUserMap[vUsers.Id] = vUsers
	}

	s.Users.Iterator(func(k int, v interface{}) bool {
		vGlobalUsers := v.(*entity.User)

		if _, ok := tmpUserMap[vGlobalUsers.Id]; !ok {
			log.Println("变更保证金，用户数据错误，数据库不存在：", vGlobalUsers)
			return true
		}

		var (
			detail string
		)
		if "binance" == vGlobalUsers.Plat {
			detail = service.Binance().GetBinanceInfo(vGlobalUsers.ApiKey, vGlobalUsers.ApiSecret)
		} else if "gate" == vGlobalUsers.Plat {
			var (
				gateUser gateapi.FuturesAccount
			)
			gateUser, err = service.Gate().GetGateContract(vGlobalUsers.ApiKey, vGlobalUsers.ApiSecret)
			if nil != err {
				log.Println("拉取保证金失败，gate：", err, vGlobalUsers)
				return true
			}

			detail = gateUser.Total

		} else {
			log.Println("获取平台保证金，错误用户信息", vGlobalUsers)
			return true
		}

		if 0 < len(detail) {
			var tmp float64
			tmp, err = strconv.ParseFloat(detail, 64)
			if nil != err {
				log.Println("拉取保证金，转化失败：", err, vGlobalUsers)
				return true
			}

			tmp *= tmpUserMap[vGlobalUsers.Id].Num
			if !s.UsersMoney.Contains(int(vGlobalUsers.Id)) {
				log.Println("初始化成功保证金", vGlobalUsers, tmp, tmpUserMap[vGlobalUsers.Id].Num)
				s.UsersMoney.Set(int(vGlobalUsers.Id), tmp)
			} else {
				//log.Println("测试保证金比较", tmp, baseMoneyUserAllMap.Get(int(vGlobalUsers.Id)).(float64), lessThanOrEqualZero(tmp, baseMoneyUserAllMap.Get(int(vGlobalUsers.Id)).(float64), 1))
				if !floatEqual(tmp, s.UsersMoney.Get(int(vGlobalUsers.Id)).(float64), 10) {
					//log.Println("变更成功", int(vGlobalUsers.Id), tmp, tmpUserMap[vGlobalUsers.Id].Num)
					s.UsersMoney.Set(int(vGlobalUsers.Id), tmp)
				}
			}
		} else {
			log.Println("保证金为0", vGlobalUsers)
		}

		time.Sleep(300 * time.Millisecond)
		return true
	})
}

// PullAndSetTraderUserPositionSide 获取并更新持仓方向
func (s *sListenAndOrder) PullAndSetTraderUserPositionSide(ctx context.Context) (err error) {
	s.TraderPositionSide.Set("ALL")
	// todo 用户和trader的持仓方向更新
	var (
		positionSide string
	)
	positionSide = service.Binance().GetBinancePositionSide(s.TraderInfo.apiKey, s.TraderInfo.apiSecret)
	if 0 > len(positionSide) {
		log.Println("查询交易员持仓方向失败")
		return nil
	}

	if "BOTH" != positionSide && "ALL" != positionSide {
		log.Println("查询交易员持仓方向失败2")
		return nil
	}

	if positionSide != s.TraderPositionSide.Val() {
		s.TraderPositionSide.Set(positionSide)
	}

	// 用户
	s.Users.Iterator(func(k int, v interface{}) bool {
		tmpUser := v.(*entity.User)
		if positionSide == s.UsersPositionSide.Get(int(tmpUser.Id)) {
			return true
		}

		if "binance" == tmpUser.Plat {
			tmp := "true"
			if "BOTH" == positionSide {
				tmp = "false"
			}

			var (
				res bool
			)
			err, res = service.Binance().RequestBinancePositionSide(tmp, tmpUser.ApiKey, tmpUser.ApiSecret)
			if nil != err || !res {
				log.Println("更新用户持仓模式失败", tmpUser, tmp)
				return true
			}

		} else if "gate" == tmpUser.Plat {
			var dual = true
			if "BOTH" == positionSide {
				dual = false
			}

			dual, err = service.Gate().SetDual(tmpUser.ApiKey, tmpUser.ApiSecret, dual)
			if nil != err {
				log.Println("更新用户持仓模式失败", v, err)
				return true
			}

		} else {
			log.Println("更新用户持仓模式失败，未知信息", tmpUser)
			return true
		}

		s.UsersPositionSide.Set(int(tmpUser.Id), positionSide)
		log.Println("更新持仓模式成功，用户：", tmpUser)
		return true
	})

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

		if 0 >= len(s.TraderPositionSide.Val()) {
			log.Println("SetUser，更新初始化状态失败，交易员持仓模式未知")
			break
		}

		if "binance" == v.Plat {
			tmp := "true"
			if "BOTH" == s.TraderPositionSide.Val() {
				tmp = "false"
			} else if "ALL" == s.TraderPositionSide.Val() {
				tmp = "true"
			} else {
				log.Println("SetUser，更新初始化状态失败，交易员持仓模式未知2")
				break
			}

			var (
				res bool
			)
			err, res = service.Binance().RequestBinancePositionSide(tmp, v.ApiKey, v.ApiSecret)
			if nil != err || !res {
				log.Println("SetUser，更新用户持仓模式失败", v, err, tmp)
				continue
			}

		} else if "gate" == v.Plat {
			var dual bool
			if "BOTH" == s.TraderPositionSide.Val() {
				dual = false
			} else if "ALL" == s.TraderPositionSide.Val() {
				dual = true
			} else {
				log.Println("SetUser，更新初始化状态失败，交易员持仓模式未知3")
				break
			}

			dual, err = service.Gate().SetDual(v.ApiKey, v.ApiSecret, dual)
			if nil != err {
				log.Println("SetUser，更新用户持仓模式失败", v, err)
				continue
			}
		} else {
			log.Println("SetUser，更新用户持仓模式失败，未知信息", v)
			continue
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
			_, err = g.Model("user").Ctx(ctx).Data("need_init", 0).Where("id=?", v.Id).Update()
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
					binanceOrderRes, orderInfoRes, err = service.Binance().RequestBinanceOrder(tmpInsertData.Symbol, side, orderType, positionSide, quantity, v.ApiKey, v.ApiSecret, false)
					if nil != err {
						log.Println("SetUser，下单", v, err, binanceOrderRes, orderInfoRes, tmpInsertData)
						return true
					}

					//binanceOrderRes = &entity.BinanceOrder{
					//	OrderId:       1,
					//	ExecutedQty:   quantity,
					//	ClientOrderId: "",
					//	Symbol:        "",
					//	AvgPrice:      "",
					//	CumQuote:      "",
					//	Side:          side,
					//	PositionSide:  positionSide,
					//	ClosePosition: false,
					//	Type:          orderType,
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
		} else {
			if "binance" == v.Plat {
				var (
					binancePosition []*entity.BinancePosition
				)

				binancePosition = service.Binance().GetBinancePositionInfo(v.ApiKey, v.ApiSecret)
				if nil == binancePosition {
					log.Println("初始化用户仓位，错误查询仓位，binance")
					continue
				}

				for _, position := range binancePosition {
					//log.Println("初始化：", position.Symbol, position.PositionAmt, position.PositionSide)

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
					if s.Position.Contains(position.Symbol + position.PositionSide) {
						tmpPosition := s.Position.Get(position.Symbol + position.PositionSide)
						if nil == tmpPosition {
							continue
						}

						// 仓位无
						if floatEqual(tmpPosition.(*TraderPosition).PositionAmount, 0, 1e-7) {
							continue
						}

						// 以下内容，当系统无此仓位时
						if "BOTH" == position.PositionSide {
							s.OrderMap.Set(position.Symbol+"&"+position.PositionSide+"&"+strUserId, currentAmount)
							log.Println("初始化，仓位拉取binance：", position.Symbol+"&"+position.PositionSide+"&"+strUserId, currentAmount)
						} else {
							s.OrderMap.Set(position.Symbol+"&"+position.PositionSide+"&"+strUserId, currentAmountAbs)
							log.Println("初始化，仓位拉取binance：", position.Symbol+"&"+position.PositionSide+"&"+strUserId, currentAmountAbs)
						}
					}
				}
			} else if "gate" == v.Plat {

				var (
					gatePositions []gateapi.Position
				)
				gatePositions, err = service.Gate().GetListPositions(v.ApiKey, v.ApiSecret)
				if nil != err {
					log.Println("初始化用户仓位，错误查询仓位，gate", err)
					continue
				}
				for _, position := range gatePositions {
					if len(position.Contract) <= 5 {
						continue
					}

					positionSide := "BOTH"
					var tmpSymbol string
					tmpSymbolKey := position.Contract[:len(position.Contract)-5]
					tmpSymbol = tmpSymbolKey + "USDT"
					if "single" == position.Mode {
						tmpSymbolKey += "USDTBOTH"
					} else if "dual_long" == position.Mode {
						tmpSymbolKey += "USDTLONG"
						positionSide = "LONG"
					} else if "dual_short" == position.Mode {
						tmpSymbolKey += "USDTSHORT"
						positionSide = "SHORT"
					} else {
						log.Println("初始化用户仓位，错误查询仓位，gate，未识别", position.Mode)
						continue
					}

					if s.Position.Contains(tmpSymbolKey) {
						tmpPosition := s.Position.Get(tmpSymbolKey)
						if nil == tmpPosition {
							continue
						}

						// 仓位无
						if floatEqual(tmpPosition.(*TraderPosition).PositionAmount, 0, 1e-7) {
							continue
						}

						// 以下内容，当系统无此仓位时
						if "BOTH" == positionSide {
							s.OrderMap.Set(tmpSymbol+"&"+positionSide+"&"+strUserId, position.Size)
							log.Println("初始化，仓位拉取gate：", tmpSymbol+"&"+positionSide+"&"+strUserId, position.Size)
						} else {
							s.OrderMap.Set(tmpSymbol+"&"+positionSide+"&"+strUserId, math.Abs(float64(position.Size)))
							log.Println("初始化，仓位拉取gate：", tmpSymbol+"&"+positionSide+"&"+strUserId, position.Size, math.Abs(float64(position.Size)))
						}
					}

				}
			}

		}

		// 用户加入
		s.Users.Set(int(v.Id), v)

		// 绑定监听队列 将监听程序加入协程池
		err = service.OrderQueue().BindUserAndQueue(int(v.Id))
		if err != nil {
			log.Println("SetUser，绑定新增协程，错误:", v, err)
			continue
		}

		tmpId := int(v.Id)
		err = s.Pool.AddWithRecover(
			ctx,
			func(ctx context.Context) {
				service.OrderQueue().ListenQueue(ctx, tmpId, s.OrderAtPlat)
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

// HandleBothPositions 处理平仓
func (s *sListenAndOrder) HandleBothPositions(ctx context.Context) {
	if "BOTH" != s.TraderPositionSide.Val() {
		return
	}

	tmpPosition := s.Position.Get("ETHUSDTBOTH")
	if nil == tmpPosition {
		return
	}

	// 仓位有不处理
	if !floatEqual(tmpPosition.(*TraderPosition).PositionAmount, 0, 1e-7) {
		return
	}

	s.Users.Iterator(func(k int, v interface{}) bool {
		tmpUser := v.(*entity.User)
		strUserId := strconv.FormatUint(uint64(tmpUser.Id), 10)

		if !s.OrderMap.Contains("ETHUSDT&BOTH&" + strUserId) {
			return true
		}

		//// 当检测到余额不为0，不执行
		//tmp := s.OrderMap.Get("ETHUSDT&BOTH&" + strUserId).(float64)
		//if !floatEqual(tmpPosition.(*TraderPosition).PositionAmount, 0, 1e-7) {
		//	return true
		//}

		var (
			err error
		)
		if "binance" == tmpUser.Plat {
			var (
				binancePosition []*entity.BinancePosition
			)

			binancePosition = service.Binance().GetBinancePositionInfo(tmpUser.ApiKey, tmpUser.ApiSecret)
			if nil == binancePosition {
				log.Println("强平仓，错误查询仓位，binance")
				return true
			}

			for _, position := range binancePosition {
				//log.Println("初始化：", position.Symbol, position.PositionAmt, position.PositionSide)
				if "BOTH" != position.PositionSide {
					continue
				}

				if "ETHUSDT" != position.Symbol {
					continue
				}

				// 新增
				var (
					currentAmount float64
				)
				currentAmount, err = strconv.ParseFloat(position.PositionAmt, 64)
				if nil != err {
					log.Println("强平仓，解析金额出错，信息", position, currentAmount)
				}

				if floatEqual(currentAmount, 0, 1e-7) {
					continue
				}

				// 下单，不用计算数量，新仓位
				var (
					binanceOrderRes *entity.BinanceOrder
					orderInfoRes    *entity.BinanceOrderInfo
				)

				side := "SELL"
				if math.Signbit(currentAmount) {
					side = "BUY"
				}

				// 请求下单
				binanceOrderRes, orderInfoRes, err = service.Binance().RequestBinanceOrder(position.Symbol, side, "MARKET", position.PositionSide, position.PositionAmt, tmpUser.ApiKey, tmpUser.ApiSecret, true)
				if nil != err {
					log.Println("强平仓，下单错误:", tmpUser, binanceOrderRes, orderInfoRes, err, position)
					continue
				}

				// 下单异常
				if 0 >= binanceOrderRes.OrderId {
					log.Println("强平仓，下单错误:", tmpUser, binanceOrderRes, orderInfoRes, err, position)
					continue
				}

				s.OrderMap.Set(position.Symbol+"&"+position.PositionSide+"&"+strUserId, 0)
				log.Println("强平仓，仓位拉取binance：", position.Symbol+"&"+position.PositionSide+"&"+strUserId, currentAmount)
			}
		} else if "gate" == tmpUser.Plat {
			var (
				gatePositions []gateapi.Position
			)
			gatePositions, err = service.Gate().GetListPositions(tmpUser.ApiKey, tmpUser.ApiSecret)
			if nil != err {
				log.Println("强平仓，错误查询仓位，gate", err)
				return true
			}
			for _, position := range gatePositions {
				if "ETH_USDT" != position.Contract {
					continue
				}

				if "single" != position.Mode {
					continue
				}

				if floatEqual(float64(position.Size), 0, 1e-7) {
					continue
				}

				var (
					gateRes gateapi.FuturesOrder
				)

				gateRes, err = service.Gate().PlaceBothOrderGate(tmpUser.ApiKey, tmpUser.ApiSecret, "ETH_USDT", 0, true, true)
				if nil != err || 0 >= gateRes.Id {
					log.Println("强平仓，Gate下单:", tmpUser, gateRes, err, position)
					continue
				}

				s.OrderMap.Set("ETHUSDT&BOTH&"+strUserId, 0)
				log.Println("强平仓，仓位拉取gate：", "ETHUSDT&BOTH&"+strUserId, position.Size)

			}
		}

		return true
	})

}

// OrderAtPlat 在平台下单
func (s *sListenAndOrder) OrderAtPlat(ctx context.Context, doValue *entity.DoValue) {
	//log.Println("OrderAtPlat :", doValue)
	currentData := doValue.Value.(*entity.OrderInfo)

	tmpUser := s.Users.Get(doValue.UserId)
	if nil == tmpUser {
		log.Println("OrderAtPlat，不存在用户:", s.Users.Get(doValue.UserId).(*entity.User), currentData)
		return
	}

	user := tmpUser.(*entity.User)
	strUserId := strconv.FormatUint(uint64(doValue.UserId), 10)
	symbolMapKey := user.Plat + currentData.Symbol
	if !s.SymbolsMap.Contains(symbolMapKey) {
		log.Println("OrderAtPlat，不存在交易对:", user, currentData)
		return
	}

	traderMoney := s.TraderMoney.Val()
	if lessThanOrEqualZero(traderMoney, 1e-7) {
		log.Println("OrderAtPlat，交易员保证金错误:", user, currentData, traderMoney)
		return
	}

	userMoney := s.UsersMoney.Get(doValue.UserId).(float64)
	if lessThanOrEqualZero(userMoney, 1e-7) {
		log.Println("OrderAtPlat，用户保证金错误:", user, currentData, userMoney)
		return
	}

	//log.Println("测试", currentData, traderMoney, userMoney, s.OrderMap.Get(currentData.Symbol+"&"+currentData.PositionSide+"&"+strUserId))

	tmp := s.OrderMap.Get(currentData.Symbol + "&" + currentData.PositionSide + "&" + strUserId)
	var userPositionAmount float64
	if nil != tmp {
		userPositionAmount = tmp.(float64)
	}

	var (
		currentAmount      float64
		reduceOnly         bool
		closePosition      string
		quantityInt64Gate  int64
		quantityFloatGate  float64
		tmpExecutedQtyGate float64 // 结果有正负both 其他持仓仓位模式正
		closeGate          bool
		reduceOnlyBinance  bool
	)
	if "BOTH" == currentData.PositionSide {
		if "BOTH" != s.UsersPositionSide.Get(doValue.UserId) { // 持仓不符合
			log.Println("OrderAtPlat，持仓用户:", user, currentData, s.UsersPositionSide.Get(doValue.UserId))
			return
		}

		// 完全平常
		if "CLOSE" == currentData.Status {
			currentAmount = math.Abs(userPositionAmount) // 本次开单数量，转换为正数

			// 认为是0
			if lessThanOrEqualZero(currentAmount, 1e-7) {
				return
			}

			reduceOnly = true
			closeGate = true

			tmpExecutedQtyGate = -userPositionAmount // 反向 正负保持

			quantityInt64Gate = 0
			quantityFloatGate = 0

			reduceOnlyBinance = true
		} else {
			currentAmount = math.Abs(currentData.Oq) * userMoney / traderMoney // 本次开单数量，转换为正数

			// 部分平仓
			if math.Signbit(currentData.Amount) && math.Signbit(currentData.LastAmount) && !math.Signbit(currentData.Oq) {

			} else if !math.Signbit(currentData.Amount) && !math.Signbit(currentData.LastAmount) && math.Signbit(currentData.Oq) {

			} else {
				// 穿仓，开新仓，检测能否开仓
				if 2 != s.Users.Get(doValue.UserId).(*entity.User).OpenStatus {
					log.Println("OrderAtPlat，暂停用户:", user, currentData)
					// 暂停开新仓
					return
				}
			}

			// 如果用户此时无仓位，正常应该是和交易员同步的
			if floatEqual(userPositionAmount, 0, 1e-7) {
				// 交易员反向交易仍未穿仓，部分平仓，则选择不开
				if math.Signbit(currentData.Amount) && math.Signbit(currentData.LastAmount) && !math.Signbit(currentData.Oq) {
					return
				} else if !math.Signbit(currentData.Amount) && !math.Signbit(currentData.LastAmount) && math.Signbit(currentData.Oq) {
					return
				}
			}

			if 0 < s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier {
				//log.Println("OrderAtPlat，交易对信息错误:", user, currentData, s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol))

				// 转化为张数=币的数量/每张币的数量
				tmpQtyGate := currentAmount / s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier
				// 按张的精度转化，
				quantityInt64Gate = int64(math.Round(tmpQtyGate))
				quantityFloatGate = float64(quantityInt64Gate)
			}

			if "SELL" == currentData.Side {
				quantityInt64Gate = -quantityInt64Gate
				quantityFloatGate = -quantityFloatGate
			}

			tmpExecutedQtyGate = quantityFloatGate
		}

	} else if "LONG" == currentData.PositionSide {
		if "ALL" != s.UsersPositionSide.Get(doValue.UserId) { // 持仓不符合
			log.Println("OrderAtPlat，持仓用户:", user, currentData, s.UsersPositionSide.Get(doValue.UserId))
			return
		}

		if "CLOSE" == currentData.Status { // 完全平仓
			// 认为是0
			if lessThanOrEqualZero(userPositionAmount, 1e-7) {
				return
			}

			currentAmount = userPositionAmount

			reduceOnly = true
			closePosition = "close_long"

			quantityInt64Gate = 0
			quantityFloatGate = 0

			tmpExecutedQtyGate = currentAmount
		} else {
			if "SELL" == currentData.Side {
				// 平多
				// 认为是0
				if lessThanOrEqualZero(userPositionAmount, 1e-7) {
					return
				}

				// 平仓数据验证
				if lessThanOrEqualZero(currentData.LastAmount, 1e-7) {
					return
				}

				currentAmount = userPositionAmount * (currentData.Oq) / currentData.LastAmount

				reduceOnly = true
				closePosition = "close_long"

				quantityInt64Gate = int64(currentAmount)
				quantityFloatGate = float64(quantityInt64Gate)

				tmpExecutedQtyGate = quantityFloatGate

				quantityInt64Gate = -quantityInt64Gate
				quantityFloatGate = -quantityFloatGate

			} else if "BUY" == currentData.Side {
				// 开新仓，检测能否开仓
				if 2 != s.Users.Get(doValue.UserId).(*entity.User).OpenStatus {
					log.Println("OrderAtPlat，暂停用户:", user, currentData)
					// 暂停开新仓
					return
				}

				// 开多
				currentAmount = currentData.Oq * userMoney / traderMoney // 本次开单数量

				if 0 < s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier {
					//log.Println("OrderAtPlat，交易对信息错误:", user, currentData, s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol))

					// 转化为张数=币的数量/每张币的数量
					tmpQtyGate := currentAmount / s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier
					// 按张的精度转化，
					quantityInt64Gate = int64(math.Round(tmpQtyGate))
					quantityFloatGate = float64(quantityInt64Gate)

					tmpExecutedQtyGate = quantityFloatGate // 正数
				}

			} else {
				return
			}
		}

	} else if "SHORT" == currentData.PositionSide {
		if "ALL" != s.UsersPositionSide.Get(doValue.UserId) { // 持仓不符合
			log.Println("OrderAtPlat，持仓用户:", user, currentData, s.UsersPositionSide.Get(doValue.UserId))
			return
		}

		if "CLOSE" == currentData.Status { // 完全平仓
			// 认为是0
			if lessThanOrEqualZero(userPositionAmount, 1e-7) {
				return
			}

			currentAmount = userPositionAmount

			reduceOnly = true
			closePosition = "close_short"

			quantityInt64Gate = 0
			quantityFloatGate = 0

			tmpExecutedQtyGate = currentAmount
		} else {
			if "BUY" == currentData.Side {
				// 平空
				// 认为是0
				if lessThanOrEqualZero(userPositionAmount, 1e-7) {
					return
				}

				// 平仓数据验证
				if lessThanOrEqualZero(currentData.LastAmount, 1e-7) {
					return
				}

				currentAmount = userPositionAmount * (currentData.Oq) / currentData.LastAmount

				reduceOnly = true
				closePosition = "close_short"

				quantityInt64Gate = int64(currentAmount)
				quantityFloatGate = float64(quantityInt64Gate)

				tmpExecutedQtyGate = quantityFloatGate

			} else if "SELL" == currentData.Side {
				// 开新仓，检测能否开仓
				if 2 != s.Users.Get(doValue.UserId).(*entity.User).OpenStatus {
					log.Println("OrderAtPlat，暂停用户:", user, currentData)
					// 暂停开新仓
					return
				}

				// 开空
				currentAmount = currentData.Oq * userMoney / traderMoney // 本次开单数量

				if 0 < s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier {
					//log.Println("OrderAtPlat，交易对信息错误:", user, currentData, s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol))

					// 转化为张数=币的数量/每张币的数量
					tmpQtyGate := currentAmount / s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier
					// 按张的精度转化，
					quantityInt64Gate = int64(math.Round(tmpQtyGate))
					quantityFloatGate = float64(quantityInt64Gate)

					tmpExecutedQtyGate = quantityFloatGate // 正数

					quantityInt64Gate = -quantityInt64Gate
					quantityFloatGate = -quantityFloatGate
				}

			} else {
				return
			}
		}

	} else {
		return
	}

	if "gate" == user.Plat {
		var (
			err          error
			gateRes      gateapi.FuturesOrder
			side         = currentData.Side
			symbolGate   = s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).Symbol + "_USDT"
			positionSide = currentData.PositionSide
		)

		if "BOTH" == currentData.PositionSide {
			gateRes, err = service.Gate().PlaceBothOrderGate(user.ApiKey, user.ApiSecret, symbolGate, quantityInt64Gate, reduceOnly, closeGate)
			if nil != err || 0 >= gateRes.Id {
				log.Println("OrderAtPlat，Gate下单:", user, currentData, gateRes, reduceOnly, closePosition, quantityInt64Gate, symbolGate)
				return
			}

			// 不存在新增，这里只能是开仓
			if !s.OrderMap.Contains(currentData.Symbol + "&" + positionSide + "&" + strUserId) {
				// 追加仓位，开仓
				s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQtyGate)
			} else {
				if "CLOSE" == currentData.Status {
					tmpExecutedQtyGate = 0
				} else {
					tmpExecutedQtyGate = userPositionAmount + tmpExecutedQtyGate
				}

				s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQtyGate)
			}

		} else {
			gateRes, err = service.Gate().PlaceOrderGate(user.ApiKey, user.ApiSecret, symbolGate, quantityInt64Gate, reduceOnly, closePosition)
			if nil != err || 0 >= gateRes.Id {
				log.Println("OrderAtPlat，Gate下单:", user, currentData, gateRes, reduceOnly, closePosition, quantityInt64Gate, symbolGate)
				return
			}

			// 不存在新增，这里只能是开仓
			if !s.OrderMap.Contains(currentData.Symbol + "&" + positionSide + "&" + strUserId) {
				// 追加仓位，开仓
				s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQtyGate)
			} else {
				// 追加仓位，开仓
				if "LONG" == positionSide {
					if "BUY" == side {
						tmpExecutedQtyGate += s.OrderMap.Get(currentData.Symbol + "&" + positionSide + "&" + strUserId).(float64)
						s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQtyGate)
					} else if "SELL" == side {
						tmpExecutedQtyGate = s.OrderMap.Get(currentData.Symbol+"&"+positionSide+"&"+strUserId).(float64) - tmpExecutedQtyGate
						if lessThanOrEqualZero(tmpExecutedQtyGate, 1e-7) {
							tmpExecutedQtyGate = 0
						}
						s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQtyGate)
					} else {
						log.Println("OrderAtPlat，Gate下单，数据存储:", user, currentData, gateRes, reduceOnly, closePosition, quantityInt64Gate, symbolGate, tmpExecutedQtyGate)

					}

				} else if "SHORT" == positionSide {
					if "SELL" == side {
						tmpExecutedQtyGate += s.OrderMap.Get(currentData.Symbol + "&" + positionSide + "&" + strUserId).(float64)
						s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQtyGate)
					} else if "BUY" == side {
						tmpExecutedQtyGate = s.OrderMap.Get(currentData.Symbol+"&"+positionSide+"&"+strUserId).(float64) - tmpExecutedQtyGate
						if lessThanOrEqualZero(tmpExecutedQtyGate, 1e-7) {
							tmpExecutedQtyGate = 0
						}
						s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQtyGate)
					} else {
						log.Println("OrderAtPlat，Gate下单，数据存储:", user, currentData, gateRes, reduceOnly, closePosition, quantityInt64Gate, symbolGate, tmpExecutedQtyGate)
					}

				} else {
					log.Println("OrderAtPlat，Gate下单，数据存储:", user, currentData, gateRes, reduceOnly, closePosition, quantityInt64Gate, symbolGate, tmpExecutedQtyGate)
				}
			}

		}

	} else if "binance" == user.Plat {
		// 精度调整
		var (
			quantity      string
			quantityFloat float64
			err           error
			side          = currentData.Side
			orderType     = "MARKET"
			positionSide  = currentData.PositionSide
		)
		if 0 >= s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantityPrecision {
			quantity = fmt.Sprintf("%d", int64(currentAmount))
		} else {
			quantity = strconv.FormatFloat(currentAmount, 'f', s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantityPrecision, 64)
		}

		quantityFloat, err = strconv.ParseFloat(quantity, 64)
		if nil != err {
			log.Println("OrderAtPlat，数量解析:", user, currentData, s.UsersPositionSide.Get(doValue.UserId), quantity)
			return
		}

		if lessThanOrEqualZero(quantityFloat, 1e-7) {
			return
		}

		// 下单，不用计算数量，新仓位
		var (
			binanceOrderRes *entity.BinanceOrder
			orderInfoRes    *entity.BinanceOrderInfo
		)

		// 请求下单
		binanceOrderRes, orderInfoRes, err = service.Binance().RequestBinanceOrder(currentData.Symbol, side, orderType, positionSide, quantity, user.ApiKey, user.ApiSecret, reduceOnlyBinance)
		if nil != err {
			log.Println("OrderAtPlat，下单错误:", user, currentData, binanceOrderRes, orderInfoRes, err, quantity)
			return
		}

		//binanceOrderRes = &entity.BinanceOrder{
		//	OrderId:       1,
		//	ExecutedQty:   quantity,
		//	ClientOrderId: "",
		//	Symbol:        "",
		//	AvgPrice:      "",
		//	CumQuote:      "",
		//	Side:          side,
		//	PositionSide:  positionSide,
		//	ClosePosition: false,
		//	Type:          orderType,
		//	Status:        "",
		//}

		// 下单异常
		if 0 >= binanceOrderRes.OrderId {
			log.Println(orderInfoRes)
			return // 返回
		}

		var tmpExecutedQty float64 // 结果有正负both 其他持仓仓位模式正
		tmpExecutedQty = quantityFloat
		if "BOTH" == positionSide && "SELL" == side {
			tmpExecutedQty = -tmpExecutedQty
		}

		// 不存在新增，这里只能是开仓
		if !s.OrderMap.Contains(currentData.Symbol + "&" + positionSide + "&" + strUserId) {
			s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
		} else {
			// 追加仓位，开仓
			if "LONG" == positionSide {
				if "BUY" == side {
					tmpExecutedQty += s.OrderMap.Get(currentData.Symbol + "&" + positionSide + "&" + strUserId).(float64)
					s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
				} else if "SELL" == side {
					tmpExecutedQty = s.OrderMap.Get(currentData.Symbol+"&"+positionSide+"&"+strUserId).(float64) - tmpExecutedQty
					if lessThanOrEqualZero(tmpExecutedQty, 1e-7) {
						tmpExecutedQty = 0
					}
					s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
				} else {
					log.Println("OrderAtPlat，binance下单，数据存储:", user, currentData, binanceOrderRes, orderInfoRes, tmpExecutedQty)
				}

			} else if "SHORT" == positionSide {
				if "SELL" == side {
					tmpExecutedQty += s.OrderMap.Get(currentData.Symbol + "&" + positionSide + "&" + strUserId).(float64)
					s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
				} else if "BUY" == side {
					tmpExecutedQty = s.OrderMap.Get(currentData.Symbol+"&"+positionSide+"&"+strUserId).(float64) - tmpExecutedQty
					if lessThanOrEqualZero(tmpExecutedQty, 1e-7) {
						tmpExecutedQty = 0
					}
					s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
				} else {
					log.Println("OrderAtPlat，binance下单，数据存储:", user, currentData, binanceOrderRes, orderInfoRes, tmpExecutedQty)
				}

			} else if "BOTH" == positionSide {
				if "CLOSE" == currentData.Status {
					tmpExecutedQty = 0
				} else {
					tmpExecutedQty = userPositionAmount + tmpExecutedQty
				}

				s.OrderMap.Set(currentData.Symbol+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
			} else {
				log.Println("OrderAtPlat，binance下单，数据存储:", user, currentData, binanceOrderRes, orderInfoRes, tmpExecutedQty)
			}
		}

	} else {
		log.Println("OrderAtPlat，用户信息错误:", user, currentData)
		return
	}

	log.Println("仓位信息：", currentData.Symbol+"&"+currentData.PositionSide+"&"+strUserId, s.OrderMap.Get(currentData.Symbol+"&"+currentData.PositionSide+"&"+strUserId))
	return
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
		//log.Println("初始化：", position.Symbol, position.PositionAmt, position.PositionSide)

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
		if !s.Position.Contains(vPosition.Symbol + "BOTH") {
			s.Position.Set(vPosition.Symbol+"BOTH", &TraderPosition{
				Symbol:         vPosition.Symbol,
				PositionSide:   "BOTH",
				PositionAmount: 0,
			})
		}

		if !s.Position.Contains(vPosition.Symbol + "LONG") {
			s.Position.Set(vPosition.Symbol+"LONG", &TraderPosition{
				Symbol:         vPosition.Symbol,
				PositionSide:   "LONG",
				PositionAmount: 0,
			})
		}

		if !s.Position.Contains(vPosition.Symbol + "SHORT") {
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
		for retry := 0; retry < 30; retry++ {
			err = service.Binance().CreateListenKey(s.TraderInfo.apiKey)
			if err != nil {
				log.Println("Error creating listen key:", err)
				continue
			}

			// Connect WebSocket initially
			err = service.Binance().ConnectWebSocket()
			if err != nil {
				log.Println("Error connecting WebSocket:", err)
				continue
			}

			break
		}

		return
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

		//log.Println(3, event, "\n\n\n")

		if "MARKET" == event.Order.OriginalOrderType {
			// 市价
			if !("NEW" == event.Order.ExecutionType && "NEW" == event.Order.OrderStatus) {
				continue
			}

			log.Println("市价，new：", event)
			// 客户端下单

		} else if "LIMIT" == event.Order.OriginalOrderType {
			// 限价 开始交易，我们的反应是全部执行市价，开或关
			if "TRADE" != event.Order.ExecutionType {
				continue
			}

			if "PARTIALLY_FILLED" != event.Order.OrderStatus && "FILLED" != event.Order.OrderStatus {
				continue
			}

			log.Println("限价，trade：", event)

			// 只要第一单，情况1一次成交完，情况2部分成交第一单，这两种情况的值相等
			if event.Order.LastExecutedQty != event.Order.CumulativeExecutedQty {
				continue
			}

		} else {
			continue
		}

		// 源方持仓向要和下单方向一致，BOTH和LONG，SHORT
		if "BOTH" == event.Order.PositionSide && "BOTH" != s.TraderPositionSide.Val() {
			log.Println("持仓方向不一致，trade：", event, s.TraderPositionSide.Val())
			continue
		} else if "BOTH" != event.Order.PositionSide && "BOTH" == s.TraderPositionSide.Val() {
			log.Println("持仓方向不一致，trade：", event, s.TraderPositionSide.Val())
			continue
		}

		var (
			oQ           float64 // 本次的数量
			side         string
			PositionSide string
			status       = "open"
		)
		oQ, err = strconv.ParseFloat(event.Order.OriginalQty, 64)
		if nil != err {
			log.Println("解析金额出错，信息", event)
			continue
		}
		if lessThanOrEqualZero(oQ, 1e-7) {
			log.Println("解析金额，下单数字太小，信息", event)
			continue
		}

		if "SELL" == event.Order.OrderSide {
			if "BOTH" == event.Order.PositionSide {
				oQ = -oQ
				PositionSide = "BOTH"
			} else if "LONG" == event.Order.PositionSide {
				PositionSide = "LONG"
			} else if "SHORT" == event.Order.PositionSide {
				PositionSide = "SHORT"
			} else {
				log.Println("解析持仓方向出错，信息", event)
				continue
			}

			side = "SELL"
		} else if "BUY" == event.Order.OrderSide {
			if "BOTH" == event.Order.PositionSide {
				PositionSide = "BOTH"
			} else if "LONG" == event.Order.PositionSide {
				PositionSide = "LONG"
			} else if "SHORT" == event.Order.PositionSide {
				PositionSide = "SHORT"
			} else {
				log.Println("解析持仓方向出错，信息", event)
				continue
			}

			side = "BUY"
		} else {
			log.Println("不识别的买卖，信息", event)
			continue
		}

		tmpPosition := s.Position.Get(event.Order.Symbol + event.Order.PositionSide)
		newPosition := &TraderPosition{
			Symbol:         event.Order.Symbol,
			PositionSide:   PositionSide,
			PositionAmount: oQ,
		}
		if nil != tmpPosition {

			if "BOTH" == PositionSide {
				// 这里暂时不做处理，确保当前仓位符合实际和binance对的上 todo
				newPosition.PositionAmount = tmpPosition.(*TraderPosition).PositionAmount + oQ
				if floatEqual(newPosition.PositionAmount, 0, 1e-7) {
					status = "CLOSE" // 完全平仓
					newPosition.PositionAmount = 0
				}

			} else if "LONG" == PositionSide {
				if "SELL" == side {
					// 保障关仓要有仓位
					if lessThanOrEqualZero(tmpPosition.(*TraderPosition).PositionAmount, 1e-7) {
						log.Println("交易员无此无仓位，信息", event)
						continue
					}

					newPosition.PositionAmount = tmpPosition.(*TraderPosition).PositionAmount - oQ
					if lessThanOrEqualZero(newPosition.PositionAmount, 1e-7) {
						status = "CLOSE" // 完全平仓
						newPosition.PositionAmount = 0
					}
				} else if "BUY" == side {
					newPosition.PositionAmount = tmpPosition.(*TraderPosition).PositionAmount + oQ
					if lessThanOrEqualZero(newPosition.PositionAmount, 1e-7) {
						status = "CLOSE" // 完全平仓
						newPosition.PositionAmount = 0
					}
				}

			} else if "SHORT" == PositionSide {
				if "SELL" == side {
					newPosition.PositionAmount = tmpPosition.(*TraderPosition).PositionAmount + oQ
					if lessThanOrEqualZero(newPosition.PositionAmount, 1e-7) {
						status = "CLOSE" // 完全平仓
						newPosition.PositionAmount = 0
					}
				} else if "BUY" == side {
					// 保障关仓要有仓位
					if lessThanOrEqualZero(tmpPosition.(*TraderPosition).PositionAmount, 1e-7) {
						log.Println("交易员无此无仓位，信息", event)
						continue
					}

					newPosition.PositionAmount = tmpPosition.(*TraderPosition).PositionAmount - oQ
					if lessThanOrEqualZero(newPosition.PositionAmount, 1e-7) {
						status = "CLOSE" // 完全平仓
						newPosition.PositionAmount = 0
					}
				}

			} else {
				log.Println("不识别的仓位方向2，信息", event)
				continue
			}
		}

		// 新仓位
		s.Position.Set(event.Order.Symbol+event.Order.PositionSide, newPosition)

		tmpMsg := &entity.OrderInfo{
			Symbol:       newPosition.Symbol,
			Amount:       newPosition.PositionAmount,
			LastAmount:   tmpPosition.(*TraderPosition).PositionAmount,
			Oq:           oQ,
			Status:       status,
			Side:         side,
			PositionSide: PositionSide,
		}

		//log.Println("本次下单信息:", side, PositionSide, oQ, "\n\n")
		log.Println("新仓位信息:", tmpMsg)

		service.OrderQueue().PushAllQueue(tmpMsg)

		continue
	}

}

// GetSystemUserNum get user num
func (s *sListenAndOrder) GetSystemUserNum(ctx context.Context) map[string]float64 {
	var (
		err   error
		users []*entity.User
		res   map[string]float64
	)
	res = make(map[string]float64, 0)

	err = g.Model("user").Ctx(ctx).Scan(&users)
	if nil != err {
		log.Println("获取用户num，数据库查询错误：", err)
		return res
	}

	for _, v := range users {
		res[v.ApiKey] = v.Num
	}

	return res
}

// SetSystemUserNum set user num
func (s *sListenAndOrder) SetSystemUserNum(ctx context.Context, apiKey string, num float64) error {
	var (
		err error
	)
	_, err = g.Model("user").Ctx(ctx).Data("num", num).Where("api_key=?", apiKey).Update()
	if nil != err {
		log.Println("更新用户num：", err)
		return err
	}

	return nil
}

// SetUseNewSystem set user num
func (s *sListenAndOrder) SetUseNewSystem(ctx context.Context, apiKey string, useNewSystem uint64) error {
	var (
		err error
	)
	_, err = g.Model("user").Ctx(ctx).Data("open_status", useNewSystem).Where("api_key=?", apiKey).Update()
	if nil != err {
		log.Println("更新用户num：", err)
		return err
	}

	return nil
}

// GetSystemUserPositions get user positions
func (s *sListenAndOrder) GetSystemUserPositions(ctx context.Context, apiKey string) map[string]float64 {
	var (
		err   error
		users []*entity.User
		res   map[string]float64
	)
	res = make(map[string]float64, 0)

	err = g.Model("user").Where("api_key=?", apiKey).Ctx(ctx).Scan(&users)
	if nil != err {
		log.Println("查看用户仓位，数据库查询错误：", err)
		return res
	}

	if 0 >= len(users) || 0 >= users[0].Id {
		return res
	}

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
			log.Println("查看用户仓位，解析id错误:", k)
		}

		if uid != uint64(users[0].Id) {
			return true
		}

		part1 := parts[1]
		amount := v.(float64)
		if "BOTH" == part1 {
			if math.Signbit(amount) {
				part1 = "SHORT"
			} else {
				part1 = "LONG"
			}
		}

		res[parts[0]+"&"+part1] = math.Abs(amount)
		return true
	})

	return res
}

// SetSystemUserPosition set user positions
func (s *sListenAndOrder) SetSystemUserPosition(ctx context.Context, system uint64, allCloseGate uint64, apiKey string, symbol string, side string, positionSide string, num float64) uint64 {
	var (
		err   error
		users []*entity.User
	)

	err = g.Model("user").Where("api_key=?", apiKey).Ctx(ctx).Scan(&users)
	if nil != err {
		log.Println("修改仓位，数据库查询错误：", err)
		return 0
	}

	if 0 >= len(users) || 0 >= users[0].Id {
		log.Println("修改仓位，数据库查询错误：", err)
		return 0
	}

	vTmpUserMap := users[0]
	strUserId := strconv.FormatUint(uint64(vTmpUserMap.Id), 10)
	symbolMapKey := vTmpUserMap.Plat + symbol + "USDT"

	if "binance" == vTmpUserMap.Plat {
		var (
			symbolRel     = symbol + "USDT"
			tmpQty        float64
			quantity      string
			quantityFloat float64
			orderType     = "MARKET"
		)
		if "LONG" == positionSide {
			if "ALL" != s.UsersPositionSide.Get(int(vTmpUserMap.Id)) {
				return 0
			}

			positionSide = "LONG"
			if "BUY" == side {
				side = "BUY"
			} else if "SELL" == side {
				side = "SELL"
			} else {
				log.Println("自定义下单，无效信息，信息", apiKey, symbol, side, positionSide, num)
				return 0
			}
		} else if "SHORT" == positionSide {
			if "ALL" != s.UsersPositionSide.Get(int(vTmpUserMap.Id)) {
				return 0
			}

			positionSide = "SHORT"
			if "BUY" == side {
				side = "BUY"
			} else if "SELL" == side {
				side = "SELL"
			} else {
				log.Println("自定义下单，无效信息，信息", apiKey, symbol, side, positionSide, num)
				return 0
			}
		} else if "BOTH" == positionSide {
			if "BOTH" != s.UsersPositionSide.Get(int(vTmpUserMap.Id)) {
				return 0
			}

			positionSide = "BOTH"
			if "BUY" == side {
				side = "BUY"
			} else if "SELL" == side {
				side = "SELL"
			} else {
				log.Println("自定义下单，无效信息，信息", apiKey, symbol, side, positionSide, num)
				return 0
			}
		} else {
			log.Println("自定义下单，无效信息，信息", apiKey, symbol, side, positionSide, num)
			return 0
		}

		tmpQty = num // 本次开单数量
		if !s.SymbolsMap.Contains(symbolMapKey) {
			log.Println("自定义下单，代币信息无效，信息", apiKey, symbol, side, positionSide, num)
			return 0
		}

		// 精度调整
		if 0 >= s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantityPrecision {
			quantity = fmt.Sprintf("%d", int64(tmpQty))
		} else {
			quantity = strconv.FormatFloat(tmpQty, 'f', s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantityPrecision, 64)
		}

		quantityFloat, err = strconv.ParseFloat(quantity, 64)
		if nil != err {
			log.Println(err)
			return 0
		}

		if lessThanOrEqualZero(quantityFloat, 1e-7) {
			return 0
		}

		// 下单，不用计算数量，新仓位
		var (
			binanceOrderRes *entity.BinanceOrder
			orderInfoRes    *entity.BinanceOrderInfo
		)

		// 请求下单
		binanceOrderRes, orderInfoRes, err = service.Binance().RequestBinanceOrder(symbolRel, side, orderType, positionSide, quantity, vTmpUserMap.ApiKey, vTmpUserMap.ApiSecret, false)
		if nil != err {
			log.Println(err)
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
			log.Println("自定义下单，binance下单错误：", orderInfoRes)
			return 0
		}

		var tmpExecutedQty float64
		tmpExecutedQty = quantityFloat
		if "BOTH" == positionSide && "SELL" == side {
			tmpExecutedQty = -tmpExecutedQty
		}

		if 1 == system {
			// 不存在新增，这里只能是开仓
			if !s.OrderMap.Contains(symbolRel + "&" + positionSide + "&" + strUserId) {
				s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
			} else {
				// 追加仓位，开仓
				if "LONG" == positionSide {
					if "BUY" == side {
						tmpExecutedQty += s.OrderMap.Get(symbolRel + "&" + positionSide + "&" + strUserId).(float64)
						s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
					} else if "SELL" == side {
						tmpExecutedQty = s.OrderMap.Get(symbolRel+"&"+positionSide+"&"+strUserId).(float64) - tmpExecutedQty
						if lessThanOrEqualZero(tmpExecutedQty, 1e-7) {
							tmpExecutedQty = 0
						}
						s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
					} else {
						log.Println("手动，binance下单，数据存储:", system, allCloseGate, apiKey, symbol, side, positionSide, num, binanceOrderRes, orderInfoRes, tmpExecutedQty)
					}

				} else if "SHORT" == positionSide {
					if "SELL" == side {
						tmpExecutedQty += s.OrderMap.Get(symbolRel + "&" + positionSide + "&" + strUserId).(float64)
						s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
					} else if "BUY" == side {
						tmpExecutedQty = s.OrderMap.Get(symbolRel+"&"+positionSide+"&"+strUserId).(float64) - tmpExecutedQty
						if lessThanOrEqualZero(tmpExecutedQty, 1e-7) {
							tmpExecutedQty = 0
						}
						s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
					} else {
						log.Println("手动，binance下单，数据存储:", system, allCloseGate, apiKey, symbol, side, positionSide, num, binanceOrderRes, orderInfoRes, tmpExecutedQty)
					}

				} else if "BOTH" == positionSide {
					tmpExecutedQty = s.OrderMap.Get(symbolRel+"&"+positionSide+"&"+strUserId).(float64) + tmpExecutedQty
					if floatEqual(tmpExecutedQty, 0, 1e-7) {
						tmpExecutedQty = 0
					}
					s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
				} else {
					log.Println("手动，binance下单，数据存储:", system, allCloseGate, apiKey, symbol, side, positionSide, num, binanceOrderRes, orderInfoRes, tmpExecutedQty)
				}
			}
		}
	} else if "gate" == vTmpUserMap.Plat {
		if 0 >= s.OrderMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier {
			log.Println("自定义下单，代币信息错误，信息", s.OrderMap.Get(symbolMapKey).(*entity.LhCoinSymbol), apiKey, symbol, side, positionSide, num)
			return 0
		}

		var (
			tmpQty         float64
			gateRes        gateapi.FuturesOrder
			symbolRel      = symbol + "_USDT"
			quantity       string
			quantityInt64  int64
			quantityFloat  float64
			tmpExecutedQty float64
			reduceOnly     bool
			closePosition  string
			closeStatus    bool
		)

		if "LONG" == positionSide {
			positionSide = "LONG"
			if "BUY" == side {
				side = "BUY"
				tmpQty = num

				// 转化为张数=币的数量/每张币的数量
				tmpQtyOkx := tmpQty / s.OrderMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier
				// 按张的精度转化，
				quantityInt64 = int64(math.Round(tmpQtyOkx))
				quantityFloat = float64(quantityInt64)
				tmpExecutedQty = quantityFloat // 正数

				if lessThanOrEqualZero(quantityFloat, 1e-7) {
					log.Println("自定义下单，下单错误，信息", apiKey, symbol, side, positionSide, num)
					return 0
				}
				quantity = strconv.FormatFloat(quantityFloat, 'f', -1, 64)

			} else if "SELL" == side {
				side = "SELL"
				// 平仓
				reduceOnly = true

				if 1 == allCloseGate {
					tmpQty = 0
					quantityInt64 = 0
					quantityFloat = 0
					closePosition = "close_long"

					// 剩余仓位
					tmpExecutedQty = s.OrderMap.Get(symbol + "USDT" + "&" + positionSide + "&" + strUserId).(float64)

				} else {
					tmpQty = num
					// 转化为张数=币的数量/每张币的数量
					tmpQtyOkx := tmpQty / s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier
					// 按张的精度转化，
					quantityInt64 = int64(math.Round(tmpQtyOkx))
					quantityFloat = float64(quantityInt64)
					tmpExecutedQty = quantityFloat // 正数

					if lessThanOrEqualZero(quantityFloat, 1e-7) {
						log.Println("自定义下单，下单错误，信息", apiKey, symbol, side, positionSide, num)
						return 0
					}

					// 部分平仓多
					quantityFloat = -quantityFloat
					quantityInt64 = -quantityInt64
				}

			} else {
				log.Println("自定义下单，无效信息，信息", apiKey, symbol, side, positionSide, num)
				return 0
			}
		} else if "SHORT" == positionSide {
			positionSide = "SHORT"
			if "BUY" == side {
				side = "BUY"
				// 平仓
				reduceOnly = true
				if 1 == allCloseGate {
					tmpQty = 0
					quantityInt64 = 0
					quantityFloat = 0
					closePosition = "close_short"

					// 剩余仓位
					tmpExecutedQty = s.OrderMap.Get(symbol + "USDT" + "&" + positionSide + "&" + strUserId).(float64)

				} else {
					tmpQty = num

					// 转化为张数=币的数量/每张币的数量
					tmpQtyOkx := tmpQty / s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier
					// 按张的精度转化，
					quantityInt64 = int64(math.Round(tmpQtyOkx))
					quantityFloat = float64(quantityInt64)
					tmpExecutedQty = quantityFloat // 正数

					if lessThanOrEqualZero(quantityFloat, 1e-7) {
						log.Println("自定义下单，下单错误，信息", apiKey, symbol, side, positionSide, num)
						return 0
					}

				}

			} else if "SELL" == side {
				side = "SELL"
				tmpQty = num

				// 转化为张数=币的数量/每张币的数量
				tmpQtyOkx := tmpQty / s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier
				// 按张的精度转化，
				quantityInt64 = int64(math.Round(tmpQtyOkx))
				quantityFloat = float64(quantityInt64)
				tmpExecutedQty = quantityFloat // 正数

				if lessThanOrEqualZero(quantityFloat, 1e-7) {
					log.Println("自定义下单，下单错误，信息", apiKey, symbol, side, positionSide, num)
					return 0
				}

				quantityFloat = -quantityFloat
				quantityInt64 = -quantityInt64

				quantity = strconv.FormatFloat(quantityFloat, 'f', -1, 64)
			} else {
				log.Println("自定义下单，无效信息，信息", apiKey, symbol, side, positionSide, num)
				return 0
			}
		} else if "BOTH" == positionSide {

			positionSide = "BOTH"
			if "BUY" == side {
				side = "BUY"
				// 平仓
				if 1 == allCloseGate {
					reduceOnly = true
					tmpQty = 0
					quantityInt64 = 0
					quantityFloat = 0
					closeStatus = true

					// 剩余仓位
					tmpExecutedQty = math.Abs(s.OrderMap.Get(symbol + "USDT" + "&" + positionSide + "&" + strUserId).(float64))

				} else {
					tmpQty = num

					// 转化为张数=币的数量/每张币的数量
					tmpQtyOkx := tmpQty / s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier
					// 按张的精度转化，
					quantityInt64 = int64(math.Round(tmpQtyOkx))
					quantityFloat = float64(quantityInt64)
					tmpExecutedQty = quantityFloat // 正数

					if lessThanOrEqualZero(quantityFloat, 1e-7) {
						log.Println("自定义下单，下单错误，信息", apiKey, symbol, side, positionSide, num)
						return 0
					}

				}

			} else if "SELL" == side {
				side = "SELL"
				if 1 == allCloseGate {
					reduceOnly = true
					tmpQty = 0
					quantityInt64 = 0
					quantityFloat = 0
					closeStatus = true

					// 剩余仓位
					tmpExecutedQty = math.Abs(s.OrderMap.Get(symbol + "USDT" + "&" + positionSide + "&" + strUserId).(float64))

				} else {
					tmpQty = num

					// 转化为张数=币的数量/每张币的数量
					tmpQtyOkx := tmpQty / s.SymbolsMap.Get(symbolMapKey).(*entity.LhCoinSymbol).QuantoMultiplier
					// 按张的精度转化，
					quantityInt64 = int64(math.Round(tmpQtyOkx))
					quantityFloat = float64(quantityInt64)
					tmpExecutedQty = quantityFloat // 正数

					if lessThanOrEqualZero(quantityFloat, 1e-7) {
						log.Println("自定义下单，下单错误，信息", apiKey, symbol, side, positionSide, num)
						return 0
					}

					quantityFloat = -quantityFloat
					quantityInt64 = -quantityInt64

					quantity = strconv.FormatFloat(quantityFloat, 'f', -1, 64)
				}
			} else {
				log.Println("自定义下单，无效信息，信息", apiKey, symbol, side, positionSide, num)
				return 0
			}
		} else {
			log.Println("自定义下单，无效信息，信息", apiKey, symbol, side, positionSide, num)
			return 0
		}

		if "BOTH" == positionSide {
			gateRes, err = service.Gate().PlaceBothOrderGate(vTmpUserMap.ApiKey, vTmpUserMap.ApiSecret, symbolRel, quantityInt64, reduceOnly, closeStatus)
			if nil != err || 0 >= gateRes.Id {
				log.Println("自定义下单，gate，Gate下单:", err, symbol, side, positionSide, quantityInt64, quantity, gateRes)
				return 0
			}
		} else {
			gateRes, err = service.Gate().PlaceOrderGate(vTmpUserMap.ApiKey, vTmpUserMap.ApiSecret, symbolRel, quantityInt64, reduceOnly, closePosition)
			if nil != err || 0 >= gateRes.Id {
				log.Println("自定义下单，gate，Gate下单:", err, symbol, side, positionSide, quantityInt64, quantity, gateRes)
				return 0
			}
		}

		if 0 >= gateRes.Id {
			log.Println("自定义下单，gate，下单错误1", err, symbol, side, positionSide, quantityInt64, quantity, gateRes)
			return 0
		}

		if 1 == system {
			// 不存在新增，这里只能是开仓
			if "BOTH" == positionSide && "SELL" == side {
				tmpExecutedQty = -tmpExecutedQty
			}

			if !s.OrderMap.Contains(symbolRel + "&" + positionSide + "&" + strUserId) {
				s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
			} else {
				// 追加仓位，开仓
				if "LONG" == positionSide {
					if "BUY" == side {
						tmpExecutedQty += s.OrderMap.Get(symbolRel + "&" + positionSide + "&" + strUserId).(float64)
						s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
					} else if "SELL" == side {
						tmpExecutedQty = s.OrderMap.Get(symbolRel+"&"+positionSide+"&"+strUserId).(float64) - tmpExecutedQty
						if lessThanOrEqualZero(tmpExecutedQty, 1e-7) {
							tmpExecutedQty = 0
						}
						s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
					} else {
						log.Println("手动，gate下单，数据存储:", system, allCloseGate, apiKey, symbol, side, positionSide, num, gateRes, tmpExecutedQty)
					}

				} else if "SHORT" == positionSide {
					if "SELL" == side {
						tmpExecutedQty += s.OrderMap.Get(symbolRel + "&" + positionSide + "&" + strUserId).(float64)
						s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
					} else if "BUY" == side {
						tmpExecutedQty = s.OrderMap.Get(symbolRel+"&"+positionSide+"&"+strUserId).(float64) - tmpExecutedQty
						if lessThanOrEqualZero(tmpExecutedQty, 1e-7) {
							tmpExecutedQty = 0
						}
						s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
					} else {
						log.Println("手动，gate下单，数据存储:", system, allCloseGate, apiKey, symbol, side, positionSide, num, gateRes, tmpExecutedQty)
					}

				} else if "BOTH" == positionSide {
					tmpExecutedQty = s.OrderMap.Get(symbolRel+"&"+positionSide+"&"+strUserId).(float64) + tmpExecutedQty
					if floatEqual(tmpExecutedQty, 0, 1e-7) {
						tmpExecutedQty = 0
					}
					s.OrderMap.Set(symbolRel+"&"+positionSide+"&"+strUserId, tmpExecutedQty)
				} else {
					log.Println("手动，gate下单，数据存储:", system, allCloseGate, apiKey, symbol, side, positionSide, num, gateRes, tmpExecutedQty)
				}
			}
		}
	} else {
		log.Println("初始化，错误用户信息，开仓", vTmpUserMap)
		return 0
	}

	return 1
}
