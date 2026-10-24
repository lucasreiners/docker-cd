<template>
  <section
    ref="root"
    class="stack-tree"
    :class="{ 'stack-tree--compact': density === 'compact' }"
    aria-label="Stack inventory"
  >
    <div class="inventory-toolbar">
      <label class="inventory-search"
        ><span class="inventory-sr-only">Search stacks, containers, images</span><input
          v-model="view.searchInput.value"
          type="search"
          placeholder="Path, container, image or digest…"
          data-inventory-focus="inventory-search"
      /></label>
      <label
        ><span class="inventory-sr-only">Show</span><select v-model="view.quick.value">
          <option value="all">All stacks</option>
          <option value="attention">Needs attention</option>
          <option value="active">Active work</option>
        </select></label
      >
      <details class="inventory-display">
        <summary>Display</summary>
        <div class="inventory-display-options">
          <label
            >Density<select
              :value="density"
              @change="
                $emit('density', ($event.target as HTMLSelectElement).value)
              "
            >
              <option value="comfortable">Comfortable</option>
              <option value="compact">Compact</option>
            </select></label
          >
          <label
            ><input v-model="view.group.value" type="checkbox" /> Group by
            directory</label
          >
          <button
            type="button"
            class="inventory-control"
            @click="view.pageDisclosure(false)"
          >
            Show container previews on this page
          </button>
          <button
            type="button"
            class="inventory-control"
            @click="view.pageDisclosure(true)"
          >
            Hide containers on this page
          </button>
        </div>
      </details>
      <details>
        <summary>More filters</summary>
        <div class="inventory-display-options">
          <label
            >Deployment<select v-model="view.deployment.value">
              <option value="all">All deployments</option>
              <option value="approval">Approval required</option>
              <option value="failed">Failed</option>
              <option value="missing">Not deployed yet</option>
              <option value="syncing">Applying</option>
              <option value="synced">Applied</option>
              <option value="deleting">Removing</option>
            </select></label
          >
          <label
            >Runtime<select v-model="view.runtime.value">
              <option value="all">All runtime</option>
              <option value="healthy">Healthy</option>
              <option value="attention">Needs attention</option>
              <option value="no-checks">No health checks</option>
              <option value="unknown">Unknown</option>
            </select></label
          >
        </div>
      </details>
    </div>
    <p v-if="view.search.value || view.quick.value !== 'all' || view.deployment.value !== 'all' || view.runtime.value !== 'all'" class="inventory-secondary inventory-filter-summary">
      {{ view.entries.value.length }} matching stacks · Filters affect inventory only
      <button v-if="view.search.value || view.quick.value !== 'all' || view.deployment.value !== 'all' || view.runtime.value !== 'all'" class="text-link" @click="view.clearFilters">Clear filters</button>
    </p>
    <p v-if="view.search.value && incomplete" class="inventory-secondary">
      Container and image search may be incomplete: some runtime observations
      are unavailable or stale.
    </p>
    <p class="inventory-scroll-hint inventory-secondary">Scroll table for state, image and actions →</p>
    <div class="inventory-table-scroll" role="region" aria-label="Stack inventory table, scroll horizontally for all columns" tabindex="0">
    <table class="inventory-table inventory-stack-list">
      <caption class="inventory-sr-only">Stacks and observed containers. Expand a stack to show its containers.</caption>
      <colgroup><col class="inventory-col-resource" /><col class="inventory-col-state" /><col class="inventory-col-image" /><col class="inventory-col-digest" /><col class="inventory-col-actions" /></colgroup>
      <thead><tr><th scope="col">Resource</th><th scope="col">State</th><th scope="col">Image</th><th scope="col">Digest</th><th scope="col">Actions</th></tr></thead>
      <template
        v-for="(entry, index) in view.visible.value"
        :key="entry.stack.path"
      >
        <tbody
          v-if="view.group.value && directoryChanged(index)"
          class="inventory-directory"
        >
          <tr><th colspan="5" scope="rowgroup">{{ directory(entry.stack.path) }}</th></tr>
        </tbody>
        <StackTreeRow
          :entry="entry"
          :children="view.children(entry)"
          :mode="view.stateFor(entry).mode"
          :matching-all="view.matchingDisclosure[entry.stack.path]?.all"
          :query="query"
          :logs-available="logsAvailable"
          @toggle="view.toggle(entry)"
          @reveal="(all) => view.reveal(entry, all)"
          @fewer="view.fewer(entry)"
          @inspect="
            (id, trigger) => $emit('inspect', entry.stack.path, id, trigger)
          "
        >
          <template v-if="$slots.actions" #actions="scope"
            ><slot name="actions" v-bind="scope"
          /></template>
          <template v-if="$slots.work" #work="scope"
            ><slot name="work" v-bind="scope"
          /></template>
        </StackTreeRow>
      </template>
    </table>
    </div>
    <p v-if="!view.entries.value.length">
      No matching stacks. Change search or filters to see more.
    </p>
    <nav class="inventory-pagination" aria-label="Stack pages">
      <span
        >{{
          view.entries.value.length ? (view.currentPage.value - 1) * 20 + 1 : 0
        }}–{{
          Math.min(view.currentPage.value * 20, view.entries.value.length)
        }}
        of {{ view.entries.value.length }} stacks</span
      >
      <button
        type="button"
        class="inventory-control"
        :disabled="view.currentPage.value <= 1"
        @click="changePage(view.currentPage.value - 1)"
      >
        Previous
      </button>
      <span
        >Page {{ view.currentPage.value }} of {{ view.pageCount.value }}</span
      >
      <button
        type="button"
        class="inventory-control"
        :disabled="view.currentPage.value >= view.pageCount.value"
        @click="changePage(view.currentPage.value + 1)"
      >
        Next
      </button>
    </nav>
  </section>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUpdate, ref, watch } from 'vue'
