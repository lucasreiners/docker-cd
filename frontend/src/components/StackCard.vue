<template>
  <article
    class="stack-card"
    :class="[`stack-card--${stack.status}`, { 'stack-row': compact }]"
    :aria-label="stack.path"
    :data-stack="stack.path"
  >
    <div class="stack-identity">
      <router-link
        class="stack-name"
        :to="{
          name: 'stack-detail',
          params: { path: stack.path.split('/') },
          query,
        }"
        >{{ stack.path }}<span aria-hidden="true">↗</span></router-link
      >
      <p
        v-if="stack.lastSyncError"
        class="error-section"
        :title="stack.lastSyncError"
      >
        {{ stack.lastSyncError }}
      </p>
      <p v-else class="stack-message" :title="stack.syncedCommitMessage">
        {{ stack.syncedCommitMessage || "Waiting for first deployment" }}
      </p>
    </div>
    <div class="stack-deployment">
      <span class="cell-label">Deployment</span>
      <StatusBadge :status="stack.status" :acknowledgement="stack.requiresAcknowledgement" />
      <code v-if="stack.syncedRevision" :title="stack.syncedRevision">{{
        stack.syncedRevision.slice(0, 8)
      }}</code>
    </div>
    <div class="stack-runtime">
      <span class="cell-label">Runtime</span>
      <span
        v-if="stack.containersTotal != null && stack.containersRunning != null"
        class="runtime-count"
        :class="{
          'text-error': stack.containersRunning < stack.containersTotal,
        }"
        >{{ stack.containersRunning }} of
        {{ stack.containersTotal }} running</span
      >
      <span v-else class="muted">Runtime unknown</span>
      <span class="health-label" :class="health.tone">{{ health.label }}</span>
    </div>
    <div class="stack-time">
      <span class="cell-label">Last deployed</span>
      <RelativeTime :value="stack.syncedAt" />
    </div>
    <div v-if="!compact" class="containers-section">
      <p v-if="state.loading" class="muted">Loading containers…</p>
      <p v-else-if="state.error" class="text-warning">
        Failed to load containers
      </p>
      <ul v-else class="containers-list">
        <li
          v-for="container in sortedContainers"
          :key="container.id"
          class="container-item"
        >
          <a
            v-if="container.port !== null"
            class="container-link"
            :href="buildPortURL(container.port)"
            target="_blank"
            rel="noopener noreferrer"
            :title="`Open ${container.name} on port ${container.port}`"
            ><span class="underlined">{{ container.name }}</span
            ><span aria-hidden="true">↗</span></a
          >
          <span v-else class="container-text">{{ container.name }}</span>
        </li>
      </ul>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { LocationQuery } from 'vue-router'
import type { RuntimeState } from '../composables/useRuntimeOverview'
import { useStackContainers } from '../composables/useStackContainers'
import type { StackRecord } from '../services/api'
import { buildPortURL, getLowestExternalPort } from '../utils/portUtils'
import { runtimeHealth } from '../utils/stackState'
import RelativeTime from './RelativeTime.vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{
  stack: StackRecord
  compact?: boolean
  runtime?: RuntimeState
  query?: LocationQuery
}>()
const local = useStackContainers(() => (props.runtime ? undefined : props.stack))
const state = computed<RuntimeState>(
  () =>
    props.runtime ?? {
      containers: local.containers.value,
      loading: local.containersLoading.value,
      error: local.containersError.value,
    },
)
const health = computed(() => runtimeHealth(state.value))
const sortedContainers = computed(() =>
  state.value.containers
    .map((c) => ({ ...c, port: getLowestExternalPort(c.ports || '') }))
    .sort((a, b) => a.name.localeCompare(b.name)),
)
</script>

<style scoped>
.stack-card {
  min-width: 0;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
  padding: 22px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: 8px;
}
.stack-identity {
  grid-column: 1 / -1;
  min-width: 0;
}
.stack-name {
  display: inline-flex;
  align-items: baseline;
  gap: 10px;
  max-width: 100%;
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  text-decoration: none;
  overflow-wrap: anywhere;
}
.stack-name span {
  color: var(--text-tertiary);
  font-size: 12px;
}
.stack-name:hover {
  color: var(--accent);
  text-decoration: underline;
}
.stack-message,
.error-section {
  margin-top: 5px;
  font-size: 12px;
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.error-section {
  color: var(--error-text);
}
.stack-deployment,
.stack-runtime,
.stack-time {
  display: flex;
  align-items: flex-start;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}
.stack-deployment code {
  color: var(--text-tertiary);
  font-size: 11px;
}
.cell-label {
  font-size: 11px;
  color: var(--text-tertiary);
  margin-bottom: 3px;
}
.health-label {
  font-size: 12px;
  color: var(--text-tertiary);
}
.health-label.text-error {
  color: var(--error-text);
}
.health-label.text-warning {
  color: var(--warning-text);
}
.health-label.text-success {
  color: var(--success-text);
}
.stack-time {
  grid-column: 1 / -1;
  font-size: 12px;
  color: var(--text-secondary);
}
.containers-section {
  grid-column: 1 / -1;
  border-top: 1px solid var(--border-color);
  padding-top: 12px;
  font-size: 12px;
}
.containers-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
}
.container-link {
  display: inline-flex;
  gap: 6px;
  align-items: center;
}
.container-text {
  color: var(--text-secondary);
}
.stack-row {
  grid-template-columns: minmax(180px, 1fr) 150px 185px 140px;
  align-items: center;
  gap: 20px;
  border: 0;
  border-bottom: 1px solid var(--border-color);
  border-radius: 0;
  padding: 19px 22px;
}
.stack-row:last-child {
  border-bottom: 0;
}
.stack-row:hover {
  background: color-mix(in srgb, var(--bg-inset) 40%, var(--bg-secondary));
}
.stack-row .stack-identity,
.stack-row .stack-time {
  grid-column: auto;
}
.stack-row .cell-label {
  display: none;
}
@media (max-width: 1000px) {
  .stack-row {
    grid-template-columns: minmax(150px, 1fr) 130px 165px;
  }
  .stack-row .stack-time {
    display: none;
  }
}
@media (max-width: 640px) {
  .stack-row {
    grid-template-columns: 1fr 1fr;
    gap: 16px;
    padding: 18px 16px;
  }
  .stack-row .stack-identity {
    grid-column: 1 / -1;
  }
  .stack-row .cell-label {
    display: block;
  }
  .stack-card {
    padding: 18px 16px;
  }
}
</style>
