package admin

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	plansvc "github.com/linusxiong/ggboard/internal/service/plan"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// registerPlans mounts /admin/plans* (admin only).
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerPlans(g *gin.RouterGroup, a *app.App) {
	pg := g.Group("/plans", auth.RequireAdmin(a))
	pg.GET("", adminListPlans(a))
	pg.POST("", adminCreatePlan(a))
	pg.POST("/sort", adminSortPlans(a))
	pg.GET("/:id", adminGetPlan(a))
	pg.PATCH("/:id", adminUpdatePlan(a))
	pg.DELETE("/:id", adminDeletePlan(a))
}

// adminPlanInput is the writable plan shape; nil fields are left unchanged on PATCH.
type adminPlanInput struct {
	GroupID            *uint64           `json:"group_id"`
	Name               *string           `json:"name" binding:"omitempty,min=1,max=191"`
	Content            *string           `json:"content"`
	Tags               *[]string         `json:"tags" binding:"omitempty,max=20,dive,max=32"`
	TransferEnable     *int64            `json:"transfer_enable" binding:"omitempty,min=0"`
	SpeedLimit         *int              `json:"speed_limit" binding:"omitempty,min=0"`
	DeviceLimit        *int              `json:"device_limit" binding:"omitempty,min=0"`
	CapacityLimit      *int              `json:"capacity_limit" binding:"omitempty,min=0"`
	Prices             *map[string]int64 `json:"prices"`
	ResetTrafficMethod *int8             `json:"reset_traffic_method" binding:"omitempty,min=0,max=4"`
	Show               *bool             `json:"show"`
	Sell               *bool             `json:"sell"`
	Renew              *bool             `json:"renew"`
	Sort               *int              `json:"sort"`
	// ForceUpdate (PATCH) pushes group, traffic and limits to every user holding the plan (Xboard force_update).
	ForceUpdate bool `json:"force_update"`
	// Nullable fields that PATCH should clear explicitly.
	ClearSpeedLimit         bool `json:"-"`
	ClearDeviceLimit        bool `json:"-"`
	ClearCapacityLimit      bool `json:"-"`
	ClearResetTrafficMethod bool `json:"-"`
}

// adminPlanNulls reports which nullable keys were sent as JSON null (PATCH clears them).
func adminPlanNulls(raw map[string]any, in *adminPlanInput) {
	for key, dst := range map[string]*bool{
		"speed_limit": &in.ClearSpeedLimit, "device_limit": &in.ClearDeviceLimit,
		"capacity_limit": &in.ClearCapacityLimit, "reset_traffic_method": &in.ClearResetTrafficMethod,
	} {
		if v, ok := raw[key]; ok && v == nil {
			*dst = true
		}
	}
}

func (in *adminPlanInput) apply(p *model.Plan) {
	if in.GroupID != nil {
		p.GroupID = *in.GroupID
	}
	if in.Name != nil {
		p.Name = *in.Name
	}
	if in.Content != nil {
		p.Content = *in.Content
	}
	if in.Tags != nil {
		p.Tags = *in.Tags
	}
	if in.TransferEnable != nil {
		p.TransferEnable = *in.TransferEnable
	}
	if in.SpeedLimit != nil || in.ClearSpeedLimit {
		p.SpeedLimit = in.SpeedLimit
	}
	if in.DeviceLimit != nil || in.ClearDeviceLimit {
		p.DeviceLimit = in.DeviceLimit
	}
	if in.CapacityLimit != nil || in.ClearCapacityLimit {
		p.CapacityLimit = in.CapacityLimit
	}
	if in.Prices != nil {
		p.Prices = *in.Prices
	}
	if in.ResetTrafficMethod != nil {
		m := model.ResetMethod(*in.ResetTrafficMethod)
		p.ResetTrafficMethod = &m
	} else if in.ClearResetTrafficMethod {
		p.ResetTrafficMethod = nil
	}
	if in.Show != nil {
		p.Show = *in.Show
	}
	if in.Sell != nil {
		p.Sell = *in.Sell
	}
	if in.Renew != nil {
		p.Renew = *in.Renew
	}
	if in.Sort != nil {
		p.Sort = *in.Sort
	}
}

