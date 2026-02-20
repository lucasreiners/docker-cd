<template>
  <n-tag
    :type="tagType"
    :bordered="false"
    size="small"
    round
  >
    <template #icon>
      <n-icon :component="statusIcon" />
    </template>
    {{ label }}
  </n-tag>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { computed, h } from 'vue'

const props = defineProps<{
  status: string
}>()

// Simple SVG icon components for each status
const CheckCircle: Component = {
  render() {
    return h('svg', { viewBox: '0 0 24 24', width: '1em', height: '1em', fill: 'currentColor' }, [
      h('path', {
        d: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z',
      }),
    ])
  },
}

const SyncIcon: Component = {
  render() {
    return h('svg', { viewBox: '0 0 24 24', width: '1em', height: '1em', fill: 'currentColor' }, [
      h('path', {
        d: 'M12 4V1L8 5l4 4V6c3.31 0 6 2.69 6 6 0 1.01-.25 1.97-.7 2.8l1.46 1.46C19.54 15.03 20 13.57 20 12c0-4.42-3.58-8-8-8zm0 14c-3.31 0-6-2.69-6-6 0-1.01.25-1.97.7-2.8L5.24 7.74C4.46 8.97 4 10.43 4 12c0 4.42 3.58 8 8 8v3l4-4-4-4v3z',
      }),
    ])
  },
}

const ErrorIcon: Component = {
  render() {
    return h('svg', { viewBox: '0 0 24 24', width: '1em', height: '1em', fill: 'currentColor' }, [
      h('path', {
        d: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z',
      }),
    ])
  },
}

const QuestionIcon: Component = {
  render() {
    return h('svg', { viewBox: '0 0 24 24', width: '1em', height: '1em', fill: 'currentColor' }, [
      h('path', {
        d: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 17h-2v-2h2v2zm2.07-7.75l-.9.92C13.45 12.9 13 13.5 13 15h-2v-.5c0-1.1.45-2.1 1.17-2.83l1.24-1.26c.37-.36.59-.86.59-1.41 0-1.1-.9-2-2-2s-2 .9-2 2H8c0-2.21 1.79-4 4-4s4 1.79 4 4c0 .88-.36 1.68-.93 2.25z',
      }),
    ])
  },
}

const DeleteIcon: Component = {
  render() {
    return h('svg', { viewBox: '0 0 24 24', width: '1em', height: '1em', fill: 'currentColor' }, [
      h('path', {
        d: 'M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z',
      }),
    ])
  },
}

type TagType = 'success' | 'warning' | 'error' | 'info' | 'default'

const tagType = computed<TagType>(() => {
  switch (props.status) {
    case 'synced':
      return 'success'
    case 'syncing':
      return 'warning'
    case 'failed':
      return 'error'
    case 'missing':
      return 'info'
    case 'deleting':
      return 'warning'
    default:
      return 'default'
  }
})

const statusIcon = computed<Component>(() => {
  switch (props.status) {
    case 'synced':
      return CheckCircle
    case 'syncing':
      return SyncIcon
    case 'failed':
      return ErrorIcon
    case 'missing':
      return QuestionIcon
    case 'deleting':
      return DeleteIcon
    default:
      return QuestionIcon
  }
})

const label = computed(() => {
  return props.status.charAt(0).toUpperCase() + props.status.slice(1)
})
</script>
