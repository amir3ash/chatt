package authz

import (
	"context"

	"github.com/gofiber/fiber/v2"
)

// New creates a new middleware handler
func NewAuthMiddleware() fiber.Handler {

	// Return new handler
	return func(c *fiber.Ctx) error {

		c.Locals(UserIdCtxKey, "343")

		return c.Next()
	}
}

// return authenticated user. if not found returns ""
func UserIdFromCtx(ctx context.Context) string {
	u, ok := ctx.Value(UserIdCtxKey).(string)
	if !ok {
		return ""
	}
	return u
}

const UserIdCtxKey = "userId"