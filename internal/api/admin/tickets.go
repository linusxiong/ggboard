package admin

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	ticketsvc "github.com/linusxiong/ggboard/internal/service/ticket"
)

// registerTickets mounts /admin/tickets* (staff).
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerTickets(g *gin.RouterGroup, a *app.App) {
	g.GET("/tickets", ticketList(a))
	g.GET("/tickets/:id", ticketShow(a))
	g.POST("/tickets/:id/reply", ticketReply(a))
	g.POST("/tickets/:id/close", ticketClose(a))
}

var ticketSort = map[string]string{
	"id": "id", "created_at": "created_at", "updated_at": "updated_at", "level": "level",
	"status": "status", "reply_status": "reply_status",
}

// ticketList: GET /admin/tickets?status&reply_status&user_id&email&page&page_size&sort → [Ticket + user].
func ticketList(a *app.App) gin.HandlerFunc {
	type query struct {
		Status      *int8   `form:"status" binding:"omitnil,oneof=0 1"`
		ReplyStatus *int8   `form:"reply_status" binding:"omitnil,oneof=0 1"`
		UserID      *uint64 `form:"user_id" binding:"omitnil,gt=0"`
		Email       string  `form:"email" binding:"max=191"`
	}
	return func(c *gin.Context) {
		var in query
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		f := ticketsvc.Filter{UserID: in.UserID, Email: in.Email}
		if in.Status != nil {
			st := model.TicketStatus(*in.Status)
			f.Status = &st
		}
		if in.ReplyStatus != nil {
			rs := model.TicketReplyStatus(*in.ReplyStatus)
			f.ReplyStatus = &rs
		}
		svc := ticketsvc.New(a)
		var rows []model.Ticket
		var total int64
		var emails map[uint64]string
		err := withLiveStaffTx(c, a, func(tx *gorm.DB, _ bool) error {
			var readErr error
			rows, total, readErr = httpx.Paginate[model.Ticket](c, svc.QueryWithDB(tx, f), ticketSort, "updated_at DESC, id DESC")
			if readErr != nil {
				return readErr
			}
			emails, readErr = svc.OwnersWithDB(tx, rows)
			return readErr
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out := make([]ticketsvc.View, 0, len(rows))
		for i := range rows {
			out = append(out, ticketsvc.ForStaff(&rows[i], emails[rows[i].UserID]))
		}
		httpx.List(c, out, total)
	}
}

// ticketShow: GET /admin/tickets/:id → Ticket with messages and user.
func ticketShow(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		svc := ticketsvc.New(a)
		var t *model.Ticket
		var email string
		err = withLiveStaffTx(c, a, func(tx *gorm.DB, _ bool) error {
			var readErr error
			t, readErr = svc.GetWithDB(tx, id, 0)
			if readErr != nil {
				return readErr
			}
			emails, readErr := svc.OwnersWithDB(tx, []model.Ticket{*t})
			email = emails[t.UserID]
			return readErr
		})
		if err != nil {
			httpx.Fail(c, ticketError(err))
			return
		}
		httpx.OK(c, ticketsvc.ForStaff(t, email))
	}
}

// ticketReply: POST /admin/tickets/:id/reply {message} → Ticket; reopens the ticket, marks it replied and
// e-mails the owner.
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
		svc := ticketsvc.New(a)
		staff := auth.CurrentUser(c)
		t, email, err := svc.ReplyAsStaffIf(c.Request.Context(), id, staff.ID, message, func(tx *gorm.DB) error {
			_, err := liveStaffRole(c, tx, true)
			return err
		})
		if err != nil {
			httpx.Fail(c, ticketError(err))
			return
		}
		httpx.Logger(c).Info("admin: ticket replied", "ticket_id", id, "staff_id", staff.ID)
		httpx.OK(c, ticketsvc.ForStaff(t, email))
	}
}

// ticketClose: POST /admin/tickets/:id/close → true.
func ticketClose(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := ticketsvc.New(a).CloseAsStaffIf(c.Request.Context(), id, func(tx *gorm.DB) error {
			_, err := liveStaffRole(c, tx, true)
			return err
		}); err != nil {
			httpx.Fail(c, ticketError(err))
			return
		}
		httpx.Logger(c).Info("admin: ticket closed", "ticket_id", id, "staff_id", auth.CurrentUser(c).ID)
		httpx.OK(c, true)
	}
}

func ticketError(err error) error {
	if errors.Is(err, ticketsvc.ErrNotFound) {
		return httpx.NotFound("ticket not found")
	}
	return err
}
