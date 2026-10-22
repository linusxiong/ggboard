package user

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	ticketsvc "github.com/linusxiong/ggboard/internal/service/ticket"
)

// registerTickets mounts /user/tickets*.
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerTickets(g *gin.RouterGroup, a *app.App) {
	g.GET("/tickets", ticketList(a))
	g.POST("/tickets", ticketCreate(a))
	g.GET("/tickets/:id", ticketShow(a))
	g.POST("/tickets/:id/reply", ticketReply(a))
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
		f := ticketsvc.Filter{UserID: &u.ID}
		if in.Status != nil {
			st := model.TicketStatus(*in.Status)
			f.Status = &st
		}
		rows, total, err := httpx.Paginate[model.Ticket](c, ticketsvc.New(a).Query(c.Request.Context(), f), ticketSort, "updated_at DESC, id DESC")
		if err != nil {
			httpx.Fail(c, err)
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
		t, err := ticketsvc.New(a).Create(c.Request.Context(), u.ID, subject, model.TicketLevel(*in.Level), message)
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
		t, err := ticketsvc.New(a).Get(c.Request.Context(), id, auth.CurrentUser(c).ID)
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
		t, err := ticketsvc.New(a).ReplyAsOwner(c.Request.Context(), id, auth.CurrentUser(c).ID, message)
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
		if err := ticketsvc.New(a).Close(c.Request.Context(), id, auth.CurrentUser(c).ID); err != nil {
			httpx.Fail(c, ticketError(err))
			return
		}
		httpx.OK(c, true)
	}
}

// ticketError maps ticket service errors to API errors.
func ticketError(err error) error {
	switch {
	case errors.Is(err, ticketsvc.ErrNotFound):
		return httpx.NotFound("ticket not found")
	case errors.Is(err, ticketsvc.ErrOpenExists):
		return httpx.Conflict("you already have an open ticket; close it or wait for it to be resolved before opening another")
	case errors.Is(err, ticketsvc.ErrClosed):
		return httpx.BadRequest("the ticket is closed and cannot be replied to")
	}
	return err
}
