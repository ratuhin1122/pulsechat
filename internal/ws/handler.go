package ws

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/auth"
	"pulsechat/internal/chat"
	"pulsechat/internal/store/db"
)

const (
	MaxMessageSize = 8 * 1024
	MaxBodySize    = 4 * 1024
	sendBuffer     = 64
)

type Repository interface {
	chat.MembershipReader
	CreateMemberMessage(context.Context, db.CreateMemberMessageParams) (db.Message, error)
	ListRoomMemberIDs(context.Context, pgtype.UUID) ([]pgtype.UUID, error)
}

type Handler struct {
	ctx    context.Context
	repo   Repository
	tokens *auth.Tokens
	hub    *Hub
	logger *slog.Logger
	origin string
}

// An empty origin permits only the request's own scheme and host. Behind a TLS
// terminating proxy, configure the exact public origin explicitly.
func New(ctx context.Context, repo Repository, tokens *auth.Tokens, hub *Hub, logger *slog.Logger, origin string) (*Handler, error) {
	if err := validateOrigin(origin); err != nil {
		return nil, err
	}
	return &Handler{ctx: ctx, repo: repo, tokens: tokens, hub: hub, logger: logger, origin: origin}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /ws", h)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin, ok := h.checkOrigin(r)
	if !ok {
		http.Error(w, "origin forbidden", http.StatusForbidden)
		return
	}
	raw := accessToken(r)
	userID, err := h.tokens.Validate(raw)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var user pgtype.UUID
	if err := user.Scan(userID); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{origin}})
	if err != nil {
		return
	} // Accept already wrote the HTTP error response.
	defer conn.CloseNow()
	conn.SetReadLimit(MaxMessageSize)
	c := newClient(user.String(), sendBuffer)
	if !h.hub.register(c) {
		conn.Close(websocket.StatusGoingAway, "server shutting down")
		return
	}
	defer h.hub.unregister(c)
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	var loops sync.WaitGroup
	loops.Add(2)
	go func() {
		defer loops.Done()
		defer cancel()
		h.readLoop(ctx, conn, c, raw)
	}()
	go func() {
		defer loops.Done()
		defer cancel()
		h.writeLoop(ctx, conn, c, raw)
	}()
	<-ctx.Done()
	conn.CloseNow()
	loops.Wait()
}
