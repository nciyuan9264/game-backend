// ws/ai_bridge.go
package ws

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

func CallPythonAI(actionType, roomID, playerID string, gameState map[string]interface{}) string {
	url := "http://localhost:8100/ai-decide"

	payload := map[string]interface{}{
		"action":    actionType, // 动作类型，比如 setTile、buyStock 等
		"roomID":    roomID,
		"playerID":  playerID,
		"gameState": gameState, // 传当前房间的游戏状态（你从 broadcastToRoom 或 Redis 拼出来的）
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println("❌ 调用 Python AI 服务失败:", err)
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Println("❌ Python AI 返回数据解析失败:", err)
		return ""
	}

	// 返回结果字段，例如 "8D"、{"Sackson": 3} 等
	val, ok := result["result"]
	if !ok {
		log.Println("❌ Python AI 没有返回 result 字段")
		return ""
	}
	resultStr, _ := json.Marshal(val) // 可以是字符串或对象，返回 JSON 字符串
	return string(resultStr)
}
