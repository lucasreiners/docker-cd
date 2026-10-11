<template>
  <div>
    <div class="page-heading">
      <div>
        <p class="eyebrow">Resources</p>
        <h1>
          Stacks <span class="count-label">{{ store.stacks.length }}</span>
        </h1>
        <p>Manage deployments and inspect the services behind them.</p>
      </div>
      <router-link to="/repository" class="button"
        ><GitBranch :size="15" />Repository</router-link
      >
    </div>
    <div class="saved-views" role="group" aria-label="Saved stack views">
      <button
        v-for="item in views"
        :key="item.value"
        :aria-pressed="filter === item.value"
        @click="setQuery({ status: item.value || undefined })"
      >
        {{ item.label
        }}<span v-if="item.value === 'attention'">{{ attentionCount }}</span>
      </button>
    </div>
    <div class="table-toolbar">
      <label class="search-field"
        ><Search :size="15" /><input
          type="search"
          :value="query"
          aria-label="Search stacks"
          placeholder="Search stacks…"
          @input="
            setQuery({
              q: ($event.target as HTMLInputElement).value || undefined,
            })
          " /></label
      ><label class="select-label"
        ><span class="sr-only">Group stacks</span
        ><select
          :value="group"
          @change="
            setQuery({
              group: ($event.target as HTMLSelectElement).value || undefined,
            })
          "
        >
          <option value="">No grouping</option>
          <option value="path">Group by directory</option>
        </select></label
      ><MultiSelect
        v-model="columns"
        :options="columnOptions"
        option-label="label"
        option-value="value"
        placeholder="Columns"
        aria-label="Visible columns"
        :max-selected-labels="0"
        selected-items-label="Columns ({0})"
        class="column-picker"
      />
      <div class="layout-switch" role="group" aria-label="Stack layout">
        <button
          :aria-pressed="view === 'list'"
          aria-label="List"
          @click="setQuery({ view: undefined })"
        >
          <List :size="16" /></button
        ><button
          :aria-pressed="view === 'grid'"
          aria-label="Grid"
          @click="setQuery({ view: 'grid' })"
        >
          <LayoutGrid :size="16" />
        </button>
      </div>
    </div>
    <div v-if="query || filter" class="active-filters">
      <span>{{ displayed.length }} matching stacks</span
      ><button
        class="text-link"
        @click="setQuery({ q: undefined, status: undefined })"
      >
        Clear filters
      </button>
    </div>
    <div v-if="store.loading" class="skeleton" role="status">
      Loading stacks…
    </div>
    <div v-else-if="store.error" class="notice notice-error" role="alert">
      {{ store.error }}
      <button class="button" @click="store.loadInitial()">Retry</button>
    </div>
    <div v-else-if="!displayed.length" class="empty-state surface">
      <Boxes :size="30" />
      <h3>
        {{
          query || filter
            ? "No stacks match your filters"
            : "Your first deployment starts in Git"
        }}
      </h3>
      <p>
        {{
          query || filter
            ? "Try another name or saved view."
            : "Add a Compose file to your configured repository and check Git for changes."
        }}
      </p>
      <button
        v-if="query || filter"
        class="button"
        @click="setQuery({ q: undefined, status: undefined })"
      >
        Clear filters</button
      ><router-link v-else to="/repository" class="button"
        >Open repository</router-link
      >
    </div>
    <div
      v-else-if="view === 'grid'"
      class="stack-grid"
      data-testid="stacks-grid"
    >
      <StackCard
        v-for="stack in displayed"
        :key="stack.path"
        :stack="stack"
        :runtime="consoleStore.runtimeFor(stack.path)"
        :query="route.query"
      />
    </div>
    <div v-else class="surface stack-table" data-testid="stacks-grid">
      <DataTable
        :value="displayed"
        data-key="path"
        :paginator="displayed.length > 20"
        :rows="20"
        :rows-per-page-options="[20, 50, 100]"
        :size="consoleStore.density === 'compact' ? 'small' : undefined"
        :row-group-mode="group === 'path' ? 'subheader' : undefined"
        :group-rows-by="group === 'path' ? 'directory' : undefined"
      >
        <template #groupheader="{ data }"
          ><span class="directory-group"
            ><Folder :size="14" />{{
              data.directory || "Repository root"
            }}</span
          ></template
        >
        <Column field="path" header="Stack"
          ><template #body="{ data }"
            ><div class="stack-identity" :data-stack="data.path">
              <router-link
                class="resource-link"
                :to="{
                  name: 'stack-detail',
                  params: { path: data.path.split('/') },
                  query: route.query,
                }"
                >{{ data.path }}<ArrowUpRight :size="12"
              /></router-link>
              <p
                class="cell-subtitle"
                :class="{
                  'text-error':
                    data.lastSyncError && !data.requiresAcknowledgement,
                }"
                :title="data.lastSyncError || data.syncedCommitMessage"
              >
                {{
                  data.requiresAcknowledgement
                    ? "Configuration change awaiting approval"
                    : data.lastSyncError ||
                      data.syncedCommitMessage ||
                      "Waiting for deployment"
                }}
              </p>
            </div></template
          ></Column
        >
        <Column v-if="columns.includes('deployment')" header="Deployment"
          ><template #body="{ data }"
            ><StatusBadge
              :status="data.status"
              :acknowledgement="data.requiresAcknowledgement" /></template
        ></Column>
        <Column v-if="columns.includes('runtime')" header="Runtime"
          ><template #body="{ data }"
            ><div class="runtime-cell">
              <span
                >{{ data.containersRunning ?? "—" }} /
                {{ data.containersTotal ?? "—" }} running</span
              ><small
                :class="runtimeHealth(consoleStore.runtimeFor(data.path)).tone"
                >{{
                  runtimeHealth(consoleStore.runtimeFor(data.path)).label
                }}</small
              >
            </div></template
          ></Column
        >
        <Column
          v-if="columns.includes('revision')"
          header="Revision"
          class="secondary-column"
          ><template #body="{ data }"
            ><code>{{
              data.syncedRevision?.slice(0, 8) || "—"
            }}</code></template
          ></Column
        >
        <Column
          v-if="columns.includes('deployed')"
          header="Last deployed"
          class="secondary-column"
          ><template #body="{ data }"
            ><RelativeTime :value="data.syncedAt" /></template
        ></Column>
      </DataTable>
    </div>
    <div class="table-caption">
      <span>Runtime observed <RelativeTime :value="observedAt"/> · Deployment and health are independent.</span
      ><button class="text-link" @click="consoleStore.loadRuntime()">
        Refresh runtime
      </button>
    </div>
  </div>
