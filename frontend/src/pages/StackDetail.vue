<template>
  <div>
    <router-link
      class="back-link"
      :to="{ path: '/stacks', query: { ...route.query, tab: undefined, container: undefined } }"
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
        />
        <StackActions :stack="stack" :reviewed-revision="tab === 'configuration' ? reviewedRevision : null" :review-to="{ query: { ...route.query, tab: 'configuration' } }" @review="reviewAgain" />
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
      <StackWork :stack="stackPath" />
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
            Desired revision: <code>{{ stack.desiredRevision || store.refreshStatus?.revision || 'Unknown' }}</code>. Review the source comparison before approving.
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
                >{{ runtime.observedAt ? containers.filter(c => c.state === 'running').length : '—' }} of
                {{ runtime.observedAt ? containers.length : '—' }} running</strong
              >
              <p :class="health.tone">{{ health.label }}</p>
              <p class="inventory-secondary">Observed <RelativeTime :value="runtime.observedAt || undefined" />{{ runtime.stale ? ' · Stale observation' : '' }}</p>
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
        <section class="detail-container-preview surface"><div class="panel-heading"><h2>Observed containers</h2><router-link :to="{query: {...route.query, tab: 'containers'}}">Show all {{ containers.length }}</router-link></div><ul class="inventory-container-list"><ContainerRow v-for="c in containers.slice(0, 2)" :key="c.id" :container="c" :stack-path="stackPath" :query="route.query" :logs-available="!!consoleStore.instance?.capabilities.logs" @inspect="inspectContainer" /></ul><p v-if="!containers.length" class="padded">{{ containersError ? 'Runtime unavailable' : containersLoading ? 'Loading runtime…' : 'No containers observed' }}</p></section>
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
            <RefreshCw :size="14" />Refresh runtime
          </button>
        </div>
        <div v-if="containersLoading" class="skeleton" role="status">
          Loading containers…
        </div>
        <div
          v-if="containersError"
          class="notice notice-error"
          role="alert"
        >
          {{ containersError }}
        </div>
        <div v-if="!containers.length && !containersError && !containersLoading" class="empty-state surface">
          <h3>No containers found</h3>
        </div>
        <div v-if="containers.length" class="container-list">
          <article
            v-for="c in containers"
            :key="c.id"
            class="container-card surface"
          >
            <div class="container-header">
              <div>
                <button class="resource-link" @click="inspectContainer(c.id, $event.currentTarget as HTMLElement)">
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
                  <ImageIdentity :image="c.image" :identity="c.imageIdentity" />
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
          <button class="button" @click="loadStackOperations(false)">
            Refresh history
          </button>
        </div>
        <OperationsTable :records="stackHistory" />
        <button v-if="historyCursor" class="button" @click="loadStackOperations(true)">Load older stack operations</button>
      </section>
    </template>
    <ContainerInspector :selection="consoleStore.inspectedContainer?.stack === stackPath ? consoleStore.inspectedContainer : null" :current="consoleStore.inspectedContainerCurrent" :removed="consoleStore.inspectedContainerRemoved" :comparison="consoleStore.inspectedContainerIdentityComparison" :runtime="runtime" :trigger="inspectorTrigger" :query="route.query" :logs-available="!!consoleStore.instance?.capabilities.logs" @close="consoleStore.inspectedContainer = null" />
  </div>
</template>
<script setup lang="ts">
import { ArrowUpRight, Box, GitCommitHorizontal, RefreshCw } from 'lucide-vue-next'
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import ContainerInspector from '../components/ContainerInspector.vue'
import ContainerLogs from '../components/ContainerLogs.vue'
import ContainerRow from '../components/ContainerRow.vue'
import ImageIdentity from '../components/ImageIdentity.vue'
import OperationsTable from '../components/OperationsTable.vue'
import RelativeTime from '../components/RelativeTime.vue'
import SourceComparison from '../components/SourceComparison.vue'
import StackActions from '../components/StackActions.vue'
import StackWork from '../components/StackWork.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { useStackContainers } from '../composables/useStackContainers'
import { type Operation, requestJSON } from '../services/api'
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
const stack = computed(() => store.getStack(stackPath.value))
// Only the loaded comparison grants approval. Never bind this to the reactive
// desired revision, which may change while the operator reviews the diff.
const reviewedRevision = ref<string | null>(null)
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
const runtime = computed(() => consoleStore.runtimeFor(stackPath.value))
const health = computed(() =>
  runtime.value.stale
    ? { label: 'Runtime observation stale · showing last known state', tone: 'text-warning' }
    : runtimeHealth(runtime.value),
)
const inspectorTrigger = ref<HTMLElement | null>(null)
function inspectContainer(id: string, trigger: HTMLElement) {
  inspectorTrigger.value = trigger
  consoleStore.inspectContainer(stackPath.value, id)
}
const source = ref(''),
  sourceError = ref(''),
  error = ref('')
const appliedSource = ref<{ content: string; revision: string } | null>(null)
const stackOperations = computed(() =>
  consoleStore.operations.filter((o) => o.stack === stackPath.value),
)
const stackHistory = ref<Operation[]>([]),
  historyCursor = ref('')
let historyRequest = 0
async function loadStackOperations(append = false) {
  const request = ++historyRequest,
    path = stackPath.value
  try {
    const result = await requestJSON<{ records: Operation[]; nextCursor?: string }>(
      `/api/operations?stack=${encodeURIComponent(path)}&limit=50${append && historyCursor.value ? `&cursor=${encodeURIComponent(historyCursor.value)}` : ''}`,
    )
    if (request !== historyRequest || stackPath.value !== path) return
    stackHistory.value = [...(append ? stackHistory.value : []), ...result.records]
    historyCursor.value = result.nextCursor || ''
    for (const operation of result.records) consoleStore.onOperation(operation)
  } catch (e) {
    if (request === historyRequest && stackPath.value === path)
      error.value = e instanceof Error ? e.message : 'History unavailable'
  }
}
let sourceRequest = 0
watch(
  [stackPath, tab],
  async ([path, section]) => {
    error.value = ''
    if (section === 'activity') await loadStackOperations()
    if (section === 'configuration') {
      await loadSource(path)
    }
  },
  { immediate: true },
)
async function loadSource(path: string) {
  const request = ++sourceRequest
  reviewedRevision.value = null
  source.value = ''
  sourceError.value = ''
  appliedSource.value = null
  try {
    const data = await requestJSON<{
      revision?: string
      content: string
      applied?: { content: string; revision: string } | null
    }>(`/api/stack/configuration?path=${encodeURIComponent(path)}`)
    if (request !== sourceRequest || stackPath.value !== path || tab.value !== 'configuration')
      return
    source.value = data.content || '# No source content available'
    appliedSource.value = data.applied || null
    reviewedRevision.value = data.revision || null
  } catch (e) {
    if (request === sourceRequest && stackPath.value === path)
      sourceError.value = e instanceof Error ? e.message : 'Source unavailable'
  }
}
function reviewAgain() {
  if (tab.value === 'configuration') void loadSource(stackPath.value)
}
async function reloadContainers() {
  await consoleStore.loadRuntime()
}
function formatPort(port: PortMapping) {
  return `${port.external !== null ? `${port.external}:` : ''}${port.internal}/${port.protocol}`
}
</script>
