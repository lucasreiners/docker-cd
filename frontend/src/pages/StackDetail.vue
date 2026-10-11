<template>
  <div>
    <router-link
      class="back-link"
      :to="{ path: '/stacks', query: { ...route.query, tab: undefined } }"
      >← All stacks</router-link
    >
    <div class="page-heading">
      <div>
        <p class="eyebrow">Resources / {{ directory || "Repository root" }}</p>
        <h1>{{ name }}</h1>
        <p class="mono">{{ stackPath }}</p>
      </div>
      <div v-if="stack" class="page-actions">
        <StatusBadge
          :status="stack.status"
          :acknowledgement="stack.requiresAcknowledgement"
        /><button
          v-if="consoleStore.instance?.capabilities.apply"
          class="button"
          :disabled="
            busy ||
            stack.status === 'syncing' ||
            !!store.refreshStatus?.updatesBlocked
          "
          @click="
            action(stack.requiresAcknowledgement ? 'acknowledge' : 'retry')
          "
        >
          <RotateCw :size="14" />{{
            stack.requiresAcknowledgement
              ? "Acknowledge & deploy"
              : "Redeploy stack"
          }}</button
        ><button
          v-if="
            consoleStore.instance?.capabilities.imageUpdates &&
            !stack.requiresAcknowledgement
          "
          class="button button-primary"
          :disabled="
            busy ||
            stack.status === 'syncing' ||
            !!store.refreshStatus?.updatesBlocked
          "
          @click="action('update')"
        >
          Update images
        </button>
      </div>
    </div>
    <div v-if="error" class="notice notice-error" role="alert">{{ error }}</div>
    <div v-if="store.loading" class="skeleton" role="status">
      Loading stack…
    </div>
    <div v-else-if="!stack" class="empty-state surface">
      <h2>Stack not found</h2>
      <p>The stack "{{ stackPath }}" was not found in the current state.</p>
    </div>
    <template v-else>
      <nav class="resource-tabs" aria-label="Stack sections">
        <router-link
          v-for="item in tabs"
          :key="item.value"
          :to="{
            query: {
              ...route.query,
              tab: item.value === 'overview' ? undefined : item.value,
            },
          }"
          :aria-current="tab === item.value ? 'page' : undefined"
          >{{ item.label }}</router-link
        >
      </nav>
      <template v-if="tab === 'overview'">
        <div
          v-if="stack.requiresAcknowledgement"
          class="notice notice-warning"
          role="status"
        >
          <strong>Configuration change awaiting approval</strong>
          <p>
            {{
              stack.statusReason ||
              "The configured drift policy requires acknowledgement before applying this revision."
            }}
          </p>
          <p>
            Reviewed revision: <code>{{ reviewedRevision }}</code>
          </p>
        </div>
        <div
          v-else-if="stack.lastSyncError"
          class="notice notice-error"
          role="alert"
        >
          <strong>Deployment failed</strong>
          <p>{{ stack.lastSyncError }}</p>
        </div>
        <div class="detail-grid">
          <section class="surface">
            <div class="panel-heading">
              <h2>Deployment</h2>
              <GitCommitHorizontal :size="18" />
            </div>
            <dl class="definition-list padded">
              <div>
                <dt>Applied revision</dt>
                <dd>
                  <code>{{
                    stack.syncedRevision?.slice(0, 12) || "Not deployed"
                  }}</code>
                </dd>
              </div>
              <div>
                <dt>Latest change</dt>
                <dd>
                  {{
                    stack.syncedCommitMessage || "Waiting for first deployment"
                  }}
                </dd>
              </div>
              <div>
                <dt>Last deployed</dt>
                <dd><RelativeTime :value="stack.syncedAt" /></dd>
              </div>
              <div>
                <dt>Last attempt</dt>
                <dd><RelativeTime :value="stack.lastSyncAt" /></dd>
              </div>
              <div>
                <dt>Compose file</dt>
                <dd>
                  <code>{{ stack.composeFile }}</code>
                </dd>
              </div>
            </dl>
          </section>
          <section class="surface">
            <div class="panel-heading">
              <h2>Runtime</h2>
              <Box :size="18" />
            </div>
            <div class="padded runtime-summary">
              <strong
                >{{ stack.containersRunning ?? "—" }} of
                {{ stack.containersTotal ?? "—" }} running</strong
              >
              <p :class="health.tone">{{ health.label }}</p>
              <p class="muted">
                Deployment state and runtime health are independent.
              </p>
              <router-link
                class="text-link"
                :to="{ query: { ...route.query, tab: 'containers' } }"
                >Inspect containers →</router-link
              >
            </div>
          </section>
        </div>
        <div class="section-heading">
          <h2>Recent operations</h2>
          <router-link :to="{ query: { ...route.query, tab: 'activity' } }"
            >View all</router-link
          >
        </div>
        <OperationsTable :records="stackOperations.slice(0, 5)" />
      </template>
      <section v-else-if="tab === 'containers'">
        <div class="section-heading">
          <h2>
            Containers <span class="count-label">{{ containers.length }}</span>
          </h2>
          <button class="button" @click="reloadContainers">
            <RefreshCw :size="14" />Refresh
          </button>
        </div>
        <div v-if="containersLoading" class="skeleton" role="status">
          Loading containers…
        </div>
        <div
          v-else-if="containersError"
          class="notice notice-error"
          role="alert"
        >
          {{ containersError }}
        </div>
        <div v-else-if="!containers.length" class="empty-state surface">
          <h3>No containers found</h3>
        </div>
        <div v-else class="container-list">
          <article
            v-for="c in containers"
            :key="c.id"
            class="container-card surface"
          >
            <div class="container-header">
              <div>
                <button class="resource-link" @click="selectedContainer = c">
                  {{ c.service }}<ArrowUpRight :size="12" />
                </button>
                <p class="cell-subtitle">{{ c.name }}</p>
              </div>
              <div class="container-badges">
                <span class="tag">{{ c.state }}</span
                ><span
                  v-if="c.health && c.health !== 'none'"
                  class="tag"
                  :class="
                    c.health === 'unhealthy' ? 'text-error' : 'text-success'
                  "
                  >{{ c.health }}</span
                ><span v-else class="muted">No health check</span>
              </div>
            </div>
            <dl class="definition-list padded">
              <div>
                <dt>Image</dt>
                <dd>
                  <code>{{ c.image }}</code>
                </dd>
              </div>
              <div v-if="c.ports">
                <dt>Published ports</dt>
                <dd class="port-pills">
                  <template
                    v-for="(port, index) in parsePortString(c.ports)"
                    :key="index"
                    ><a
                      v-if="port.external !== null && port.protocol === 'tcp'"
                      class="tag port-pill"
                      :href="buildPortURL(port.external)"
                      target="_blank"
                      rel="noopener noreferrer"
                      :aria-label="`Open ${c.service} on port ${port.external}`"
                      >{{ formatPort(port) }} ↗</a
                    ><span v-else class="tag port-pill-internal">{{
                      formatPort(port)
                    }}</span></template
                  >
                </dd>
              </div>
              <div>
                <dt>Container ID</dt>
                <dd>
                  <code>{{ c.id }}</code>
                </dd>
              </div>
            </dl>
          </article>
        </div>
      </section>
      <section v-else-if="tab === 'configuration'" class="surface">
        <div class="panel-heading">
          <h2>{{ stack.composeFile }}</h2>
          <span class="tag">Read-only · desired state</span>
        </div>
        <div class="padded">
          <p v-if="sourceError" class="notice notice-error" role="alert">
            {{ sourceError }}
          </p>
          <SourceComparison v-else-if="source" :content="source" :applied="appliedSource"/>
          <p v-else class="muted">Loading Compose source…</p>
          <dl class="definition-list">
            <div>
              <dt>Desired Compose hash</dt>
              <dd>
                <code>{{ stack.composeHash }}</code>
              </dd>
            </div>
            <div>
              <dt>Applied Compose hash</dt>
              <dd>
                <code>{{ stack.syncedComposeHash || "Not deployed" }}</code>
              </dd>
            </div>
            <div>
              <dt>Applied Git revision</dt>
              <dd>
                <code>{{ stack.syncedRevision || "Not deployed" }}</code>
              </dd>
            </div>
          </dl>
        </div>
      </section>
      <ContainerLogs v-else-if="tab==='logs'" :key="stackPath" :stack-path="stackPath" :containers="containers"/>
      <section v-else>
        <div class="section-heading">
          <h2>Stack activity</h2>
          <button class="button" @click="loadStackOperations">
            Refresh history
          </button>
        </div>
        <OperationsTable :records="stackOperations" />
      </section>
    </template>
    <Drawer
      :visible="!!selectedContainer"
      header="Container inspector"
      position="right"
      style="width: min(510px, 100vw)"
      class="operation-drawer"
      @update:visible="
        (value) => {
          if (!value) selectedContainer = null;
        }
      "
      ><template v-if="selectedContainer"
        ><h2>{{ selectedContainer.name }}</h2>
        <dl class="definition-list">
          <div v-for="(value, key) in selectedContainer" :key="key">
            <dt>{{ key }}</dt>
            <dd>
              <code>{{ value }}</code>
            </dd>
          </div>
        </dl></template
      ></Drawer
    >
  </div>
