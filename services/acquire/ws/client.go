package ws

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

func (c *Client) readPump(hub *Hub) {
	defer func() {
		hub.RemoveClient(c.RoomID, c.PlayerID)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, messageBytes, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket错误 [房间:%s, 玩家:%s]: %v", c.RoomID, c.PlayerID, err)
			}
			break
		}

		log.Printf("收到消息 [房间:%s, 玩家:%s]: %s", c.RoomID, c.PlayerID, string(messageBytes))

		// 解析为原有格式的msgMap
		msgMap := make(map[string]interface{})
		msgMap["playerID"] = c.PlayerID
		if err := json.Unmarshal(messageBytes, &msgMap); err != nil {
			log.Printf("解析消息失败: %v", err)
			continue
		}

		// 转换为新的Message格式
		msg := Message{
			Type: func() string {
				if msgType, ok := msgMap["type"].(string); ok {
					return msgType
				}
				return "unknown"
			}(),
			Data: msgMap,
		}

		// 委托给service层处理
		ctx := context.Background()
		_, err = hub.handler.HandleMessage(ctx, c.RoomID, c.PlayerID, msg)
		if err != nil {
			// 发送错误消息给客户端
			errorMsg := map[string]interface{}{
				"type":    "error",
				"message": err.Error(),
			}
			if msgBytes, err := json.Marshal(errorMsg); err == nil {
				c.Send <- msgBytes
			}
			continue
		}
	}
	// 添加广播逻辑
	// hub.BroadcastToRoom(c.RoomID, msg)
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
