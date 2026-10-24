<template>
  <div class="image-identity" :class="{ 'image-identity--compact': part }">
    <span v-if="part !== 'digest'" class="inventory-mono image-reference" :title="identity?.reference || image">{{
      identity?.reference || image
    }}</span>
    <template v-if="preferred && part !== 'reference'">
      <span class="image-digest" :title="reference"
        ><span v-if="!part">Repository digest </span
        ><code :aria-label="preferred.digest">{{ shortened }}</code></span
      >
      <button
        type="button"
        class="inventory-control"
        :aria-label="`Copy repository digest for ${image}`"
        @click="copy"
      >
        <Copy v-if="part" :size="13" aria-hidden="true" /><span v-else>Copy digest</span>
      </button>
    </template>
    <span v-else-if="part !== 'reference'" class="inventory-secondary" :aria-label="part ? (identity?.status === 'ambiguous' ? 'Repository digest ambiguous' : 'Digest unavailable') : undefined">{{ part ? '—' :
      identity?.status === "ambiguous"
        ? "Repository digest ambiguous"
        : "Digest unavailable"
    }}</span>
    <span class="inventory-sr-only" role="status">{{ notice }}</span>
  </div>
</template>
<script setup lang="ts">
import { Copy } from 'lucide-vue-next'
import { computed, ref } from 'vue'
import type { ImageIdentity } from '../services/api'

const props = defineProps<{
  image: string
  identity?: ImageIdentity
  part?: 'reference' | 'digest'
}>()
const preferred = computed(() =>
  props.identity?.status === 'available' ? props.identity.preferredRepositoryDigest : undefined,
)
const reference = computed(() =>
  preferred.value ? `${preferred.value.repository}@${preferred.value.digest}` : '',
)
const shortened = computed(() => {
  const value = preferred.value?.digest ?? ''
  if (props.part) return value.length > 16 ? `${value.slice(0, 11)}…${value.slice(-4)}` : value
  return value.length > 28 ? `${value.slice(0, 19)}…${value.slice(-8)}` : value
})
const notice = ref('')
async function copy() {
  notice.value = ''
  if (!preferred.value) return
  try {
    await navigator.clipboard.writeText(preferred.value.digest)
    notice.value = 'Full repository digest copied'
  } catch {
    notice.value = 'Copy unavailable. Open image inspection to select the full repository digest.'
  }
}
</script>
