package initial

import (
	"TaipeiCityDashboardBE/app/services"
	"TaipeiCityDashboardBE/logs"
	"time"

	"github.com/robfig/cron/v3"
)

// InitKnowledgeRefreshCron starts background cron jobs that periodically
// read disaster CSV files, convert them to natural language summaries,
// and upsert the results into the Qdrant city_knowledge collection.
//
// Only "real-time" data types are refreshed here.
// Static data (shelter stats, population) is loaded via POST /api/v1/qdrant/rebuild/city.
//
// To add a new real-time topic in the future, add a new c.AddFunc block below.
func InitKnowledgeRefreshCron() {
	logs.Info("Initializing knowledge refresh cron jobs...")

	c := cron.New(
		cron.WithLocation(time.Local),
		cron.WithSeconds(),
	)

	// Water level & rainfall: every 10 minutes
	if _, err := c.AddFunc("0 */10 * * * *", refreshFloodData); err != nil {
		logs.Error("Failed to add flood data refresh cron:", err)
	}

	// ER status: every 15 minutes
	if _, err := c.AddFunc("0 */15 * * * *", refreshERData); err != nil {
		logs.Error("Failed to add ER data refresh cron:", err)
	}

	// Earthquake: every hour (data updates are event-driven, hourly polling is sufficient)
	if _, err := c.AddFunc("0 0 * * * *", refreshEarthquakeData); err != nil {
		logs.Error("Failed to add earthquake data refresh cron:", err)
	}

	c.Start()
	logs.Info("Knowledge refresh cron jobs started.")

	// Run once immediately on startup so the collection is populated before the first cron tick
	go refreshFloodData()
	go refreshERData()
	go refreshEarthquakeData()
}

// refreshFloodData reads water_level.csv and rainfull.csv, then upserts summaries.
func refreshFloodData() {
	var chunks []services.CityKnowledgeChunk

	wl, err := services.GenerateWaterLevelChunks()
	if err != nil {
		logs.Error("Knowledge refresh: water level error:", err)
	} else {
		chunks = append(chunks, wl...)
	}

	rf, err := services.GenerateRainfallChunks()
	if err != nil {
		logs.Error("Knowledge refresh: rainfall error:", err)
	} else {
		chunks = append(chunks, rf...)
	}

	if len(chunks) == 0 {
		return
	}
	if err := services.UpsertCityKnowledgeChunks(chunks); err != nil {
		logs.Error("Knowledge refresh: upsert flood chunks error:", err)
	} else {
		logs.FInfo("Knowledge refresh: upserted %d flood chunks", len(chunks))
	}
}

// refreshERData reads er.csv and upserts ER status summaries.
func refreshERData() {
	chunks, err := services.GenerateERStatusChunks()
	if err != nil {
		logs.Error("Knowledge refresh: ER status error:", err)
		return
	}
	if err := services.UpsertCityKnowledgeChunks(chunks); err != nil {
		logs.Error("Knowledge refresh: upsert ER chunks error:", err)
	} else {
		logs.FInfo("Knowledge refresh: upserted %d ER chunks", len(chunks))
	}
}

// refreshEarthquakeData reads earthquake.csv and upserts a summary chunk.
func refreshEarthquakeData() {
	chunks, err := services.GenerateEarthquakeChunks()
	if err != nil {
		logs.Error("Knowledge refresh: earthquake error:", err)
		return
	}
	if err := services.UpsertCityKnowledgeChunks(chunks); err != nil {
		logs.Error("Knowledge refresh: upsert earthquake chunks error:", err)
	} else {
		logs.FInfo("Knowledge refresh: upserted %d earthquake chunks", len(chunks))
	}
}
