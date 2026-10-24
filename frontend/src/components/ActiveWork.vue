<template>
  <section class="active-work" :class="{ 'work-idle': work.restored && !displayed.length }" aria-label="Instance work">
    <div class="work-heading"><h2>Work in progress <span class="count-label">{{ groups.length }}</span></h2><button v-if="groups.length" class="text-link" :aria-expanded="expanded" @click="expanded = !expanded">{{ expanded ? 'Show preview' : `Show all work (${groups.length})` }}</button></div>
    <p v-if="!work.restored" class="inventory-warning">{{ work.readinessReason }}. Last received work is retained; disappearance does not imply success.</p>
    <p v-else-if="!groups.length" class="inventory-secondary">No work in progress</p>
    <ol class="workflow-list" :class="{ 'workflow-preview': !expanded }">
      <li v-for="(group, index) in displayed" :key="group.key" :class="{ 'second-workflow': !expanded && index === 1 && !pinned[group.key] }">
        <div class="workflow-summary"><div><strong>{{ humanize(group.root?.kind || group.operations[0]?.kind || 'Workflow') }}</strong><span v-for="path in targets(group)" :key="path" class="workflow-target"><router-link :to="{name: 'stack-detail', params: {path: path.split('/')}}">{{ path }}</router-link><span v-if="visiblePaths && !visiblePaths.includes(path)">Outside current view · Open stack</span></span><p v-if="!group.runs.length" role="status">{{ group.operations.find(op => op.status === 'running')?.message || 'Awaiting execution' }}</p></div><button class="button" :data-operation-id="group.rootOperationId || group.operations[0]?.id" @click="consoleStore.inspect(group.rootOperationId || group.operations[0]?.id || group.runs[0]?.operationId || '')">Details</button></div>
        <p v-if="group.groupingUnavailable" class="inventory-secondary">Workflow ancestry unavailable</p>
        <p v-if="!groups.some(active => active.key === group.key)" class="inventory-secondary">{{ groupOutcome(group) }}</p>
        <DeploymentProgress v-for="run in group.runs" :key="run.runId" compact :run="run" :detail-unavailable="!work.activeRuns.includes(run) && !work.terminalRuns.includes(run)" :stale="!work.restored || !groups.some(active => active.key === group.key)" @disclosure="open => pin(group, run.runId, open)" />
        <p v-if="!work.supported" class="inventory-secondary">Detailed live progress unavailable on this agent.</p>
      </li>
    </ol>
    <div v-if="lastResult" class="work-last-result" :class="{'inventory-warning': lastResult.status !== 'completed'}"><p :role="recent ? 'status' : undefined">{{ recent ? 'Result' : 'Last result' }} · {{ humanize(lastResult.kind) }} · {{ lastResult.status }} · {{ lastResult.message }}</p><button class="text-link" @click="consoleStore.inspect(lastResult.id)">Result details</button></div>
    <nav v-if="expanded && groups.length > 10" class="inventory-pagination" aria-label="Work pages"><button class="button" :disabled="page === 1" @click="page--">Previous work</button><span>Page {{ page }} of {{ Math.ceil(groups.length / 10) }}</span><button class="button" :disabled="page * 10 >= groups.length" @click="page++">Next work</button></nav>
  </section>
</template>
<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useWorkflowGroups } from '../composables/useWorkflowGroups'
import { useConsoleStore } from '../store/console'
import { useWorkStore, type WorkflowGroup } from '../store/work'
import DeploymentProgress from './DeploymentProgress.vue'

defineProps<{ visiblePaths?: readonly string[] }>()
const work = useWorkStore(),
  consoleStore = useConsoleStore()
const expanded = ref(false),
  page = ref(1)
const groups = useWorkflowGroups()
const pinned = ref<Record<string, WorkflowGroup>>({}),
  openRuns = new Set<string>()
function pin(group: WorkflowGroup, runId: string, open: boolean) {
  if (open) {
    openRuns.add(runId)
    pinned.value[group.key] = group
  } else {
    openRuns.delete(runId)
    if (!group.runs.some((run) => openRuns.has(run.runId))) delete pinned.value[group.key]
  }
}
watch(
  () => [groups.value, work.terminalRuns] as const,
  () => {
    for (const [key, group] of Object.entries(pinned.value)) {
      const active = groups.value.find((current) => current.key === key)
      const retained = group.runs
        .filter((run) => openRuns.has(run.runId))
        .map((run) => work.terminalRuns.find((terminal) => terminal.runId === run.runId) || run)
      pinned.value[key] = {
        ...(active || group),
        runs: [
          ...(active?.runs || []),
          ...retained.filter((run) => !active?.runs.some((current) => current.runId === run.runId)),
        ],
      }
    }
  },
)
const displayed = computed(() => {
  const preview = expanded.value
    ? groups.value.slice((page.value - 1) * 10, page.value * 10)
    : groups.value.slice(0, 2)
  return [
    ...preview.map((group) => pinned.value[group.key] || group),
    ...Object.values(pinned.value).filter(
      (group) => !preview.some((active) => active.key === group.key),
    ),
  ]
})
const groupOutcome = (group: WorkflowGroup) => {
  const operation =
    group.rootOperationId &&
    (work.history[group.rootOperationId] ||
      work.outcomes[group.rootOperationId]?.operation ||
      consoleStore.operationMap[group.rootOperationId])
  return operation && operation.status !== 'running'
    ? `Workflow ${operation.status} · ${operation.message}`
    : 'No longer active · Outcome not confirmed'
}
const lastResult = computed(
  () =>
    [...Object.values(work.history), ...consoleStore.operations]
      .filter(
        (op) =>
          !op.parentId &&
          op.status !== 'running' &&
          !groups.value.some((group) => group.rootOperationId === op.id),
      )
      .sort((a, b) => b.startedAt.localeCompare(a.startedAt))[0],
)
const recent = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
watch(
  () => lastResult.value?.id,
  () => {
    if (timer) clearTimeout(timer)
    const remaining = 10000 - (Date.now() - Date.parse(lastResult.value?.finishedAt || ''))
    recent.value = remaining > 0
    if (remaining > 0)
      timer = setTimeout(() => {
        recent.value = false
      }, remaining)
  },
  { immediate: true },
)
onUnmounted(() => {
  if (timer) clearTimeout(timer)
})
watch(
  () => groups.value.length,
  (length) => {
    page.value = Math.min(page.value, Math.max(1, Math.ceil(length / 10)))
  },
)
const targets = (group: WorkflowGroup) => [
  ...new Set(
    [...group.runs.map((r) => r.stack), ...group.operations.map((op) => op.stack)].filter(
      (path): path is string => !!path,
    ),
  ),
]
const humanize = (text: string) => text.replace(/_/g, ' ')
</script>
