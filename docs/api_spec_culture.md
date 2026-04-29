# 文化共融 API 規格書 (v1.0)

## 1. Cultural Data

### [GET]/api/v1/festival

【輸入 (Input)】
| 參數名 | 型態 | 必填 | 說明 | 範例 |
| :--- | :--- | :--- | :--- | :--- |
| status | string | 否 | 活動狀態 (ongoing/upcoming) | ?status=ongoing |
| lunar_month| int | 否 | 指定農曆月份過濾 | ?lunar_month=3 |

【輸出 (Output)】:

```json
{
  "status": "success",
  "data": [
    {
      "id": 801,
      "name": "萬華好熱鬧",
      "type": "祭典活動",
      "lunar_date": "10-22",
      "solar_date": "2026-11-30",
      "latitude": 25.037,
      "longitude": 121.499,
      "place": "艋舺青山宮",
      "impact": {
        "traffic_control": "環河南路二段管制",
        "decibel_level": 85,
        "scale": "Large"
      },
      "aiHarmony": "High",
      "aiSummary": "台北三大祭典之一，展現強烈社區凝聚力，建議外籍遊客由導覽員陪同參與。"
    }
  ]
}
```

## 2.Component Data

### [GET]/api/v1/component/{id}?city={city}

【輸入 (Input)】:
Path Parameter: {id} (組件識別碼，如 rain_trend, hospital_bed)
Query Parameter: city: 過濾縣市 (TP/NTP)

| 參數名 | 型態   | 必填 | 說明                               | 範例              |
| :----- | :----- | :--- | :--------------------------------- | :---------------- |
| city   | string | 否   | 過濾縣市 (TP/NTP)                  | ?city=TP          |
| sort   | string | 否   | 排序依據 (waiting_icu/waiting_bed) | ?sort=waiting_icu |

【輸出 (Output)】:

#### (1)二維資料

> 對應Go結構：`TwoDimensionalDataOutput`

```json
{
  "status": "success",
  "data": [
    { "x": "媽祖", "y": 45 },
    { "x": "保生大帝", "y": 20 },
    { "x": "基督教會", "y": 25 }
  ]
}
```

#### (2)時間序列

> 對應Go結構：`TimeSeriesDataOutput`

```json
{
  "status": "success",
  "data": [
    {
      "name": "參與人次預估",
      "data": [
        { "x": "2026-03-01", "y": 5000 },
        { "x": "2026-04-01", "y": 12000 }
      ]
    }
  ]
}
```

#### (3)各區共融性

[GET]/api/v1/component/cultural_integration

【輸出 (Output)】:

```json
{
  "status": "success",
  "data": {
    "labels": ["宗教多樣性", "文化融合度", "空間友善度"],
    "datasets": [
      {
        "name": "萬華區",
        "values": [90, 85, 70]
      }
    ]
  }
}
```

## 3.query

### [POST]/api/v1/query

【輸入 (Input)】:

```json
{
  "query": "為什麼祭典要燒金紙？",
  "location": "萬華龍山寺"
}
```

【輸出 (Output)】:

```json
{
  "status": "success",
  "aiSummary": "燒金紙在傳統信仰中象徵傳遞心意與財富給神靈或祖先。在共融視角下，龍山寺現推行『心誠則靈』減香減爐，展現環境友善與文化保存的平衡。",
  "aiHarmony": "High",
  "related_festival": "盂蘭盆勝會"
}
```

## 4.qdrant

### [POST]/api/v1/qdrant/rebuild

【輸入 (Input)】:

```json
{ "force": true }
```

【輸出 (Output)】:

#### (1)成功

```json
{
  "status": "success",
  "message": "Synchronous rebuild complete (up to implemented steps).",
  "data": {
    "total_points": 100,
    "indexed_points": 50,
    "indexed_vectors": 50,
    "indexed_payloads": 50
  }
}
```

#### (2)失敗

```json
{
  "status": "error",
  "message": "qdrant rebuild is already in progress"
}
```

## websocket

### [GET]/api/v1/websocket

### [連接資訊]

- **Endpoint**:`ws://[host]/api/v1/websocket`
- **Protocol**:WebSocket(RFC 6455)
- **Authentication**:需在連線時的Header帶入`Authorization:Bearer{JWT_TOKEN}`
