<template>
  <div>
    <div class="saved-views" role="group" aria-label="Configuration view">
      <button :aria-pressed="view === 'source'" @click="view = 'source'">
        Desired source</button
      ><button :aria-pressed="view === 'diff'" @click="view = 'diff'">
        Changes from applied
      </button>
    </div>
    <pre v-if="view === 'source'" class="source-code">{{ content }}</pre>
    <div v-else-if="!applied" class="notice notice-warning">
      No applied source has been recorded yet. The next successful deployment
      will capture it for comparison.
    </div>
    <div v-else-if="!changes" class="notice notice-warning">
      This comparison exceeds the interactive diff limit. Inspect the desired
      source and applied revision in Git.
    </div>
    <div v-else>
      <p class="configuration-note">
        Applied revision <code>{{ applied.revision.slice(0, 12) }}</code> →
        desired configuration
      </p>
      <p
        v-if="!changes.some((change) => change.added || change.removed)"
        class="notice"
      >
        No configuration changes.
      </p>
      <div class="source-diff" aria-label="Compose configuration changes">
        <div
          v-for="(change, index) in changes"
          :key="index"
          :class="
            change.added ? 'diff-added' : change.removed ? 'diff-removed' : ''
          "
        >
          <span aria-hidden="true">{{
            change.added ? "+" : change.removed ? "−" : " "
          }}</span
          ><span class="sr-only">{{
            change.added ? "Added: " : change.removed ? "Removed: " : ""
          }}</span
          ><code>{{ change.value }}</code>
        </div>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { diffLines } from 'diff'
import { computed, ref } from 'vue'

const props = defineProps<{
  content: string
  applied?: { content: string; revision: string } | null
}>()
const view = ref('source')
const changes = computed(() =>
  !props.applied || props.content.length > 200000 || props.applied.content.length > 200000
    ? null
    : diffLines(props.applied.content, props.content, {
        oneChangePerToken: true,
        timeout: 100,
        maxEditLength: 2000,
      }),
)
</script>
<style scoped>
.configuration-note {
  margin-bottom: 16px;
}
.source-diff {
  max-height: 550px;
  overflow: auto;
  border: 1px solid var(--border-color);
  border-radius: 6px;
  background: var(--bg-primary);
  margin-bottom: 22px;
}
.source-diff > div {
  display: flex;
  padding: 0 12px;
  gap: 14px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  min-height: 22px;
}
.source-diff code {
  font-size: 11px;
}
.source-diff > div > span:first-child {
  min-width: 10px;
  user-select: none;
}
.diff-added {
  background: color-mix(in srgb, var(--success-text) 12%, transparent);
  color: var(--success-text);
}
.diff-removed {
  background: var(--error-bg);
  color: var(--error-text);
}
</style>
