package services

import (
	"TaipeiCityDashboardBE/app/models"
	"TaipeiCityDashboardBE/global"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// isCityKnowledgeRebuilding prevents concurrent full rebuilds.
var isCityKnowledgeRebuilding atomic.Bool

// CityKnowledgeChunk represents a single knowledge unit to be embedded into Qdrant.
type CityKnowledgeChunk struct {
	Topic     string // e.g. "disaster"
	SubTopic  string // e.g. "shelter", "water_level", "er_status"
	City      string // "TP" | "NTP" | ""（空白表示跨市）
	District  string // e.g. "中山區"；空白表示全市摘要
	Content   string // 自然語言摘要，供 LLM 讀取
	Source    string // CSV 檔名
	UpdatedAt string // RFC3339，讓 LLM 知道資料新鮮度
}

// chunkID generates a deterministic uint64 point ID.
// Same inputs → same ID, so re-upserting naturally overwrites existing points.
func chunkID(topic, subTopic, city, district string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(topic + "|" + subTopic + "|" + city + "|" + district))
	return h.Sum64()
}

// disasterDataPath returns the base path for disaster CSV files.
func disasterDataPath() string {
	p := os.Getenv("DISASTER_DATA_PATH")
	if p == "" {
		p = "./disaster-data/韌性防災資料"
	}
	return p
}

// RebuildCityKnowledgeCollection reads static/semi-static data for the given topic
// and upserts them into the city_knowledge Qdrant collection.
// Dynamic data (water_level, rainfall, er_status, earthquake) is handled by the cron job.
func RebuildCityKnowledgeCollection(topic string) ([]CityKnowledgeChunk, error) {
	if !isCityKnowledgeRebuilding.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("city knowledge rebuild is already in progress")
	}
	defer isCityKnowledgeRebuilding.Store(false)

	log.Printf("Starting city_knowledge rebuild for topic: %s", topic)
	ctx := context.Background()

	var chunks []CityKnowledgeChunk

	switch topic {
	case "disaster":
		if sc, err := readShelterStats(); err != nil {
			log.Printf("Warning: shelter stats: %v", err)
		} else {
			chunks = append(chunks, sc...)
		}
		if pc, err := readPopulationStats(); err != nil {
			log.Printf("Warning: population stats: %v", err)
		} else {
			chunks = append(chunks, pc...)
		}
	default:
		return nil, fmt.Errorf("topic '%s' is not yet supported", topic)
	}

	if len(chunks) == 0 {
		return chunks, fmt.Errorf("no chunks generated for topic '%s'", topic)
	}

	points, vectorSize, err := chunksToPoints(chunks)
	if err != nil {
		return chunks, fmt.Errorf("vector generation error: %w", err)
	}

	collectionName := global.Qdrant.CityCollection
	if err := createCollectionIfNotExists(ctx, collectionName, uint64(vectorSize)); err != nil {
		return chunks, fmt.Errorf("ensure collection error: %w", err)
	}
	if err := upsertPoints(ctx, collectionName, points); err != nil {
		return chunks, fmt.Errorf("upsert error: %w", err)
	}

	log.Printf("city_knowledge rebuild complete: %d chunks upserted for topic '%s'", len(points), topic)
	return chunks, nil
}

// UpsertCityKnowledgeChunks vectorizes and upserts a batch of chunks.
// Called by the cron job to update dynamic data without touching the rest of the collection.
func UpsertCityKnowledgeChunks(chunks []CityKnowledgeChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	ctx := context.Background()

	points, vectorSize, err := chunksToPoints(chunks)
	if err != nil {
		return fmt.Errorf("vector generation error: %w", err)
	}

	collectionName := global.Qdrant.CityCollection
	if err := createCollectionIfNotExists(ctx, collectionName, uint64(vectorSize)); err != nil {
		return fmt.Errorf("ensure collection error: %w", err)
	}
	if err := upsertPoints(ctx, collectionName, points); err != nil {
		return fmt.Errorf("upsert error: %w", err)
	}

	log.Printf("Upserted %d dynamic chunks into city_knowledge", len(points))
	return nil
}

