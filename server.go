// main.go
//
// A small-but-realistic Go multiplayer server (~250 LOC):
// - HTTP: /health, /stats
// - WebSocket: /ws
// - Matchmaking: auto-pairs players into 2p rooms
// - Rooms: chat + simple "turn" game loop + state broadcasts
//
// Protocol (client -> server JSON):
//   {"type":"hello","name":"Evan"}
//   {"type":"chat","text":"hi"}
//   {"type":"move","move":"PLAY:7H"}   // your game can encode moves however you want
//   {"type":"leave"}
//
// Server -> client JSON:
//   {"type":"welcome","id":"c-...","msg":"..."}           (after connect)
//   {"type":"queued","pos":1}
//   {"type":"match","room":"r-...","you":"p1","opponent":"Alice"}
//   {"type":"chat","from":"Alice","text":"hi","ts":...}
//   {"type":"state","room":"r-...","turn":"p1","tick":5}
//   {"type":"error","msg":"..."}
//   {"type":"left","msg":"..."}

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// ---------- messages ----------

type InMsg struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
	Move string `json:"move,omitempty"`
}

type OutMsg struct {
	Type     string `json:"type"`
	Msg      string `json:"msg,omitempty"`
	ID       string `json:"id,omitempty"`
	Room     string `json:"room,omitempty"`
	You      string `json:"you,omitempty"`
	Opponent string `json:"opponent,omitempty"`
	From     string `json:"from,omitempty"`
	Text     string `json:"text,omitempty"`
	Turn     string `json:"turn,omitempty"`
	Tick     int64  `json:"tick,omitempty"`
	Pos      int    `json:"pos,omitempty"`
	TS       int64  `json:"ts,omitempty"`
}

// ---------- client ----------

type Client struct {
	id   string
	name string

	conn *websocket.Conn
	send chan []byte

	roomMu sync.RWMutex
	room   *Room

	closed atomic.Bool
}

func (c *Client) setRoom(r *Room) {
	c.roomMu.Lock()
	c.room = r
	c.roomMu.Unlock()
}

func (c *Client) getRoom() *Room {
	c.roomMu.RLock()
	defer c.roomMu.RUnlock()
	return c.room
}

func (c *Client) writeJSON(v any) {
	b, _ := json.Marshal(v)
	select {
	case c.send <- b:
	default:
		// backpressure: drop if client is slow
	}
}

func (c *Client) close() {
	if c.closed.Swap(true) {
		return
	}
	close(c.send)
	_ = c.conn.Close()
}

// ---------- room ----------

type Room struct {
	id string

	mu      sync.RWMutex
	players [2]*Client
	names   [2]string // display names
	turn    int       // 0 or 1
	tick    int64

	// inbound events (moves/chats) are funneled here for single-threaded room handling
	events chan roomEvent
	quit   chan struct{}
}

type roomEvent struct {
	from *Client
	msg  InMsg
}

func newRoom(p1, p2 *Client) *Room {
	r := &Room{
		id:      fmt.Sprintf("r-%08x", rand.Uint32()),
		players: [2]*Client{p1, p2},
		names:   [2]string{safeName(p1.name, "Player1"), safeName(p2.name, "Player2")},
		turn:    0,
		events:  make(chan roomEvent, 64),
		quit:    make(chan struct{}),
	}

	p1.setRoom(r)
	p2.setRoom(r)

	go r.loop()
	go r.ticker() // periodic state broadcast
	return r
}

func (r *Room) loop() {
	r.broadcast(OutMsg{Type: "state", Room: r.id, Turn: r.turnLabel(), Tick: atomic.LoadInt64(&r.tick)})

	for {
		select {
		case ev := <-r.events:
			switch strings.ToLower(ev.msg.Type) {
			case "chat":
				r.handleChat(ev.from, ev.msg)
			case "move":
				r.handleMove(ev.from, ev.msg)
			case "leave":
				r.handleLeave(ev.from, "player left")
				return
			default:
				ev.from.writeJSON(OutMsg{Type: "error", Msg: "unknown message type"})
			}

		case <-r.quit:
			return
		}
	}
}

func (r *Room) ticker() {
	t := time.NewTicker(750 * time.Millisecond)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			atomic.AddInt64(&r.tick, 1)
			r.broadcast(OutMsg{Type: "state", Room: r.id, Turn: r.turnLabel(), Tick: atomic.LoadInt64(&r.tick)})
		case <-r.quit:
			return
		}
	}
}

func (r *Room) turnLabel() string {
	if r.turn == 0 {
		return "p1"
	}
	return "p2"
}

func (r *Room) playerIndex(c *Client) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.players[0] == c {
		return 0
	}
	if r.players[1] == c {
		return 1
	}
	return -1
}

func (r *Room) otherName(c *Client) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.players[0] == c {
		return r.names[1]
	}
	if r.players[1] == c {
		return r.names[0]
	}
	return ""
}

func (r *Room) broadcast(o OutMsg) {
	o.TS = time.Now().UnixMilli()
	b, _ := json.Marshal(o)

	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.players {
		if p != nil {
			select {
			case p.send <- b:
			default:
			}
		}
	}
}

func (r *Room) handleChat(from *Client, m InMsg) {
	txt := strings.TrimSpace(m.Text)
	if txt == "" {
		return
	}
	r.broadcast(OutMsg{
		Type: "chat",
		From: safeName(from.name, "Player"),
		Text: txt,
	})
}

