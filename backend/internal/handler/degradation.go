package handler

import (
	"encoding/json"
	"net/http"
)

// 企业为何需要：优雅降级防级联故障。当非关键依赖不可用时，返回降级响应而非 500，
// 让客户端能继续提供部分功能。这是 Netflix Hystrix 的核心设计理念。

// DegradedResponse represents a response when a dependency is unavailable.
// Enterprise: Graceful degradation returns useful partial data instead of hard errors,
// preventing cascade failures when non-critical dependencies are down.
type DegradedResponse struct {
	Data     interface{} `json:"data"`
	Degraded bool        `json:"degraded"`
	Message  string      `json:"message,omitempty"`
}

// WriteDegradedJSON writes a degraded response with the given status, data, and message.
func WriteDegradedJSON(w http.ResponseWriter, status int, data interface{}, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(DegradedResponse{
		Data:     data,
		Degraded: true,
		Message:  message,
	})
}
