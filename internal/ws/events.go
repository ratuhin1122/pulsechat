package ws

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"pulsechat/internal/tracing"
)

// Event is the internal transport envelope, never the browser protocol.
type Event struct {
	TraceContext propagation.MapCarrier `json:"trace_context,omitempty"`
	RoomID       string                 `json:"room_id,omitempty"`
	UserIDs      []string               `json:"user_ids,omitempty"`
	Payload      json.RawMessage        `json:"payload,omitempty"`
	Presence     *presenceEvent         `json:"presence,omitempty"`
}

type Publisher interface {
	Publish(context.Context, Event) error
}

func NewHubWithPublisher(publisher Publisher) *Hub {
	hub := NewHub()
	hub.publisher = publisher
	return hub
}

func (h *Hub) PublishRoom(ctx context.Context, roomID string, userIDs []string, payload []byte) error {
	return h.publish(ctx, Event{RoomID: roomID, UserIDs: userIDs, Payload: payload})
}

func (h *Hub) PublishToUsers(ctx context.Context, userIDs []string, payload []byte) error {
	return h.publish(ctx, Event{UserIDs: userIDs, Payload: payload})
}

func (h *Hub) publish(ctx context.Context, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if h.publisher != nil {
		return h.publisher.Publish(ctx, event)
	}
	h.deliver(event)
	return nil
}

// deliver never publishes, so Redis subscribers cannot create a feedback loop.
func (h *Hub) deliver(event Event) {
	if event.RoomID != "" {
		h.Broadcast(event.RoomID, event.UserIDs, event.Payload)
	} else {
		h.BroadcastToUsers(event.UserIDs, event.Payload)
	}
}

func (h *Handler) DeliverEvent(ctx context.Context, event Event) (deliverErr error) {
	ctx = tracing.Extract(ctx, event.TraceContext)
	ctx, span := h.telemetry.Start(ctx, "websocket.notify.deliver", trace.WithSpanKind(trace.SpanKindConsumer))
	defer func() { tracing.End(span, deliverErr) }()
	if event.Presence == nil {
		h.hub.deliver(event)
		return nil
	}
	ids, err := h.presenceRecipients(ctx, event.Presence.UserID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(event.Presence)
	if err != nil {
		return err
	}
	h.hub.BroadcastToUsers(ids, data)
	return nil
}
