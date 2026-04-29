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
				// 將 CSV 的 city 對應到原有的 areaCode
				let areaCode = "";
				if (item.city === "臺北市") areaCode = "01";
				else if (item.city === "新北市") areaCode = "31";

				return {
					areaCode,
					hospitalName: item.hospital_name || "未提供醫院名稱",
					waitBedCount: Number.parseInt(item.wait_push_bed, 10) || 0,
				};
			});

			const filtered = normalized.filter(
				(item) => item.areaCode === "01" || item.areaCode === "31",
			);

			nhiData.value = filtered.sort(
				(a, b) => b.waitBedCount - a.waitBedCount,
			);
		}
	} catch (error) {
		console.error("無法解析 CSV 資料:", error);
	}
};

onMounted(() => {
	fetchNhiData();
});

const chartCategories = computed(() => {
	if (filteredData.value.length > 0) {
		return filteredData.value.map((item) => item.hospitalName);
	}
	return props.chart_config.categories || [];
});

// 3. 設定 ApexCharts 選項
const chartOptions = computed(() => {
	return {
		chart: {
			offsetY: 15,
			toolbar: { show: false },
			background: "transparent",
			fontFamily: "inherit",
			redrawOnParentResize: true,
			redrawOnWindowResize: true,
		},
		theme: { mode: "dark" },
		legend: { show: false },
		colors: props.chart_config.color || ["#e74c3c"],
		plotOptions: {
			bar: {
				horizontal: true,
				borderRadius: 4,
				barHeight: "72%",
			},
		},
		dataLabels: {
			enabled: true,
			textAnchor: "start",
			style: { colors: ["#fff"] },
			formatter: function (val) {
				return val > 0 ? val : ""; // 數值為 0 時不顯示文字
			},
			offsetX: 10,
		},
		xaxis: {
			categories: chartCategories.value,
			labels: { style: { colors: "var(--color-complement-text)" } },
		},
		yaxis: {
			labels: {
				style: { colors: "var(--color-normal-text)", fontSize: "12px" },
				maxWidth: 180,
				formatter: function (value) {
					return value.length > 12
						? `${value.slice(0, 12)}...`
						: value;
				},
			},
		},
		tooltip: {
			theme: "dark",
			custom: function ({ series, seriesIndex, dataPointIndex, w }) {
				const label = w.globals.labels[dataPointIndex];
				const value = series[seriesIndex][dataPointIndex];
				return `
					<div class="chart-tooltip">
						<h6>${label}</h6>
						<span>${value} ${props.chart_config.unit || "人"}</span>
					</div>
				`;
			},
		},
		grid: {
			show: false,
		},
		noData: {
			text: "暫無資料",
			style: { color: "var(--color-normal-text)" },
		},
	};
});

// 4. 綁定資料系列
const chartSeries = computed(() => {
	if (filteredData.value.length > 0) {
		return [
			{
				name: "等待推床人數",
				data: filteredData.value.map((item) => item.waitBedCount),
			},
		];
	}
	return props.series;
});

// 5. 依資料筆數動態計算高度，對齊 BarPercentChart 的條高與間距
const ROW_HEIGHT = 30;
const CHART_PADDING = 50;
const MIN_CHART_HEIGHT = 200; // 確保無資料時仍有足夠空間顯示提示

const chartHeight = computed(() => {
	const count = chartSeries.value?.[0]?.data?.length ?? 0;
	const calculated = count * ROW_HEIGHT + CHART_PADDING;
	return Math.max(calculated, MIN_CHART_HEIGHT);
});

const chartRenderKey = computed(() => {
	const dataCount = chartSeries.value?.[0]?.data?.length ?? 0;
	return `${props.activeCity || "default"}-${dataCount}`;
});
</script>

<template>
	<div v-if="activeChart === 'NhiEmergencyChart'" class="nhiemergencychart">
		<div class="nhiemergencychart-chart">
			<VueApexCharts
				:key="chartRenderKey"
				type="bar"
				width="100%"
				:height="chartHeight"
				:options="chartOptions"
				:series="chartSeries"
			/>
		</div>
	</div>
</template>

<style lang="scss" scoped>
/* 嚴格遵守 BEM 命名與 CSS 變數規範 */
.nhiemergencychart {
	display: flex;
	flex-direction: column;
	background-color: var(--color-component-background);
	border-radius: 8px;

	&-header {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 12px;
		color: var(--color-highlight);

		span {
			font-family: var(
				--font-icon
			); /* 使用 README 規定的 Material Icons Round */
			font-size: var(--font-xl);
		}

		h3 {
			font-size: var(--font-m);
			color: var(--color-normal-text);
			margin: 0;
		}
	}

	&-chart {
		overflow-y: auto;
		overflow-x: hidden;
		max-height: 480px;
	}
}
</style>
