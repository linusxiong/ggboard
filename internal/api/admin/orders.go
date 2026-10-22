package admin

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	ordersvc "github.com/linusxiong/ggboard/internal/service/order"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// registerOrders mounts /admin/orders* (admin only).
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerOrders(g *gin.RouterGroup, a *app.App) {
	og := g.Group("/orders", auth.RequireAdmin(a))
	og.GET("", adminListOrders(a))
	og.POST("", adminAssignOrder(a))
	og.GET("/:id", adminGetOrder(a))
	og.PATCH("/:id", adminUpdateOrder(a))
	og.POST("/:id/paid", adminMarkOrderPaid(a))
	og.POST("/:id/cancel", adminCancelOrder(a))
}

// adminOrderQuery holds the list filters. Dates accept YYYY-MM-DD (date_to inclusive) or RFC 3339.
type adminOrderQuery struct {
	TradeNo          string  `form:"trade_no" binding:"max=32"`
	UserID           *uint64 `form:"user_id"`
	Email            string  `form:"email" binding:"max=191"`
	Status           *int8   `form:"status" binding:"omitempty,min=0,max=4"`
	CommissionStatus *int8   `form:"commission_status" binding:"omitempty,min=0,max=3"`
	PlanID           *uint64 `form:"plan_id"`
	PaymentID        *uint64 `form:"payment_id"`
	InviteUserID     *uint64 `form:"invite_user_id"`
	Type             *int8   `form:"type" binding:"omitempty,min=1,max=4"`
	DateFrom         string  `form:"date_from"`
	DateTo           string  `form:"date_to"`
}

func parseCommerceDate(field, s string, endOfDay bool) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t, nil
	}
	d, err := time.ParseInLocation(time.DateOnly, s, time.Local)
	if err != nil {
		return nil, httpx.FieldError(field, "must be YYYY-MM-DD or an RFC 3339 time")
	}
	if endOfDay {
		d = d.AddDate(0, 0, 1)
	}
	return &d, nil
}

func adminListOrders(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q adminOrderQuery
		if err := httpx.BindQuery(c, &q); err != nil {
			httpx.Fail(c, err)
			return
		}
		from, err := parseCommerceDate("date_from", q.DateFrom, false)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		to, err := parseCommerceDate("date_to", q.DateTo, len(strings.TrimSpace(q.DateTo)) == len(time.DateOnly))
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		ctx := c.Request.Context()
		db := a.DB.WithContext(ctx).Model(&model.Order{})
		if q.TradeNo != "" {
			db = db.Where("trade_no = ?", strings.TrimSpace(q.TradeNo))
		}
		if q.UserID != nil {
			db = db.Where("user_id = ?", *q.UserID)
		}
		if q.Email != "" {
			db = db.Where("user_id IN (?)", a.DB.Model(&model.User{}).Select("id").Where("email = ?", usersvc.NormalizeEmail(q.Email)))
		}
		if q.Status != nil {
			db = db.Where("status = ?", *q.Status)
		}
		if q.CommissionStatus != nil {
			db = db.Where("commission_status = ?", *q.CommissionStatus)
		}
		if q.PlanID != nil {
			db = db.Where("plan_id = ?", *q.PlanID)
		}
		if q.PaymentID != nil {
			db = db.Where("payment_id = ?", *q.PaymentID)
		}
		if q.InviteUserID != nil {
			db = db.Where("invite_user_id = ?", *q.InviteUserID)
		}
		if q.Type != nil {
			db = db.Where("type = ?", *q.Type)
		}
		if from != nil {
			db = db.Where("created_at >= ?", *from)
		}
		if to != nil {
			db = db.Where("created_at < ?", *to)
		}
		orders, total, err := httpx.Paginate[model.Order](c, db, map[string]string{
			"id": "id", "created_at": "created_at", "paid_at": "paid_at", "total_amount": "total_amount", "status": "status",
		}, "id DESC")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		views, err := ordersvc.AdminViews(ctx, a.DB, orders)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, views, total)
	}
}

