<template>
  <Drawer
    id="container-image-inspector"
    :visible="!!selection"
    position="right"
    class="container-inspector"
    :style="{ width: 'min(560px, 100vw)' }"
    :aria-label="
      selection
        ? `Image inspection for ${selection.container.name} in ${selection.stack}`
        : 'Image inspection'
    "
    block-scroll
    :header="
      selection
        ? `${selection.container.name} / ${selection.stack}`
        : 'Image inspection'
    "
    @update:visible="
      (visible) => {
        if (!visible) close();
      }
    "
    @after-hide="restoreFocus"
  >
    <template v-if="selection && current && selected">
      <p v-if="removed" role="status" class="inventory-warning">
        Container no longer observed. Retaining the selected image identity; no
        replacement has been selected.
      </p>
      <p
        v-else-if="comparison?.currentUnavailable"
        role="status"
        class="inventory-warning"
      >
        Current image metadata is
        {{ comparison.currentStatus.replace(/_/g, " ") }}. Retaining the
        selected, dated identity.
      </p>
      <p
        v-if="!removed && comparison?.referenceChanged"
        role="status"
        class="inventory-warning"
      >
        The reported image reference has changed. The selected reference below
        has not been replaced.
      </p>
      <p
        v-if="!removed && comparison?.metadataChanged"
        class="inventory-secondary"
      >
        Current image metadata differs from this selection. This is not a
        registry freshness check. Close and inspect again to select the new
        observation.
      </p>
      <p v-if="runtime?.error || runtime?.stale" class="inventory-warning">
        {{
          runtime?.error ? "Runtime unavailable" : "Runtime observation stale"
        }}. Last successful runtime observation:
        {{ runtime?.observedAt || "Unknown" }}.
      </p>
      <p>
        {{ removed ? "Last known runtime" : "Runtime" }}: {{ current.state }} ·
        {{
          !current.health || current.health === "none"
            ? "no health check"
            : current.health
        }}
      </p>
      <dl class="inventory-inspection-data">
        <div>
          <dt>Running image reference</dt>
          <dd>
            <code>{{
              selected.imageIdentity?.reference || selected.image
            }}</code>
          </dd>
        </div>
        <div>
          <dt>Repository digest</dt>
          <dd v-if="preferred">
            <code>{{ preferred.repository }}@{{ preferred.digest }}</code
            ><button
              type="button"
              class="inventory-control"
              @click="copyReference"
            >
              Copy reference
            </button>
          </dd>
          <dd v-else>
            {{
              selected.imageIdentity?.status === "ambiguous"
                ? "Ambiguous — no preferred repository digest"
                : "Digest unavailable"
            }}<span v-if="selected.imageIdentity?.reason"> · {{ reason }}</span
            ><span v-else-if="!selected.imageIdentity">
              · Not reported by this backend</span
            >
          </dd>
        </div>
        <div>
          <dt>Digest kind</dt>
          <dd>{{ kindLabel }} (reported by local daemon)</dd>
        </div>
        <div>
          <dt>Running image platform</dt>
          <dd>{{ platform }}</dd>
        </div>
        <div>
          <dt>Image inspected</dt>
          <dd>{{ selected.imageIdentity?.observedAt || "Not recorded" }}</dd>
        </div>
        <div>
          <dt>Observation source</dt>
          <dd>
            {{
              selected.imageIdentity?.source === "container_image_inspect"
                ? "Local container → immutable image inspection"
                : "Not reported"
            }}
          </dd>
        </div>
        <div>
          <dt>Selected runtime observation</dt>
          <dd>{{ selection.observedAt || "Not recorded" }}</dd>
        </div>
        <div>
          <dt>Registry freshness</dt>
          <dd>Not checked</dd>
        </div>
      </dl>
      <p class="inventory-secondary">
        Compare against the same repository and digest kind. A platform manifest
        differs from its multi-platform index. Different digests do not
        establish which artifact is newer.
      </p>
      <details v-if="alternatives.length" class="inventory-inspection-details">
        <summary>
          Show other reported digests ({{ alternatives.length }})
        </summary>
        <ul>
          <li
            v-for="digest in alternatives"
            :key="`${digest.repository}@${digest.digest}`"
          >
            <code>{{ digest.repository }}@{{ digest.digest }}</code
            ><span> · {{ digest.kind }}</span>
          </li>
        </ul>
      </details>
      <details class="inventory-inspection-details">
        <summary>Technical identifiers</summary>
        <dl class="inventory-inspection-data">
          <div>
            <dt>Local image ID</dt>
            <dd>
              <code>{{
                selected.imageIdentity?.imageId || "Not recorded"
              }}</code>
            </dd>
          </div>
          <div>
            <dt>
              Container ID{{
                selected.id.length < 64 ? " (short, as reported)" : ""
              }}
            </dt>
            <dd>
              <code>{{ selected.id }}</code>
            </dd>
          </div>
        </dl>
      </details>
      <div class="inventory-inspector-links">
        <router-link
          v-if="logsAvailable"
          :to="{
            name: 'stack-detail',
            params: { path: selection.stack.split('/') },
            query: { ...query, tab: 'logs', container: selection.id },
          }"
          @click="close"
          >View logs</router-link
        >
        <router-link
          :to="{
            name: 'stack-detail',
            params: { path: selection.stack.split('/') },
            query,
          }"
          @click="close"
          >Open stack</router-link
        >
      </div>
      <p v-if="selected.ports" class="inventory-secondary">
        Ports: {{ selected.ports }}
      </p>
      <p role="status" class="inventory-secondary">{{ copyNotice }}</p>
    </template>
  </Drawer>
