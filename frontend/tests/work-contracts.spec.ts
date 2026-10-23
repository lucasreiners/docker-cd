import { describe, expect, it } from 'vitest'
import { fixtureData, operationalWorkspaceFixtures } from '../../tests/e2e/fixtures/frontend-data'
import type { WorkRun } from '../src/services/work'

describe('operational workspace contract fixtures (no application consumer)', () => {
  it('leaves default fixtures and advertised capabilities unchanged', () => {
    const legacy = fixtureData()
    expect(legacy.instance.capabilities).not.toHaveProperty('liveWorkVersion')
    expect(legacy.instance.capabilities).not.toHaveProperty('imageIdentity')
    for (const containers of Object.values(legacy.containers)) {
      for (const container of containers) expect(container).not.toHaveProperty('imageIdentity')
    }
    expect(operationalWorkspaceFixtures().capabilities).toEqual({
      liveWorkVersion: 2,
      imageIdentity: { version: 1, source: 'container_image_inspect' },
    })
  })

  it('serializes deterministic snapshot, upsert, terminal and restart payloads', () => {
    const first = operationalWorkspaceFixtures()
    const second = operationalWorkspaceFixtures()
    expect(first).toEqual(second)
    expect(JSON.parse(JSON.stringify(first))).toEqual(first)
    expect(first.events.map(({ data }) => [data.epoch, data.sequence])).toEqual([
      ['fixture-agent-a', 10],
      ['fixture-agent-a', 11],
      ['fixture-agent-a', 12],
      ['fixture-agent-a', 13],
      ['fixture-agent-a', 14],
      ['fixture-agent-a', 15],
      ['fixture-agent-a', 16],
      ['fixture-agent-a', 20],
      ['fixture-agent-b', 0],
    ])
    expect(first.reconnect.runs).toEqual([])
    expect(first.reconnect.operations).toEqual([])
    expect(first.restart.epoch).not.toBe(first.snapshot.epoch)
    expect(first.interruptedOperation.status).toBe('interrupted')
  })

  it('bootstraps an old active child and its completed ancestor outside latest-50 history', () => {
    const { snapshot, latest50 } = operationalWorkspaceFixtures()
    const [childRun, schedulerRun] = snapshot.runs
    const child = snapshot.operations.find((operation) => operation.id === childRun.operationId)
    const parent = snapshot.operations.find(
      (operation) => operation.id === childRun.rootOperationId,
    )
    expect(child?.parentId).toBe(parent?.id)
    expect(child?.status).toBe('running')
    expect(parent?.status).toBe('completed')
    expect(latest50).toHaveLength(50)
    expect(
      latest50.some((operation) => operation.id === child?.id || operation.id === parent?.id),
    ).toBe(false)
    expect((child?.startedAt ?? '') < latest50[49].startedAt).toBe(true)
    expect(new Set(snapshot.runs.map((run) => run.rootOperationId)).size).toBe(2)
    expect(schedulerRun.rootOperationId).not.toBe(childRun.rootOperationId)
    for (const operation of snapshot.operations) {
      expect(operation.version).toBeGreaterThan(0)
      expect(operation).not.toHaveProperty('steps')
    }
  })

  it('includes lock-wait and no-image operation-only envelopes without a null/fake run', () => {
    const { operationOnly, checkOnly } = operationalWorkspaceFixtures()
    for (const event of [operationOnly, checkOnly]) {
      expect(event).not.toHaveProperty('run')
      expect(event.operations).toHaveLength(1)
      expect(event.operations[0].status).toBe('running')
    }
    expect(operationOnly.operations[0].kind).toBe('action')
    expect(checkOnly.operations[0].kind).toBe('refresh')
  })

  it('isolates identical image/layer identifiers using run and image-attempt identity', () => {
    const { snapshot, bytes, terminal, retry, pruning } = operationalWorkspaceFixtures()
    const [child, scheduler] = snapshot.runs
    expect(child.images[0].reference).toBe(scheduler.images[0].reference)
    expect(child.images[0].layers[0].id).toBe(scheduler.images[0].layers[0].id)
    expect(child.images[0].pullAttemptId).not.toBe(scheduler.images[0].pullAttemptId)
    expect(bytes.run?.images[0].layers[0].bytesCurrent).toBe(256)
    expect(scheduler.images[0].layers[0].bytesCurrent).toBe(128)
    expect(bytes.run?.images[0].layers[0].bytesTotal).toBeNull()
    expect(terminal.run?.status).toBe('completed')
    expect(terminal.run?.finishedAt).toBeDefined()
    expect(
      snapshot.operations.find((operation) => operation.id === scheduler.operationId)?.status,
    ).toBe('running')
    expect(pruning.run?.rootOperationId).toBe(scheduler.rootOperationId)
    expect(pruning.run?.phase).toBe('pruning')
    expect(pruning.run).not.toHaveProperty('stack')
    expect(retry.run?.runId).not.toBe(terminal.run?.runId)
    expect(retry.run?.images[0].pullAttemptId).not.toBe(terminal.run?.images[0].pullAttemptId)
    expect(retry.run?.cycleId).toBe(terminal.run?.cycleId)
  })

  it('declares omitted detail while preserving every active run summary and known counts', () => {
    const { snapshot, truncated, events } = operationalWorkspaceFixtures()
    expect(truncated.runs.map((run) => run.runId)).toEqual(snapshot.runs.map((run) => run.runId))
    expect(truncated.operations).toEqual(snapshot.operations)
    expect(truncated.detailsTruncated).toBe(true)
    expect(truncated.runs[0].imageCount).toBe(40)
    expect(truncated.runs[0].images).toEqual([])
    expect(truncated.runs[0].detailsTruncated).toBe(true)
    for (const { data } of [...events, { data: truncated }]) {
      const runs: WorkRun[] = 'runs' in data ? data.runs : data.run ? [data.run] : []
      let detailBytes = 0
      for (const run of runs) {
        expect(run.images.length).toBeLessThanOrEqual(32)
        expect(run.images.length).toBeLessThanOrEqual(run.imageCount)
        if (!run.detailsTruncated) expect(run.images).toHaveLength(run.imageCount)
        detailBytes += new TextEncoder().encode(JSON.stringify(run.images)).length
        for (const image of run.images) {
          expect(image.layers.length).toBeLessThanOrEqual(200)
          if (!image.detailsTruncated) expect(image.layers).toHaveLength(image.layerCount)
        }
        if (run.stackResults) {
          const results = run.stackResults
          expect(results.outcomes.length).toBeLessThanOrEqual(100)
          expect(results.failed).toBeLessThanOrEqual(results.processed)
          if (!results.detailsTruncated) expect(results.outcomes).toHaveLength(results.outcomeCount)
          detailBytes += new TextEncoder().encode(JSON.stringify(results.outcomes)).length
        }
      }
      expect(detailBytes).toBeLessThanOrEqual(2_097_152)
    }
  })

  it('preserves full unknown-kind repository digests without inventing a preferred identity', () => {
    const { identities } = operationalWorkspaceFixtures()
    expect(identities.uniqueUnknownKind.status).toBe('available')
    expect(identities.uniqueUnknownKind.preferredRepositoryDigest?.kind).toBe('unknown')
    expect(identities.multipleRepositories.repositoryDigests).toHaveLength(2)
    expect(identities.ambiguous.status).toBe('ambiguous')
    expect(identities.localBuild.repositoryDigests).toEqual([])
    expect(identities.localBuild.imageId).toMatch(/^sha256:[a-f0-9]{64}$/)
    for (const identity of Object.values(identities)) {
      expect(identity.freshness).toBe('not_checked')
      expect(identity.source).toBe('container_image_inspect')
      expect(identity.observedAt).toBeTruthy()
      for (const digest of identity.repositoryDigests) {
        expect(digest.digest).toMatch(/^sha256:[a-f0-9]{64}$/)
        expect(digest.kind).toBe('unknown')
      }
      if (identity.status === 'available') {
        expect(identity.repositoryDigests).toContainEqual(identity.preferredRepositoryDigest)
        expect(identity).not.toHaveProperty('reason')
      } else {
        expect(identity.reason).toBeTruthy()
        expect(identity).not.toHaveProperty('preferredRepositoryDigest')
      }
      expect(identity).not.toHaveProperty('env')
      expect(identity).not.toHaveProperty('rawInspect')
      expect(identity).not.toHaveProperty('checkedAt')
    }
  })
})
