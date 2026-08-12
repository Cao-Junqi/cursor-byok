<script setup>
import { computed } from "vue";
import Tooltip from "@/components/ui/Tooltip.vue";
import { formatDuration } from "@/state/appState";

const props = defineProps({
  result: {
    type: Object,
    default: null,
  },
  stale: {
    type: Boolean,
    default: false,
  },
  compact: {
    type: Boolean,
    default: false,
  },
  showMetrics: {
    type: Boolean,
    default: false,
  },
  title: {
    type: String,
    default: "模型测试",
  },
  emptyText: {
    type: String,
    default: "尚未测试",
  },
});

const normalizedStatus = computed(() => {
  const status = String(props.result?.status || "").trim().toLowerCase();
  return ["running", "success", "error"].includes(status) ? status : "idle";
});

const summaryText = computed(() => {
  const text = String(props.result?.summaryText || "").trim();
  if (text) {
    return text;
  }
  if (normalizedStatus.value === "running") {
    return "测试中...";
  }
  if (normalizedStatus.value === "error") {
    return "测试失败";
  }
  return props.emptyText;
});

const rawResponseText = computed(() => {
  const raw = String(props.result?.rawResponse || "").trim();
  if (raw) {
    return raw;
  }
  if (normalizedStatus.value === "error") {
    return String(props.result?.error || "").trim();
  }
  return "";
});

const panelClass = computed(() => {
  if (props.stale) {
    return "border-amber-500/30 bg-amber-500/10";
  }
  if (normalizedStatus.value === "running") {
    return "border-sky-500/30 bg-sky-500/10";
  }
  if (normalizedStatus.value === "error") {
    return "border-red-500/30 bg-red-500/10";
  }
  if (normalizedStatus.value === "success" && props.result?.tokensEstimated) {
    return "border-amber-500/30 bg-amber-500/10";
  }
  if (normalizedStatus.value === "success") {
    return "border-brand-500/30 bg-brand-500/10";
  }
  return "border-line bg-surface-input";
});

const summaryClass = computed(() => {
  if (props.stale) {
    return "text-amber-400";
  }
  if (normalizedStatus.value === "running") {
    return "text-sky-400";
  }
  if (normalizedStatus.value === "error") {
    return "text-red-400";
  }
  if (normalizedStatus.value === "success" && props.result?.tokensEstimated) {
    return "text-amber-400";
  }
  if (normalizedStatus.value === "success") {
    return "text-brand-400";
  }
  return "text-ink-secondary";
});
</script>

<template>
  <div class="rounded-[8px] border px-3 py-3" :class="panelClass">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-1.5">
          <div
            :class="compact ? 'text-[11px] uppercase tracking-[0.08em] text-ink-muted' : 'text-sm font-medium text-ink-primary'"
          >
            {{ title }}
          </div>
          <div v-if="rawResponseText" class="center-row gap-1 text-[11px] text-ink-muted">
            <span>原始返回</span>
            <Tooltip :content="rawResponseText" copyable />
          </div>
        </div>
        <div class="mt-1 text-sm leading-relaxed" :class="summaryClass">
          {{ summaryText }}
        </div>
      </div>
      <span
        v-if="stale"
        class="shrink-0 rounded-full border border-amber-500/30 px-2 py-1 text-xs text-amber-400"
      >
        需重测
      </span>
    </div>

    <div v-if="stale" class="mt-2 text-xs text-amber-400">
      配置已变更，请重新测试
    </div>

    <div
      v-if="showMetrics && normalizedStatus === 'success'"
      class="mt-3 grid grid-cols-1 gap-2 md:grid-cols-2"
    >
      <div class="rounded-md bg-surface-input px-3 py-2">
        <div class="text-[11px] uppercase tracking-[0.08em] text-ink-muted">总耗时</div>
        <div class="mt-1 text-sm text-ink-primary">{{ formatDuration(result?.totalDurationMS) }}</div>
      </div>
      <div class="rounded-md bg-surface-input px-3 py-2">
        <div class="text-[11px] uppercase tracking-[0.08em] text-ink-muted">输出 Token</div>
        <div class="mt-1 text-sm text-ink-primary">{{ result?.outputTokens ?? 0 }}</div>
      </div>
    </div>

    <div
      v-if="normalizedStatus === 'success' && result?.tokensEstimated"
      class="mt-2 text-xs text-ink-muted"
    >
      输出 Token 为估算值
    </div>
  </div>
</template>
