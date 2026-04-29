<script setup>
defineProps({
	activeChart: { type: String, required: true },
});
</script>

<template>
	<div v-if="activeChart === 'LongTermCareText'" class="longtermcaretext">
		這是長照關懷
	</div>
</template>

<style scoped lang="scss">
.longtermcaretext {
	display: flex;
	align-items: center;
	justify-content: center;
	min-height: 160px;
	padding: 16px;
	color: var(--color-normal-text);
	font-size: var(--font-m);
	line-height: 1.5;
}
</style>