</template>
<script setup lang="ts">
import { ArrowUpRight, Box, GitCommitHorizontal, RefreshCw, RotateCw } from 'lucide-vue-next'
import Drawer from 'primevue/drawer'
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import ContainerLogs from '../components/ContainerLogs.vue'
import OperationsTable from '../components/OperationsTable.vue'
import RelativeTime from '../components/RelativeTime.vue'
import SourceComparison from '../components/SourceComparison.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { useStackContainers } from '../composables/useStackContainers'
import { type ContainerInfo, fetchContainers, type Operation, requestJSON } from '../services/api'
import { useConsoleStore } from '../store/console'
import { useStacksStore } from '../store/stacks'
import { buildPortURL, type PortMapping, parsePortString } from '../utils/portUtils'
import { runtimeHealth } from '../utils/stackState'

const route = useRoute(),
  store = useStacksStore(),
  consoleStore = useConsoleStore()
const stackPath = computed(() =>
  Array.isArray(route.params.path) ? route.params.path.join('/') : route.params.path,
)
const name = computed(() => stackPath.value.split('/').pop()),
  directory = computed(() =>
    stackPath.value.includes('/') ? stackPath.value.slice(0, stackPath.value.lastIndexOf('/')) : '',
  )
const stack = computed(() => store.getStack(stackPath.value)),
  reviewedRevision = computed(
    () => stack.value?.desiredRevision || store.refreshStatus?.revision || '',
  )
