<template>
  <div>
    <div class="page-heading">
      <div>
        <p class="eyebrow">Administration</p>
        <h1>System</h1>
        <p>Instance configuration, policies and your workspace preferences.</p>
      </div>
      <button class="button" @click="consoleStore.reload()">
        <RefreshCw :size="14" />Refresh status
      </button>
    </div>
    <div class="settings-grid">
      <section class="surface">
        <div class="panel-heading">
          <h2>Instance</h2>
          <Server :size="17" />
        </div>
        <dl class="definition-list padded">
          <div>
            <dt>Name</dt>
            <dd>{{ instance?.name || "Docker-CD" }}</dd>
          </div>
          <div>
            <dt>Version</dt>
            <dd>
              <code>{{ instance?.version || "dev" }}</code>
            </dd>
          </div>
          <div>
            <dt>Docker endpoint</dt>
            <dd>
              <code>{{ instance?.dockerEndpoint || "Default socket" }}</code>
            </dd>
          </div>
          <div>
            <dt>Runtime inspection</dt>
            <dd>
              {{
                runtimeStatus
              }}
            </dd>
          </div>
          <div>
            <dt>Operation history</dt>
            <dd>
              {{
                instance?.history.persistent
                  ? "Persistent on agent volume"
                  : "Agent session only"
              }}
            </dd>
          </div>
          <div>
            <dt>Retention</dt>
            <dd>
              {{ instance?.history.retention || 1000 }} completed operations
            </dd>
          </div>
        </dl>
      </section>
      <section class="surface">
        <div class="panel-heading">
          <h2>Deployment policies</h2>
          <SlidersHorizontal :size="17" />
        </div>
        <div class="setting-row">
          <div>
            <strong>Automatic reconciliation</strong>
            <p>Apply eligible Compose changes from Git.</p>
          </div>
          <span class="tag">{{
            instance?.policies.reconcile ? "Enabled" : "Disabled"
          }}</span>
        </div>
        <div class="setting-row">
          <div>
            <strong>Remove deleted stacks</strong>
            <p>Remove runtime stacks deleted from Git.</p>
          </div>
          <span class="tag">{{
            instance?.policies.remove ? "Enabled" : "Disabled"
          }}</span>
        </div>
        <div class="setting-row">
          <div>
            <strong>Configuration drift</strong>
            <p>Behavior when applied configuration differs.</p>
          </div>
          <span class="tag">{{
            instance?.policies.drift === "flag"
              ? "Require review"
              : "Auto-apply"
          }}</span>
        </div>
        <p class="configuration-note padded">
          Managed by the agent environment. Update the deployment configuration
          to change these policies.
        </p>
      </section>
      <section class="surface">
        <div class="panel-heading">
          <h2>Image update schedule</h2>
          <Clock3 :size="17" />
        </div>
        <dl class="definition-list padded">
          <div>
            <dt>State</dt>
            <dd>{{ instance?.schedule.enabled ? "Enabled" : "Disabled" }}</dd>
          </div>
          <div>
            <dt>Effective cron</dt>
            <dd>
              <code>{{ instance?.schedule.cron || "—" }}</code>
            </dd>
          </div>
          <div>
            <dt>Timezone</dt>
            <dd>{{ instance?.schedule.timezone || "—" }}</dd>
          </div>
          <div>
            <dt>Next run</dt>
            <dd>
              <RelativeTime
                v-if="instance?.schedule.enabled"
                :value="instance.schedule.nextRun"
              /><span v-else>Not scheduled</span>
            </dd>
          </div>
          <div>
            <dt>Configuration</dt>
            <dd>
              <code>UPDATER_ENABLED<br />UPDATER_CRON</code>
            </dd>
          </div>
        </dl>
      </section>
      <section class="surface">
        <div class="panel-heading">
          <h2>Workspace preferences</h2>
          <Palette :size="17" />
        </div>
        <div class="setting-row">
          <div>
            <strong>Appearance</strong>
            <p>Saved in this browser.</p>
          </div>
          <ThemeToggle />
        </div>
        <div class="setting-row">
          <div>
            <label for="density"><strong>Table density</strong></label>
            <p>Choose a comfortable or compact workspace.</p>
          </div>
          <select
            id="density"
            :value="consoleStore.density"
            @change="
              consoleStore.setDensity(
                ($event.target as HTMLSelectElement).value,
              )
            "
          >
            <option value="comfortable">Comfortable</option>
            <option value="compact">Compact</option>
          </select>
        </div>
        <div class="setting-row">
          <div>
            <strong>Sidebar</strong>
            <p>Keep more space for resource tables.</p>
          </div>
          <button
            class="button"
            @click="consoleStore.setCollapsed(!consoleStore.collapsed)"
          >
            {{ consoleStore.collapsed ? "Expand" : "Collapse" }}
          </button>
        </div>
      </section>
    </div>
  </div>
</template>
<script setup lang="ts">
import { Clock3, Palette, RefreshCw, Server, SlidersHorizontal } from 'lucide-vue-next'
import { computed } from 'vue'
import RelativeTime from '../components/RelativeTime.vue'
import ThemeToggle from '../components/ThemeToggle.vue'
import { useConsoleStore } from '../store/console'

const consoleStore = useConsoleStore(),
  instance = computed(() => consoleStore.instance),
  observations = computed(() => Object.values(consoleStore.runtime))
const runtimeStatus = computed(() => {
  if (consoleStore.runtimeError) return consoleStore.runtimeError
  if (!observations.value.length) return 'Waiting for observation'
  const failures = observations.value.filter((o) => o.error).length
  return failures === observations.value.length
    ? 'Unavailable'
    : failures
      ? 'Partially available'
      : 'Available'
})
</script>
