package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

// Hub 管理所有WebSocket连接
type Hub struct {
	rooms   map[string]*Room // roomID -> Room映射
	mu      sync.RWMutex     // 读写锁保证并发安全
	handler MessageHandler   // 消息处理接口
}

type Room struct {
	ID      string             // 房间ID
	Clients map[string]*Client // playerID -> Client映射
	mu      sync.RWMutex       // 读写锁保证并发安全
}

type Client struct {
	ID       string          // 客户端ID（格式：roomID_playerID）
	Conn     *websocket.Conn // WebSocket连接
	RoomID   string          // 所属房间ID
	PlayerID string          // 玩家ID
	Send     chan []byte     // 发送消息通道
}

type Message struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// MessageHandler 接口 - 将消息处理委托给service层
type MessageHandler interface {
	HandleMessage(ctx context.Context, roomID, playerID string, msg Message) (interface{}, error)
}

func NewHub(handler MessageHandler) *Hub {
	return &Hub{
		rooms:   make(map[string]*Room),
		handler: handler,
	}
}

func (h *Hub) AddClient(roomID, playerID string, conn *websocket.Conn) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, exists := h.rooms[roomID]
	if !exists {
		room = &Room{
			ID:      roomID,
			Clients: make(map[string]*Client),
		}
		h.rooms[roomID] = room
	}

	// 检查房间是否已满（最多6人）
	if len(room.Clients) >= 6 {
		return fmt.Errorf("房间已满")
	}

	client := &Client{
		ID:       fmt.Sprintf("%s_%s", roomID, playerID),
		Conn:     conn,
		RoomID:   roomID,
		PlayerID: playerID,
		Send:     make(chan []byte, 256),
	}

	room.mu.Lock()
	room.Clients[playerID] = client
	room.mu.Unlock()

	// 启动客户端读写协程
	go client.writePump()
	go client.readPump(h)

	return nil
}

func (h *Hub) RemoveClient(roomID, playerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, exists := h.rooms[roomID]
	if !exists {
		return
	}

	room.mu.Lock()
	if client, exists := room.Clients[playerID]; exists {
		close(client.Send)
		delete(room.Clients, playerID)
	}
	room.mu.Unlock()

	// 如果房间为空，删除房间
	if len(room.Clients) == 0 {
		delete(h.rooms, roomID)
	}
}

func (h *Hub) BroadcastToRoom(roomID string, message interface{}) {
	h.mu.RLock()
	room, exists := h.rooms[roomID]
	h.mu.RUnlock()

	if !exists {
		return
	}

	msgBytes, err := json.Marshal(message)
	if err != nil {
		log.Printf("序列化消息失败: %v", err)
		return
	}

	room.mu.RLock()
	for _, client := range room.Clients {
		select {
		case client.Send <- msgBytes:
		default:
			close(client.Send)
			delete(room.Clients, client.PlayerID)
		}
	}
	room.mu.RUnlock()
}

func (h *Hub) Run() {
	// Hub的运行逻辑，如果需要的话
	// 目前可以是空的，因为主要逻辑在client的readPump和writePump中
	select {} // 保持goroutine运行
}

// HandleWebSocket 处理WebSocket连接
func HandleWebSocket(hub *Hub, c *gin.Context) {
	// 升级连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "WebSocket升级失败"})
		return
	}

	// 获取参数
	roomID := c.Query("roomID")
	playerID := c.Query("userID")

	if roomID == "" || playerID == "" {
		conn.Close()
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必要参数"})
		return
	}

	// 创建客户端
	client := &Client{
		ID:       playerID,
		Conn:     conn,
		RoomID:   roomID,
		PlayerID: playerID,
		Send:     make(chan []byte, 256),
	}

	// 添加客户端到Hub
	if err := hub.AddClient(roomID, playerID, conn); err != nil {
		conn.Close()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 启动读写协程
	go client.writePump()
	go client.readPump(hub)
}

// GetAllRooms 获取所有房间信息
func (h *Hub) GetAllRooms() map[string]*Room {
	h.mu.RLock()
	defer h.mu.RUnlock()

	rooms := make(map[string]*Room)
	for roomID, room := range h.rooms {
		rooms[roomID] = room
	}
	return rooms
}

// GetRoomClients 获取指定房间的客户端列表
func (h *Hub) GetRoomClients(roomID string) map[string]*Client {
	h.mu.RLock()
	room, exists := h.rooms[roomID]
	h.mu.RUnlock()

	if !exists {
		return make(map[string]*Client)
	}

	room.mu.RLock()
	defer room.mu.RUnlock()

	clients := make(map[string]*Client)
	for playerID, client := range room.Clients {
		clients[playerID] = client
	}
	return clients
}
