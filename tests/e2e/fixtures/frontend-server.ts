// Isolated browser-test backend. Never imported by the production application.
import { fixtureData } from './frontend-data'

let state = fixtureData()
let updateStatus = 202
let refreshStatus = 202
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
      state = fixtureData(); updateStatus = 202; refreshStatus = 202
      return Response.json({ ok: true })
    }
    if (path === '/test/state') return Response.json(state)
    if (path === '/test/configure') {
      const config = await request.json()
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
    if (path === '/api/runtime') return Response.json(Object.fromEntries(state.stacks.map(s=>[s.path,{containers:state.containers[s.path]||[],error:null,loading:false,observedAt:new Date().toISOString()}])))
    if (path === '/api/operations' && request.method === 'GET') {
      const params=new URL(request.url).searchParams
      const records=state.operations.filter(o=>(!params.get('stack')||o.stack===params.get('stack'))&&(!params.get('kind')||o.kind===params.get('kind')))
      return Response.json({records,nextCursor:'',persistent:false,storageError:''})
    }
    if (path.startsWith('/api/operations/')) { const record=state.operations.find(o=>o.id===path.split('/').pop());return record?Response.json(record):Response.json({error:'Operation not found'},{status:404}) }
    if (path === '/api/operations' && request.method === 'POST') {
      const body=await request.json();const stack=state.stacks.find(s=>s.path===body.stack)
      if(!stack)return Response.json({error:'Stack not found'},{status:404})
      if(body.revision!==state.refresh.revision)return Response.json({error:'Desired revision changed; review the stack again'},{status:409})
      const operation={id:crypto.randomUUID(),kind:'action',trigger:body.action,stack:body.stack,revision:body.revision,status:'completed',message:'Stack action completed',startedAt:new Date().toISOString(),finishedAt:new Date().toISOString(),steps:[]}
      state.operations.unshift(operation);publish('operation.upsert',operation)
      stack.requiresAcknowledgement=false;stack.status='synced';stack.statusReason='';stack.lastSyncError=''
      publish('stack.upsert',{record:stack})
      return Response.json({operationId:operation.id},{status:202})
    }
    if(path==='/api/stack/configuration')return Response.json({content:'services:\n  web:\n    image: ghcr.io/example/application:2026.10\n    restart: unless-stopped\n',applied:{revision:'previous123',content:'services:\n  web:\n    image: ghcr.io/example/application:2026.09\n    restart: unless-stopped\n'}})
    if (path === '/api/refresh-status') return Response.json(state.refresh)
    if (path.startsWith('/api/stacks/containers/')) {
      return Response.json(state.containers[decodeURIComponent(path.slice('/api/stacks/containers/'.length))] || [])
    }
    if (path === '/api/update') return Response.json({ accepted: updateStatus === 202 }, { status: updateStatus })
    if (path === '/api/refresh') return Response.json({ accepted: refreshStatus === 202 }, { status: refreshStatus })
    if (path === '/api/events') {
      let current: ReadableStreamDefaultController<Uint8Array>
      const body = new ReadableStream<Uint8Array>({
        start(controller) {
          current = controller; streams.add(controller)
          controller.enqueue(encode('stack.snapshot', { records: state.stacks }))
          controller.enqueue(encode('refresh.status', state.refresh))
          controller.enqueue(encode('update.snapshot', { progress: [] }))
        },
        cancel() { streams.delete(current) },
      })
      return new Response(body, { headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache', Connection: 'keep-alive' } })
    }
    return new Response('Not found', { status: 404 })
  },
})
