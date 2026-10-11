<template>
  <section
    v-if="progress || store.pullProgress"
    class="deployment-progress surface"
    aria-label="Deployment progress"
  >
    <div class="progress-heading">
      <div>
        <p class="eyebrow">Current operation</p>
        <p role="status">
          {{
            progress?.error || progress?.message || "Pulling container images"
          }}
        </p>
      </div>
      <span
        class="tag"
        :class="{
          'text-error': progress?.type === 'stack_error' || progress?.errors,
        }"
        >{{ phase }}</span
      >
    </div>
    <details v-if="store.pullProgress" class="pull-progress-section">
      <summary>
        Image pull details
        <span class="muted"
          >· {{ store.pullProgress.stack }} ·
          {{ store.pullProgress.current }}/{{ store.pullProgress.total }}</span
        >
      </summary>
      <div
        v-for="img in store.pullProgress.images"
        :key="img.image"
        class="pull-image"
        :data-image="img.image"
        :data-done="img.done"
      >
        <div class="pull-image-heading">
          <code>{{ img.image }}</code
          ><span :class="img.done ? 'text-success' : 'muted'">{{
            img.done ? "Complete" : img.status
          }}</span>
        </div>
        <div
          v-for="layer in img.layers"
          :key="layer.id"
          class="pull-layer"
          :data-layer-id="layer.id"
          :data-bytes="layer.bytesCurrent"
        >
          <div class="pull-layer-label">
            <code>{{ layer.id }}</code
            ><span>{{ layer.status }}</span
            ><span>{{
              layer.bytesTotal > 0
                ? `${formatBytes(layer.bytesCurrent)} / ${formatBytes(layer.bytesTotal)}`
                : layer.progress
            }}</span>
          </div>
          <progress
            v-if="layer.bytesTotal > 0"
            :value="layer.bytesCurrent"
            :max="layer.bytesTotal"
            :aria-label="`${img.image} layer ${layer.id} ${layer.status}`"
          />
        </div>
      </div>
    </details>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useStacksStore } from '../store/stacks'

const store = useStacksStore()
const progress = computed(() => store.updateProgress)
const phase = computed(() => {
  if (progress.value?.type === 'completed' && progress.value.errors) return 'Completed with errors'
  if (store.pullProgress) return 'Pulling images'
  const labels: Record<string, string> = {
    started: 'Starting',
    stack_progress: 'Updating',
    stack_sync_started: 'Preparing',
    stack_deploying: 'Deploying',
    pruning: 'Cleaning up',
    completed: 'Complete',
    stack_success: 'Deployed',
    stack_error: 'Failed',
    cancelled: 'Cancelled',
  }
  return labels[progress.value?.type || ''] || 'In progress'
})
function formatBytes(bytes: number) {
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`
}
</script>

<style scoped>
.deployment-progress {
  padding: 18px 22px;
  margin-bottom: 22px;
  border-left: 3px solid var(--accent);
}
.progress-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
}
.progress-heading .eyebrow {
  margin-bottom: 5px;
}
.pull-progress-section {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--border-color);
}
.pull-progress-section summary {
  font-size: 12px;
  overflow-wrap: anywhere;
}
.pull-image {
  margin-top: 16px;
}
.pull-image-heading,
.pull-layer-label {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
}
.pull-layer {
  padding: 10px 0 0 16px;
}
.pull-layer-label {
  justify-content: flex-start;
  color: var(--text-secondary);
}
.pull-layer-label span:last-child {
  margin-left: auto;
  font-family: var(--font-mono);
  font-size: 11px;
}
progress {
  width: 100%;
  height: 5px;
  display: block;
  margin-top: 8px;
  accent-color: var(--accent);
}
@media (max-width: 640px) {
  .deployment-progress {
    padding: 16px;
  }
  .progress-heading {
    align-items: flex-start;
  }
  .pull-layer {
    padding-left: 0;
  }
}
</style>
