export const RISK_LEVELS = {
	CRITICAL: "critical",
	HIGH: "high",
	MEDIUM: "medium",
	LOW: "low",
	INFO: "info",
	NORMAL: "normal",
	IDLE: "idle",
	EMPTY: "empty",
	DISABLED: "disabled",
	UNKNOWN: "unknown",
	MAINTENANCE: "maintenance",
};

export const RISK_COLORS = {
	[RISK_LEVELS.CRITICAL]: "#FF5252",
	[RISK_LEVELS.HIGH]: "#FFAB40",
	[RISK_LEVELS.MEDIUM]: "#FFE16F",
	[RISK_LEVELS.LOW]: "#69F0AE",
	[RISK_LEVELS.INFO]: "#40C4FF",
	[RISK_LEVELS.NORMAL]: "#9E9E9E",
	[RISK_LEVELS.IDLE]: "#BDBDBD",
	[RISK_LEVELS.EMPTY]: "#E0E0E0",
	[RISK_LEVELS.DISABLED]: "#C7CCD1",
	[RISK_LEVELS.UNKNOWN]: "#90A4AE",
	[RISK_LEVELS.MAINTENANCE]: "#9575CD",
};

export const RISK_CHART_SEQUENCE = [
	RISK_COLORS[RISK_LEVELS.CRITICAL],
	RISK_COLORS[RISK_LEVELS.HIGH],
	RISK_COLORS[RISK_LEVELS.MEDIUM],
	RISK_COLORS[RISK_LEVELS.LOW],
	RISK_COLORS[RISK_LEVELS.INFO],
];

export function riskColor(level, fallback = RISK_COLORS[RISK_LEVELS.UNKNOWN]) {
	if (!level) return fallback;
	return RISK_COLORS[String(level).toLowerCase()] || fallback;
}

export function riskCssVar(level) {
	return `var(--risk-${String(level || RISK_LEVELS.UNKNOWN).toLowerCase()})`;
}
