# 韌性防災儀表板 — 競賽專案 CLAUDE.md

## 專案結構
此專案是 Taipei-City-Dashboard 的 fork（https://github.com/taipei-doit/Taipei-City-Dashboard）

## 技術棧（請嚴格遵守，不要引入新框架）
- 後端：Go + Gin 框架，位於 Taipei-City-Dashboard-BE/
- 前端：Vue 3 (Options API 風格) + Pinia + ApexCharts + Turf.js（已安裝），位於 Taipei-City-Dashboard-FE/
- 資料庫：PostgreSQL + PostGIS extension
- 路由：所有 BE 路由集中在 app/routes/router.go，新增路由請加在此檔案底部

## 我們新增的功能：防災風險評分卡儀表板
儀表板 index：`disaster-risk`
功能：使用者選擇行政區 → 系統查詢多個 PostGIS 資料庫 → AI 產生白話摘要

## 新增的資料庫 tables（前綴統一用 disaster_）
- disaster_flood_simulation（積水模擬圖，PostGIS polygon）
- disaster_slope_warning（列管邊坡警戒值）
- disaster_settlement_warning（老舊聚落警戒值）
- disaster_earthquake_buildings（地震列管建築物）
- disaster_shelters（台北+新北避難收容處所）

## 命名慣例（遵循原專案風格）
- Go 檔案：snake_case（disaster_risk.go）
- Vue 元件：PascalCase（DisasterRiskCard.vue）
- Pinia store：camelCase（useDisasterStore.js）

## 當前完成進度
完成摘要
修改的檔案

檔案	動作
db-sample-data/disaster_tables.sql	覆寫（前版本為 5 表簡版）
每張表的欄位數

表名	欄位數
disaster_flood_zones	8
disaster_slope_warnings	10
disaster_settlement_warnings	8
disaster_earthquake_buildings	8
disaster_shelters	17
disaster_risk_scores	15
索引：geom 欄位用 GIST，所有 district 欄位用 B-tree；disaster_risk_scores 另有 (city, district) 唯一索引。

給下個 Session 的最小資訊摘要
DB: PostgreSQL + PostGIS 3.4，資料庫名 dashboard
Schema: public，所有表名前綴 disaster_
geom 欄位：
  - disaster_flood_zones.geom → GEOMETRY(POLYGON, 4326)
  - disaster_shelters.geom    → GEOMETRY(POINT, 4326)
  - 其他表無 geom，只有 lat/lon float
city 值慣例：'taipei' | 'ntpc'
risk_level 值慣例：'low' | 'medium' | 'high' | 'critical'
scenario 值慣例（flood）：'78.8mm/h' | '100mm/h' | '130mm/h'
disaster_risk_scores 是快取表，由計算腳本定期寫入，不從原始資料直接匯入
disaster_shelters 合併台北市 + 新北市，以 city 欄位區分
所有表均有 created_at / updated_at TIMESTAMPTZ DEFAULT NOW()