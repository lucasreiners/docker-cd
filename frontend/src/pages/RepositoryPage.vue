<template>
  <div>
    <div class="page-heading">
      <div>
        <p class="eyebrow">Administration</p>
        <h1>Repository</h1>
        <p>The source of truth for this deployment environment.</p>
      </div>
      <button
        class="button button-primary"
        :disabled="busy || store.refreshStatus?.refreshStatus === 'refreshing'"
        @click="refresh"
      >
        <RefreshCw :size="14" />{{
          busy ? "Checking…" : "Check Git for changes"
        }}
      </button>
    </div>
    <div v-if="error" class="notice notice-error" role="alert">{{ error }}</div>
    <p v-if="message" class="notice" role="status">{{ message }}</p>
    <div
      v-if="
        store.isUpdating || store.stacks.some((s) => s.status === 'syncing')
      "
      class="notice notice-warning"
    >
      A manual Git check requests cancellation of active deployment/image-update
      work before refreshing.
    </div>
    <div
      v-if="store.refreshStatus?.updatesBlocked"
      class="notice notice-error"
      role="alert"
    >
      <strong>Deployments blocked</strong>
      <p>
        {{
          store.refreshStatus.refreshError || store.refreshStatus.blockedReason
        }}
      </p>
    </div>
    <div class="repository-columns">
      <section class="surface">
        <div class="panel-heading">
          <h2>Source configuration</h2>
          <StatusBadge
            v-if="store.refreshStatus"
            :status="store.refreshStatus.refreshStatus"
          />
        </div>
        <dl class="definition-list padded">
          <div>
            <dt>Repository</dt>
            <dd>
              <a
                v-if="repoURL"
                :href="repoURL"
                target="_blank"
                rel="noopener noreferrer"
                >{{ consoleStore.instance?.repository.url }} ↗</a
              ><span v-else>{{
                consoleStore.instance?.repository.url || "Not available"
              }}</span>
            </dd>
          </div>
          <div>
            <dt>Tracked ref</dt>
            <dd>
              <code>{{
                consoleStore.instance?.repository.ref ||
                store.refreshStatus?.ref
              }}</code>
            </dd>
          </div>
          <div>
            <dt>Deploy directory</dt>
            <dd>
              <code>{{
                consoleStore.instance?.repository.deployDir || "/"
              }}</code>
            </dd>
          </div>
          <div>
            <dt>Observed revision</dt>
            <dd>
              <code>{{
                store.refreshStatus?.revision || "Not yet checked"
              }}</code>
            </dd>
          </div>
          <div>
            <dt>Latest commit</dt>
            <dd>{{ store.refreshStatus?.commitMessage || "—" }}</dd>
          </div>
          <div>
            <dt>Last successful check</dt>
            <dd><RelativeTime :value="store.refreshStatus?.refreshedAt" /></dd>
          </div>
          <div>
            <dt>Local clone</dt>
            <dd>
              <code>{{ store.refreshStatus?.localPath || "/repo" }}</code>
            </dd>
          </div>
        </dl>
      </section>
      <section class="surface">
        <div class="panel-heading">
          <h2>Delivery triggers</h2>
          <Webhook :size="17" />
        </div>
        <div class="padded">
          <h3>GitHub webhook</h3>
          <p class="muted">
            Send push events as JSON to your publicly reachable agent URL with
            the path below.
          </p>
          <code class="code-block">POST /api/webhook</code>
          <p class="configuration-note">
            {{
              consoleStore.instance?.repository.webhookConfigured
                ? "Signature verification configured"
                : "Webhook signature secret not configured"
            }}
          </p>
          <h3 class="section-label">Periodic checks</h3>
          <p>
            {{
              consoleStore.instance?.repository.pollInterval === "0s"
                ? "Periodic refresh disabled"
                : `Every ${consoleStore.instance?.repository.pollInterval || "—"}`
            }}
          </p>
          <p class="muted">
            Periodic checks queue behind active deployment work.
          </p>
          <router-link to="/system" class="text-link"
            >View effective configuration →</router-link
          >
        </div>
      </section>
    </div>
    <div class="section-heading">
      <h2>Discovered stacks</h2>
      <span class="muted">{{ store.stacks.length }} Compose definitions</span>
    </div>
    <div class="surface directory-list">
      <router-link
        v-for="stack in sortedStacks"
        :key="stack.path"
        :to="{ name: 'stack-detail', params: { path: stack.path.split('/') } }"
        ><Folder :size="16" /><span
          >{{ stack.path }}<small>{{ stack.composeFile }}</small></span
        ><StatusBadge
          :status="stack.status"
          :acknowledgement="stack.requiresAcknowledgement" /><ArrowUpRight
          :size="13"
      /></router-link>
      <p v-if="!store.stacks.length" class="padded muted">
        No Compose stacks discovered yet.
      </p>
    </div>
    <div class="section-heading"><h2>Recent webhook deliveries</h2><button class="text-link" @click="loadDeliveries">Refresh deliveries</button></div>
    <OperationsTable :records="deliveries"/>
  </div>
</template>
<script setup lang="ts">
import { ArrowUpRight, Folder, RefreshCw, Webhook } from 'lucide-vue-next'
import { computed, onMounted, ref } from 'vue'
import OperationsTable from '../components/OperationsTable.vue'
import RelativeTime from '../components/RelativeTime.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { type Operation, requestJSON, triggerRefresh } from '../services/api'
import { useConsoleStore } from '../store/console'
import { useStacksStore } from '../store/stacks'

const consoleStore = useConsoleStore(),
  store = useStacksStore(),
  busy = ref(false),
  error = ref(''),
  message = ref('')
const repoURL = computed(() =>
  /^https?:\/\//.test(consoleStore.instance?.repository.url || '')
    ? consoleStore.instance?.repository.url
    : '',
)
const sortedStacks = computed(() => [...store.stacks].sort((a, b) => a.path.localeCompare(b.path)))
const deliveries = computed(() =>
  consoleStore.operations.filter((o) => o.kind === 'webhook').slice(0, 10),
)
async function loadDeliveries() {
  try {
    const result = await requestJSON<{ records: Operation[] }>(
      '/api/operations?kind=webhook&limit=10',
    )
    result.records.forEach(consoleStore.onOperation)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Delivery history unavailable'
  }
}
onMounted(loadDeliveries)
async function refresh() {
  busy.value = true
  error.value = ''
  try {
    await triggerRefresh()
    message.value = 'Repository check requested. Eligible changes deploy automatically.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Repository check failed'
  } finally {
    busy.value = false
  }
}
</script>
