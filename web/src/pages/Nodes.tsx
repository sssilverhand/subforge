import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listNodes, createNode, getNodeInbounds, createInbound, type Node } from '@/lib/api'
import { Plus, ChevronDown, ChevronUp, Circle } from 'lucide-react'

const PROTOCOLS = ['vless-xhttp', 'vless-reality', 'vless-ws', 'hysteria2']

function InboundForm({ nodeId, onClose }: { nodeId: string; onClose: () => void }) {
  const qc = useQueryClient()
  const [form, setForm] = useState({
    tag: '', protocol: 'vless-xhttp', port: 443,
    settings: '{\n  "path": "/",\n  "host": ""\n}',
  })
  const [err, setErr] = useState('')

  const create = useMutation({
    mutationFn: () => {
      let settings: Record<string, unknown>
      try { settings = JSON.parse(form.settings) } catch { throw new Error('Invalid JSON in settings') }
      return createInbound(nodeId, { tag: form.tag, protocol: form.protocol, port: form.port, settings })
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['inbounds', nodeId] }); onClose() },
    onError: (e: Error) => setErr(e.message),
  })

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-card border border-border rounded-lg w-full max-w-md p-6" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-semibold mb-4">Добавить inbound</h2>
        <div className="space-y-3">
          <div>
            <label className="text-sm mb-1 block">Tag</label>
            <input value={form.tag} onChange={(e) => setForm({ ...form, tag: e.target.value })}
              placeholder="vless-xhttp-in"
              className="w-full px-3 py-2 rounded bg-muted border border-border text-sm font-mono focus:outline-none" />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-sm mb-1 block">Протокол</label>
              <select value={form.protocol} onChange={(e) => setForm({ ...form, protocol: e.target.value })}
                className="w-full px-3 py-2 rounded bg-muted border border-border text-sm focus:outline-none">
                {PROTOCOLS.map((p) => <option key={p}>{p}</option>)}
              </select>
            </div>
            <div>
              <label className="text-sm mb-1 block">Порт</label>
              <input type="number" value={form.port} onChange={(e) => setForm({ ...form, port: +e.target.value })}
                className="w-full px-3 py-2 rounded bg-muted border border-border text-sm focus:outline-none" />
            </div>
          </div>
          <div>
            <label className="text-sm mb-1 block">Settings (JSON)</label>
            <textarea value={form.settings} onChange={(e) => setForm({ ...form, settings: e.target.value })}
              rows={6}
              className="w-full px-3 py-2 rounded bg-muted border border-border text-xs font-mono focus:outline-none resize-none" />
          </div>
          {err && <p className="text-red-400 text-xs">{err}</p>}
        </div>
        <div className="flex gap-3 mt-4">
          <button onClick={onClose} className="flex-1 py-2 rounded border border-border text-sm hover:bg-muted">Отмена</button>
          <button onClick={() => create.mutate()} disabled={create.isPending}
            className="flex-1 py-2 rounded bg-primary text-primary-foreground text-sm font-medium hover:opacity-90 disabled:opacity-50">
            {create.isPending ? 'Создание...' : 'Добавить'}
          </button>
        </div>
      </div>
    </div>
  )
}

