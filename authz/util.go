package authz

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Creates a new middleware handler
func NewFiberAuthMiddleware() fiber.Handler {

	// Return new handler
	return func(c *fiber.Ctx) error {
		userId := c.Cookies("userId", "343")
		userId = strings.Clone(userId)

		c.Locals(UserIdCtxKey, userId)

		return c.Next()
	}
}

func NewHttpAuthMiddleware(next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			err := recover()
			if err != nil {
				slog.Error("recovering in http middleware", "err", "err")
			}
		}()

		userIdCookie, err := r.Cookie("userId")
		userId := "343"
		if err == nil {
			userId = userIdCookie.Value
		}

		ctx := context.WithValue(r.Context(), UserIdCtxKey, userId)
		req := r.WithContext(ctx)

		next.ServeHTTP(w, req)
	})
}

// return authenticated user. if not found returns ""
func UserIdFromCtx(ctx context.Context) string {
	u, ok := ctx.Value(UserIdCtxKey).(string)
	if !ok {
		return ""
	}
	return u
}

type userIdType string

var UserIdCtxKey = userIdType("userId")
