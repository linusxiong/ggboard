package user

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	ticketsvc "github.com/linusxiong/ggboard/internal/service/ticket"
)

// registerTickets mounts /user/tickets*.
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
// Ticket writes are per-user token buckets (security audit 2026-10-22, notification-queue flood): every create
// emits ticket_new to the admins and the "one open ticket" rule is no limit for an open/close loop.
const (
	ticketCreatePerHour = 30
	ticketCreateBurst   = 10
	ticketReplyPerHour  = 120
	ticketReplyBurst    = 30
)

func registerTickets(g *gin.RouterGroup, a *app.App) {
	byUser := func(c *gin.Context) string {
		if u := auth.CurrentUser(c); u != nil {
			return strconv.FormatUint(u.ID, 10)
		}
		return c.ClientIP()
	}
	create := httpx.Limit("tickets", httpx.NewLimiter(httpx.Per(ticketCreatePerHour, time.Hour), ticketCreateBurst), byUser)
	reply := httpx.Limit("ticket_replies", httpx.NewLimiter(httpx.Per(ticketReplyPerHour, time.Hour), ticketReplyBurst), byUser)
	g.GET("/tickets", ticketList(a))
	g.POST("/tickets", create, ticketCreate(a))
	g.GET("/tickets/:id", ticketShow(a))
	g.POST("/tickets/:id/reply", reply, ticketReply(a))
	g.POST("/tickets/:id/close", ticketClose(a))
}

var ticketSort = map[string]string{"id": "id", "created_at": "created_at", "updated_at": "updated_at"}

// ticketList: GET /user/tickets?page&page_size&status — own tickets, newest activity first, no messages.
func ticketList(a *app.App) gin.HandlerFunc {
	type query struct {
		Status *int8 `form:"status" binding:"omitnil,oneof=0 1"`
	}
	return func(c *gin.Context) {
		var in query
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		u := auth.CurrentUser(c)
		var rows []model.Ticket
		var total int64
		err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			if _, err := ticketsvc.LockOwner(tx, u); err != nil {
				return err
			}
			q := tx.Model(&model.Ticket{}).Where("user_id = ?", u.ID)
			if in.Status != nil {
				q = q.Where("status = ?", *in.Status)
			}
			var err error
			rows, total, err = httpx.Paginate[model.Ticket](c, q, ticketSort, "updated_at DESC, id DESC")
			return err
		})
		if err != nil {
			httpx.Fail(c, ticketError(err))
			return
		}
		out := make([]ticketsvc.View, 0, len(rows))
		for i := range rows {
			out = append(out, ticketsvc.ForOwner(&rows[i]))
		}
		httpx.List(c, out, total)
	}
}

// ticketCreate: POST /user/tickets {subject, level, message} → 201 Ticket (409 while another ticket is open).
func ticketCreate(a *app.App) gin.HandlerFunc {
	type req struct {
		Subject string `json:"subject" binding:"required,max=255"`
		Level   *int8  `json:"level" binding:"required,oneof=0 1 2"`
		Message string `json:"message" binding:"required,max=10000"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		subject, message := strings.TrimSpace(in.Subject), strings.TrimSpace(in.Message)
		if subject == "" {
			httpx.Fail(c, httpx.FieldError("subject", "is required"))
			return
		}
		if message == "" {
			httpx.Fail(c, httpx.FieldError("message", "is required"))
			return
		}
		u := auth.CurrentUser(c)
		t, err := ticketsvc.New(a).CreateForOwner(c.Request.Context(), u, subject, model.TicketLevel(*in.Level), message)
		if err != nil {
			httpx.Fail(c, ticketError(err))
			return
		}
		httpx.Created(c, ticketsvc.ForOwner(t))
	}
}

// ticketShow: GET /user/tickets/:id — own ticket with messages (404 for other users' tickets).
func ticketShow(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		t, err := ticketsvc.New(a).GetForOwner(c.Request.Context(), id, auth.CurrentUser(c))
		if err != nil {
			httpx.Fail(c, ticketError(err))
			return
		}
		httpx.OK(c, ticketsvc.ForOwner(t))
	}
}

// ticketReply: POST /user/tickets/:id/reply {message} → Ticket (closed → 400).
func ticketReply(a *app.App) gin.HandlerFunc {
	type req struct {
		Message string `json:"message" binding:"required,max=10000"`
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
		message := strings.TrimSpace(in.Message)
		if message == "" {
			httpx.Fail(c, httpx.FieldError("message", "is required"))
			return
		}
		t, err := ticketsvc.New(a).ReplyForOwner(c.Request.Context(), id, auth.CurrentUser(c), message)
		if err != nil {
			httpx.Fail(c, ticketError(err))
			return
		}
		httpx.OK(c, ticketsvc.ForOwner(t))
	}
}

// ticketClose: POST /user/tickets/:id/close → true.
func ticketClose(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := ticketsvc.New(a).CloseForOwner(c.Request.Context(), id, auth.CurrentUser(c)); err != nil {
			httpx.Fail(c, ticketError(err))
			return
		}
		httpx.OK(c, true)
	}
}

// ticketError maps ticket service errors to API errors.
func ticketError(err error) error {
	switch {
	case errors.Is(err, ticketsvc.ErrSessionExpired):
		return httpx.Unauthorized("session expired, please sign in again")
	case errors.Is(err, ticketsvc.ErrNotFound):
		return httpx.NotFound("ticket not found")
	case errors.Is(err, ticketsvc.ErrOpenExists):
		return httpx.Conflict("you already have an open ticket; close it or wait for it to be resolved before opening another")
	case errors.Is(err, ticketsvc.ErrClosed):
		return httpx.BadRequest("the ticket is closed and cannot be replied to")
	}
	return err
}