// commerceBindRaw decodes the JSON body into dst, validates it (binding tags) and also returns the raw object
// so PATCH handlers can tell an explicit null (clear) from an absent key (keep).
func commerceBindRaw(c *gin.Context, dst any) (map[string]any, error) {
	body, err := c.GetRawData()
	if err != nil {
		return nil, httpx.Validation(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, httpx.Validation(err)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return nil, httpx.Validation(err)
	}
	if err := httpx.Validate(dst); err != nil {
		return nil, err
	}
	return raw, nil
}

func adminBindPlan(c *gin.Context) (*adminPlanInput, error) {
	var in adminPlanInput
	raw, err := commerceBindRaw(c, &in)
	if err != nil {
		return nil, err
	}
	adminPlanNulls(raw, &in)
	return &in, nil
}

func adminValidatePlan(c *gin.Context, a *app.App, p *model.Plan) error {
	e := commerceFieldErrs{}
	if p.Name == "" {
		e.add("name", "is required")
	}
	if errs := plansvc.ValidatePrices(p.Prices); len(errs) > 0 {
		for k, v := range errs {
			for _, m := range v {
				e.add(k, m)
			}
		}
	}
	if p.GroupID != 0 {
		var n int64
		if err := a.DB.WithContext(c.Request.Context()).Model(&model.ServerGroup{}).Where("id = ?", p.GroupID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			e.add("group_id", "server group not found")
		}
	}
	return e.err()
}

func adminPlanView(c *gin.Context, a *app.App, p *model.Plan) (*plansvc.AdminView, error) {
	n, err := plansvc.ActiveUserCount(c.Request.Context(), a.DB, p.ID, 0, time.Now())
	if err != nil {
		return nil, err
	}
	return &plansvc.AdminView{Plan: *p, UserCount: n}, nil
}

func adminListPlans(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		plans, err := plansvc.ListForAdmin(c.Request.Context(), a.DB, time.Now())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, plans, int64(len(plans)))
	}
}

func adminLoadPlan(c *gin.Context, a *app.App) (*model.Plan, bool) {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	var p model.Plan
	if err := a.DB.WithContext(c.Request.Context()).First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = httpx.NotFound("plan not found")
		}
		httpx.Fail(c, err)
		return nil, false
	}
	return &p, true
}

func adminGetPlan(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := adminLoadPlan(c, a)
		if !ok {
			return
		}
		v, err := adminPlanView(c, a, p)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, v)
	}
}

func adminCreatePlan(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		in, err := adminBindPlan(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if in.GroupID == nil {
			httpx.Fail(c, httpx.FieldError("group_id", "is required"))
			return
		}
		p := &model.Plan{Sell: true, Renew: true, Prices: model.JSONObject[int64]{}, Tags: model.JSONSlice[string]{}}
		in.apply(p)
		if err := adminValidatePlan(c, a, p); err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := a.DB.WithContext(c.Request.Context()).Create(p).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("plan created", "plan_id", p.ID, "admin_id", auth.CurrentUser(c).ID)
		httpx.Created(c, plansvc.AdminView{Plan: *p})
	}
}

func adminUpdatePlan(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := adminLoadPlan(c, a)
		if !ok {
			return
		}
		in, err := adminBindPlan(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		in.apply(p)
		if err := adminValidatePlan(c, a, p); err != nil {
			httpx.Fail(c, err)
			return
		}
		err = a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(p).Error; err != nil {
				return err
			}
			if !in.ForceUpdate {
				return nil
			}
			return tx.Model(&model.User{}).Where("plan_id = ?", p.ID).Updates(map[string]any{
				"group_id": p.GroupID, "transfer_enable": p.TransferBytes(), "speed_limit": p.SpeedLimit, "device_limit": p.DeviceLimit,
			}).Error
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if in.ForceUpdate {
			usersvc.New(a).InvalidateNodeUsers()
		}
		httpx.Logger(c).Info("plan updated", "plan_id", p.ID, "force_update", in.ForceUpdate, "admin_id", auth.CurrentUser(c).ID)
		v, err := adminPlanView(c, a, p)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, v)
	}
}

func adminDeletePlan(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := adminLoadPlan(c, a)
		if !ok {
			return
		}
		ctx := c.Request.Context()
		holders, err := plansvc.HolderCount(ctx, a.DB, p.ID)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if holders > 0 {
			httpx.Fail(c, httpx.Conflict("users still hold this plan").WithField("user_count", "must be 0"))
			return
		}
		var pending int64
		if err := a.DB.WithContext(ctx).Model(&model.Order{}).Where("plan_id = ? AND status IN ?", p.ID,
			[]model.OrderStatus{model.OrderStatusPending, model.OrderStatusProcessing}).Count(&pending).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		if pending > 0 {
			httpx.Fail(c, httpx.Conflict("the plan has unpaid orders"))
			return
		}
		if err := a.DB.WithContext(ctx).Delete(&model.Plan{}, p.ID).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("plan deleted", "plan_id", p.ID, "admin_id", auth.CurrentUser(c).ID)
		httpx.OK(c, true)
	}
}

// commerceFieldErrs collects field-level messages into a single 400 validation_failed error.
type commerceFieldErrs map[string][]string

func (f commerceFieldErrs) add(field, msg string) { f[field] = append(f[field], msg) }

func (f commerceFieldErrs) err() error {
	if len(f) == 0 {
		return nil
	}
	e := httpx.BadRequest("validation failed")
	e.Errors = f
	return e
}

// commerceSortReq is {ids:[...]}: rows get sort = position (0-based) in ids.
type commerceSortReq struct {
	IDs []uint64 `json:"ids" binding:"required,min=1,max=1000,dive,gt=0"`
}

func commerceSort[T any](c *gin.Context, a *app.App) {
	var in commerceSortReq
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		for i, id := range in.IDs {
			if err := tx.Model(new(T)).Where("id = ?", id).UpdateColumn("sort", i).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, true)
}

func adminSortPlans(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) { commerceSort[model.Plan](c, a) }
}
