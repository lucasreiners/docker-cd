<template>
  <section v-if="run" class="run-progress" :class="{ 'run-compact': compact }" :aria-label="`Progress for ${run.stack || 'instance'} run ${run.runId}`">
    <p class="run-phase" :class="{ 'inventory-sr-only': compact }" role="status">{{ phaseLabel }}<span v-if="stale"> · Last received {{ run.updatedAt }}</span></p>
    <p v-if="!compact && run.message && !detailUnavailable" class="inventory-secondary">{{ run.message }}</p>
    <p v-if="compact && run.status === 'failed' && !detailUnavailable" class="inventory-warning">{{ run.message }}</p>
    <p v-if="run.stackResults && !detailUnavailable" class="inventory-secondary">{{ run.stackResults.processed }} stacks processed · {{ run.stackResults.skipped }} skipped · {{ run.stackResults.failed }} failed</p>
    <details class="pull-details" @toggle="toggleDetails">
      <summary><template v-if="compact">{{ phaseLabel }} · </template>Pull details · {{ run.imageCount }} reported images</summary>
      <p v-if="detailsOpen && detailUnavailable" class="inventory-warning">Previous live details invalidated. Retrieve the outcome from operation history; disappearance does not imply success.</p>
       <template v-if="detailsOpen && !detailUnavailable">
       <p v-if="compact && run.message" class="inventory-secondary">{{ run.message }}</p>
       <p v-if="compact && stale" class="inventory-secondary">Last received {{ run.updatedAt }}</p>
      <p class="inventory-secondary">Bytes describe each layer’s current phase, not total deployment progress. Runtime still describes the observed containers.</p>
      <article v-for="image in run.images.slice(0, imageLimit)" :key="image.pullAttemptId" class="pull-image">
        <strong class="mono">{{ image.reference }}</strong>
        <p>{{ image.done ? (image.status === 'completed' ? 'Images pulled — deployment may still be applying or pruning' : image.status) : 'Pulling image' }}<span v-if="image.position"> · Image {{ image.position }}<template v-if="image.total"> of {{ image.total }}</template></span></p>
        <ul class="pull-layers">
          <li v-for="layer in image.layers.slice(0, layerLimit)" :key="layer.id">
            <code>{{ layer.id }}</code><span>{{ layer.phase }} · {{ layer.status }}</span>
            <span>{{ bytes(layer.bytesCurrent) }} / {{ bytes(layer.bytesTotal) }}</span>
            <progress v-if="layer.bytesTotal != null && layer.bytesTotal > 0 && layer.bytesCurrent != null && ['downloading', 'extracting'].includes(layer.phase)" :value="layer.bytesCurrent" :max="layer.bytesTotal" :aria-label="`${layer.id} ${layer.phase} bytes`" />
          </li>
        </ul>
        <button v-if="image.layers.length > layerLimit" class="text-link" @click="layerLimit += 20">Show 20 more layers</button>
        <p v-if="image.detailsTruncated" class="inventory-secondary">Some layer details were omitted by the agent.</p>
      </article>
      <button v-if="run.images.length > imageLimit" class="text-link" @click="imageLimit += 5">Show 5 more images</button>
      <p v-if="!run.images.length">Live layer detail unavailable. Durable outcomes are available in the operation history.</p>
      <p v-if="run.detailsTruncated" class="inventory-secondary">Run detail is incomplete; workflow status remains authoritative.</p>
      </template>
    </details>
  </section>
  <p v-else class="inventory-secondary">Detailed live progress unavailable on this agent. Follow the operation stage.</p>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import type { WorkRun } from '../services/work'

const props = defineProps<{
  run?: WorkRun
  stale?: boolean
  detailUnavailable?: boolean
  compact?: boolean
}>()
const emit = defineEmits<{ disclosure: [open: boolean] }>()
const layerLimit = ref(20)
const imageLimit = ref(5),
  detailsOpen = ref(false)
function toggleDetails(event: Event) {
  detailsOpen.value = (event.target as HTMLDetailsElement).open
  emit('disclosure', detailsOpen.value)
}
const phaseLabel = computed(() =>
  props.detailUnavailable
    ? 'Execution outcome not confirmed'
    : props.run?.status === 'running'
      ? {
          waiting: 'Awaiting execution',
          checking_repository: 'Checking repository',
          pulling: 'Pulling images',
          applying: 'Applying Compose',
          pruning: 'Pruning unused images',
          finished: 'Finished',
        }[props.run.phase]
      : props.run?.status === 'completed'
        ? 'Run completed · Runtime observation pending'
        : props.run?.status === 'cancelled'
          ? 'Cancelled by server'
          : 'Run failed',
)
function bytes(value?: number | null) {
  if (value == null) return 'unknown'
  return value >= 1048576
    ? `${(value / 1048576).toFixed(1)} MiB`
    : value >= 1024
      ? `${(value / 1024).toFixed(1)} KiB`
      : `${value} B`
}
</script>
