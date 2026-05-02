package services

import (
	"TaipeiCityDashboardBE/app/models"
	"TaipeiCityDashboardBE/global"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// isCityKnowledgeRebuilding prevents concurrent full rebuilds.
var isCityKnowledgeRebuilding atomic.Bool

// CityKnowledgeChunk represents a single knowledge unit to be embedded into Qdrant.
type CityKnowledgeChunk struct {
	Topic     string // e.g. "disaster"
	SubTopic  string // e.g. "er_status"
	City      string // "TP" | "NTP" | ""
	District  string // e.g. "板橋區"；空白表示全市摘要
	Content   string // 自然語言摘要，供 LLM 讀取
	Source    string // 資料來源描述
	UpdatedAt string // RFC3339
}

// chunkID generates a deterministic uint64 point ID.
// Same inputs → same ID, so re-upserting naturally overwrites existing points.
func chunkID(topic, subTopic, city, district string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(topic + "|" + subTopic + "|" + city + "|" + district))
	return h.Sum64()
}

// GenerateAllChunks generates all chunks for a given topic.
// Used by the unified cron job to refresh everything in one pass.
func GenerateAllChunks(topic string) ([]CityKnowledgeChunk, error) {
	var chunks []CityKnowledgeChunk

	switch topic {
	case "food_safety":
		if er, err := GenerateERStatusChunks(); err != nil {
			log.Printf("Warning: ER status: %v", err)
		} else {
			chunks = append(chunks, er...)
		}
		if dc, err := GenerateDiarrheaChunks(); err != nil {
			log.Printf("Warning: diarrhea count: %v", err)
		} else {
			chunks = append(chunks, dc...)
		}
	case "transportation":
		if ub, err := GenerateUBikeChunks(); err != nil {
			log.Printf("Warning: ubike: %v", err)
		} else {
			chunks = append(chunks, ub...)
		}
		if bn, err := GenerateBikeNetworkChunks(); err != nil {
			log.Printf("Warning: bike_network: %v", err)
		} else {
			chunks = append(chunks, bn...)
		}
		if bs, err := GenerateBusInfoChunks(); err != nil {
			log.Printf("Warning: bus_info: %v", err)
		} else {
			chunks = append(chunks, bs...)
		}
	case "population":
		if pa, err := GeneratePopulationAgeChunks(); err != nil {
			log.Printf("Warning: population_age: %v", err)
		} else {
			chunks = append(chunks, pa...)
		}
		if dr, err := GenerateDependencyRatioChunks(); err != nil {
			log.Printf("Warning: dependency_ratio: %v", err)
		} else {
			chunks = append(chunks, dr...)
		}
		if em, err := GenerateEmploymentAgeChunks(); err != nil {
			log.Printf("Warning: employment_age: %v", err)
		} else {
			chunks = append(chunks, em...)
		}
	default:
		return nil, fmt.Errorf("topic '%s' is not yet supported", topic)
	}

	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks generated for topic '%s'", topic)
	}
	return chunks, nil
}

// UpsertCityKnowledgeChunks vectorizes and upserts a batch of chunks into Qdrant.
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

	log.Printf("Upserted %d chunks into city_knowledge", len(points))
	return nil
}

// RebuildCityKnowledgeCollection is a manual trigger (POST /api/v1/qdrant/rebuild/city).
func RebuildCityKnowledgeCollection(topic string) ([]CityKnowledgeChunk, error) {
	if !isCityKnowledgeRebuilding.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("city knowledge rebuild is already in progress")
	}
	defer isCityKnowledgeRebuilding.Store(false)

	chunks, err := GenerateAllChunks(topic)
	if err != nil {
		return nil, err
	}
	if err := UpsertCityKnowledgeChunks(chunks); err != nil {
		return chunks, err
	}
	return chunks, nil
}

// -- Data source: food_safety_medical (PostgreSQL) --

type foodSafetyMedicalRow struct {
	City          string     `gorm:"column:city"`
	Town          string     `gorm:"column:town"`
	HospitalName  string     `gorm:"column:hospital_name"`
	IsFull119     bool       `gorm:"column:is_full_119"`
	WaitSee       int        `gorm:"column:wait_see"`
	WaitAdmission int        `gorm:"column:wait_admission"`
	WaitICU       int        `gorm:"column:wait_icu"`
	DataTime      *time.Time `gorm:"column:data_time"`
}

