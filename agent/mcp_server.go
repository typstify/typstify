package agent

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"looz.ws/typstify/version"
)

const (
	ServerName = "Typstify"
)

type McpServer struct {
	mcpServer  *mcpsdk.Server
	httpServer *http.Server
	serverAddr string
	port       int
	started    atomic.Bool
	running    atomic.Bool
}

type McpToolProvider interface {
	RegisterTools(s *McpServer) error
}

func NewMcpServer() *McpServer {
	return &McpServer{
		mcpServer: mcpsdk.NewServer(
			&mcpsdk.Implementation{
				Name:    "typstify-mcp-server",
				Title:   ServerName,
				Version: version.BinVersion,
			},
			&mcpsdk.ServerOptions{
				Instructions: "Use this server when you need to use exported tools and resources of the Typstify editor environment. Show preview result before editing the typst files if possible.",
				Logger:       slog.Default(),
				PageSize:     mcpsdk.DefaultPageSize,
				GetSessionID: func() string {
					return uuid.NewString()
				},
				Capabilities: &mcpsdk.ServerCapabilities{
					Logging:   &mcpsdk.LoggingCapabilities{},
					Prompts:   &mcpsdk.PromptCapabilities{},
					Resources: &mcpsdk.ResourceCapabilities{},
					Tools:     &mcpsdk.ToolCapabilities{},
				},
			},
		),
	}
}

func (s *McpServer) Run() error {
	if s.mcpServer == nil {
		return errors.New("mcp server is not initialized")
	}

	sseHandler := mcpsdk.NewStreamableHTTPHandler(func(request *http.Request) *mcpsdk.Server {
		return s.mcpServer
	}, &mcpsdk.StreamableHTTPOptions{})

	serveMux := http.NewServeMux()
	serveMux.Handle("/", recoveryMiddleware(sseHandler))

	if s.started.CompareAndSwap(false, true) {
		address := "127.0.0.1:0" // use random port.
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			panic(err)
		}

		netAddr := listener.Addr().(*net.TCPAddr)
		s.serverAddr = netAddr.IP.String()
		s.port = netAddr.Port

		log.Printf("mcp server is running at %s:%d", s.serverAddr, s.port)

		s.httpServer = &http.Server{
			Addr:         address,
			WriteTimeout: time.Second * 10,
			Handler:      serveMux,
		}

		go func() {
			if err := s.httpServer.Serve(listener); err != nil {
				s.started.Store(false)
				listener.Close()
				log.Println("mcp server down: ", err)
			}

			log.Println("mcp server is down.")
		}()
	}

	return nil
}

func (s *McpServer) Addr() (string, int) {
	return s.serverAddr, s.port
}

func (s *McpServer) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *McpServer) RegisterToolProvider(provider McpToolProvider) {
	provider.RegisterTools(s)
}

func (s *McpServer) AddResource(resource *mcpsdk.Resource, handler mcpsdk.ResourceHandler) error {
	if s.mcpServer == nil {
		return errors.New("mcp server is not initialized")
	}

	s.mcpServer.AddResource(resource, handler)
	return nil
}

func (s *McpServer) RemoveTools(names ...string) error {
	if s.mcpServer == nil {
		return errors.New("mcp server is not initialized")
	}

	s.mcpServer.RemoveTools(names...)
	return nil
}

func (s *McpServer) RemoveResources(uris ...string) error {
	if s.mcpServer == nil {
		return errors.New("mcp server is not initialized")
	}

	s.mcpServer.RemoveResources(uris...)
	return nil
}

func AddMcpTool[In, Out any](s *McpServer, tool *mcpsdk.Tool, handler mcpsdk.ToolHandlerFor[In, Out]) error {
	if s.mcpServer == nil {
		return errors.New("mcp server is not initialized")
	}

	mcpsdk.AddTool(s.mcpServer, tool, handler)
	return nil
}

func recoveryMiddleware(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Println("recovering mcp server, error:", err)

				w.WriteHeader(http.StatusInternalServerError)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write([]byte("Server Panics"))
				return
			}
		}()

		h.ServeHTTP(w, r)
	}
}
