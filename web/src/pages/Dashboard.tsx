import { useQuery } from '@tanstack/react-query'
import { listSubs, listNodes, fmtBytes } from '@/lib/api'
import { Users, Server, TrendingUp, AlertCircle } from 'lucide-react'

function StatCard({ label, value, icon: Icon, sub }: {
  label: string; value: string | number; icon: React.ElementType; sub?: string
}) {
  return (
    <div className="bg-card border border-border rounded-lg p-5">
      <div className="flex items-center justify-between mb-3">
        <span className="text-sm text-muted-foreground">{label}</span>
        <Icon size={16} className="text-muted-foreground" />
      </div>
      <div className="text-2xl font-bold">{value}</div>
      {sub && <div className="text-xs text-muted-foreground mt-1">{sub}</div>}
    </div>
  )
}

export default function DashboardPage() {
  const { data: subsData } = useQuery({
    queryKey: ['subs'],
    queryFn: () => listSubs(200),
  })
  const { data: nodesData } = useQuery({
    queryKey: ['nodes'],
    queryFn: listNodes,
  })

  const subs = subsData?.data.items ?? []
  const nodes = nodesData?.data.items ?? []

  const active = subs.filter((s) => s.is_enabled && !s.is_expired && !s.is_traffic_exceeded).length
  const expired = subs.filter((s) => s.is_expired || s.is_traffic_exceeded).length
  const totalUsed = subs.reduce((acc, s) => acc + s.traffic_used_bytes, 0)

  return (
    <div>
      <h1 className="text-xl font-semibold mb-6">Dashboard</h1>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <StatCard label="Всего подписок" value={subs.length} icon={Users} />
        <StatCard label="Активных" value={active} icon={TrendingUp}
          sub={`${subs.length - active} неактивных`} />
        <StatCard label="Проблемных" value={expired} icon={AlertCircle} />
        <StatCard label="Нод" value={nodes.length} icon={Server}
          sub={`${nodes.filter((n) => n.is_active).length} активных`} />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Recent subs */}
        <div className="bg-card border border-border rounded-lg">
          <div className="px-5 py-4 border-b border-border">
            <h2 className="font-medium">Последние подписки</h2>
          </div>
          <div className="divide-y divide-border">
            {subs.slice(0, 8).map((s) => (
              <div key={s.id} className="px-5 py-3 flex items-center justify-between">
                <div>
                  <div className="text-sm font-medium">{s.name ?? s.token.slice(0, 8)}</div>
                  <div className="text-xs text-muted-foreground">
                    {fmtBytes(s.traffic_used_bytes)} / {fmtBytes(s.traffic_limit_bytes)}
                  </div>
                </div>
                <span className={`text-xs px-2 py-0.5 rounded-full ${
                  s.is_enabled && !s.is_expired && !s.is_traffic_exceeded
                    ? 'bg-green-900 text-green-300'
                    : 'bg-red-900 text-red-300'
                }`}>
                  {s.is_expired ? 'expired' : s.is_traffic_exceeded ? 'overlimit' : s.is_enabled ? 'active' : 'disabled'}
                </span>
              </div>
            ))}
            {subs.length === 0 && (
              <div className="px-5 py-8 text-center text-muted-foreground text-sm">
                Нет подписок
              </div>
            )}
          </div>
        </div>

        {/* Traffic */}
        <div className="bg-card border border-border rounded-lg">
          <div className="px-5 py-4 border-b border-border">
            <h2 className="font-medium">Суммарный трафик</h2>
          </div>
          <div className="p-5">
            <div className="text-3xl font-bold mb-1">{fmtBytes(totalUsed)}</div>
            <div className="text-sm text-muted-foreground">использовано всеми подписками</div>
          </div>
        </div>
      </div>
    </div>
  )
}
