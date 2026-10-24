<template>
  <div
    class="admin-app"
    :class="{ 'sidebar-collapsed': consoleStore.collapsed }"
    :data-density="consoleStore.density"
  >
    <a class="skip-link button" href="#main-content">Skip to content</a>
    <aside class="sidebar desktop-sidebar" aria-label="Main navigation">
      <router-link to="/stacks" class="admin-brand" aria-label="Docker-CD stacks"
        ><Boxes :size="25" /><span>Docker <b>/ CD</b></span></router-link
      >
      <nav>
        <p class="nav-section">Workspace</p>
        <router-link
          v-for="item in navigation.slice(0, 2)"
          :key="item.path"
          :to="item.path"
          :class="{ 'nav-active': activePath(item.path) }"
          :title="item.label"
          ><component :is="item.icon" :size="18" /><span>{{ item.label }}</span
          ><small v-if="item.path === '/stacks' && attention">{{
            attention
          }}</small
          ><small
            v-if="item.path === '/operations' && workflowCount"
            >{{ workflowCount }}</small
          ></router-link
        >
        <p class="nav-section">Administration</p>
        <router-link
          v-for="item in navigation.slice(2)"
          :key="item.path"
          :to="item.path"
          :class="{ 'nav-active': activePath(item.path) }"
          :title="item.label"
          ><component :is="item.icon" :size="18" /><span>{{
            item.label
          }}</span></router-link
        >
      </nav>
      <div class="sidebar-bottom">
        <span>Agent {{ consoleStore.instance?.version || "dev" }}</span
        ><button
          :aria-label="
            consoleStore.collapsed ? 'Expand sidebar' : 'Collapse sidebar'
          "
          @click="consoleStore.setCollapsed(!consoleStore.collapsed)"
        >
          <PanelLeftClose
            v-if="!consoleStore.collapsed"
            :size="17"
          /><PanelLeftOpen v-else :size="17" />
        </button>
      </div>
    </aside>
    <div class="admin-workspace">
      <header class="admin-header">
        <button
          class="icon-button mobile-menu"
          aria-label="Open navigation"
          @click="mobileNavigation = true"
        >
          <Menu :size="19" />
        </button>
        <div class="breadcrumbs">
          <span class="breadcrumb-workspace"
            >Workspace <ChevronRight :size="13" /></span
          ><strong>{{ pageTitle }}</strong>
        </div>
        <div class="header-tools">
          <button
            class="command-trigger"
            aria-label="Search workspace"
            @click="searchOpen = true"
          >
            <Search :size="15" /><span>Search workspace</span
            ><kbd>⌘ K</kbd></button
          ><button class="button header-work" aria-label="Open instance work" @click="workOpen = true">Work <span>{{ workflowCount }}</span></button><ConnectionBanner /><ThemeToggle />
        </div>
      </header>
      <main id="main-content" class="admin-content" tabindex="-1">
        <div
          v-if="consoleStore.instanceError"
          class="notice notice-warning"
          role="alert"
        >
          Instance information unavailable: {{ consoleStore.instanceError }}
          <button class="text-link" @click="consoleStore.loadInstance()">
            Retry
          </button>
        </div>
        <div
          v-if="consoleStore.instance?.history.error"
          class="notice notice-error"
          role="alert"
        >
          Operation history could not be saved:
          {{ consoleStore.instance.history.error }}
        </div>
        <router-view />
      </main>
    </div>
    <Drawer
      v-model:visible="workOpen"
      header="Instance work"
      position="right"
      style="width: min(620px, 100vw)"
    ><ActiveWork /><router-link class="button" to="/operations" @click="workOpen = false">Open operations history</router-link></Drawer>
    <Drawer
      v-model:visible="mobileNavigation"
      header="Docker / CD"
      class="navigation-drawer"
      ><nav class="mobile-nav">
        <router-link
          v-for="item in navigation"
          :key="item.path"
          :to="item.path"
          @click="mobileNavigation = false"
          ><component :is="item.icon" :size="18" />{{ item.label }}</router-link
        >
      </nav></Drawer
    >
    <Drawer
      :visible="!!consoleStore.inspectedId"
      header="Operation inspector"
      position="right"
      style="width: min(510px, 100vw)"
      class="operation-drawer"
      @after-hide="restoreOperationFocus"
      @update:visible="
        (value) => {
          if (!value) consoleStore.inspectedId = '';
        }
      "
      ><OperationDetails :id="consoleStore.inspectedId" /><template #footer
        ><router-link
          class="button"
          :to="`/operations/${consoleStore.inspectedId}`"
          @click="consoleStore.inspectedId = ''"
          >Open full operation</router-link
        ></template
      ></Drawer
    >
    <Dialog
      v-model:visible="searchOpen"
      modal
      header="Search workspace"
      class="command-dialog"
      ><input
        v-model="search"
        class="command-input"
        aria-label="Search stacks and pages"
        placeholder="Find a stack or page…"
        autofocus
      />
      <div class="command-results">
        <router-link
          v-for="item in searchResults"
          :key="item.path"
          :to="item.path"
          @click="searchOpen = false"
          ><component :is="item.icon" :size="17" /><span>{{ item.label }}</span
          ><ArrowUpRight :size="13"
        /></router-link>
        <p v-if="!searchResults.length" class="muted">
          No matching stacks or pages.
        </p>
      </div></Dialog
    >
  </div>
