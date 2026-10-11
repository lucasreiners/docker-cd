<template>
  <div v-if="operation" class="operation-details">
    <div class="operation-title">
      <StatusBadge :status="operation.status" /><span class="mono muted">{{
        humanize(operation.kind)
      }}</span>
    </div>
    <h2>{{ operation.stack || "All managed stacks" }}</h2>
    <p class="muted">{{ operation.message || "Operation in progress" }}</p>
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
    <DeploymentProgress v-if="operation.status === 'running' && (!operation.stack || stacks.pullProgress?.stack === operation.stack || stacks.updateProgress?.stack === operation.stack)" />
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
      <li v-for="(step, index) in operation.steps" :key="index">
        <span class="timeline-dot" />
        <div>
          <strong>{{ humanize(step.phase) }}</strong>
          <p>{{ step.message }}</p>
        </div>
        <RelativeTime :value="step.at" />
      </li>
    </ol>
    <template v-if="children.length"
      ><h3 class="section-label">Related operations</h3>
      <router-link
        v-for="child in children"
        :key="child.id"
        class="related-operation"
        :to="`/operations/${child.id}`"
        @click="consoleStore.inspectedId = ''"
        ><span>{{ child.stack || child.kind }}</span
        ><StatusBadge :status="child.status" /></router-link
    ></template>
  </div>
  <div v-else class="empty-state">
    <h3>Operation unavailable</h3>
    <p>
      {{
        consoleStore.operationsError ||
        "The record may have expired from retained history."
      }}
    </p>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useConsoleStore } from '../store/console'
import { useStacksStore } from '../store/stacks'
import DeploymentProgress from './DeploymentProgress.vue'
import RelativeTime from './RelativeTime.vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{ id: string }>()
function humanize(value: string) {
  return value.replace(/_/g, ' ')
}
const consoleStore = useConsoleStore()
const stacks = useStacksStore()
const operation = computed(() => consoleStore.operationMap[props.id])
const children = computed(() => consoleStore.operations.filter((o) => o.parentId === props.id))
</script>
