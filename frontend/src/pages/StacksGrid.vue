<template>
  <div class="operational-home">
    <div class="page-heading inventory-page-heading"><h1>Stacks <span class="count-label">{{ store.loading ? '—' : store.stacks.length }}</span></h1><WorkspaceActions :show-runtime="false" /></div>
    <ActiveWork :visible-paths="visiblePaths" />
    <div v-if="store.loading" class="inventory-loading" role="status">Connecting to agent · Loading stacks and observed containers…<div v-for="n in 3" :key="n" class="inventory-skeleton"><span/><span/><span/></div></div>
    <div v-else-if="store.error" class="notice notice-error" role="alert">{{ store.error }} <button class="button" @click="store.loadInitial()">Retry</button></div>
    <div v-else-if="!store.stacks.length" class="empty-state surface"><h2>No stacks discovered</h2><p>Add Compose in the configured Git repository.</p><router-link to="/repository" class="button">Open repository</router-link></div>
    <StackTree v-else :stacks="store.stacks" :runtime-for="consoleStore.runtimeFor" :active-paths="activePaths" :density="consoleStore.density" :query="route.query" :logs-available="!!consoleStore.instance?.capabilities.logs" @density="consoleStore.setDensity($event)" @inspect="inspect" @visible="visiblePaths = $event">
      <template #actions="{ stack }"><StackActions :stack="stack" compact /></template>
      <template #work="{ stack }"><StackWork :stack="stack.path" compact /></template>
    </StackTree>
    <div class="table-caption runtime-caption"><span>Runtime observed <RelativeTime :value="observedAt" /> · Observations refresh automatically every 15s.</span><button class="text-link" :disabled="consoleStore.runtimeLoading" @click="consoleStore.loadRuntime()">Refresh runtime</button></div>
    <ContainerInspector :selection="consoleStore.inspectedContainer" :current="consoleStore.inspectedContainerCurrent" :removed="consoleStore.inspectedContainerRemoved" :comparison="consoleStore.inspectedContainerIdentityComparison" :runtime="consoleStore.inspectedContainer ? consoleStore.runtimeFor(consoleStore.inspectedContainer.stack) : null" :trigger="inspectorTrigger" :query="route.query" :logs-available="!!consoleStore.instance?.capabilities.logs" @close="consoleStore.inspectedContainer = null" />
  </div>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import ActiveWork from '../components/ActiveWork.vue'
import ContainerInspector from '../components/ContainerInspector.vue'
import RelativeTime from '../components/RelativeTime.vue'
import StackActions from '../components/StackActions.vue'
import StackTree from '../components/StackTree.vue'
import StackWork from '../components/StackWork.vue'
import WorkspaceActions from '../components/WorkspaceActions.vue'
import { useConsoleStore } from '../store/console'
import { useStacksStore } from '../store/stacks'
import { useWorkStore } from '../store/work'

const store = useStacksStore(),
  consoleStore = useConsoleStore(),
  work = useWorkStore(),
  route = useRoute()
const visiblePaths = ref<string[]>([]),
  inspectorTrigger = ref<HTMLElement | null>(null)
const activePaths = computed(() => [
  ...new Set(
    [
      ...work.activeRuns.map((run) => run.stack),
      ...Object.values(work.liveOperations)
        .filter((op) => op.status === 'running')
        .map((op) => op.stack),
      ...work.legacyActiveOperations.map((op) => op.stack),
    ].filter((path): path is string => !!path),
  ),
])
const observedAt = computed(
  () =>
    Object.values(consoleStore.runtime)
      .map((runtime) => runtime.observedAt || '')
      .filter(Boolean)
      .sort()[0],
)
function inspect(stack: string, id: string, trigger: HTMLElement) {
  inspectorTrigger.value = trigger
  consoleStore.inspectContainer(stack, id)
}
</script>