// chunksToPoints vectorizes CityKnowledgeChunks into qdrantPoints.
func chunksToPoints(chunks []CityKnowledgeChunk) ([]qdrantPoint, int, error) {
	var points []qdrantPoint
	var vectorSize int

	for _, chunk := range chunks {
		if strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		vector, err := models.GenVector(chunk.Content)
		if err != nil {
			return nil, 0, fmt.Errorf("GenVector [%s/%s/%s]: %w",
				chunk.Topic, chunk.SubTopic, chunk.District, err)
		}
		if vectorSize == 0 {
			vectorSize = len(vector)
		}
		points = append(points, qdrantPoint{
			ID:     chunkID(chunk.Topic, chunk.SubTopic, chunk.City, chunk.District),
			Vector: vector,
			Payload: map[string]interface{}{
				"topic":      chunk.Topic,
				"sub_topic":  chunk.SubTopic,
				"city":       chunk.City,
				"district":   chunk.District,
				"content":    chunk.Content,
				"source":     chunk.Source,
				"updated_at": chunk.UpdatedAt,
			},
		})
	}
	return points, vectorSize, nil
}

// createCollectionIfNotExists creates the Qdrant collection only if it doesn't exist.
// Unlike recreateCollection, this does NOT delete existing data.
func createCollectionIfNotExists(ctx context.Context, collectionName string, vectorSize uint64) error {
	cfg := global.Qdrant

	// PUT is idempotent in newer Qdrant versions but older versions return error if exists.
	// We use GET first to check existence.
	checkURL := fmt.Sprintf("%s/collections/%s", cfg.Url, collectionName)
	checkReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if cfg.ApiKey != "" {
		checkReq.Header.Set("api-key", cfg.ApiKey)
	}
	checkResp, err := http.DefaultClient.Do(checkReq)
	if err != nil {
		return fmt.Errorf("check collection error: %w", err)
	}
	checkResp.Body.Close()

	if checkResp.StatusCode == http.StatusOK {
		log.Printf("Collection '%s' already exists, skipping creation.", collectionName)
		return nil
	}

	// Create collection
	log.Printf("Creating collection '%s' with vector size %d", collectionName, vectorSize)
	createBody, _ := json.Marshal(map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     vectorSize,
			"distance": "Cosine",
		},
	})
	createURL := fmt.Sprintf("%s/collections/%s?timeout=30", cfg.Url, collectionName)
	createReq, err := http.NewRequestWithContext(ctx, http.MethodPut, createURL, bytes.NewBuffer(createBody))
	if err != nil {
		return fmt.Errorf("create collection request error: %w", err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	if cfg.ApiKey != "" {
		createReq.Header.Set("api-key", cfg.ApiKey)
	}

	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		return fmt.Errorf("create collection error: %w", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(createResp.Body)
		return fmt.Errorf("create collection returned %s: %s", createResp.Status, string(b))
	}
	return nil
}

// -- Static CSV Readers --

// readShelterStats reads shelter_stats.csv → one chunk per district.
func readShelterStats() ([]CityKnowledgeChunk, error) {
	filePath := filepath.Join(disasterDataPath(), "收容所", "避難所", "shelter_stats.csv")
	now := time.Now().Format(time.RFC3339)

	records, err := readCSV(filePath)
	if err != nil {
		return nil, err
	}
	idx := csvIndex(records[0])

	var chunks []CityKnowledgeChunk
	for _, row := range records[1:] {
		if len(row) <= maxIdx(idx) {
			continue
		}
		city := row[idx["city"]]
		district := row[idx["district"]]
		cityName := cityDisplayName(city)

		content := fmt.Sprintf(
			"【%s%s避難所資訊】總容量 %s 人，其中 %s 所具無障礙/弱勢友善設施。"+
				"依災害類型支援數量：水災 %s 所、地震 %s 所、土石流 %s 所、海嘯 %s 所。",
			cityName, district,
			row[idx["total_capacity"]],
			row[idx["weak_friendly_count"]],
			row[idx["flood_count"]],
			row[idx["earthquake_count"]],
			row[idx["landslide_count"]],
			row[idx["tsunami_count"]],
		)
		chunks = append(chunks, CityKnowledgeChunk{
			Topic: "disaster", SubTopic: "shelter",
			City: city, District: district,
			Content: content, Source: "shelter_stats.csv", UpdatedAt: now,
		})
	}
	return chunks, nil
}

// readPopulationStats reads population_stats.csv → one chunk per district.
func readPopulationStats() ([]CityKnowledgeChunk, error) {
	filePath := filepath.Join(disasterDataPath(), "收容所", "總人口", "population_stats.csv")
	now := time.Now().Format(time.RFC3339)

	records, err := readCSV(filePath)
	if err != nil {
		return nil, err
	}
	idx := csvIndex(records[0])

	var chunks []CityKnowledgeChunk
	for _, row := range records[1:] {
		if len(row) <= maxIdx(idx) {
			continue
		}
		city := row[idx["city"]]
		district := row[idx["district"]]
		if district == "總計" || strings.TrimSpace(district) == "" {
			continue
		}
		cityName := cityDisplayName(city)

		content := fmt.Sprintf(
			"【%s%s人口統計】總人口 %s 人（男 %s 人、女 %s 人），共 %s 戶。",
			cityName, district,
			row[idx["total_population"]],
			row[idx["male"]],
			row[idx["female"]],
			row[idx["households"]],
		)
		chunks = append(chunks, CityKnowledgeChunk{
			Topic: "disaster", SubTopic: "population",
			City: city, District: district,
			Content: content, Source: "population_stats.csv", UpdatedAt: now,
		})
	}
	return chunks, nil
}

// -- Dynamic Summary Generators (called by cron job) --

// GenerateWaterLevelChunks reads water_level.csv → summary chunk per city.
func GenerateWaterLevelChunks() ([]CityKnowledgeChunk, error) {
	filePath := filepath.Join(disasterDataPath(), "水災", "河川", "water_level.csv")
	now := time.Now().Format(time.RFC3339)

	records, err := readCSV(filePath)
	if err != nil {
		return nil, err
	}
	idx := csvIndex(records[0])

	type stationLine struct{ text string }
	linesTP := []stationLine{}
	linesNTP := []stationLine{}

	for _, row := range records[1:] {
		if len(row) <= maxIdx(idx) {
			continue
		}
		name := row[idx["station_name"]]
		levelStr := row[idx["water_level"]]
		a1Str := row[idx["alert_level_1"]]
		a2Str := row[idx["alert_level_2"]]
		a3Str := row[idx["alert_level_3"]]
		addr := row[idx["address"]]

		level, _ := strconv.ParseFloat(levelStr, 64)
		a1, _ := strconv.ParseFloat(a1Str, 64)
		a2, _ := strconv.ParseFloat(a2Str, 64)
		a3, _ := strconv.ParseFloat(a3Str, 64)

		status := "正常"
		if a3 > 0 && level >= a3 {
			status = "⚠️ 超過三級警戒"
		} else if a2 > 0 && level >= a2 {
			status = "⚠️ 超過二級警戒"
		} else if a1 > 0 && level >= a1 {
			status = "⚠️ 超過一級警戒"
		}

		line := stationLine{fmt.Sprintf("%s：%.2fm（%s）", name, level, status)}
		if strings.Contains(addr, "臺北市") || strings.Contains(addr, "台北市") {
			linesTP = append(linesTP, line)
		} else {
			linesNTP = append(linesNTP, line)
		}
	}

	build := func(lines []stationLine, cityName, cityCode string) CityKnowledgeChunk {
		parts := make([]string, len(lines))
		for i, l := range lines {
			parts[i] = l.text
		}
		return CityKnowledgeChunk{
			Topic: "disaster", SubTopic: "water_level", City: cityCode,
			Content:   fmt.Sprintf("【%s河川水位 %s】%s", cityName, now[:16], strings.Join(parts, "；")),
			Source:    "water_level.csv", UpdatedAt: now,
		}
	}

	var chunks []CityKnowledgeChunk
	if len(linesTP) > 0 {
		chunks = append(chunks, build(linesTP, "台北市", "TP"))
	}
	if len(linesNTP) > 0 {
		chunks = append(chunks, build(linesNTP, "新北市", "NTP"))
	}
	return chunks, nil
}

// GenerateERStatusChunks reads er.csv → summary chunk per city.
func GenerateERStatusChunks() ([]CityKnowledgeChunk, error) {
	filePath := filepath.Join(disasterDataPath(), "醫療資源", "er.csv")
	now := time.Now().Format(time.RFC3339)

	records, err := readCSV(filePath)
	if err != nil {
		return nil, err
	}
	idx := csvIndex(records[0])

	type hospLine struct{ text string }
	linesTP := []hospLine{}
	linesNTP := []hospLine{}

	for _, row := range records[1:] {
		if len(row) <= maxIdx(idx) {
			continue
		}
		city := row[idx["city"]]
		name := row[idx["hospital_name"]]
		fullMark := ""
		if row[idx["is_full_119"]] == "1" {
			fullMark = "🔴滿床 "
		}
		text := fmt.Sprintf("%s%s（候診%s人、待住院%s人、待加護%s人）",
			fullMark, name,
			row[idx["wait_see"]],
			row[idx["wait_admission"]],
			row[idx["wait_icu"]],
		)
		line := hospLine{text}
		if city == "臺北市" || city == "台北市" {
			linesTP = append(linesTP, line)
		} else {
			linesNTP = append(linesNTP, line)
		}
	}

	build := func(lines []hospLine, cityName, cityCode string) CityKnowledgeChunk {
		parts := make([]string, len(lines))
		for i, l := range lines {
			parts[i] = l.text
		}
		return CityKnowledgeChunk{
			Topic: "disaster", SubTopic: "er_status", City: cityCode,
			Content:   fmt.Sprintf("【%s急診狀況 %s】%s", cityName, now[:16], strings.Join(parts, "；")),
			Source:    "er.csv", UpdatedAt: now,
		}
	}

	var chunks []CityKnowledgeChunk
	if len(linesTP) > 0 {
		chunks = append(chunks, build(linesTP, "台北市", "TP"))
	}
	if len(linesNTP) > 0 {
		chunks = append(chunks, build(linesNTP, "新北市", "NTP"))
	}
	return chunks, nil
}

// GenerateRainfallChunks reads rainfull.csv → summary chunk per city.
func GenerateRainfallChunks() ([]CityKnowledgeChunk, error) {
	filePath := filepath.Join(disasterDataPath(), "水災", "降雨", "rainfull.csv")
	now := time.Now().Format(time.RFC3339)

	records, err := readCSV(filePath)
	if err != nil {
		return nil, err
	}
	idx := csvIndex(records[0])

	type stLine struct{ text string }
	linesTP := []stLine{}
	linesNTP := []stLine{}

	for _, row := range records[1:] {
		if len(row) <= maxIdx(idx) {
			continue
		}
		city := row[idx["city"]]
		text := fmt.Sprintf("%s（%s）時雨量%smm/24hr累積%smm",
			row[idx["station_name"]], row[idx["district"]],
			row[idx["rain_1hr"]], row[idx["rain_24hr"]],
		)
		line := stLine{text}
		if city == "臺北市" || city == "台北市" {
			linesTP = append(linesTP, line)
		} else {
			linesNTP = append(linesNTP, line)
		}
	}

	build := func(lines []stLine, cityName, cityCode string) CityKnowledgeChunk {
		parts := make([]string, len(lines))
		for i, l := range lines {
			parts[i] = l.text
		}
		return CityKnowledgeChunk{
			Topic: "disaster", SubTopic: "rainfall", City: cityCode,
			Content:   fmt.Sprintf("【%s降雨狀況 %s】%s", cityName, now[:16], strings.Join(parts, "；")),
			Source:    "rainfull.csv", UpdatedAt: now,
		}
	}

	var chunks []CityKnowledgeChunk
	if len(linesTP) > 0 {
		chunks = append(chunks, build(linesTP, "台北市", "TP"))
	}
	if len(linesNTP) > 0 {
		chunks = append(chunks, build(linesNTP, "新北市", "NTP"))
	}
	return chunks, nil
}

// GenerateEarthquakeChunks reads earthquake.csv → one summary chunk.
func GenerateEarthquakeChunks() ([]CityKnowledgeChunk, error) {
	filePath := filepath.Join(disasterDataPath(), "地震", "earthquake.csv")
	now := time.Now().Format(time.RFC3339)

	records, err := readCSV(filePath)
	if err != nil {
		return nil, err
	}
	idx := csvIndex(records[0])

	parts := []string{}
	for i, row := range records[1:] {
		if i >= 10 || len(row) <= maxIdx(idx) {
			break
		}
		dateStr := row[idx["update_time"]]
		if len(dateStr) >= 10 {
			dateStr = dateStr[:10]
		}
		parts = append(parts, fmt.Sprintf("%s %s站 強度%s",
			dateStr, row[idx["station_name"]], row[idx["intensity"]]))
	}

	return []CityKnowledgeChunk{{
		Topic: "disaster", SubTopic: "earthquake",
		Content:   fmt.Sprintf("【近期地震紀錄 %s】%s", now[:16], strings.Join(parts, "；")),
		Source:    "earthquake.csv", UpdatedAt: now,
	}}, nil
}

// -- Utility helpers --

func readCSV(filePath string) ([][]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv %s: %w", filePath, err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("csv %s has no data rows", filePath)
	}
	return records, nil
}

// csvIndex builds a column-name → index map from the header row.
func csvIndex(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.TrimSpace(h)] = i
	}
	return m
}

// maxIdx returns the maximum index value in the map (used for bounds checking).
func maxIdx(m map[string]int) int {
	max := 0
	for _, v := range m {
		if v > max {
			max = v
		}
	}
	return max
}

func cityDisplayName(code string) string {
	switch code {
	case "TP":
		return "台北市"
	case "NTP":
		return "新北市"
	default:
		return code
	}
}