// adminOrderDetail is an order with its commission logs and the orders its surplus consumed.
type adminOrderDetail struct {
	ordersvc.AdminView
	CommissionLogs []model.CommissionLog `json:"commission_logs"`
	SurplusOrders  []ordersvc.AdminView  `json:"surplus_orders"`
}

func adminOrderResponse(c *gin.Context, a *app.App, o *model.Order) {
	ctx := c.Request.Context()
	views, err := ordersvc.AdminViews(ctx, a.DB, []model.Order{*o})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	d := adminOrderDetail{AdminView: views[0], CommissionLogs: []model.CommissionLog{}, SurplusOrders: []ordersvc.AdminView{}}
	if err := a.DB.WithContext(ctx).Where("trade_no = ?", o.TradeNo).Order("id").Find(&d.CommissionLogs).Error; err != nil {
		httpx.Fail(c, err)
		return
	}
	if len(o.SurplusOrderIDs) > 0 {
		var surplus []model.Order
		if err := a.DB.WithContext(ctx).Where("id IN ?", []uint64(o.SurplusOrderIDs)).Order("id").Find(&surplus).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		if d.SurplusOrders, err = ordersvc.AdminViews(ctx, a.DB, surplus); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	httpx.OK(c, d)
}

func adminLoadOrder(c *gin.Context, a *app.App) (*model.Order, bool) {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	var o model.Order
	if err := a.DB.WithContext(c.Request.Context()).First(&o, id).Error; err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	return &o, true
}

func adminGetOrder(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if o, ok := adminLoadOrder(c, a); ok {
			adminOrderResponse(c, a, o)
		}
	}
}

func adminAssignOrder(a *app.App) gin.HandlerFunc {
	type req struct {
		UserID      uint64 `json:"user_id" binding:"required_without=Email"`
		Email       string `json:"email" binding:"required_without=UserID,omitempty,email,max=191"`
		PlanID      uint64 `json:"plan_id" binding:"required"`
		Period      string `json:"period" binding:"required"`
		TotalAmount *int64 `json:"total_amount" binding:"omitempty,min=0"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		o, err := ordersvc.New(a).Assign(c.Request.Context(), ordersvc.AssignInput{
			UserID: in.UserID, Email: in.Email, PlanID: in.PlanID, Period: in.Period, TotalAmount: in.TotalAmount,
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("order assigned by admin", "trade_no", o.TradeNo, "admin_id", auth.CurrentUser(c).ID)
		views, err := ordersvc.AdminViews(c.Request.Context(), a.DB, []model.Order{*o})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Created(c, views[0])
	}
}

func adminMarkOrderPaid(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		o, err := ordersvc.New(a).MarkPaid(c.Request.Context(), id)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("order marked paid by admin", "trade_no", o.TradeNo, "admin_id", auth.CurrentUser(c).ID)
		adminOrderResponse(c, a, o)
	}
}

func adminCancelOrder(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		o, err := ordersvc.New(a).CancelBy(c.Request.Context(), id, ordersvc.CancelByAdmin)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("order cancelled by admin", "trade_no", o.TradeNo, "admin_id", auth.CurrentUser(c).ID)
		adminOrderResponse(c, a, o)
	}
}

func adminUpdateOrder(a *app.App) gin.HandlerFunc {
	type req struct {
		CommissionStatus *int8 `json:"commission_status" binding:"required,min=0,max=3"`
	}
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		o, err := ordersvc.New(a).SetCommissionStatus(c.Request.Context(), id, model.CommissionStatus(*in.CommissionStatus))
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("order commission updated by admin", "trade_no", o.TradeNo, "commission_status", o.CommissionStatus, "admin_id", auth.CurrentUser(c).ID)
		adminOrderResponse(c, a, o)
	}
}
