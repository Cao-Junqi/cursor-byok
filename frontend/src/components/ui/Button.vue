<script setup>
defineProps({
  variant: {
    type: String,
    default: "default",
    validator: (v) => ["default", "primary", "text"].includes(v),
  },
});
</script>

<template>
  <button
    type="button"
    class="!whitespace-nowrap shrink-0 cursor-pointer center-row min-h-[24px] gap-1 rounded-md text-sm transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 active:scale-95 disabled:cursor-not-allowed disabled:opacity-60"
    :class="{
      'border border-line-strong bg-surface-input px-2.5 text-ink-primary hover:bg-surface-hover': variant === 'default',
      'border border-brand-600 bg-brand-500 px-2.5 font-medium text-white hover:bg-brand-600': variant === 'primary',
      'px-0.5 text-ink-secondary hover:text-brand-500 active:text-brand-600': variant === 'text',
    }"
  >
    <slot />
  </button>
</template>
