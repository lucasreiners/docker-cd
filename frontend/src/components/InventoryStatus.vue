<template>
  <span class="inventory-status" :class="`inventory-status--${tone}`">
    <component :is="icon" :size="14" aria-hidden="true" :class="{ 'inventory-spin': tone === 'active' }" />
    <span>{{ label }}</span>
  </span>
</template>
<script setup lang="ts">
import { Check, Circle, CircleHelp, LoaderCircle, OctagonX, TriangleAlert } from 'lucide-vue-next'
import { computed } from 'vue'

const props = defineProps<{
  label: string
  tone: 'success' | 'active' | 'warning' | 'danger' | 'neutral' | 'unknown'
}>()
const icon = computed(
  () =>
    ({
      success: Check,
      active: LoaderCircle,
      warning: TriangleAlert,
      danger: OctagonX,
      neutral: Circle,
      unknown: CircleHelp,
    })[props.tone],
)
</script>