// GenerateERStatusChunks queries food_safety_medical from PostgreSQL → one chunk per district.
func GenerateERStatusChunks() ([]CityKnowledgeChunk, error) {
	if models.DBDashboard == nil {
		return nil, fmt.Errorf("DBDashboard is not initialized")
	}

	var rows []foodSafetyMedicalRow
	if err := models.DBDashboard.
		Table("food_safety_medical").
		Select("city, town, hospital_name, is_full_119, wait_see, wait_admission, wait_icu, data_time").
		Order("city, town, hospital_name").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query food_safety_medical: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("food_safety_medical: no rows")
	}

	now := time.Now().Format(time.RFC3339)

	type hospLine struct{ text string }
	type districtKey struct{ cityCode, district string }
	grouped := map[districtKey][]hospLine{}
	var keyOrder []districtKey
	seen := map[districtKey]bool{}

	for _, row := range rows {
		var cityCode string
		switch {
		case strings.Contains(row.City, "臺北") || strings.Contains(row.City, "台北"):
			cityCode = "TP"
		case strings.Contains(row.City, "新北"):
			cityCode = "NTP"
		default:
			continue
		}

		fullMark := ""
		if row.IsFull119 {
			fullMark = "滿床 "
		}
		updatedAt := now[:16]
		if row.DataTime != nil {
			updatedAt = row.DataTime.Format("2006-01-02T15:04")
		}
		text := fmt.Sprintf("%s%s（候診%d人、待住院%d人、待加護%d人，更新%s）",
			fullMark, row.HospitalName,
			row.WaitSee, row.WaitAdmission, row.WaitICU, updatedAt,
		)

		district := strings.TrimSpace(row.Town)
		if district == "" {
			district = cityDisplayName(cityCode)
		}
		key := districtKey{cityCode, district}
		if !seen[key] {
			keyOrder = append(keyOrder, key)
			seen[key] = true
		}
		grouped[key] = append(grouped[key], hospLine{text})
	}

	var chunks []CityKnowledgeChunk
	for _, key := range keyOrder {
		lines := grouped[key]
		parts := make([]string, len(lines))
		for i, l := range lines {
			parts[i] = l.text
		}
		cityName := cityDisplayName(key.cityCode)
		chunks = append(chunks, CityKnowledgeChunk{
			Topic: "food_safety", SubTopic: "er_status",
			City: key.cityCode, District: key.district,
			Content:   fmt.Sprintf("【%s%s急診狀況 %s】%s", cityName, key.district, now[:16], strings.Join(parts, "；")),
			Source:    "food_safety_medical (PostgreSQL)", UpdatedAt: now,
		})
	}
	return chunks, nil
}

// -- Data source: food_safety_diarrhea_count (PostgreSQL) --

type diarrheaCountRow struct {
	City          string     `gorm:"column:city"`
	CityCode      string     `gorm:"column:city_code"`
	DiarrheaCount int        `gorm:"column:diarrhea_count"`
	DataTime      *time.Time `gorm:"column:data_time"`
}

// GenerateDiarrheaChunks queries food_safety_diarrhea_count → one chunk per city (latest data).
func GenerateDiarrheaChunks() ([]CityKnowledgeChunk, error) {
	if models.DBDashboard == nil {
		return nil, fmt.Errorf("DBDashboard is not initialized")
	}

	// 取每個城市最新一筆
	var rows []diarrheaCountRow
	if err := models.DBDashboard.
		Table("food_safety_diarrhea_count").
		Select("city, city_code, diarrhea_count, data_time").
		Where("data_time IN (?)",
			models.DBDashboard.Table("food_safety_diarrhea_count").
				Select("MAX(data_time)").
				Group("city_code"),
		).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query food_safety_diarrhea_count: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("food_safety_diarrhea_count: no rows")
	}

	now := time.Now().Format(time.RFC3339)
	var chunks []CityKnowledgeChunk

	for _, row := range rows {
		var cityCode string
		switch strings.ToLower(row.CityCode) {
		case "tpe":
			cityCode = "TP"
		case "nwt":
			cityCode = "NTP"
		default:
			continue
		}

		dataDate := now[:10]
		if row.DataTime != nil {
			dataDate = row.DataTime.Format("2006-01-02")
		}

		chunks = append(chunks, CityKnowledgeChunk{
			Topic: "food_safety", SubTopic: "diarrhea_count",
			City:      cityCode,
			Content:   fmt.Sprintf("【%s腹瀉就診人數 %s】腹瀉就診人數 %d 人", row.City, dataDate, row.DiarrheaCount),
			Source:    "food_safety_diarrhea_count (PostgreSQL)", UpdatedAt: now,
		})
	}
	return chunks, nil
}

// -- Data source: transportation --

