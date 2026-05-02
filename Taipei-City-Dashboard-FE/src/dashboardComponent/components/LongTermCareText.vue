<script setup>
import { onMounted, ref } from 'vue';
import axios from 'axios'; // 建議先使用原生 axios 以避免與主後端的攔截器衝突

// 接收來自父組件的 Props
const props = defineProps({
	activeChart: { type: String, required: true },
});

// 定義響應式變數
const incidents = ref([]);
const loading = ref(true);
const error = ref(null);

// 串接 API 的函式
const fetchIncidents = async () => {
	loading.value = true;
	try {
		// 注意：請確保 123.py 正在運行，且 URL 正確
		// 預設 Flask 運行於 http://127.0.0.1:5000
		const response = await axios.get('http://127.0.0.1:5000/api/incidents');
		
		if (response.data.status === 'success') {
			incidents.value = response.data.data;
		}
	} catch (err) {
		console.error('API 串接出錯:', err);
		error.value = '無法取得即時資料，請檢查後端服務是否啟動';
	} finally {
		loading.value = false;
	}
};

// 組件掛載時執行抓取
onMounted(() => {
	if (props.activeChart === 'LongTermCareText') {
		fetchIncidents();
	}
});
</script>

<template>
	<div v-if="activeChart === 'LongTermCareText'" class="longtermcaretext">
		<!-- 載入中狀態 -->
		<div v-if="loading" class="status-msg">載入即時災情資料中...</div>
		
		<!-- 錯誤訊息 -->
		<div v-else-if="error" class="status-msg error">{{ error }}</div>
		
		<!-- 資料清單 -->
		<div v-else class="incident-list">
			<div v-for="item in incidents" :key="item.ID" class="incident-card" :class="`risk-${item.aiRisk.toLowerCase()}`">
				<div class="card-header">
					<span class="type-tag">{{ item.inctype }}</span>
					<span class="time">{{ item.reportTime }}</span>
				</div>
				<div class="description">{{ item.description }}</div>
				<div class="ai-summary">
					<span class="label">AI 摘要：</span>{{ item.aiSummary }}
				</div>
				<div class="risk-level">
					風險等級：<span class="risk-value">{{ item.aiRisk }}</span>
				</div>
			</div>
			
			<div v-if="incidents.length === 0" class="status-msg">目前尚無即時通報資料</div>
		</div>
	</div>
</template>

<style scoped lang="scss">
.longtermcaretext {
	width: 100%;
	min-height: 200px;
	padding: 16px;
	color: var(--color-normal-text);
	font-size: var(--font-m);
}

.status-msg {
	text-align: center;
	padding: 20px;
	&.error { color: #ff5252; }
}

.incident-list {
	display: flex;
	flex-direction: column;
	gap: 12px;
}

.incident-card {
	background: rgba(255, 255, 255, 0.05);
	border-left: 4px solid #9e9e9e;
	border-radius: 4px;
	padding: 12px;
	transition: transform 0.2s;

	&:hover {
		background: rgba(255, 255, 255, 0.08);
	}

	&.risk-high { border-left-color: #ff5252; }
	&.risk-med { border-left-color: #ffb74d; }

	.card-header {
		display: flex;
		justify-content: space-between;
		margin-bottom: 8px;
		font-size: var(--font-s);
		
		.type-tag {
			background: var(--color-highlight);
			color: #000;
			padding: 2px 6px;
			border-radius: 4px;
			font-weight: bold;
		}
		.time { opacity: 0.6; }
	}

	.description {
		margin-bottom: 8px;
		line-height: 1.4;
	}

	.ai-summary {
		font-size: var(--font-s);
		background: rgba(0, 0, 0, 0.2);
		padding: 8px;
		border-radius: 4px;
		margin-bottom: 4px;
		.label { color: var(--color-highlight); }
	}

	.risk-level {
		font-size: var(--font-xs);
		text-align: right;
		.risk-value { font-weight: bold; }
	}
}
</style>
