<template>
  <div>
    <div class="page-heading">
      <div>
        <p class="eyebrow">Execution</p>
        <h1>Operations</h1>
        <p>Follow active work and investigate previous results.</p>
      </div>
      <button
        class="button button-primary"
        :disabled="
          busy ||
          store.isUpdating ||
          !consoleStore.instance?.capabilities.imageUpdates ||
          !!store.refreshStatus?.updatesBlocked
        "
        @click="update"
      >
        <RefreshCw :size="14" />{{
          busy || store.isUpdating
            ? "Updating images…"
            : "Update container images"
        }}
      </button>
    </div>
    <div v-if="error" class="notice notice-error" role="alert">{{ error }}</div>
    <p v-if="message" class="notice" role="status">{{ message }}</p>
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
    <DeploymentProgress />
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
      v-if="consoleStore.operationsError"
      class="notice notice-error"
      role="alert"
    >
      {{ consoleStore.operationsError }}
    </div>
    <OperationsTable :records="records" />
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
        v-if="consoleStore.nextCursor"
        class="button"
        @click="consoleStore.loadOperations(true)"
      >
        Load older operations
      </button>
    </div>
  </div>
</template>
<script setup lang="ts">
import { ArrowUpRight, Clock3, RefreshCw } from 'lucide-vue-next'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DeploymentProgress from '../components/DeploymentProgress.vue'
import OperationsTable from '../components/OperationsTable.vue'
import RelativeTime from '../components/RelativeTime.vue'
import { triggerUpdate } from '../services/api'
import { useConsoleStore } from '../store/console'
import { useStacksStore } from '../store/stacks'

const consoleStore = useConsoleStore(),
  store = useStacksStore(),
  route = useRoute(),
  router = useRouter()
const error = ref(''),
  message = ref(''),
  busy = ref(false)
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
const records = computed(() =>
  consoleStore.operations.filter(
    (o) =>
      (!route.query.kind || o.kind === route.query.kind) &&
      (!route.query.status || o.status === route.query.status),
  ),
)
function choose(item: (typeof filters)[number]) {
  router.replace({
    query: { kind: item.kind || undefined, status: item.status || undefined },
  })
}
async function update() {
  busy.value = true
  error.value = ''
  try {
    await triggerUpdate()
    message.value = 'Image update requested. Progress and results appear below.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Image update failed'
  } finally {
    busy.value = false
  }
}
</script>