import type { LocationQuery } from 'vue-router'
import { runtimeUnknown, useInventoryView } from '../composables/useInventoryView'
import type { StackRecord } from '../services/api'
import type { RuntimeView } from '../store/console'
import StackTreeRow from './StackTreeRow.vue'

const props = withDefaults(
  defineProps<{
    stacks: StackRecord[]
    runtimeFor: (path: string) => RuntimeView
    activePaths?: readonly string[]
    density?: string
    query?: LocationQuery
    logsAvailable?: boolean
    syncHistory?: boolean
  }>(),
  {
    activePaths: () => [],
    density: 'comfortable',
    logsAvailable: true,
    syncHistory: true,
  },
)
const emit = defineEmits<{
  visible: [paths: string[]]
  inspect: [stack: string, id: string, trigger: HTMLElement]
  density: [value: string]
}>()
const root = ref<HTMLElement>()
const view = useInventoryView({
  stacks: () => props.stacks,
  runtimeFor: (path) => props.runtimeFor(path),
  activePaths: () => props.activePaths,
  syncHistory: props.syncHistory,
})
const incomplete = computed(() =>
  props.stacks.some((s) => runtimeUnknown(props.runtimeFor(s.path))),
)
watch(
  () => view.visible.value.map((entry) => entry.stack.path),
  (paths) => emit('visible', paths),
  { immediate: true },
)
const directory = (path: string) =>
  path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : 'Root'
const directoryChanged = (index: number) =>
  !index ||
  directory(view.visible.value[index - 1]?.stack.path || '') !==
    directory(view.visible.value[index]?.stack.path || '')
async function changePage(page: number) {
  view.save()
  view.page.value = page
  await nextTick()
  root.value?.querySelector<HTMLInputElement>('input[type="search"]')?.focus()
}
onBeforeUpdate(() => {
  const active = document.activeElement as HTMLElement | null
  const path = active?.closest('[data-stack]')?.getAttribute('data-stack')
  if (path && !view.visible.value.some((e) => e.stack.path === path))
    root.value?.querySelector<HTMLInputElement>('input[type="search"]')?.focus()
})
</script>
