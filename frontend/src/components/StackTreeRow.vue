<template>
  <tbody
    :id="childrenId"
    class="inventory-stack inventory-container-list"
    :data-stack="entry.stack.path"
    :aria-labelledby="headingId"
  >
    <tr class="inventory-stack-header">
      <th scope="row">
        <div class="inventory-resource">
          <button
            ref="toggleButton"
            type="button"
            class="inventory-disclosure inventory-control"
            :aria-expanded="open"
            :aria-controls="childrenId"
            :aria-label="`${open ? 'Hide' : 'Show'} containers for ${entry.stack.path}`"
            :data-inventory-focus="`disclosure:${entry.stack.path}`"
            @click="$emit('toggle')"
          >
            <ChevronDown v-if="open" :size="15" aria-hidden="true" />
            <ChevronRight v-else :size="15" aria-hidden="true" />
          </button>
          <Folder :size="16" aria-hidden="true" />
          <span
            :id="headingId"
            tabindex="-1"
            :data-inventory-focus="`stack:${entry.stack.path}`"
            class="inventory-resource-name"
          >
            <router-link
              class="inventory-name"
              :data-inventory-focus="`stack-link:${entry.stack.path}`"
              :to="{ name: 'stack-detail', params: { path: entry.stack.path.split('/') }, query }"
            >{{ entry.stack.path }}</router-link>
          </span>
        </div>
      </th>
      <td class="inventory-state" :title="entry.reason || undefined">
        <InventoryStatus :label="deploymentLabel" :tone="deploymentTone" :aria-label="`Deployment: ${deploymentLabel}`" />
        <span
          class="inventory-runtime-summary"
          :class="{ 'inventory-runtime-summary--attention': runtimeUnhealthy }"
          :aria-label="`Runtime: ${runtimeSummary}`"
        ><TriangleAlert v-if="runtimeUnhealthy" :size="11" aria-hidden="true" />{{ runtimeSummary }}</span>
        <span class="inventory-sr-only" role="status">{{ entry.stack.path }} deployment: {{ deploymentLabel }}</span>
      </td>
      <td class="inventory-stack-image" />
      <td><span class="inventory-secondary" aria-label="Repository digest applies to containers">—</span></td>
      <td class="inventory-stack-actions"><slot name="actions" :stack="entry.stack" /></td>
    </tr>
    <tr class="inventory-work-row"><td colspan="5"><slot name="work" :stack="entry.stack" /></td></tr>
    <tr v-if="open && runtimeNotice" class="inventory-observation-row"><td colspan="5"><span class="inventory-secondary">{{ runtimeNotice }}</span></td></tr>
    <template v-if="open">
      <ContainerRow
        v-for="container in children"
        :key="container.id"
        table
        :container="container"
        :stack-path="entry.stack.path"
        :query="query"
        :logs-available="logsAvailable"
        @inspect="(id, trigger) => $emit('inspect', id, trigger)"
      />
      <tr v-if="entry.runtime.containers.length && hasControls" class="inventory-child-controls-row">
        <td colspan="5"><div class="inventory-child-controls">
        <span v-if="entry.childOnly && !matchingAll" class="inventory-secondary">{{ children.length }} of {{ entry.matches.length }} matching containers</span>
        <button v-if="entry.childOnly && !matchingAll && children.length < entry.matches.length" type="button" class="inventory-control" @click="$emit('reveal', false)">{{ mode === 'preview' ? 'Show matching containers' : 'Show 20 more matching containers' }}</button>
        <button v-if="(entry.childOnly && !matchingAll) || (mode === 'preview' && entry.runtime.containers.length > 2)" type="button" class="inventory-control" @click="$emit('reveal', true)">Show all {{ entry.runtime.containers.length }} containers</button>
        <button v-else-if="mode === 'full' && children.length < entry.runtime.containers.length" type="button" class="inventory-control" @click="$emit('reveal', true)">Show 20 more</button>
        <button v-if="mode === 'full'" type="button" class="inventory-control" @click="showFewer">Show fewer<span class="inventory-sr-only"> containers for {{ entry.stack.path }}</span></button>
        </div></td>
      </tr>
    </template>
  </tbody>
