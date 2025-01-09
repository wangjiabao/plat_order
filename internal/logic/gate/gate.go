package gate

import (
	"context"
	"fmt"
	"github.com/gateio/gateapi-go/v6"
	"plat_order/internal/service"
)

type (
	sGate struct{}
)

func init() {
	service.RegisterGate(New())
}

func New() *sGate {
	return &sGate{}
}

// GetGateContract 获取合约账号信息
func (s *sGate) GetGateContract(apiK, apiS string) (gateapi.FuturesAccount, error) {
	client := gateapi.NewAPIClient(gateapi.NewConfiguration())
	// uncomment the next line if your are testing against testnet
	// client.ChangeBasePath("https://fx-api-testnet.gateio.ws/api/v4")
	ctx := context.WithValue(context.Background(),
		gateapi.ContextGateAPIV4,
		gateapi.GateAPIV4{
			Key:    apiK,
			Secret: apiS,
		},
	)

	result, _, err := client.FuturesApi.ListFuturesAccounts(ctx, "usdt")
	if err != nil {
		if e, ok := err.(gateapi.GateAPIError); ok {
			fmt.Println("gate api error: ", e.Error())
		} else {
			fmt.Println("generic error: ", err.Error())
		}
	}

	return result, nil
}

// PlaceOrderGate places an order on the Gate.io API with dynamic parameters
func (s *sGate) PlaceOrderGate(apiK, apiS, contract string, size int64, reduceOnly bool, autoSize string) (gateapi.FuturesOrder, error) {
	client := gateapi.NewAPIClient(gateapi.NewConfiguration())
	// uncomment the next line if your are testing against testnet
	// client.ChangeBasePath("https://fx-api-testnet.gateio.ws/api/v4")
	ctx := context.WithValue(context.Background(),
		gateapi.ContextGateAPIV4,
		gateapi.GateAPIV4{
			Key:    apiK,
			Secret: apiS,
		},
	)

	order := gateapi.FuturesOrder{
		Contract: contract,
		Size:     size,
		Tif:      "ioc",
		Price:    "0",
	}

	if autoSize != "" {
		order.AutoSize = autoSize
	}

	// 如果 reduceOnly 为 true，添加到请求数据中
	if reduceOnly {
		order.ReduceOnly = reduceOnly
	}

	result, _, err := client.FuturesApi.CreateFuturesOrder(ctx, "usdt", order)

	if err != nil {
		if e, ok := err.(gateapi.GateAPIError); ok {
			fmt.Println("gate api error: ", e.Error())
		} else {
			fmt.Println("generic error: ", err.Error())
		}
	}

	return result, nil
}
