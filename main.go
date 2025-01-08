package main

import (
	_ "plat_order/internal/packed"

	"github.com/gogf/gf/v2/os/gctx"

	"plat_order/internal/cmd"
)

func main() {
	cmd.Main.Run(gctx.GetInitCtx())
}
