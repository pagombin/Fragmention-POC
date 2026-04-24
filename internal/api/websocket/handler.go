package websocket

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
)

// Serve upgrades an HTTP request to WebSocket and pipes hub frames into the
// client connection. The handler exits cleanly on client disconnect, context
// cancellation, or send failure.
func Serve(ctx context.Context, logger zerolog.Logger, hub *Hub, w http.ResponseWriter, r *http.Request) {
	opts := &websocket.AcceptOptions{
		InsecureSkipVerify: true, // same-origin enforced by middleware CSP; dev requests from other origins ok.
	}
	conn, err := websocket.Accept(w, r, opts)
	if err != nil {
		logger.Warn().Err(err).Msg("ws accept failed")
		return
	}
	defer func() { _ = conn.CloseNow() }()

	id, ch, cancel := hub.Subscribe()
	defer cancel()
	logger.Debug().Int64("sub_id", id).Str("hub", hub.name).Msg("ws subscriber connected")

	// Heartbeat loop: every 30s send a small ping frame so intermediaries
	// don't silently close the socket.
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.Context().Done():
			return
		case <-pingTicker.C:
			pctx, cncl := context.WithTimeout(ctx, 5*time.Second)
			if err := conn.Ping(pctx); err != nil {
				cncl()
				return
			}
			cncl()
		case frame, ok := <-ch:
			if !ok {
				return
			}
			wctx, cncl := context.WithTimeout(ctx, 5*time.Second)
			raw, err := EncodeFrame(frame)
			if err != nil {
				cncl()
				continue
			}
			if err := conn.Write(wctx, websocket.MessageText, raw); err != nil {
				cncl()
				return
			}
			cncl()
		}
	}
}