func (r *Room) handleMove(from *Client, m InMsg) {
	move := strings.TrimSpace(m.Move)
	if move == "" {
		from.writeJSON(OutMsg{Type: "error", Msg: "empty move"})
		return
	}

	idx := r.playerIndex(from)
	if idx < 0 {
		from.writeJSON(OutMsg{Type: "error", Msg: "not in this room"})
		return
	}

	// enforce "turns" just as an example
	if idx != r.turn {
		from.writeJSON(OutMsg{Type: "error", Msg: "not your turn"})
		return
	}

	// TODO: validate/execute move for your actual game here.
	// This demo just flips turn on any move.
	r.turn = 1 - r.turn

	r.broadcast(OutMsg{
		Type: "chat",
		From: "server",
		Text: fmt.Sprintf("%s played %q", safeName(from.name, "Player"), move),
	})
}

func (r *Room) handleLeave(from *Client, why string) {
	close(r.quit) // stop loops

	r.mu.Lock()
	defer r.mu.Unlock()

	// notify remaining player
	for i, p := range r.players {
		if p == nil {
			continue
		}
		if p == from {
			r.players[i] = nil
			continue
		}
		p.writeJSON(OutMsg{Type: "left", Msg: why})
		p.setRoom(nil)
	}
}

// ---------- matchmaking ----------

type Matchmaker struct {
	mu    sync.Mutex
	queue []*Client

	roomsMu sync.RWMutex
	rooms   map[string]*Room
}

func NewMatchmaker() *Matchmaker {
	return &Matchmaker{
		rooms: make(map[string]*Room),
	}
}

func (m *Matchmaker) enqueue(c *Client) (pos int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// avoid duplicates
	for _, q := range m.queue {
		if q == c {
			return 0
		}
	}

	m.queue = append(m.queue, c)
	pos = len(m.queue)

	if len(m.queue) >= 2 {
		p1 := m.queue[0]
		p2 := m.queue[1]
		m.queue = m.queue[2:]

		room := newRoom(p1, p2)

		m.roomsMu.Lock()
		m.rooms[room.id] = room
		m.roomsMu.Unlock()

		p1.writeJSON(OutMsg{Type: "match", Room: room.id, You: "p1", Opponent: safeName(p2.name, "Player2")})
		p2.writeJSON(OutMsg{Type: "match", Room: room.id, You: "p2", Opponent: safeName(p1.name, "Player1")})
	}
	return pos
}

func (m *Matchmaker) stats() (queued int, rooms int) {
	m.mu.Lock()
	queued = len(m.queue)
	m.mu.Unlock()

	m.roomsMu.RLock()
	rooms = len(m.rooms)
	m.roomsMu.RUnlock()
	return
}

// ---------- websocket plumbing ----------

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// For local dev; tighten for prod:
	CheckOrigin: func(r *http.Request) bool { return true },
}

var clientSeq uint64

func serveWS(mm *Matchmaker, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}

	id := fmt.Sprintf("c-%06d", atomic.AddUint64(&clientSeq, 1))
	c := &Client{
		id:   id,
		name: "",
		conn: conn,
		send: make(chan []byte, 64),
	}

	c.writeJSON(OutMsg{Type: "welcome", ID: c.id, Msg: "send {type:hello,name:...} to set your name"})

	go writerPump(c)
	readerPump(mm, c)
}

func readerPump(mm *Matchmaker, c *Client) {
	defer func() {
		// if client disconnects, notify room
		if r := c.getRoom(); r != nil {
			r.events <- roomEvent{from: c, msg: InMsg{Type: "leave"}}
		}
		c.close()
	}()

	c.conn.SetReadLimit(64 * 1024)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		var m InMsg
		if err := json.Unmarshal(data, &m); err != nil {
			c.writeJSON(OutMsg{Type: "error", Msg: "bad json"})
			continue
		}

		switch strings.ToLower(m.Type) {
		case "hello":
			c.name = strings.TrimSpace(m.Name)
			pos := mm.enqueue(c)
			if pos > 0 {
				c.writeJSON(OutMsg{Type: "queued", Pos: pos})
			}

		case "chat", "move", "leave":
			r := c.getRoom()
			if r == nil {
				c.writeJSON(OutMsg{Type: "error", Msg: "not in a match yet"})
				continue
			}
			select {
			case r.events <- roomEvent{from: c, msg: m}:
			default:
				c.writeJSON(OutMsg{Type: "error", Msg: "room busy"})
			}

		default:
			c.writeJSON(OutMsg{Type: "error", Msg: "unknown type"})
		}
	}
}

func writerPump(c *Client) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ---------- http handlers ----------

func health(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("ok"))
}

func stats(mm *Matchmaker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, rooms := mm.stats()
		out := map[string]any{
			"queued": q,
			"rooms":  rooms,
			"ts":     time.Now().UnixMilli(),
		}
		_ = json.NewEncoder(w).Encode(out)
	}
}

// ---------- helpers ----------

func safeName(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	if len(s) > 24 {
		return s[:24]
	}
	return s
}

// ---------- main ----------

func main() {
	rand.Seed(time.Now().UnixNano())

	mm := NewMatchmaker()

	http.HandleFunc("/health", health)
	http.HandleFunc("/stats", stats(mm))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) { serveWS(mm, w, r) })

	log.Println("listening on :8080 (GET /health, /stats, WS /ws)")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
