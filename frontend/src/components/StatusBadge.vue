<template>
  <span class="status-badge" :class="`status-${tone}`"
    ><component :is="icon" :size="12" aria-hidden="true" />{{ label }}</span
  >
</template>
<script setup lang="ts">
import { CircleCheck, CircleDashed, CircleX, Clock3, LoaderCircle } from 'lucide-vue-next'
import { computed } from 'vue'

const props = defineProps<{ status: string; acknowledgement?: boolean }>()
const label = computed(() =>
  props.acknowledgement
    ? 'Awaiting approval'
    : {
        synced: 'In sync',
        syncing: 'Deploying',
        missing: 'Not deployed',
        deleting: 'Removing',
        completed: 'Completed',
        completed_with_errors: 'Completed with errors',
        refreshing: 'Checking Git',
        queued: 'Queued',
        running: 'Running',
        failed: 'Failed',
        cancelled: 'Cancelled',
        interrupted: 'Interrupted',
      }[props.status] || props.status,
)
const tone = computed(() =>
  props.acknowledgement
    ? 'warning'
    : ['synced', 'completed'].includes(props.status)
      ? 'success'
      : ['failed', 'completed_with_errors'].includes(props.status)
        ? 'error'
        : ['syncing', 'refreshing', 'running'].includes(props.status)
          ? 'info'
          : ['queued', 'interrupted', 'deleting'].includes(props.status)
            ? 'warning'
            : 'neutral',
)
const icon = computed(() =>
  tone.value === 'success'
    ? CircleCheck
    : tone.value === 'error'
      ? CircleX
      : tone.value === 'info'
        ? LoaderCircle
        : tone.value === 'warning'
          ? Clock3
          : CircleDashed,
)
</script>
