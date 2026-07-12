package server

import (
	"log"
	"sync"

	"github.com/jhh78/ws-server/config"
)

// Hub manages clients, areas, and channels for scoped JSON delivery.
type Hub struct {
	area    config.AreaLimits
	channel config.ChannelLimits

	mu       sync.RWMutex
	clients  map[string]*Client
	areas    map[string]map[*Client]struct{}
	channels map[string]map[*Client]struct{} // "kind:id"
}

func NewHub(area config.AreaLimits, ch config.ChannelLimits) *Hub {
	return &Hub{
		area:     area,
		channel:  ch,
		clients:  make(map[string]*Client),
		areas:    make(map[string]map[*Client]struct{}),
		channels: make(map[string]map[*Client]struct{}),
	}
}

func channelKey(kind, id string) string {
	return kind + ":" + id
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c.id] = c
	log.Printf("client registered id=%s addr=%s total=%d", c.id, c.remote, len(h.clients))
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c.id]; !ok {
		return
	}
	delete(h.clients, c.id)
	for area, set := range h.areas {
		if _, in := set[c]; in {
			delete(set, c)
			if len(set) == 0 {
				delete(h.areas, area)
			}
		}
	}
	for ch, set := range h.channels {
		if _, in := set[c]; in {
			delete(set, c)
			if len(set) == 0 {
				delete(h.channels, ch)
			}
		}
	}
	close(c.send)
	log.Printf("client unregistered id=%s total=%d", c.id, len(h.clients))
}

func (h *Hub) getByID(id string) *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.clients[id]
}

func (h *Hub) joinArea(c *Client, area string) string {
	if area == "" {
		return "target (area id) is required"
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	set, ok := h.areas[area]
	if !ok {
		if h.area.MaxAreas > 0 && len(h.areas) >= h.area.MaxAreas {
			return "max areas reached"
		}
		set = make(map[*Client]struct{})
		h.areas[area] = set
	}
	if _, already := set[c]; already {
		return ""
	}
	if h.area.MaxClientsPerArea > 0 && len(set) >= h.area.MaxClientsPerArea {
		return "area is full"
	}
	set[c] = struct{}{}
	c.areas[area] = struct{}{}
	return ""
}

func (h *Hub) leaveArea(c *Client, area string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.leaveAreaLocked(c, area)
}

func (h *Hub) leaveAreaLocked(c *Client, area string) {
	set, ok := h.areas[area]
	if !ok {
		return
	}
	delete(set, c)
	if len(set) == 0 {
		delete(h.areas, area)
	}
	delete(c.areas, area)
}

func (h *Hub) inArea(c *Client, area string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := c.areas[area]
	return ok
}

func (h *Hub) broadcastArea(area string, data []byte, exclude *Client) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	set, ok := h.areas[area]
	if !ok {
		return 0
	}
	return deliver(set, data, exclude)
}

func (h *Hub) joinChannel(c *Client, kind, id string) string {
	if kind == "" {
		return "channel_kind is required (party|guild|whisper|custom)"
	}
	if id == "" {
		return "target (channel id) is required"
	}
	key := channelKey(kind, id)

	h.mu.Lock()
	defer h.mu.Unlock()

	set, ok := h.channels[key]
	if !ok {
		if h.channel.MaxChannels > 0 && len(h.channels) >= h.channel.MaxChannels {
			return "max channels reached"
		}
		set = make(map[*Client]struct{})
		h.channels[key] = set
	}
	if _, already := set[c]; already {
		return ""
	}
	if h.channel.MaxClientsPerChannel > 0 && len(set) >= h.channel.MaxClientsPerChannel {
		return "channel is full"
	}
	set[c] = struct{}{}
	c.channels[key] = struct{}{}
	return ""
}

func (h *Hub) leaveChannel(c *Client, kind, id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.leaveChannelLocked(c, channelKey(kind, id))
}

func (h *Hub) leaveChannelLocked(c *Client, key string) {
	set, ok := h.channels[key]
	if !ok {
		return
	}
	delete(set, c)
	if len(set) == 0 {
		delete(h.channels, key)
	}
	delete(c.channels, key)
}

func (h *Hub) inChannel(c *Client, kind, id string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := c.channels[channelKey(kind, id)]
	return ok
}

func (h *Hub) broadcastChannel(kind, id string, data []byte, exclude *Client) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	set, ok := h.channels[channelKey(kind, id)]
	if !ok {
		return 0
	}
	return deliver(set, data, exclude)
}

func (h *Hub) leaveAll(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for area := range c.areas {
		h.leaveAreaLocked(c, area)
	}
	for key := range c.channels {
		h.leaveChannelLocked(c, key)
	}
}

func deliver(set map[*Client]struct{}, data []byte, exclude *Client) int {
	n := 0
	for c := range set {
		if c == exclude {
			continue
		}
		if c.trySend(data) {
			n++
		}
	}
	return n
}

func (h *Hub) AreaCount(area string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.areas[area])
}

func (h *Hub) ChannelCount(kind, id string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.channels[channelKey(kind, id)])
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
