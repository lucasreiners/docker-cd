// Isolated browser-test backend. Never imported by the production application.
import { fixtureData, workspaceBrowserFixture } from './frontend-data'
import type { WorkSnapshot } from '../../../frontend/src/services/work'

let state = fixtureData()
let updateStatus = 202
let refreshStatus = 202
let work: WorkSnapshot | undefined
let runtimeStatus = 200, historyStatus = 200, runtimeDelay = 0
let observedAt: string | undefined
let holdActions = false
let runtimeRequests = 0, runtimeInFlight = 0, runtimeMaxInFlight = 0
const actions: { path: string; body: unknown }[] = []
const streams = new Set<ReadableStreamDefaultController<Uint8Array>>()
const encoder = new TextEncoder()
const encode = (event: string, data: unknown) => encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`)
function publish(event: string, data: unknown) {
  for (const stream of streams) {
    try { stream.enqueue(encode(event, data)) } catch { streams.delete(stream) }
  }
}

Bun.serve({
  hostname: '127.0.0.1', port: 18082, idleTimeout: 0,
  async fetch(request) {
    const path = new URL(request.url).pathname
    if (path === '/test/ready') return Response.json({ ready: true })
    if (path === '/test/reset') {
      state = fixtureData(); updateStatus = 202; refreshStatus = 202; work = undefined
      runtimeStatus = 200; historyStatus = 200; runtimeDelay = 0; observedAt = undefined
      holdActions = false
      runtimeRequests = 0; runtimeInFlight = 0; runtimeMaxInFlight = 0; actions.length = 0
      return Response.json({ ok: true })
    }
    if (path === '/test/state') return Response.json(state)
    if (path === '/test/metrics') return Response.json({ runtimeRequests, runtimeInFlight, runtimeMaxInFlight, actions })
    if (path === '/test/configure') {
      const config = await request.json()
      if (config.v2) { const next = workspaceBrowserFixture(); state = next; work = next.work }
      if ('work' in config) work = config.work
      if ('operations' in config) state.operations = config.operations
      if ('runtimeStatus' in config) runtimeStatus = config.runtimeStatus
      if ('historyStatus' in config) historyStatus = config.historyStatus
      if ('runtimeDelay' in config) runtimeDelay = config.runtimeDelay
      if ('observedAt' in config) observedAt = config.observedAt
      if ('holdActions' in config) holdActions = config.holdActions
      if (config.updateStatus) updateStatus = config.updateStatus
      if (config.refreshStatus) refreshStatus = config.refreshStatus
      if (config.stacks) state.stacks = config.stacks
      if (config.refresh) state.refresh = config.refresh
      if (config.instance) state.instance = config.instance
      if (config.containers) state.containers = config.containers
      return Response.json({ ok: true })
    }
    if (path === '/test/event') {
      const { event, data, containers } = await request.json()
      if (event === 'stack.upsert') {
        state.stacks = state.stacks.filter((s) => s.path !== data.record.path).concat(data.record)
        if (containers) state.containers[data.record.path] = containers
      }
      if (event === 'refresh.status') state.refresh = data
      if (event === 'operation.upsert') state.operations = [data,...state.operations.filter(o=>o.id!==data.id)]
      if (event === 'work.snapshot') work = data
      if (event === 'work.upsert' && work && data.epoch === work.epoch && data.sequence > work.sequence) {
        work = { ...work, sequence: data.sequence,
          runs: data.run ? [...work.runs.filter(run => run.runId !== data.run.runId), data.run].filter(run => run.status === 'running') : work.runs,
          operations: [...work.operations.filter(operation => !data.operations.some((next: {id: string}) => next.id === operation.id)), ...data.operations],
        }
      }
      publish(event, data)
      return Response.json({ ok: true })
    }
    if (path === '/test/disconnect') {
      for (const stream of streams) { try { stream.close() } catch {} }
      streams.clear()
      return Response.json({ ok: true })
    }
    if (path === '/api/stacks') return Response.json(state.stacks)
    if (path === '/api/instance') return Response.json(state.instance)
    if (path === '/api/container-logs') {
      let keepAlive:ReturnType<typeof setInterval>
      const body=new ReadableStream<Uint8Array>({start(controller){controller.enqueue(encoder.encode('2026-10-11T12:00:00Z INFO application started\n2026-10-11T12:00:01Z ERROR health probe timed out\n'));keepAlive=setInterval(()=>{controller.enqueue(encoder.encode('2026-10-11T12:00:02Z INFO heartbeat\n'))},1000)},cancel(){clearInterval(keepAlive)}})
      return new Response(body,{headers:{'Content-Type':'text/plain'}})
    }
    if (path === '/api/runtime') {
      runtimeRequests++; runtimeInFlight++; runtimeMaxInFlight = Math.max(runtimeMaxInFlight, runtimeInFlight)
      try {
        if (runtimeDelay) await Bun.sleep(runtimeDelay)
        if (runtimeStatus !== 200) return Response.json({error:'Fixture runtime observation failed'}, {status:runtimeStatus})
        return Response.json(Object.fromEntries(state.stacks.map(s=>[s.path,{containers:state.containers[s.path]||[],error:null,loading:false,observedAt:observedAt || new Date().toISOString()}])))
      } finally { runtimeInFlight-- }
    }
    if (path === '/api/operations' && request.method === 'GET') {
      if (historyStatus !== 200) return Response.json({error:'Fixture history unavailable'}, {status:historyStatus})
      const params=new URL(request.url).searchParams
      const matching=state.operations.filter(o=>(!params.get('stack')||o.stack===params.get('stack'))&&(!params.get('kind')||o.kind===params.get('kind'))&&(!params.get('status')||o.status===params.get('status'))&&(!params.get('parentId')||o.parentId===params.get('parentId')))
      const offset=Number(params.get('cursor')||0),limit=Math.min(100,Number(params.get('limit')||50))
      const records=matching.slice(offset,offset+limit)
      return Response.json({records,nextCursor:offset+limit<matching.length?String(offset+limit):'',persistent:false,storageError:''})
    }
    if (path.startsWith('/api/operations/')) { const record=state.operations.find(o=>o.id===path.split('/').pop());return record?Response.json(record):Response.json({error:'Operation not found'},{status:404}) }
    if (path === '/api/operations' && request.method === 'POST') {
      const body=await request.json();const stack=state.stacks.find(s=>s.path===body.stack)
      actions.push({path,body})
      if(!stack)return Response.json({error:'Stack not found'},{status:404})
      if(body.revision!==state.refresh.revision)return Response.json({error:'Desired revision changed; review the stack again'},{status:409})
      const id = crypto.randomUUID()
      const operation={id,rootOperationId:id,version:1,kind:'action',trigger:body.action,stack:body.stack,revision:body.revision,status:holdActions?'running':'completed',message:holdActions?'Waiting for execution lock':'Stack action completed',startedAt:new Date().toISOString(),...(holdActions?{}:{finishedAt:new Date().toISOString()}),steps:[]}
      state.operations.unshift(operation);publish('operation.upsert',operation)
      if (holdActions && work) {
        work = {...work, sequence:work.sequence+1, operations:[...work.operations,operation]}
        publish('work.upsert',{schemaVersion:2,epoch:work.epoch,sequence:work.sequence,operations:[operation]})
      }
      if (holdActions) return Response.json({operationId:id},{status:202})
      stack.requiresAcknowledgement=false;stack.status='synced';stack.statusReason='';stack.lastSyncError=''
      publish('stack.upsert',{record:stack})
      return Response.json({operationId:operation.id},{status:202})
    }
    if(path==='/api/stack/configuration')return Response.json({content:'services:\n  web:\n    image: ghcr.io/example/application:2026.10\n    restart: unless-stopped\n',applied:{revision:'previous123',content:'services:\n  web:\n    image: ghcr.io/example/application:2026.09\n    restart: unless-stopped\n'}})
    if (path === '/api/refresh-status') return Response.json(state.refresh)
    if (path.startsWith('/api/stacks/containers/')) {
      return Response.json(state.containers[decodeURIComponent(path.slice('/api/stacks/containers/'.length))] || [])
    }
    if (path === '/api/update' || path === '/api/refresh') {
      actions.push({path,body:null})
      const status = path === '/api/update' ? updateStatus : refreshStatus
      return Response.json({accepted:status === 202},{status})
    }
    if (path === '/api/events') {
      let current: ReadableStreamDefaultController<Uint8Array>
      const body = new ReadableStream<Uint8Array>({
        start(controller) {
          current = controller; streams.add(controller)
          controller.enqueue(encode('stack.snapshot', { records: state.stacks }))
          controller.enqueue(encode('refresh.status', state.refresh))
          controller.enqueue(encode('update.snapshot', { progress: [] }))
          if (work) controller.enqueue(encode('work.snapshot', work))
        },
        cancel() { streams.delete(current) },
      })
      return new Response(body, { headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache', Connection: 'keep-alive' } })
    }
    return new Response('Not found', { status: 404 })
  },
})
