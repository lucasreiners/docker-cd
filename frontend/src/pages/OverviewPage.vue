<template>
  <div>
    <div class="page-heading">
      <div>
        <p class="eyebrow">Workspace</p>
        <h1>Overview</h1>
        <p>Your deployment environment, in focus.</p>
      </div>
      <router-link to="/stacks" class="button"
        >Browse stacks <ArrowRight :size="14"
      /></router-link>
    </div>
    <div class="health-overview">
      <router-link to="/stacks" class="health-metric"
        ><span class="metric-icon"><Layers :size="19" /></span
        ><span
          >Managed stacks<strong>{{ store.stacks.length }}</strong
          ><small>{{ store.statusCounts.synced || 0 }} in sync</small></span
        ></router-link
      >
      <router-link to="/stacks?status=attention" class="health-metric"
        ><span class="metric-icon warning"><TriangleAlert :size="19" /></span
        ><span
          >Need attention<strong>{{ attention.length }}</strong
          ><small>{{
            attention.length
              ? "Review deployment or runtime"
              : "No reported issues"
          }}</small></span
        ></router-link
      >
      <router-link to="/operations?status=running" class="health-metric"
        ><span class="metric-icon"><Activity :size="19" /></span
        ><span
          >Active operations<strong>{{ consoleStore.active.length }}</strong
          ><small>Server-owned execution</small></span
        ></router-link
      >
      <router-link to="/repository" class="health-metric"
        ><span class="metric-icon"><GitBranch :size="19" /></span
        ><span
          >Repository<strong class="metric-word">{{
            store.refreshStatus?.ref || "Connecting"
          }}</strong
          ><small>{{
            store.refreshStatus?.updatesBlocked
              ? "Deployments blocked"
              : "Source of desired state"
          }}</small></span
        ></router-link
      >
    </div>
    <div
      v-if="store.refreshStatus?.updatesBlocked"
      class="notice notice-error"
      role="alert"
    >
      <strong>Deployments are blocked</strong>
      <p>
        {{ store.refreshStatus.blockedReason }}
        <router-link to="/repository">Inspect repository →</router-link>
      </p>
    </div>
    <div class="overview-columns">
      <section class="surface">
        <div class="panel-heading">
          <h2>
            Needs attention
            <span class="count-label">{{ attention.length }}</span>
          </h2>
          <router-link to="/stacks?status=attention">View all</router-link>
        </div>
        <div v-if="!attention.length" class="empty-state">
          <CircleCheck :size="28" class="text-success" />
          <h3>Everything is accounted for</h3>
          <p>No deployment or runtime problems reported.</p>
        </div>
        <div
          v-for="stack in attention.slice(0, 6)"
          :key="stack.path"
          class="attention-row"
        >
          <span class="attention-symbol"><TriangleAlert :size="16" /></span>
          <div>
            <router-link
              class="resource-link"
              :to="{
                name: 'stack-detail',
                params: { path: stack.path.split('/') },
              }"
              >{{ stack.path }}</router-link
            >
            <p>
              {{
                stack.requiresAcknowledgement
                  ? "A configuration change awaits your approval"
                  : stack.lastSyncError ||
                    runtimeHealth(consoleStore.runtimeFor(stack.path)).label
              }}
            </p>
          </div>
          <ChevronRight :size="15" />
        </div>
      </section>
      <section class="surface instance-panel">
        <div class="panel-heading">
          <h2>Environment</h2>
          <Server :size="16" />
        </div>
        <dl class="definition-list">
          <div>
            <dt>Instance</dt>
            <dd>{{ consoleStore.instance?.name || "Docker-CD" }}</dd>
          </div>
          <div>
            <dt>Reconciliation</dt>
            <dd>
              {{
                consoleStore.instance?.policies.reconcile
                  ? "Automatic"
                  : "Disabled"
              }}
            </dd>
          </div>
          <div>
            <dt>Drift policy</dt>
            <dd>
              {{
                consoleStore.instance?.policies.drift === "flag"
                  ? "Review required"
                  : "Automatically apply"
              }}
            </dd>
          </div>
          <div>
            <dt>Last Git check</dt>
            <dd><RelativeTime :value="store.refreshStatus?.refreshedAt" /></dd>
          </div>
          <div>
            <dt>Next image update</dt>
            <dd>
              <RelativeTime
                v-if="consoleStore.instance?.schedule.enabled"
                :value="consoleStore.instance.schedule.nextRun"
              /><span v-else>Disabled</span>
            </dd>
          </div>
        </dl>
        <router-link to="/system" class="environment-link"
          >View instance configuration <ArrowRight :size="13"
        /></router-link>
      </section>
    </div>
    <div class="section-heading">
      <h2>Recent operations</h2>
      <router-link to="/operations" class="text-link"
        >View history <ArrowRight :size="13"
      /></router-link>
    </div>
    <div
      v-if="consoleStore.operationsError"
      class="notice notice-warning"
      role="alert"
    >
      {{ consoleStore.operationsError }}
    </div>
    <OperationsTable :records="consoleStore.operations.slice(0, 6)" />
  </div>
</template>
<script setup lang="ts">
import {
  Activity,
  ArrowRight,
  ChevronRight,
  CircleCheck,
  GitBranch,
  Layers,
  Server,
  TriangleAlert,
} from 'lucide-vue-next'
import { computed } from 'vue'
import OperationsTable from '../components/OperationsTable.vue'
import RelativeTime from '../components/RelativeTime.vue'
import { useConsoleStore } from '../store/console'
import { useStacksStore } from '../store/stacks'
import { needsAttention, runtimeHealth } from '../utils/stackState'

const store = useStacksStore(),
  consoleStore = useConsoleStore()
const attention = computed(() =>
  store.stacks.filter((stack) => needsAttention(stack, consoleStore.runtimeFor(stack.path))),
)
</script>
