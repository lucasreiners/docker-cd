<template>
  <div v-if="operation" class="operation-details">
    <div class="operation-title">
      <StatusBadge :status="operation.status" /><span class="mono muted">{{
        humanize(operation.kind)
      }}</span>
    </div>
    <h2>{{ operation.stack || "All managed stacks" }}</h2>
    <p class="muted">{{ operation.message || "Operation in progress" }}</p>
    <p v-if="operation.status !== 'running' && runs.some(run => run.status === 'running')" class="inventory-warning">This operation record is terminal; related execution is still active. Pull completion does not imply deployment completion.</p>
    <dl class="definition-list">
      <div>
        <dt>Trigger</dt>
        <dd>{{ operation.trigger }}</dd>
      </div>
      <div>
        <dt>Started</dt>
        <dd><RelativeTime :value="operation.startedAt" /></dd>
      </div>
      <div v-if="operation.finishedAt">
        <dt>Finished</dt>
        <dd><RelativeTime :value="operation.finishedAt" /></dd>
      </div>
      <div v-if="operation.revision">
        <dt>Revision</dt>
        <dd>
          <code>{{ operation.revision }}</code>
        </dd>
      </div>
      <div>
        <dt>Operation ID</dt>
        <dd>
          <code>{{ operation.id }}</code>
        </dd>
      </div>
    </dl>
    <router-link
      v-if="operation.parentId"
      :to="`/operations/${operation.parentId}`"
      class="text-link"
      @click="consoleStore.inspectedId = ''"
      >View parent operation →</router-link
    >
    <DeploymentProgress v-for="run in runs" :key="run.runId" :run="run" :detail-unavailable="!work.activeRuns.includes(run) && !work.terminalRuns.includes(run)" :stale="!work.restored" />
    <p v-if="operation.status === 'running' && !runs.length" class="inventory-secondary">{{ work.supported === false ? 'Detailed live progress unavailable on this agent.' : 'No image work reported for this operation.' }}</p>
    <p v-if="lookupError" class="inventory-warning">{{ lookupError }}</p>
    <h3 class="section-label">Execution timeline</h3>
    <ol class="timeline">
      <li>
        <span class="timeline-dot" />
        <div>
          <strong>Started</strong>
          <p>{{ operation.trigger }} · {{ humanize(operation.kind) }}</p>
        </div>
        <RelativeTime :value="operation.startedAt" />
      </li>
      <li v-for="(step, index) in ('steps' in operation ? operation.steps : [])" :key="index">
        <span class="timeline-dot" />
        <div>
          <strong>{{ humanize(step.phase) }}</strong>
          <p>{{ step.message }}</p>
        </div>
        <RelativeTime :value="step.at" />
      </li>
    </ol>
    <h3 class="section-label">Related operations</h3>
    <p v-if="childrenError" class="inventory-warning">{{ childrenError }} <button class="text-link" @click="loadChildren(false)">Retry related history</button></p>
    <template v-if="children.length">
      <router-link
        v-for="child in children"
        :key="child.id"
        class="related-operation"
        :to="`/operations/${child.id}`"
        @click="consoleStore.inspectedId = ''"
        ><span>{{ child.stack || child.kind }}</span
        ><StatusBadge :status="child.status" /></router-link
    ></template>
    <button v-if="nextCursor" class="button" :disabled="childrenLoading" @click="loadChildren(true)">Load more related operations</button>
    <p class="inventory-secondary">{{ childrenLoading ? 'Loading related history…' : nextCursor ? 'More related operations available.' : childrenError ? 'Related history is incomplete.' : 'All retained direct children loaded; expired records may be absent.' }}</p>
  </div>
  <div v-else class="empty-state">
    <h3>{{ loading ? 'Loading operation…' : 'Operation not found or expired' }}</h3>
    <p>
      {{
        lookupError ||
        "The record may have expired from retained history."
      }}
    </p>
  </div>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { type Operation, requestJSON } from '../services/api'
import { useConsoleStore } from '../store/console'
import { useWorkStore } from '../store/work'
import DeploymentProgress from './DeploymentProgress.vue'
import RelativeTime from './RelativeTime.vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{ id: string }>()
function humanize(value: string) {
  return value.replace(/_/g, ' ')
}
const consoleStore = useConsoleStore()
const work = useWorkStore()
const operation = computed(
  () =>
    [
      consoleStore.operationMap[props.id],
      work.history[props.id],
      work.outcomes[props.id]?.operation,
      work.liveOperations[props.id],
    ]
      .filter((op) => !!op)
      .sort((a, b) => (b.version || 0) - (a.version || 0))[0],
)
const retainedRuns = ref<Record<string, import('../services/work').WorkRun>>({})
const runs = computed(() => Object.values(retainedRuns.value))
watch(
  () => [props.id, work.activeRuns, work.terminalRuns] as const,
  ([id, active, terminal], previous) => {
    if (id !== previous?.[0]) retainedRuns.value = {}
    for (const run of [...active, ...terminal])
      if (run.operationId === id || run.rootOperationId === id) retainedRuns.value[run.runId] = run
  },
  { immediate: true },
)
const loading = ref(false),
  lookupError = ref(''),
  childrenError = ref(''),
  childrenLoading = ref(false),
  nextCursor = ref('')
const children = ref<Operation[]>([])
let generation = 0
watch(
  () => props.id,
  async (id) => {
    const current = ++generation
    children.value = []
    nextCursor.value = ''
    lookupError.value = ''
    childrenError.value = ''
    if (!id) return
    loading.value = true
    void loadChildren(false)
    await work.lookup(id)
    if (current !== generation) return
    loading.value = false
    const outcome = work.outcomes[id]
    if (outcome?.state === 'unavailable')
      lookupError.value = 'Operation not found or expired from retained history.'
    if (outcome?.state === 'failed')
      lookupError.value = 'Live work may be available; durable operation history unavailable.'
  },
  { immediate: true },
)
async function loadChildren(append: boolean) {
  const current = generation,
    id = props.id
  childrenLoading.value = true
  childrenError.value = ''
  try {
    const query = new URLSearchParams({ parentId: id, limit: '20' })
    if (append && nextCursor.value) query.set('cursor', nextCursor.value)
    const result = await requestJSON<{ records: Operation[]; nextCursor?: string }>(
      `/api/operations?${query}`,
    )
    if (current !== generation) return
    children.value = [
      ...new Map(
        [...(append ? children.value : []), ...result.records].map((op) => [op.id, op]),
      ).values(),
    ]
    nextCursor.value = result.nextCursor || ''
  } catch (error) {
    if (current === generation)
      childrenError.value = error instanceof Error ? error.message : 'Related history unavailable'
  } finally {
    if (current === generation) childrenLoading.value = false
  }
}
</script>