function NodeCard({ node }: { node: Node }) {
  const [expanded, setExpanded] = useState(false)
  const [showAddInbound, setShowAddInbound] = useState(false)
  const { data } = useQuery({
    queryKey: ['inbounds', node.id],
    queryFn: () => getNodeInbounds(node.id),
    enabled: expanded,
  })
  const inbounds = data?.data.items ?? []

  return (
    <div className="bg-card border border-border rounded-lg overflow-hidden">
      <div className="px-5 py-4 flex items-center justify-between cursor-pointer"
           onClick={() => setExpanded(!expanded)}>
        <div className="flex items-center gap-3">
          <Circle size={8} className={node.is_active ? 'text-green-400 fill-green-400' : 'text-red-400 fill-red-400'} />
          <div>
            <div className="font-medium">{node.name}</div>
            <div className="text-xs text-muted-foreground font-mono">{node.public_host}</div>
          </div>
        </div>
        <div className="flex items-center gap-4 text-muted-foreground">
          {node.agent_version && <span className="text-xs">agent {node.agent_version}</span>}
          {expanded ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
        </div>
      </div>

      {expanded && (
        <div className="border-t border-border px-5 py-4">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-sm font-medium">Inbounds</h3>
            <button onClick={() => setShowAddInbound(true)}
              className="flex items-center gap-1 text-xs px-3 py-1 rounded bg-muted hover:bg-muted/80 border border-border">
              <Plus size={12} /> Добавить
            </button>
          </div>
          {inbounds.length === 0 ? (
            <p className="text-xs text-muted-foreground">Нет inbounds</p>
          ) : (
            <div className="space-y-1">
              {inbounds.map((ib) => (
                <div key={ib.id} className="flex items-center gap-3 text-xs bg-muted/30 rounded px-3 py-2">
                  <Circle size={6} className={ib.is_active ? 'text-green-400 fill-green-400' : 'text-red-400 fill-red-400'} />
                  <span className="font-mono font-medium">{ib.protocol}</span>
                  <span className="text-muted-foreground">:{ib.port}</span>
                  <span className="text-muted-foreground">{ib.tag}</span>
                </div>
              ))}
            </div>
          )}
          {node.xray_api_addr && (
            <div className="mt-3 text-xs text-muted-foreground">
              xray gRPC: <span className="font-mono">{node.xray_api_addr}</span>
            </div>
          )}
          {node.hy2_api_url && (
            <div className="text-xs text-muted-foreground">
              hysteria2 API: <span className="font-mono">{node.hy2_api_url}</span>
            </div>
          )}
        </div>
      )}

      {showAddInbound && <InboundForm nodeId={node.id} onClose={() => setShowAddInbound(false)} />}
    </div>
  )
}

function CreateNodeModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient()
  const [form, setForm] = useState({
    name: '', public_host: '',
    xray_api_addr: '', hy2_api_url: '', hy2_api_secret: '',
    agent_url: '', agent_secret: '',
  })

  const f = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm({ ...form, [k]: e.target.value })

  const create = useMutation({
    mutationFn: () => createNode({
      name: form.name,
      public_host: form.public_host,
      xray_api_addr: form.xray_api_addr || undefined,
      hy2_api_url: form.hy2_api_url || undefined,
      hy2_api_secret: form.hy2_api_secret || undefined,
      agent_url: form.agent_url || undefined,
      agent_secret: form.agent_secret || undefined,
    }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['nodes'] }); onClose() },
  })

  const row = (label: string, key: keyof typeof form, placeholder = '') => (
    <div>
      <label className="text-sm mb-1 block">{label}</label>
      <input value={form[key]} onChange={f(key)} placeholder={placeholder}
        className="w-full px-3 py-2 rounded bg-muted border border-border text-sm font-mono focus:outline-none" />
    </div>
  )

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-card border border-border rounded-lg w-full max-w-md p-6 max-h-[90vh] overflow-auto"
           onClick={(e) => e.stopPropagation()}>
        <h2 className="font-semibold mb-4">Добавить ноду</h2>
        <div className="space-y-3">
          {row('Имя *', 'name', 'Node 1')}
          {row('Public host *', 'public_host', '5.8.248.41')}
          <div className="border-t border-border pt-3">
            <p className="text-xs text-muted-foreground mb-2">xray</p>
            {row('xray gRPC API', 'xray_api_addr', '127.0.0.1:10085')}
          </div>
          <div className="border-t border-border pt-3">
            <p className="text-xs text-muted-foreground mb-2">hysteria2</p>
            {row('hysteria2 API URL', 'hy2_api_url', 'http://127.0.0.1:11451')}
            {row('hysteria2 API secret', 'hy2_api_secret')}
          </div>
          <div className="border-t border-border pt-3">
            <p className="text-xs text-muted-foreground mb-2">Agent (опционально)</p>
            {row('Agent URL', 'agent_url', 'http://5.8.248.41:9090')}
            {row('Agent secret', 'agent_secret')}
          </div>
        </div>
        <div className="flex gap-3 mt-5">
          <button onClick={onClose} className="flex-1 py-2 rounded border border-border text-sm hover:bg-muted">Отмена</button>
          <button onClick={() => create.mutate()} disabled={create.isPending || !form.name || !form.public_host}
            className="flex-1 py-2 rounded bg-primary text-primary-foreground text-sm font-medium hover:opacity-90 disabled:opacity-50">
            {create.isPending ? 'Создание...' : 'Добавить'}
          </button>
        </div>
      </div>
    </div>
  )
}

export default function NodesPage() {
  const [showCreate, setShowCreate] = useState(false)
  const { data, isLoading } = useQuery({ queryKey: ['nodes'], queryFn: listNodes })
  const nodes = data?.data.items ?? []

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold">Ноды</h1>
        <button onClick={() => setShowCreate(true)}
          className="flex items-center gap-2 px-4 py-2 rounded bg-primary text-primary-foreground text-sm font-medium hover:opacity-90">
          <Plus size={16} /> Добавить ноду
        </button>
      </div>

      <div className="space-y-3">
        {isLoading && <p className="text-muted-foreground text-sm">Загрузка...</p>}
        {!isLoading && nodes.length === 0 && (
          <div className="bg-card border border-border rounded-lg px-5 py-8 text-center text-muted-foreground text-sm">
            Нет нод. Добавьте первую.
          </div>
        )}
        {nodes.map((n) => <NodeCard key={n.id} node={n} />)}
      </div>

      {showCreate && <CreateNodeModal onClose={() => setShowCreate(false)} />}
    </div>
  )
}
