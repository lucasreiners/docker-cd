<template>
  <div v-if="displayedRuns.length || stages.length || result || missing.length" class="stack-work" :class="{ 'stack-work--compact': compact }" aria-label="Stack work">
    <DeploymentProgress v-for="run in displayedRuns" :key="run.runId" :run="run" :compact="compact" :detail-unavailable="!work.activeRuns.includes(run) && !work.terminalRuns.includes(run)" :stale="!work.restored || (!runs.some(active => active.runId === run.runId) && run.status === 'running')" @disclosure="open => pin(run, open)" />
    <div v-for="operation in stages.filter(op => !runs.some(run => run.operationId === op.id))" :key="operation.id">
      <p role="status">{{ operation.message || 'Requested; awaiting execution' }}</p>
      <p v-if="!work.supported" class="inventory-secondary">Detailed live progress unavailable on this agent.</p>
      <button class="text-link" @click="consoleStore.inspect(operation.id)">Work details</button>
    </div>
    <p v-for="run in missing" :key="run.runId" class="inventory-warning">Outcome not confirmed · {{ work.outcomes[run.operationId]?.state === 'unavailable' ? 'Operation not found or expired' : 'Retrieving operation outcome' }} <button class="text-link" @click="consoleStore.inspect(run.operationId)">Inspect operation</button></p>
    <div v-if="result && !runs.length && !stages.length" :class="result.status === 'failed' ? 'inventory-warning' : 'inventory-secondary'">
      <p :role="recent ? 'status' : undefined" :title="compact ? result.message : undefined">{{ recent ? '' : 'Last result · ' }}{{ compact ? resultLabel.split(' · ')[0] : resultLabel }}<span v-if="result.message && !compact"> · {{ result.message }}</span></p>
      <button class="text-link" @click="consoleStore.inspect(result.id)">Result details</button>
    </div>
  </div>
</template>
<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import type { WorkRun } from '../services/work'
import { useConsoleStore } from '../store/console'
import { useWorkStore } from '../store/work'
import DeploymentProgress from './DeploymentProgress.vue'

const props = defineProps<{ stack: string; compact?: boolean }>()
const work = useWorkStore(),
  consoleStore = useConsoleStore()
const runs = computed(() => work.runsForStack(props.stack))
const pinned = ref<Record<string, WorkRun>>({})
const displayedRuns = computed(() => [
  ...runs.value,
  ...Object.values(pinned.value).filter(
    (run) => !runs.value.some((active) => active.runId === run.runId),
  ),
])
function pin(run: WorkRun, open: boolean) {
  if (open) pinned.value[run.runId] = run
  else delete pinned.value[run.runId]
}
watch(
  () => [...runs.value, ...work.terminalRuns],
  (latest) => {
    for (const run of latest) if (pinned.value[run.runId]) pinned.value[run.runId] = run
  },
)
const stages = computed(() => work.operationStagesForStack(props.stack))
const missing = computed(() =>
  Object.values(work.missingRuns).filter(
    (run) => run.stack === props.stack && !work.outcomes[run.operationId]?.operation?.finishedAt,
  ),
)
const result = computed(
  () =>
    [
      ...Object.values(work.history),
      ...consoleStore.operations,
      ...Object.values(work.outcomes).flatMap((o) => (o.operation ? [o.operation] : [])),
    ]
      .filter((op) => op.stack === props.stack && op.status !== 'running')
      .sort((a, b) => b.startedAt.localeCompare(a.startedAt))[0],
)
const recent = ref(false)
const resultLabel = computed(() => {
  const operation = result.value
  if (!operation) return ''
  if (operation.status === 'completed')
    return operation.kind === 'deployment'
      ? 'Deployment completed · Runtime observation is independent'
      : 'Operation completed · Deployment and runtime are independent'
  if (operation.status === 'cancelled') return 'Cancelled by server'
  if (operation.status === 'interrupted') return 'Interrupted by server'
  if (operation.status === 'completed_with_errors') return 'Completed with errors'
  return operation.kind === 'deployment' ? 'Deployment failed' : 'Operation failed'
})
let timer: ReturnType<typeof setTimeout> | undefined
watch(
  () => result.value?.id,
  () => {
    if (timer) clearTimeout(timer)
    const remaining = 10000 - (Date.now() - Date.parse(result.value?.finishedAt || ''))
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
</script>
