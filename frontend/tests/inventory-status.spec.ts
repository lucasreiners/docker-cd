import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import InventoryStatus from '../src/components/InventoryStatus.vue'

describe('inventory status cues', () => {
  it.each([
    ['success', 'Applied', 'check'],
    ['active', 'Applying', 'loader-circle'],
    ['warning', 'Approval required', 'triangle-alert'],
    ['danger', 'Unhealthy', 'octagon-x'],
    ['neutral', 'Stopped', 'circle'],
    ['unknown', 'Runtime unknown', 'circle-question-mark'],
  ] as const)('pairs %s color with text and a meaningful icon', (tone, label, icon) => {
    const wrapper = mount(InventoryStatus, { props: { tone, label } })
    expect(wrapper.text()).toBe(label)
    expect(wrapper.classes()).toContain(`inventory-status--${tone}`)
    expect(wrapper.get(`svg.lucide-${icon}`).attributes('aria-hidden')).toBe('true')
    expect(wrapper.find('.inventory-spin').exists()).toBe(tone === 'active')
    wrapper.unmount()
  })
})
