package controllers

import (
	"TaipeiCityDashboardBE/app/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

/*
TriggerQdrantRebuild is a test endpoint to manually trigger the Qdrant rebuild process.
POST /api/v1/qdrant/rebuild
*/
func TriggerQdrantRebuild(c *gin.Context) {
    // 同步執行，並取得回傳的資料
    data, err := services.RebuildQdrantPublicCollection()
    if err != nil {
        // 如果是「正在重建中」的錯誤，回傳 409 Conflict 會更語意化
        if err.Error() == "qdrant rebuild is already in progress" {
            c.JSON(http.StatusConflict, gin.H{"status": "error", "message": err.Error()})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "status":  "success",
        "message": "Synchronous rebuild complete (up to implemented steps).",
        "data":    data,
    })
}

// TriggerCityKnowledgeRebuild rebuilds the city_knowledge Qdrant collection for a given topic.
// POST /api/v1/qdrant/rebuild/city
// Body: { "topic": "disaster" }
func TriggerCityKnowledgeRebuild(c *gin.Context) {
	var body struct {
		Topic string `json:"topic" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "topic is required",
		})
		return
	}

	topic := strings.TrimSpace(body.Topic)
	chunks, err := services.RebuildCityKnowledgeCollection(topic)
	if err != nil {
		if strings.Contains(err.Error(), "already in progress") {
			c.JSON(http.StatusConflict, gin.H{"status": "error", "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "City knowledge rebuild complete.",
		"data": gin.H{
			"topic":          topic,
			"indexed_points": len(chunks),
		},
	})
}