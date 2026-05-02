import requests
from flask import Flask, jsonify
import uuid
from datetime import datetime, timezone

app = Flask(__name__)

# --- 加入這段處理 CORS ---
@app.after_request
def after_request(response):
    # 允許所有來源存取，解決前端與後端 Port 不同的問題
    response.headers.add('Access-Control-Allow-Origin', '*')
    response.headers.add('Access-Control-Allow-Headers', 'Content-Type,Authorization')
    response.headers.add('Access-Control-Allow-Methods', 'GET,PUT,POST,DELETE,OPTIONS')
    return response
# ------------------------

def fetch_cwa_warnings():
    """從氣象署 API 獲取強風特報"""
    cwa_url = "https://opendata.cwa.gov.tw/api/v1/rest/datastore/W-C0033-001?Authorization=CWA-50F54371-6B55-4165-85F1-982CB65556AA&locationName=%E6%96%B0%E5%8C%97%E5%B8%82&limit=5"
    incidents = []
    try:
        response = requests.get(cwa_url, timeout=5)
        if response.status_code == 200:
            result = response.json()
            locations = result.get("records", {}).get("location", [])
            for loc in locations:
                hazards = loc.get("hazardConditions", {}).get("hazards", [])
                for h in hazards:
                    info = h.get("info", {})
                    v_time = h.get("validTime", {})
                    incidents.append({
                        "ID": str(uuid.uuid4()),
                        "inctype": "WIND",
                        "description": f"【氣象特報】{loc['locationName']}發布{info['phenomena']}{info['significance']}。",
                        "distance": 0.0,
                        "latitude": 25.0123,
                        "longitude": 121.4657,
                        "place": loc['locationName'],
                        "reportTime": v_time.get("startTime", datetime.now().isoformat()),
                        "status": "ACTIVE",
                        "city": "NTP",
                        "aiSummary": f"偵測到氣象署發布之{info['phenomena']}，影響時間自 {v_time.get('startTime')} 起。",
                        "aiRisk": "High" if info['significance'] == "警報" else "Med"
                    })
    except Exception as e:
        print(f"Error fetching Warning data: {e}")
    return incidents

def fetch_cwa_earthquakes():
    """從氣象署 API 獲取地震報告 (針對新北市感測)"""
    # 這裡預設抓取最近的一筆
    cwa_url = "https://opendata.cwa.gov.tw/api/v1/rest/datastore/E-A0015-001?Authorization=CWA-50F54371-6B55-4165-85F1-982CB65556AA&limit=1"
    incidents = []
    try:
        response = requests.get(cwa_url, timeout=5)
        if response.status_code == 200:
            data = response.json()
            eq_list = data.get("records", {}).get("Earthquake", [])
            for eq in eq_list:
                report_content = eq.get("ReportContent", "")
                eq_info = eq.get("EarthquakeInfo", {})
                epicenter = eq_info.get("Epicenter", {})
                
                # 檢查新北市是否有感
                shaking_areas = eq.get("Intensity", {}).get("ShakingArea", [])
                ntp_intensity = next((area for area in shaking_areas if area.get("CountyName") == "新北市"), None)
                
                if ntp_intensity:
                    incidents.append({
                        "ID": str(uuid.uuid4()),
                        "inctype": "EARTHQUAKE",
                        "description": report_content,
                        "distance": 0.0,
                        "latitude": epicenter.get("EpicenterLatitude"),  # 使用震央緯度
                        "longitude": epicenter.get("EpicenterLongitude"), # 使用震央經度
                        "place": epicenter.get("Location"),
                        "reportTime": eq_info.get("OriginTime"),
                        "status": "ACTIVE",
                        "city": "NTP",
                        "aiSummary": f"新北市觀測到最大震度為 {ntp_intensity.get('AreaIntensity')}。震央位於{epicenter.get('Location')}。",
                        "aiRisk": "High" if "4級" in ntp_intensity.get("AreaIntensity", "") else "Med"
                    })
    except Exception as e:
        print(f"Error fetching Earthquake data: {e}")
    return incidents

@app.route('/api/incidents', methods=['GET'])
def get_incidents():
    # 1. 執行多方資料擷取
    warning_data = fetch_cwa_warnings()
    earthquake_data = fetch_cwa_earthquakes()
    
    # 2. 合併資料
    all_incidents = warning_data + earthquake_data
    
    response = {
        "status": "success",
        "message": "取得即時災情與氣象資料成功",
        "count": len(all_incidents),
        "data": all_incidents
    }
    
    return jsonify(response), 200

if __name__ == '__main__':
    app.run(debug=True)
