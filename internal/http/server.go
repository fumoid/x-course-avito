package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/metric"

	api "github.com/fumoid/x-course-avito/internal/generated"
	"github.com/fumoid/x-course-avito/internal/http/domainerror"
	"github.com/fumoid/x-course-avito/internal/http/health"
	"github.com/fumoid/x-course-avito/internal/http/middleware"
	createhttp "github.com/fumoid/x-course-avito/internal/http/trip/create"
	finishhttp "github.com/fumoid/x-course-avito/internal/http/trip/finish"
	gethttp "github.com/fumoid/x-course-avito/internal/http/trip/get"
)

type Server struct {
	create *createhttp.Handler
	get    *gethttp.Handler
	finish *finishhttp.Handler
	health *health.Handler
}

func New(
	createHandler *createhttp.Handler,
	getHandler *gethttp.Handler,
	finishHandler *finishhttp.Handler,
	healthHandler *health.Handler,
) *Server {
	return &Server{
		create: createHandler,
		get:    getHandler,
		finish: finishHandler,
		health: healthHandler,
	}
}

func (s *Server) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	s.create.CreateTrip(w, r, params)
}

func (s *Server) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.get.GetTrip(w, r, tripID)
}

func (s *Server) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.finish.FinishTrip(w, r, tripID)
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	s.health.Health(w, r)
}

func (s *Server) Ready(w http.ResponseWriter, r *http.Request) {
	s.health.Ready(w, r)
}

func NewRouter(
	server *Server,
	log *slog.Logger,
	active metric.Int64UpDownCounter,
) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.Recover(log))
	router.Use(middleware.Tracing(router))
	router.Use(middleware.Logging(log, router))
	router.Use(middleware.Metrics(active, router))

	api.HandlerWithOptions(server, api.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			domainerror.WriteInvalidRequest(w, r, err)
		},
	})
	return router
}

func NewHTTPServer(addr string, handler http.Handler, read, readHeader, write, idle time.Duration) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       read,
		ReadHeaderTimeout: readHeader,
		WriteTimeout:      write,
		IdleTimeout:       idle,
	}
}
