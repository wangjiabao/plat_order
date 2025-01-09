package main

import (
	_ "plat_order/internal/packed"

	"github.com/gogf/gf/v2/os/gctx"

	_ "github.com/gogf/gf/contrib/drivers/mysql/v2"
	_ "plat_order/internal/logic"

	"plat_order/internal/cmd"
)

func main() {
	cmd.Main.Run(gctx.GetInitCtx())
}
