package mcp

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"time"

	errors "github.com/Laisky/errors/v2"
	logSDK "github.com/Laisky/go-utils/v5/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/laisky-blog-graphql/internal/mcp/askuser"
	"github.com/Laisky/laisky-blog-graphql/internal/mcp/tools"
	"github.com/Laisky/laisky-blog-graphql/library"
	"github.com/Laisky/laisky-blog-graphql/library/billing/oneapi"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/laisky-blog-graphql/library/log"
	searchlib "github.com/Laisky/laisky-blog-graphql/library/search"
	mcp "github.com/mark3labs/mcp-go/mcp"
	srv "github.com/mark3labs/mcp-go/server"
)

type ctxKey string

const (
	keyAuthorization ctxKey = "authorization"
	httpLogBodyLimit        = 4096
)

// Server wraps the MCP server state for the HTTP transport.
type Server struct {
	handler   http.Handler
	logger    logSDK.Logger
	webSearch *tools.WebSearchTool
	webFetch  *tools.WebFetchTool
	askUser   *tools.AskUserTool
}

// NewServer constructs an MCP HTTP server.
// searchProvider enables the web_search tool when not nil.
// askUserService enables the ask_user tool when not nil.
// rdb enables the web_fetch tool when not nil.
// logger overrides the default logger when provided.
// It returns the configured server or an error if no capability is available.
func NewServer(searchProvider searchlib.Provider, askUserService *askuser.Service, rdb *rlibs.DB, logger logSDK.Logger) (*Server, error) {
	if searchProvider == nil && askUserService == nil && rdb == nil {
		return nil, errors.New("at least one MCP capability must be enabled")
	}
	if logger == nil {
		logger = log.Logger
	}

	hooks := newMCPHooks(logger.Named("mcp_hooks"))

	mcpServer := srv.NewMCPServer(
		"LAISKY MCP SERVER",
		"1.0.0",
		srv.WithToolCapabilities(true),
		srv.WithInstructions("Use web_search for Google Programmable Search queries and web_fetch to retrieve dynamic web pages."),
		srv.WithRecovery(),
		srv.WithHooks(hooks),
	)

	streamable := srv.NewStreamableHTTPServer(
		mcpServer,
		srv.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return context.WithValue(ctx, keyAuthorization, r.Header.Get("Authorization"))
		}),
	)

	serverLogger := logger.Named("mcp")

	s := &Server{
		handler: withHTTPLogging(streamable, serverLogger.Named("http")),
		logger:  serverLogger,
	}

	apiKeyProvider := func(ctx context.Context) string {
		authHeader, _ := ctx.Value(keyAuthorization).(string)
		return extractAPIKey(authHeader)
	}

	if searchProvider != nil {
		webSearchTool, err := tools.NewWebSearchTool(
			searchProvider,
			serverLogger.Named("web_search"),
			apiKeyProvider,
			oneapi.CheckUserExternalBilling,
			nil,
		)
		if err != nil {
			return nil, errors.Wrap(err, "init web_search tool")
		}
		s.webSearch = webSearchTool
		mcpServer.AddTool(webSearchTool.Definition(), s.handleWebSearch)
	}

	if rdb != nil {
		webFetchTool, err := tools.NewWebFetchTool(
			rdb,
			serverLogger.Named("web_fetch"),
			apiKeyProvider,
			oneapi.CheckUserExternalBilling,
			searchlib.FetchDynamicURLContent,
			nil,
		)
		if err != nil {
			return nil, errors.Wrap(err, "init web_fetch tool")
		}
		s.webFetch = webFetchTool
		mcpServer.AddTool(webFetchTool.Definition(), s.handleWebFetch)
	}

	if askUserService != nil {
		headerProvider := func(ctx context.Context) string {
			authHeader, _ := ctx.Value(keyAuthorization).(string)
			return authHeader
		}

		askUserTool, err := tools.NewAskUserTool(
			askUserService,
			serverLogger.Named("ask_user"),
			headerProvider,
			askuser.ParseAuthorizationContext,
			0,
		)
		if err != nil {
			return nil, errors.Wrap(err, "init ask_user tool")
		}
		s.askUser = askUserTool
		mcpServer.AddTool(askUserTool.Definition(), s.handleAskUser)
	}

	return s, nil
}

// Handler returns the HTTP handler that should be mounted to serve MCP traffic.
func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) handleWebSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.webSearch == nil {
		return mcp.NewToolResultError("web search is not configured"), nil
	}

	return s.webSearch.Handle(ctx, req)
}

// handleWebFetch executes the web_fetch MCP tool. The context carries request metadata,
// and the request supplies the target URL. It returns a structured response when the
// fetch succeeds or a tool error when processing fails.
func (s *Server) handleWebFetch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.webFetch == nil {
		return mcp.NewToolResultError("web fetch is not configured"), nil
	}

	return s.webFetch.Handle(ctx, req)
}

func extractAPIKey(authHeader string) string {
	return library.StripBearerPrefix(authHeader)
}

