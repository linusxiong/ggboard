package admin

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/payment"
	ordersvc "github.com/linusxiong/ggboard/internal/service/order"
	plansvc "github.com/linusxiong/ggboard/internal/service/plan"
)

// registerPayments mounts /admin/payments* (admin only).
// Owner: BE-C commerce. Implement in this file; do not add routes elsewhere.
func registerPayments(g *gin.RouterGroup, a *app.App) {
	pg := g.Group("/payments", auth.RequireAdmin(a))
	pg.GET("", adminListPayments(a))
	pg.GET("/gateways", adminListGateways)
	pg.POST("", adminCreatePayment(a))
	pg.POST("/sort", adminSortPayments(a))
	pg.GET("/:id", adminGetPayment(a))
	pg.PATCH("/:id", adminUpdatePayment(a))
	pg.DELETE("/:id", adminDeletePayment(a))
}

// gatewayConfigValidator is implemented by gateways that check their configuration beyond required fields
// (key parsing, currency support). siteCurrency is settings.currency.
type gatewayConfigValidator interface {
	ValidateConfig(cfg map[string]string, siteCurrency string) error
}

// adminPaymentView is a payment with its configuration (admin only) and the notify URL to register with the
// provider.
type adminPaymentView struct {
	model.Payment
	GatewayLabel string `json:"gateway_label"`
	NotifyURL    string `json:"notify_url"`
}

func commerceRequestBase(c *gin.Context) string { return httpx.RequestBase(c) }

func adminPaymentViewOf(c *gin.Context, a *app.App, p *model.Payment) adminPaymentView {
	v := adminPaymentView{Payment: *p, NotifyURL: ordersvc.New(a).PaymentNotifyURL(p, commerceRequestBase(c))}
	if v.Config == nil {
		v.Config = model.JSONObject[string]{}
	}
	if gw, ok := payment.Get(p.Gateway); ok {
		v.GatewayLabel = gw.Label()
	}
	return v
}

func adminListPayments(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var pays []model.Payment
		if err := a.DB.WithContext(c.Request.Context()).Clauses(plansvc.OrderBySort()).Find(&pays).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		out := make([]adminPaymentView, len(pays))
		for i := range pays {
			out[i] = adminPaymentViewOf(c, a, &pays[i])
		}
		httpx.List(c, out, int64(len(out)))
	}
}

type adminGatewayView struct {
	Name   string          `json:"name"`
	Label  string          `json:"label"`
	Fields []payment.Field `json:"fields"`
}

func adminListGateways(c *gin.Context) {
	gws := payment.List()
	out := make([]adminGatewayView, len(gws))
	for i, gw := range gws {
		fields := gw.Fields()
		if fields == nil {
			fields = []payment.Field{}
		}
		out[i] = adminGatewayView{Name: gw.Name(), Label: gw.Label(), Fields: fields}
	}
	httpx.List(c, out, int64(len(out)))
}

// adminPaymentInput is the writable payment shape; nil fields are left unchanged on PATCH.
type adminPaymentInput struct {
	Gateway            *string            `json:"gateway" binding:"omitempty,min=1,max=32"`
	Name               *string            `json:"name" binding:"omitempty,min=1,max=191"`
	Icon               *string            `json:"icon" binding:"omitempty,max=255"`
	Config             *map[string]string `json:"config"`
	NotifyDomain       *string            `json:"notify_domain" binding:"omitempty,max=255"`
	HandlingFeeFixed   *int64             `json:"handling_fee_fixed" binding:"omitempty,min=0"`
	HandlingFeePercent *float64           `json:"handling_fee_percent" binding:"omitempty,min=0,max=100"`
	Enabled            *bool              `json:"enabled"`
	Sort               *int               `json:"sort"`
}