type ubikeRow struct {
	ServiceStatus        string `gorm:"column:service_status"`
	AvailableRentGeneral int    `gorm:"column:available_rent_general_bikes"`
	AvailableRentElectric int   `gorm:"column:available_rent_electric_bikes"`
	AvailableReturn      int    `gorm:"column:available_return_bikes"`
}

// GenerateUBikeChunks aggregates YouBike realtime → one chunk per city.
func GenerateUBikeChunks() ([]CityKnowledgeChunk, error) {
	if models.DBDashboard == nil {
		return nil, fmt.Errorf("DBDashboard is not initialized")
	}
	type agg struct{ total, normal, rentGeneral, rentElectric, returnSlots int }
	cities := map[string]*agg{"TP": {}, "NTP": {}}
	tables := map[string]string{"TP": "tran_ubike_realtime", "NTP": "tran_ubike_realtime_new_tpe"}

	for cityCode, table := range tables {
		var rows []ubikeRow
		if err := models.DBDashboard.Table(table).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("query %s: %w", table, err)
		}
		a := cities[cityCode]
		for _, r := range rows {
			a.total++
			if r.ServiceStatus == "正常營運" {
				a.normal++
			}
			a.rentGeneral += r.AvailableRentGeneral
			a.rentElectric += r.AvailableRentElectric
			a.returnSlots += r.AvailableReturn
		}
	}

	now := time.Now().Format(time.RFC3339)
	var chunks []CityKnowledgeChunk
	for _, cityCode := range []string{"TP", "NTP"} {
		a := cities[cityCode]
		if a.total == 0 {
			continue
		}
		chunks = append(chunks, CityKnowledgeChunk{
			Topic: "transportation", SubTopic: "ubike", City: cityCode,
			Content: fmt.Sprintf("【%sYouBike即時狀況 %s】共%d站，正常營運%d站，可借一般車%d輛、電動車%d輛，可還車位%d個",
				cityDisplayName(cityCode), now[:16], a.total, a.normal, a.rentGeneral, a.rentElectric, a.returnSlots),
			Source: "tran_ubike_realtime (PostgreSQL)", UpdatedAt: now,
		})
	}
	return chunks, nil
}

type bikeNetworkRow struct {
	City         string  `gorm:"column:city"`
	CityCode     string  `gorm:"column:city_code"`
	CyclingType  string  `gorm:"column:cycling_type"`
	CyclingLength float64 `gorm:"column:cycling_length"`
}

// GenerateBikeNetworkChunks aggregates bike routes → one chunk per city.
func GenerateBikeNetworkChunks() ([]CityKnowledgeChunk, error) {
	if models.DBDashboard == nil {
		return nil, fmt.Errorf("DBDashboard is not initialized")
	}
	type agg struct{ city string; routes int; totalLength float64 }
	cities := map[string]*agg{
		"TP":  {city: "台北市"},
		"NTP": {city: "新北市"},
	}
	tables := map[string]string{"TP": "bike_network_tpe", "NTP": "bike_network_new_tpe"}

	for cityCode, table := range tables {
		var rows []bikeNetworkRow
		if err := models.DBDashboard.Table(table).
			Select("city, city_code, cycling_type, cycling_length").
			Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("query %s: %w", table, err)
		}
		a := cities[cityCode]
		for _, r := range rows {
			a.routes++
			a.totalLength += r.CyclingLength
		}
	}

	now := time.Now().Format(time.RFC3339)
	var chunks []CityKnowledgeChunk
	for _, cityCode := range []string{"TP", "NTP"} {
		a := cities[cityCode]
		if a.routes == 0 {
			continue
		}
		chunks = append(chunks, CityKnowledgeChunk{
			Topic: "transportation", SubTopic: "bike_network", City: cityCode,
			Content: fmt.Sprintf("【%s自行車路網】共%d條路段，總長度約%.0f公尺（%.1f公里）",
				a.city, a.routes, a.totalLength, a.totalLength/1000),
			Source: "bike_network (PostgreSQL)", UpdatedAt: now,
		})
	}
	return chunks, nil
}

type busInfoRow struct {
	IsElectric   int `gorm:"column:is_electric"`
	IsLowFloor   int `gorm:"column:is_low_floor"`
	HasWifi      int `gorm:"column:has_wifi"`
	HasLiftOrRamp int `gorm:"column:has_lift_or_ramp"`
}

