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
	ListUserRoomIDs(context.Context, pgtype.UUID) ([]pgtype.UUID, error)
	GetUserMessage(context.Context, db.GetUserMessageParams) (db.Message, error)
	GetLatestUserMessage(context.Context, pgtype.UUID) (db.Message, error)
	ListReplayMessages(context.Context, db.ListReplayMessagesParams) ([]db.Message, error)
	ListAttachmentsForMessage(context.Context, pgtype.UUID) ([]db.Attachment, error)
}

type Handler struct {
	ctx        context.Context
	repo       Repository
	tokens     *auth.Tokens
	hub        *Hub
	logger     *slog.Logger
	origin     string
	presence   Presence
	presenceMu sync.Mutex
	messageMu  sync.Mutex
}

// An empty origin permits only the request's own scheme and host. Behind a TLS
// terminating proxy, configure the exact public origin explicitly.
func New(ctx context.Context, repo Repository, tokens *auth.Tokens, hub *Hub, presence Presence, logger *slog.Logger, origin string) (*Handler, error) {
	if err := validateOrigin(origin); err != nil {
		return nil, err
	}
	return &Handler{ctx: ctx, repo: repo, tokens: tokens, hub: hub, presence: presence, logger: logger, origin: origin}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /ws", h)
	mux.Handle("GET /presence", h.tokens.Middleware(http.HandlerFunc(h.listPresence)))
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
	cursor, ok := h.replayCursor(w, r, user)
	if !ok {
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{origin}})
	if err != nil {
		return
	} // Accept already wrote the HTTP error response.
	defer conn.CloseNow()
	conn.SetReadLimit(MaxMessageSize)
	c := newClient(user.String(), sendBuffer)
	plan, err := h.registerForReplay(h.ctx, c, cursor, user)
	if err != nil {
		h.logger.ErrorContext(h.ctx, "prepare websocket replay", "error", err)
		conn.Close(websocket.StatusInternalError, "replay unavailable")
		return
	}
	defer h.hub.unregister(c)
	if err := h.changePresence(h.ctx, c, true); err != nil {
		h.logger.ErrorContext(h.ctx, "connect presence", "error", err)
		conn.Close(websocket.StatusInternalError, "presence unavailable")
		return
	}
	defer func() {
		// changePresence starts its timeout after acquiring the transition lock.
		cleanupCtx := context.WithoutCancel(h.ctx)
		if err := h.changePresence(cleanupCtx, c, false); err != nil {
			h.logger.ErrorContext(cleanupCtx, "disconnect presence", "error", err)
		}
	}()
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
		h.writeLoop(ctx, conn, c, raw, plan)
	}()
	<-ctx.Done()
	conn.CloseNow()
	loops.Wait()
}