</template>
<script setup lang="ts">
import { ChevronDown, ChevronRight, Folder, TriangleAlert } from 'lucide-vue-next'
import { computed, onBeforeUpdate, ref } from 'vue'
import type { LocationQuery } from 'vue-router'
import type { InventoryDisclosure, InventoryEntry } from '../composables/useInventoryView'
import type { ContainerInfo } from '../services/api'
import ContainerRow from './ContainerRow.vue'
import InventoryStatus from './InventoryStatus.vue'

const props = defineProps<{
  entry: InventoryEntry
  children: ContainerInfo[]
  mode: InventoryDisclosure['mode']
  matchingAll?: boolean
  query?: LocationQuery
  logsAvailable?: boolean
}>()
const emit = defineEmits<{
  toggle: []
  reveal: [all: boolean]
  fewer: []
  inspect: [id: string, trigger: HTMLElement]
}>()
const toggleButton = ref<HTMLButtonElement>()
const open = computed(() => props.mode !== 'hidden')
const headingId = computed(() => `inventory-stack-${encodeURIComponent(props.entry.stack.path)}`)
const childrenId = computed(() => `${headingId.value}-containers`)
const runtimeUnhealthy = computed(() =>
  props.entry.runtime.containers.some((c) => c.health === 'unhealthy' || c.state === 'dead'),
)
const deploymentLabel = computed(() =>
  props.entry.stack.requiresAcknowledgement
    ? 'Approval required'
    : {
        synced: 'Applied',
        syncing: 'Applying',
        failed: 'Failed',
        missing: 'Not deployed yet',
        deleting: 'Removing',
      }[props.entry.stack.status],
)
const deploymentTone = computed(() =>
  props.entry.stack.requiresAcknowledgement
    ? 'warning'
    : props.entry.stack.status === 'synced'
      ? 'success'
      : props.entry.stack.status === 'failed'
        ? 'danger'
        : ['syncing', 'deleting'].includes(props.entry.stack.status)
          ? 'active'
          : 'unknown',
)
const runtimeSummary = computed(() => {
  const r = props.entry.runtime
  if (!r.observedAt) return r.error ? 'Runtime unavailable' : 'Runtime unknown'
  const unhealthy = r.containers.filter((c) => c.health === 'unhealthy').length
  return `${r.containers.filter((c) => c.state === 'running').length} / ${r.containers.length} running${unhealthy ? ` · ${unhealthy} unhealthy` : ''}${r.error || r.stale ? ' · last known' : ''}`
})
const runtimeNotice = computed(() => {
  const r = props.entry.runtime
  if (r.error || r.stale)
    return `${r.error ? 'Runtime unavailable' : 'Runtime observation stale'} · ${r.observedAt ? `Last observed ${r.observedAt}` : 'Not yet observed'}.${r.containers.length ? ' Showing last known containers.' : ''}`
  if (r.loading && !r.observedAt) return 'Checking runtime…'
  if (!r.containers.length)
    return `No containers observed · ${r.observedAt || 'Observation time unavailable'}${props.entry.stack.status === 'missing' ? ' · Not deployed yet.' : ''}`
  return ''
})
const hasControls = computed(
  () =>
    props.mode === 'full' ||
    (props.entry.childOnly && !props.matchingAll) ||
    props.entry.runtime.containers.length > 2,
)
function showFewer() {
  toggleButton.value?.focus()
  emit('fewer')
}
onBeforeUpdate(() => {
  const active = document.activeElement as HTMLElement | null
  const row = active?.closest<HTMLElement>('[data-container]')
  if (
    row?.closest('[data-stack]')?.getAttribute('data-stack') === props.entry.stack.path &&
    !props.children.some((c) => c.id === row.dataset.container)
  )
    toggleButton.value?.focus()
})
</script>
