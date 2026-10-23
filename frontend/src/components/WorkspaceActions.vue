<template>
  <section class="workspace-actions" aria-label="Workspace actions">
    <div class="action-buttons">
      <button v-if="showRuntime" class="button" :disabled="consoleStore.runtimeLoading" @click="consoleStore.loadRuntime()">Refresh runtime</button>
      <button v-if="showGit" ref="gitTrigger" class="button" :disabled="!!actions.workspaceReason('refresh')" @click="openGit">Check Git for changes…</button>
      <button v-if="showUpdate" ref="updateTrigger" class="button button-primary" :disabled="!!actions.workspaceReason('update')" @click="confirmation = 'update'">Update all stacks…</button>
    </div>
    <p v-if="showGit && actions.workspaceReason('refresh')" class="muted">{{ actions.workspaceReason('refresh') }}</p>
    <p v-if="showUpdate && actions.workspaceReason('update')" class="muted">{{ actions.workspaceReason('update') }}. <router-link to="/system">System configuration</router-link></p>
    <div v-if="confirmation === 'git-inline'" class="inline-confirm" role="group" aria-label="Confirm Git policy impact" @keydown.esc="close">
      <strong>May deploy/remove stacks according to policy</strong>
      <p>{{ policyImpact }}</p>
      <button ref="inlineConfirm" class="button button-primary" :disabled="!!actions.workspaceReason('refresh')" @click="submit('refresh')">Check Git and apply policy</button>
      <button class="button" @click="close">Cancel</button>
    </div>
    <template v-for="warning in warnings" :key="warning.key">
      <div class="request-warning" role="alert">
        <p>{{ warning.notice.message }}</p>
        <router-link to="/operations">View operations</router-link>
        <button class="button" @click="actions.dismissNotice(warning.key)">Dismiss</button>
      </div>
    </template>
    <Dialog :visible="confirmation === 'update' || confirmation === 'git-modal'" modal :header="confirmation === 'update' ? 'Update all stacks' : 'Interrupt work and check Git'" :style="{ width: 'min(36rem, calc(100vw - 2rem))' }" @update:visible="value => { if (!value) close() }" @after-hide="restoreFocus">
      <template v-if="confirmation === 'update'">
        <p>All stacks on this instance, regardless of inventory filters or the current page. Eligible stacks are processed sequentially; failed stacks are skipped and policy blocks remain enforced.</p>
        <p>Changed or unknown images may replace containers through Compose.</p>
        <p><strong>Host-wide prune:</strong> removes unused/dangling images, including images outside managed stacks.</p>
      </template>
      <template v-else>
        <p>Requests interruption; work may already have made changes.</p>
        <ul class="impact-workflows"><li v-for="workflow in actions.activeWorkflows" :key="workflow.key">{{ workflow.label }}</li></ul>
        <p>May deploy/remove stacks according to policy. {{ policyImpact }}</p>
      </template>
      <p v-if="actions.workspaceReason(confirmation === 'update' ? 'update' : 'refresh')" role="status">{{ actions.workspaceReason(confirmation === 'update' ? 'update' : 'refresh') }}</p>
      <template #footer>
        <button class="button" autofocus @click="close">Cancel</button>
        <button class="button button-primary" :disabled="!!actions.workspaceReason(confirmation === 'update' ? 'update' : 'refresh')" @click="submit(confirmation === 'update' ? 'update' : 'refresh')">{{ confirmation === 'update' ? 'Update all stacks and prune images' : 'Interrupt work and check Git' }}</button>
      </template>
    </Dialog>
  </section>
</template>
<script setup lang="ts">
import Dialog from 'primevue/dialog'
import { computed, nextTick, ref, watch } from 'vue'
import { useOperationActions } from '../composables/useOperationActions'
import { useConsoleStore } from '../store/console'

const props = withDefaults(
  defineProps<{ showRuntime?: boolean; showGit?: boolean; showUpdate?: boolean }>(),
  { showRuntime: true, showGit: true, showUpdate: true },
)
const actions = useOperationActions()
const consoleStore = useConsoleStore()
const confirmation = ref<'' | 'update' | 'git-inline' | 'git-modal'>('')
const gitTrigger = ref<HTMLButtonElement>()
const updateTrigger = ref<HTMLButtonElement>()
const inlineConfirm = ref<HTMLButtonElement>()
let returnTarget: 'update' | 'refresh' = 'refresh'
const shownActions = computed(() =>
  (['refresh', 'update'] as const).filter((action) =>
    action === 'refresh' ? props.showGit : props.showUpdate,
  ),
)
const warnings = computed(() =>
  shownActions.value.flatMap((action) => {
    const key = `workspace:${action}`
    const notice = actions.notice(key)
    return notice && ['failed', 'uncertain'].includes(notice.phase) ? [{ key, notice }] : []
  }),
)
const policyImpact = computed(() => {
  const policy = consoleStore.instance?.policies
  return `Reconciliation ${policy?.reconcile ? 'enabled' : 'disabled'}; removal ${policy?.remove ? 'enabled' : 'disabled'}; drift policy: ${policy?.drift || 'unknown'}.`
})
function openGit() {
  returnTarget = 'refresh'
  confirmation.value = actions.activeWorkflows.length ? 'git-modal' : 'git-inline'
}
watch(confirmation, async (value) => {
  if (value === 'update') returnTarget = 'update'
  if (value === 'git-inline') {
    await nextTick()
    inlineConfirm.value?.focus()
  }
})
// Active work can appear while the compact confirmation is open. Never submit
// an interruption through the idle confirmation surface.
watch(
  () => actions.activeWorkflows.length,
  (length) => {
    if (length && confirmation.value === 'git-inline') confirmation.value = 'git-modal'
  },
)
function restoreFocus() {
  ;(returnTarget === 'update' ? updateTrigger.value : gitTrigger.value)?.focus()
}
function close() {
  confirmation.value = ''
  void nextTick(restoreFocus)
}
async function submit(action: 'update' | 'refresh') {
  if (actions.workspaceReason(action)) return
  if (
    action === 'refresh' &&
    actions.activeWorkflows.length &&
    confirmation.value !== 'git-modal'
  ) {
    confirmation.value = 'git-modal'
    return
  }
  close()
  await actions.submitWorkspace(action)
}
</script>
<style scoped>
.workspace-actions { min-width: 0; }
.button { max-width: 100%; white-space: normal; overflow-wrap: anywhere; }
.impact-workflows { overflow-wrap: anywhere; }
.action-buttons { display: flex; flex-wrap: wrap; gap: .5rem; }
.workspace-actions p { margin: .5rem 0; overflow-wrap: anywhere; }
.inline-confirm { border: 1px solid var(--border-color, currentColor); padding: .75rem; margin-top: .75rem; }
.request-warning { margin-top: .75rem; }
.inline-confirm .button { margin: .25rem .5rem .25rem 0; }
</style>
