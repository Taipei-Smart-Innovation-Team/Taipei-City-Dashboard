package initial

import (
	"TaipeiCityDashboardBE/app/services"
	"TaipeiCityDashboardBE/logs"
	"time"

	"github.com/robfig/cron/v3"
)

// InitKnowledgeRefreshCron starts a single background cron job that refreshes
// all city_knowledge chunks (static + real-time) every 15 minutes.
//
// To add a new topic in the future, add a new c.AddFunc block below.
func InitKnowledgeRefreshCron() {
	logs.Info("Initializing knowledge refresh cron jobs...")

	c := cron.New(
		cron.WithLocation(time.Local),
		cron.WithSeconds(),
	)

	// food_safety: every 15 minutes (即時醫療資料)
	if _, err := c.AddFunc("0 */15 * * * *", func() { refreshAllChunks("food_safety") }); err != nil {
		logs.Error("Failed to add food_safety cron:", err)
	}
	// transportation: every 5 minutes (YouBike 即時)
	if _, err := c.AddFunc("0 */5 * * * *", func() { refreshAllChunks("transportation") }); err != nil {
		logs.Error("Failed to add transportation cron:", err)
	}
	// population: every hour (統計資料，更新頻率低)
	if _, err := c.AddFunc("0 0 * * * *", func() { refreshAllChunks("population") }); err != nil {
		logs.Error("Failed to add population cron:", err)
	}

	c.Start()
	logs.Info("Knowledge refresh cron started.")

	// Run once immediately on startup
	go refreshAllChunks("food_safety")
	go refreshAllChunks("transportation")
	go refreshAllChunks("population")
}

// refreshAllChunks generates all chunks for the given topic and upserts them into Qdrant.
func refreshAllChunks(topic string) {
	chunks, err := services.GenerateAllChunks(topic)
	if err != nil {
		logs.FError("Knowledge refresh [%s]: %v", topic, err)
		return
	}
	if err := services.UpsertCityKnowledgeChunks(chunks); err != nil {
		logs.FError("Knowledge refresh [%s]: upsert error: %v", topic, err)
	} else {
		logs.FInfo("Knowledge refresh [%s]: upserted %d chunks", topic, len(chunks))
	}
}
