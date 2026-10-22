package admin

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	ordersvc "github.com/linusxiong/ggboard/internal/service/order"
)

// registerCoupons mounts /admin/coupons* (admin only).
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerCoupons(g *gin.RouterGroup, a *app.App) {
	cg := g.Group("/coupons", auth.RequireAdmin(a))
	cg.GET("", adminListCoupons(a))
	cg.POST("", adminCreateCoupons(a))
	cg.GET("/:id", adminGetCoupon(a))
	cg.PATCH("/:id", adminUpdateCoupon(a))
	cg.DELETE("/:id", adminDeleteCoupon(a))
}

// adminCouponView is a coupon plus the number of non-cancelled orders that used it.
type adminCouponView struct {
	model.Coupon
	UsedCount int64 `json:"used_count"`
}

const couponCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"

// newCouponCode returns an 8-character random code (Xboard randomChar(8), without look-alike characters).
func newCouponCode() string {
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(couponCodeAlphabet))))
		if err != nil {
			panic("admin: crypto/rand failed: " + err.Error())
		}
		b[i] = couponCodeAlphabet[n.Int64()]
	}
	return string(b)
}

func adminCouponViews(c *gin.Context, a *app.App, coupons []model.Coupon) ([]adminCouponView, error) {
	ids := make([]uint64, len(coupons))
	for i, cp := range coupons {
		ids[i] = cp.ID
	}
	usage, err := ordersvc.CouponUsage(c.Request.Context(), a.DB, ids)
	if err != nil {
		return nil, err
	}
	out := make([]adminCouponView, len(coupons))
	for i, cp := range coupons {
		out[i] = adminCouponView{Coupon: cp, UsedCount: usage[cp.ID]}
	}
	return out, nil
}

func adminListCoupons(a *app.App) gin.HandlerFunc {
	type query struct {
		Code string `form:"code" binding:"max=64"`
		Name string `form:"name" binding:"max=191"`
	}
	return func(c *gin.Context) {
		var q query
		if err := httpx.BindQuery(c, &q); err != nil {
			httpx.Fail(c, err)
			return
		}
		db := a.DB.WithContext(c.Request.Context()).Model(&model.Coupon{})
		if q.Code != "" {
			db = db.Where("code = ?", strings.TrimSpace(q.Code))
		}
		if q.Name != "" {
			db = db.Where("name LIKE ?", "%"+q.Name+"%")
		}
		coupons, total, err := httpx.Paginate[model.Coupon](c, db, map[string]string{
			"id": "id", "created_at": "created_at", "ended_at": "ended_at", "started_at": "started_at", "value": "value",
		}, "id DESC")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		views, err := adminCouponViews(c, a, coupons)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, views, total)
	}
}

// adminCouponInput is the writable coupon shape; nil fields are left unchanged on PATCH.
type adminCouponInput struct {
	Name             *string    `json:"name" binding:"omitempty,min=1,max=191"`
	Code             *string    `json:"code" binding:"omitempty,min=1,max=64"`
	Type             *int8      `json:"type" binding:"omitempty,oneof=1 2"`
	Value            *int64     `json:"value" binding:"omitempty,gt=0"`
	Count            int        `json:"count" binding:"omitempty,min=1,max=500"`
	LimitUse         *int       `json:"limit_use" binding:"omitempty,min=0"`
	LimitUseWithUser *int       `json:"limit_use_with_user" binding:"omitempty,min=1"`
	LimitPlanIDs     *[]uint64  `json:"limit_plan_ids" binding:"omitempty,max=200"`
	LimitPeriods     *[]string  `json:"limit_periods" binding:"omitempty,max=8"`
	StartedAt        *time.Time `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at"`
	Show             *bool      `json:"show"`
	clearLimitUse    bool
	clearLimitUser   bool
}

func (in *adminCouponInput) apply(cp *model.Coupon) {
	if in.Name != nil {
		cp.Name = *in.Name
	}
	if in.Code != nil {
		cp.Code = strings.TrimSpace(*in.Code)
	}
	if in.Type != nil {
		cp.Type = model.CouponType(*in.Type)
	}
	if in.Value != nil {
		cp.Value = *in.Value
	}
	if in.LimitUse != nil || in.clearLimitUse {
		cp.LimitUse = in.LimitUse
	}
	if in.LimitUseWithUser != nil || in.clearLimitUser {
		cp.LimitUseWithUser = in.LimitUseWithUser
	}
	if in.LimitPlanIDs != nil {
		cp.LimitPlanIDs = *in.LimitPlanIDs
	}
	if in.LimitPeriods != nil {
		cp.LimitPeriods = *in.LimitPeriods
	}
	if in.StartedAt != nil {
		cp.StartedAt = *in.StartedAt
	}
	if in.EndedAt != nil {
		cp.EndedAt = *in.EndedAt
	}
	if in.Show != nil {
		cp.Show = *in.Show
	}
}

func adminBindCoupon(c *gin.Context) (*adminCouponInput, error) {
	var in adminCouponInput
	raw, err := commerceBindRaw(c, &in)
	if err != nil {
		return nil, err
	}
	if v, ok := raw["limit_use"]; ok && v == nil {
		in.clearLimitUse = true
	}
	if v, ok := raw["limit_use_with_user"]; ok && v == nil {
		in.clearLimitUser = true
	}
	return &in, nil
}

