// FIXME(thaJeztah): remove once we are a module; the go:build directive prevents go from downgrading language version to go1.16:
//go:build go1.23

package system

import (
	"context"
	"sync"

	"github.com/docker/docker/api/server/router"
	"resenje.org/singleflight"
)

// EventsCloser is an interface for routers that can close event connections.
type EventsCloser interface {
	CloseEventConnections()
}

// systemRouter provides information about the Docker system overall.
// It gathers information about host, daemon and container events.
type systemRouter struct {
	backend  Backend
	cluster  ClusterBackend
	routes   []router.Route
	builder  BuildBackend
	features func() map[string]bool

	// collectSystemInfo is a single-flight for the /info endpoint,
	// unique per API version (as different API versions may return
	// a different API response).
	collectSystemInfo singleflight.Group[string, *infoResponse]

	// eventConnections tracks active /events connections by storing their cancel functions
	eventConnectionsMu sync.Mutex
	eventConnections   []context.CancelFunc
}

// NewRouter initializes a new system router
func NewRouter(b Backend, c ClusterBackend, builder BuildBackend, features func() map[string]bool) router.Router {
	r := &systemRouter{
		backend:          b,
		cluster:          c,
		builder:          builder,
		features:         features,
		eventConnections: make([]context.CancelFunc, 0),
	}

	r.routes = []router.Route{
		router.NewOptionsRoute("/{anyroute:.*}", optionsHandler),
		router.NewGetRoute("/_ping", r.pingHandler),
		router.NewHeadRoute("/_ping", r.pingHandler),
		router.NewGetRoute("/events", r.getEvents),
		router.NewGetRoute("/info", r.getInfo),
		router.NewGetRoute("/version", r.getVersion),
		router.NewGetRoute("/system/df", r.getDiskUsage),
		router.NewPostRoute("/auth", r.postAuth),
	}

	return r
}

// Routes returns all the API routes dedicated to the docker system
func (s *systemRouter) Routes() []router.Route {
	return s.routes
}

func (s *systemRouter) registerEventConnection(cancel context.CancelFunc) {
	s.eventConnectionsMu.Lock()
	defer s.eventConnectionsMu.Unlock()
	s.eventConnections = append(s.eventConnections, cancel)
}

// CloseEventConnections cancels all active /events connections.
// This is called after all containers have been shut down during daemon shutdown,
// allowing orchestrators to receive all container shutdown events before the
// connections are closed.
func (s *systemRouter) CloseEventConnections() {
	s.eventConnectionsMu.Lock()
	cancels := s.eventConnections
	s.eventConnections = make([]context.CancelFunc, 0)
	s.eventConnectionsMu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}
