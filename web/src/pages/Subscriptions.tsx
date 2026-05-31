import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listSubs, createSub, deleteSub, enableSub, disableSub, resetTraffic,
  listNodes, getNodeInbounds, fmtBytes, fmtDate,
  type Subscription, type Inbound,
} from '@/lib/api'
import { Plus, Copy, ToggleLeft, ToggleRight, Trash2, RefreshCw, ChevronDown, ChevronUp } from 'lucide-react'

function Badge({ ok, label }: { ok: boolean; label: string }) {
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full ${ok ? 'bg-green-900 text-green-300' : 'bg-red-900 text-red-300'}`}>
      {label}
    </span>
  )
}

function SubRow({ sub, onAction }: { sub: Subscription; onAction: () => void }) {
  const [expanded, setExpanded] = useState(false)
  const qc = useQueryClient()

  const mutOpts = { onSuccess: () => { qc.invalidateQueries({ queryKey: ['subs'] }); onAction() } }
  const toggle = useMutation({ mutationFn: () => sub.is_enabled ? disableSub(sub.id) : enableSub(sub.id), ...mutOpts })
  const remove = useMutation({ mutationFn: () => deleteSub(sub.id), ...mutOpts })
  const reset  = useMutation({ mutationFn: () => resetTraffic(sub.id), ...mutOpts })

  const name = sub.name ?? sub.token.slice(0, 12)
  const isOk = sub.is_enabled && !sub.is_expired && !sub.is_traffic_exceeded
  const pct = sub.traffic_limit_bytes
    ? Math.min(100, (sub.traffic_used_bytes / sub.traffic_limit_bytes) * 100)
    : 0

  const copy = () => navigator.clipboard.writeText(sub.sub_url)

  return (
    <>
      <tr className="border-b border-border hover:bg-muted/30 cursor-pointer"
          onClick={() => setExpanded(!expanded)}>
        <td className="px-4 py-3">
          <div className="font-medium text-sm">{name}</div>
          <div className="text-xs text-muted-foreground font-mono">{sub.token.slice(0, 12)}…</div>
        </td>
        <td className="px-4 py-3">
          <Badge ok={isOk}
            label={sub.is_expired ? 'expired' : sub.is_traffic_exceeded ? 'overlimit' : sub.is_enabled ? 'active' : 'disabled'} />
        </td>
        <td className="px-4 py-3">
          <div className="text-sm">{fmtBytes(sub.traffic_used_bytes)} / {fmtBytes(sub.traffic_limit_bytes)}</div>
          {sub.traffic_limit_bytes && (
            <div className="mt-1 h-1.5 w-24 bg-muted rounded-full overflow-hidden">
              <div className={`h-full rounded-full ${pct > 80 ? 'bg-red-500' : 'bg-green-500'}`}
                   style={{ width: `${pct}%` }} />
            </div>
          )}
        </td>
        <td className="px-4 py-3 text-sm">{fmtDate(sub.expires_at)}</td>
        <td className="px-4 py-3" onClick={(e) => e.stopPropagation()}>
          <div className="flex items-center gap-1">
            <button onClick={copy} title="Copy link" className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground">
              <Copy size={14} />
            </button>
            <button onClick={() => toggle.mutate()} title="Toggle"
              className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground">
              {sub.is_enabled ? <ToggleRight size={14} /> : <ToggleLeft size={14} />}
            </button>
            <button onClick={() => reset.mutate()} title="Reset traffic"
              className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground">
              <RefreshCw size={14} />
            </button>
            <button onClick={() => { if (confirm('Удалить?')) remove.mutate() }} title="Delete"
              className="p-1.5 rounded hover:bg-muted text-red-400 hover:text-red-300">
              <Trash2 size={14} />
            </button>
          </div>
        </td>
        <td className="px-4 py-3 text-muted-foreground">
          {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
        </td>
      </tr>
      {expanded && (
        <tr className="bg-muted/20 border-b border-border">
          <td colSpan={6} className="px-6 py-3">
            <div className="text-xs space-y-1 text-muted-foreground">
              <div>Sub URL: <span className="font-mono text-foreground">{sub.sub_url}</span></div>
              <div className="flex gap-4">
                <a href={`${sub.sub_url}?client=singbox`} target="_blank"
                   className="text-blue-400 hover:underline">Sing-box</a>
                <a href={`${sub.sub_url}?client=clash`} target="_blank"
                   className="text-blue-400 hover:underline">Clash</a>
                <a href={`${sub.sub_url}?client=raw`} target="_blank"
                   className="text-blue-400 hover:underline">Raw</a>
              </div>
              {sub.last_used_at && <div>Последнее использование: {new Date(sub.last_used_at).toLocaleString('ru-RU')}</div>}
            </div>
          </td>
        </tr>
      )}
    </>
  )
}

function CreateModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [selectedInbounds, setSelectedInbounds] = useState<string[]>([])
  const [limitGB, setLimitGB] = useState('')
  const [expiryDays, setExpiryDays] = useState('')

  const { data: nodesData } = useQuery({ queryKey: ['nodes'], queryFn: listNodes })
  const nodes = nodesData?.data.items ?? []

  const [expandedNode, setExpandedNode] = useState<string | null>(null)
  const { data: inboundsData } = useQuery({
    queryKey: ['inbounds', expandedNode],
    queryFn: () => getNodeInbounds(expandedNode!),
    enabled: !!expandedNode,
  })

  const toggleInbound = (id: string) =>
    setSelectedInbounds((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id],
    )

  const create = useMutation({
    mutationFn: () => createSub({
      name: name || undefined,
      inbound_ids: selectedInbounds,
      traffic_limit_bytes: limitGB ? Math.round(parseFloat(limitGB) * 1024 ** 3) : undefined,
      expires_at: expiryDays
        ? new Date(Date.now() + parseInt(expiryDays) * 86400_000).toISOString()
        : undefined,
    }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['subs'] })
      onClose()
    },
  })

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-card border border-border rounded-lg w-full max-w-lg p-6" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-semibold mb-4">Новая подписка</h2>

        <div className="space-y-4">
          <div>
            <label className="text-sm mb-1 block">Имя (опционально)</label>
            <input value={name} onChange={(e) => setName(e.target.value)}
              placeholder="Например: Иван iPhone"
              className="w-full px-3 py-2 rounded bg-muted border border-border text-sm focus:outline-none" />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-sm mb-1 block">Лимит (GB, пусто=∞)</label>
              <input type="number" value={limitGB} onChange={(e) => setLimitGB(e.target.value)}
                placeholder="100"
                className="w-full px-3 py-2 rounded bg-muted border border-border text-sm focus:outline-none" />
            </div>
            <div>
              <label className="text-sm mb-1 block">Срок (дней, пусто=∞)</label>
              <input type="number" value={expiryDays} onChange={(e) => setExpiryDays(e.target.value)}
                placeholder="30"
                className="w-full px-3 py-2 rounded bg-muted border border-border text-sm focus:outline-none" />
            </div>
          </div>

          <div>
            <label className="text-sm mb-2 block">Протоколы</label>
            <div className="border border-border rounded divide-y divide-border max-h-48 overflow-auto">
              {nodes.map((node) => (
                <div key={node.id}>
                  <button
                    className="w-full text-left px-3 py-2 text-sm hover:bg-muted flex items-center justify-between"
                    onClick={() => setExpandedNode(expandedNode === node.id ? null : node.id)}
                  >
                    <span>🖥 {node.name}</span>
                    {expandedNode === node.id ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
                  </button>
                  {expandedNode === node.id && (
                    <div className="bg-muted/50 px-3 py-2 space-y-1">
                      {(inboundsData?.data.items ?? []).map((ib: Inbound) => (
                        <label key={ib.id} className="flex items-center gap-2 text-xs cursor-pointer">
                          <input type="checkbox" checked={selectedInbounds.includes(ib.id)}
                            onChange={() => toggleInbound(ib.id)} />
                          <span className="font-mono">{ib.protocol}</span>
                          <span className="text-muted-foreground">:{ib.port}</span>
                          <span className="text-muted-foreground">{ib.tag}</span>
                        </label>
                      ))}
                      {(inboundsData?.data.items ?? []).length === 0 && (
                        <div className="text-xs text-muted-foreground">Нет inbounds</div>
                      )}
                    </div>
                  )}
                </div>
              ))}
            </div>
            <div className="text-xs text-muted-foreground mt-1">Выбрано: {selectedInbounds.length}</div>
          </div>
        </div>

        <div className="flex gap-3 mt-6">
          <button onClick={onClose}
            className="flex-1 py-2 rounded border border-border text-sm hover:bg-muted">
            Отмена
          </button>
          <button
            onClick={() => create.mutate()}
            disabled={create.isPending}
            className="flex-1 py-2 rounded bg-primary text-primary-foreground text-sm font-medium hover:opacity-90 disabled:opacity-50">
            {create.isPending ? 'Создание...' : 'Создать'}
          </button>
        </div>
      </div>
    </div>
  )
}

export default function SubscriptionsPage() {
  const [showCreate, setShowCreate] = useState(false)
  const { data, isLoading } = useQuery({ queryKey: ['subs'], queryFn: () => listSubs(200) })
  const subs = data?.data.items ?? []

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold">Подписки</h1>
        <button onClick={() => setShowCreate(true)}
          className="flex items-center gap-2 px-4 py-2 rounded bg-primary text-primary-foreground text-sm font-medium hover:opacity-90">
          <Plus size={16} /> Создать
        </button>
      </div>

      <div className="bg-card border border-border rounded-lg overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="border-b border-border text-left">
              <th className="px-4 py-3 text-xs text-muted-foreground font-medium">Имя / токен</th>
              <th className="px-4 py-3 text-xs text-muted-foreground font-medium">Статус</th>
              <th className="px-4 py-3 text-xs text-muted-foreground font-medium">Трафик</th>
              <th className="px-4 py-3 text-xs text-muted-foreground font-medium">Истекает</th>
              <th className="px-4 py-3 text-xs text-muted-foreground font-medium">Действия</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr><td colSpan={6} className="px-4 py-8 text-center text-muted-foreground text-sm">Загрузка...</td></tr>
            )}
            {!isLoading && subs.length === 0 && (
              <tr><td colSpan={6} className="px-4 py-8 text-center text-muted-foreground text-sm">Нет подписок</td></tr>
            )}
            {subs.map((s) => <SubRow key={s.id} sub={s} onAction={() => {}} />)}
          </tbody>
        </table>
      </div>

      {showCreate && <CreateModal onClose={() => setShowCreate(false)} />}
    </div>
  )
}