func adminValidateCoupon(c *gin.Context, a *app.App, cp *model.Coupon) error {
	e := commerceFieldErrs{}
	if cp.Name == "" {
		e.add("name", "is required")
	}
	if strings.ContainsAny(cp.Code, " \t\r\n") {
		e.add("code", "must not contain whitespace")
	}
	switch cp.Type {
	case model.CouponTypeAmount:
	case model.CouponTypePercent:
		if cp.Value > 100 {
			e.add("value", "must be at most 100 for a percent coupon")
		}
	default:
		e.add("type", "must be one of: 1, 2")
	}
	if cp.Value <= 0 {
		e.add("value", "must be greater than 0")
	}
	if cp.StartedAt.IsZero() {
		e.add("started_at", "is required")
	}
	if cp.EndedAt.IsZero() {
		e.add("ended_at", "is required")
	} else if !cp.EndedAt.After(cp.StartedAt) {
		e.add("ended_at", "must be after started_at")
	}
	for _, p := range cp.LimitPeriods {
		if !model.IsValidPeriod(p) {
			e.add("limit_periods", "unknown period "+p)
		}
	}
	if len(cp.LimitPlanIDs) > 0 {
		var n int64
		if err := a.DB.WithContext(c.Request.Context()).Model(&model.Plan{}).Where("id IN ?", []uint64(cp.LimitPlanIDs)).Count(&n).Error; err != nil {
			return err
		}
		if n != int64(len(cp.LimitPlanIDs)) {
			e.add("limit_plan_ids", "contains unknown plans")
		}
	}
	return e.err()
}

func adminCreateCoupons(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		in, err := adminBindCoupon(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		tmpl := model.Coupon{Show: true, LimitPlanIDs: model.JSONSlice[uint64]{}, LimitPeriods: model.JSONSlice[string]{}}
		in.apply(&tmpl)
		if err := adminValidateCoupon(c, a, &tmpl); err != nil {
			httpx.Fail(c, err)
			return
		}
		count := max(in.Count, 1)
		batch := count > 1 // a batch always gets generated codes
		if !batch && tmpl.Code == "" {
			tmpl.Code = newCouponCode()
		}
		coupons := make([]model.Coupon, count)
		for i := range coupons {
			coupons[i] = tmpl
			if batch {
				coupons[i].Code = newCouponCode()
			}
		}
		codes := make([]string, count)
		for i, cp := range coupons {
			codes[i] = cp.Code
		}
		var dup int64
		if err := a.DB.WithContext(c.Request.Context()).Model(&model.Coupon{}).Where("code IN ?", codes).Count(&dup).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		if dup > 0 {
			httpx.Fail(c, httpx.Conflict("coupon code already exists").WithField("code", "already exists"))
			return
		}
		if err := a.DB.WithContext(c.Request.Context()).CreateInBatches(&coupons, 100).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("coupons created", "count", count, "admin_id", auth.CurrentUser(c).ID)
		views, err := adminCouponViews(c, a, coupons)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Created(c, views)
	}
}

func adminLoadCoupon(c *gin.Context, a *app.App) (*model.Coupon, bool) {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	var cp model.Coupon
	if err := a.DB.WithContext(c.Request.Context()).First(&cp, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = httpx.NotFound("coupon not found")
		}
		httpx.Fail(c, err)
		return nil, false
	}
	return &cp, true
}

func adminCouponResponse(c *gin.Context, a *app.App, cp *model.Coupon) {
	views, err := adminCouponViews(c, a, []model.Coupon{*cp})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, views[0])
}

func adminGetCoupon(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cp, ok := adminLoadCoupon(c, a); ok {
			adminCouponResponse(c, a, cp)
		}
	}
}

func adminUpdateCoupon(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		cp, ok := adminLoadCoupon(c, a)
		if !ok {
			return
		}
		in, err := adminBindCoupon(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if in.Count > 0 {
			httpx.Fail(c, httpx.FieldError("count", "is only accepted when creating coupons"))
			return
		}
		in.apply(cp)
		if cp.Code == "" {
			httpx.Fail(c, httpx.FieldError("code", "is required"))
			return
		}
		if err := adminValidateCoupon(c, a, cp); err != nil {
			httpx.Fail(c, err)
			return
		}
		var dup int64
		if err := a.DB.WithContext(c.Request.Context()).Model(&model.Coupon{}).Where("code = ? AND id <> ?", cp.Code, cp.ID).Count(&dup).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		if dup > 0 {
			httpx.Fail(c, httpx.Conflict("coupon code already exists").WithField("code", "already exists"))
			return
		}
		if err := a.DB.WithContext(c.Request.Context()).Save(cp).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("coupon updated", "coupon_id", cp.ID, "admin_id", auth.CurrentUser(c).ID)
		adminCouponResponse(c, a, cp)
	}
}

func adminDeleteCoupon(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		cp, ok := adminLoadCoupon(c, a)
		if !ok {
			return
		}
		if err := a.DB.WithContext(c.Request.Context()).Delete(&model.Coupon{}, cp.ID).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("coupon deleted", "coupon_id", cp.ID, "admin_id", auth.CurrentUser(c).ID)
		httpx.OK(c, true)
	}
}
