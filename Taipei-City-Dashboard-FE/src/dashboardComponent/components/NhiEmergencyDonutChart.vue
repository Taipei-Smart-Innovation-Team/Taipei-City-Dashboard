<script setup>
import { computed, onMounted, ref } from "vue";
import VueApexCharts from "vue3-apexcharts";
import erCsvRaw from "../assets/er.csv?raw";

// 1. 嚴格遵守 README 規定的 Props 結構
const props = defineProps({
	activeChart: { type: String, required: true },
	activeCity: { type: String, default: "" },
	chart_config: { type: Object, required: true },
	series: { type: Array, required: true },
	map_config: { type: Array, default: null },
	map_filter: { type: Object, default: null },
	map_filter_on: { type: Boolean, default: false },
});

// 2. API 串接與內部城市切換邏輯
const nhiData = ref([]);

const filteredData = computed(() => {
	if (props.activeCity === "taipei") {
		return nhiData.value.filter((item) => item.areaCode === "01");
	}
	if (props.activeCity === "newtaipei" || props.activeCity === "new_taipei") {
		return nhiData.value.filter((item) => item.areaCode === "31");
	}
	return nhiData.value;
});

// TODO: API_INTEGRATION_POINT - 目前改為使用組員提供的 CSV 檔案作為資料來源
// API_STUB
const isMock = true;

function parseCSV(csvText) {
	const lines = csvText.trim().split(/\r?\n/);
	if (lines.length < 2) return [];
	const headers = lines[0]
		.split(",")
		.map((header) => header.trim().replace(/^\uFEFF/, ""));
	return lines.slice(1).map((line) => {
		const values = line.split(",");
		const record = {};
		headers.forEach((header, index) => {
			record[header] = values[index]?.trim();
		});
		return record;
	});
}

const fetchNhiData = async () => {
	try {
		const rawList = isMock ? parseCSV(erCsvRaw) : props.series;

		if (rawList && Array.isArray(rawList)) {
			const normalized = rawList.map((item) => {
				let areaCode = "";
				if (item.city === "臺北市") areaCode = "01";
				else if (item.city === "新北市") areaCode = "31";

				return {
					areaCode,
					hospitalName: item.hospital_name || "未提供醫院名稱",
					waitSee: Number.parseInt(item.wait_see, 10) || 0,
					waitPushBed: Number.parseInt(item.wait_push_bed, 10) || 0,
					waitAdmission: Number.parseInt(item.wait_admission, 10) || 0,
					waitIcu: Number.parseInt(item.wait_icu, 10) || 0,
				};
			});

			const filtered = normalized.filter(
				(item) => item.areaCode === "01" || item.areaCode === "31",
			);

			nhiData.value = filtered;
		}
	} catch (error) {
		console.error("無法解析 CSV 資料:", error);
	}
};

onMounted(() => {
	fetchNhiData();
});

// 3. 圓餅圖相關邏輯
const donutSeries = computed(() => {
	const see = filteredData.value.reduce((sum, item) => sum + item.waitSee, 0);
	const push = filteredData.value.reduce((sum, item) => sum + item.waitPushBed, 0);
	const admission = filteredData.value.reduce((sum, item) => sum + item.waitAdmission, 0);
	const icu = filteredData.value.reduce((sum, item) => sum + item.waitIcu, 0);
	return [see, push, admission, icu];
});

const donutOptions = computed(() => {
	return {
		chart: {
			toolbar: { show: false },
			background: "transparent",
			fontFamily: "inherit",
		},
		theme: { mode: "dark" },
		labels: ["等待看診", "等待推床", "等待住院", "等待加護病房"],
		colors: ["#40C4FF", "#FF5252", "#FFAB40", "#9b59b6"],
		legend: {
			show: true,
			position: "bottom",
			labels: { colors: "var(--color-normal-text)" },
		},
		plotOptions: {
			pie: {
				donut: {
					size: "80%",
					labels: {
						show: true,
						name: {
							show: true,
							color: "var(--color-complement-text)",
							fontSize: "14px",
							offsetY: -10,
						},
						value: {
							show: true,
							color: "var(--color-normal-text)",
							fontSize: "20px",
							fontWeight: "bold",
							offsetY: 10,
							formatter: (val) => `${val} 人`,
						},
						total: {
							show: true,
							label: "總等待人數",
							color: "var(--color-highlight)",
							fontSize: "14px",
							formatter: (w) => {
								const total = w.globals.seriesTotals.reduce((a, b) => a + b, 0);
								return `${total} 人`;
							},
						},
					},
				},
			},
		},
		tooltip: { theme: "dark" },
		stroke: { show: false },
		noData: {
			text: "暫無資料",
			style: { color: "var(--color-normal-text)" },
		},
	};
});
</script>

<template>
	<div v-if="activeChart === 'NhiEmergencyDonutChart'" class="nhiemergencydonutchart">
		<div class="nhiemergencydonutchart-chart">
			<VueApexCharts
				:key="`donut-${props.activeCity || 'default'}`"
				type="donut"
				width="100%"
				height="300px"
				:options="donutOptions"
				:series="donutSeries"
			/>
		</div>
	</div>
</template>

<!-- <style lang="scss" scoped>
.nhiemergencydonutchart {
	display: flex;
	flex-direction: column;
	background-color: var(--color-component-background);
	border-radius: 8px;
	text-align: center;

	&-chart {
		overflow: hidden;
		//max-height: 480px;
		display: flex;
		justify-content: center;
		align-items: center;
	}
}
</style> -->
<style lang="scss" scoped>
.nhiemergencydonutchart {
	display: flex;
	flex-direction: column;
	background-color: var(--color-component-background);
	border-radius: 12px; 
	//padding: 16px;       // 增加內距，讓圖表不要貼齊邊緣
	transition: all 0.3s ease;

	&-chart {
		position: relative;
		display: flex;
		justify-content: center;
		align-items: center;
		width: 100%;
		
		// 確保在不同尺寸螢幕下都有合適的高度限制
		min-height: 240px;
		max-height: 300px; 
		
		// 讓內部的 ApexCharts 填滿容器
		:deep(.vue-apexcharts) {
			width: 100%;
			display: flex;
			justify-content: center;
		}

		/* --- ApexCharts 內部樣式 --- */
		// Legend (圖例) 的間距
		:deep(.apexcharts-legend) {
			//padding: 6px;
			gap: 12px;
		}
	}
}
</style>