func (s *Server) handleAskUser(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.askUser == nil {
		return mcp.NewToolResultError("ask_user tool is not available"), nil
	}

	return s.askUser.Handle(ctx, req)
}

func newMCPHooks(logger logSDK.Logger) *srv.Hooks {
	if logger == nil {
		return nil
	}

	hooks := &srv.Hooks{}

	hooks.AddBeforeAny(func(ctx context.Context, id any, method mcp.MCPMethod, message any) {
		fields := hookLogFields(ctx, id, method)
		if message != nil {
			fields = append(fields, zap.Any("request", message))
		}
		logger.Debug("mcp request received", fields...)
	})

	hooks.AddOnSuccess(func(ctx context.Context, id any, method mcp.MCPMethod, message any, result any) {
		fields := hookLogFields(ctx, id, method)
		if result != nil {
			fields = append(fields, zap.Any("response", result))
		}
		logger.Info("mcp request succeeded", fields...)
	})

	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		fields := hookLogFields(ctx, id, method)
		if message != nil {
			fields = append(fields, zap.Any("request", message))
		}
		fields = append(fields, zap.Error(err))
		logger.Error("mcp request failed", fields...)
	})

	hooks.AddOnRegisterSession(func(ctx context.Context, session srv.ClientSession) {
		logger.Info("mcp session registered", zap.String("session_id", session.SessionID()))
	})

	hooks.AddOnUnregisterSession(func(ctx context.Context, session srv.ClientSession) {
		logger.Info("mcp session unregistered", zap.String("session_id", session.SessionID()))
	})

	return hooks
}

func hookLogFields(ctx context.Context, id any, method mcp.MCPMethod) []zap.Field {
	fields := []zap.Field{
		zap.Any("request_id", id),
		zap.String("method", string(method)),
	}

	if session := srv.ClientSessionFromContext(ctx); session != nil {
		fields = append(fields, zap.String("session_id", session.SessionID()))
	}

	return fields
}

func withHTTPLogging(next http.Handler, logger logSDK.Logger) http.Handler {
	if next == nil {
		return nil
	}
	if logger == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startAt := time.Now()
		body, truncated, err := readAndRestoreRequestBody(r, httpLogBodyLimit)
		if err != nil {
			logger.Error("read request body", zap.Error(err))
		}

		logger.Debug("incoming http request",
			zap.String("method", r.Method),
			zap.String("url", r.URL.String()),
			zap.String("body", body),
			zap.Bool("body_truncated", truncated),
			zap.String("remote_addr", r.RemoteAddr),
		)

		lrw := newLoggingResponseWriter(w, httpLogBodyLimit)
		next.ServeHTTP(lrw, r)

		status := lrw.Status()
		respBody, respTruncated := lrw.Body()
		logger.Debug("outgoing http response",
			zap.String("method", r.Method),
			zap.String("url", r.URL.String()),
			zap.Int("status", status),
			zap.String("body", respBody),
			zap.Bool("body_truncated", respTruncated),
			zap.String("remote_addr", r.RemoteAddr),
			zap.Duration("cost", time.Since(startAt)),
		)
	})
}

func readAndRestoreRequestBody(r *http.Request, limit int) (string, bool, error) {
	if r.Body == nil {
		return "", false, nil
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		return "", false, err
	}
	if err := r.Body.Close(); err != nil {
		return "", false, err
	}

	r.Body = io.NopCloser(bytes.NewReader(data))
	truncatedBody, truncated := truncateForLog(data, limit)
	return truncatedBody, truncated, nil
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status    int
	buffer    bytes.Buffer
	truncated bool
	bodyLimit int
}

func newLoggingResponseWriter(w http.ResponseWriter, limit int) *loggingResponseWriter {
	return &loggingResponseWriter{
		ResponseWriter: w,
		bodyLimit:      limit,
	}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.status = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	if lrw.status == 0 {
		lrw.status = http.StatusOK
	}

	if lrw.buffer.Len() < lrw.bodyLimit {
		remaining := lrw.bodyLimit - lrw.buffer.Len()
		if len(b) > remaining {
			lrw.buffer.Write(b[:remaining])
			lrw.truncated = true
		} else {
			lrw.buffer.Write(b)
		}
	} else {
		lrw.truncated = true
	}

	return lrw.ResponseWriter.Write(b)
}

func (lrw *loggingResponseWriter) Status() int {
	if lrw.status == 0 {
		return http.StatusOK
	}
	return lrw.status
}

func (lrw *loggingResponseWriter) Body() (string, bool) {
	return lrw.buffer.String(), lrw.truncated
}

func (lrw *loggingResponseWriter) Flush() {
	if flusher, ok := lrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (lrw *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := lrw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("hijacker not supported")
}

func (lrw *loggingResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := lrw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func truncateForLog(data []byte, limit int) (string, bool) {
	if len(data) <= limit {
		return string(data), false
	}
	return string(data[:limit]), true
}
