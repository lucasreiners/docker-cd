<template>
  <section class="stack-actions" :class="{ 'stack-actions--compact': compact }" :aria-label="`Actions for ${stack.path}`">
    <template v-if="stack.requiresAcknowledgement">
      <router-link class="button" :to="reviewTo || defaultReviewTo" @click="emit('review')">Review change</router-link>
      <button v-if="reviewedRevision" class="button button-primary" :disabled="!!approvalReason" @click="approve">Approve &amp; deploy {{ reviewedRevision }}</button>
      <p :class="compact ? 'inventory-sr-only' : 'muted'">{{ approvalReason || `Approval applies only to ${stack.path} at ${reviewedRevision}.` }}</p>
    </template>
    <template v-else>
      <button ref="trigger" class="button" :disabled="!!reason" :title="reason || undefined" :aria-describedby="reason ? reasonId : undefined" @click="open">{{ action === 'retry' ? 'Retry deployment…' : 'Update images…' }}</button>
      <p v-if="reason" :id="reasonId" :class="compact ? 'inventory-sr-only' : 'muted'">{{ reason }}</p>
      <div v-if="confirmation" class="inline-confirm" role="group" :aria-label="`Confirm action for ${confirmation.stack}`" @keydown.esc="close">
        <strong>{{ confirmation.stack }}</strong>
        <p>Pull images and apply Compose at reviewed revision <code>{{ confirmation.revision }}</code>. Unchanged containers may remain running. No image prune.</p>
        <p v-if="revisionChanged" role="alert">Desired revision changed; review the stack again.</p>
        <button ref="confirmButton" class="button button-primary" :disabled="!!reason || revisionChanged" @click="submit">{{ confirmation.action === 'retry' ? `Retry ${confirmation.stack}` : `Update ${confirmation.stack}` }}</button>
        <button class="button" @click="close">Cancel</button>
      </div>
    </template>
    <div v-if="notice && ['failed', 'uncertain'].includes(notice.phase)" role="alert" class="request-warning">
      <p>{{ notice.message }}</p>
      <router-link to="/operations">View operations</router-link>
      <button class="button" @click="actions.dismissNotice(key)">Dismiss</button>
    </div>
  </section>
</template>
<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import { stackActionKey, useOperationActions } from '../composables/useOperationActions'
import type { ReviewedStackAction, StackRecord } from '../services/api'
import { useStacksStore } from '../store/stacks'

const props = defineProps<{
  stack: StackRecord
  reviewedRevision?: string | null
  reviewTo?: RouteLocationRaw
  compact?: boolean
}>()
const emit = defineEmits<{ review: [] }>()
const reasonId = useId()
const actions = useOperationActions()
const stacks = useStacksStore()
const key = computed(() => stackActionKey(props.stack.path))
const notice = computed(() => actions.notice(key.value))
const action = computed(() => (props.stack.status === 'failed' ? 'retry' : 'update'))
const currentRevision = computed(
  () => props.stack.desiredRevision || stacks.refreshStatus?.revision || '',
)
const reason = computed(
  () =>
    actions.stackReason(props.stack, action.value) ||
    (!currentRevision.value ? 'Desired revision unavailable' : ''),
)
const approvalInvalid = ref(false)
const attemptedApproval = ref(false)
const approvalReason = computed(
  () =>
    actions.stackReason(props.stack, 'acknowledge') ||
    (approvalInvalid.value ? 'Approval rejected; load the source comparison again' : '') ||
    (!props.reviewedRevision
      ? 'Load the source comparison before approving this revision'
      : props.reviewedRevision !== currentRevision.value
        ? 'Desired revision changed; review the stack again'
        : ''),
)
const defaultReviewTo = computed<RouteLocationRaw>(() => ({
  name: 'stack-detail',
  params: { path: props.stack.path.split('/') },
  query: { tab: 'configuration' },
}))
const confirmation = ref<ReviewedStackAction | null>(null)
const trigger = ref<HTMLButtonElement>()
const confirmButton = ref<HTMLButtonElement>()
const revisionChanged = computed(
  () =>
    !!confirmation.value &&
    (confirmation.value.revision !== currentRevision.value ||
      confirmation.value.stack !== props.stack.path),
)
async function open() {
  if (reason.value) return
  confirmation.value = {
    action: action.value,
    stack: props.stack.path,
    revision: currentRevision.value,
  }
  await nextTick()
  confirmButton.value?.focus()
}
function close() {
  confirmation.value = null
  void nextTick(() => trigger.value?.focus())
}
async function submit() {
  if (!confirmation.value || reason.value || revisionChanged.value) return
  const frozen = { ...confirmation.value }
  close()
  await actions.submitStack(frozen)
}
async function approve() {
  if (approvalReason.value || !props.reviewedRevision) return
  attemptedApproval.value = true
  await actions.submitStack({
    action: 'acknowledge',
    stack: props.stack.path,
    revision: props.reviewedRevision,
  })
  if (actions.notice(key.value)?.phase === 'failed') approvalInvalid.value = true
}
watch(
  () => props.reviewedRevision,
  (revision) => {
    if (revision) {
      approvalInvalid.value = false
      attemptedApproval.value = false
    }
  },
)
watch(
  () => notice.value?.phase,
  (phase) => {
    if (phase === 'failed' && attemptedApproval.value) approvalInvalid.value = true
  },
)
watch(() => props.stack.path, close)
</script>
<style scoped>
.stack-actions { min-width: 0; overflow-wrap: anywhere; }
.button { max-width: 100%; white-space: normal; overflow-wrap: anywhere; }
.stack-actions p { margin: .5rem 0; }
.inline-confirm { border: 1px solid var(--border-color, currentColor); padding: .75rem; margin-top: .5rem; }
.request-warning { margin-top: .5rem; }
.inline-confirm .button { margin: .25rem .5rem .25rem 0; }
</style>
