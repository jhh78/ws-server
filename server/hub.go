package server

import (
	"log"
	"sync"

	"github.com/jhh78/ws-server/config"
)

// Hub 는 클라이언트·에리어·채널 멤버십과 범위 브로드캐스트를 관리합니다.
//
// 내부 채널 키 형식: "{channel_kind}:{target}" (예: party:party-42).
type Hub struct {
	area    config.AreaLimits
	channel config.ChannelLimits

	mu       sync.RWMutex
	clients  map[string]*Client
	areas    map[string]map[*Client]struct{}
	channels map[string]map[*Client]struct{} // "kind:id"
}

// NewHub 는 빈 멤버십 맵을 가진 Hub 를 생성합니다.
//
// Parameters:
//   - area: 에리어 한도
//   - ch: 채널 한도
//
// Returns:
//   - *Hub: 새 허브
func NewHub(area config.AreaLimits, ch config.ChannelLimits) *Hub {
	return &Hub{
		area:     area,
		channel:  ch,
		clients:  make(map[string]*Client),
		areas:    make(map[string]map[*Client]struct{}),
		channels: make(map[string]map[*Client]struct{}),
	}
}

// channelKey 는 채널 맵 키를 만듭니다.
//
// Parameters:
//   - kind: channel_kind
//   - id: target
//
// Returns:
//   - string: "kind:id"
func channelKey(kind, id string) string {
	return kind + ":" + id
}

// register 는 클라이언트를 전역 맵에 등록합니다.
//
// Parameters:
//   - c: 등록할 클라이언트
func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c.id] = c
	log.Printf("client registered id=%s addr=%s total=%d", c.id, c.remote, len(h.clients))
}

// unregister 는 클라이언트를 제거하고 멤버십을 정리한 뒤 send 채널을 닫습니다.
//
// Parameters:
//   - c: 해제할 클라이언트 (중복 호출 시 no-op)
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

// getByID 는 client_id 로 연결을 조회합니다 (whisper 용).
//
// Parameters:
//   - id: 서버 발급 client_id
//
// Returns:
//   - *Client: 없거나 오프라인이면 nil
func (h *Hub) getByID(id string) *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.clients[id]
}

// joinArea 는 클라이언트를 에리어에 가입시킵니다.
//
// Parameters:
//   - c: 클라이언트
//   - area: 에리어 ID (target)
//
// Returns:
//   - string: 오류 메시지 (성공 시 빈 문자열)
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

// leaveArea 는 에리어 멤버십을 해제합니다.
//
// Parameters:
//   - c: 클라이언트
//   - area: 에리어 ID
func (h *Hub) leaveArea(c *Client, area string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.leaveAreaLocked(c, area)
}

// leaveAreaLocked 는 이미 락을 잡은 상태에서 에리어 탈퇴를 수행합니다.
//
// Parameters:
//   - c: 클라이언트
//   - area: 에리어 ID
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

// inArea 는 클라이언트가 해당 에리어 멤버인지 확인합니다.
//
// Parameters:
//   - c: 클라이언트
//   - area: 에리어 ID
//
// Returns:
//   - bool: 멤버이면 true
func (h *Hub) inArea(c *Client, area string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := c.areas[area]
	return ok
}

// broadcastArea 는 에리어 멤버에게 data 를 전달합니다 (exclude 제외).
//
// Parameters:
//   - area: 에리어 ID
//   - data: 인코딩된 프레임
//   - exclude: 제외할 클라이언트 (nil 가능)
//
// Returns:
//   - int: 큐에 넣은 수신자 수
func (h *Hub) broadcastArea(area string, data []byte, exclude *Client) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	set, ok := h.areas[area]
	if !ok {
		return 0
	}
	return deliver(set, data, exclude)
}

// joinChannel 은 kind+id 채널에 가입시킵니다.
//
// Parameters:
//   - c: 클라이언트
//   - kind: channel_kind
//   - id: channel target
//
// Returns:
//   - string: 오류 메시지 (성공 시 "")
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

// leaveChannel 은 채널 멤버십을 해제합니다.
//
// Parameters:
//   - c: 클라이언트
//   - kind: channel_kind
//   - id: target
func (h *Hub) leaveChannel(c *Client, kind, id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.leaveChannelLocked(c, channelKey(kind, id))
}

// leaveChannelLocked 는 락 보유 상태에서 채널 탈퇴를 수행합니다.
//
// Parameters:
//   - c: 클라이언트
//   - key: channelKey 결과
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

// inChannel 은 채널 멤버십 여부를 반환합니다.
//
// Parameters:
//   - c: 클라이언트
//   - kind: channel_kind
//   - id: target
//
// Returns:
//   - bool: 멤버이면 true
func (h *Hub) inChannel(c *Client, kind, id string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := c.channels[channelKey(kind, id)]
	return ok
}

// broadcastChannel 은 채널 멤버에게 data 를 전달합니다.
//
// Parameters:
//   - kind: channel_kind
//   - id: target
//   - data: 프레임 바이트
//   - exclude: 제외 클라이언트
//
// Returns:
//   - int: 전달 성공 수
func (h *Hub) broadcastChannel(kind, id string, data []byte, exclude *Client) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	set, ok := h.channels[channelKey(kind, id)]
	if !ok {
		return 0
	}
	return deliver(set, data, exclude)
}

// leaveAll 은 클라이언트의 모든 에리어·채널 멤버십을 해제합니다 (연결 종료 시).
//
// Parameters:
//   - c: 클라이언트
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

// deliver 는 집합의 각 클라이언트에 trySend 합니다.
//
// Parameters:
//   - set: 수신자 집합
//   - data: 프레임
//   - exclude: 제외 (동일 포인터)
//
// Returns:
//   - int: 큐 적재 성공 수
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

// AreaCount 는 에리어 현재 인원을 반환합니다.
//
// Parameters:
//   - area: 에리어 ID
//
// Returns:
//   - int: 멤버 수
func (h *Hub) AreaCount(area string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.areas[area])
}

// ChannelCount 는 채널 현재 인원을 반환합니다.
//
// Parameters:
//   - kind: channel_kind
//   - id: target
//
// Returns:
//   - int: 멤버 수
func (h *Hub) ChannelCount(kind, id string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.channels[channelKey(kind, id)])
}

// ClientCount 는 등록된 전체 연결 수를 반환합니다.
//
// Returns:
//   - int: clients 맵 크기
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