// GenerateBusInfoChunks aggregates bus fleet info → one chunk per city.
func GenerateBusInfoChunks() ([]CityKnowledgeChunk, error) {
	if models.DBDashboard == nil {
		return nil, fmt.Errorf("DBDashboard is not initialized")
	}
	type agg struct{ total, electric, lowFloor, wifi, accessible int }
	cities := map[string]*agg{"TP": {}, "NTP": {}}
	tables := map[string]string{"TP": "bus_info_tpe", "NTP": "bus_info_new_tpe"}

	for cityCode, table := range tables {
		var rows []busInfoRow
		if err := models.DBDashboard.Table(table).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("query %s: %w", table, err)
		}
		a := cities[cityCode]
		for _, r := range rows {
			a.total++
			if r.IsElectric == 1 { a.electric++ }
			if r.IsLowFloor == 1 { a.lowFloor++ }
			if r.HasWifi == 1 { a.wifi++ }
			if r.HasLiftOrRamp == 1 { a.accessible++ }
		}
	}

	now := time.Now().Format(time.RFC3339)
	var chunks []CityKnowledgeChunk
	for _, cityCode := range []string{"TP", "NTP"} {
		a := cities[cityCode]
		if a.total == 0 {
			continue
		}
		chunks = append(chunks, CityKnowledgeChunk{
			Topic: "transportation", SubTopic: "bus", City: cityCode,
			Content: fmt.Sprintf("【%s公車車隊資訊】共%d輛，電動車%d輛、低地板車%d輛、無障礙車%d輛、有WiFi車%d輛",
				cityDisplayName(cityCode), a.total, a.electric, a.lowFloor, a.accessible, a.wifi),
			Source: "bus_info (PostgreSQL)", UpdatedAt: now,
		})
	}
	return chunks, nil
}

// -- Data source: population --

type populationAgeRow struct {
	Year                          int     `gorm:"column:year"`
	YoungPopulation               int     `gorm:"column:young_population"`
	YoungPopulationPct            float64 `gorm:"column:young_population_percentage"`
	WorkingAgePopulation          int     `gorm:"column:working_age_population"`
	WorkingAgePopulationPct       float64 `gorm:"column:working_age_population_percentage"`
	ElderlyPopulation             int     `gorm:"column:elderly_population"`
	ElderlyPopulationPct          float64 `gorm:"column:elderly_population_percentage"`
	TotalDependencyRatio          float64 `gorm:"column:total_dependency_ratio"`
	AgingIndex                    float64 `gorm:"column:aging_index"`
}

// GeneratePopulationAgeChunks queries latest year population distribution → one chunk per city.
func GeneratePopulationAgeChunks() ([]CityKnowledgeChunk, error) {
	if models.DBDashboard == nil {
		return nil, fmt.Errorf("DBDashboard is not initialized")
	}
	tables := map[string]string{"TP": "population_age_distribution_tpe", "NTP": "population_age_distribution_new_tpe"}
	now := time.Now().Format(time.RFC3339)
	var chunks []CityKnowledgeChunk

	for _, cityCode := range []string{"TP", "NTP"} {
		table := tables[cityCode]
		var row populationAgeRow
		if err := models.DBDashboard.Table(table).
			Order("year DESC").Limit(1).Find(&row).Error; err != nil {
			log.Printf("Warning: %s: %v", table, err)
			continue
		}
		chunks = append(chunks, CityKnowledgeChunk{
			Topic: "population", SubTopic: "age_distribution", City: cityCode,
			Content: fmt.Sprintf("【%s人口年齡分佈 %d年】幼年人口%d人(%.1f%%)，工作年齡人口%d人(%.1f%%)，老年人口%d人(%.1f%%)，總扶養比%.2f，老化指數%.2f",
				cityDisplayName(cityCode), row.Year,
				row.YoungPopulation, row.YoungPopulationPct,
				row.WorkingAgePopulation, row.WorkingAgePopulationPct,
				row.ElderlyPopulation, row.ElderlyPopulationPct,
				row.TotalDependencyRatio, row.AgingIndex),
			Source: table + " (PostgreSQL)", UpdatedAt: now,
		})
	}
	return chunks, nil
}

type dependencyRatioRow struct {
	EndOfYear                     string  `gorm:"column:end_of_year"`
	YoungPopulation               int     `gorm:"column:young_population"`
	YoungPopulationPct            float64 `gorm:"column:young_population_percentage"`
	WorkingAgePopulation          int     `gorm:"column:working_age_population"`
	ElderlyPopulation             int     `gorm:"column:elderly_population"`
	ElderlyPopulationPct          float64 `gorm:"column:elderly_population_percentage"`
	ElderlyDependencyRatio        float64 `gorm:"column:elderly_dependency_ratio"`
	YouthDependencyRatio          float64 `gorm:"column:youth_dependency_ratio"`
	TotalDependencyRatio          float64 `gorm:"column:total_dependency_ratio"`
	AgingIndex                    float64 `gorm:"column:aging_index"`
}

