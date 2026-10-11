<template>
  <time
    :datetime="valid ? value : undefined"
    :title="valid ? new Date(value!).toLocaleString() : 'No recorded time'"
    >{{ label }}</time
  >
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'

const props = defineProps<{ value?: string }>()
const now = ref(Date.now())
const valid = computed(
  () => !!props.value && Number.isFinite(Date.parse(props.value)) && Date.parse(props.value) > 0,
)
const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
const label = computed(() => {
  if (!valid.value) return 'Not yet'
  const seconds = (Date.parse(props.value || '') - now.value) / 1000
  if (Math.abs(seconds) < 60) return 'Just now'
  if (Math.abs(seconds) < 3600) return formatter.format(Math.round(seconds / 60), 'minute')
  if (Math.abs(seconds) < 86400) return formatter.format(Math.round(seconds / 3600), 'hour')
  return formatter.format(Math.round(seconds / 86400), 'day')
})
let timer: ReturnType<typeof setInterval>
onMounted(() => {
  timer = setInterval(() => {
    now.value = Date.now()
  }, 30_000)
})
onUnmounted(() => clearInterval(timer))
</script>
