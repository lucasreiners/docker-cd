<template>
  <div>
    <div class="page-heading">
      <div>
        <p class="eyebrow">Execution</p>
        <h1>Operations</h1>
        <p>Follow active work and investigate previous results.</p>
      </div>
      <WorkspaceActions :show-runtime="false" :show-git="false" />
    </div>
    <div class="schedule-strip surface">
      <Clock3 :size="18" />
      <div>
        <strong>{{
          consoleStore.instance?.schedule.enabled
            ? "Scheduled image updates"
            : "Scheduled updates disabled"
        }}</strong>
        <p v-if="consoleStore.instance?.schedule.enabled">
          <code>{{ consoleStore.instance.schedule.cron }}</code> ·
          {{ consoleStore.instance.schedule.timezone }} · Next
          <RelativeTime :value="consoleStore.instance.schedule.nextRun" />
        </p>
        <p v-else>
          Enable UPDATER_ENABLED in the agent environment to use image updates.
        </p>
      </div>
      <router-link to="/system"
        >Configuration <ArrowUpRight :size="12"
      /></router-link>
    </div>
    <ActiveWork />
    <div class="saved-views" role="group" aria-label="Operation views">
      <button
        v-for="item in filters"
        :key="item.label"
        :aria-pressed="selected === item.label"
        @click="choose(item)"
      >
        {{ item.label }}
      </button>
    </div>
    <div
      v-if="historyError"
      class="notice notice-error"
      role="alert"
    >
      {{ historyError }} <button class="text-link" @click="loadHistory(false)">Retry history</button>
    </div>
    <p v-if="historyLoading" role="status">Loading history…</p>
    <div class="section-heading"><h2>Operation history</h2><button class="button" :disabled="historyLoading" @click="loadHistory(false)">Refresh history</button></div>
    <OperationsTable :records="displayRecords" />
    <div class="table-caption">
      <span
        >{{
          consoleStore.instance?.history.persistent
            ? "History retained on the agent"
            : "History available for this agent session"
        }}
        · Up to {{ consoleStore.instance?.history.retention || 1000 }} completed
        operations</span
      ><button
        v-if="nextCursor"
        class="button"
        :disabled="historyLoading"
        @click="loadHistory(true)"
      >
        Load older operations
      </button>
    </div>
  </div>
</template>
<script setup lang="ts">
import { ArrowUpRight, Clock3 } from 'lucide-vue-next'
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ActiveWork from '../components/ActiveWork.vue'
import OperationsTable from '../components/OperationsTable.vue'
import RelativeTime from '../components/RelativeTime.vue'
import WorkspaceActions from '../components/WorkspaceActions.vue'
import { type Operation, requestJSON } from '../services/api'
import { useConsoleStore } from '../store/console'

const consoleStore = useConsoleStore(),
  route = useRoute(),
  router = useRouter()
const filters = [
  { label: 'All operations', kind: '', status: '' },
  { label: 'Active', kind: '', status: 'running' },
  { label: 'Deployments', kind: 'deployment', status: '' },
  { label: 'Image updates', kind: 'image_update', status: '' },
  { label: 'Repository checks', kind: 'refresh', status: '' },
  { label: 'Webhooks', kind: 'webhook', status: '' },
]
const selected = computed(
  () =>
    filters.find(
      (f) => f.kind === (route.query.kind || '') && f.status === (route.query.status || ''),
    )?.label || 'All operations',
)
const records = ref<Operation[]>([]),
  historyLoading = ref(false),
  historyError = ref(''),
  nextCursor = ref('')
let generation = 0
let requestSequence = 0
const displayRecords = computed(() =>
  records.value
    .map((record) => {
      const live = consoleStore.operationMap[record.id]
      return live && (live.version || 0) >= (record.version || 0) ? live : record
    })
    .filter((record) => !route.query.status || record.status === route.query.status),
)
watch(
  () => [route.query.kind, route.query.status, route.query.stack],
  () => {
    generation++
    records.value = []
    nextCursor.value = ''
    void loadHistory(false)
  },
  { immediate: true },
)
async function loadHistory(append: boolean) {
  const current = generation
  const request = ++requestSequence
  const isCurrent = () => current === generation && request === requestSequence
  historyLoading.value = true
  historyError.value = ''
  const query = new URLSearchParams({ limit: '50' })
  for (const key of ['kind', 'status', 'stack']) {
    const value = route.query[key]
    if (typeof value === 'string' && value) query.set(key, value)
  }
  if (append && nextCursor.value) query.set('cursor', nextCursor.value)
  try {
    const result = await requestJSON<{ records: Operation[]; nextCursor?: string }>(
      `/api/operations?${query}`,
    )
    if (!isCurrent()) return
    records.value = [
      ...new Map(
        [...(append ? records.value : []), ...result.records].map((op) => [op.id, op]),
      ).values(),
    ]
    nextCursor.value = result.nextCursor || ''
  } catch (error) {
    if (isCurrent())
      historyError.value = error instanceof Error ? error.message : 'History unavailable'
  } finally {
    if (isCurrent()) historyLoading.value = false
  }
}
function choose(item: (typeof filters)[number]) {
  router.replace({
    query: { ...route.query, kind: item.kind || undefined, status: item.status || undefined },
  })
}
</script>