func (in *adminPaymentInput) apply(p *model.Payment) {
	if in.Gateway != nil {
		p.Gateway = strings.TrimSpace(*in.Gateway)
	}
	if in.Name != nil {
		p.Name = *in.Name
	}
	if in.Icon != nil {
		p.Icon = *in.Icon
	}
	if in.Config != nil {
		cfg := model.JSONObject[string]{}
		for k, v := range *in.Config {
			cfg[k] = strings.TrimSpace(v)
		}
		p.Config = cfg
	}
	if in.NotifyDomain != nil {
		p.NotifyDomain = strings.TrimRight(strings.TrimSpace(*in.NotifyDomain), "/")
	}
	if in.HandlingFeeFixed != nil {
		p.HandlingFeeFixed = *in.HandlingFeeFixed
	}
	if in.HandlingFeePercent != nil {
		p.HandlingFeePercent = *in.HandlingFeePercent
	}
	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}
	if in.Sort != nil {
		p.Sort = *in.Sort
	}
}

// adminValidatePayment checks the gateway, required config fields, the notify domain and the gateway's own
// configuration rules.
func adminValidatePayment(a *app.App, p *model.Payment) error {
	e := commerceFieldErrs{}
	if p.Name == "" {
		e.add("name", "is required")
	}
	gw, ok := payment.Get(p.Gateway)
	if !ok {
		e.add("gateway", "unknown gateway; one of: "+strings.Join(payment.Names(), ", "))
		return e.err()
	}
	for _, f := range gw.Fields() {
		if f.Required && strings.TrimSpace(p.Config[f.Key]) == "" {
			e.add("config."+f.Key, "is required")
		}
	}
	if d := p.NotifyDomain; d != "" && !strings.HasPrefix(d, "https://") && !strings.HasPrefix(d, "http://") {
		e.add("notify_domain", "must start with https:// (scheme and host only)")
	}
	if err := e.err(); err != nil {
		return err
	}
	if v, ok := gw.(gatewayConfigValidator); ok {
		if err := v.ValidateConfig(map[string]string(p.Config), a.Settings.String("currency")); err != nil {
			return httpx.FieldError("config", err.Error())
		}
	}
	return nil
}

func adminCreatePayment(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in adminPaymentInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		if in.Gateway == nil {
			httpx.Fail(c, httpx.FieldError("gateway", "is required"))
			return
		}
		if in.Config == nil {
			httpx.Fail(c, httpx.FieldError("config", "is required"))
			return
		}
		p := &model.Payment{UUID: strings.ReplaceAll(uuid.NewString(), "-", "")}
		in.apply(p)
		if err := adminValidatePayment(a, p); err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := a.DB.WithContext(c.Request.Context()).Create(p).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("payment created", "payment_id", p.ID, "gateway", p.Gateway, "admin_id", auth.CurrentUser(c).ID)
		httpx.Created(c, adminPaymentViewOf(c, a, p))
	}
}

func adminLoadPayment(c *gin.Context, a *app.App) (*model.Payment, bool) {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	var p model.Payment
	if err := a.DB.WithContext(c.Request.Context()).First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = httpx.NotFound("payment not found")
		}
		httpx.Fail(c, err)
		return nil, false
	}
	return &p, true
}

func adminGetPayment(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p, ok := adminLoadPayment(c, a); ok {
			httpx.OK(c, adminPaymentViewOf(c, a, p))
		}
	}
}

func adminUpdatePayment(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := adminLoadPayment(c, a)
		if !ok {
			return
		}
		var in adminPaymentInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		in.apply(p)
		if err := adminValidatePayment(a, p); err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := a.DB.WithContext(c.Request.Context()).Save(p).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("payment updated", "payment_id", p.ID, "gateway", p.Gateway, "enabled", p.Enabled, "admin_id", auth.CurrentUser(c).ID)
		httpx.OK(c, adminPaymentViewOf(c, a, p))
	}
}

func adminDeletePayment(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := adminLoadPayment(c, a)
		if !ok {
			return
		}
		if err := a.DB.WithContext(c.Request.Context()).Delete(&model.Payment{}, p.ID).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("payment deleted", "payment_id", p.ID, "gateway", p.Gateway, "admin_id", auth.CurrentUser(c).ID)
		httpx.OK(c, true)
	}
}

func adminSortPayments(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) { commerceSort[model.Payment](c, a) }
}
