<template>
  <div class="surface operations-table">
    <div v-if="!records.length" class="empty-state">
      <h3>No operations recorded</h3>
      <p>Repository checks, deployments and image updates will appear here.</p>
    </div>
    <DataTable
      v-else
      :value="records"
      data-key="id"
      :size="consoleStore.density === 'compact' ? 'small' : undefined"
      :paginator="records.length > 20"
      :rows="20"
      :rows-per-page-options="[20, 50, 100]"
    >
      <Column header="Operation"
        ><template #body="{ data }"
          ><button
            class="resource-link"
            :data-operation-id="data.id"
            @click="consoleStore.inspect(data.id)"
          >
            {{ names[data.kind] || data.kind }}<ArrowUpRight :size="12" />
          </button>
          <p class="cell-subtitle">
            {{ data.stack || "Instance-wide" }}
          </p><router-link class="text-link" :to="`/operations/${encodeURIComponent(data.id)}`" :aria-label="`Open full operation ${data.id}`">Full record →</router-link></template
        ></Column
      >
      <Column header="Result"
        ><template #body="{ data }"
          ><StatusBadge :status="data.status" /></template
      ></Column>
      <Column header="Trigger" class="secondary-column"
        ><template #body="{ data }"
          ><span class="tag">{{ data.trigger }}</span></template
        ></Column
      >
      <Column header="Started" class="secondary-column"
        ><template #body="{ data }"
          ><RelativeTime :value="data.startedAt" /></template
      ></Column>
      <Column header="Duration" class="secondary-column"
        ><template #body="{ data }">{{ duration(data) }}</template></Column
      >
    </DataTable>
  </div>
</template>
<script setup lang="ts">
import { ArrowUpRight } from 'lucide-vue-next'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import type { Operation } from '../services/api'
import { useConsoleStore } from '../store/console'
import RelativeTime from './RelativeTime.vue'
import StatusBadge from './StatusBadge.vue'

defineProps<{ records: Operation[] }>()
const consoleStore = useConsoleStore()
const names: Record<string, string> = {
  webhook: 'Webhook delivery',
  deployment: 'Stack deployment',
  image_update: 'Image update',
  refresh: 'Repository check',
  action: 'Stack action',
  removal: 'Stack removal',
}
function duration(operation: Operation) {
  if (!operation.finishedAt) return 'In progress'
  const seconds = Math.max(
    0,
    Math.round((Date.parse(operation.finishedAt) - Date.parse(operation.startedAt)) / 1000),
  )
  return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}
</script>
