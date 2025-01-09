package user

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"plat_order/internal/model/entity"
	"plat_order/internal/service"
)

type (
	sUser struct{}
)

func init() {
	service.RegisterUser(New())
}

func New() *sUser {
	return &sUser{}
}

func (s *sUser) GetAllTraders(ctx context.Context) (users []*entity.User, err error) {
	err = g.Model("user").Ctx(ctx).Where("is_open=?", 1).Scan(&users)
	return users, err
}
