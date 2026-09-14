package realtime

import (
	"github.com/gorilla/websocket"
	"sync"
)

type Event struct {
	Type           string `json:"type"`
	OrganizationID string `json:"organizationId"`
	OccurredAt     string `json:"occurredAt"`
	Payload        any    `json:"payload"`
}
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[*websocket.Conn]struct{}
}

func NewHub() *Hub { return &Hub{rooms: map[string]map[*websocket.Conn]struct{}{}} }
func (h *Hub) Add(org string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[org] == nil {
		h.rooms[org] = map[*websocket.Conn]struct{}{}
	}
	h.rooms[org][c] = struct{}{}
}
func (h *Hub) Remove(org string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms[org], c)
	if len(h.rooms[org]) == 0 {
		delete(h.rooms, org)
	}
}
func (h *Hub) Publish(org string, event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.rooms[org] {
		_ = c.WriteJSON(event)
	}
}
