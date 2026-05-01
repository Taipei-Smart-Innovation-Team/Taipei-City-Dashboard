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

	// All disaster data: every 15 minutes
	if _, err := c.AddFunc("0 */15 * * * *", func() { refreshAllChunks("disaster") }); err != nil {
		logs.Error("Failed to add knowledge refresh cron:", err)
	}

	c.Start()
	logs.Info("Knowledge refresh cron started.")

	// Run once immediately on startup
	go refreshAllChunks("disaster")
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
