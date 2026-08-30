package transport

import (
	"github.com/go-chi/chi/v5"
)

// Routes mounts this handler's endpoints onto r, behind the auth middleware.
//
// Every route is inside the group, without exception. There is no public send
// endpoint and no unauthenticated read: a mailbox is one user's, and enqueue is
// an internal capability other modules reach through an adapter at the
// composition root rather than over HTTP.
//
// The paths are written out in full against the router the module is handed,
// the way modules/user writes /users/me, rather than through a
// chi.Router.Route("/notifications", …) subrouter. The subrouter would register
// the list endpoint as "/notifications/" — chi mounts a subrouter's "/" under
// the prefix with the separator kept — and the route table in the blueprint,
// the swagger annotations and cmd/api's TestRouteTable all say
// "/api/v1/notifications". One spelling everywhere is worth more than the
// grouping.
//
// This IS an id-bearing route table, which is new in this repository: E16 was
// closed in modules/user by deleting the id-bearing routes, and that option
// does not exist here — marking one notification read requires naming it. What
// stands in its place is that {id} names a row and never a recipient. The
// recipient is the Principal, the query is scoped by it (R5), and an id that
// resolves to somebody else's row is answered exactly as an id that resolves to
// nothing. See Handler.MarkRead.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.authMW)

		r.Get("/notifications", h.List())
		r.Post("/notifications/read-all", h.MarkAllRead())
		r.Get("/notifications/preferences", h.GetPreferences())
		r.Put("/notifications/preferences", h.UpdatePreferences())

		// The only route with an id, registered last so it reads as the
		// exception it is. It cannot shadow the four above it: it is one
		// segment deeper and ends in /read, and chi matches a static segment
		// before a parameter in any case — so "preferences" is never taken for
		// an id, whatever the registration order.
		r.Post("/notifications/{id}/read", h.MarkRead())
	})
}