</template>
<script setup lang="ts">
import Drawer from 'primevue/drawer'
import { computed, ref, watch } from 'vue'
import type { LocationQuery } from 'vue-router'
import type { ContainerInfo } from '../services/api'
import type {
  ContainerInspectionIdentityComparison,
  ContainerInspectionSelection,
  RuntimeView,
} from '../store/console'

const props = withDefaults(
  defineProps<{
    selection: ContainerInspectionSelection | null
    /** console.inspectedContainerCurrent: pinned image identity plus live operational state. */
    current: ContainerInfo | null
    removed?: boolean
    comparison?: ContainerInspectionIdentityComparison | null
    runtime?: RuntimeView | null
    trigger?: HTMLElement | null
    query?: LocationQuery
    logsAvailable?: boolean
  }>(),
  { logsAvailable: true },
)
const emit = defineEmits<{ close: [] }>()
const selected = computed(() => props.selection?.container)
const identity = computed(() => props.selection?.container.imageIdentity)
const preferred = computed(() =>
  identity.value?.status === 'available' ? identity.value.preferredRepositoryDigest : undefined,
)
const alternatives = computed(
  () =>
    identity.value?.repositoryDigests.filter(
      (digest) =>
        digest.repository !== preferred.value?.repository ||
        digest.digest !== preferred.value?.digest,
    ) ?? [],
)
const kindLabel = computed(
  () =>
    ({
      index: 'Multi-platform index',
      manifest: 'Platform manifest',
      unknown: 'Unknown',
    })[preferred.value?.kind ?? 'unknown'],
)
const platform = computed(() => {
  const p = identity.value?.platform
  return p ? [p.os, p.architecture, p.variant].filter(Boolean).join('/') : 'Unknown'
})
const reason = computed(() => identity.value?.reason?.replace(/_/g, ' '))
const copyNotice = ref('')
let returnTarget: HTMLElement | null = null
let returnStack = ''
watch(
  () => props.selection,
  (selection, previous) => {
    if (selection && !previous) {
      returnTarget = props.trigger || (document.activeElement as HTMLElement | null)
      returnStack = selection.stack
      copyNotice.value = ''
    }
  },
  { immediate: true },
)
function close() {
  emit('close')
}
function restoreFocus() {
  if (returnTarget?.isConnected) {
    returnTarget.focus()
    return
  }
  const heading = Array.from(document.querySelectorAll<HTMLElement>('[data-inventory-focus]')).find(
    (node) => node.dataset.inventoryFocus === `stack:${returnStack}`,
  )
  const target = heading || document.querySelector<HTMLElement>('main h1, #main-content h1, h1')
  if (target) {
    target.tabIndex = -1
    target.focus()
  }
}
async function copyReference() {
  copyNotice.value = ''
  if (!preferred.value) return
  try {
    await navigator.clipboard.writeText(`${preferred.value.repository}@${preferred.value.digest}`)
    copyNotice.value = 'Full repository reference copied'
  } catch {
    copyNotice.value = 'Copy unavailable. Select the full reference above to copy manually.'
  }
}
</script>
