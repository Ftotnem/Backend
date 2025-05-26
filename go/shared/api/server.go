// go/shared/http/server.go
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type BaseServer struct {
	Router *mux.Router
	Server *http.Server
}

func NewBaseServer(addr string) *BaseServer {
	router := mux.NewRouter()

	// Apply common middleware
	router.Use(LoggingMiddleware)
	router.Use(CORSMiddleware)

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &BaseServer{
		Router: router,
		Server: server,
	}
}

func (bs *BaseServer) Start() error {
	return bs.Server.ListenAndServe()
}

func (bs *BaseServer) Shutdown(ctx context.Context) error {
	return bs.Server.Shutdown(ctx)
}
