<template>
  <tr v-if="table" class="inventory-container inventory-container-row" :data-container="container.id">
    <td><div class="inventory-resource inventory-resource--child"><Box :size="15" aria-hidden="true" /><button type="button" class="inventory-name inventory-control" aria-haspopup="dialog" :data-inventory-focus="`container:${stackPath}:${container.id}`" @click="$emit('inspect', container.id, $event.currentTarget as HTMLElement)">{{ container.name }}</button></div></td>
    <td><InventoryStatus :label="runtimeLabel" :tone="runtimeTone" /></td>
    <td><ImageIdentity :image="container.image" :identity="container.imageIdentity" part="reference" /></td>
    <td><ImageIdentity :image="container.image" :identity="container.imageIdentity" part="digest" /></td>
    <td><div class="inventory-row-links">
      <router-link v-if="logsAvailable" class="inventory-icon-link" :aria-label="`View logs for ${container.name}`" :title="`View logs for ${container.name}`" :to="{ name: 'stack-detail', params: { path: stackPath.split('/') }, query: { ...query, tab: 'logs', container: container.id } }"><Logs :size="16" aria-hidden="true" /></router-link>
      <a v-if="port !== null" class="inventory-icon-link" :href="buildPortURL(port)" target="_blank" rel="noopener noreferrer" :aria-label="`Open port ${port} for ${container.name}`" :title="`Open port ${port} for ${container.name}`"><ExternalLink :size="15" aria-hidden="true" /></a>
    </div></td>
  </tr>
  <li v-else class="inventory-container" :data-container="container.id">
    <div class="inventory-container-heading">
      <button
        type="button"
        class="inventory-name inventory-control"
        aria-haspopup="dialog"
        :data-inventory-focus="`container:${stackPath}:${container.id}`"
        @click="
          $emit('inspect', container.id, $event.currentTarget as HTMLElement)
        "
      >
        {{ container.name }}
      </button>
      <span>{{ runtimeLabel }}</span>
    </div>
    <ImageIdentity
      :image="container.image"
      :identity="container.imageIdentity"
    />
    <div class="inventory-container-links">
      <router-link
        v-if="logsAvailable"
        :to="{
          name: 'stack-detail',
          params: { path: stackPath.split('/') },
          query: { ...query, tab: 'logs', container: container.id },
        }"
        >View logs<span class="inventory-sr-only">
          for {{ container.name }}</span
        ></router-link
      >
      <a
        v-if="port !== null"
        :href="buildPortURL(port)"
        target="_blank"
        rel="noopener noreferrer"
        >Open port {{ port
        }}<span class="inventory-sr-only"> for {{ container.name }}</span></a
      >
      <span v-if="container.ports" class="inventory-secondary inventory-ports"
        >Ports: {{ container.ports }}</span
      >
    </div>
  </li>
</template>
<script setup lang="ts">
import { Box, ExternalLink, Logs } from 'lucide-vue-next'
import { computed } from 'vue'
import type { LocationQuery } from 'vue-router'
import type { ContainerInfo } from '../services/api'
import { buildPortURL, getLowestExternalPort } from '../utils/portUtils'
import ImageIdentity from './ImageIdentity.vue'
import InventoryStatus from './InventoryStatus.vue'

const props = withDefaults(
  defineProps<{
    container: ContainerInfo
    stackPath: string
    query?: LocationQuery
    logsAvailable?: boolean
    table?: boolean
  }>(),
  { logsAvailable: true },
)
defineEmits<{ inspect: [id: string, trigger: HTMLElement] }>()
const port = computed(() => getLowestExternalPort(props.container.ports || ''))
const runtimeLabel = computed(() => {
  const c = props.container
  const state = c.state ? c.state[0]?.toUpperCase() + c.state.slice(1) : 'State unknown'
  return `${state} · ${!c.health || c.health === 'none' ? 'no health check' : c.health}`
})
const runtimeTone = computed(() => {
  const c = props.container
  if (c.health === 'unhealthy' || c.state === 'dead') return 'danger'
  if (c.health === 'starting' || c.state === 'restarting') return 'active'
  if (c.state === 'running') return c.health === 'healthy' ? 'success' : 'neutral'
  return c.state ? 'neutral' : 'unknown'
})
</script>
