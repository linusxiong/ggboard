package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/router"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/settings"
)

func serveCmd(a *app.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the HTTP server, scheduler and mail queue",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return Serve(ctx, a)
		},
	}
	cmd.Flags().String("addr", "", "listen address, e.g. :8080 (overrides server.addr)")
	return cmd
}

// Serve runs the server until ctx is cancelled, then shuts down gracefully (10s drain). The caller closes a.
func Serve(ctx context.Context, a *app.App) error {
	handler, err := router.New(a)
	if err != nil {
		return err
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", a.Cfg.Server.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", a.Cfg.Server.Addr, err)
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(a.Log.Handler(), slog.LevelWarn),
	}
	a.Start()
	a.Log.Info("ggboard listening", "addr", ln.Addr().String(), "version", a.Version, "db", a.DBDriver())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	a.Log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func migrateCmd(a *app.App) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Create or update the database schema",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// The App runs migrations during initialization.
			fmt.Fprintf(cmd.OutOrStdout(), "database migrated (%s)\n", a.DBDriver())
			return nil
		},
	}
}

func versionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:         "version",
		Short:       "Print the version",
		Annotations: map[string]string{AnnotationNoApp: "true"},
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "ggboard %s (%s %s/%s)\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		},
	}
}

func adminCmd(a *app.App) *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Manage administrator accounts"}

	var email, password string
	var staff bool
	create := &cobra.Command{
		Use:   "create",
		Short: "Create an administrator (or staff with --staff)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			pw, generated, err := passwordOrRandom(password)
			if err != nil {
				return err
			}
			if err := checkEmail(email); err != nil {
				return err
			}
			users := usersvc.New(a)
			u, err := users.Create(cmd.Context(), email, pw, usersvc.CreateOptions{IsAdmin: !staff, IsStaff: staff})
			if err != nil {
				return err
			}
			// The operator chose the address: a later password reset is not an ownership change.
			if err := users.MarkEmailVerified(cmd.Context(), u); err != nil {
				return err
			}
			role := "administrator"
			if staff {
				role = "staff"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created %s %s (id %d)\n", role, u.Email, u.ID)
			if generated {
				fmt.Fprintf(cmd.OutOrStdout(), "generated password: %s\n", pw)
			}
			return nil
		},
	}
	create.Flags().StringVar(&email, "email", "", "email address (required)")
	create.Flags().StringVar(&password, "password", "", "password (min 8 chars; generated when omitted)")
	create.Flags().BoolVar(&staff, "staff", false, "create a staff member instead of an administrator")
	_ = create.MarkFlagRequired("email")

	var rpEmail, rpPassword string
	reset := &cobra.Command{
		Use:   "reset-password",
		Short: "Set a user's password (signs out existing sessions)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			pw, generated, err := passwordOrRandom(rpPassword)
			if err != nil {
				return err
			}
			svc := usersvc.New(a)
			u, err := svc.FindByEmail(cmd.Context(), rpEmail)
			if err != nil {
				return fmt.Errorf("%s: %w", rpEmail, err)
			}
			if err := svc.SetPassword(cmd.Context(), u, pw); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "password updated for %s\n", u.Email)
			if generated {
				fmt.Fprintf(cmd.OutOrStdout(), "generated password: %s\n", pw)
			}
			return nil
		},
	}
	reset.Flags().StringVar(&rpEmail, "email", "", "email address (required)")
	reset.Flags().StringVar(&rpPassword, "password", "", "new password (min 8 chars; generated when omitted)")
	_ = reset.MarkFlagRequired("email")

	var prEmail string
	promote := &cobra.Command{
		Use:   "promote",
		Short: "Grant administrator rights to an existing user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			u, err := usersvc.New(a).FindByEmail(cmd.Context(), prEmail)
			if err != nil {
				return fmt.Errorf("%s: %w", prEmail, err)
			}
			if err := a.DB.WithContext(cmd.Context()).Model(&model.User{}).Where("id = ?", u.ID).
				UpdateColumn("is_admin", true).Error; err != nil {
				return err
			}
			auth.InvalidateUser(a, u.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "%s is now an administrator\n", u.Email)
			return nil
		},
	}
	promote.Flags().StringVar(&prEmail, "email", "", "email address (required)")
	_ = promote.MarkFlagRequired("email")

	cmd.AddCommand(create, reset, promote)
	return cmd
}

func passwordOrRandom(pw string) (string, bool, error) {
	if pw == "" {
		return settings.RandomToken(16), true, nil
	}
	if len(pw) < 8 {
		return "", false, errors.New("password must be at least 8 characters")
	}
	return pw, false, nil
}

func checkEmail(email string) error {
	at := strings.LastIndex(email, "@")
	if at < 1 || at == len(email)-1 || strings.ContainsAny(email, " \t\r\n") {
		return fmt.Errorf("invalid email %q", email)
	}
	return nil
}
