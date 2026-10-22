package user

import (
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	ordersvc "github.com/linusxiong/ggboard/internal/service/order"
	plansvc "github.com/linusxiong/ggboard/internal/service/plan"
)

// registerShop mounts GET /user/plans, GET /user/plans/:id, GET /user/payment-methods, POST /user/coupons/check, /user/orders*.
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerShop(g *gin.RouterGroup, a *app.App) {
	byUser := func(c *gin.Context) string {
		if u := auth.CurrentUser(c); u != nil {
			return strconv.FormatUint(u.ID, 10)
		}
		return c.ClientIP()
	}
	limit := httpx.Limit("shop_orders", httpx.NewLimiter(httpx.Per(30, time.Minute), 10), byUser)

	g.GET("/plans", shopListPlans(a))
	g.GET("/plans/:id", shopGetPlan(a))
	g.GET("/payment-methods", shopPaymentMethods(a))
	g.POST("/coupons/check", limit, shopCheckCoupon(a))
	g.GET("/orders", shopListOrders(a))
	g.POST("/orders", limit, shopCreateOrder(a))
	g.GET("/orders/:trade_no", shopGetOrder(a))
	g.POST("/orders/:trade_no/checkout", limit, shopCheckout(a))
	g.GET("/orders/:trade_no/status", shopOrderStatus(a))
	g.POST("/orders/:trade_no/cancel", shopCancelOrder(a))
}

func shopListPlans(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		plans, err := plansvc.ListForUser(c.Request.Context(), a.DB, auth.CurrentUser(c), time.Now())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, plans, int64(len(plans)))
	}
}

func shopGetPlan(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		p, err := plansvc.GetForUser(c.Request.Context(), a.DB, auth.CurrentUser(c), id, time.Now())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, p)
	}
}

// shopPaymentMethod is an enabled payment as shown to users (no gateway configuration).
type shopPaymentMethod struct {
	ID                 uint64  `json:"id"`
	Name               string  `json:"name"`
	Icon               string  `json:"icon"`
	Gateway            string  `json:"gateway"`
	HandlingFeeFixed   int64   `json:"handling_fee_fixed"`
	HandlingFeePercent float64 `json:"handling_fee_percent"`
}

func shopPaymentMethods(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var pays []model.Payment
		err := a.DB.WithContext(c.Request.Context()).Where("enabled = ?", true).Clauses(plansvc.OrderBySort()).Find(&pays).Error
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out := make([]shopPaymentMethod, 0, len(pays))
		for _, p := range pays {
			out = append(out, shopPaymentMethod{ID: p.ID, Name: p.Name, Icon: p.Icon, Gateway: p.Gateway,
				HandlingFeeFixed: p.HandlingFeeFixed, HandlingFeePercent: p.HandlingFeePercent})
		}
		httpx.List(c, out, int64(len(out)))
	}
}

func shopCheckCoupon(a *app.App) gin.HandlerFunc {
	type req struct {
		Code   string `json:"code" binding:"required,max=64"`
		PlanID uint64 `json:"plan_id" binding:"required"`
		Period string `json:"period" binding:"required"`
	}
	type resp struct {
		ID           uint64           `json:"id"`
		Code         string           `json:"code"`
		Name         string           `json:"name"`
		Type         model.CouponType `json:"type"`
		Value        int64            `json:"value"`
		LimitPlanIDs []uint64         `json:"limit_plan_ids"`
		LimitPeriods []string         `json:"limit_periods"`
		EndedAt      time.Time        `json:"ended_at"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		cp, err := ordersvc.New(a).CheckCoupon(c.Request.Context(), auth.CurrentUser(c).ID, in.Code, in.PlanID, in.Period)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, resp{ID: cp.ID, Code: cp.Code, Name: cp.Name, Type: cp.Type, Value: cp.Value,
			LimitPlanIDs: shopNonNil(cp.LimitPlanIDs), LimitPeriods: shopNonNil(cp.LimitPeriods), EndedAt: cp.EndedAt})
	}
}

func shopNonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func shopListOrders(a *app.App) gin.HandlerFunc {
	type query struct {
		Status *int8 `form:"status" binding:"omitempty,min=0,max=4"`
	}
	return func(c *gin.Context) {
		var q query
		if err := httpx.BindQuery(c, &q); err != nil {
			httpx.Fail(c, err)
			return
		}
		db := a.DB.WithContext(c.Request.Context()).Model(&model.Order{}).Where("user_id = ?", auth.CurrentUser(c).ID)
		if q.Status != nil {
			db = db.Where("status = ?", *q.Status)
		}
		orders, total, err := httpx.Paginate[model.Order](c, db, map[string]string{"created_at": "created_at", "id": "id"}, "id DESC")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		views, err := ordersvc.Views(c.Request.Context(), a.DB, orders)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, views, total)
	}
}

func shopCreateOrder(a *app.App) gin.HandlerFunc {
	type req struct {
		PlanID     uint64 `json:"plan_id" binding:"required"`
		Period     string `json:"period" binding:"required"`
		CouponCode string `json:"coupon_code" binding:"max=64"`
	}
	type resp struct {
		TradeNo string            `json:"trade_no"`
		Status  model.OrderStatus `json:"status"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		o, err := ordersvc.New(a).Create(c.Request.Context(), auth.CurrentUser(c).ID,
			ordersvc.CreateInput{PlanID: in.PlanID, Period: in.Period, CouponCode: in.CouponCode})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Created(c, resp{TradeNo: o.TradeNo, Status: o.Status})
	}
}

// shopOrder loads the current user's order by :trade_no (404 otherwise).
func shopOrder(c *gin.Context, a *app.App) (*model.Order, bool) {
	var o model.Order
	err := a.DB.WithContext(c.Request.Context()).Where("trade_no = ? AND user_id = ?", c.Param("trade_no"), auth.CurrentUser(c).ID).First(&o).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = httpx.NotFound("order not found")
		}
		httpx.Fail(c, err)
		return nil, false
	}
	return &o, true
}

func shopGetOrder(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		o, ok := shopOrder(c, a)
		if !ok {
			return
		}
		views, err := ordersvc.Views(c.Request.Context(), a.DB, []model.Order{*o})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, views[0])
	}
}

// shopRequestBase is scheme://host of the request (used when app_url is not configured; X-Forwarded-Proto
// only from trusted proxies).
func shopRequestBase(c *gin.Context) string { return httpx.RequestBase(c) }

func shopCheckout(a *app.App) gin.HandlerFunc {
	type req struct {
		PaymentID *uint64 `json:"payment_id"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		res, err := ordersvc.New(a).Checkout(c.Request.Context(), auth.CurrentUser(c).ID, c.Param("trade_no"), ordersvc.CheckoutInput{
			PaymentID: in.PaymentID, ClientIP: c.ClientIP(), BaseURL: shopRequestBase(c),
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, res)
	}
}

func shopOrderStatus(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		o, ok := shopOrder(c, a)
		if !ok {
			return
		}
		httpx.OK(c, gin.H{"status": o.Status})
	}
}

func shopCancelOrder(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		o, ok := shopOrder(c, a)
		if !ok {
			return
		}
		if _, err := ordersvc.New(a).Cancel(c.Request.Context(), o.ID); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, true)
	}
}
