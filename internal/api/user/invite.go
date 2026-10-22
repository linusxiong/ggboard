package user

import (
	"errors"
	"strings"

	"github.com/linusxiong/ggboard/internal/service/account"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	invitesvc "github.com/linusxiong/ggboard/internal/service/invite"
	ticketsvc "github.com/linusxiong/ggboard/internal/service/ticket"
)

// registerInvite mounts GET /user/invite, POST /user/invite/codes, GET /user/invite/commissions, POST /user/commission/transfer, POST /user/commission/withdraw.
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerInvite(g *gin.RouterGroup, a *app.App) {
	g.GET("/invite", inviteOverview(a))
	g.POST("/invite/codes", inviteGenerate(a))
	g.GET("/invite/commissions", inviteCommissions(a))
	g.POST("/commission/transfer", commissionTransfer(a))
	g.POST("/commission/withdraw", commissionWithdraw(a))
}

// inviteOverview: GET /user/invite → {codes, stat}.
func inviteOverview(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		ov, err := invitesvc.New(a).Overview(c.Request.Context(), auth.CurrentUser(c).ID)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, ov)
	}
}

// inviteGenerate: POST /user/invite/codes → 201 {code} (400 at invite_gen_limit unused codes).
func inviteGenerate(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		code, err := invitesvc.New(a).GenerateCode(c.Request.Context(), auth.CurrentUser(c).ID)
		if errors.Is(err, invitesvc.ErrCodeLimit) {
			httpx.Fail(c, httpx.BadRequest("you already have the maximum number of unused invite codes"))
			return
		}
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Created(c, gin.H{"code": code})
	}
}

// inviteCommissions: GET /user/invite/commissions?page&page_size → [{trade_no, order_amount, get_amount, created_at}].
func inviteCommissions(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := invitesvc.New(a).CommissionQuery(c.Request.Context(), auth.CurrentUser(c).ID)
		rows, total, err := httpx.Paginate[invitesvc.Commission](c, q, nil, "created_at DESC, id DESC")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, rows, total)
	}
}

// commissionTransfer: POST /user/commission/transfer {amount (cents)} → User.
func commissionTransfer(a *app.App) gin.HandlerFunc {
	type req struct {
		Amount int64 `json:"amount" binding:"required,gt=0"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		u, err := invitesvc.New(a).Transfer(c.Request.Context(), auth.CurrentUser(c).ID, in.Amount)
		if errors.Is(err, invitesvc.ErrInsufficientCommission) {
			httpx.Fail(c, httpx.FieldError("amount", "exceeds your commission balance"))
			return
		}
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		view, err := account.New(a).View(c.Request.Context(), u)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, view)
	}
}

// commissionWithdraw: POST /user/commission/withdraw {method, account} → 201 {ticket_id}.
func commissionWithdraw(a *app.App) gin.HandlerFunc {
	type req struct {
		Method  string `json:"method" binding:"required,max=64"`
		Account string `json:"account" binding:"required,max=255"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		account := strings.TrimSpace(in.Account)
		if account == "" {
			httpx.Fail(c, httpx.FieldError("account", "is required"))
			return
		}
		svc := invitesvc.New(a)
		t, err := svc.Withdraw(c.Request.Context(), auth.CurrentUser(c).ID, strings.TrimSpace(in.Method), account)
		switch {
		case errors.Is(err, invitesvc.ErrWithdrawClosed):
			httpx.Fail(c, httpx.Forbidden("commission withdrawal is disabled"))
		case errors.Is(err, invitesvc.ErrWithdrawMethod):
			httpx.Fail(c, httpx.FieldError("method", "is not a supported withdrawal method"))
		case errors.Is(err, invitesvc.ErrWithdrawLimit):
			httpx.Fail(c, httpx.BadRequest("the minimum commission for a withdrawal is "+svc.WithdrawLimitText()))
		case errors.Is(err, ticketsvc.ErrOpenExists):
			httpx.Fail(c, httpx.Conflict("you already have an open ticket; close it or wait for it to be resolved before requesting a withdrawal"))
		case err != nil:
			httpx.Fail(c, err)
		default:
			httpx.Created(c, gin.H{"ticket_id": t.ID})
		}
	}
}