</template>
<script setup lang="ts">
import {
  ArrowUpRight,
  Boxes,
  ChevronRight,
  GitBranch,
  Layers,
  ListChecks,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  Search,
  Settings2,
} from 'lucide-vue-next'
import Dialog from 'primevue/dialog'
import Drawer from 'primevue/drawer'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import ActiveWork from './components/ActiveWork.vue'
import ConnectionBanner from './components/ConnectionBanner.vue'
import OperationDetails from './components/OperationDetails.vue'
import ThemeToggle from './components/ThemeToggle.vue'
import { useWorkflowGroups } from './composables/useWorkflowGroups'
import { useConsoleStore } from './store/console'
import { useStacksStore } from './store/stacks'
import { needsAttention } from './utils/stackState'

const consoleStore = useConsoleStore(),
  store = useStacksStore(),
  route = useRoute()
const searchOpen = ref(false),
  workOpen = ref(false),
  mobileNavigation = ref(false),
  search = ref('')
const navigation = [
  { path: '/stacks', label: 'Stacks', icon: Layers },
  { path: '/operations', label: 'Operations', icon: ListChecks },
  { path: '/repository', label: 'Repository', icon: GitBranch },
  { path: '/system', label: 'System', icon: Settings2 },
]
const workflows = useWorkflowGroups()
const workflowCount = computed(() => workflows.value.length)
const activePath = (path: string) =>
  path === '/' ? route.path === '/' : route.path.startsWith(path)
const pageTitle = computed(
  () => navigation.find((item) => activePath(item.path))?.label || 'Stack detail',
)
const attention = computed(
  () =>
    store.stacks.filter((stack) => needsAttention(stack, consoleStore.runtimeFor(stack.path)))
      .length,
)
const searchResults = computed(() =>
  [
    ...navigation,
    ...store.stacks.map((stack) => ({
      path: `/stacks/${stack.path.split('/').map(encodeURIComponent).join('/')}`,
      label: stack.path,
      icon: Boxes,
    })),
  ]
    .filter((item) => item.label.toLowerCase().includes(search.value.toLowerCase()))
    .slice(0, 15),
)
function keydown(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    searchOpen.value = !searchOpen.value
  }
}
let operationFocus: HTMLElement | null = null
let previousOperation = ''
watch(
  () => consoleStore.inspectedId,
  (id, previous) => {
    if (id && !previous) {
      operationFocus = document.activeElement as HTMLElement
      previousOperation = id
    }
  },
  { flush: 'sync' },
)
function restoreOperationFocus() {
  const target = operationFocus?.isConnected
    ? operationFocus
    : document.querySelector<HTMLElement>(`[data-operation-id="${CSS.escape(previousOperation)}"]`)
  target?.focus()
}
watch(
  () => store.stacks,
  () => {
    consoleStore.refreshRuntime()
  },
)
watch(
  () => route.fullPath,
  () => {
    document.title = `${pageTitle.value} · ${consoleStore.instance?.name || 'Docker-CD'}`
  },
  { immediate: true },
)
onMounted(async () => {
  window.addEventListener('keydown', keydown)
  store.connectSSE(consoleStore.onOperation, () => {
    consoleStore.loadInstance()
    consoleStore.loadOperations()
    consoleStore.refreshRuntime(true)
  })
  consoleStore.startRuntimePolling(consoleStore.loadInstance)
  await Promise.all([
    store.loadInitial(),
    consoleStore.loadInstance(),
    consoleStore.loadOperations(),
  ])
})
onUnmounted(() => {
  window.removeEventListener('keydown', keydown)
  consoleStore.stopRuntimePolling()
  store.disconnectSSE()
})
</script>