</template>
<script setup lang="ts">
import { ArrowUpRight, Boxes, Folder, GitBranch, LayoutGrid, List, Search } from 'lucide-vue-next'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import MultiSelect from 'primevue/multiselect'
import { computed, ref, watch } from 'vue'
import { type LocationQueryRaw, useRoute, useRouter } from 'vue-router'
import RelativeTime from '../components/RelativeTime.vue'
import StackCard from '../components/StackCard.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { useConsoleStore } from '../store/console'
import { useStacksStore } from '../store/stacks'
import { needsAttention, runtimeHealth } from '../utils/stackState'

const store = useStacksStore(),
  consoleStore = useConsoleStore(),
  route = useRoute(),
  router = useRouter()
const query = computed(() => (typeof route.query.q === 'string' ? route.query.q : ''))
const filter = computed(() => (typeof route.query.status === 'string' ? route.query.status : ''))
const group = computed(() => (route.query.group === 'path' ? 'path' : ''))
const view = computed(() => (route.query.view === 'grid' ? 'grid' : 'list'))
const views = [
  { label: 'All stacks', value: '' },
  { label: 'Needs attention', value: 'attention' },
  { label: 'Deploying', value: 'syncing' },
  { label: 'No health checks', value: 'unchecked' },
]
const columnOptions = [
  { label: 'Deployment', value: 'deployment' },
  { label: 'Runtime', value: 'runtime' },
  { label: 'Revision', value: 'revision' },
  { label: 'Last deployed', value: 'deployed' },
]
function loadColumns() {
  try {
    const value = JSON.parse(localStorage.getItem('docker-cd-columns') || 'null')
    if (Array.isArray(value) && value.every((v) => columnOptions.some((c) => c.value === v)))
      return value as string[]
  } catch {}
  return columnOptions.map((c) => c.value)
}
const columns = ref(loadColumns())
const observedAt = computed(() =>
  Object.values(consoleStore.runtime)
    .map((o) => o.observedAt || '')
    .sort()
    .pop(),
)
watch(columns, (value) => localStorage.setItem('docker-cd-columns', JSON.stringify(value)))
function setQuery(values: LocationQueryRaw) {
  router.replace({ query: { ...route.query, ...values } })
}
const attentionCount = computed(
  () => store.stacks.filter((s) => needsAttention(s, consoleStore.runtimeFor(s.path))).length,
)
const displayed = computed(() =>
  store.stacks
    .filter(
      (s) =>
        s.path.toLowerCase().includes(query.value.toLowerCase()) &&
        (!filter.value ||
          (filter.value === 'attention'
            ? needsAttention(s, consoleStore.runtimeFor(s.path))
            : filter.value === 'unchecked'
              ? runtimeHealth(consoleStore.runtimeFor(s.path)).label === 'No health checks'
              : s.status === filter.value)),
    )
    .map((s) => ({
      ...s,
      directory: s.path.includes('/') ? s.path.slice(0, s.path.lastIndexOf('/')) : '',
    }))
    .sort(
      (a, b) =>
        (group.value ? a.directory.localeCompare(b.directory) : 0) || a.path.localeCompare(b.path),
    ),
)
</script>