const tabs = computed(() => [
  { label: 'Overview', value: 'overview' },
  { label: 'Containers', value: 'containers' },
  { label: 'Configuration', value: 'configuration' },
  { label: 'Activity', value: 'activity' },
  ...(consoleStore.instance?.capabilities.logs ? [{ label: 'Logs', value: 'logs' }] : []),
])
const tab = computed(() =>
  tabs.value.some((t) => t.value === route.query.tab) ? String(route.query.tab) : 'overview',
)
const { containers, containersLoading, containersError } = useStackContainers(() => stack.value)
const health = computed(() =>
  runtimeHealth({
    containers: containers.value,
    loading: containersLoading.value,
    error: containersError.value,
  }),
)
const selectedContainer = ref<ContainerInfo | null>(null),
  source = ref(''),
  sourceError = ref(''),
  busy = ref(false),
  error = ref('')
const appliedSource = ref<{ content: string; revision: string } | null>(null)
const stackOperations = computed(() =>
  consoleStore.operations.filter((o) => o.stack === stackPath.value),
)
async function loadStackOperations() {
  try {
    const result = await requestJSON<{ records: Operation[] }>(
      `/api/operations?stack=${encodeURIComponent(stackPath.value)}&limit=100`,
    )
    for (const operation of result.records) consoleStore.onOperation(operation)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'History unavailable'
  }
}
watch(
  [stackPath, tab],
  async ([path, section]) => {
    error.value = ''
    if (section === 'activity') await loadStackOperations()
    if (section === 'configuration') {
      source.value = ''
      sourceError.value = ''
      try {
        const data = await requestJSON<{
          content: string
          applied?: { content: string; revision: string } | null
        }>(`/api/stack/configuration?path=${encodeURIComponent(path)}`)
        if (stackPath.value === path) {
          source.value = data.content || '# No source content available'
          appliedSource.value = data.applied || null
        }
      } catch (e) {
        if (stackPath.value === path)
          sourceError.value = e instanceof Error ? e.message : 'Source unavailable'
      }
    }
  },
  { immediate: true },
)
async function action(actionName: string) {
  busy.value = true
  error.value = ''
  try {
    const result = await requestJSON<{ operationId: string }>('/api/operations', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        action: actionName,
        stack: stackPath.value,
        revision: reviewedRevision.value,
      }),
    })
    await consoleStore.inspect(result.operationId)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Action failed'
  } finally {
    busy.value = false
  }
}
async function reloadContainers() {
  containersLoading.value = true
  try {
    containers.value = await fetchContainers(stackPath.value)
    containersError.value = null
  } catch (e) {
    containersError.value = e instanceof Error ? e.message : 'Runtime unavailable'
  } finally {
    containersLoading.value = false
  }
}
function formatPort(port: PortMapping) {
  return `${port.external !== null ? `${port.external}:` : ''}${port.internal}/${port.protocol}`
}
</script>