// GenerateDependencyRatioChunks queries latest year dependency ratio → one chunk per city.
func GenerateDependencyRatioChunks() ([]CityKnowledgeChunk, error) {
	if models.DBDashboard == nil {
		return nil, fmt.Errorf("DBDashboard is not initialized")
	}
	tables := map[string]string{"TP": "dependency_ratio_and_aging_index_tpe", "NTP": "dependency_ratio_and_aging_index_new_tpe"}
	now := time.Now().Format(time.RFC3339)
	var chunks []CityKnowledgeChunk

	for _, cityCode := range []string{"TP", "NTP"} {
		table := tables[cityCode]
		var row dependencyRatioRow
		if err := models.DBDashboard.Table(table).
			Order("end_of_year DESC").Limit(1).Find(&row).Error; err != nil {
			log.Printf("Warning: %s: %v", table, err)
			continue
		}
		chunks = append(chunks, CityKnowledgeChunk{
			Topic: "population", SubTopic: "dependency_ratio", City: cityCode,
			Content: fmt.Sprintf("【%s扶養比與老化指數 %s年】幼年人口%d人(%.1f%%)，老年人口%d人(%.1f%%)，老年扶養比%.2f，幼年扶養比%.2f，總扶養比%.2f，老化指數%.2f",
				cityDisplayName(cityCode), row.EndOfYear,
				row.YoungPopulation, row.YoungPopulationPct,
				row.ElderlyPopulation, row.ElderlyPopulationPct,
				row.ElderlyDependencyRatio, row.YouthDependencyRatio,
				row.TotalDependencyRatio, row.AgingIndex),
			Source: table + " (PostgreSQL)", UpdatedAt: now,
		})
	}
	return chunks, nil
}

type employmentAgeRow struct {
	Year         string  `gorm:"column:year"`
	Gender       string  `gorm:"column:gender"`
	AgeStructure string  `gorm:"column:age_structure"`
	Percentage   float64 `gorm:"column:percentage"`
}

// GenerateEmploymentAgeChunks queries latest year employment age structure → one chunk per city per gender.
func GenerateEmploymentAgeChunks() ([]CityKnowledgeChunk, error) {
	if models.DBDashboard == nil {
		return nil, fmt.Errorf("DBDashboard is not initialized")
	}
	tables := map[string]string{"TP": "employment_age_structure_tpe", "NTP": "employment_age_structure_new_tpe"}
	now := time.Now().Format(time.RFC3339)
	var chunks []CityKnowledgeChunk

	for _, cityCode := range []string{"TP", "NTP"} {
		table := tables[cityCode]
		// 取最新年度
		var latestYear struct{ Year string `gorm:"column:year"` }
		if err := models.DBDashboard.Table(table).Select("year").Order("year DESC").Limit(1).Scan(&latestYear).Error; err != nil {
			log.Printf("Warning: %s latest year: %v", table, err)
			continue
		}

		var rows []employmentAgeRow
		if err := models.DBDashboard.Table(table).
			Where("year = ?", latestYear.Year).
			Order("gender, age_structure").
			Find(&rows).Error; err != nil {
			log.Printf("Warning: %s: %v", table, err)
			continue
		}

		// group by gender
		grouped := map[string][]string{}
		for _, r := range rows {
			if r.AgeStructure == "就業人口" {
				continue // skip total row
			}
			grouped[r.Gender] = append(grouped[r.Gender], fmt.Sprintf("%s:%.1f%%", r.AgeStructure, r.Percentage))
		}

		for _, gender := range []string{"總計", "男", "女"} {
			parts, ok := grouped[gender]
			if !ok || len(parts) == 0 {
				continue
			}
			chunks = append(chunks, CityKnowledgeChunk{
				Topic: "population", SubTopic: "employment_age", City: cityCode,
				District: gender,
				Content: fmt.Sprintf("【%s就業人口年齡結構(%s) %s年】%s",
					cityDisplayName(cityCode), gender, latestYear.Year, strings.Join(parts, "、")),
				Source: table + " (PostgreSQL)", UpdatedAt: now,
			})
		}
	}
	return chunks, nil
}

// -- Qdrant helpers --

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

func createCollectionIfNotExists(ctx context.Context, collectionName string, vectorSize uint64) error {
	cfg := global.Qdrant
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
